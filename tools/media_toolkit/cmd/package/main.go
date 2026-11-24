// SPDX-License-Identifier: Apache-2.0

// Command package builds the release archives of the media toolkit.
//
// Run it from tools/media_toolkit:
//
//	go run ./cmd/package -version 1.0.0
//
// For every platform it compiles the toolkit binary once, then writes one
// archive per tool, <tool>-<version>-<os>-<arch>.zip, containing
// <tool>/<tool>[.exe], <tool>/<tool>.json, README.md and LICENSE, plus a
// SHA256SUMS.txt that installers verify downloads against.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var platforms = []struct{ OS, Arch string }{
	{"windows", "amd64"}, {"windows", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"},
}

type entry struct {
	name string
	data []byte
	mode fs.FileMode
}

func main() {
	if err := build(); err != nil {
		fmt.Fprintln(os.Stderr, "package:", err)
		os.Exit(1)
	}
}

func build() error {
	version := flag.String("version", "1.0.0", "release version")
	out := flag.String("out", "dist", "output directory")
	only := flag.String("platform", "", "build a single platform, e.g. windows-amd64 (default: all)")
	flag.Parse()
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$`).MatchString(*version) {
		return fmt.Errorf("invalid version %q", *version)
	}
	dest := filepath.Join(*out, *version)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return fmt.Errorf("refusing to reuse %s: %w", dest, err)
	}
	license, err := os.ReadFile(filepath.Join("..", "..", "LICENSE"))
	if err != nil {
		return fmt.Errorf("LICENSE: %w", err)
	}
	dirs, err := os.ReadDir("agent_tools")
	if err != nil {
		return err
	}
	var tools []string
	for _, d := range dirs {
		if d.IsDir() {
			tools = append(tools, d.Name())
		}
	}
	sort.Strings(tools)
	if len(tools) == 0 {
		return fmt.Errorf("no tools found in agent_tools")
	}

	var sums []string
	for _, p := range platforms {
		if *only != "" && *only != p.OS+"-"+p.Arch {
			continue
		}
		bin := filepath.Join(dest, "ztmedia-"+p.OS+"-"+p.Arch)
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -X main.version="+*version, "-o", bin, ".")
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "GOOS=") && !strings.HasPrefix(v, "GOARCH=") && !strings.HasPrefix(v, "CGO_ENABLED=") {
				cmd.Env = append(cmd.Env, v)
			}
		}
		cmd.Env = append(cmd.Env, "GOOS="+p.OS, "GOARCH="+p.Arch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s/%s: %w", p.OS, p.Arch, err)
		}
		binary, err := os.ReadFile(bin)
		if err != nil {
			return err
		}
		for _, tool := range tools {
			exe := tool
			if p.OS == "windows" {
				exe += ".exe"
			}
			raw, err := os.ReadFile(filepath.Join("agent_tools", tool, tool+".json"))
			if err != nil {
				return err
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				return fmt.Errorf("%s manifest: %w", tool, err)
			}
			m["command"] = []string{"{dir}/" + exe, "--agent-json"}
			m["platforms"] = []string{p.OS}
			manifest, _ := json.MarshalIndent(m, "", "  ")
			readme, err := os.ReadFile(filepath.Join("agent_tools", tool, "README.md"))
			if err != nil {
				return err
			}
			name := fmt.Sprintf("%s-%s-%s-%s.zip", tool, *version, p.OS, p.Arch)
			if err := writeZip(filepath.Join(dest, name), tool, []entry{
				{exe, binary, 0o755},
				{tool + ".json", append(manifest, '\n'), 0o644},
				{"README.md", readme, 0o644},
				{"LICENSE", license, 0o644},
			}); err != nil {
				return err
			}
			archive, err := os.ReadFile(filepath.Join(dest, name))
			if err != nil {
				return err
			}
			sums = append(sums, fmt.Sprintf("%x  %s", sha256.Sum256(archive), name))
		}
		if err := os.Remove(bin); err != nil {
			return err
		}
		fmt.Printf("built %d archives for %s/%s\n", len(tools), p.OS, p.Arch)
	}
	sort.Strings(sums)
	return os.WriteFile(filepath.Join(dest, "SHA256SUMS.txt"), []byte(strings.Join(sums, "\n")+"\n"), 0o644)
}

func writeZip(path, dir string, entries []entry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	for _, e := range entries {
		h := &zip.FileHeader{Name: dir + "/" + e.name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		h.SetMode(e.mode)
		w, err := z.CreateHeader(h)
		if err == nil {
			_, err = w.Write(e.data)
		}
		if err != nil {
			z.Close()
			f.Close()
			return err
		}
	}
	if err := z.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
