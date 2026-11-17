package browsercli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

type fakeExtension struct {
	mu       sync.Mutex
	requests []map[string]any
	reply    func(string, map[string]any, int) (map[string]any, *ToolError)
}

func startFakeExtension(t *testing.T, reply func(string, map[string]any, int) (map[string]any, *ToolError)) (*fakeExtension, string, string) {
	t.Helper()
	h := &fakeExtension{reply: reply}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			bridgeJSON(w, map[string]any{"name": "zpilot-tool-bridge", "protocol": bridgeProtocol, "version": Version})
		case "/ready":
			bridgeJSON(w, map[string]bool{"connected": true})
		case "/call":
			var req struct {
				Tool string         `json:"tool"`
				Args map[string]any `json:"args"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if r.Header.Get("Authorization") != "Bearer "+testPair {
				http.Error(w, "wrong ID", 401)
				return
			}
			h.mu.Lock()
			h.requests = append(h.requests, req.Args)
			n := len(h.requests)
			h.mu.Unlock()
			w.Header().Set("Content-Type", "application/x-ndjson")
			enc := json.NewEncoder(w)
			_ = enc.Encode(map[string]any{"type": "progress", "message": "working"})
			w.(http.Flusher).Flush()
			result, toolErr := h.reply(req.Tool, req.Args, n)
			if r.Context().Err() != nil {
				return
			}
			if toolErr != nil {
				_ = enc.Encode(map[string]any{"type": "result", "ok": false, "error": toolErr})
			} else {
				_ = enc.Encode(map[string]any{"type": "result", "ok": true, "result": result})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	previous := newBridgeHTTPClient
	newBridgeHTTPClient = func() *http.Client {
		client := bridgeHTTPClient()
		client.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
		}
		return client
	}
	t.Cleanup(func() { server.Close(); newBridgeHTTPClient = previous })
	root := t.TempDir()
	config := defaultConfig()
	config.AutoLaunch = false
	config.ExtensionID = testPair
	config.OutputDir = filepath.Join(root, "out")
	config.StateDir = filepath.Join(root, "state")
	path := filepath.Join(root, "config.json")
	if err := atomicJSON(path, config); err != nil {
		t.Fatal(err)
	}
	return h, path, root
}
func runCLI(t *testing.T, tool, config string, args ...string) (int, map[string]any, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	argv := append([]string{"--config", config, "--json"}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code := Run(ctx, tool, argv, &out, &errOut)
	var result map[string]any
	if out.Len() > 0 {
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("invalid result JSON: %s (%v)", out.String(), err)
		}
	}
	return code, result, errOut.String()
}

func TestSearchIndependentCLI(t *testing.T) {
	h, config, _ := startFakeExtension(t, func(tool string, a map[string]any, n int) (map[string]any, *ToolError) {
		if tool != "web_search" {
			t.Errorf("tool=%s", tool)
		}
		return map[string]any{"engine": "google", "query": a["query"], "results": []any{map[string]any{"title": "Go", "url": "https://go.dev", "snippet": "Go language"}}}, nil
	})
	code, r, stderr := runCLI(t, "web_search", config, "--query", "--literal-prompt-with-hyphens")
	if code != 0 || !strings.Contains(stderr, "working") || r["query"] != "--literal-prompt-with-hyphens" {
		t.Fatalf("code=%d r=%v stderr=%s", code, r, stderr)
	}
	if h.requests[0]["max_results"] != float64(10) {
		t.Fatalf("default results: %v", h.requests)
	}
	code, _, stderr = runCLI(t, "web_search", config, "--query", "golang", "--result-count", "3")
	if code != 0 || h.requests[1]["max_results"] != float64(3) {
		t.Fatalf("override: %s %v", stderr, h.requests)
	}
	code, _, _ = runCLI(t, "web_search", config, "--query", "x", "--result_count", "0")
	if code != 2 || len(h.requests) != 2 {
		t.Fatal("invalid flags reached host")
	}
}

func imageReply(_ string, a map[string]any, n int) (map[string]any, *ToolError) {
	chat := fmt.Sprintf("https://chatgpt.com/c/chat%d", n)
	if a["conversation_url"] != nil {
		chat = a["conversation_url"].(string)
	}
	return map[string]any{"conversation_url": chat, "images": []any{map[string]any{"data": testPNG, "mime": "image/png", "width": 1, "height": 1}}}, nil
}

func TestImageSaveContinueRotateAndExplicitChat(t *testing.T) {
	h, config, root := startFakeExtension(t, imageReply)
	for i := 0; i < 3; i++ {
		code, r, stderr := runCLI(t, "generate_image", config, "--prompt", "a fox", "--name", "fox.png", "--path", filepath.Join(root, "images"), "--session", "story", "--chat_limit", "2", "--style", "watercolor")
		if code != 0 {
			t.Fatal(stderr)
		}
		files := r["saved_files"].([]any)
		if _, err := os.Stat(files[0].(string)); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(fmt.Sprint(r), testPNG) {
			t.Fatal("base64 leaked into output")
		}
	}
	if h.requests[0]["new_chat"] != true || h.requests[1]["conversation_url"] != "https://chatgpt.com/c/chat1" || h.requests[2]["new_chat"] != true {
		t.Fatalf("rotation: %v", h.requests)
	}
	if len(h.requests[2]["reference_images"].([]any)) != 2 {
		t.Fatal("rotation did not carry references")
	}
	for _, name := range []string{"fox.png", "fox-2.png", "fox-3.png"} {
		if _, err := os.Stat(filepath.Join(root, "images", name)); err != nil {
			t.Fatal(err)
		}
	}
	code, _, stderr := runCLI(t, "generate_image", config, "--prompt", "fox jumping", "--session", "story", "--chat_id", "chosen", "--chat_limit", "1")
	if code != 0 || h.requests[3]["conversation_url"] != "https://chatgpt.com/c/chosen" {
		t.Fatalf("explicit chat: %s %v", stderr, h.requests)
	}
}

func TestMusicSaveAndContinue(t *testing.T) {
	dl := filepath.Join(t.TempDir(), "track.mp3")
	if err := os.WriteFile(dl, []byte("ID3audio"), 0600); err != nil {
		t.Fatal(err)
	}
	h, config, root := startFakeExtension(t, func(_ string, a map[string]any, n int) (map[string]any, *ToolError) {
		chat := "https://gemini.google.com/app/abc"
		if x, ok := a["conversation_url"].(string); ok {
			chat = x
		}
		return map[string]any{"conversation_url": chat, "reply_text": "notes", "files": map[string]any{"audio": map[string]any{"path": dl, "mime": "audio/mpeg"}}, "cover": map[string]any{"data": testPNG, "mime": "image/png"}}, nil
	})
	for i := 0; i < 2; i++ {
		code, _, stderr := runCLI(t, "generate_music", config, "--prompt", "calm piano", "--path", filepath.Join(root, "music"), "--name", "intro")
		if code != 0 {
			t.Fatal(stderr)
		}
	}
	if h.requests[1]["conversation_url"] != "https://gemini.google.com/app/abc" {
		t.Fatal("music chat not persisted")
	}
	for _, p := range []string{dl, filepath.Join(root, "music", "intro.mp3"), filepath.Join(root, "music", "intro.cover.png"), filepath.Join(root, "music", "intro.txt"), filepath.Join(root, "music", "intro-2.mp3")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadFullPageAndBrowserFiles(t *testing.T) {
	_, config, root := startFakeExtension(t, func(tool string, a map[string]any, n int) (map[string]any, *ToolError) {
		if tool == "webpage_read" {
			if a["max_chars"] != float64(0) || a["offset"] != float64(0) {
				t.Error("save must request full page")
			}
			return map[string]any{"content": "# Example\nHello", "format": "markdown"}, nil
		}
		return map[string]any{"message": "screenshot", "image": map[string]any{"data": testPNG, "mime": "image/jpeg"}}, nil
	})
	page := filepath.Join(root, "page.md")
	code, _, stderr := runCLI(t, "webpage_read", config, "--url", "example.com", "--path", page, "--offset", "100")
	if code != 0 {
		t.Fatal(stderr)
	}
	b, err := os.ReadFile(page)
	if err != nil || string(b) != "# Example\nHello" {
		t.Fatalf("saved %q: %v", b, err)
	}
	shot := filepath.Join(root, "shot.jpg")
	code, r, stderr := runCLI(t, "zagent_in_chrome", config, "--action", "screenshot", "--path", shot)
	if code != 0 {
		t.Fatal(stderr)
	}
	if _, err := os.Stat(shot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(r), testPNG) {
		t.Fatal("base64 leaked")
	}
}

func TestErrorsAndValidation(t *testing.T) {
	h, config, _ := startFakeExtension(t, func(_ string, a map[string]any, n int) (map[string]any, *ToolError) {
		return nil, &ToolError{Code: "RATE_LIMITED", Message: "limit reached"}
	})
	for _, tc := range []struct {
		tool string
		args []string
	}{{"generate_image", nil}, {"generate_music", []string{"--prompt", "x", "--chat_id", "x", "--new_chat"}}, {"generate_image", []string{"--prompt", "x", "--chat_id", "https://chatgpt.com.evil/c/id"}}, {"zagent_in_chrome", []string{"--action", "click", "--coordinate", "1,2,3"}}, {"web_search", []string{"--query", "x", "--unknown", "y"}}, {"generate_image", []string{"--prompt", "x", "--name", "x.jpg", "--format", "png"}}} {
		code, _, _ := runCLI(t, tc.tool, config, tc.args...)
		if code != 2 {
			t.Errorf("%s %v code=%d", tc.tool, tc.args, code)
		}
	}
	if len(h.requests) != 0 {
		t.Fatal("validation contacted host")
	}
	code, _, stderr := runCLI(t, "generate_image", config, "--prompt", "x")
	if code != 1 || !strings.Contains(stderr, `"code":"RATE_LIMITED"`) {
		t.Fatalf("error %d %s", code, stderr)
	}
}

func TestConfigAndChatURLs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"chrome_profile":"Work","auto_launch":false,"state_dir":"state","browser_args":["--user-agent=hello world"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if c.ResultCount != 10 || c.ImageChatLimit != 20 || c.AutoLaunch || c.StateDir != filepath.Join(dir, "state") || len(c.BrowserArgs) != 1 {
		t.Fatalf("config: %+v", c)
	}
	if err := initConfig(path); err == nil {
		t.Fatal("init overwrote existing config")
	}
	for _, s := range []string{"https://chatgpt.com/c/id/extra", "https://chatgpt.com:443/c/id", "https://evil.test/c/id", "file:///c/id", "../foo"} {
		if _, err := chatURL("generate_image", s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
	if s, err := chatURL("generate_music", "abc_123"); err != nil || s != "https://gemini.google.com/app/abc_123" {
		t.Fatal(s, err)
	}
}

func TestConcurrentMediaCommands(t *testing.T) {
	h, config, _ := startFakeExtension(t, func(tool string, a map[string]any, n int) (map[string]any, *ToolError) {
		time.Sleep(30 * time.Millisecond)
		return imageReply(tool, a, n)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type outcome struct {
		code     int
		out, err string
	}
	finished := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() {
			var out, errOut bytes.Buffer
			code := Run(ctx, "generate_image", []string{"--config", config, "--prompt", "a fox", "--json"}, &out, &errOut)
			finished <- outcome{code, out.String(), errOut.String()}
		}()
	}
	var stateFile string
	for i := 0; i < 2; i++ {
		r := <-finished
		if r.code != 0 {
			t.Fatalf("%d %s", r.code, r.err)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(r.out), &result); err != nil {
			t.Fatal(err)
		}
		stateFile = result["session_file"].(string)
	}
	b, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var s mediaSession
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 2 || s.Items[0].Chat != s.Items[1].Chat || s.Items[0].File == s.Items[1].File {
		t.Fatalf("session was overwritten: %+v", s)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) != 2 || h.requests[1]["conversation_url"] != "https://chatgpt.com/c/chat1" {
		t.Fatalf("concurrent commands did not continue chat: %v", h.requests)
	}
}

func TestExactOutputDoesNotOverwrite(t *testing.T) {
	h, config, root := startFakeExtension(t, imageReply)
	path := filepath.Join(root, "image.png")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "generate_image", config, "--prompt", "a fox", "--output", path)
	if code != 1 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("%d %s", code, stderr)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) != 0 {
		t.Fatal("generation ran before checking destination")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "original" {
		t.Fatal("output replaced")
	}
}

func TestLockAndCancellation(t *testing.T) {
	dir := t.TempDir()
	unlock, err := lockState(context.Background(), dir, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if release, err := lockState(ctx, dir, func(string) {}); err == nil {
		release()
		t.Fatal("second command acquired owned lock")
	}
	_, config, _ := startFakeExtension(t, func(_ string, a map[string]any, n int) (map[string]any, *ToolError) {
		time.Sleep(100 * time.Millisecond)
		return map[string]any{}, nil
	})
	c, err := loadConfig(config, true)
	if err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if _, err := (&Client{Config: c}).Call(ctx2, "web_search", map[string]any{"query": "x"}); err == nil {
		t.Fatal("cancelled call succeeded")
	}
}
