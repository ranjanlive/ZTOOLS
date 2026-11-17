package browsercli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Version = "2.0.0"

type option struct{ name, kind, help string }
type arguments map[string]any
type valueFlag struct {
	args       arguments
	name, kind string
}

func (v valueFlag) String() string   { return "" }
func (v valueFlag) IsBoolFlag() bool { return v.kind == "bool" }
func (v valueFlag) Set(s string) error {
	var x any = s
	var err error
	switch v.kind {
	case "bool":
		x, err = strconv.ParseBool(s)
	case "int":
		x, err = strconv.Atoi(s)
	case "number":
		x, err = strconv.ParseFloat(s, 64)
	case "list":
		previous, _ := v.args[v.name].([]string)
		x = append(previous, s)
	case "numbers":
		var values []float64
		if strings.HasPrefix(strings.TrimSpace(s), "[") {
			err = json.Unmarshal([]byte(s), &values)
		} else {
			for _, part := range strings.Split(s, ",") {
				n, e := strconv.ParseFloat(strings.TrimSpace(part), 64)
				if e != nil {
					err = e
					break
				}
				values = append(values, n)
			}
		}
		x = values
	}
	if err != nil {
		return err
	}
	v.args[v.name] = x
	return nil
}

func options(tool string) []option {
	switch tool {
	case "generate_image":
		return []option{
			{"prompt", "string", "Required image description or edit instruction"},
			{"session", "string", "Saved series name (default from config)"}, {"chat_id", "string", "Continue a ChatGPT chat ID or full conversation URL"},
			{"name", "string", "Output filename; extension is optional"}, {"path", "string", "Directory to save images (default output_dir)"}, {"output", "string", "Exact output file path"},
			{"style", "string", "Style brief saved with the series"}, {"aspect", "string", "Aspect ratio, e.g. 16:9 or portrait"},
			{"format", "string", "png (default), jpeg or webp"}, {"reference_image", "list", "Reference image path; repeat for multiple images"},
			{"chat_limit", "int", "Images per chat before automatic rotation (default image_chat_limit)"}, {"carry_references", "int", "Recent reference images carried into a new chat, 0-4 (default 2)"},
			{"new_chat", "bool", "Start a new saved chat"}, {"temporary_chat", "bool", "Use an unsaved temporary chat"}, {"background", "bool", "Work in a background tab (default true)"},
		}
	case "generate_music":
		return []option{
			{"prompt", "string", "Required music description"}, {"session", "string", "Saved series name (default from config)"}, {"chat_id", "string", "Continue a Gemini chat ID or full conversation URL"},
			{"name", "string", "Output filename; .mp3 is optional"}, {"path", "string", "Directory to save audio (default output_dir)"}, {"output", "string", "Exact .mp3 output file path"},
			{"style", "string", "Sound brief saved with the series"}, {"length", "string", "short or standard"}, {"vocals", "string", "on, instrumental or custom"}, {"genre", "string", "Genre name"},
			{"chat_limit", "int", "Tracks per chat before rotation (default music_chat_limit)"}, {"new_chat", "bool", "Start a new saved chat"},
			{"cover", "bool", "Save cover art (default true; --cover=false disables)"}, {"background", "bool", "Work in a background tab (default true)"},
		}
	case "web_search":
		return []option{
			{"query", "string", "Required search query"}, {"result_count", "int", "Maximum results, 1-50 (default 10)"}, {"engine", "string", "google, bing, duckduckgo, brave or yahoo"},
			{"site", "string", "Restrict results to a site"}, {"time_range", "string", "any, day, week, month or year"}, {"language", "string", "Language code"}, {"region", "string", "Country code"},
			{"fallback", "bool", "Try another engine if blocked (default true)"}, {"background", "bool", "Work in a background tab (default true)"}, {"path", "string", "Save results as JSON to this file"},
		}
	case "webpage_read":
		return []option{
			{"url", "string", "Required page URL"}, {"format", "string", "markdown (default), text or html"}, {"path", "string", "Save full page content to this file"},
			{"main_content_only", "bool", "Strip navigation and sidebars (default true except html)"}, {"include_links", "bool", "Keep links (default true)"}, {"include_images", "bool", "Keep images (default true)"}, {"include_media", "bool", "Keep media (default true)"},
			{"max_chars", "int", "Characters returned, 0-2000000; 0 means all (default 50000)"}, {"offset", "int", "Character offset (default 0)"}, {"wait_for", "string", "CSS selector to wait for"}, {"wait_ms", "int", "Extra wait, 0-30000 ms"},
			{"background", "bool", "Work in background (default true)"}, {"keep_tab", "bool", "Leave the page tab open"},
		}
	case "zagent_in_chrome":
		var opts []option
		for _, name := range strings.Fields("action url ref to_ref text keys value modifiers direction filter selector script pattern filename format") {
			opts = append(opts, option{name, "string", "Browser action argument: " + name})
		}
		for _, name := range strings.Fields("tab_id amount max_chars offset limit width height repeat") {
			opts = append(opts, option{name, "int", "Browser action argument: " + name})
		}
		for _, name := range strings.Fields("viewport_only clear submit expect_download accept foreground close_tabs") {
			opts = append(opts, option{name, "bool", "Browser action argument: " + name})
		}
		for _, name := range strings.Fields("coordinate end_coordinate region") {
			opts = append(opts, option{name, "numbers", "Coordinates as x,y or a JSON array"})
		}
		return append(opts, option{"seconds", "number", "Wait duration"}, option{"file", "list", "Upload file path; repeat for multiple files"}, option{"path", "string", "Save screenshot/download/page to this file or existing directory"})
	}
	return nil
}

