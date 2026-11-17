package browsercli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func atomicJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".zpilot-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// createFile never overwrites existing files, including symlinks. Automatic names
// receive a suffix; an explicitly requested exact output must be unused.
func createFile(ctx context.Context, path string, exact bool, write func(io.Writer) error) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	base, ext := strings.TrimSuffix(path, filepath.Ext(path)), filepath.Ext(path)
	for n := 1; ; n++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		candidate := path
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
		f, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) && !exact {
			continue
		}
		if err != nil {
			return "", err
		}
		err = write(f)
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err == nil {
			err = ce
		}
		if err != nil {
			_ = os.Remove(candidate)
			return "", err
		}
		return candidate, nil
	}
}

func saveBytes(ctx context.Context, path string, b []byte, exact bool) (string, error) {
	return createFile(ctx, path, exact, func(w io.Writer) error { _, err := w.Write(b); return err })
}

func saveBlob(ctx context.Context, path string, blob map[string]any, exact bool) (string, error) {
	data := stringValue(blob, "data")
	if data == "" {
		return "", fmt.Errorf("browser returned no file data")
	}
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("decode returned file: %w", err)
	}
	p, err := saveBytes(ctx, path, b, exact)
	if err != nil {
		return "", err
	}
	delete(blob, "data")
	blob["saved_to"] = p
	return p, nil
}

func copyDownload(ctx context.Context, path string, file map[string]any, exact bool) (string, error) {
	src := stringValue(file, "path")
	if src == "" {
		return "", fmt.Errorf("download has no local file path")
	}
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("download remains at %s: %w", src, err)
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("download is not a regular file: %s", src)
	}
	// Keep Chrome's original download: a local CLI should not delete the user's files.
	p, err := createFile(ctx, path, exact, func(w io.Writer) error { _, err := io.Copy(w, in); return err })
	if err != nil {
		return "", fmt.Errorf("download remains at %s: %w", src, err)
	}
	file["saved_to"] = p
	return p, nil
}

func extensionFor(mime string) string {
	switch strings.Split(mime, ";")[0] {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "video/mp4":
		return ".mp4"
	case "application/pdf":
		return ".pdf"
	case "multipart/related":
		return ".mhtml"
	}
	return ".bin"
}

func outputTarget(a arguments, c Config, defaultName string) (string, bool, error) {
	if exact := a.str("output", ""); exact != "" {
		p, err := filepath.Abs(exact)
		return p, true, err
	}
	dir := a.str("path", c.OutputDir)
	if dir == "" {
		dir = "."
	}
	name := a.str("name", defaultName)
	if filepath.Ext(name) == "" {
		name += filepath.Ext(defaultName)
	}
	return filepath.Join(dir, cleanName(name)), false, nil
}

func saveResult(ctx context.Context, tool string, a arguments, c Config, r map[string]any) error {
	path := a.str("path", "")
	switch tool {
	case "web_search":
		if path != "" {
			b, err := json.MarshalIndent(r, "", "  ")
			if err != nil {
				return err
			}
			p, err := saveBytes(ctx, path, append(b, '\n'), true)
			if err != nil {
				return err
			}
			r["saved_to"] = p
		}
	case "webpage_read":
		if path != "" {
			p, err := saveBytes(ctx, path, []byte(stringValue(r, "content")), true)
			if err != nil {
				return err
			}
			r["saved_to"] = p
		}
	case "zagent_in_chrome":
		var saved []string
		for _, key := range []string{"image", "page_file", "file"} {
			blob, ok := r[key].(map[string]any)
			if !ok {
				continue
			}
			name := stringValue(blob, "filename")
			if name == "" {
				name = a.str("action", "browser") + extensionFor(stringValue(blob, "mime"))
			}
			target := filepath.Join(c.OutputDir, cleanName(name))
			exact := false
			if path != "" {
				st, err := os.Stat(path)
				if strings.HasSuffix(path, "/") || strings.HasSuffix(path, `\`) || (err == nil && st.IsDir()) {
					target = filepath.Join(path, cleanName(name))
				} else {
					target = path
					exact = true
				}
			}
			var p string
			var err error
			if key == "file" {
				p, err = copyDownload(ctx, target, blob, exact)
			} else {
				p, err = saveBlob(ctx, target, blob, exact)
			}
			if err != nil {
				return err
			}
			saved = append(saved, p)
		}
		if len(saved) > 0 {
			r["saved_files"] = saved
		}
	}
	return nil
}

func printResult(w io.Writer, tool string, r map[string]any) error {
	switch tool {
	case "web_search":
		fmt.Fprintf(w, "%s results for %q\n", stringValue(r, "engine"), stringValue(r, "query"))
		if s := stringValue(r, "answer"); s != "" {
			fmt.Fprintln(w, s)
		}
		if results, ok := r["results"].([]any); ok {
			for i, raw := range results {
				if x, ok := raw.(map[string]any); ok {
					fmt.Fprintf(w, "\n%d. %s\n   %s\n", i+1, stringValue(x, "title"), stringValue(x, "url"))
					if s := stringValue(x, "snippet"); s != "" {
						fmt.Fprintln(w, "  ", s)
					}
				}
			}
		}
		for _, key := range []string{"notes", "related_questions"} {
			if values, ok := r[key].([]any); ok {
				for _, v := range values {
					fmt.Fprintf(w, "%s: %v\n", key, v)
				}
			}
		}
	case "webpage_read":
		if stringValue(r, "saved_to") == "" {
			fmt.Fprintln(w, stringValue(r, "content"))
		}
		if more, _ := r["has_more"].(bool); more {
			fmt.Fprintf(w, "\nContinue with --offset %d\n", intValue(r, "next_offset"))
		}
	case "generate_image", "generate_music":
		if files, ok := r["saved_files"].([]string); ok {
			for _, p := range files {
				fmt.Fprintln(w, "Saved", p)
			}
		}
		if s := stringValue(r, "conversation_url"); s != "" {
			fmt.Fprintln(w, "Chat:", s)
		}
		if s := stringValue(r, "session"); s != "" {
			fmt.Fprintln(w, "Session:", s)
		}
	default:
		if s := stringValue(r, "message"); s != "" {
			fmt.Fprintln(w, s)
		} else {
			b, _ := json.MarshalIndent(r, "", "  ")
			fmt.Fprintln(w, string(b))
		}
		if files, ok := r["saved_files"].([]string); ok {
			for _, p := range files {
				fmt.Fprintln(w, "Saved", p)
			}
		}
	}
	if p := stringValue(r, "saved_to"); p != "" {
		fmt.Fprintln(w, "Saved", p)
	}
	if warnings, ok := r["warnings"].([]any); ok {
		for _, v := range warnings {
			fmt.Fprintln(w, "Warning:", v)
		}
	}
	return nil
}
