package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Request struct {
	RepoPath       string `json:"repo_path"`
	FromRef        string `json:"from_ref"`
	ToRef          string `json:"to_ref"`
	CurrentVersion string `json:"current_version"`
	Format         string `json:"format"`
	IncludeHashes  bool   `json:"include_hashes"`
}

type Response struct {
	Output  string   `json:"output"`
	IsError bool     `json:"is_error"`
	Media   []string `json:"media"`
}

type CommitInfo struct {
	Hash       string `json:"hash"`
	ShortHash  string `json:"short_hash"`
	Author     string `json:"author"`
	Date       string `json:"date"`
	RawSubject string `json:"raw_subject"`
	Type       string `json:"type"`
	Scope      string `json:"scope,omitempty"`
	Subject    string `json:"subject"`
	IsBreaking bool   `json:"is_breaking"`
}

type ChangelogData struct {
	CurrentVersion string                  `json:"current_version"`
	NextVersion    string                  `json:"next_version"`
	BumpType       string                  `json:"bump_type"`
	TotalCommits   int                     `json:"total_commits"`
	Categories     map[string][]CommitInfo `json:"categories"`
	Breaking       []CommitInfo            `json:"breaking,omitempty"`
}

var convCommitRegex = regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^)]+)\))?(!)?:\s*(.+)$`)

func main() {
	agentJSON := flag.Bool("agent-json", false, "Operate in ZAgent JSON stdio mode")
	repoFlag := flag.String("repo", ".", "Path to git repository")
	fromFlag := flag.String("from", "", "Starting tag or commit")
	toFlag := flag.String("to", "HEAD", "Ending tag, branch, or commit")
	versionFlag := flag.String("version", "", "Current SemVer version")
	formatFlag := flag.String("format", "markdown", "Output format (markdown, json)")
	hashesFlag := flag.Bool("hashes", true, "Include short commit hashes")
	flag.Parse()

	var req Request
	if *agentJSON {
		inputData, err := io.ReadAll(os.Stdin)
		if err == nil && len(inputData) > 0 {
			_ = json.Unmarshal(inputData, &req)
		}
	} else {
		req.RepoPath = *repoFlag
		req.FromRef = *fromFlag
		req.ToRef = *toFlag
		req.CurrentVersion = *versionFlag
		req.Format = *formatFlag
		req.IncludeHashes = *hashesFlag
	}

	if req.RepoPath == "" {
		req.RepoPath = "."
	}
	if req.ToRef == "" {
		req.ToRef = "HEAD"
	}
	if req.Format == "" {
		req.Format = "markdown"
	}

	output, isErr := runChangelog(req)
	resp := Response{
		Output:  output,
		IsError: isErr,
		Media:   []string{},
	}

	if *agentJSON {
		_ = json.NewEncoder(os.Stdout).Encode(resp)
	} else {
		if isErr {
			fmt.Fprintln(os.Stderr, "Error:", output)
			os.Exit(1)
		} else {
			fmt.Println(output)
		}
	}
}

func runChangelog(req Request) (string, bool) {
	if req.RepoPath == "" {
		req.RepoPath = "."
	}
	if req.ToRef == "" {
		req.ToRef = "HEAD"
	}
	if req.Format == "" {
		req.Format = "markdown"
	}
	absRepo, err := filepath.Abs(req.RepoPath)
	if err != nil {
		return fmt.Sprintf("Failed to resolve repo path %s: %v", req.RepoPath, err), true
	}

	// Verify git repo
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = absRepo
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("Path is not a valid git repository: %s", absRepo), true
	}

	// Determine from_ref if empty
	fromRef := req.FromRef
	currentVer := req.CurrentVersion

	if fromRef == "" {
		// Try latest tag
		tagCmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
		tagCmd.Dir = absRepo
		var tagOut bytes.Buffer
		tagCmd.Stdout = &tagOut
		if err := tagCmd.Run(); err == nil {
			fromRef = strings.TrimSpace(tagOut.String())
			if currentVer == "" {
				currentVer = fromRef
			}
		}
	}

	var logArgs []string
	if fromRef != "" {
		logArgs = []string{"log", fmt.Sprintf("%s..%s", fromRef, req.ToRef), "--format=%H|%h|%an|%ad|%s", "--date=short"}
	} else {
		logArgs = []string{"log", req.ToRef, "--format=%H|%h|%an|%ad|%s", "--date=short"}
	}
	logCmd := exec.Command("git", logArgs...)
	logCmd.Dir = absRepo
	var logOut bytes.Buffer
	var logErr bytes.Buffer
	logCmd.Stdout = &logOut
	logCmd.Stderr = &logErr
	if err := logCmd.Run(); err != nil {
		return fmt.Sprintf("git log failed (%s): %s", err, logErr.String()), true
	}

	lines := strings.Split(strings.TrimSpace(logOut.String()), "\n")
	var commits []CommitInfo
	categories := map[string][]CommitInfo{
		"feat":     {},
		"fix":      {},
		"perf":     {},
		"docs":     {},
		"refactor": {},
		"test":     {},
		"chore":    {},
		"other":    {},
	}
	var breaking []CommitInfo
	hasBreaking := false
	hasFeat := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		c := CommitInfo{
			Hash:       parts[0],
			ShortHash:  parts[1],
			Author:     parts[2],
			Date:       parts[3],
			RawSubject: parts[4],
		}

		// Parse Conventional Commit
		match := convCommitRegex.FindStringSubmatch(c.RawSubject)
		if len(match) > 0 {
			c.Type = strings.ToLower(match[1])
			c.Scope = match[2]
			if match[3] == "!" || strings.Contains(strings.ToUpper(c.RawSubject), "BREAKING") {
				c.IsBreaking = true
				hasBreaking = true
			}
			c.Subject = match[4]
		} else {
			c.Type = "other"
			c.Subject = c.RawSubject
			if strings.Contains(strings.ToUpper(c.RawSubject), "BREAKING") {
				c.IsBreaking = true
				hasBreaking = true
			}
		}

		if c.Type == "feat" {
			hasFeat = true
		}

		if c.IsBreaking {
			breaking = append(breaking, c)
		}

		if _, ok := categories[c.Type]; ok {
			categories[c.Type] = append(categories[c.Type], c)
		} else {
			categories["other"] = append(categories["other"], c)
		}
		commits = append(commits, c)
	}

	// Calculate Next SemVer
	bumpType := "patch"
	if hasBreaking {
		bumpType = "major"
	} else if hasFeat {
		bumpType = "minor"
	}

	nextVer := calculateNextVersion(currentVer, bumpType)

	data := ChangelogData{
		CurrentVersion: currentVer,
		NextVersion:    nextVer,
		BumpType:       bumpType,
		TotalCommits:   len(commits),
		Categories:     categories,
		Breaking:       breaking,
	}

	if req.Format == "json" {
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Sprintf("JSON marshal error: %v", err), true
		}
		return string(jsonData), false
	}

	// Generate Markdown
	var sb strings.Builder
	titleVersion := nextVer
	if titleVersion == "" {
		titleVersion = "Unreleased"
	}
	sb.WriteString(fmt.Sprintf("## Release %s\n\n", titleVersion))
	if currentVer != "" {
		sb.WriteString(fmt.Sprintf("*Changes since `%s` (Recommended bump: **%s**)*\n\n", currentVer, bumpType))
	}

	if len(breaking) > 0 {
		sb.WriteString("### ⚠️ BREAKING CHANGES\n")
		for _, c := range breaking {
			scopeStr := ""
			if c.Scope != "" {
				scopeStr = fmt.Sprintf("**%s:** ", c.Scope)
			}
			hashStr := ""
			if req.IncludeHashes {
				hashStr = fmt.Sprintf(" (`%s`)", c.ShortHash)
			}
			sb.WriteString(fmt.Sprintf("- %s%s%s\n", scopeStr, c.Subject, hashStr))
		}
		sb.WriteString("\n")
	}

	sectionOrder := []struct {
		key   string
		title string
		emoji string
	}{
		{"feat", "Features", "🚀"},
		{"fix", "Bug Fixes", "🐛"},
		{"perf", "Performance Improvements", "⚡"},
		{"refactor", "Code Refactoring", "♻️"},
		{"docs", "Documentation", "📝"},
		{"test", "Tests", "🧪"},
		{"chore", "Maintenance & Chores", "🔧"},
		{"other", "Other Changes", "📦"},
	}

	for _, sec := range sectionOrder {
		items := categories[sec.key]
		if len(items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s %s\n", sec.emoji, sec.title))
		for _, c := range items {
			scopeStr := ""
			if c.Scope != "" {
				scopeStr = fmt.Sprintf("**%s:** ", c.Scope)
			}
			hashStr := ""
			if req.IncludeHashes {
				hashStr = fmt.Sprintf(" (`%s`)", c.ShortHash)
			}
			sb.WriteString(fmt.Sprintf("- %s%s%s\n", scopeStr, c.Subject, hashStr))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("---\n*Total Commits: %d*\n", len(commits)))
	return sb.String(), false
}

func calculateNextVersion(current string, bumpType string) string {
	if current == "" {
		switch bumpType {
		case "major":
			return "v1.0.0"
		case "minor":
			return "v0.1.0"
		default:
			return "v0.0.1"
		}
	}

	hasV := strings.HasPrefix(current, "v")
	clean := strings.TrimPrefix(current, "v")
	parts := strings.Split(clean, ".")
	if len(parts) < 3 {
		return current
	}

	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return current
	}

	switch bumpType {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	}

	res := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	if hasV {
		res = "v" + res
	}
	return res
}

