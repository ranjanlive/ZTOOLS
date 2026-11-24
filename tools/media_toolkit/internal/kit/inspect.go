// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// ---------------------------------------------------------------- media_probe

type probeReq struct {
	Input  string `json:"input"`
	Format string `json:"format"`
}

func init() {
	register("media_probe", "Inspect a video or audio file: duration, streams, codecs, resolution, bitrate, chapters.", runProbe)
}

func runProbe(ctx context.Context, c *proto.Ctx, r *probeReq) proto.Result {
	if err := oneOf("format", orDefault(r.Format, "summary"), "summary", "json"); err != nil {
		return proto.Fail("%v", err)
	}
	path, err := c.Input("input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	info, err := ff.Probe(ctx, path)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if r.Format == "json" {
		info.Format.Filename = c.Rel(path)
		b, _ := json.MarshalIndent(info, "", "  ")
		return proto.OK("%s", b)
	}
	return proto.OK("%s", summarize(c, path, info))
}

func summarize(c *proto.Ctx, path string, info *ff.Info) string {
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s\n", c.Rel(path))
	fmt.Fprintf(&b, "Container: %s (%s)\n", info.Format.FormatName, info.Format.LongName)
	fmt.Fprintf(&b, "Duration: %s (%.3f s)\n", proto.Clock(info.Duration()), info.Duration())
	size, _ := strconv.ParseInt(info.Format.Size, 10, 64)
	br, _ := strconv.ParseFloat(info.Format.BitRate, 64)
	fmt.Fprintf(&b, "Size: %s, overall bitrate %.0f kb/s\n", proto.Size(size), br/1000)
	if t := info.Format.Tags["title"]; t != "" {
		fmt.Fprintf(&b, "Title: %s\n", t)
	}
	b.WriteString("Streams:\n")
	for _, s := range info.Streams {
		lang := s.Tags["language"]
		if lang != "" {
			lang = " [" + lang + "]"
		}
		sbr, _ := strconv.ParseFloat(s.BitRate, 64)
		brs := ""
		if sbr > 0 {
			brs = fmt.Sprintf(", %.0f kb/s", sbr/1000)
		}
		switch s.CodecType {
		case "video":
			kind := "video"
			if s.Disposition["attached_pic"] == 1 {
				kind = "cover art"
			}
			w, h := s.DisplaySize()
			rot := ""
			if r := s.Rotation(); r != 0 {
				rot = fmt.Sprintf(", rotated %d°", r)
			}
			fmt.Fprintf(&b, "  #%d %s: %s %s, %dx%d, %.3g fps, %s%s%s%s\n", s.Index, kind, s.CodecName, s.Profile, w, h, s.FPS(), s.PixFmt, brs, rot, lang)
		case "audio":
			layout := s.ChannelLay
			if layout == "" {
				layout = fmt.Sprintf("%d ch", s.Channels)
			}
			fmt.Fprintf(&b, "  #%d audio: %s, %s Hz, %s%s%s\n", s.Index, s.CodecName, s.SampleRate, layout, brs, lang)
		case "subtitle":
			fmt.Fprintf(&b, "  #%d subtitles: %s%s\n", s.Index, s.CodecName, lang)
		default:
			fmt.Fprintf(&b, "  #%d %s: %s\n", s.Index, s.CodecType, s.CodecName)
		}
	}
	if len(info.Chapters) > 0 {
		fmt.Fprintf(&b, "Chapters (%d):\n", len(info.Chapters))
		for i, ch := range info.Chapters {
			if i == 50 {
				fmt.Fprintf(&b, "  … %d more\n", len(info.Chapters)-50)
				break
			}
			st, _ := strconv.ParseFloat(ch.StartTime, 64)
			fmt.Fprintf(&b, "  %s %s\n", proto.Clock(st), ch.Tags["title"])
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// --------------------------------------------------------------- video_frames

type framesReq struct {
	Input       string       `json:"input"`
	Mode        string       `json:"mode"`
	Count       int          `json:"count"`
	Interval    proto.Time   `json:"interval"`
	Timestamps  []proto.Time `json:"timestamps"`
	Width       int          `json:"width"`
	ImageFormat string       `json:"image_format"`
	Columns     int          `json:"columns"`
	Labels      *bool        `json:"labels"`
	OutputDir   string       `json:"output_dir"`
	Attach      *bool        `json:"attach"`
}

func init() {
	register("video_frames", "Extract still frames or a contact sheet from a video so an agent can see what is in it.", runFrames)
}

const maxFrames = 300

func runFrames(ctx context.Context, c *proto.Ctx, r *framesReq) proto.Result {
	mode := orDefault(r.Mode, "count")
	if err := oneOf("mode", mode, "count", "interval", "timestamps", "contact_sheet"); err != nil {
		return proto.Fail("%v", err)
	}
	format := orDefault(r.ImageFormat, "jpg")
	if err := oneOf("image_format", format, "jpg", "png"); err != nil {
		return proto.Fail("%v", err)
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasVideo() {
		return proto.Fail("%s has no video stream", r.Input)
	}
	d := m.duration()
	width := r.Width
	if width == 0 {
		width = 640
		if mode == "contact_sheet" {
			width = 320
		}
	}
	if width < 16 || width > 7680 {
		return proto.Fail("width must be between 16 and 7680")
	}

	var times []float64
	switch mode {
	case "count", "contact_sheet":
		n := r.Count
		if n == 0 {
			n = 6
			if mode == "contact_sheet" {
				n = 12
			}
		}
		if n < 1 || n > 100 {
			return proto.Fail("count must be between 1 and 100")
		}
		for i := 0; i < n; i++ {
			times = append(times, d*(float64(i)+0.5)/float64(n))
		}
	case "interval":
		if !r.Interval.Set || r.Interval.Seconds <= 0 {
			return proto.Fail("interval is required for mode \"interval\" (for example 10 for one frame every 10 seconds)")
		}
		for t := 0.0; t < d; t += r.Interval.Seconds {
			times = append(times, t)
			if len(times) > maxFrames {
				return proto.Fail("that interval gives more than %d frames for a %s video; use a larger interval", maxFrames, proto.Clock(d))
			}
		}
	case "timestamps":
		if len(r.Timestamps) == 0 {
			return proto.Fail("timestamps is required for mode \"timestamps\"")
		}
		if len(r.Timestamps) > maxFrames {
			return proto.Fail("at most %d timestamps", maxFrames)
		}
		for _, t := range r.Timestamps {
			if d > 0 && t.Seconds >= d {
				return proto.Fail("timestamp %s is past the end of the video (%s)", proto.Clock(t.Seconds), proto.Clock(d))
			}
			times = append(times, t.Seconds)
		}
	}

	base := strings.TrimSuffix(filepath.Base(m.Path), filepath.Ext(m.Path))
	var outDir string
	switch {
	case mode == "contact_sheet" && r.OutputDir == "":
		// a single image next to the input; no frames folder
	case r.OutputDir == "":
		dir := filepath.Join(filepath.Dir(m.Path), base+"_frames")
		if !strings.HasPrefix(dir, c.Workspace) {
			dir = filepath.Join(c.Workspace, base+"_frames")
		}
		for i := 2; ; i++ {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				break
			}
			dir = filepath.Join(filepath.Dir(dir), fmt.Sprintf("%s_frames_%d", base, i))
		}
		outDir, err = c.Dir("output_dir", dir, "")
	default:
		outDir, err = c.Dir("output_dir", r.OutputDir, "")
	}
	if err != nil {
		return proto.Fail("%v", err)
	}

	labels := mode == "contact_sheet"
	if r.Labels != nil {
		labels = *r.Labels
	}
	font := ""
	if labels {
		font = ff.FindFont()
	}
	files := map[string]string{}
	if font != "" {
		files["font.ttf"] = font
	}
	work, cleanup, err := ff.Workdir(c, files)
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()

	var outputs []string
	for i, t := range times {
		vf := fmt.Sprintf("scale=%d:-2", width)
		if font != "" {
			label := fmt.Sprintf("l%03d.txt", i)
			if err := os.WriteFile(filepath.Join(work, label), []byte(proto.Clock(t)), 0o644); err != nil {
				return proto.Fail("%v", err)
			}
			vf += fmt.Sprintf(",drawtext=fontfile=font.ttf:textfile=%s:fontsize=h/14:fontcolor=white:box=1:boxcolor=black@0.6:boxborderw=6:x=8:y=h-th-12", label)
		}
		name := fmt.Sprintf("frame_%03d_%s.%s", i+1, strings.NewReplacer(":", "m", ".", "s").Replace(proto.Clock(t)), format)
		target := filepath.Join(outDir, name)
		if mode == "contact_sheet" {
			target = filepath.Join(work, fmt.Sprintf("f%03d.png", i))
		}
		args := []string{"-ss", ff.Sec(t), "-i", m.Path, "-frames:v", "1", "-vf", vf, "-an", "-sn"}
		if format == "jpg" && mode != "contact_sheet" {
			args = append(args, "-q:v", "3")
		}
		args = append(args, "-update", "1", target)
		if _, err := ff.Run(ctx, ff.Job{Args: args, Dir: work}); err != nil {
			return proto.Fail("frame at %s: %v", proto.Clock(t), err)
		}
		if proto.FileSize(target) == 0 { // seeking past the last keyframe can yield nothing
			continue
		}
		outputs = append(outputs, target)
		c.Notify(false, "video_frames: %d of %d frames", i+1, len(times))
	}
	if len(outputs) == 0 {
		return proto.Fail("no frames could be extracted")
	}

	attach := r.Attach == nil || *r.Attach
	if mode == "contact_sheet" {
		cols := r.Columns
		if cols == 0 {
			cols = int(math.Ceil(math.Sqrt(float64(len(outputs)))))
			if cols > 6 {
				cols = 6
			}
		}
		if cols < 1 || cols > 20 {
			return proto.Fail("columns must be between 1 and 20")
		}
		rows := (len(outputs) + cols - 1) / cols
		sheet, err := c.Output("", m.Path, "contact_sheet", format, false)
		if err != nil {
			return proto.Fail("%v", err)
		}
		if r.OutputDir != "" {
			sheet = filepath.Join(outDir, base+"_contact_sheet."+format)
		}
		args := []string{"-framerate", "1", "-start_number", "0", "-i", "f%03d.png",
			"-vf", fmt.Sprintf("tile=%dx%d:padding=4:margin=4:color=0x202020", cols, rows), "-frames:v", "1"}
		if format == "jpg" {
			args = append(args, "-q:v", "3")
		}
		args = append(args, "-update", "1", sheet)
		if err := renumber(work, outputs); err != nil {
			return proto.Fail("%v", err)
		}
		if _, err := ff.Run(ctx, ff.Job{Args: args, Dir: work}); err != nil {
			return proto.Fail("contact sheet: %v", err)
		}
		res := proto.OK("Contact sheet of %d frames (%d×%d grid) from %s (%s long):\n%s\nFrames are in reading order at: %s",
			len(outputs), cols, rows, c.Rel(m.Path), proto.Clock(d), c.Rel(sheet), joinTimes(times))
		if attach {
			c.Attach(&res, []string{sheet}, 1)
		}
		return res
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Extracted %d frames from %s (%s long) into %s:\n", len(outputs), c.Rel(m.Path), proto.Clock(d), c.Rel(outDir))
	for _, o := range outputs {
		fmt.Fprintf(&b, "  %s\n", filepath.Base(o))
	}
	res := proto.OK("%s", strings.TrimRight(b.String(), "\n"))
	if attach {
		c.Attach(&res, outputs, 16)
		if c.Vision && len(outputs) > 16 {
			res.Output += "\n(The first 16 frames are attached; open the others with view_file if needed.)"
		}
	}
	return res
}

// renumber makes frame files contiguous (f000, f001, …) after skipped frames.
func renumber(dir string, files []string) error {
	for i, f := range files {
		want := filepath.Join(dir, fmt.Sprintf("f%03d.png", i))
		if f != want {
			if err := os.Rename(f, want); err != nil {
				return err
			}
		}
	}
	return nil
}

func joinTimes(ts []float64) string {
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = proto.Clock(t)
	}
	return strings.Join(s, ", ")
}

// --------------------------------------------------------------- scene_detect

type sceneReq struct {
	Input      string     `json:"input"`
	Threshold  float64    `json:"threshold"`
	MinScene   proto.Time `json:"min_scene_length"`
	MaxScenes  int        `json:"max_scenes"`
	Thumbnails bool       `json:"thumbnails"`
	OutputDir  string     `json:"output_dir"`
}

func init() {
	register("scene_detect", "Find the cuts and scene changes in a video, with optional thumbnails of each scene.", runScenes)
}

var ptsRe = regexp.MustCompile(`pts_time:\s*([0-9.]+)`)

func runScenes(ctx context.Context, c *proto.Ctx, r *sceneReq) proto.Result {
	th := r.Threshold
	if th == 0 {
		th = 0.3
	}
	if th < 0.05 || th > 0.95 {
		return proto.Fail("threshold must be between 0.05 and 0.95 (lower finds more, subtler changes)")
	}
	minLen := 1.0
	if r.MinScene.Set {
		minLen = r.MinScene.Seconds
	}
	maxScenes := r.MaxScenes
	if maxScenes == 0 {
		maxScenes = 200
	}
	if maxScenes < 1 || maxScenes > 2000 {
		return proto.Fail("max_scenes must be between 1 and 2000")
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasVideo() {
		return proto.Fail("%s has no video stream", r.Input)
	}
	d := m.duration()
	c.Notify(true, "scene_detect: analysing %s of video", proto.Clock(d))
	log, err := ff.RunLog(ctx, "info", "-i", m.Path, "-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("scale=320:-2,select='gt(scene,%.3f)',showinfo", th), "-f", "null", "-")
	if err != nil {
		return proto.Fail("%v", err)
	}
	var cuts []float64
	for _, line := range strings.Split(log, "\n") {
		if !strings.Contains(line, "showinfo") {
			continue
		}
		if mm := ptsRe.FindStringSubmatch(line); mm != nil {
			t, _ := strconv.ParseFloat(mm[1], 64)
			cuts = append(cuts, t)
		}
	}
	sort.Float64s(cuts)
	starts := []float64{0}
	for _, t := range cuts {
		if t-starts[len(starts)-1] >= minLen && (d == 0 || d-t >= minLen/2) {
			starts = append(starts, t)
		}
	}
	truncated := false
	if len(starts) > maxScenes {
		starts, truncated = starts[:maxScenes], true
	}

	var thumbs []string
	var outDir string
	if r.Thumbnails {
		base := strings.TrimSuffix(filepath.Base(m.Path), filepath.Ext(m.Path))
		outDir, err = c.Dir("output_dir", r.OutputDir, filepath.Join(c.Rel(filepath.Dir(m.Path)), base+"_scenes"))
		if err != nil {
			return proto.Fail("%v", err)
		}
		for i, s := range starts {
			t := s + 0.1
			if i+1 < len(starts) {
				t = s + math.Min(0.5, (starts[i+1]-s)/2)
			}
			p := filepath.Join(outDir, fmt.Sprintf("scene_%03d.jpg", i+1))
			if _, err := ff.Run(ctx, ff.Job{Args: []string{"-ss", ff.Sec(t), "-i", m.Path, "-frames:v", "1", "-vf", "scale=480:-2", "-q:v", "4", "-update", "1", p}}); err == nil && proto.FileSize(p) > 0 {
				thumbs = append(thumbs, p)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d scenes in %s (%s long, threshold %.2f):\n", len(starts), c.Rel(m.Path), proto.Clock(d), th)
	for i, s := range starts {
		e := d
		if i+1 < len(starts) {
			e = starts[i+1]
		}
		fmt.Fprintf(&b, "  %3d. %s – %s (%.1f s)\n", i+1, proto.Clock(s), proto.Clock(e), e-s)
	}
	if truncated {
		fmt.Fprintf(&b, "(stopped at max_scenes=%d)\n", maxScenes)
	}
	if len(thumbs) > 0 {
		fmt.Fprintf(&b, "Thumbnails: %s/scene_NNN.jpg\n", c.Rel(outDir))
	}
	res := proto.OK("%s", strings.TrimRight(b.String(), "\n"))
	c.Attach(&res, thumbs, 16)
	return res
}
