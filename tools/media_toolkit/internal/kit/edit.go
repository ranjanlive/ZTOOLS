// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// ----------------------------------------------------------------- media_trim

type trimReq struct {
	Input     string     `json:"input"`
	Start     proto.Time `json:"start"`
	End       proto.Time `json:"end"`
	Duration  proto.Time `json:"duration"`
	Output    string     `json:"output"`
	Mode      string     `json:"mode"`
	Quality   string     `json:"quality"`
	Overwrite bool       `json:"overwrite"`
}

func init() {
	register("media_trim", "Cut a clip out of a video or audio file between two timestamps.", runTrim)
}

func runTrim(ctx context.Context, c *proto.Ctx, r *trimReq) proto.Result {
	mode := orDefault(r.Mode, "accurate")
	if err := oneOf("mode", mode, "accurate", "fast"); err != nil {
		return proto.Fail("%v", err)
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !r.Start.Set && !r.End.Set && !r.Duration.Set {
		return proto.Fail("give at least one of start, end or duration")
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	s, e, err := span(r.Start, r.End, r.Duration, m.duration())
	if err != nil {
		return proto.Fail("%v", err)
	}
	ext := sameExt(r.Output, m.Path)
	out, err := c.Output(r.Output, m.Path, "trim", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if err := cut(ctx, c, m, s, e, out, mode, crf, "media_trim"); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	note := ""
	if mode == "fast" && m.hasVideo() {
		note = "\nFast mode copies the streams without re-encoding, so the cut snaps to the nearest keyframe and can be off by up to a few seconds. Use mode \"accurate\" for frame-exact cuts."
	}
	return proto.OK("Cut %s – %s (%.2f s) from %s:\n%s%s", proto.Clock(s), proto.Clock(e), e-s, c.Rel(m.Path), describe(ctx, c, out), note)
}

// cut writes [s, e) of m to out.
func cut(ctx context.Context, c *proto.Ctx, m *media, s, e float64, out, mode string, crf int, label string) error {
	ext := extOf(out, "mp4")
	args := []string{"-ss", ff.Sec(s), "-i", m.Path, "-t", ff.Sec(e - s)}
	if mode == "fast" {
		args = append(args, "-map", "0:v:0?", "-map", "0:a?", "-c", "copy", "-avoid_negative_ts", "make_zero")
		if ff.AudioOnlyExt(ext) {
			args = []string{"-ss", ff.Sec(s), "-i", m.Path, "-t", ff.Sec(e - s), "-vn", "-map", "0:a?", "-c", "copy"}
		}
	} else {
		video := m.hasVideo() && !ff.AudioOnlyExt(ext)
		enc, err := ff.Encoders(ext, video, m.hasAudio(), crf, "")
		if err != nil {
			return err
		}
		if video {
			args = append(args, "-map", "0:v:0")
		}
		if m.hasAudio() {
			args = append(args, "-map", "0:a?")
		}
		args = append(args, enc...)
	}
	args = append(args, out)
	_, err := ff.Run(ctx, ff.Job{Args: args, Duration: e - s, Label: label, Ctx: c})
	return err
}

// ---------------------------------------------------------------- media_split

type splitReq struct {
	Input         string       `json:"input"`
	Mode          string       `json:"mode"`
	SegmentLength proto.Time   `json:"segment_length"`
	Parts         int          `json:"parts"`
	At            []proto.Time `json:"at"`
	OutputDir     string       `json:"output_dir"`
	Cut           string       `json:"cut"`
	Quality       string       `json:"quality"`
}

func init() {
	register("media_split", "Split a video or audio file into parts: every N seconds, into N equal parts, or at given timestamps.", runSplit)
}

func runSplit(ctx context.Context, c *proto.Ctx, r *splitReq) proto.Result {
	mode := orDefault(r.Mode, "segment_length")
	if err := oneOf("mode", mode, "segment_length", "parts", "at"); err != nil {
		return proto.Fail("%v", err)
	}
	cutMode := orDefault(r.Cut, "accurate")
	if err := oneOf("cut", cutMode, "accurate", "fast"); err != nil {
		return proto.Fail("%v", err)
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	d := m.duration()
	if d <= 0 {
		return proto.Fail("cannot split %s: its duration is unknown", r.Input)
	}
	var points []float64
	switch mode {
	case "segment_length":
		if !r.SegmentLength.Set || r.SegmentLength.Seconds < 0.5 {
			return proto.Fail("segment_length is required for mode \"segment_length\" and must be at least 0.5 seconds")
		}
		for t := r.SegmentLength.Seconds; t < d-0.25; t += r.SegmentLength.Seconds {
			points = append(points, t)
		}
	case "parts":
		if r.Parts < 2 || r.Parts > 500 {
			return proto.Fail("parts must be between 2 and 500")
		}
		for i := 1; i < r.Parts; i++ {
			points = append(points, d*float64(i)/float64(r.Parts))
		}
	case "at":
		if len(r.At) == 0 {
			return proto.Fail("at is required for mode \"at\" (the timestamps to split at)")
		}
		prev := 0.0
		for _, t := range r.At {
			if t.Seconds <= prev || t.Seconds >= d {
				return proto.Fail("split points must be increasing and inside the media (0 – %s); %s is not", proto.Clock(d), proto.Clock(t.Seconds))
			}
			points = append(points, t.Seconds)
			prev = t.Seconds
		}
	}
	if len(points)+1 > 500 {
		return proto.Fail("that would create %d parts; the limit is 500", len(points)+1)
	}
	base := strings.TrimSuffix(filepath.Base(m.Path), filepath.Ext(m.Path))
	dir, err := c.Dir("output_dir", r.OutputDir, filepath.Join(c.Rel(filepath.Dir(m.Path)), base+"_parts"))
	if err != nil {
		return proto.Fail("%v", err)
	}
	ext := extOf(m.Path, "mp4")
	bounds := append(append([]float64{0}, points...), d)
	var b strings.Builder
	fmt.Fprintf(&b, "Split %s (%s) into %d parts in %s:\n", c.Rel(m.Path), proto.Clock(d), len(bounds)-1, c.Rel(dir))
	for i := 0; i+1 < len(bounds); i++ {
		out := filepath.Join(dir, fmt.Sprintf("%s_part%03d.%s", base, i+1, ext))
		if _, err := os.Stat(out); err == nil {
			return proto.Fail("%s already exists; choose an empty output_dir", c.Rel(out))
		}
		if err := cut(ctx, c, m, bounds[i], bounds[i+1], out, cutMode, crf, ""); err != nil {
			return proto.Fail("part %d: %v", i+1, err)
		}
		c.Notify(false, "media_split: %d of %d parts done", i+1, len(bounds)-1)
		fmt.Fprintf(&b, "  %s  %s – %s\n", filepath.Base(out), proto.Clock(bounds[i]), proto.Clock(bounds[i+1]))
	}
	return proto.OK("%s", strings.TrimRight(b.String(), "\n"))
}

// ---------------------------------------------------------------- media_merge

type mergeReq struct {
	Inputs    []string `json:"inputs"`
	Output    string   `json:"output"`
	Mode      string   `json:"mode"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
	FPS       float64  `json:"fps"`
	Quality   string   `json:"quality"`
	Overwrite bool     `json:"overwrite"`
}

func init() {
	register("media_merge", "Join several videos or audio files end to end into one file.", runMerge)
}

func runMerge(ctx context.Context, c *proto.Ctx, r *mergeReq) proto.Result {
	mode := orDefault(r.Mode, "auto")
	if err := oneOf("mode", mode, "auto", "fast", "reencode"); err != nil {
		return proto.Fail("%v", err)
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if len(r.Inputs) < 2 {
		return proto.Fail("inputs needs at least two files")
	}
	if len(r.Inputs) > 200 {
		return proto.Fail("at most 200 inputs")
	}
	var ms []*media
	total := 0.0
	for i, in := range r.Inputs {
		m, err := openMedia(ctx, c, fmt.Sprintf("inputs[%d]", i), in)
		if err != nil {
			return proto.Fail("%v", err)
		}
		ms = append(ms, m)
		total += m.duration()
	}
	ext := sameExt(r.Output, ms[0].Path)
	out, err := c.Output(r.Output, ms[0].Path, "merged", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if mode == "auto" {
		mode = "reencode"
		if compatible(ms) && extOf(ms[0].Path, "") == ext {
			mode = "fast"
		}
	}
	if mode == "fast" {
		err = mergeFast(ctx, c, ms, out, total)
	} else {
		err = mergeReencode(ctx, c, ms, out, ext, r, crf, total)
	}
	if err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	how := "re-encoded to a common format"
	if mode == "fast" {
		how = "joined without re-encoding"
	}
	return proto.OK("Merged %d files (%s):\n%s", len(ms), how, describe(ctx, c, out))
}

// compatible reports whether files can be concatenated without re-encoding.
func compatible(ms []*media) bool {
	type sig struct {
		vcodec, pix string
		w, h        int
		fps         float64
		acodec, sr  string
		ch          int
	}
	get := func(m *media) sig {
		var s sig
		if v := m.Info.Video(); v != nil {
			s.vcodec, s.pix, s.w, s.h, s.fps = v.CodecName, v.PixFmt, v.Width, v.Height, math.Round(v.FPS()*100)/100
		}
		if a := m.Info.Audio(); a != nil {
			s.acodec, s.sr, s.ch = a.CodecName, a.SampleRate, a.Channels
		}
		return s
	}
	first := get(ms[0])
	for _, m := range ms[1:] {
		if get(m) != first || extOf(m.Path, "") != extOf(ms[0].Path, "") {
			return false
		}
	}
	return true
}

func mergeFast(ctx context.Context, c *proto.Ctx, ms []*media, out string, total float64) error {
	work, cleanup, err := ff.Workdir(c, nil)
	if err != nil {
		return err
	}
	defer cleanup()
	var list strings.Builder
	for _, m := range ms {
		p := filepath.ToSlash(m.Path)
		fmt.Fprintf(&list, "file '%s'\n", strings.ReplaceAll(p, "'", `'\''`))
	}
	if err := os.WriteFile(filepath.Join(work, "list.txt"), []byte(list.String()), 0o644); err != nil {
		return err
	}
	_, err = ff.Run(ctx, ff.Job{Args: []string{"-f", "concat", "-safe", "0", "-i", "list.txt", "-map", "0:v?", "-map", "0:a?", "-c", "copy", out},
		Dir: work, Duration: total, Label: "media_merge", Ctx: c})
	return err
}

func mergeReencode(ctx context.Context, c *proto.Ctx, ms []*media, out, ext string, r *mergeReq, crf int, total float64) error {
	video := !ff.AudioOnlyExt(ext)
	anyVideo, anyAudio := false, false
	for _, m := range ms {
		anyVideo = anyVideo || m.hasVideo()
		anyAudio = anyAudio || m.hasAudio()
	}
	video = video && anyVideo
	w, h, fps := r.Width, r.Height, r.FPS
	if video {
		for _, m := range ms {
			if v := m.Info.Video(); v != nil {
				vw, vh := v.DisplaySize()
				if w == 0 && h == 0 {
					w, h = vw, vh
				}
				if fps == 0 {
					fps = v.FPS()
				}
				break
			}
		}
		if w == 0 || h == 0 {
			return fmt.Errorf("give both width and height, or neither")
		}
		if fps <= 0 || fps > 120 {
			fps = 30
		}
		w, h = ff.Even(w), ff.Even(h)
	}

	var args []string
	var g strings.Builder
	var concatIn strings.Builder
	extra := len(ms)
	for _, m := range ms {
		args = append(args, "-i", m.Path)
	}
	for i, m := range ms {
		d := m.duration()
		if video {
			if m.hasVideo() {
				fmt.Fprintf(&g, "[%d:v:0]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1,fps=%.3f,format=yuv420p[v%d];\n", i, w, h, w, h, fps, i)
			} else {
				args = append(args, "-f", "lavfi", "-t", ff.Sec(d), "-i", fmt.Sprintf("color=c=black:s=%dx%d:r=%.3f", w, h, fps))
				fmt.Fprintf(&g, "[%d:v]format=yuv420p,setsar=1[v%d];\n", extra, i)
				extra++
			}
			fmt.Fprintf(&concatIn, "[v%d]", i)
		}
		if anyAudio {
			if m.hasAudio() {
				fmt.Fprintf(&g, "[%d:a:0]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo[a%d];\n", i, i)
			} else {
				args = append(args, "-f", "lavfi", "-t", ff.Sec(d), "-i", "anullsrc=r=48000:cl=stereo")
				fmt.Fprintf(&g, "[%d:a]aformat=sample_fmts=fltp:channel_layouts=stereo[a%d];\n", extra, i)
				extra++
			}
			fmt.Fprintf(&concatIn, "[a%d]", i)
		}
	}
	v, a := 0, 0
	maps := []string{}
	if video {
		v = 1
		maps = append(maps, "-map", "[vout]")
	}
	if anyAudio {
		a = 1
		maps = append(maps, "-map", "[aout]")
	}
	if v+a == 0 {
		return fmt.Errorf("nothing to merge")
	}
	outs := ""
	if video {
		outs += "[vout]"
	}
	if anyAudio {
		outs += "[aout]"
	}
	fmt.Fprintf(&g, "%sconcat=n=%d:v=%d:a=%d%s", concatIn.String(), len(ms), v, a, outs)

	work, cleanup, err := ff.Workdir(c, nil)
	if err != nil {
		return err
	}
	defer cleanup()
	script := filepath.Join(work, "graph.txt")
	if err := os.WriteFile(script, []byte(g.String()), 0o644); err != nil {
		return err
	}
	enc, err := ff.Encoders(ext, video, anyAudio, crf, "")
	if err != nil {
		return err
	}
	args = append(args, ff.FilterScriptArgs(script)...)
	args = append(args, maps...)
	args = append(args, enc...)
	args = append(args, out)
	_, err = ff.Run(ctx, ff.Job{Args: args, Duration: total, Label: "media_merge", Ctx: c})
	return err
}
