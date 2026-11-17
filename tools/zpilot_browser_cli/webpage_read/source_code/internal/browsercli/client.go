package browsercli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type ToolError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *ToolError) Error() string { return e.Code + ": " + e.Message }

type Client struct {
	Config   Config
	Progress io.Writer
}

func findBrowser(c Config) (string, error) {
	if c.BrowserPath != "" {
		if p, err := exec.LookPath(c.BrowserPath); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("browser_path %q was not found", c.BrowserPath)
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		for _, rel := range []string{`Google\Chrome\Application\chrome.exe`, `Microsoft\Edge\Application\msedge.exe`, `BraveSoftware\Brave-Browser\Application\brave.exe`, `Chromium\Application\chrome.exe`} {
			for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
				if base := os.Getenv(key); base != "" {
					candidates = append(candidates, filepath.Join(base, rel))
				}
			}
		}
	case "darwin":
		for _, app := range []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium"} {
			candidates = append(candidates, filepath.Join("/Applications", app+".app", "Contents", "MacOS", app))
		}
	default:
		candidates = []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"}
	}
	for _, path := range candidates {
		if p, err := exec.LookPath(path); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no supported browser found; set browser_path in config.json or --browser_path")
}

func (c *Client) Call(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	client, err := c.connectDirect(ctx)
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	body, err := json.Marshal(map[string]any{"tool": tool, "args": args})
	if err != nil {
		return nil, err
	}
	if len(body) > maxBridgeBody {
		return nil, fmt.Errorf("request exceeds 64 MiB")
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", bridgeURL+"/call", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Config.ExtensionID)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extension connection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("extension connection: %s", strings.TrimSpace(string(message)))
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), maxBridgeBody)
	for sc.Scan() {
		var msg struct {
			Type    string         `json:"type"`
			Message string         `json:"message"`
			OK      bool           `json:"ok"`
			Result  map[string]any `json:"result"`
			Error   *ToolError     `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			return nil, fmt.Errorf("invalid extension response: %w", err)
		}
		switch msg.Type {
		case "progress":
			if c.Progress != nil {
				fmt.Fprintln(c.Progress, msg.Message)
			}
		case "result":
			if msg.OK && msg.Result != nil {
				return msg.Result, nil
			}
			if msg.Error != nil {
				return nil, msg.Error
			}
			return nil, fmt.Errorf("extension returned an empty result")
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if sc.Err() != nil {
		return nil, fmt.Errorf("read extension response: %w", sc.Err())
	}
	return nil, fmt.Errorf("extension disconnected before returning a result")
}

func stringValue(m map[string]any, key string) string { s, _ := m[key].(string); return s }
func intValue(m map[string]any, key string) int       { n, _ := m[key].(float64); return int(n) }
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	s = strings.Trim(b.String(), " .")
	runes := []rune(s)
	if len(runes) > 100 {
		ext := filepath.Ext(s)
		if len([]rune(ext)) > 10 {
			ext = ""
		}
		s = string(runes[:100-len([]rune(ext))]) + ext
	}
	if s == "" {
		s = "output"
	}
	base := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	if oneOf(base, "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9") {
		s = "_" + s
	}
	return s
}
