package browsercli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type mediaItem struct {
	Chat    string    `json:"chat"`
	File    string    `json:"file"`
	Prompt  string    `json:"prompt"`
	Created time.Time `json:"created"`
}
type mediaSession struct {
	Name  string      `json:"name"`
	Chat  string      `json:"chat"`
	Style string      `json:"style"`
	Items []mediaItem `json:"items"`
}

var chatIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func chatURL(tool, id string) (string, error) {
	if id == "" {
		return "", nil
	}
	host, prefix := "chatgpt.com", "/c/"
	if tool == "generate_music" {
		host, prefix = "gemini.google.com", "/app/"
	}
	if strings.Contains(id, "://") {
		u, err := url.Parse(id)
		if err != nil || u.Scheme != "https" || u.Host != host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, prefix) {
			return "", fmt.Errorf("chat_id must be an ID or https://%s%s<ID>", host, prefix)
		}
		id = strings.TrimSuffix(strings.TrimPrefix(u.Path, prefix), "/")
	}
	if !chatIDPattern.MatchString(id) {
		return "", fmt.Errorf("invalid chat_id %q", id)
	}
	return "https://" + host + prefix + id, nil
}

func stateRoot(c Config, tool string) (string, error) {
	base := c.StateDir
	if base == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(dir, "zpilot", "cli")
	}
	identity := c.ChromeProfile + "\n" + c.UserDataDir + "\n" + c.ExtensionID
	hash := sha256.Sum256([]byte(identity))
	root := filepath.Join(base, tool, hex.EncodeToString(hash[:8]))
	// Preserve media sessions written before removal of the old transport. This
	// lookup is migration only; no socket is opened or needed for connections.
	if _, err := os.Stat(root); os.IsNotExist(err) {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		oldIdentity := c.ChromeProfile + "\n" + c.UserDataDir + "\n" + filepath.Join(cache, "zpilot", "zbrowser.sock") + "\n" + c.ExtensionID
		oldHash := sha256.Sum256([]byte(oldIdentity))
		previous := filepath.Join(base, tool, hex.EncodeToString(oldHash[:8]))
		if info, err := os.Stat(previous); err == nil && info.IsDir() {
			return previous, nil
		}
	}
	return root, nil
}

