// SPDX-License-Identifier: Apache-2.0

// Package ff locates and drives FFmpeg and FFprobe.
package ff

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// InstallHint explains how to get FFmpeg on this platform.
func InstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "Install it with `winget install Gyan.FFmpeg` (or download from https://www.gyan.dev/ffmpeg/builds/) and make sure ffmpeg.exe is on PATH, or set ZTOOLS_FFMPEG to its full path."
	case "darwin":
		return "Install it with `brew install ffmpeg`, or set ZTOOLS_FFMPEG to its full path."
	}
	return "Install it with your package manager (for example `sudo apt install ffmpeg` or `sudo dnf install ffmpeg`), or set ZTOOLS_FFMPEG to its full path."
}

var (
	findMu sync.Mutex
	found  = map[string]string{}
)

// Find returns the path of "ffmpeg" or "ffprobe".
func Find(name string) (string, error) {
	findMu.Lock()
	defer findMu.Unlock()
	if p, ok := found[name]; ok {
		return p, nil
	}
	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	var candidates []string
	env := "ZTOOLS_" + strings.ToUpper(name)
	if p := os.Getenv(env); p != "" {
		candidates = append(candidates, p)
	}
	// An explicit ffmpeg path implies ffprobe next to it.
	if name == "ffprobe" {
		if p := os.Getenv("ZTOOLS_FFMPEG"); p != "" {
			candidates = append(candidates, filepath.Join(filepath.Dir(p), exe))
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		candidates = append(candidates, p)
	}
	switch runtime.GOOS {
	case "windows":
		for _, d := range []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links"),
			`C:\ffmpeg\bin`, `C:\tools\ffmpeg\bin`, `C:\Program Files\ffmpeg\bin`,
			filepath.Join(os.Getenv("USERPROFILE"), "scoop", "shims"),
			`C:\ProgramData\chocolatey\bin`,
		} {
			candidates = append(candidates, filepath.Join(d, exe))
		}
	case "darwin":
		candidates = append(candidates, "/opt/homebrew/bin/"+name, "/usr/local/bin/"+name)
	default:
		candidates = append(candidates, "/usr/bin/"+name, "/usr/local/bin/"+name, "/snap/bin/"+name)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			found[name] = c
			return c, nil
		}
	}
	return "", fmt.Errorf("%s was not found. This tool needs FFmpeg. %s", name, InstallHint())
}

var verRe = regexp.MustCompile(`version n?(\d+)\.`)

// Major returns FFmpeg's major version, or 0 for git builds without one.
func Major() int {
	bin, err := Find("ffmpeg")
	if err != nil {
		return 0
	}
	out, _ := exec.Command(bin, "-hide_banner", "-version").Output()
	if m := verRe.FindSubmatch(out); m != nil {
		v, _ := strconv.Atoi(string(m[1]))
		return v
	}
	return 99 // git snapshot builds ("version 2026-09-17-git-...") are current
}

var (
	encOnce sync.Once
	encList string
)

// HasEncoder reports whether this FFmpeg build includes an encoder.
func HasEncoder(name string) bool {
	encOnce.Do(func() {
		if bin, err := Find("ffmpeg"); err == nil {
			out, _ := exec.Command(bin, "-hide_banner", "-encoders").Output()
			encList = string(out)
		}
	})
	return regexp.MustCompile(`(?m)^\s*\S+\s+` + regexp.QuoteMeta(name) + `\s`).MatchString(encList)
}

// FilterScriptArgs returns the arguments that load a filter graph from a file.
// FFmpeg 7 replaced -filter_complex_script with the -/filter_complex syntax.
func FilterScriptArgs(path string) []string {
	if Major() >= 7 {
		return []string{"-/filter_complex", path}
	}
	return []string{"-filter_complex_script", path}
}

