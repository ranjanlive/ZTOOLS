// SPDX-License-Identifier: Apache-2.0

// Command ztmedia is the ZTools media toolkit: video, audio and download
// tools for ZAgent, ZEngine and ZPilot agents.
//
// One binary serves every tool. Release archives ship it under each tool's
// name (media_trim, video_frames, ...), and it picks the tool from its own
// file name. It can also be called as `ztmedia <tool>`.
//
//	echo '{"input":"clip.mp4","start":"0:05","end":"0:12"}' | media_trim --agent-json
//	ztmedia media_trim --args '{"input":"clip.mp4","start":5,"end":12}'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/kit"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	self := strings.TrimSuffix(filepath.Base(argv[0]), ".exe")
	args := argv[1:]
	tool, ok := kit.Get(self)
	if !ok {
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return usage(args, stdout, stderr)
		}
		if tool, ok = kit.Get(args[0]); !ok {
			fmt.Fprintf(stderr, "unknown tool %q; run `ztmedia --list` to see all tools\n", args[0])
			return 2
		}
		args = args[1:]
	}

	var raw []byte
	mode := ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--agent-json" || a == "--json":
			mode = "stdin"
		case a == "--args" && i+1 < len(args):
			raw, mode = []byte(args[i+1]), "args"
			i++
		case strings.HasPrefix(a, "--args="):
			raw, mode = []byte(strings.TrimPrefix(a, "--args=")), "args"
		case a == "--version":
			fmt.Fprintf(stdout, "%s (ZTools media toolkit) %s\n", tool.Name, version)
			return 0
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "%s: %s\n\nUsage:\n  echo '{...}' | %s --agent-json\n  %s --args '{...}'\n\nSee README.md for the parameters.\n", tool.Name, tool.Summary, tool.Name, tool.Name)
			return 0
		default:
			fmt.Fprintf(stderr, "unknown argument %q (use --agent-json, --args JSON, --help or --version)\n", a)
			return 2
		}
	}
	if mode == "" {
		mode = "stdin"
	}
	if mode == "stdin" {
		b, err := io.ReadAll(io.LimitReader(stdin, 16<<20))
		if err != nil {
			return emit(stdout, proto.Fail("cannot read arguments from stdin: %v", err))
		}
		raw = b
	}

	c, err := proto.FromEnv()
	if err != nil {
		return emit(stdout, proto.Fail("%v", err))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return emit(stdout, tool.Run(ctx, c, raw))
}

func emit(w io.Writer, r proto.Result) int {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return 1
	}
	return 0
}

func usage(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "--version" {
		fmt.Fprintf(stdout, "ztmedia (ZTools media toolkit) %s\n", version)
		return 0
	}
	w := stdout
	code := 0
	if len(args) > 0 && args[0] != "--list" && args[0] != "-h" && args[0] != "--help" {
		w, code = stderr, 2
	}
	fmt.Fprintf(w, "ZTools media toolkit %s\n\nUsage: ztmedia <tool> [--agent-json | --args JSON]\n\nTools:\n", version)
	for _, t := range kit.All() {
		fmt.Fprintf(w, "  %-18s %s\n", t.Name, t.Summary)
	}
	return code
}
