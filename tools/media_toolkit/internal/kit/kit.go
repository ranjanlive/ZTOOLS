// SPDX-License-Identifier: Apache-2.0

// Package kit contains the media tools. Each tool registers a request type
// (whose JSON fields must match its manifest's parameters exactly, which the
// tests verify) and a run function.
package kit

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// Tool is one registered tool.
type Tool struct {
	Name    string
	Summary string
	Request reflect.Type
	run     func(context.Context, *proto.Ctx, []byte) proto.Result
}

var registry = map[string]*Tool{}

func register[T any](name, summary string, run func(context.Context, *proto.Ctx, *T) proto.Result) {
	if _, dup := registry[name]; dup {
		panic("duplicate tool " + name)
	}
	registry[name] = &Tool{
		Name: name, Summary: summary, Request: reflect.TypeOf((*T)(nil)).Elem(),
		run: func(ctx context.Context, c *proto.Ctx, raw []byte) proto.Result {
			req := new(T)
			if err := proto.Decode(raw, req); err != nil {
				return proto.Fail("%v", err)
			}
			return run(ctx, c, req)
		},
	}
}

// Get returns a tool by name.
func Get(name string) (*Tool, bool) {
	t, ok := registry[name]
	return t, ok
}

// All returns every tool sorted by name.
func All() []*Tool {
	out := make([]*Tool, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Run executes the tool with raw JSON arguments.
func (t *Tool) Run(ctx context.Context, c *proto.Ctx, raw []byte) (res proto.Result) {
	defer func() {
		if r := recover(); r != nil {
			res = proto.Fail("%s crashed: %v", t.Name, r)
		}
		if res.Media == nil {
			res.Media = []proto.Media{}
		}
	}()
	return t.run(ctx, c, raw)
}

// media is a probed input file.
type media struct {
	Path string
	Info *ff.Info
}

func (m *media) duration() float64 { return m.Info.Duration() }
func (m *media) hasVideo() bool    { return m.Info.Video() != nil }
func (m *media) hasAudio() bool    { return m.Info.Audio() != nil }

// openMedia resolves and probes an input.
func openMedia(ctx context.Context, c *proto.Ctx, field, p string) (*media, error) {
	path, err := c.Input(field, p)
	if err != nil {
		return nil, err
	}
	info, err := ff.Probe(ctx, path)
	if err != nil {
		return nil, err
	}
	if info.Video() == nil && info.Audio() == nil {
		return nil, fmt.Errorf("%s has no audio or video stream", p)
	}
	return &media{Path: path, Info: info}, nil
}

// extOf returns a lowercase extension without the dot, defaulting to def.
func extOf(p, def string) string {
	e := strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
	if e == "" {
		return def
	}
	return e
}

// sameExt picks an output extension: the requested file's, else the input's.
func sameExt(requested, input string) string {
	if requested != "" && filepath.Ext(requested) != "" {
		return extOf(requested, "mp4")
	}
	return extOf(input, "mp4")
}

// describe summarises an output file for the agent.
func describe(ctx context.Context, c *proto.Ctx, p string) string {
	info, err := ff.Probe(ctx, p)
	size := proto.Size(proto.FileSize(p))
	if err != nil {
		return fmt.Sprintf("%s (%s)", c.Rel(p), size)
	}
	parts := []string{proto.Clock(info.Duration())}
	if v := info.Video(); v != nil {
		w, h := v.DisplaySize()
		parts = append(parts, fmt.Sprintf("%dx%d %s", w, h, v.CodecName))
	}
	if a := info.Audio(); a != nil {
		parts = append(parts, a.CodecName+" audio")
	}
	parts = append(parts, size)
	return fmt.Sprintf("%s (%s)", c.Rel(p), strings.Join(parts, ", "))
}

// span resolves start/end/duration against a media length.
func span(start, end, dur proto.Time, length float64) (float64, float64, error) {
	s := 0.0
	if start.Set {
		s = start.Seconds
	}
	e := length
	switch {
	case end.Set && dur.Set:
		return 0, 0, fmt.Errorf("give either end or duration, not both")
	case end.Set:
		e = end.Seconds
	case dur.Set:
		e = s + dur.Seconds
	}
	if length > 0 {
		if s >= length {
			return 0, 0, fmt.Errorf("start %s is past the end of the media (%s long)", proto.Clock(s), proto.Clock(length))
		}
		if e > length {
			e = length
		}
	}
	if e <= s {
		return 0, 0, fmt.Errorf("the end (%s) must be after the start (%s)", proto.Clock(e), proto.Clock(s))
	}
	return s, e, nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func oneOf(field, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %s, got %q", field, strings.Join(allowed, ", "), v)
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
