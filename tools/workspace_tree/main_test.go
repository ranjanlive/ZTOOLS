package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()

	// Create directory structure:
	// dir/
	//   ├── src/
	//   │   ├── main.go
	//   │   └── utils.go
	//   ├── docs/
	//   │   └── readme.md
	//   ├── node_modules/   (should be ignored)
	//   │   └── dummy.js
	//   ├── .git/           (should be ignored)
	//   │   └── config
	//   └── config.json

	_ = os.MkdirAll(filepath.Join(dir, "src"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "node_modules"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	_ = os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "src", "utils.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "docs", "readme.md"), []byte("# Docs"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "node_modules", "dummy.js"), []byte("// ignored"), 0644)
	_ = os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644)

	return dir
}

func TestTreeBasic(t *testing.T) {
	dir := setupTestDir(t)

	req := Request{
		Path:     dir,
		MaxDepth: 3,
		Format:   "tree",
	}

	out, isErr := runTree(req)
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}

	// Must contain src and docs
	if !strings.Contains(out, "src/") || !strings.Contains(out, "docs/") {
		t.Errorf("expected src/ and docs/ in tree output:\n%s", out)
	}

	// Must NOT contain node_modules or .git
	if strings.Contains(out, "node_modules") {
		t.Errorf("node_modules was not ignored:\n%s", out)
	}
	if strings.Contains(out, ".git") {
		t.Errorf(".git was not ignored:\n%s", out)
	}
}

func TestTreeExtensionFilter(t *testing.T) {
	dir := setupTestDir(t)

	req := Request{
		Path:       dir,
		MaxDepth:   3,
		Format:     "tree",
		Extensions: []string{".go"},
	}

	out, isErr := runTree(req)
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}

	if !strings.Contains(out, "main.go") {
		t.Errorf("expected main.go in output:\n%s", out)
	}
	if strings.Contains(out, "readme.md") {
		t.Errorf("readme.md should have been filtered out:\n%s", out)
	}
	if strings.Contains(out, "config.json") {
		t.Errorf("config.json should have been filtered out:\n%s", out)
	}
}

func TestTreeJSONFormat(t *testing.T) {
	dir := setupTestDir(t)

	req := Request{
		Path:     dir,
		MaxDepth: 3,
		Format:   "json",
	}

	out, isErr := runTree(req)
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}

	var data map[string]interface{}
	err := json.Unmarshal([]byte(out), &data)
	if err != nil {
		t.Fatalf("failed to parse json output: %v", err)
	}

	if data["total_files"] == 0 {
		t.Errorf("expected total_files > 0, got %v", data["total_files"])
	}
}

func TestTreeDepthLimit(t *testing.T) {
	dir := setupTestDir(t)

	req := Request{
		Path:     dir,
		MaxDepth: 1,
		Format:   "tree",
	}

	out, isErr := runTree(req)
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}

	// At depth 1, child files inside src/ should not appear
	if strings.Contains(out, "main.go") {
		t.Errorf("main.go should not appear at depth 1:\n%s", out)
	}
}
