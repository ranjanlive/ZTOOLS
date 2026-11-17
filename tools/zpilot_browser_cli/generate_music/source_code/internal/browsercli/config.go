package browsercli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Config belongs to a CLI installation, not to an agent or the extension.
type Config struct {
	ExtensionID    string   `json:"extension_id"`
	ChromeProfile  string   `json:"profile"`
	BrowserPath    string   `json:"path"`
	BrowserArgs    []string `json:"browser_args"`
	UserDataDir    string   `json:"user_data_dir"`
	AutoLaunch     bool     `json:"auto_launch"`
	OutputDir      string   `json:"output_dir"`
	StateDir       string   `json:"state_dir"`
	Session        string   `json:"session"`
	ImageChatLimit int      `json:"image_chat_limit"`
	MusicChatLimit int      `json:"music_chat_limit"`
	ResultCount    int      `json:"result_count"`
	SearchEngine   string   `json:"search_engine"`
	ImageTimeout   int      `json:"image_timeout_sec"`
	MusicTimeout   int      `json:"music_timeout_sec"`
	ConnectTimeout int      `json:"connect_timeout_sec"`
}

func defaultConfig() Config {
	return Config{ChromeProfile: "Default", BrowserArgs: []string{}, AutoLaunch: true,
		OutputDir: ".", Session: "default", ImageChatLimit: 20, MusicChatLimit: 10,
		ResultCount: 10, SearchEngine: "google", ImageTimeout: 300, MusicTimeout: 600, ConnectTimeout: 45}
}

func defaultConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	// Installers may expose the executable through a symlink in a bin directory.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func loadConfig(path string, explicit bool) (Config, error) {
	c := defaultConfig()
	f, err := os.Open(path)
	if os.IsNotExist(err) && !explicit {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	// A null document is not a configuration object.
	var raw json.RawMessage
	if err := d.Decode(&raw); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}
	if len(raw) == 0 || raw[0] != '{' {
		return c, fmt.Errorf("config %s must be a JSON object", path)
	}
	// Preserve old configs while keeping new files small and easy to read.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return c, err
	}
	for old, current := range map[string]string{"chrome_profile": "profile", "browser_path": "path"} {
		if value, ok := fields[old]; ok {
			if _, duplicate := fields[current]; duplicate {
				return c, fmt.Errorf("config contains both %s and %s; keep %s", old, current, current)
			}
			fields[current] = value
			delete(fields, old)
		}
	}
	raw, _ = json.Marshal(fields)
	object := json.NewDecoder(strings.NewReader(string(raw)))
	object.DisallowUnknownFields()
	if err := object.Decode(&c); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("config must contain exactly one JSON object")
	}
	if err := c.validate(); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}
	// Configured paths are relative to the config, so launching from another cwd is predictable.
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	for _, p := range []*string{&c.StateDir, &c.UserDataDir} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
	if c.OutputDir != "." && c.OutputDir != "" && !filepath.IsAbs(c.OutputDir) {
		c.OutputDir = filepath.Join(base, c.OutputDir)
	}
	return c, nil
}

func (c Config) validate() error {
	for _, v := range []struct {
		name          string
		value, lo, hi int
	}{
		{"image_chat_limit", c.ImageChatLimit, 1, 200}, {"music_chat_limit", c.MusicChatLimit, 1, 100},
		{"result_count", c.ResultCount, 1, 50}, {"image_timeout_sec", c.ImageTimeout, 30, 900},
		{"music_timeout_sec", c.MusicTimeout, 60, 1500}, {"connect_timeout_sec", c.ConnectTimeout, 1, 300},
	} {
		if v.value < v.lo || v.value > v.hi {
			return fmt.Errorf("%s must be %d-%d", v.name, v.lo, v.hi)
		}
	}
	if !oneOf(c.SearchEngine, "google", "bing", "duckduckgo", "brave", "yahoo") {
		return fmt.Errorf("unsupported search_engine %q", c.SearchEngine)
	}
	if c.ChromeProfile == "" || c.Session == "" {
		return fmt.Errorf("profile and session cannot be empty")
	}
	if c.ExtensionID != "" && !pairingPattern.MatchString(c.ExtensionID) {
		return fmt.Errorf("extension_id must be the Connection ID copied from the ZPilot Browser popup")
	}
	return nil
}

func initConfig(path string) error {
	b, _ := json.MarshalIndent(struct {
		ExtensionID string `json:"extension_id"`
		AutoLaunch  bool   `json:"auto_launch"`
		Profile     string `json:"profile"`
		Path        string `json:"path"`
		OutputDir   string `json:"output_dir"`
	}{"", true, "Default", "", "."}, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create config (existing files are preserved): %w", err)
	}
	_, err = f.Write(append(b, '\n'))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func oneOf(s string, options ...string) bool {
	for _, option := range options {
		if s == option {
			return true
		}
	}
	return false
}