func Main(tool string) {
	if len(os.Args) == 2 && os.Args[1] == "--internal-bridge" {
		os.Exit(runEmbeddedBridge())
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if len(os.Args) == 2 && os.Args[1] == "--agent-json" {
		os.Exit(RunAgent(ctx, tool, os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(Run(ctx, tool, os.Args[1:], os.Stdout, os.Stderr))
}

// Run is also used by offline protocol tests; it never calls another CLI executable.
func Run(ctx context.Context, tool string, argv []string, stdout, stderr io.Writer) int {
	a := arguments{}
	fs := flag.NewFlagSet(tool, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath, profile, browserPath, extensionID string
	var init, printConfig, version, jsonOutput, quiet bool
	fs.StringVar(&configPath, "config", "", "Config file (default config.json beside this executable)")
	fs.StringVar(&profile, "profile", "", "Browser profile folder, e.g. Default or Profile 1")
	fs.StringVar(&extensionID, "extension_id", "", "Connection ID copied from the extension popup")
	fs.StringVar(&browserPath, "browser_path", "", "Override browser_path")
	fs.BoolVar(&init, "init_config", false, "Create default config.json without overwriting an existing file")
	fs.BoolVar(&printConfig, "print_config", false, "Print effective configuration and exit")
	fs.BoolVar(&version, "version", false, "Print version and exit")
	fs.BoolVar(&jsonOutput, "json", false, "Print structured JSON results and errors")
	fs.BoolVar(&quiet, "quiet", false, "Suppress progress messages")
	fs.Var(valueFlag{a, "auto_launch", "bool"}, "auto_launch", "Override browser auto launch; use --auto_launch=false to disable")
	fs.Var(valueFlag{a, "timeout_sec", "int"}, "timeout_sec", "Override tool timeout in seconds")
	for _, o := range options(tool) {
		fs.Var(valueFlag{a, o.name, o.kind}, o.name, o.help)
	}
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s [flags]\n\nStandalone ZPilot Browser tool. Copy the extension popup's Connection ID into config.json.\nNo separate native host or registration needed.\nUse underscores or hyphens in flag names. Boolean values: --flag=false.\n\n", tool)
		fs.PrintDefaults()
		if tool == "zagent_in_chrome" {
			fmt.Fprintln(stderr, "\nActions: tabs, new_tab, switch_tab, close_tab, attach_tab, navigate, screenshot, zoom, read_page, find, get_text, click, double_click, triple_click, right_click, hover, drag, type, key, fill, scroll, scroll_to, wait, wait_for, javascript, console, network, upload, download, downloads, save_page, handle_dialog, resize, end")
		}
	}
	// Only normalize option tokens, never prompt text, paths, or other values.
	args := append([]string(nil), argv...)
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		if !strings.HasPrefix(args[i], "-") {
			continue
		}
		parts := strings.SplitN(args[i], "=", 2)
		name := strings.ReplaceAll(strings.TrimLeft(parts[0], "-"), "-", "_")
		if name == "max_results" {
			name = "result_count"
		}
		args[i] = "--" + name
		if len(parts) == 2 {
			args[i] += "=" + parts[1]
			continue
		}
		if f := fs.Lookup(name); f != nil {
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
				i++
			}
		}
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	fail := func(err error, code int) int {
		if jsonOutput {
			payload := map[string]any{"ok": false, "error": map[string]any{"code": "CLI_ERROR", "message": err.Error()}}
			var te *ToolError
			if errors.As(err, &te) {
				payload["error"] = te
			}
			_ = json.NewEncoder(stderr).Encode(payload)
		} else {
			fmt.Fprintln(stderr, "error:", err)
		}
		return code
	}
	if fs.NArg() != 0 {
		return fail(fmt.Errorf("unexpected positional arguments; use named flags (see --help)"), 2)
	}
	if version {
		fmt.Fprintln(stdout, tool, Version)
		return 0
	}
	explicit := configPath != ""
	if !explicit {
		configPath = defaultConfigPath()
	}
	if init {
		if err := initConfig(configPath); err != nil {
			return fail(err, 1)
		}
		fmt.Fprintln(stdout, "Created", configPath)
		return 0
	}
	c, err := loadConfig(configPath, explicit)
	if err != nil {
		return fail(err, 2)
	}
	if profile != "" {
		c.ChromeProfile = profile
	}
	if browserPath != "" {
		c.BrowserPath = browserPath
	}
	if extensionID != "" {
		c.ExtensionID = extensionID
	}
	if err := c.validate(); err != nil {
		return fail(err, 2)
	}
	if v, ok := a["auto_launch"].(bool); ok {
		c.AutoLaunch = v
	}
	delete(a, "auto_launch")
	if printConfig {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(c)
		return 0
	}
	request, timeout, err := prepare(tool, a, c)
	if err != nil {
		return fail(err, 2)
	}
	var progress io.Writer = stderr
	if quiet {
		progress = nil
	}
	client := &Client{Config: c, Progress: progress}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout+c.ConnectTimeout+15)*time.Second)
	defer cancel()
	var result map[string]any
	if tool == "generate_image" || tool == "generate_music" {
		result, err = generate(callCtx, tool, a, request, client)
	} else {
		result, err = client.Call(callCtx, tool, request)
		if err == nil {
			err = saveResult(callCtx, tool, a, c, result)
		}
	}
	if err != nil {
		return fail(err, 1)
	}
	if jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return fail(err, 1)
		}
	} else {
		if err := printResult(stdout, tool, result); err != nil {
			return fail(err, 1)
		}
	}
	return 0
}