// Stream is one stream from ffprobe.
type Stream struct {
	Index        int               `json:"index"`
	CodecType    string            `json:"codec_type"`
	CodecName    string            `json:"codec_name"`
	Profile      string            `json:"profile"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	PixFmt       string            `json:"pix_fmt"`
	RFrameRate   string            `json:"r_frame_rate"`
	AvgFrameRate string            `json:"avg_frame_rate"`
	SampleRate   string            `json:"sample_rate"`
	Channels     int               `json:"channels"`
	ChannelLay   string            `json:"channel_layout"`
	BitRate      string            `json:"bit_rate"`
	Duration     string            `json:"duration"`
	Tags         map[string]string `json:"tags"`
	Disposition  map[string]int    `json:"disposition"`
	SideData     []map[string]any  `json:"side_data_list"`
}

// Info is ffprobe's view of a file.
type Info struct {
	Streams []Stream `json:"streams"`
	Format  struct {
		Filename   string            `json:"filename"`
		FormatName string            `json:"format_name"`
		LongName   string            `json:"format_long_name"`
		Duration   string            `json:"duration"`
		Size       string            `json:"size"`
		BitRate    string            `json:"bit_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Chapters []struct {
		StartTime string            `json:"start_time"`
		EndTime   string            `json:"end_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
}

// Probe runs ffprobe on a file.
func Probe(ctx context.Context, path string) (*Info, error) {
	bin, err := Find("ffprobe")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", "-show_chapters", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("cannot read %s as media: %s", filepath.Base(path), lastLines(msg, 5))
	}
	var info Info
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("ffprobe output: %v", err)
	}
	return &info, nil
}

// Duration is the container duration in seconds (0 if unknown).
func (i *Info) Duration() float64 {
	d, _ := strconv.ParseFloat(i.Format.Duration, 64)
	if d > 0 {
		return d
	}
	for _, s := range i.Streams {
		if v, _ := strconv.ParseFloat(s.Duration, 64); v > d {
			d = v
		}
	}
	return d
}

// Video returns the first real video stream (not cover art), or nil.
func (i *Info) Video() *Stream {
	for k := range i.Streams {
		s := &i.Streams[k]
		if s.CodecType == "video" && s.Disposition["attached_pic"] == 0 {
			return s
		}
	}
	return nil
}

// Audio returns the first audio stream, or nil.
func (i *Info) Audio() *Stream {
	for k := range i.Streams {
		if i.Streams[k].CodecType == "audio" {
			return &i.Streams[k]
		}
	}
	return nil
}

// FPS parses a stream's frame rate.
func (s *Stream) FPS() float64 {
	for _, r := range []string{s.AvgFrameRate, s.RFrameRate} {
		n, d, ok := strings.Cut(r, "/")
		if !ok {
			continue
		}
		a, _ := strconv.ParseFloat(n, 64)
		b, _ := strconv.ParseFloat(d, 64)
		if a > 0 && b > 0 {
			return a / b
		}
	}
	return 0
}

// Rotation returns the display rotation in degrees from side data or tags.
func (s *Stream) Rotation() int {
	for _, sd := range s.SideData {
		if r, ok := sd["rotation"].(float64); ok {
			return int(r)
		}
	}
	if r, err := strconv.Atoi(s.Tags["rotate"]); err == nil {
		return r
	}
	return 0
}

// DisplaySize is the frame size after applying rotation metadata.
func (s *Stream) DisplaySize() (int, int) {
	r := s.Rotation()
	if r%180 != 0 {
		return s.Height, s.Width
	}
	return s.Width, s.Height
}

// Job is one FFmpeg invocation.
type Job struct {
	Args     []string // arguments after the global options
	Duration float64  // expected output duration, for progress
	Label    string   // what is being done, for progress lines
	Dir      string   // working directory (optional)
	Ctx      *proto.Ctx
}

// Run executes FFmpeg and reports progress through the notify file.
// It returns FFmpeg's stderr (log level "info" lines are not included).
func Run(ctx context.Context, j Job) (string, error) {
	bin, err := Find("ffmpeg")
	if err != nil {
		return "", err
	}
	args := append([]string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}, j.Args...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = j.Dir
	var stderr limitedBuffer
	stderr.max = 256 << 10
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	sc := bufio.NewScanner(stdout)
	lastPct := -1
	for sc.Scan() {
		line := sc.Text()
		if j.Ctx == nil || j.Duration <= 0 {
			continue
		}
		if v, ok := strings.CutPrefix(line, "out_time_us="); ok {
			us, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			pct := int(us / 1e6 / j.Duration * 100)
			if pct > 99 {
				pct = 99
			}
			if pct/10 != lastPct/10 && pct > 0 {
				lastPct = pct
				j.Ctx.Notify(false, "%s: %d%%", j.Label, pct)
			}
		}
	}
	_, _ = io.Copy(io.Discard, stdout)
	err = cmd.Wait()
	log := strings.TrimSpace(stderr.String())
	if ctx.Err() != nil {
		return log, ctx.Err()
	}
	if err != nil {
		if log == "" {
			log = err.Error()
		}
		return log, fmt.Errorf("ffmpeg failed: %s", lastLines(log, 8))
	}
	return log, nil
}

// RunLog runs FFmpeg with a chosen log level and returns all stderr, for
// analysis filters (silencedetect, showinfo, loudnorm) that print results.
func RunLog(ctx context.Context, level string, args ...string) (string, error) {
	bin, err := Find("ffmpeg")
	if err != nil {
		return "", err
	}
	all := append([]string{"-hide_banner", "-nostdin", "-nostats", "-loglevel", level}, args...)
	cmd := exec.CommandContext(ctx, bin, all...)
	var stderr limitedBuffer
	stderr.max = 64 << 20
	cmd.Stderr = &stderr
	err = cmd.Run()
	log := stderr.String()
	if err != nil {
		if ctx.Err() != nil {
			return log, ctx.Err()
		}
		return log, fmt.Errorf("ffmpeg failed: %s", lastLines(strings.TrimSpace(log), 8))
	}
	return log, nil
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		// keep the tail, which holds the error
		keep := b.Bytes()
		if len(keep) > b.max/2 {
			keep = append([]byte(nil), keep[len(keep)-b.max/2:]...)
		}
		b.Reset()
		b.Write(keep)
	}
	return b.Buffer.Write(p)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Sec formats seconds for FFmpeg arguments.
func Sec(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}

// Quality maps a quality name to an x264-style CRF.
func Quality(q string) (int, error) {
	switch strings.ToLower(q) {
	case "", "high":
		return 18, nil
	case "medium":
		return 23, nil
	case "low":
		return 28, nil
	}
	return 0, fmt.Errorf("quality must be high, medium or low, got %q", q)
}

// Encoders returns codec arguments suited to an output extension.
// crf is an x264-scale value (18 high .. 28 low).
func Encoders(ext string, video, audio bool, crf int, audioBitrate string) ([]string, error) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if audioBitrate == "" {
		audioBitrate = "192k"
	}
	var a []string
	switch ext {
	case "mp4", "m4v", "mov", "mkv", "avi", "ts", "flv":
		if video {
			a = append(a, "-c:v", "libx264", "-preset", "medium", "-crf", strconv.Itoa(crf), "-pix_fmt", "yuv420p")
		}
		if audio {
			if ext == "avi" {
				a = append(a, "-c:a", "libmp3lame", "-b:a", audioBitrate)
			} else {
				a = append(a, "-c:a", "aac", "-b:a", audioBitrate)
			}
		}
		if ext == "mp4" || ext == "m4v" || ext == "mov" {
			a = append(a, "-movflags", "+faststart")
		}
	case "webm":
		if video {
			a = append(a, "-c:v", "libvpx-vp9", "-crf", strconv.Itoa(crf+13), "-b:v", "0", "-row-mt", "1", "-deadline", "good", "-cpu-used", "4")
		}
		if audio {
			a = append(a, "-c:a", "libopus", "-b:a", audioBitrate)
		}
	case "mp3":
		a = append(a, "-vn", "-c:a", "libmp3lame", "-b:a", audioBitrate)
	case "m4a", "aac":
		a = append(a, "-vn", "-c:a", "aac", "-b:a", audioBitrate)
		if ext == "m4a" {
			a = append(a, "-movflags", "+faststart")
		}
	case "wav":
		a = append(a, "-vn", "-c:a", "pcm_s16le")
	case "flac":
		a = append(a, "-vn", "-c:a", "flac")
	case "ogg", "opus":
		a = append(a, "-vn", "-c:a", "libopus", "-b:a", audioBitrate)
	default:
		return nil, fmt.Errorf("unsupported output format .%s (use mp4, mov, mkv, webm, avi, mp3, m4a, wav, flac, ogg or opus)", ext)
	}
	if (ext == "mp3" || ext == "m4a" || ext == "aac" || ext == "wav" || ext == "flac" || ext == "ogg" || ext == "opus") && !audio {
		return nil, errors.New("the input has no audio stream, so it cannot be saved as an audio file")
	}
	return a, nil
}

// AudioOnlyExt reports whether an extension is an audio-only format.
func AudioOnlyExt(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "mp3", "m4a", "aac", "wav", "flac", "ogg", "opus":
		return true
	}
	return false
}

// Even rounds down to an even number (codecs need even dimensions).
func Even(v int) int {
	if v < 2 {
		return 2
	}
	return v &^ 1
}

// FindFont returns a TrueType font for drawing text, or "".
func FindFont() string {
	if p := os.Getenv("ZTOOLS_FONT"); p != "" {
		return p
	}
	var c []string
	switch runtime.GOOS {
	case "windows":
		w := os.Getenv("WINDIR")
		if w == "" {
			w = `C:\Windows`
		}
		for _, f := range []string{"arialbd.ttf", "arial.ttf", "segoeui.ttf", "calibri.ttf", "verdana.ttf"} {
			c = append(c, filepath.Join(w, "Fonts", f))
		}
	case "darwin":
		c = []string{"/System/Library/Fonts/Supplemental/Arial Bold.ttf", "/System/Library/Fonts/Supplemental/Arial.ttf", "/Library/Fonts/Arial.ttf", "/System/Library/Fonts/Helvetica.ttc"}
	default:
		c = []string{"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", "/usr/share/fonts/TTF/DejaVuSans.ttf",
			"/usr/share/fonts/dejavu/DejaVuSans.ttf", "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf", "/usr/share/fonts/noto/NotoSans-Regular.ttf"}
	}
	for _, p := range c {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Workdir creates a private scratch directory for filter files, and copies
// the given files into it under simple names. FFmpeg filter arguments need
// awkward escaping for drive letters, colons and quotes; running FFmpeg in
// this directory with plain relative names avoids that entirely.
func Workdir(c *proto.Ctx, files map[string]string) (string, func(), error) {
	dir, err := os.MkdirTemp(c.OutputDir, "work-")
	if err != nil {
		dir, err = os.MkdirTemp("", "ztools-work-")
		if err != nil {
			return "", nil, err
		}
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	for name, src := range files {
		b, err := os.ReadFile(src)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return dir, cleanup, nil
}
