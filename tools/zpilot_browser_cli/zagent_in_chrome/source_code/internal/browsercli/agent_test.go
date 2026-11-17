package browsercli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rawAgentArgs(t *testing.T, values map[string]any) map[string]json.RawMessage {
	t.Helper()
	encoded, _ := json.Marshal(values)
	var args map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &args)
	return args
}

func TestAgentArgumentConversion(t *testing.T) {
	argv, err := agentArguments("generate_image", rawAgentArgs(t, map[string]any{"prompt": `literal "quote"; $(echo hi)`, "background": false, "carry_references": 0, "reference_image": []string{"a b.png", "c.png"}, "help": false}), "")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(argv, "\n")
	for _, want := range []string{`--prompt=literal "quote"; $(echo hi)`, "--background=false", "--carry_references=0", "--reference_image=a b.png", "--reference_image=c.png", "--json=true"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %v", want, argv)
		}
	}
	if strings.Contains(joined, "--help") {
		t.Fatal("false help would trigger Go's implicit help flag")
	}
	args, err := agentArguments("zagent_in_chrome", rawAgentArgs(t, map[string]any{"action": "click", "coordinate": []float64{0, 2}}), "")
	if err != nil || !strings.Contains(strings.Join(args, "\n"), "--coordinate=[0,2]") {
		t.Fatal(args, err)
	}
}

func TestAgentRejectsInvalidJSONArguments(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"query":"hello","unknown":1}`, `{"query":"hello","fallback":"false"}`, `{"query":"hello","result_count":1.5}`, `{"query":"hello","max_results":2,"result_count":3}`, `{"query":null}`, `{} {}`} {
		var out bytes.Buffer
		if code := RunAgent(context.Background(), "web_search", strings.NewReader(raw), &out, io.Discard); code != 0 {
			t.Fatal(code)
		}
		var result struct {
			IsError bool `json:"is_error"`
		}
		if json.Unmarshal(out.Bytes(), &result) != nil || !result.IsError {
			t.Fatalf("accepted %s: %s", raw, out.String())
		}
	}
}

func TestAgentDefaultsAndExplicitOutputPaths(t *testing.T) {
	dir := t.TempDir()
	argv, err := agentArguments("generate_image", rawAgentArgs(t, map[string]any{"prompt": "fox"}), dir)
	if err != nil || !strings.Contains(strings.Join(argv, "\n"), "--path="+dir) {
		t.Fatal(argv, err)
	}
	argv, err = agentArguments("generate_image", rawAgentArgs(t, map[string]any{"prompt": "fox", "output": "chosen.png"}), dir)
	if err != nil || strings.Contains(strings.Join(argv, "\n"), "--path=") {
		t.Fatal(argv, err)
	}
	argv, err = agentArguments("webpage_read", rawAgentArgs(t, map[string]any{"url": "https://example.com"}), dir)
	if err != nil || strings.Contains(strings.Join(argv, "\n"), "--path=") {
		t.Fatal(argv, err)
	}
}

func TestAgentMediaResultAndProgress(t *testing.T) {
	_, config, _ := startFakeExtension(t, imageReply)
	dir := t.TempDir()
	note := filepath.Join(dir, "notify.txt")
	t.Setenv("ZAGENT_OUTPUT_DIR", dir)
	t.Setenv("ZAGENT_NOTIFY_FILE", note)
	input, _ := json.Marshal(map[string]any{"config": config, "prompt": "a fox"})
	var out, diagnostics bytes.Buffer
	if code := RunAgent(context.Background(), "generate_image", bytes.NewReader(input), &out, &diagnostics); code != 0 {
		t.Fatal(code)
	}
	var result struct {
		Output  string       `json:"output"`
		IsError bool         `json:"is_error"`
		Media   []agentMedia `json:"media"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Media) != 1 || !strings.HasPrefix(result.Media[0].Path, dir) {
		t.Fatalf("%+v; %s", result, diagnostics.String())
	}
	if _, err := os.Stat(result.Media[0].Path); err != nil {
		t.Fatal(err)
	}
	progress, err := os.ReadFile(note)
	if err != nil || !bytes.Contains(progress, []byte("working")) {
		t.Fatal(string(progress), err)
	}
}

func TestAgentPrintConfigRedactsConnectionID(t *testing.T) {
	config := defaultConfig()
	config.ExtensionID = testPair
	file := filepath.Join(t.TempDir(), "config.json")
	if err := atomicJSON(file, config); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"config": file, "print_config": true})
	var out bytes.Buffer
	RunAgent(context.Background(), "web_search", bytes.NewReader(input), &out, io.Discard)
	if strings.Contains(out.String(), testPair) || !strings.Contains(out.String(), "REDACTED_CONNECTION_ID") {
		t.Fatal(out.String())
	}
}