// One process owns state changes for a media tool/profile at a time, including
// the browser call. This prevents stale chat counters and lost session updates.
func lockState(ctx context.Context, dir string, progress func(string)) (func(), error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock := filepath.Join(dir, ".lock")
	announced := false
	for {
		err := os.Mkdir(lock, 0700)
		if err == nil {
			owner := filepath.Join(lock, "owner.txt")
			if err := os.WriteFile(owner, []byte(fmt.Sprintf("pid=%d\nstarted=%s\n", os.Getpid(), time.Now().Format(time.RFC3339))), 0600); err != nil {
				_ = os.Remove(lock)
				return nil, err
			}
			return func() { _ = os.Remove(owner); _ = os.Remove(lock) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if !announced {
			progress("Waiting for another media command; if a process crashed, inspect " + filepath.Join(lock, "owner.txt") + " and remove the abandoned lock directory.")
			announced = true
		}
		t := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, fmt.Errorf("waiting for lock %s: %w", lock, ctx.Err())
		case <-t.C:
		}
	}
}

func generate(ctx context.Context, tool string, a arguments, request map[string]any, client *Client) (map[string]any, error) {
	c := client.Config
	name := a.str("session", c.Session)
	if !sessionPattern.MatchString(name) {
		return nil, fmt.Errorf("session must be 1-64 letters, digits, underscores or hyphens, starting with a letter or digit")
	}
	root, err := stateRoot(c, tool)
	if err != nil {
		return nil, err
	}
	progress := func(s string) {
		if client.Progress != nil {
			fmt.Fprintln(client.Progress, s)
		}
	}
	unlock, err := lockState(ctx, root, progress)
	if err != nil {
		return nil, err
	}
	defer unlock()
	stateFile := filepath.Join(root, name+".json")
	s := mediaSession{Name: name, Items: []mediaItem{}}
	if b, err := os.ReadFile(stateFile); err == nil {
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("read session %s: %w", stateFile, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if style := a.str("style", ""); style != "" {
		s.Style = style
	}
	chat, err := chatURL(tool, a.str("chat_id", ""))
	if err != nil {
		return nil, err
	}
	explicit := chat != ""
	if !explicit {
		chat = s.Chat
	}
	if chat != "" {
		if _, err := chatURL(tool, chat); err != nil {
			return nil, fmt.Errorf("invalid saved chat in %s: %w", stateFile, err)
		}
	}
	limit := c.ImageChatLimit
	if tool == "generate_music" {
		limit = c.MusicChatLimit
	}
	limit = a.integer("chat_limit", limit)
	count := 0
	for _, item := range s.Items {
		if item.Chat == chat && chat != "" {
			count++
		}
	}
	temporary := a.boolean("temporary_chat", false)
	newChat := temporary || a.boolean("new_chat", false) || chat == "" || (!explicit && count >= limit)
	if !newChat {
		request["conversation_url"] = chat
	}
	if tool == "generate_image" {
		request["new_chat"] = newChat
	}
	prompt := a.str("prompt", "")
	if s.Style != "" && (newChat || a.str("style", "") != "") {
		request["prompt"] = "Style for this series: " + s.Style + "\n\n" + prompt
	}
	if tool == "generate_image" && newChat && !temporary && len(s.Items) > 0 {
		refs, _ := request["reference_images"].([]string)
		remaining := a.integer("carry_references", 2)
		for i := len(s.Items) - 1; i >= 0 && remaining > 0 && len(refs) < 10; i-- {
			item := s.Items[i]
			if st, err := os.Stat(item.File); err == nil && st.Mode().IsRegular() {
				refs = append(refs, item.File)
				remaining--
			}
		}
		if len(refs) > 0 {
			request["reference_images"] = refs
			request["prompt"] = "Continue the existing image series; match the attached references.\n\n" + request["prompt"].(string)
		}
	}
	ext := ".mp3"
	if tool == "generate_image" {
		ext = map[string]string{"png": ".png", "jpeg": ".jpg", "webp": ".webp"}[request["format"].(string)]
	}
	defaultName := cleanName(prompt) + ext
	target, exact, err := outputTarget(a, c, defaultName)
	if err != nil {
		return nil, err
	}
	if filepath.Ext(target) == "" {
		target += ext
	}
	// Discover invalid destinations before spending a generation.
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return nil, err
	}
	if exact {
		if _, err := os.Lstat(target); err == nil {
			return nil, fmt.Errorf("output already exists: %s", target)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	result, err := client.Call(ctx, tool, request)
	if err != nil {
		return nil, err
	}
	var primary string
	if tool == "generate_image" {
		images, ok := result["images"].([]any)
		if !ok || len(images) == 0 {
			return nil, fmt.Errorf("browser returned no images")
		}
		image, ok := images[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid image result")
		}
		if want := extensionFor(stringValue(image, "mime")); want != ext {
			return nil, fmt.Errorf("browser returned unexpected image type %q", stringValue(image, "mime"))
		}
		primary, err = saveBlob(ctx, target, image, exact)
	} else {
		files, _ := result["files"].(map[string]any)
		audio, _ := files["audio"].(map[string]any)
		if audio == nil {
			return nil, fmt.Errorf("browser returned no audio file")
		}
		primary, err = copyDownload(ctx, target, audio, exact)
	}
	if err != nil {
		return nil, err
	}
	saved := []string{primary}
	base := strings.TrimSuffix(primary, filepath.Ext(primary))
	if cover, ok := result["cover"].(map[string]any); ok {
		p, e := saveBlob(ctx, base+".cover"+extensionFor(stringValue(cover, "mime")), cover, false)
		if e != nil {
			return nil, fmt.Errorf("saved %s, but cover failed: %w", primary, e)
		}
		saved = append(saved, p)
	}
	if text := stringValue(result, "reply_text"); text != "" {
		p, e := saveBytes(ctx, base+".txt", []byte(text+"\n"), false)
		if e != nil {
			return nil, fmt.Errorf("saved %s, but notes failed: %w", primary, e)
		}
		saved = append(saved, p)
	}
	result["saved_files"] = saved
	result["session"] = name
	result["new_chat"] = newChat
	result["session_file"] = stateFile
	returnedChat := stringValue(result, "conversation_url")
	if !temporary {
		if returnedChat == "" {
			if newChat {
				return nil, fmt.Errorf("saved %s, but browser did not return the new conversation URL; cannot persist chat continuity", primary)
			}
			returnedChat = chat
		}
		if returnedChat != "" {
			normalized, e := chatURL(tool, returnedChat)
			if e != nil {
				return nil, fmt.Errorf("saved %s, but browser returned an invalid conversation URL: %w", primary, e)
			}
			returnedChat = normalized
		}
		s.Chat = returnedChat
	}
	s.Items = append(s.Items, mediaItem{Chat: returnedChat, File: primary, Prompt: prompt, Created: time.Now()})
	if err := atomicJSON(stateFile, s); err != nil {
		return nil, fmt.Errorf("saved %s, but could not save session: %w", primary, err)
	}
	metadata := map[string]any{"prompt": prompt, "prompt_sent": request["prompt"], "created": time.Now(), "result": result}
	b, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, err
	}
	metaFile, err := saveBytes(ctx, base+".json", append(b, '\n'), false)
	if err != nil {
		return nil, fmt.Errorf("saved %s, but metadata failed: %w", primary, err)
	}
	result["metadata_file"] = metaFile
	return result, nil
}