func (a arguments) str(key, def string) string {
	if s, ok := a[key].(string); ok {
		return s
	}
	return def
}
func (a arguments) integer(key string, def int) int {
	if n, ok := a[key].(int); ok {
		return n
	}
	return def
}
func (a arguments) boolean(key string, def bool) bool {
	if b, ok := a[key].(bool); ok {
		return b
	}
	return def
}

func prepare(tool string, a arguments, c Config) (map[string]any, int, error) {
	r := map[string]any{}
	for k, v := range a {
		if !oneOf(k, "path", "output", "name", "session", "style", "chat_id", "chat_limit", "carry_references", "reference_image", "file") {
			r[k] = v
		}
	}
	def, lo, hi := 60, 5, 300
	switch tool {
	case "generate_image":
		def, lo, hi = c.ImageTimeout, 30, 900
	case "generate_music":
		def, lo, hi = c.MusicTimeout, 60, 1500
	case "web_search", "webpage_read":
		def = 120
	case "zagent_in_chrome":
		if oneOf(a.str("action", ""), "download", "save_page") || a.boolean("expect_download", false) {
			def = 300
		}
	default:
		return nil, 0, fmt.Errorf("unknown tool %q", tool)
	}
	timeout := a.integer("timeout_sec", def)
	if timeout < lo || timeout > hi {
		return nil, 0, fmt.Errorf("timeout_sec must be %d-%d", lo, hi)
	}
	r["timeout_sec"] = timeout
	required := map[string]string{"generate_image": "prompt", "generate_music": "prompt", "web_search": "query", "webpage_read": "url", "zagent_in_chrome": "action"}[tool]
	if strings.TrimSpace(a.str(required, "")) == "" {
		return nil, 0, fmt.Errorf("--%s is required (see --help)", required)
	}
	if tool == "web_search" {
		n := a.integer("result_count", c.ResultCount)
		if n < 1 || n > 50 {
			return nil, 0, fmt.Errorf("result_count must be 1-50")
		}
		delete(r, "result_count")
		r["max_results"] = n
		r["engine"] = a.str("engine", c.SearchEngine)
		if !oneOf(r["engine"].(string), "google", "bing", "duckduckgo", "brave", "yahoo") {
			return nil, 0, fmt.Errorf("invalid engine")
		}
		if !oneOf(a.str("time_range", "any"), "any", "day", "week", "month", "year") {
			return nil, 0, fmt.Errorf("invalid time_range")
		}
	}
	if tool == "webpage_read" {
		if !oneOf(a.str("format", "markdown"), "markdown", "text", "html") {
			return nil, 0, fmt.Errorf("format must be markdown, text or html")
		}
		for _, v := range []struct {
			key         string
			def, lo, hi int
		}{{"max_chars", 50000, 0, 2000000}, {"offset", 0, 0, 2147483647}, {"wait_ms", 0, 0, 30000}} {
			n := a.integer(v.key, v.def)
			if n < v.lo || n > v.hi {
				return nil, 0, fmt.Errorf("%s must be %d-%d", v.key, v.lo, v.hi)
			}
		}
		if a.str("path", "") != "" {
			r["max_chars"] = 0
			r["offset"] = 0
		}
	}
	if tool == "generate_image" || tool == "generate_music" {
		if !sessionPattern.MatchString(a.str("session", c.Session)) {
			return nil, 0, fmt.Errorf("session must be 1-64 letters, digits, underscores or hyphens, starting with a letter or digit")
		}
		delete(r, "new_chat") // Set explicitly after loading persistent state.
		if a.str("chat_id", "") != "" && (a.boolean("new_chat", false) || a.boolean("temporary_chat", false)) {
			return nil, 0, fmt.Errorf("chat_id cannot be combined with new_chat or temporary_chat")
		}
		if _, err := chatURL(tool, a.str("chat_id", "")); err != nil {
			return nil, 0, err
		}
		if a.str("output", "") != "" && (a.str("name", "") != "" || a.str("path", "") != "") {
			return nil, 0, fmt.Errorf("use --output alone, or --path with optional --name")
		}
		limit, maxLimit := c.ImageChatLimit, 200
		if tool == "generate_music" {
			limit, maxLimit = c.MusicChatLimit, 100
		}
		limit = a.integer("chat_limit", limit)
		if limit < 1 || limit > maxLimit {
			return nil, 0, fmt.Errorf("chat_limit must be 1-%d", maxLimit)
		}
		if tool == "generate_image" {
			format := a.str("format", "png")
			file := a.str("output", a.str("name", ""))
			ext := strings.ToLower(filepath.Ext(file))
			if ext != "" {
				want := map[string]string{".png": "png", ".jpg": "jpeg", ".jpeg": "jpeg", ".webp": "webp"}[ext]
				if want == "" {
					return nil, 0, fmt.Errorf("image filename must end with .png, .jpg, .jpeg or .webp")
				}
				if _, set := a["format"]; set && format != want {
					return nil, 0, fmt.Errorf("filename extension conflicts with --format")
				}
				format = want
			}
			if !oneOf(format, "png", "jpeg", "webp") {
				return nil, 0, fmt.Errorf("format must be png, jpeg or webp")
			}
			r["format"] = format
			r["max_images"] = 1
			carry := a.integer("carry_references", 2)
			if carry < 0 || carry > 4 {
				return nil, 0, fmt.Errorf("carry_references must be 0-4")
			}
			if refs, ok := a["reference_image"].([]string); ok {
				if len(refs) > 10 {
					return nil, 0, fmt.Errorf("at most 10 reference images")
				}
				var paths []string
				for _, ref := range refs {
					p, err := filepath.Abs(ref)
					if err != nil {
						return nil, 0, err
					}
					st, err := os.Stat(p)
					if err != nil || !st.Mode().IsRegular() {
						return nil, 0, fmt.Errorf("reference_image %q must be an existing regular file", ref)
					}
					paths = append(paths, p)
				}
				r["reference_images"] = paths
			}
		} else {
			for _, v := range []struct {
				key     string
				allowed []string
			}{{"length", []string{"short", "standard"}}, {"vocals", []string{"on", "instrumental", "custom"}}} {
				if s := a.str(v.key, ""); s != "" && !oneOf(s, v.allowed...) {
					return nil, 0, fmt.Errorf("invalid %s", v.key)
				}
			}
			if ext := strings.ToLower(filepath.Ext(a.str("output", a.str("name", "")))); ext != "" && ext != ".mp3" {
				return nil, 0, fmt.Errorf("music filename must end with .mp3")
			}
		}
		if name := a.str("name", ""); name != "" && filepath.Base(name) != name {
			return nil, 0, fmt.Errorf("name must be a filename; use --path for its directory")
		}
	}
	if tool == "zagent_in_chrome" {
		if !oneOf(a.str("action", ""), strings.Fields("tabs new_tab switch_tab close_tab attach_tab navigate screenshot zoom read_page find get_text click double_click triple_click right_click hover drag type key fill scroll scroll_to wait wait_for javascript console network upload download downloads save_page handle_dialog resize end")...) {
			return nil, 0, fmt.Errorf("unknown browser action; see --help")
		}
		if files, ok := a["file"].([]string); ok {
			var paths []string
			for _, file := range files {
				p, err := filepath.Abs(file)
				if err != nil {
					return nil, 0, err
				}
				st, err := os.Stat(p)
				if err != nil || !st.Mode().IsRegular() {
					return nil, 0, fmt.Errorf("upload file %q must exist", file)
				}
				paths = append(paths, p)
			}
			r["paths"] = paths
		}
		for _, v := range []struct {
			key string
			n   int
		}{{"coordinate", 2}, {"end_coordinate", 2}, {"region", 4}} {
			if p, ok := a[v.key].([]float64); ok && len(p) != v.n {
				return nil, 0, fmt.Errorf("%s needs %d numbers", v.key, v.n)
			}
		}
	}
	return r, timeout, nil
}
