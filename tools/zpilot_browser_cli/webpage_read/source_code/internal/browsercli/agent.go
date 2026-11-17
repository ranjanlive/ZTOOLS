package browsercli

// ZAgent's JSON adapter is compiled into every binary: no interpreter, shell,
// wrapper process, or external runtime is needed.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var connectionInText = regexp.MustCompile(`[a-p]{32}\.[a-f0-9]{64}`)

func redactAgent(s string) string {
	return connectionInText.ReplaceAllString(s, "[REDACTED_CONNECTION_ID]")
}

type agentOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *agentOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 32<<20 {
		b.overflow = true
		return 0, fmt.Errorf("agent output exceeds 32 MiB; reduce max_chars or save to a path")
	}
	return b.Buffer.Write(p)
}

type agentProgress struct {
	mu         sync.Mutex
	diagnostic io.Writer
	pending    string
	text       strings.Builder
}

func (p *agentProgress) line(s string) {
	s = redactAgent(s)
	if p.text.Len() < 1<<20 {
		p.text.WriteString(s + "\n")
	}
	_, _ = fmt.Fprintln(p.diagnostic, s)
	if notify := os.Getenv("ZAGENT_NOTIFY_FILE"); notify != "" {
		if f, err := os.OpenFile(notify, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
			_ = json.NewEncoder(f).Encode(map[string]string{"text": s})
			_ = f.Close()
		}
	}
}
func (p *agentProgress) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending += string(data)
	for {
		i := strings.IndexByte(p.pending, '\n')
		if i < 0 {
			break
		}
		p.line(strings.TrimSuffix(p.pending[:i], "\r"))
		p.pending = p.pending[i+1:]
	}
	if len(p.pending) > 1<<20 {
		p.line(p.pending[:1<<20])
		p.pending = ""
	}
	return len(data), nil
}
func (p *agentProgress) finish() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending != "" {
		p.line(p.pending)
		p.pending = ""
	}
	return p.text.String()
}

func agentArguments(tool string, args map[string]json.RawMessage, outputDir string) ([]string, error) {
	kinds := map[string]string{"config": "string", "profile": "string", "extension_id": "string", "browser_path": "string", "auto_launch": "bool", "timeout_sec": "int", "json": "bool", "quiet": "bool", "help": "bool", "h": "bool", "version": "bool", "print_config": "bool", "init_config": "bool"}
	for _, option := range options(tool) {
		kinds[option.name] = option.kind
	}
	if tool == "web_search" {
		kinds["max_results"] = "int"
	}
	if _, a := args["result_count"]; a {
		if _, b := args["max_results"]; b {
			return nil, fmt.Errorf("use result_count or max_results, not both")
		}
	}
	boolean := func(key string) bool { var v bool; _ = json.Unmarshal(args[key], &v); return v }
	stringValue := func(key string) string { var v string; _ = json.Unmarshal(args[key], &v); return v }
	inspection := boolean("help") || boolean("h") || boolean("version") || boolean("print_config") || boolean("init_config")
	if outputDir != "" && !inspection {
		if stringValue("path") == "" && stringValue("output") == "" {
			if tool == "generate_image" || tool == "generate_music" {
				args["path"], _ = json.Marshal(outputDir)
			} else if tool == "zagent_in_chrome" && (oneOf(stringValue("action"), "screenshot", "zoom", "download", "save_page") || boolean("expect_download")) {
				args["path"], _ = json.Marshal(outputDir + string(os.PathSeparator))
			}
		}
		if err := os.MkdirAll(outputDir, 0700); err != nil {
			return nil, err
		}
	}
	if _, ok := args["json"]; !ok {
		args["json"] = json.RawMessage("true")
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var argv []string
	for _, key := range keys {
		raw := args[key]
		kind, ok := kinds[key]
		if !ok {
			return nil, fmt.Errorf("unknown argument %q", key)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("%s cannot be null", key)
		}
		add := func(value string) { argv = append(argv, "--"+key+"="+value) }
		switch kind {
		case "string":
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return nil, fmt.Errorf("%s must be a string", key)
			}
			add(value)
		case "bool":
			var value bool
			if json.Unmarshal(raw, &value) != nil {
				return nil, fmt.Errorf("%s must be a boolean", key)
			}
			if (key == "help" || key == "h") && !value {
				continue
			}
			add(fmt.Sprint(value))
		case "int":
			var value int
			if json.Unmarshal(raw, &value) != nil {
				return nil, fmt.Errorf("%s must be an integer", key)
			}
			add(fmt.Sprint(value))
		case "number":
			var value float64
			if json.Unmarshal(raw, &value) != nil {
				return nil, fmt.Errorf("%s must be a number", key)
			}
			add(string(raw))
		case "list":
			var values []string
			if json.Unmarshal(raw, &values) != nil {
				return nil, fmt.Errorf("%s must be an array of strings", key)
			}
			for _, value := range values {
				add(value)
			}
		case "numbers":
			var values []float64
			if json.Unmarshal(raw, &values) != nil {
				return nil, fmt.Errorf("%s must be an array of numbers", key)
			}
			encoded, _ := json.Marshal(values)
			add(string(encoded))
		}
	}
	return argv, nil
}

