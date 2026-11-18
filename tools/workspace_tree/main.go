package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Request structure matching workspace_tree.json schema
type Request struct {
	Path           string   `json:"path"`
	MaxDepth       int      `json:"max_depth"`
	ShowHidden     bool     `json:"show_hidden"`
	ShowSizes      bool     `json:"show_sizes"`
	Extensions     []string `json:"extensions"`
	IgnorePatterns []string `json:"ignore_patterns"`
	Format         string   `json:"format"`
	MaxEntries     int      `json:"max_entries"`
}

// Response structure matching ZAgent tool protocol
type Response struct {
	Output  string   `json:"output"`
	IsError bool     `json:"is_error"`
	Media   []string `json:"media"`
}

type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	IsDir    bool    `json:"is_dir"`
	Size     int64   `json:"size,omitempty"`
	Children []*Node `json:"children,omitempty"`
}

var defaultIgnores = map[string]bool{
	".git":         true,
	"node_modules": true,
	".gradle":      true,
	"build":        true,
	"dist":         true,
	"out":          true,
	"target":       true,
	"bin":          true,
	"obj":          true,
	".idea":        true,
	".vscode":      true,
	"__pycache__":  true,
	".venv":        true,
	"env":          true,
	".DS_Store":    true,
	"Thumbs.db":    true,
	"coverage":     true,
	".next":        true,
	".turbo":       true,
	".astro":       true,
}

