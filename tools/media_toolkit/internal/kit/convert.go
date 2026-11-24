// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// -------------------------------------------------------------- media_convert

type convertReq struct {
	Input        string  `json:"input"`
	Output       string  `json:"output"`
	Format       string  `json:"format"`
	VideoCodec   string  `json:"video_codec"`
	AudioCodec   string  `json:"audio_codec"`
	Quality      string  `json:"quality"`
	AudioBitrate string  `json:"audio_bitrate"`
	MaxHeight    int     `json:"max_height"`
	FPS          float64 `json:"fps"`
	Overwrite    bool    `json:"overwrite"`
}

func init() {
	register("media_convert", "Convert a video or audio file to another format or codec (mp4, webm, mkv, mov, mp3, wav, flac, h265, av1, …).", runConvert)
}

var bitrateRe = strings.NewReplacer("k", "", "K", "")

func checkBitrate(s string) error {
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(bitrateRe.Replace(s))
	if err != nil || v < 8 || v > 1536 || !strings.HasSuffix(strings.ToLower(s), "k") {
		return fmt.Errorf("audio_bitrate must look like 128k (between 8k and 1536k), got %q", s)
	}
	return nil
}

func runConvert(ctx context.Context, c *proto.Ctx, r *convertReq) proto.Result {
	vc := orDefault(r.VideoCodec, "auto")
	if err := oneOf("video_codec", vc, "auto", "h264", "h265", "vp9", "av1", "copy"); err != nil {
		return proto.Fail("%v", err)
	}
	ac := orDefault(r.AudioCodec, "auto")
	if err := oneOf("audio_codec", ac, "auto", "aac", "mp3", "opus", "flac", "copy", "none"); err != nil {
		return proto.Fail("%v", err)
	}
	if err := checkBitrate(r.AudioBitrate); err != nil {
		return proto.Fail("%v", err)
	}
	if r.MaxHeight < 0 || r.MaxHeight > 4320 || r.FPS < 0 || r.FPS > 240 {
		return proto.Fail("max_height must be up to 4320 and fps up to 240")
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	ext := strings.TrimPrefix(strings.ToLower(r.Format), ".")
	if r.Output != "" && filepath.Ext(r.Output) != "" {
		ext = extOf(r.Output, "")
	}
	if ext == "" {
		return proto.Fail("give an output file name with an extension, or a format such as mp4, webm or mp3")
	}
	out, err := c.Output(r.Output, m.Path, "converted", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	video := m.hasVideo() && !ff.AudioOnlyExt(ext)
	audio := m.hasAudio() && ac != "none"
	enc, err := ff.Encoders(ext, video, audio, crf, r.AudioBitrate)
	if err != nil {
		return proto.Fail("%v", err)
	}
	// Explicit codecs replace the defaults chosen for the container.
	if video && vc != "auto" {
		enc = dropArgs(enc, "-c:v", "-preset", "-crf", "-b:v", "-row-mt", "-deadline", "-cpu-used")
		switch vc {
		case "copy":
			enc = append(enc, "-c:v", "copy")
		case "h264":
			enc = append(enc, "-c:v", "libx264", "-preset", "medium", "-crf", strconv.Itoa(crf))
		case "h265":
			enc = append(enc, "-c:v", "libx265", "-preset", "medium", "-crf", strconv.Itoa(crf+5))
			if ext == "mp4" || ext == "mov" || ext == "m4v" {
				enc = append(enc, "-tag:v", "hvc1")
			}
		case "vp9":
			enc = append(enc, "-c:v", "libvpx-vp9", "-crf", strconv.Itoa(crf+13), "-b:v", "0", "-row-mt", "1")
		case "av1":
			switch {
			case ff.HasEncoder("libsvtav1"):
				enc = append(enc, "-c:v", "libsvtav1", "-preset", "8", "-crf", strconv.Itoa(crf+14))
			case ff.HasEncoder("libaom-av1"):
				enc = append(enc, "-c:v", "libaom-av1", "-cpu-used", "6", "-row-mt", "1", "-crf", strconv.Itoa(crf+14), "-b:v", "0")
			default:
				return proto.Fail("this FFmpeg build has no AV1 encoder (libsvtav1 or libaom-av1)")
			}
		}
	}
	if audio && ac != "auto" {
		enc = dropArgs(enc, "-c:a", "-b:a")
		switch ac {
		case "copy":
			enc = append(enc, "-c:a", "copy")
		case "aac":
			enc = append(enc, "-c:a", "aac", "-b:a", orDefault(r.AudioBitrate, "192k"))
		case "mp3":
			enc = append(enc, "-c:a", "libmp3lame", "-b:a", orDefault(r.AudioBitrate, "192k"))
		case "opus":
			enc = append(enc, "-c:a", "libopus", "-b:a", orDefault(r.AudioBitrate, "160k"))
		case "flac":
			enc = append(enc, "-c:a", "flac")
		}
	}
	args := []string{"-i", m.Path}
	if video {
		args = append(args, "-map", "0:v:0")
		var vf []string
		if r.MaxHeight > 0 {
			vf = append(vf, fmt.Sprintf("scale=-2:'min(%d,ih)'", r.MaxHeight))
		}
		if r.FPS > 0 {
			vf = append(vf, fmt.Sprintf("fps=%g", r.FPS))
		}
		if len(vf) > 0 {
			if vc == "copy" {
				return proto.Fail("max_height and fps need re-encoding; they cannot be used with video_codec \"copy\"")
			}
			args = append(args, "-vf", strings.Join(vf, ","))
		}
	} else {
		args = append(args, "-vn")
	}
	if audio {
		args = append(args, "-map", "0:a:0")
	} else {
		args = append(args, "-an")
	}
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: m.duration(), Label: "media_convert", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Converted %s (%s) to:\n%s", c.Rel(m.Path), proto.Size(proto.FileSize(m.Path)), describe(ctx, c, out))
}

// dropArgs removes flags (and their values) from an argument list.
func dropArgs(args []string, flags ...string) []string {
	drop := map[string]bool{}
	for _, f := range flags {
		drop[f] = true
	}
	var out []string
	for i := 0; i < len(args); i++ {
		if drop[args[i]] && i+1 < len(args) {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// ------------------------------------------------------------- video_compress

type compressReq struct {
	Input        string  `json:"input"`
	TargetSizeMB float64 `json:"target_size_mb"`
	Quality      string  `json:"quality"`
	MaxHeight    int     `json:"max_height"`
	Codec        string  `json:"codec"`
	AudioBitrate string  `json:"audio_bitrate"`
	Output       string  `json:"output"`
	Overwrite    bool    `json:"overwrite"`
}

func init() {
	register("video_compress", "Make a video file smaller: hit a target size (e.g. under 25 MB) or a quality level, optionally lowering resolution.", runCompress)
}

func runCompress(ctx context.Context, c *proto.Ctx, r *compressReq) proto.Result {
	codec := orDefault(r.Codec, "h264")
	if err := oneOf("codec", codec, "h264", "h265"); err != nil {
		return proto.Fail("%v", err)
	}
	if err := checkBitrate(r.AudioBitrate); err != nil {
		return proto.Fail("%v", err)
	}
	ab := orDefault(r.AudioBitrate, "128k")
	abK, _ := strconv.Atoi(bitrateRe.Replace(ab))
	crf := 0
	switch orDefault(r.Quality, "medium") {
	case "high":
		crf = 22
	case "medium":
		crf = 26
	case "low":
		crf = 30
	default:
		return proto.Fail("quality must be high, medium or low")
	}
	if codec == "h265" {
		crf += 4
	}
	if r.TargetSizeMB < 0 || r.MaxHeight < 0 || r.MaxHeight > 4320 {
		return proto.Fail("target_size_mb and max_height must be positive")
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasVideo() {
		return proto.Fail("%s has no video stream; use audio_extract or media_convert for audio", r.Input)
	}
	d := m.duration()
	if d <= 0 {
		return proto.Fail("cannot compress %s: its duration is unknown", r.Input)
	}
	ext := sameExt(r.Output, m.Path)
	if ext != "mp4" && ext != "mkv" && ext != "mov" {
		ext = "mp4"
	}
	out, err := c.Output(r.Output, m.Path, "compressed", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	lib := "libx264"
	if codec == "h265" {
		lib = "libx265"
	}
	base := []string{"-i", m.Path, "-map", "0:v:0"}
	if r.MaxHeight > 0 {
		base = append(base, "-vf", fmt.Sprintf("scale=-2:'min(%d,ih)'", r.MaxHeight))
	}
	tag := []string{}
	if codec == "h265" && ext != "mkv" {
		tag = []string{"-tag:v", "hvc1"}
	}
	audio := []string{"-an"}
	if m.hasAudio() {
		audio = []string{"-map", "0:a:0", "-c:a", "aac", "-b:a", ab}
	} else {
		abK = 0
	}
	finish := []string{"-pix_fmt", "yuv420p", "-movflags", "+faststart"}
	if ext == "mkv" {
		finish = finish[:2]
	}

	how := ""
	if r.TargetSizeMB > 0 {
		totalK := r.TargetSizeMB * 8 * 1024 * 0.96 / d // 4% container overhead
		videoK := int(totalK) - abK
		if videoK < 60 {
			return proto.Fail("%.1f MB is too small for %s of video (that leaves %d kb/s for the picture). Ask for a larger size, set a lower audio_bitrate, or trim the video first.", r.TargetSizeMB, proto.Clock(d), videoK)
		}
		work, cleanup, err := ff.Workdir(c, nil)
		if err != nil {
			return proto.Fail("%v", err)
		}
		defer cleanup()
		logf := filepath.Join(work, "pass")
		rate := []string{"-c:v", lib, "-preset", "medium", "-b:v", fmt.Sprintf("%dk", videoK), "-maxrate", fmt.Sprintf("%dk", videoK*3/2), "-bufsize", fmt.Sprintf("%dk", videoK*2)}
		null := "/dev/null"
		if runtime.GOOS == "windows" {
			null = "NUL"
		}
		var p1, p2 []string
		if codec == "h265" {
			p1 = append(append(append([]string{}, base...), rate...), "-x265-params", "pass=1:stats="+filepath.ToSlash(logf)+".log:log-level=error", "-an", "-f", "null", null)
			p2 = append(append(append(append(append([]string{}, base...), rate...), "-x265-params", "pass=2:stats="+filepath.ToSlash(logf)+".log:log-level=error"), tag...), audio...)
		} else {
			p1 = append(append(append([]string{}, base...), rate...), "-pass", "1", "-passlogfile", logf, "-an", "-f", "null", null)
			p2 = append(append(append(append([]string{}, base...), rate...), "-pass", "2", "-passlogfile", logf), audio...)
		}
		p2 = append(append(p2, finish...), out)
		c.Notify(true, "video_compress: pass 1 of 2 (analysing)")
		if _, err := ff.Run(ctx, ff.Job{Args: p1, Dir: work, Duration: d, Label: "video_compress pass 1", Ctx: c}); err != nil {
			return proto.Fail("%v", err)
		}
		c.Notify(true, "video_compress: pass 2 of 2 (encoding)")
		if _, err := ff.Run(ctx, ff.Job{Args: p2, Dir: work, Duration: d, Label: "video_compress pass 2", Ctx: c}); err != nil {
			_ = os.Remove(out)
			return proto.Fail("%v", err)
		}
		how = fmt.Sprintf("two-pass, target %.1f MB (%d kb/s video)", r.TargetSizeMB, videoK)
	} else {
		args := append(append([]string{}, base...), "-c:v", lib, "-preset", "medium", "-crf", strconv.Itoa(crf))
		args = append(append(append(args, tag...), audio...), finish...)
		args = append(args, out)
		if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: d, Label: "video_compress", Ctx: c}); err != nil {
			_ = os.Remove(out)
			return proto.Fail("%v", err)
		}
		how = fmt.Sprintf("%s quality, CRF %d", orDefault(r.Quality, "medium"), crf)
	}
	before, after := proto.FileSize(m.Path), proto.FileSize(out)
	msg := fmt.Sprintf("Compressed %s from %s to %s (%.0f%% of the original; %s, %s):\n%s",
		c.Rel(m.Path), proto.Size(before), proto.Size(after), 100*float64(after)/float64(max(before, 1)), codec, how, describe(ctx, c, out))
	if r.TargetSizeMB > 0 && float64(after) > r.TargetSizeMB*1024*1024 {
		msg += fmt.Sprintf("\nNote: the result is slightly over the %.1f MB target; run again with a smaller target_size_mb or a max_height.", r.TargetSizeMB)
	}
	if after >= before {
		msg += "\nNote: the result is not smaller than the original, which was already efficiently compressed; try quality \"low\", a max_height, or codec h265."
	}
	return proto.OK("%s", msg)
}

// --------------------------------------------------------------- video_to_gif

type gifReq struct {
	Input     string     `json:"input"`
	Start     proto.Time `json:"start"`
	End       proto.Time `json:"end"`
	Duration  proto.Time `json:"duration"`
	FPS       float64    `json:"fps"`
	Width     int        `json:"width"`
	Loop      *bool      `json:"loop"`
	Output    string     `json:"output"`
	Overwrite bool       `json:"overwrite"`
}

func init() {
	register("video_to_gif", "Turn a video clip into a high-quality animated GIF or animated WebP.", runGIF)
}

func runGIF(ctx context.Context, c *proto.Ctx, r *gifReq) proto.Result {
	fps := r.FPS
	if fps == 0 {
		fps = 12
	}
	width := r.Width
	if width == 0 {
		width = 480
	}
	if fps < 1 || fps > 50 || width < 32 || width > 1920 {
		return proto.Fail("fps must be 1–50 and width 32–1920")
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasVideo() {
		return proto.Fail("%s has no video stream", r.Input)
	}
	s, e, err := span(r.Start, r.End, r.Duration, m.duration())
	if err != nil {
		return proto.Fail("%v", err)
	}
	if e-s > 60 {
		return proto.Fail("animated images are limited to 60 seconds; set start and duration (the selection is %s)", proto.Clock(e-s))
	}
	ext := "gif"
	if r.Output != "" && filepath.Ext(r.Output) != "" {
		ext = extOf(r.Output, "gif")
	}
	if err := oneOf("output type", ext, "gif", "webp"); err != nil {
		return proto.Fail("%v", err)
	}
	out, err := c.Output(r.Output, m.Path, "anim", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	loop := "0"
	if r.Loop != nil && !*r.Loop {
		loop = "-1"
		if ext == "webp" {
			loop = "1"
		}
	}
	args := []string{"-ss", ff.Sec(s), "-t", ff.Sec(e - s), "-i", m.Path, "-an", "-sn"}
	scale := fmt.Sprintf("fps=%g,scale=%d:-2:flags=lanczos", fps, width)
	if ext == "gif" {
		args = append(args, "-filter_complex", scale+",split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle", "-loop", loop)
	} else {
		args = append(args, "-vf", scale, "-c:v", "libwebp", "-lossless", "0", "-q:v", "70", "-compression_level", "5", "-loop", loop)
	}
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: e - s, Label: "video_to_gif", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Created an animated %s from %s – %s of %s: %s (%s, %d px wide, %g fps)",
		strings.ToUpper(ext), proto.Clock(s), proto.Clock(e), c.Rel(m.Path), c.Rel(out), proto.Size(proto.FileSize(out)), width, fps)
}

// ------------------------------------------------------------ images_to_video

type slideshowReq struct {
	Images            []string `json:"images"`
	Folder            string   `json:"folder"`
	SecondsPerImage   float64  `json:"seconds_per_image"`
	Transition        string   `json:"transition"`
	TransitionSeconds float64  `json:"transition_seconds"`
	Width             int      `json:"width"`
	Height            int      `json:"height"`
	Fit               string   `json:"fit"`
	FPS               int      `json:"fps"`
	Audio             string   `json:"audio"`
	Output            string   `json:"output"`
	Quality           string   `json:"quality"`
	Overwrite         bool     `json:"overwrite"`
}

func init() {
	register("images_to_video", "Make a slideshow video from images, with transitions and optional background music.", runSlideshow)
}

var imageExts = map[string]bool{"jpg": true, "jpeg": true, "png": true, "webp": true, "bmp": true, "tif": true, "tiff": true, "gif": true}

var transitions = []string{"fade", "none", "fadeblack", "fadewhite", "dissolve", "wipeleft", "wiperight", "slideleft", "slideright", "circleopen", "circleclose", "smoothleft", "smoothright", "zoomin"}

func runSlideshow(ctx context.Context, c *proto.Ctx, r *slideshowReq) proto.Result {
	spi := r.SecondsPerImage
	if spi == 0 {
		spi = 3
	}
	if spi < 0.2 || spi > 600 {
		return proto.Fail("seconds_per_image must be between 0.2 and 600")
	}
	tr := orDefault(r.Transition, "fade")
	if err := oneOf("transition", tr, transitions...); err != nil {
		return proto.Fail("%v", err)
	}
	td := r.TransitionSeconds
	if td == 0 {
		td = 0.6
	}
	if tr == "none" {
		td = 0
	}
	if td < 0 || td >= spi {
		return proto.Fail("transition_seconds must be shorter than seconds_per_image")
	}
	fit := orDefault(r.Fit, "blur")
	if err := oneOf("fit", fit, "pad", "crop", "blur"); err != nil {
		return proto.Fail("%v", err)
	}
	w, h, fps := r.Width, r.Height, r.FPS
	if w == 0 && h == 0 {
		w, h = 1920, 1080
	}
	if w == 0 || h == 0 || w > 7680 || h > 7680 || w < 16 || h < 16 {
		return proto.Fail("give both width and height (for example 1920×1080, or 1080×1920 for vertical)")
	}
	w, h = ff.Even(w), ff.Even(h)
	if fps == 0 {
		fps = 30
	}
	if fps < 1 || fps > 60 {
		return proto.Fail("fps must be between 1 and 60")
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}

	var imgs []string
	for i, p := range r.Images {
		a, err := c.Input(fmt.Sprintf("images[%d]", i), p)
		if err != nil {
			return proto.Fail("%v", err)
		}
		imgs = append(imgs, a)
	}
	if r.Folder != "" {
		dir, err := c.Dir("folder", r.Folder, "")
		if err != nil {
			return proto.Fail("%v", err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return proto.Fail("%v", err)
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && imageExts[extOf(e.Name(), "")] {
				names = append(names, e.Name())
			}
		}
		sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
		for _, n := range names {
			imgs = append(imgs, filepath.Join(dir, n))
		}
	}
	if len(imgs) == 0 {
		return proto.Fail("give images (a list of files) or a folder that contains images")
	}
	if len(imgs) > 300 {
		return proto.Fail("at most 300 images (got %d)", len(imgs))
	}
	if len(imgs) == 1 {
		td = 0
	}
	clip := spi + td
	total := float64(len(imgs))*spi + td

	var g strings.Builder
	var args []string
	for i, img := range imgs {
		args = append(args, "-loop", "1", "-framerate", strconv.Itoa(fps), "-t", ff.Sec(clip), "-i", img)
		in := fmt.Sprintf("[%d:v]", i)
		switch fit {
		case "pad":
			fmt.Fprintf(&g, "%sscale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", in, w, h, w, h)
		case "crop":
			fmt.Fprintf(&g, "%sscale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", in, w, h, w, h)
		case "blur":
			fmt.Fprintf(&g, "%ssplit=2[a%d][b%d];[a%d]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,boxblur=luma_radius=min(h\\,w)/20:luma_power=2,eq=brightness=-0.08[bg%d];"+
				"[b%d]scale=%d:%d:force_original_aspect_ratio=decrease[fg%d];[bg%d][fg%d]overlay=(W-w)/2:(H-h)/2",
				in, i, i, i, w, h, w, h, i, i, w, h, i, i, i)
		}
		fmt.Fprintf(&g, ",setsar=1,fps=%d,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS[s%d];\n", fps, ff.Sec(clip), i)
	}
	switch {
	case len(imgs) == 1:
		g.WriteString("[s0]null[vout]")
	case td == 0:
		for i := range imgs {
			fmt.Fprintf(&g, "[s%d]", i)
		}
		fmt.Fprintf(&g, "concat=n=%d:v=1:a=0[vout]", len(imgs))
	default:
		prev := "s0"
		for k := 1; k < len(imgs); k++ {
			next := fmt.Sprintf("x%d", k)
			if k == len(imgs)-1 {
				next = "vout"
			}
			fmt.Fprintf(&g, "[%s][s%d]xfade=transition=%s:duration=%s:offset=%s[%s];\n", prev, k, tr, ff.Sec(td), ff.Sec(float64(k)*spi), next)
			prev = next
		}
	}
	maps := []string{"-map", "[vout]"}
	hasAudio := false
	if r.Audio != "" {
		am, err := openMedia(ctx, c, "audio", r.Audio)
		if err != nil {
			return proto.Fail("%v", err)
		}
		if !am.hasAudio() {
			return proto.Fail("%s has no audio", r.Audio)
		}
		hasAudio = true
		args = append(args, "-stream_loop", "-1", "-i", am.Path)
		fade := 2.0
		if total < 6 {
			fade = total / 3
		}
		s := strings.TrimSuffix(g.String(), ";\n")
		g.Reset()
		g.WriteString(s)
		fmt.Fprintf(&g, ";\n[%d:a:0]atrim=0:%s,asetpts=PTS-STARTPTS,afade=t=out:st=%s:d=%s[aout]", len(imgs), ff.Sec(total), ff.Sec(total-fade), ff.Sec(fade))
		maps = append(maps, "-map", "[aout]")
	}
	ext := "mp4"
	if r.Output != "" && filepath.Ext(r.Output) != "" {
		ext = extOf(r.Output, "mp4")
	}
	if ff.AudioOnlyExt(ext) {
		return proto.Fail("the output must be a video format such as mp4")
	}
	firstBase := imgs[0]
	if r.Folder != "" {
		firstBase = filepath.Join(filepath.Dir(imgs[0]), "slideshow")
	}
	out, err := c.Output(r.Output, firstBase, "slideshow", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	work, cleanup, err := ff.Workdir(c, nil)
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()
	script := filepath.Join(work, "graph.txt")
	if err := os.WriteFile(script, []byte(strings.TrimSuffix(g.String(), ";\n")), 0o644); err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(ext, true, hasAudio, crf, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args = append(args, ff.FilterScriptArgs(script)...)
	args = append(args, maps...)
	args = append(args, enc...)
	args = append(args, "-t", ff.Sec(total), "-r", strconv.Itoa(fps), out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: total, Label: "images_to_video", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Made a %s slideshow of %d images (%g s each, %s transition, %s fit):\n%s",
		proto.Clock(total), len(imgs), spi, tr, fit, describe(ctx, c, out))
}

// naturalLess sorts "img2" before "img10".
func naturalLess(a, b string) bool {
	al, bl := strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(al) && j < len(bl) {
		ca, cb := al[i], bl[j]
		if isDigit(ca) && isDigit(cb) {
			si := i
			for i < len(al) && isDigit(al[i]) {
				i++
			}
			sj := j
			for j < len(bl) && isDigit(bl[j]) {
				j++
			}
			na := strings.TrimLeft(al[si:i], "0")
			nb := strings.TrimLeft(bl[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(al)-i < len(bl)-j
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
