// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

// -------------------------------------------------------------- audio_extract

type extractReq struct {
	Input     string     `json:"input"`
	Format    string     `json:"format"`
	Bitrate   string     `json:"bitrate"`
	Start     proto.Time `json:"start"`
	End       proto.Time `json:"end"`
	Output    string     `json:"output"`
	Overwrite bool       `json:"overwrite"`
}

func init() {
	register("audio_extract", "Extract the audio track of a video as MP3, M4A, WAV, FLAC or Opus.", runExtract)
}

func runExtract(ctx context.Context, c *proto.Ctx, r *extractReq) proto.Result {
	format := strings.ToLower(orDefault(r.Format, "mp3"))
	if r.Output != "" && filepath.Ext(r.Output) != "" {
		format = extOf(r.Output, format)
	}
	if err := oneOf("format", format, "mp3", "m4a", "wav", "flac", "opus", "ogg"); err != nil {
		return proto.Fail("%v", err)
	}
	if err := checkBitrate(r.Bitrate); err != nil {
		return proto.Fail("%v", err)
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasAudio() {
		return proto.Fail("%s has no audio track", r.Input)
	}
	s, e, err := span(r.Start, r.End, proto.Time{}, m.duration())
	if err != nil {
		return proto.Fail("%v", err)
	}
	out, err := c.Output(r.Output, m.Path, "audio", format, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(format, false, true, 0, r.Bitrate)
	if err != nil {
		return proto.Fail("%v", err)
	}
	args := []string{}
	if s > 0 {
		args = append(args, "-ss", ff.Sec(s))
	}
	args = append(args, "-i", m.Path, "-t", ff.Sec(e-s), "-map", "0:a:0", "-map_metadata", "0")
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: e - s, Label: "audio_extract", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Extracted the audio of %s:\n%s", c.Rel(m.Path), describe(ctx, c, out))
}

// --------------------------------------------------------------- audio_adjust

type adjustReq struct {
	Input      string     `json:"input"`
	Volume     string     `json:"volume"`
	Normalize  bool       `json:"normalize"`
	TargetLUFS float64    `json:"target_lufs"`
	FadeIn     proto.Time `json:"fade_in"`
	FadeOut    proto.Time `json:"fade_out"`
	Denoise    string     `json:"denoise"`
	Highpass   int        `json:"highpass"`
	Mono       bool       `json:"mono"`
	Output     string     `json:"output"`
	Overwrite  bool       `json:"overwrite"`
}

func init() {
	register("audio_adjust", "Fix the audio of a video or audio file: loudness normalisation, volume, fades, noise reduction, rumble filter.", runAdjust)
}

var volRe = regexp.MustCompile(`^([+-]?[0-9]*\.?[0-9]+)\s*(dB|db|%|x)?$`)

// parseVolume turns "+6dB", "-3 dB", "150%", "1.5" or "0.5x" into an FFmpeg volume value.
func parseVolume(s string) (string, error) {
	m := volRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", fmt.Errorf("volume must look like +6dB, -3dB, 150%% or 1.5, got %q", s)
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	switch strings.ToLower(m[2]) {
	case "db":
		if v < -60 || v > 40 {
			return "", fmt.Errorf("volume in dB must be between -60 and +40")
		}
		return fmt.Sprintf("%gdB", v), nil
	case "%":
		v /= 100
	}
	if v < 0 || v > 100 {
		return "", fmt.Errorf("volume multiplier must be between 0 and 100")
	}
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}

type loudness struct {
	InputI      string `json:"input_i"`
	InputTP     string `json:"input_tp"`
	InputLRA    string `json:"input_lra"`
	InputThresh string `json:"input_thresh"`
	Offset      string `json:"target_offset"`
}

func measureLoudness(ctx context.Context, path, pre string, target float64) (*loudness, error) {
	af := fmt.Sprintf("loudnorm=I=%g:TP=-1.5:LRA=11:print_format=json", target)
	if pre != "" {
		af = pre + "," + af
	}
	log, err := ff.RunLog(ctx, "info", "-i", path, "-map", "0:a:0", "-af", af, "-f", "null", "-")
	if err != nil {
		return nil, err
	}
	i := strings.LastIndex(log, "{")
	j := strings.LastIndex(log, "}")
	if i < 0 || j < i {
		return nil, fmt.Errorf("could not measure loudness")
	}
	var l loudness
	if err := json.Unmarshal([]byte(log[i:j+1]), &l); err != nil {
		return nil, fmt.Errorf("could not read loudness measurement: %v", err)
	}
	if l.InputI == "-inf" {
		return nil, fmt.Errorf("the audio is silent, so it cannot be normalised")
	}
	return &l, nil
}

func runAdjust(ctx context.Context, c *proto.Ctx, r *adjustReq) proto.Result {
	denoise := orDefault(r.Denoise, "none")
	if err := oneOf("denoise", denoise, "none", "light", "medium", "strong"); err != nil {
		return proto.Fail("%v", err)
	}
	target := r.TargetLUFS
	if target == 0 {
		target = -16
	}
	if target < -40 || target > -5 {
		return proto.Fail("target_lufs must be between -40 and -5 (-14 suits YouTube/Spotify, -16 podcasts, -23 broadcast)")
	}
	if r.Highpass < 0 || r.Highpass > 1000 {
		return proto.Fail("highpass must be between 20 and 1000 Hz")
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasAudio() {
		return proto.Fail("%s has no audio track", r.Input)
	}
	d := m.duration()
	var pre []string
	var did []string
	if r.Highpass > 0 {
		pre = append(pre, fmt.Sprintf("highpass=f=%d", r.Highpass))
		did = append(did, fmt.Sprintf("high-pass %d Hz", r.Highpass))
	}
	if denoise != "none" {
		nr := map[string]int{"light": 8, "medium": 14, "strong": 22}[denoise]
		pre = append(pre, fmt.Sprintf("afftdn=nr=%d:nf=-40:tn=1", nr))
		did = append(did, denoise+" noise reduction")
	}
	if r.Mono {
		pre = append(pre, "pan=mono|c0=0.5*c0+0.5*c1")
		did = append(did, "mono")
	}
	if r.Volume != "" {
		v, err := parseVolume(r.Volume)
		if err != nil {
			return proto.Fail("%v", err)
		}
		pre = append(pre, "volume="+v)
		did = append(did, "volume "+r.Volume)
	}
	chain := append([]string{}, pre...)
	if r.Normalize {
		c.Notify(true, "audio_adjust: measuring loudness")
		l, err := measureLoudness(ctx, m.Path, strings.Join(pre, ","), target)
		if err != nil {
			return proto.Fail("%v", err)
		}
		sr := 48000
		if a := m.Info.Audio(); a != nil {
			if v, _ := strconv.Atoi(a.SampleRate); v > 0 {
				sr = v
			}
		}
		chain = append(chain, fmt.Sprintf("loudnorm=I=%g:TP=-1.5:LRA=11:measured_I=%s:measured_TP=%s:measured_LRA=%s:measured_thresh=%s:offset=%s:linear=true",
			target, l.InputI, l.InputTP, l.InputLRA, l.InputThresh, l.Offset), fmt.Sprintf("aresample=%d", sr))
		did = append(did, fmt.Sprintf("loudness %s → %g LUFS", l.InputI, target))
	}
	if r.FadeIn.Set && r.FadeIn.Seconds > 0 {
		chain = append(chain, fmt.Sprintf("afade=t=in:st=0:d=%s", ff.Sec(r.FadeIn.Seconds)))
		did = append(did, fmt.Sprintf("fade in %gs", r.FadeIn.Seconds))
	}
	if r.FadeOut.Set && r.FadeOut.Seconds > 0 {
		if r.FadeOut.Seconds >= d {
			return proto.Fail("fade_out is longer than the media")
		}
		chain = append(chain, fmt.Sprintf("afade=t=out:st=%s:d=%s", ff.Sec(d-r.FadeOut.Seconds), ff.Sec(r.FadeOut.Seconds)))
		did = append(did, fmt.Sprintf("fade out %gs", r.FadeOut.Seconds))
	}
	if len(chain) == 0 {
		return proto.Fail("nothing to do: set normalize, volume, fade_in, fade_out, denoise, highpass or mono")
	}
	ext := sameExt(r.Output, m.Path)
	out, err := c.Output(r.Output, m.Path, "adjusted", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	video := m.hasVideo() && !ff.AudioOnlyExt(ext)
	enc, err := ff.Encoders(ext, false, true, 0, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args := []string{"-i", m.Path}
	if video {
		args = append(args, "-map", "0:v:0", "-c:v", "copy")
		enc = removeFlag(enc, "-vn")
	}
	args = append(args, "-map", "0:a:0", "-af", strings.Join(chain, ","))
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: d, Label: "audio_adjust", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Adjusted the audio of %s (%s):\n%s", c.Rel(m.Path), strings.Join(did, ", "), describe(ctx, c, out))
}

// ------------------------------------------------------------------ audio_mix

type mixReq struct {
	Input       string     `json:"input"`
	Audio       string     `json:"audio"`
	Mode        string     `json:"mode"`
	AudioVolume float64    `json:"audio_volume"`
	InputVolume float64    `json:"input_volume"`
	StartAt     proto.Time `json:"start_at"`
	Loop        *bool      `json:"loop"`
	Duck        bool       `json:"duck"`
	FadeOut     proto.Time `json:"fade_out"`
	Output      string     `json:"output"`
	Overwrite   bool       `json:"overwrite"`
}

func init() {
	register("audio_mix", "Add background music or a voice-over to a video (with optional ducking), or replace its soundtrack.", runMix)
}

func runMix(ctx context.Context, c *proto.Ctx, r *mixReq) proto.Result {
	mode := orDefault(r.Mode, "mix")
	if err := oneOf("mode", mode, "mix", "replace"); err != nil {
		return proto.Fail("%v", err)
	}
	av := r.AudioVolume
	if av == 0 {
		av = 1
		if mode == "mix" {
			av = 0.3
		}
	}
	iv := r.InputVolume
	if iv == 0 {
		iv = 1
	}
	if av < 0 || av > 10 || iv < 0 || iv > 10 {
		return proto.Fail("audio_volume and input_volume are multipliers between 0 and 10 (0.3 = background level)")
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	am, err := openMedia(ctx, c, "audio", r.Audio)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !am.hasAudio() {
		return proto.Fail("%s has no audio", r.Audio)
	}
	d := m.duration()
	if d <= 0 {
		return proto.Fail("the duration of %s is unknown", r.Input)
	}
	if !m.hasAudio() {
		mode = "replace" // nothing to mix with
	}
	loop := r.Loop == nil || *r.Loop
	start := 0.0
	if r.StartAt.Set {
		start = r.StartAt.Seconds
		if start >= d {
			return proto.Fail("start_at is past the end of %s", r.Input)
		}
	}
	fade := 0.0
	if r.FadeOut.Set {
		fade = r.FadeOut.Seconds
	} else if loop || am.duration() > d-start {
		fade = 2 // the added track is cut at the end: let it fade instead of stopping abruptly
	}
	if fade > d-start {
		fade = (d - start) / 2
	}

	args := []string{"-i", m.Path}
	if loop {
		args = append(args, "-stream_loop", "-1")
	}
	args = append(args, "-i", am.Path)
	added := fmt.Sprintf("[1:a:0]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo,volume=%.3f", av)
	if start > 0 {
		ms := int(start * 1000)
		added += fmt.Sprintf(",adelay=%d|%d", ms, ms)
	}
	added += fmt.Sprintf(",atrim=0:%s,asetpts=PTS-STARTPTS", ff.Sec(d))
	if fade > 0 {
		added += fmt.Sprintf(",afade=t=out:st=%s:d=%s", ff.Sec(d-fade), ff.Sec(fade))
	}
	var g string
	if mode == "replace" {
		g = added + ",apad[aout]"
	} else {
		norm := ""
		if ff.Major() >= 5 {
			norm = ":normalize=0"
		}
		orig := fmt.Sprintf("[0:a:0]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo,volume=%.3f", iv)
		if r.Duck {
			g = added + "[music];" + orig + ",asplit=2[o1][o2];[music][o1]sidechaincompress=threshold=0.02:ratio=10:attack=15:release=600:makeup=1[ducked];" +
				"[o2][ducked]amix=inputs=2:duration=first:dropout_transition=0" + norm + "[aout]"
		} else {
			g = added + "[music];" + orig + "[o];[o][music]amix=inputs=2:duration=first:dropout_transition=0" + norm + "[aout]"
		}
	}
	ext := sameExt(r.Output, m.Path)
	video := m.hasVideo() && !ff.AudioOnlyExt(ext)
	out, err := c.Output(r.Output, m.Path, "mixed", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(ext, false, true, 0, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args = append(args, "-filter_complex", g)
	if video {
		args = append(args, "-map", "0:v:0", "-c:v", "copy")
		enc = removeFlag(enc, "-vn")
	}
	args = append(args, "-map", "[aout]", "-t", ff.Sec(d))
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: d, Label: "audio_mix", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	how := fmt.Sprintf("mixed %s under the original audio at %.0f%% volume", c.Rel(am.Path), av*100)
	if mode == "replace" {
		how = "replaced the soundtrack with " + c.Rel(am.Path)
	}
	if r.Duck && mode == "mix" {
		how += ", ducking it while the original audio is loud"
	}
	return proto.OK("Audio %s:\n%s", how, describe(ctx, c, out))
}

func removeFlag(args []string, flag string) []string {
	var out []string
	for _, a := range args {
		if a != flag {
			out = append(out, a)
		}
	}
	return out
}

// ---------------------------------------------------------------- silence_cut

type silenceReq struct {
	Input       string     `json:"input"`
	Action      string     `json:"action"`
	ThresholdDB float64    `json:"threshold_db"`
	MinSilence  proto.Time `json:"min_silence"`
	Padding     proto.Time `json:"padding"`
	Output      string     `json:"output"`
	Quality     string     `json:"quality"`
	Overwrite   bool       `json:"overwrite"`
}

func init() {
	register("silence_cut", "Find silent parts of a recording, or cut them out to make tight jump-cut edits of talks and podcasts.", runSilence)
}

var (
	silStartRe = regexp.MustCompile(`silence_start:\s*(-?[0-9.]+)`)
	silEndRe   = regexp.MustCompile(`silence_end:\s*([0-9.]+)`)
)

type interval struct{ a, b float64 }

func detectSilence(ctx context.Context, path string, db, min float64, length float64) ([]interval, error) {
	log, err := ff.RunLog(ctx, "info", "-i", path, "-map", "0:a:0", "-af", fmt.Sprintf("silencedetect=n=%gdB:d=%g", db, min), "-f", "null", "-")
	if err != nil {
		return nil, err
	}
	var out []interval
	open := -1.0
	for _, line := range strings.Split(log, "\n") {
		if m := silStartRe.FindStringSubmatch(line); m != nil {
			open, _ = strconv.ParseFloat(m[1], 64)
			if open < 0 {
				open = 0
			}
		}
		if m := silEndRe.FindStringSubmatch(line); m != nil && open >= 0 {
			e, _ := strconv.ParseFloat(m[1], 64)
			out = append(out, interval{open, e})
			open = -1
		}
	}
	if open >= 0 {
		out = append(out, interval{open, length})
	}
	return out, nil
}

func runSilence(ctx context.Context, c *proto.Ctx, r *silenceReq) proto.Result {
	action := orDefault(r.Action, "detect")
	if err := oneOf("action", action, "detect", "remove"); err != nil {
		return proto.Fail("%v", err)
	}
	db := r.ThresholdDB
	if db == 0 {
		db = -35
	}
	if db < -90 || db > -5 {
		return proto.Fail("threshold_db must be between -90 and -5 (sound quieter than this counts as silence)")
	}
	minSil := 0.6
	if r.MinSilence.Set {
		minSil = r.MinSilence.Seconds
	}
	pad := 0.15
	if r.Padding.Set {
		pad = r.Padding.Seconds
	}
	if minSil < 0.1 || pad < 0 || pad*2 >= minSil {
		return proto.Fail("min_silence must be at least 0.1 s and more than twice the padding")
	}
	crf, err := ff.Quality(r.Quality)
	if err != nil {
		return proto.Fail("%v", err)
	}
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasAudio() {
		return proto.Fail("%s has no audio track", r.Input)
	}
	d := m.duration()
	c.Notify(true, "silence_cut: analysing audio")
	sil, err := detectSilence(ctx, m.Path, db, minSil, d)
	if err != nil {
		return proto.Fail("%v", err)
	}
	silent := 0.0
	for _, s := range sil {
		silent += s.b - s.a
	}
	if action == "detect" {
		var b strings.Builder
		fmt.Fprintf(&b, "%d silent stretches (quieter than %g dB for at least %g s) in %s, %.1f s of %.1f s total (%.0f%%):\n", len(sil), db, minSil, c.Rel(m.Path), silent, d, 100*silent/maxf(d, 0.001))
		for i, s := range sil {
			if i == 300 {
				fmt.Fprintf(&b, "  … %d more\n", len(sil)-300)
				break
			}
			fmt.Fprintf(&b, "  %s – %s (%.2f s)\n", proto.Clock(s.a), proto.Clock(s.b), s.b-s.a)
		}
		return proto.OK("%s", strings.TrimRight(b.String(), "\n"))
	}
	if len(sil) == 0 {
		return proto.OK("No silences longer than %g s (below %g dB) were found in %s; nothing was cut.", minSil, db, c.Rel(m.Path))
	}
	// Keep everything that is not silence, with some padding around speech.
	var keep []interval
	cur := 0.0
	for _, s := range sil {
		a, b := s.a+pad, s.b-pad
		if s.a <= 0.001 {
			a = 0
		}
		if s.b >= d-0.001 {
			b = d
		}
		if a > cur {
			keep = append(keep, interval{cur, a})
		}
		cur = b
	}
	if cur < d {
		keep = append(keep, interval{cur, d})
	}
	if len(keep) == 0 {
		return proto.Fail("the whole recording is silent at %g dB; try a lower threshold_db", db)
	}
	var expr strings.Builder
	kept := 0.0
	for i, k := range keep {
		if i > 0 {
			expr.WriteString("+")
		}
		fmt.Fprintf(&expr, "between(t,%s,%s)", ff.Sec(k.a), ff.Sec(k.b))
		kept += k.b - k.a
	}
	ext := sameExt(r.Output, m.Path)
	video := m.hasVideo() && !ff.AudioOnlyExt(ext)
	var g strings.Builder
	if video {
		fmt.Fprintf(&g, "[0:v:0]select='%s',setpts=N/FRAME_RATE/TB[v];\n", expr.String())
	}
	fmt.Fprintf(&g, "[0:a:0]aselect='%s',asetpts=N/SR/TB[a]", expr.String())
	work, cleanup, err := ff.Workdir(c, nil)
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()
	script := filepath.Join(work, "graph.txt")
	if err := os.WriteFile(script, []byte(g.String()), 0o644); err != nil {
		return proto.Fail("%v", err)
	}
	out, err := c.Output(r.Output, m.Path, "nosilence", ext, r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	enc, err := ff.Encoders(ext, video, true, crf, "")
	if err != nil {
		return proto.Fail("%v", err)
	}
	args := []string{"-i", m.Path}
	args = append(args, ff.FilterScriptArgs(script)...)
	if video {
		args = append(args, "-map", "[v]")
	}
	args = append(args, "-map", "[a]")
	args = append(args, enc...)
	args = append(args, out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: kept, Label: "silence_cut", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Removed %d silent stretches from %s: %s → %s (%.1f s shorter):\n%s",
		len(sil), c.Rel(m.Path), proto.Clock(d), proto.Clock(kept), d-kept, describe(ctx, c, out))
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// ------------------------------------------------------------- audio_waveform

type waveformReq struct {
	Input           string `json:"input"`
	Kind            string `json:"kind"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Color           string `json:"color"`
	BackgroundImage string `json:"background_image"`
	Output          string `json:"output"`
	Overwrite       bool   `json:"overwrite"`
}

func init() {
	register("audio_waveform", "Draw an audio file as a waveform or spectrogram image, or render an audiogram video for social media.", runWaveform)
}

func runWaveform(ctx context.Context, c *proto.Ctx, r *waveformReq) proto.Result {
	kind := orDefault(r.Kind, "waveform")
	if err := oneOf("kind", kind, "waveform", "spectrogram", "audiogram"); err != nil {
		return proto.Fail("%v", err)
	}
	color := orDefault(r.Color, "0x4F8EF7")
	if !colorRe.MatchString(color) {
		return proto.Fail("color must be a name (white) or hex (#4F8EF7)")
	}
	color = strings.Replace(color, "#", "0x", 1)
	w, h := r.Width, r.Height
	if w == 0 {
		w = map[string]int{"waveform": 1600, "spectrogram": 1600, "audiogram": 1080}[kind]
	}
	if h == 0 {
		h = map[string]int{"waveform": 400, "spectrogram": 600, "audiogram": 1080}[kind]
	}
	if w < 64 || h < 64 || w > 7680 || h > 4320 {
		return proto.Fail("width and height must be between 64 and 7680")
	}
	w, h = ff.Even(w), ff.Even(h)
	m, err := openMedia(ctx, c, "input", r.Input)
	if err != nil {
		return proto.Fail("%v", err)
	}
	if !m.hasAudio() {
		return proto.Fail("%s has no audio", r.Input)
	}
	d := m.duration()
	if kind != "audiogram" {
		out, err := c.Output(r.Output, m.Path, kind, "png", r.Overwrite)
		if err != nil {
			return proto.Fail("%v", err)
		}
		g := fmt.Sprintf("[0:a:0]aformat=channel_layouts=mono,showwavespic=s=%dx%d:colors=%s:filter=peak", w, h, color)
		if kind == "spectrogram" {
			g = fmt.Sprintf("[0:a:0]showspectrumpic=s=%dx%d:legend=1:color=intensity", w, h)
		}
		if _, err := ff.Run(ctx, ff.Job{Args: []string{"-i", m.Path, "-filter_complex", g, "-frames:v", "1", "-update", "1", out}}); err != nil {
			_ = os.Remove(out)
			return proto.Fail("%v", err)
		}
		res := proto.OK("Drew a %s of %s (%s): %s", kind, c.Rel(m.Path), proto.Clock(d), c.Rel(out))
		c.Attach(&res, []string{out}, 1)
		return res
	}
	if d > 3600 {
		return proto.Fail("audiograms are limited to one hour of audio; trim it first")
	}
	out, err := c.Output(r.Output, m.Path, "audiogram", "mp4", r.Overwrite)
	if err != nil {
		return proto.Fail("%v", err)
	}
	args := []string{"-i", m.Path}
	bg := fmt.Sprintf("color=c=0x111318:s=%dx%d:r=30[bg]", w, h)
	if r.BackgroundImage != "" {
		img, err := c.Input("background_image", r.BackgroundImage)
		if err != nil {
			return proto.Fail("%v", err)
		}
		args = append(args, "-loop", "1", "-framerate", "30", "-i", img)
		bg = fmt.Sprintf("[1:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,fps=30[bg]", w, h, w, h)
	}
	wh := ff.Even(h / 3)
	g := bg + fmt.Sprintf(";[0:a:0]showwaves=s=%dx%d:mode=cline:rate=30:colors=%s,format=yuva420p[wave];[bg][wave]overlay=(W-w)/2:H-h-%d:shortest=1,format=yuv420p[v]", ff.Even(w*9/10), wh, color, h/12)
	args = append(args, "-filter_complex", g, "-map", "[v]", "-map", "0:a:0", "-t", ff.Sec(d),
		"-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", out)
	if _, err := ff.Run(ctx, ff.Job{Args: args, Duration: d, Label: "audio_waveform", Ctx: c}); err != nil {
		_ = os.Remove(out)
		return proto.Fail("%v", err)
	}
	return proto.OK("Rendered an audiogram video of %s:\n%s", c.Rel(m.Path), describe(ctx, c, out))
}