type agentMedia struct {
	Path string `json:"path"`
	MIME string `json:"mime"`
}

func agentFiles(value any) []agentMedia {
	paths := map[string]bool{}
	var visit func(any)
	visit = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, child := range t {
				visit(child)
			}
		case map[string]any:
			for key, child := range t {
				if key == "saved_to" || key == "saved_path" {
					if p, ok := child.(string); ok {
						paths[p] = true
					}
				} else if key == "saved_files" {
					if files, ok := child.([]any); ok {
						for _, file := range files {
							if p, ok := file.(string); ok {
								paths[p] = true
							}
						}
					}
				} else {
					visit(child)
				}
			}
		}
	}
	visit(value)
	mimes := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".mp3": "audio/mpeg", ".mp4": "video/mp4", ".pdf": "application/pdf"}
	var files []agentMedia
	for p := range paths {
		mime := mimes[strings.ToLower(filepath.Ext(p))]
		if mime == "" {
			continue
		}
		absolute, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if st, err := os.Stat(absolute); err == nil && st.Mode().IsRegular() {
			files = append(files, agentMedia{absolute, mime})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

// RunAgent prints exactly one ZAgent result object. Tool errors use is_error,
// while the ordinary CLI keeps its existing exit codes and human/JSON output.
func RunAgent(ctx context.Context, tool string, input io.Reader, stdout, stderr io.Writer) int {
	reply := func(text string, failed bool, media []agentMedia) int {
		result := struct {
			Output  string       `json:"output"`
			IsError bool         `json:"is_error"`
			Media   []agentMedia `json:"media,omitempty"`
		}{redactAgent(text), failed, media}
		if json.NewEncoder(stdout).Encode(result) != nil {
			return 1
		}
		return 0
	}
	raw, err := io.ReadAll(io.LimitReader(input, (8<<20)+1))
	if err != nil {
		return reply(err.Error(), true, nil)
	}
	if len(raw) > 8<<20 {
		return reply("agent input exceeds 8 MiB", true, nil)
	}
	var args map[string]json.RawMessage
	if json.Unmarshal(raw, &args) != nil || args == nil {
		return reply("input must be one JSON object", true, nil)
	}
	argv, err := agentArguments(tool, args, os.Getenv("ZAGENT_OUTPUT_DIR"))
	if err != nil {
		return reply(err.Error(), true, nil)
	}
	var out agentOutput
	progress := &agentProgress{diagnostic: stderr}
	code := Run(ctx, tool, argv, &out, progress)
	diagnostics := strings.TrimSpace(progress.finish())
	if out.overflow {
		return reply("agent output exceeds 32 MiB; reduce max_chars or save to a path", true, nil)
	}
	text := strings.TrimSpace(out.String())
	if code != 0 && diagnostics != "" || text == "" {
		text = diagnostics
	}
	if text == "" {
		text = "Tool finished without text output."
	}
	var value any
	_ = json.Unmarshal(out.Bytes(), &value)
	return reply(text, code != 0, agentFiles(value))
}
