// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// ------------------------------------------------------------ video_watermark

type watermarkReq struct {
	Input      string     `json:"input"`
	Image      string     `json:"image"`
	Text       string     `json:"text"`
	Position   string     `json:"position"`
	Margin     int        `json:"margin"`
	Opacity    float64    `json:"opacity"`
	Size       float64    `json:"size"`
	TextColor  string     `json:"text_color"`
	Background *bool      `json:"background"`
	Start      proto.Time `json:"start"`
	End        proto.Time `json:"end"`
	Output     string     `json:"output"`
	Quality    string     `json:"quality"`
	Overwrite  bool       `json:"overwrite"`
}

func init() {
	register("video_watermark", "Overlay a logo image and/or a text caption on a video, at a chosen position and time range.", runWatermark)
}

var colorRe = regexp.MustCompile(`^(#|0x)?[A-Za-z0-9]{1,20}$`)

func runWatermark(ctx context.Context, c *proto.Ctx, r *watermarkReq) proto.Result {
	if r.Image == "" && strings.TrimSpace(r.Text) == "" {
		return proto.Fail("give an image, a text, or both")
	}
	pos := orDefault(r.Position, "bottom_right")
	if err := oneOf("position", pos, "top_left", "top", "top_right", "left", "center", "right", "bottom_left", "bottom", "bottom_right"); err != nil {
		return proto.Fail("%v", err)
	}
	opacity := r.Opacity
	if opacity == 0 {
		opacity = 0.85
	}
	if opacity < 0.05 || opacity > 1 {
		return proto.Fail("opacity must be between 0.05 and 1")
	}
	margin := r.Margin
	if margin == 0 {
		margin = 24
	}
	if margin < 0 || margin > 2000 {
		return proto.Fail("margin must be between 0 and 2000 pixels")
	}
	color := orDefault(r.TextColor, "white")
	if !colorRe.MatchString(color) {
		return proto.Fail("text_color must be a color name (white, yellow) or hex (#FFCC00)")
	}
	color = strings.Replace(color, "#", "0x", 1)
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
	vw, vh := v.DisplaySize()
	enable := ""
	if r.Start.Set || r.End.Set {
		s, e, err := span(r.Start, r.End, proto.Time{}, m.duration())
		if err != nil {
			return proto.Fail("%v", err)
		}
		enable = fmt.Sprintf(":enable='between(t,%s,%s)'", ff.Sec(s), ff.Sec(e))
	}
	place := func(w, h string) (string, string) {
		x, y := fmt.Sprintf("(W-%s)/2", w), fmt.Sprintf("(H-%s)/2", h)
		if strings.Contains(pos, "left") {
			x = fmt.Sprint(margin)
		}
		if strings.Contains(pos, "right") {
			x = fmt.Sprintf("W-%s-%d", w, margin)
		}
		if strings.HasPrefix(pos, "top") {
			y = fmt.Sprint(margin)
		}
		if strings.HasPrefix(pos, "bottom") {
			y = fmt.Sprintf("H-%s-%d", h, margin)
		}
		return x, y
	}

	files := map[string]string{}
	args := []string{"-i", m.Path}
	graph := "[0:v:0]null[base]"
	last := "base"
	if r.Image != "" {
		img, err := c.Input("image", r.Image)
		if err != nil {
			return proto.Fail("%v", err)
		}
		size := r.Size
		if size == 0 {
			size = 0.15
		}
		if size < 0.01 || size > 1 {
			return proto.Fail("size must be between 0.01 and 1 (a fraction of the video width for images)")
		}
		lw := ff.Even(int(float64(vw) * size))
		args = append(args, "-loop", "1", "-i", img)
		x, y := place("w", "h")
		graph += fmt.Sprintf(";[1:v]scale=%d:-2,format=rgba,colorchannelmixer=aa=%.3f[logo];[%s][logo]overlay=%s:%s:shortest=1%s[wm]", lw, opacity, last, x, y, enable)
		last = "wm"
	}
	if text := strings.TrimSpace(r.Text); text != "" {
		font := ff.FindFont()
		if font == "" {
			return proto.Fail("no font was found for drawing text; set ZTOOLS_FONT to the path of a .ttf font")
		}
		files["font.ttf"] = font
		size := r.Size
		if size == 0 || r.Image != "" {
			size = 0.05
		}
		if size < 0.01 || size > 0.5 {
			return proto.Fail("size must be between 0.01 and 0.5 for text (a fraction of the video height)")
		}
		fs := int(float64(vh) * size)
		if fs < 8 {
			fs = 8
		}
		bg := r.Background == nil || *r.Background
		box := ""
		if bg {
			box = fmt.Sprintf(":box=1:boxcolor=black@%.2f:boxborderw=%d", 0.45*opacity, fs/3+2)
		}
		x, y := place("tw", "th")
		x = strings.ReplaceAll(x, "W", "w")
		y = strings.ReplaceAll(y, "H", "h")
		graph += fmt.Sprintf(";[%s]drawtext=fontfile=font.ttf:textfile=caption.txt:expansion=none:fontsize=%d:fontcolor=%s@%.3f:line_spacing=%d%s:x=%s:y=%s%s[tx]",
			last, fs, color, opacity, fs/4, box, x, y, enable)
		last = "tx"
	}
	graph += fmt.Sprintf(";[%s]setsar=1[v]", last)

	work, cleanup, err := ff.Workdir(c, files)
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()
	if t := strings.TrimSpace(r.Text); t != "" {
		if err := os.WriteFile(filepath.Join(work, "caption.txt"), []byte(t), 0o644); err != nil {
			return proto.Fail("%v", err)
		}
	}
	ext := sameExt(r.Output, m.Path)
	out, err := c.Output(r.Output, m.Path, "watermarked", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(ext, true, m.hasAudio(), crf, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args = append(args, "-filter_complex", graph, "-map", "[v]", "-map", "0:a?", "-t", ff.Sec(m.duration()))
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Dir: work, Duration: m.duration(), Label: "video_watermark", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Added the watermark (%s) to %s:\n%s", strings.ReplaceAll(pos, "_", " "), c.Rel(m.Path), describe(ctx, c, out))
}

// ------------------------------------------------------------ video_subtitles

type subtitlesReq struct {
	Input     string `json:"input"`
	Subtitles string `json:"subtitles"`
	Mode      string `json:"mode"`
	Language  string `json:"language"`
	FontSize  int    `json:"font_size"`
	Position  string `json:"position"`
	Output    string `json:"output"`
	Quality   string `json:"quality"`
	Overwrite bool   `json:"overwrite"`
}

func init() {
	register("video_subtitles", "Add subtitles (SRT, VTT or ASS) to a video, burned into the picture or as a selectable track.", runSubtitles)
}

var langRe = regexp.MustCompile(`^[a-z]{2,3}$`)

func runSubtitles(ctx context.Context, c *proto.Ctx, r *subtitlesReq) proto.Result {
	mode := orDefault(r.Mode, "burn")
	if err := oneOf("mode", mode, "burn", "soft"); err != nil {
		return proto.Fail("%v", err)
	}
	position := orDefault(r.Position, "bottom")
	if err := oneOf("position", position, "bottom", "top"); err != nil {
		return proto.Fail("%v", err)
	}
	if r.Language != "" && !langRe.MatchString(r.Language) {
		return proto.Fail("language must be an ISO 639 code such as en, eng, hi or hin")
	}
	if r.FontSize < 0 || r.FontSize > 200 {
		return proto.Fail("font_size must be between 1 and 200")
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
	subs, err := c.Input("subtitles", r.Subtitles)
	if err != nil {
		return proto.Fail("%v", err)
	}
	subExt := extOf(subs, "")
	if err := oneOf("subtitles file type", subExt, "srt", "vtt", "ass", "ssa"); err != nil {
		return proto.Fail("%v", err)
	}
	ext := sameExt(r.Output, m.Path)
	out, err := c.Output(r.Output, m.Path, "subtitled", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	work, cleanup, err := ff.Workdir(c, map[string]string{"subs." + subExt: subs})
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()

	args := []string{"-i", m.Path}
	if mode == "burn" {
		var style []string
		if r.FontSize > 0 {
			style = append(style, fmt.Sprintf("FontSize=%d", r.FontSize))
		}
		if subExt != "ass" && subExt != "ssa" {
			style = append(style, "Outline=2", "Shadow=1", "BorderStyle=1")
			if position == "top" {
				style = append(style, "Alignment=8")
			}
		}
		vf := "subtitles=subs." + subExt
		if len(style) > 0 {
			vf += ":force_style='" + strings.Join(style, ",") + "'"
		}
		enc, err := ff.Encoders(ext, true, m.hasAudio(), crf, "")
		if err != nil {
			return proto.Fail("%v", err)
		}
		args = append(args, "-map", "0:v:0", "-map", "0:a?", "-vf", vf)
		args = append(args, enc...)
	} else {
		codec := ""
		switch ext {
		case "mp4", "m4v", "mov":
			codec = "mov_text"
		case "mkv":
			codec = "srt"
			if subExt == "ass" || subExt == "ssa" {
				codec = "ass"
			}
		case "webm":
			codec = "webvtt"
		default:
			return proto.Fail("soft subtitles need an mp4, mov, mkv or webm output")
		}
		args = append(args, "-i", "subs."+subExt, "-map", "0:v", "-map", "0:a?", "-map", "1:0", "-c", "copy", "-c:s", codec, "-disposition:s:0", "default")
		if r.Language != "" {
			args = append(args, "-metadata:s:s:0", "language="+r.Language)
		}
	}
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Dir: work, Duration: m.duration(), Label: "video_subtitles", Ctx: c}); err != nil {
		_ = os.Remove(out)
		if mode == "burn" && strings.Contains(err.Error(), "No such filter") {
			return proto.Fail("this FFmpeg build has no subtitles filter (libass); use mode \"soft\" or install a full FFmpeg build")
		}
		return proto.Fail("%v", err)
	}
	how := "burned into the picture"
	if mode == "soft" {
		how = "added as a selectable subtitle track"
	}
	return proto.OK("Subtitles from %s %s:\n%s", c.Rel(subs), how, describe(ctx, c, out))
}