func main() {
	agentJSON := flag.Bool("agent-json", false, "Operate in ZAgent JSON stdio mode")
	pathFlag := flag.String("path", ".", "Target directory path")
	depthFlag := flag.Int("depth", 3, "Max directory depth")
	hiddenFlag := flag.Bool("hidden", false, "Show hidden files")
	sizesFlag := flag.Bool("sizes", false, "Show file sizes")
	formatFlag := flag.String("format", "tree", "Output format (tree, json, flat)")
	extFlag := flag.String("ext", "", "Comma-separated file extensions to include")
	maxFlag := flag.Int("max", 500, "Maximum total entries")
	flag.Parse()

	var req Request
	if *agentJSON {
		inputData, err := io.ReadAll(os.Stdin)
		if err == nil && len(inputData) > 0 {
			_ = json.Unmarshal(inputData, &req)
		}
	} else {
		req.Path = *pathFlag
		req.MaxDepth = *depthFlag
		req.ShowHidden = *hiddenFlag
		req.ShowSizes = *sizesFlag
		req.Format = *formatFlag
		req.MaxEntries = *maxFlag
		if *extFlag != "" {
			req.Extensions = strings.Split(*extFlag, ",")
		}
	}

	// Apply defaults
	if req.Path == "" {
		req.Path = "."
	}
	if req.MaxDepth <= 0 {
		req.MaxDepth = 3
	}
	if req.MaxEntries <= 0 {
		req.MaxEntries = 500
	}
	if req.Format == "" {
		req.Format = "tree"
	}

	// Sanitize extensions
	for i, ext := range req.Extensions {
		ext = strings.TrimSpace(strings.ToLower(ext))
		if !strings.HasPrefix(ext, ".") && ext != "" {
			ext = "." + ext
		}
		req.Extensions[i] = ext
	}

	output, isErr := runTree(req)
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

func runTree(req Request) (string, bool) {
	if req.MaxEntries <= 0 {
		req.MaxEntries = 500
	}
	if req.MaxDepth <= 0 {
		req.MaxDepth = 3
	}
	if req.Format == "" {
		req.Format = "tree"
	}
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		return fmt.Sprintf("Failed to resolve path %s: %v", req.Path, err), true
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Sprintf("Path does not exist: %s (%v)", req.Path, err), true
	}
	if !info.IsDir() {
		return fmt.Sprintf("Path is a file, not a directory: %s", req.Path), true
	}

	// Load gitignore patterns if present
	gitignores := loadGitignorePatterns(absPath)
	for _, p := range req.IgnorePatterns {
		if p != "" {
			gitignores = append(gitignores, strings.TrimSpace(p))
		}
	}

	rootNode := &Node{
		Name:  filepath.Base(absPath),
		Path:  absPath,
		IsDir: true,
	}

	totalDirs := 0
	totalFiles := 0
	var totalSize int64 = 0
	truncated := false
	entryCount := 0

	var buildTree func(node *Node, currentDepth int)
	buildTree = func(node *Node, currentDepth int) {
		if currentDepth > req.MaxDepth || entryCount >= req.MaxEntries {
			if entryCount >= req.MaxEntries {
				truncated = true
			}
			return
		}

		entries, err := os.ReadDir(node.Path)
		if err != nil {
			return
		}

		// Sort: directories first, then alphabetical
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() && !entries[j].IsDir() {
				return true
			}
			if !entries[i].IsDir() && entries[j].IsDir() {
				return false
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})

		for _, entry := range entries {
			name := entry.Name()

			// Check hidden
			if !req.ShowHidden && strings.HasPrefix(name, ".") {
				continue
			}

			// Check default ignore directories/files
			if defaultIgnores[name] {
				continue
			}

			// Check custom ignore patterns
			if isIgnored(name, gitignores) {
				continue
			}

			entryPath := filepath.Join(node.Path, name)
			if entry.IsDir() {
				totalDirs++
				entryCount++
				child := &Node{
					Name:  name,
					Path:  entryPath,
					IsDir: true,
				}
				node.Children = append(node.Children, child)
				buildTree(child, currentDepth+1)
			} else {
				// File extension filtering
				if len(req.Extensions) > 0 {
					ext := strings.ToLower(filepath.Ext(name))
					matched := false
					for _, targetExt := range req.Extensions {
						if ext == targetExt {
							matched = true
							break
						}
					}
					if !matched {
						continue
					}
				}

				entryCount++
				totalFiles++
				var size int64 = 0
				if fileInfo, err := entry.Info(); err == nil {
					size = fileInfo.Size()
					totalSize += size
				}

				child := &Node{
					Name:  name,
					Path:  entryPath,
					IsDir: false,
					Size:  size,
				}
				node.Children = append(node.Children, child)
			}

			if entryCount >= req.MaxEntries {
				truncated = true
				break
			}
		}
	}

	buildTree(rootNode, 1)

	switch req.Format {
	case "json":
		data, err := json.MarshalIndent(map[string]interface{}{
			"root":         rootNode,
			"total_dirs":   totalDirs,
			"total_files":  totalFiles,
			"total_size":   formatBytes(totalSize),
			"is_truncated": truncated,
		}, "", "  ")
		if err != nil {
			return fmt.Sprintf("JSON serialization error: %v", err), true
		}
		return string(data), false

	case "flat":
		var lines []string
		var collectFlat func(n *Node, rel string)
		collectFlat = func(n *Node, rel string) {
			currentRel := filepath.Join(rel, n.Name)
			if rel == "" {
				currentRel = n.Name
			}
			if n != rootNode {
				if n.IsDir {
					lines = append(lines, currentRel+"/")
				} else {
					if req.ShowSizes {
						lines = append(lines, fmt.Sprintf("%-50s (%s)", currentRel, formatBytes(n.Size)))
					} else {
						lines = append(lines, currentRel)
					}
				}
			}
			for _, child := range n.Children {
				collectFlat(child, currentRel)
			}
		}
		collectFlat(rootNode, "")
		footer := fmt.Sprintf("\nSummary: %d directories, %d files (%s)", totalDirs, totalFiles, formatBytes(totalSize))
		if truncated {
			footer += " [TRUNCATED at entry limit]"
		}
		return strings.Join(lines, "\n") + footer, false

	default: // "tree"
		var sb strings.Builder
		sb.WriteString(rootNode.Name + "/\n")

		var renderTree func(n *Node, prefix string)
		renderTree = func(n *Node, prefix string) {
			count := len(n.Children)
			for i, child := range n.Children {
				isLast := i == count-1
				branch := "├── "
				nextPrefix := prefix + "│   "
				if isLast {
					branch = "└── "
					nextPrefix = prefix + "    "
				}

				name := child.Name
				if child.IsDir {
					name += "/"
				} else if req.ShowSizes {
					name += fmt.Sprintf(" (%s)", formatBytes(child.Size))
				}

				sb.WriteString(prefix + branch + name + "\n")
				if child.IsDir && len(child.Children) > 0 {
					renderTree(child, nextPrefix)
				}
			}
		}

		renderTree(rootNode, "")
		sb.WriteString(fmt.Sprintf("\n%d directories, %d files", totalDirs, totalFiles))
		if totalSize > 0 {
			sb.WriteString(fmt.Sprintf(" (%s)", formatBytes(totalSize)))
		}
		if truncated {
			sb.WriteString(fmt.Sprintf(" [Truncated: reached %d entries limit]", req.MaxEntries))
		}
		return sb.String(), false
	}
}

func loadGitignorePatterns(root string) []string {
	var patterns []string
	gitignorePath := filepath.Join(root, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return patterns
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func isIgnored(name string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSuffix(p, "/")
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		if p == name {
			return true
		}
	}
	return false
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

