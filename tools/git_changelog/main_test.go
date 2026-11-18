package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCalculateNextVersion(t *testing.T) {
	tests := []struct {
		current  string
		bumpType string
		expected string
	}{
		{"v1.2.3", "patch", "v1.2.4"},
		{"v1.2.3", "minor", "v1.3.0"},
		{"v1.2.3", "major", "v2.0.0"},
		{"0.1.0", "patch", "0.1.1"},
		{"0.1.0", "minor", "0.2.0"},
		{"0.1.0", "major", "1.0.0"},
		{"", "minor", "v0.1.0"},
		{"", "major", "v1.0.0"},
	}

	for _, tt := range tests {
		got := calculateNextVersion(tt.current, tt.bumpType)
		if got != tt.expected {
			t.Errorf("calculateNextVersion(%q, %q) = %q, expected %q", tt.current, tt.bumpType, got, tt.expected)
		}
	}
}

func setupTestGitRepo(t *testing.T) string {
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, string(out))
		}
	}

	run("init")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")

	f1 := filepath.Join(dir, "file1.txt")
	_ = os.WriteFile(f1, []byte("hello"), 0644)
	run("add", ".")
	run("commit", "-m", "feat(core): initial feature release")

	f2 := filepath.Join(dir, "file2.txt")
	_ = os.WriteFile(f2, []byte("world"), 0644)
	run("add", ".")
	run("commit", "-m", "fix(api): resolve timeout issue on disconnect")

	f3 := filepath.Join(dir, "file3.txt")
	_ = os.WriteFile(f3, []byte("docs"), 0644)
	run("add", ".")
	run("commit", "-m", "docs: update quickstart instructions")

	return dir
}

func TestRunChangelog(t *testing.T) {
	repoDir := setupTestGitRepo(t)

	req := Request{
		RepoPath:       repoDir,
		CurrentVersion: "v1.0.0",
		Format:         "markdown",
		IncludeHashes:  true,
	}

	out, isErr := runChangelog(req)
	if isErr {
		t.Fatalf("runChangelog failed: %s", out)
	}

	// Because there is a "feat", bump should be minor -> v1.1.0
	if !strings.Contains(out, "## Release v1.1.0") {
		t.Errorf("expected release title with v1.1.0:\n%s", out)
	}

	// Must contain Features and Bug Fixes sections
	if !strings.Contains(out, "### 🚀 Features") {
		t.Errorf("expected Features section:\n%s", out)
	}
	if !strings.Contains(out, "### 🐛 Bug Fixes") {
		t.Errorf("expected Bug Fixes section:\n%s", out)
	}
	if !strings.Contains(out, "### 📝 Documentation") {
		t.Errorf("expected Documentation section:\n%s", out)
	}

	// Must contain parsed scopes
	if !strings.Contains(out, "**core:**") {
		t.Errorf("expected core scope:\n%s", out)
	}
	if !strings.Contains(out, "**api:**") {
		t.Errorf("expected api scope:\n%s", out)
	}
}

func TestRunChangelogJSON(t *testing.T) {
	repoDir := setupTestGitRepo(t)

	req := Request{
		RepoPath:       repoDir,
		CurrentVersion: "v1.0.0",
		Format:         "json",
	}

	out, isErr := runChangelog(req)
	if isErr {
		t.Fatalf("runChangelog failed: %s", out)
	}

	if !strings.Contains(out, `"bump_type": "minor"`) {
		t.Errorf("expected bump_type minor in JSON:\n%s", out)
	}
	if !strings.Contains(out, `"next_version": "v1.1.0"`) {
		t.Errorf("expected next_version v1.1.0 in JSON:\n%s", out)
	}
}
