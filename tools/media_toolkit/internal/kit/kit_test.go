// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// fixture builds a workspace with generated media. Tests that need FFmpeg
// are skipped when it is not installed.
type fixture struct {
	t  *testing.T
	ws string
	c  *proto.Ctx
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	bin, err := ff.Find("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ws := t.TempDir()
	out := t.TempDir()
	c, err := proto.NewCtx(ws, out)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, ws: c.Workspace, c: c}
	gen := func(args ...string) {
		cmd := exec.Command(bin, append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...)
		cmd.Dir = ws
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generating fixture: %v\n%s", err, b)
		}
	}
	// 6 s 640x360 test pattern; a tone with a silent gap from 2 s to 4 s.
	gen("-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=6", "-f", "lavfi", "-i", "aevalsrc='0.5*sin(2*PI*440*t)*(lt(t,2)+gte(t,4))':s=48000:d=6",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", "video.mp4")
	// a second clip with another size and no audio
	gen("-f", "lavfi", "-i", "smptebars=s=320x240:r=30:d=3", "-c:v", "libx264", "-pix_fmt", "yuv420p", "bars.mp4")
	gen("-f", "lavfi", "-i", "sine=frequency=220:sample_rate=44100:duration=4", "-c:a", "libmp3lame", "music.mp3")
	gen("-f", "lavfi", "-i", "testsrc=s=128x64:d=1", "-frames:v", "1", "logo.png")
	if err := os.MkdirAll(filepath.Join(ws, "photos"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i, color := range []string{"red", "green", "blue"} {
		gen("-f", "lavfi", "-i", "color=c="+color+":s=800x600:d=1", "-frames:v", "1", filepath.Join("photos", "img"+string(rune('1'+i))+".png"))
	}
	srt := "1\n00:00:00,500 --> 00:00:02,000\nHello from ZTools\n\n2\n00:00:03,000 --> 00:00:05,000\nSecond line\n"
	if err := os.WriteFile(filepath.Join(ws, "subs.srt"), []byte(srt), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// run calls a tool and fails the test on an error result.
func (f *fixture) run(name string, args map[string]any) proto.Result {
	f.t.Helper()
	r := f.try(name, args)
	if r.IsError {
		f.t.Fatalf("%s(%v) failed: %s", name, args, r.Output)
	}
	return r
}

func (f *fixture) try(name string, args map[string]any) proto.Result {
	f.t.Helper()
	tool, ok := Get(name)
	if !ok {
		f.t.Fatalf("no tool %s", name)
	}
	raw, _ := json.Marshal(args)
	return tool.Run(context.Background(), f.c, raw)
}

func (f *fixture) probe(rel string) *ff.Info {
	f.t.Helper()
	info, err := ff.Probe(context.Background(), filepath.Join(f.ws, rel))
	if err != nil {
		f.t.Fatalf("probe %s: %v", rel, err)
	}
	return info
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestFFmpegTools(t *testing.T) {
	f := newFixture(t)

	t.Run("media_probe", func(t *testing.T) {
		r := f.run("media_probe", map[string]any{"input": "video.mp4"})
		for _, want := range []string{"640x360", "h264", "aac", "Duration: 0:06"} {
			if !strings.Contains(r.Output, want) {
				t.Errorf("summary lacks %q:\n%s", want, r.Output)
			}
		}
		r = f.run("media_probe", map[string]any{"input": "video.mp4", "format": "json"})
		if !strings.Contains(r.Output, `"codec_type": "video"`) {
			t.Errorf("json output unexpected")
		}
	})

	t.Run("media_trim", func(t *testing.T) {
		f.run("media_trim", map[string]any{"input": "video.mp4", "start": "1", "end": "0:03.5", "output": "trim.mp4"})
		if d := f.probe("trim.mp4").Duration(); !near(d, 2.5, 0.15) {
			t.Errorf("accurate trim is %.3f s, want 2.5", d)
		}
		f.run("media_trim", map[string]any{"input": "video.mp4", "start": 0, "duration": 2, "mode": "fast", "output": "fast.mp4"})
		f.run("media_trim", map[string]any{"input": "music.mp3", "start": 1, "end": 2})
		if d := f.probe("music_trim.mp3").Duration(); !near(d, 1, 0.1) {
			t.Errorf("audio trim is %.3f s", d)
		}
		// errors are reported, not crashes
		if r := f.try("media_trim", map[string]any{"input": "video.mp4", "start": 10}); !r.IsError {
			t.Error("start past the end should fail")
		}
		if r := f.try("media_trim", map[string]any{"input": "video.mp4", "start": 1, "output": "trim.mp4"}); !r.IsError || !strings.Contains(r.Output, "already exists") {
			t.Errorf("existing output should be refused: %s", r.Output)
		}
		if r := f.try("media_trim", map[string]any{"input": "video.mp4", "strat": 1}); !r.IsError || !strings.Contains(r.Output, "unknown parameter") {
			t.Errorf("misspelled parameter should be reported: %s", r.Output)
		}
	})

	t.Run("media_split", func(t *testing.T) {
		r := f.run("media_split", map[string]any{"input": "video.mp4", "mode": "parts", "parts": 3})
		parts, _ := filepath.Glob(filepath.Join(f.ws, "video_parts", "video_part*.mp4"))
		if len(parts) != 3 {
			t.Fatalf("want 3 parts, got %d\n%s", len(parts), r.Output)
		}
		f.run("media_split", map[string]any{"input": "music.mp3", "mode": "at", "at": []any{1, "2.5"}, "output_dir": "music_parts"})
		if m, _ := filepath.Glob(filepath.Join(f.ws, "music_parts", "*.mp3")); len(m) != 3 {
			t.Errorf("want 3 audio parts, got %d", len(m))
		}
	})

	t.Run("media_merge", func(t *testing.T) {
		r := f.run("media_merge", map[string]any{"inputs": []string{"video.mp4", "bars.mp4", "video.mp4"}, "output": "merged.mp4"})
		if !strings.Contains(r.Output, "re-encoded") {
			t.Errorf("mixed inputs should be re-encoded: %s", r.Output)
		}
		info := f.probe("merged.mp4")
		if d := info.Duration(); !near(d, 15, 0.3) {
			t.Errorf("merged duration %.2f, want 15", d)
		}
		if v := info.Video(); v.Width != 640 || v.Height != 360 {
			t.Errorf("merged size %dx%d", v.Width, v.Height)
		}
		if info.Audio() == nil {
			t.Error("merged file lost its audio")
		}
		r = f.run("media_merge", map[string]any{"inputs": []string{"video.mp4", "video.mp4"}, "output": "double.mp4"})
		if !strings.Contains(r.Output, "without re-encoding") {
			t.Errorf("identical inputs should be joined without re-encoding: %s", r.Output)
		}
	})

	t.Run("video_resize", func(t *testing.T) {
		for _, fit := range []string{"crop", "pad", "blur"} {
			out := "vertical_" + fit + ".mp4"
			f.run("video_resize", map[string]any{"input": "video.mp4", "aspect": "9:16", "fit": fit, "height": 640, "output": out})
			if v := f.probe(out).Video(); v.Width != 360 || v.Height != 640 {
				t.Errorf("%s: got %dx%d, want 360x640", fit, v.Width, v.Height)
			}
		}
		f.run("video_resize", map[string]any{"input": "video.mp4", "width": 320})
		if v := f.probe("video_320x180.mp4").Video(); v.Height != 180 {
			t.Errorf("width-only resize kept aspect wrong: %dx%d", v.Width, v.Height)
		}
	})

	t.Run("video_rotate", func(t *testing.T) {
		f.run("video_rotate", map[string]any{"input": "video.mp4", "rotate": 90, "flip": "horizontal", "output": "rot.mp4"})
		if v := f.probe("rot.mp4").Video(); v.Width != 360 || v.Height != 640 {
			t.Errorf("rotated size %dx%d", v.Width, v.Height)
		}
	})

	t.Run("video_speed", func(t *testing.T) {
		f.run("video_speed", map[string]any{"input": "video.mp4", "speed": 2, "output": "fast2x.mp4"})
		if d := f.probe("fast2x.mp4").Duration(); !near(d, 3, 0.2) {
			t.Errorf("2x speed duration %.2f, want 3", d)
		}
		f.run("video_speed", map[string]any{"input": "video.mp4", "speed": 0.5, "reverse": true, "output": "slowrev.mp4"})
		if d := f.probe("slowrev.mp4").Duration(); !near(d, 12, 0.3) {
			t.Errorf("0.5x speed duration %.2f, want 12", d)
		}
	})

	t.Run("video_watermark", func(t *testing.T) {
		f.run("video_watermark", map[string]any{"input": "video.mp4", "image": "logo.png", "position": "top_left", "output": "wm_logo.mp4"})
		if ff.FindFont() != "" {
			f.run("video_watermark", map[string]any{"input": "video.mp4", "image": "logo.png", "text": "ZTools: it's \"great\" 100%", "start": 1, "end": 4, "output": "wm_both.mp4"})
		}
		if d := f.probe("wm_logo.mp4").Duration(); !near(d, 6, 0.2) {
			t.Errorf("watermarked duration %.2f", d)
		}
	})

	t.Run("video_subtitles", func(t *testing.T) {
		f.run("video_subtitles", map[string]any{"input": "video.mp4", "subtitles": "subs.srt", "mode": "soft", "language": "eng", "output": "soft.mp4"})
		info := f.probe("soft.mp4")
		hasSub := false
		for _, s := range info.Streams {
			hasSub = hasSub || (s.CodecType == "subtitle" && s.CodecName == "mov_text")
		}
		if !hasSub {
			t.Error("soft subtitles missing")
		}
		r := f.try("video_subtitles", map[string]any{"input": "video.mp4", "subtitles": "subs.srt", "output": "burned.mp4"})
		if r.IsError && !strings.Contains(r.Output, "libass") {
			t.Errorf("burn failed: %s", r.Output)
		}
	})

	t.Run("media_convert", func(t *testing.T) {
		f.run("media_convert", map[string]any{"input": "video.mp4", "format": "webm", "max_height": 180})
		info := f.probe("video_converted.webm")
		if v := info.Video(); v.CodecName != "vp9" || v.Height != 180 {
			t.Errorf("webm: %s %dx%d", v.CodecName, v.Width, v.Height)
		}
		f.run("media_convert", map[string]any{"input": "video.mp4", "output": "audio.flac"})
		if a := f.probe("audio.flac"); a.Video() != nil || a.Audio().CodecName != "flac" {
			t.Error("flac conversion wrong")
		}
		if ff.HasEncoder("libx265") {
			f.run("media_convert", map[string]any{"input": "video.mp4", "output": "h265.mp4", "video_codec": "h265", "audio_codec": "copy"})
			if v := f.probe("h265.mp4").Video(); v.CodecName != "hevc" {
				t.Errorf("h265 codec is %s", v.CodecName)
			}
		}
	})

	t.Run("video_compress", func(t *testing.T) {
		r := f.run("video_compress", map[string]any{"input": "video.mp4", "target_size_mb": 0.25, "output": "small.mp4"})
		size := proto.FileSize(filepath.Join(f.ws, "small.mp4"))
		if float64(size) > 0.30*1024*1024 {
			t.Errorf("target 0.25 MB, got %s\n%s", proto.Size(size), r.Output)
		}
		f.run("video_compress", map[string]any{"input": "video.mp4", "quality": "low", "max_height": 240})
		if v := f.probe("video_compressed.mp4").Video(); v.Height != 240 {
			t.Errorf("max_height ignored: %d", v.Height)
		}
		if r := f.try("video_compress", map[string]any{"input": "video.mp4", "target_size_mb": 0.001}); !r.IsError {
			t.Error("an impossible target should be refused")
		}
	})

	t.Run("video_to_gif", func(t *testing.T) {
		f.run("video_to_gif", map[string]any{"input": "video.mp4", "start": 1, "duration": 2, "width": 200})
		info := f.probe("video_anim.gif")
		if v := info.Video(); v.CodecName != "gif" || v.Width != 200 {
			t.Errorf("gif: %s %d", v.CodecName, v.Width)
		}
		if ff.HasEncoder("libwebp") {
			f.run("video_to_gif", map[string]any{"input": "video.mp4", "duration": 1, "output": "anim.webp"})
		}
	})

	t.Run("video_frames", func(t *testing.T) {
		f.c.Vision = true
		defer func() { f.c.Vision = false }()
		r := f.run("video_frames", map[string]any{"input": "video.mp4", "count": 3, "width": 160})
		if len(r.Media) != 3 {
			t.Errorf("want 3 attached frames, got %d", len(r.Media))
		}
		frames, _ := filepath.Glob(filepath.Join(f.ws, "video_frames", "*.jpg"))
		if len(frames) != 3 {
			t.Errorf("want 3 frame files, got %d", len(frames))
		}
		r = f.run("video_frames", map[string]any{"input": "video.mp4", "mode": "contact_sheet", "count": 6, "columns": 3})
		if len(r.Media) != 1 {
			t.Fatalf("contact sheet not attached: %s", r.Output)
		}
		sheet := f.probe(f.c.Rel(r.Media[0].Path)).Video()
		if sheet.Width < 900 || sheet.Height < 300 {
			t.Errorf("contact sheet is %dx%d", sheet.Width, sheet.Height)
		}
		f.run("video_frames", map[string]any{"input": "video.mp4", "mode": "timestamps", "timestamps": []any{"0:01", 2.5}, "image_format": "png", "output_dir": "shots"})
		if p, _ := filepath.Glob(filepath.Join(f.ws, "shots", "*.png")); len(p) != 2 {
			t.Errorf("want 2 png shots, got %d", len(p))
		}
	})

	t.Run("scene_detect", func(t *testing.T) {
		r := f.run("media_merge", map[string]any{"inputs": []string{"video.mp4", "bars.mp4"}, "output": "two_scenes.mp4"})
		_ = r
		r = f.run("scene_detect", map[string]any{"input": "two_scenes.mp4", "thumbnails": true})
		if !strings.Contains(r.Output, "2 scenes") {
			t.Errorf("expected a cut at 6 s:\n%s", r.Output)
		}
	})

	t.Run("images_to_video", func(t *testing.T) {
		f.run("images_to_video", map[string]any{"folder": "photos", "seconds_per_image": 1.5, "width": 640, "height": 360, "audio": "music.mp3", "fps": 25})
		info := f.probe("photos/slideshow_slideshow.mp4")
		if d := info.Duration(); !near(d, 3*1.5+0.6, 0.2) {
			t.Errorf("slideshow duration %.2f, want 5.1", d)
		}
		if info.Audio() == nil {
			t.Error("slideshow has no audio")
		}
		f.run("images_to_video", map[string]any{"images": []string{"photos/img1.png", "photos/img2.png"}, "transition": "none", "fit": "crop", "width": 320, "height": 320, "output": "square.mp4"})
		if v := f.probe("square.mp4").Video(); v.Width != 320 || v.Height != 320 {
			t.Errorf("square slideshow %dx%d", v.Width, v.Height)
		}
	})

	t.Run("audio_extract", func(t *testing.T) {
		f.run("audio_extract", map[string]any{"input": "video.mp4", "format": "wav", "start": 1, "end": 3})
		info := f.probe("video_audio.wav")
		if info.Video() != nil || !near(info.Duration(), 2, 0.1) {
			t.Errorf("extract: %.2f s", info.Duration())
		}
		if r := f.try("audio_extract", map[string]any{"input": "bars.mp4"}); !r.IsError {
			t.Error("a silent video should be refused")
		}
	})

	t.Run("audio_adjust", func(t *testing.T) {
		r := f.run("audio_adjust", map[string]any{"input": "video.mp4", "normalize": true, "target_lufs": -16, "fade_in": 0.5, "fade_out": "1", "highpass": 80, "denoise": "light"})
		if !strings.Contains(r.Output, "LUFS") {
			t.Errorf("normalisation not reported: %s", r.Output)
		}
		info := f.probe("video_adjusted.mp4")
		if info.Video() == nil || info.Video().CodecName != "h264" {
			t.Error("video should be copied unchanged")
		}
		f.run("audio_adjust", map[string]any{"input": "music.mp3", "volume": "-6dB", "mono": true})
		if a := f.probe("music_adjusted.mp3").Audio(); a.Channels != 1 {
			t.Errorf("mono: %d channels", a.Channels)
		}
	})

	t.Run("audio_mix", func(t *testing.T) {
		f.run("audio_mix", map[string]any{"input": "video.mp4", "audio": "music.mp3", "duck": true, "output": "mixed.mp4"})
		info := f.probe("mixed.mp4")
		if !near(info.Duration(), 6, 0.2) || info.Audio() == nil {
			t.Errorf("mix: %.2f s", info.Duration())
		}
		f.run("audio_mix", map[string]any{"input": "bars.mp4", "audio": "music.mp3", "mode": "replace", "start_at": 0.5})
		if info := f.probe("bars_mixed.mp4"); info.Audio() == nil || !near(info.Duration(), 3, 0.2) {
			t.Error("replace on a silent video failed")
		}
	})

	t.Run("silence_cut", func(t *testing.T) {
		r := f.run("silence_cut", map[string]any{"input": "video.mp4"})
		if !strings.Contains(r.Output, "1 silent stretch") {
			t.Errorf("expected one silence 2–4 s:\n%s", r.Output)
		}
		f.run("silence_cut", map[string]any{"input": "video.mp4", "action": "remove", "padding": 0.1})
		info := f.probe("video_nosilence.mp4")
		if d := info.Duration(); !near(d, 4.2, 0.35) {
			t.Errorf("after removing ~1.8 s of silence: %.2f s", d)
		}
	})

	t.Run("audio_waveform", func(t *testing.T) {
		f.c.Vision = true
		defer func() { f.c.Vision = false }()
		r := f.run("audio_waveform", map[string]any{"input": "music.mp3", "width": 800, "height": 200})
		if len(r.Media) != 1 {
			t.Error("waveform not attached")
		}
		f.run("audio_waveform", map[string]any{"input": "music.mp3", "kind": "spectrogram"})
		f.run("audio_waveform", map[string]any{"input": "music.mp3", "kind": "audiogram", "background_image": "photos/img3.png", "width": 480, "height": 480})
		if info := f.probe("music_audiogram.mp4"); info.Video() == nil || info.Audio() == nil {
			t.Error("audiogram needs video and audio")
		}
	})
}

// Paths outside the workspace are refused for inputs and outputs.
func TestWorkspaceConfinement(t *testing.T) {
	f := newFixture(t)
	outside := filepath.Join(t.TempDir(), "secret.mp4")
	_ = os.WriteFile(outside, []byte("x"), 0o644)
	for _, args := range []map[string]any{
		{"input": outside},
		{"input": "../video.mp4"},
		{"input": "video.mp4", "output": filepath.Join(filepath.Dir(outside), "out.mp4")},
		{"input": "video.mp4", "output": "../escape.mp4"},
	} {
		if r := f.try("media_trim", mergeMap(args, map[string]any{"start": 1})); !r.IsError || !strings.Contains(r.Output, "outside the workspace") && !strings.Contains(r.Output, "not found") {
			t.Errorf("%v should be refused, got: %s", args, r.Output)
		}
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(f.ws, "link")); err == nil {
		if r := f.try("media_trim", map[string]any{"input": "video.mp4", "start": 1, "output": "link/out.mp4"}); !r.IsError {
			t.Error("writing through a symlink out of the workspace should be refused")
		}
	}
}

func mergeMap(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func TestHelpers(t *testing.T) {
	if got := strings.Join(atempo(4), ","); got != "atempo=2.0,atempo=2.000000" {
		t.Errorf("atempo(4) = %s", got)
	}
	if got := strings.Join(atempo(0.3), ","); got != "atempo=0.5,atempo=0.600000" {
		t.Errorf("atempo(0.3) = %s", got)
	}
	for in, want := range map[string]string{"+6dB": "6dB", "-3 dB": "-3dB", "150%": "1.5", "0.5x": "0.5", "2": "2"} {
		if got, err := parseVolume(in); err != nil || got != want {
			t.Errorf("parseVolume(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := parseVolume("loud"); err == nil {
		t.Error("parseVolume should reject words")
	}
	if a, _ := parseAspect("9:16"); !near(a, 0.5625, 1e-9) {
		t.Error("aspect 9:16")
	}
	names := []string{"img10.png", "img2.png", "IMG1.png", "img2b.png"}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if naturalLess(names[j], names[i]) {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	if strings.Join(names, " ") != "IMG1.png img2.png img2b.png img10.png" {
		t.Errorf("natural order: %v", names)
	}
}
