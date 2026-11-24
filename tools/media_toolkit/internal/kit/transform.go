// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// --------------------------------------------------------------- video_resize

type resizeReq struct {
	Input     string `json:"input"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Aspect    string `json:"aspect"`
	Fit       string `json:"fit"`
	Focus     string `json:"focus"`
	PadColor  string `json:"pad_color"`
	Output    string `json:"output"`
	Quality   string `json:"quality"`
	Overwrite bool   `json:"overwrite"`
}

func init() {
	register("video_resize", "Resize a video or change its aspect ratio (e.g. 16:9 to 9:16 for Shorts/Reels) by cropping, padding or a blurred background.", runResize)
}

func parseAspect(s string) (float64, error) {
	a, b, ok := strings.Cut(strings.ReplaceAll(s, "/", ":"), ":")
	if !ok {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			return v, nil
		}
		return 0, fmt.Errorf("aspect must look like 16:9, 9:16, 1:1 or 4:5, got %q", s)
	}
	x, err1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
	y, err2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if err1 != nil || err2 != nil || x <= 0 || y <= 0 {
		return 0, fmt.Errorf("aspect must look like 16:9, 9:16, 1:1 or 4:5, got %q", s)
	}
	return x / y, nil
}

func runResize(ctx context.Context, c *proto.Ctx, r *resizeReq) proto.Result {
	fit := orDefault(r.Fit, "pad")
	if err := oneOf("fit", fit, "pad", "crop", "blur", "stretch"); err != nil {
		return proto.Fail("%v", err)
	}
	focus := orDefault(r.Focus, "center")
	if err := oneOf("focus", focus, "center", "left", "right", "top", "bottom"); err != nil {
		return proto.Fail("%v", err)
	}
	if r.Width < 0 || r.Height < 0 || r.Width > 7680 || r.Height > 7680 {
		return proto.Fail("width and height must be between 2 and 7680")
	}
	if r.Aspect == "" && r.Width == 0 && r.Height == 0 {
		return proto.Fail("give width and/or height, or an aspect ratio")
	}
	if r.Aspect != "" && r.Width > 0 && r.Height > 0 {
		return proto.Fail("with an aspect ratio, give at most one of width or height")
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	v := m.Info.Video()
	if v == nil {
		return proto.Fail("%s has no video stream", r.Input)
	}
	sw, sh := v.DisplaySize()
	src := float64(sw) / float64(sh)

	tw, th := r.Width, r.Height
	keepAspect := false
	if r.Aspect != "" {
		ar, err := parseAspect(r.Aspect)
		if err != nil {
			return proto.Fail("%v", err)
		}
		switch {
		case tw > 0:
			th = int(math.Round(float64(tw) / ar))
		case th > 0:
			tw = int(math.Round(float64(th) * ar))
		case (fit == "crop") == (src > ar):
			// crop: largest region of the new shape inside the frame;
			// pad/blur/stretch: smallest canvas that holds the whole frame.
			th, tw = sh, int(math.Round(float64(sh)*ar))
		default:
			tw, th = sw, int(math.Round(float64(sw)/ar))
		}
	} else if tw == 0 || th == 0 {
		keepAspect = true
		if tw == 0 {
			tw = int(math.Round(float64(th) * src))
		} else {
			th = int(math.Round(float64(tw) / src))
		}
	}
	tw, th = ff.Even(tw), ff.Even(th)

	color := orDefault(r.PadColor, "black")
	if strings.ContainsAny(color, ":;,[]'\" ") {
		return proto.Fail("pad_color must be a color name like black or white, or a hex value like 0x1E1E1E")
	}
	cx, cy := "(iw-ow)/2", "(ih-oh)/2"
	switch focus {
	case "left":
		cx = "0"
	case "right":
		cx = "iw-ow"
	case "top":
		cy = "0"
	case "bottom":
		cy = "ih-oh"
	}
	var graph string
	switch {
	case keepAspect || fit == "stretch":
		graph = fmt.Sprintf("[0:v:0]scale=%d:%d,setsar=1[v]", tw, th)
	case fit == "crop":
		graph = fmt.Sprintf("[0:v:0]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d:%s:%s,setsar=1[v]", tw, th, tw, th, cx, cy)
	case fit == "pad":
		graph = fmt.Sprintf("[0:v:0]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=%s,setsar=1[v]", tw, th, tw, th, color)
	case fit == "blur":
		graph = fmt.Sprintf("[0:v:0]split=2[a][b];[a]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,boxblur=luma_radius=min(h\\,w)/20:luma_power=2,eq=brightness=-0.08[bg];"+
			"[b]scale=%d:%d:force_original_aspect_ratio=decrease[fg];[bg][fg]overlay=(W-w)/2:(H-h)/2,setsar=1[v]", tw, th, tw, th, tw, th)
	}
	ext := sameExt(r.Output, m.Path)
	if ff.AudioOnlyExt(ext) {
		return proto.Fail("the output must be a video format")
	}
	out, err := c.Output(r.Output, m.Path, fmt.Sprintf("%dx%d", tw, th), ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(ext, true, m.hasAudio(), crf, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args := []string{"-i", m.Path, "-filter_complex", graph, "-map", "[v]", "-map", "0:a?"}
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: m.duration(), Label: "video_resize", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	how := fit
	if keepAspect {
		how = "kept aspect ratio"
	}
	return proto.OK("Resized %s from %dx%d to %dx%d (%s):\n%s", c.Rel(m.Path), sw, sh, tw, th, how, describe(ctx, c, out))
}

// --------------------------------------------------------------- video_rotate

type rotateReq struct {
	Input     string `json:"input"`
	Rotate    int    `json:"rotate"`
	Flip      string `json:"flip"`
	Output    string `json:"output"`
	Quality   string `json:"quality"`
	Overwrite bool   `json:"overwrite"`
}

func init() {
	register("video_rotate", "Rotate a video by 90/180/270 degrees and/or mirror it horizontally or vertically.", runRotate)
}

func runRotate(ctx context.Context, c *proto.Ctx, r *rotateReq) proto.Result {
	flip := orDefault(r.Flip, "none")
	if err := oneOf("flip", flip, "none", "horizontal", "vertical", "both"); err != nil {
		return proto.Fail("%v", err)
	}
	rot := ((r.Rotate % 360) + 360) % 360
	if rot%90 != 0 {
		return proto.Fail("rotate must be a multiple of 90 (90 = clockwise, -90 or 270 = counter-clockwise)")
	}
	if rot == 0 && flip == "none" {
		return proto.Fail("nothing to do: give rotate and/or flip")
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasVideo() {
		return proto.Fail("%s has no video stream", r.Input)
	}
	var f []string
	switch rot {
	case 90:
		f = append(f, "transpose=clock")
	case 180:
		f = append(f, "hflip", "vflip")
	case 270:
		f = append(f, "transpose=cclock")
	}
	switch flip {
	case "horizontal":
		f = append(f, "hflip")
	case "vertical":
		f = append(f, "vflip")
	case "both":
		f = append(f, "hflip", "vflip")
	}
	ext := sameExt(r.Output, m.Path)
	out, err := c.Output(r.Output, m.Path, "rotated", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(ext, true, m.hasAudio(), crf, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args := []string{"-i", m.Path, "-map", "0:v:0", "-map", "0:a?", "-vf", strings.Join(f, ",") + ",setsar=1", "-metadata:s:v:0", "rotate=0"}
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: m.duration(), Label: "video_rotate", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Rotated %s (rotate %d°, flip %s):\n%s", c.Rel(m.Path), rot, flip, describe(ctx, c, out))
}

// ---------------------------------------------------------------- video_speed

type speedReq struct {
	Input     string  `json:"input"`
	Speed     float64 `json:"speed"`
	KeepPitch *bool   `json:"keep_pitch"`
	Reverse   bool    `json:"reverse"`
	Output    string  `json:"output"`
	Quality   string  `json:"quality"`
	Overwrite bool    `json:"overwrite"`
}

func init() {
	register("video_speed", "Speed up, slow down or reverse a video or audio file, keeping the voice pitch natural.", runSpeed)
}

// atempo chains tempo filters; each atempo accepts 0.5–2.0.
func atempo(speed float64) []string {
	var f []string
	for speed > 2.0 {
		f = append(f, "atempo=2.0")
		speed /= 2.0
	}
	for speed < 0.5 {
		f = append(f, "atempo=0.5")
		speed /= 0.5
	}
	if math.Abs(speed-1) > 1e-6 {
		f = append(f, fmt.Sprintf("atempo=%.6f", speed))
	}
	return f
}

func runSpeed(ctx context.Context, c *proto.Ctx, r *speedReq) proto.Result {
	speed := r.Speed
	if speed == 0 {
		speed = 1
	}
	if speed < 0.25 || speed > 8 {
		return proto.Fail("speed must be between 0.25 (4× slower) and 8 (8× faster)")
	}
	if speed == 1 && !r.Reverse {
		return proto.Fail("nothing to do: set speed (for example 2 for double speed, 0.5 for half) and/or reverse")
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
	if r.Reverse && d > 600 {
		return proto.Fail("reverse holds the whole file in memory, so it is limited to 10 minutes; trim the media first (it is %s)", proto.Clock(d))
	}
	keepPitch := r.KeepPitch == nil || *r.KeepPitch
	ext := sameExt(r.Output, m.Path)
	video := m.hasVideo() && !ff.AudioOnlyExt(ext)
	var vf, af []string
	if r.Reverse {
		vf = append(vf, "reverse")
		af = append(af, "areverse")
	}
	if speed != 1 {
		vf = append(vf, fmt.Sprintf("setpts=PTS/%.6f", speed))
		if keepPitch {
			af = append(af, atempo(speed)...)
		} else if a := m.Info.Audio(); a != nil {
			sr, _ := strconv.Atoi(a.SampleRate)
			if sr == 0 {
				sr = 48000
			}
			af = append(af, fmt.Sprintf("asetrate=%d", int(float64(sr)*speed)), fmt.Sprintf("aresample=%d", sr))
		}
	}
	suffix := fmt.Sprintf("%gx", speed)
	if r.Reverse {
		suffix = "reversed"
		if speed != 1 {
			suffix = fmt.Sprintf("reversed_%gx", speed)
		}
	}
	out, err := c.Output(r.Output, m.Path, suffix, ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(ext, video, m.hasAudio(), crf, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args := []string{"-i", m.Path}
	if video {
		args = append(args, "-map", "0:v:0", "-vf", strings.Join(vf, ","))
	}
	if m.hasAudio() {
		args = append(args, "-map", "0:a:0")
		if len(af) > 0 {
			args = append(args, "-af", strings.Join(af, ","))
		}
	}
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: d / speed, Label: "video_speed", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	what := fmt.Sprintf("speed %g×", speed)
	if r.Reverse {
		what = "reversed, " + what
	}
	return proto.OK("Changed %s (%s, %s → %s):\n%s", c.Rel(m.Path), what, proto.Clock(d), proto.Clock(d/speed), describe(ctx, c, out))
}
