// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/dl"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/ff"
	"github.com/ranjanlive/ZTOOLS/tools/media_toolkit/internal/proto"
)

const rightsNote = "Only download content you own or have permission to use, and respect the site's terms."

// ------------------------------------------------------------- media_download

type downloadReq struct {
	URL              string     `json:"url"`
	Kind             string     `json:"kind"`
	Quality          string     `json:"quality"`
	AudioFormat      string     `json:"audio_format"`
	OutputDir        string     `json:"output_dir"`
	Start            proto.Time `json:"start"`
	End              proto.Time `json:"end"`
	Subtitles        bool       `json:"subtitles"`
	SubtitleLanguage string     `json:"subtitle_language"`
	PlaylistLimit    int        `json:"playlist_limit"`
	MaxFilesizeMB    int        `json:"max_filesize_mb"`
	Backend          string     `json:"backend"`
}

func init() {
	register("media_download", "Download a video or its audio from YouTube and 1000+ other sites (yt-dlp, or pytubefix for YouTube).", runDownload)
}

var qualities = map[string]int{"best": 0, "2160p": 2160, "1440p": 1440, "1080p": 1080, "720p": 720, "480p": 480, "360p": 360, "240p": 240}

func runDownload(ctx context.Context, c *proto.Ctx, r *downloadReq) proto.Result {
	kind := orDefault(r.Kind, "video")
	if err := oneOf("kind", kind, "video", "audio"); err != nil {
		return proto.Fail("%v", err)
	}
	q := orDefault(r.Quality, "best")
	maxH, ok := qualities[q]
	if !ok {
		return proto.Fail("quality must be one of best, 2160p, 1440p, 1080p, 720p, 480p, 360p or 240p")
	}
	af := orDefault(r.AudioFormat, "mp3")
	if err := oneOf("audio_format", af, "mp3", "m4a", "opus", "wav", "flac"); err != nil {
		return proto.Fail("%v", err)
	}
	if r.PlaylistLimit < 0 || r.PlaylistLimit > 200 {
		return proto.Fail("playlist_limit must be between 0 (single video) and 200")
	}
	if r.MaxFilesizeMB < 0 {
		return proto.Fail("max_filesize_mb must be positive")
	}
	u, err := dl.CheckURL(r.URL)
	if err != nil {
		return proto.Fail("%v", err)
	}
	backend, err := dl.Backend(r.Backend, u)
	if err != nil {
		return proto.Fail("%v", err)
	}
	ffmpeg, ffErr := ff.Find("ffmpeg")
	if ffErr != nil && (kind == "audio" && af != "m4a" || r.Start.Set || r.End.Set) {
		return proto.Fail("%v", ffErr)
	}
	start, end := 0.0, 0.0
	if r.Start.Set {
		start = r.Start.Seconds
	}
	if r.End.Set {
		end = r.End.Seconds
		if end <= start {
			return proto.Fail("end must be after start")
		}
	}
	outDir, err := c.Dir("output_dir", r.OutputDir, "downloads")
	if err != nil {
		return proto.Fail("%v", err)
	}
	work, cleanup, err := ff.Workdir(c, nil)
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()

	c.Notify(true, "media_download: starting (%s)", backend)
	items, err := dl.Download(ctx, backend, dl.Options{
		URL: u.String(), Kind: kind, MaxHeight: maxH, AudioFormat: af, OutputDir: outDir, Start: start, End: end,
		Subtitles: r.Subtitles, SubLang: r.SubtitleLanguage, PlaylistLimit: r.PlaylistLimit, MaxFilesizeMB: r.MaxFilesizeMB,
		FFmpeg: ffmpeg, WorkDir: work,
	}, func(pct float64, _ string) { c.Notify(false, "media_download: %.0f%%", pct) })
	if err != nil {
		return proto.Fail("download failed: %v", err)
	}
	if backend == "pytubefix" {
		for i := range items {
			if err := finishPytube(ctx, c, &items[i], kind, af, outDir, start, end); err != nil {
				return proto.Fail("%v", err)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Downloaded %d item(s) with %s into %s:\n", len(items), backend, c.Rel(outDir))
	for _, it := range items {
		fmt.Fprintf(&b, "• %s", it.Title)
		if it.Uploader != "" {
			fmt.Fprintf(&b, " — %s", it.Uploader)
		}
		b.WriteString("\n")
		for _, f := range it.Files {
			if strings.HasSuffix(strings.ToLower(f), ".srt") || strings.HasSuffix(strings.ToLower(f), ".vtt") {
				fmt.Fprintf(&b, "    %s (subtitles)\n", c.Rel(f))
				continue
			}
			fmt.Fprintf(&b, "    %s\n", describe(ctx, c, f))
		}
	}
	b.WriteString(rightsNote)
	return proto.OK("%s", b.String())
}

var unsafeName = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

func safeName(title, id string) string {
	n := strings.Trim(unsafeName.ReplaceAllString(title, "_"), " .")
	if r := []rune(n); len(r) > 150 {
		n = string(r[:150])
	}
	if n == "" {
		n = "video"
	}
	return fmt.Sprintf("%s [%s]", n, id)
}

// finishPytube merges separately downloaded streams, converts audio and
// applies a requested section, then moves the result into outDir.
func finishPytube(ctx context.Context, c *proto.Ctx, it *dl.Item, kind, af, outDir string, start, end float64) error {
	base := safeName(it.Title, it.ID)
	ext := "mp4"
	if kind == "audio" {
		ext = af
	}
	final := uniquePath(filepath.Join(outDir, base+"."+ext))
	var args []string
	section := func() {
		if start > 0 {
			args = append(args, "-ss", ff.Sec(start))
		}
		if end > 0 {
			args = append(args, "-to", ff.Sec(end))
		}
	}
	switch {
	case it.PendingVideo != "":
		section()
		args = append(args, "-i", it.PendingVideo)
		if it.PendingAudio != "" {
			section()
			args = append(args, "-i", it.PendingAudio, "-map", "0:v:0", "-map", "1:a:0")
		}
		if start > 0 || end > 0 {
			args = append(args, "-c:v", "libx264", "-crf", "18", "-preset", "medium", "-c:a", "aac", "-b:a", "192k")
		} else {
			args = append(args, "-c", "copy")
		}
		args = append(args, "-movflags", "+faststart", final)
	case it.PendingAudio != "":
		section()
		args = append(args, "-i", it.PendingAudio, "-vn")
		if af == "m4a" && start == 0 && end == 0 {
			args = append(args, "-c:a", "copy")
		} else {
			enc, err := ff.Encoders(af, false, true, 0, "")
			if err != nil {
				return err
			}
			if af == "mp3" {
				enc = dropArgs(enc, "-b:a")
				enc = append(enc, "-q:a", "0")
			}
			args = append(args, enc...)
		}
		args = append(args, final)
	case len(it.Files) > 0 && (start > 0 || end > 0):
		section()
		args = append(args, "-i", it.Files[0], "-c:v", "libx264", "-crf", "18", "-preset", "medium", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", final)
		it.Files = it.Files[1:]
	case len(it.Files) > 0:
		moved, err := moveFile(it.Files[0], final)
		if err != nil {
			return err
		}
		it.Files[0] = moved
		return moveSubs(c, it, outDir)
	default:
		return fmt.Errorf("nothing was downloaded for %s", it.Title)
	}
	if _, err := ff.Run(ctx, ff.Job{Args: args, Label: "media_download (processing)", Ctx: c}); err != nil {
		_ = os.Remove(final)
		return err
	}
	it.Files = append([]string{final}, it.Files...)
	return moveSubs(c, it, outDir)
}

func moveSubs(c *proto.Ctx, it *dl.Item, outDir string) error {
	for i, f := range it.Files {
		if strings.HasSuffix(strings.ToLower(f), ".srt") && filepath.Dir(f) != outDir {
			moved, err := moveFile(f, uniquePath(filepath.Join(outDir, filepath.Base(f))))
			if err != nil {
				return err
			}
			it.Files[i] = moved
		}
	}
	return nil
}

func uniquePath(p string) string {
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
		p = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
}

// moveFile renames, falling back to copy+delete across volumes.
func moveFile(src, dst string) (string, error) {
	if err := os.Rename(src, dst); err == nil {
		return dst, nil
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	in.Close()
	_ = os.Remove(src)
	return dst, nil
}

// --------------------------------------------------------------- media_lookup

type lookupReq struct {
	URL            string `json:"url"`
	PlaylistLimit  int    `json:"playlist_limit"`
	MaxDescription int    `json:"max_description"`
	Backend        string `json:"backend"`
}

func init() {
	register("media_lookup", "Look up an online video or playlist without downloading: title, channel, duration, views, chapters, qualities, captions.", runLookup)
}

func runLookup(ctx context.Context, c *proto.Ctx, r *lookupReq) proto.Result {
	limit := r.PlaylistLimit
	if limit == 0 {
		limit = 25
	}
	if limit < 1 || limit > 500 {
		return proto.Fail("playlist_limit must be between 1 and 500")
	}
	maxDesc := r.MaxDescription
	if maxDesc == 0 {
		maxDesc = 1500
	}
	u, err := dl.CheckURL(r.URL)
	if err != nil {
		return proto.Fail("%v", err)
	}
	backend, err := dl.Backend(r.Backend, u)
	if err != nil {
		return proto.Fail("%v", err)
	}
	work, cleanup, err := ff.Workdir(c, nil)
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()
	m, err := dl.Lookup(ctx, backend, u.String(), limit, work)
	if err != nil {
		return proto.Fail("lookup failed: %v", err)
	}
	var b strings.Builder
	if m.Kind == "playlist" {
		fmt.Fprintf(&b, "Playlist: %s\n", m.Title)
		if m.Uploader != "" {
			fmt.Fprintf(&b, "By: %s\n", m.Uploader)
		}
		if m.EntryCount > 0 {
			fmt.Fprintf(&b, "Videos: %d\n", m.EntryCount)
		}
		for i, e := range m.Entries {
			d := ""
			if e.Duration > 0 {
				d = " (" + proto.Clock(e.Duration) + ")"
			}
			fmt.Fprintf(&b, "  %3d. %s%s  %s\n", i+1, e.Title, d, e.URL)
		}
		return proto.OK("%s", strings.TrimRight(b.String(), "\n"))
	}
	fmt.Fprintf(&b, "Title: %s\n", m.Title)
	if m.Uploader != "" {
		fmt.Fprintf(&b, "Channel: %s\n", m.Uploader)
	}
	if m.Live {
		b.WriteString("Live stream: yes\n")
	}
	if m.Duration > 0 {
		fmt.Fprintf(&b, "Duration: %s\n", proto.Clock(m.Duration))
	}
	if len(m.UploadDate) == 8 {
		fmt.Fprintf(&b, "Published: %s-%s-%s\n", m.UploadDate[:4], m.UploadDate[4:6], m.UploadDate[6:])
	}
	if m.Views > 0 {
		fmt.Fprintf(&b, "Views: %d", m.Views)
		if m.Likes > 0 {
			fmt.Fprintf(&b, ", likes: %d", m.Likes)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "URL: %s\nID: %s\n", m.URL, m.ID)
	if len(m.Heights) > 0 {
		sort.Sort(sort.Reverse(sort.IntSlice(m.Heights)))
		hs := make([]string, len(m.Heights))
		for i, h := range m.Heights {
			hs[i] = fmt.Sprintf("%dp", h)
		}
		fmt.Fprintf(&b, "Video qualities: %s\n", strings.Join(hs, ", "))
	}
	if len(m.Captions) > 0 {
		sort.Strings(m.Captions)
		fmt.Fprintf(&b, "Subtitles: %s\n", strings.Join(m.Captions, ", "))
	}
	if len(m.AutoCaption) > 0 {
		sort.Strings(m.AutoCaption)
		ac := m.AutoCaption
		if len(ac) > 20 {
			ac = append(ac[:20], fmt.Sprintf("… %d more", len(m.AutoCaption)-20))
		}
		fmt.Fprintf(&b, "Automatic captions: %s\n", strings.Join(ac, ", "))
	}
	if len(m.Tags) > 0 {
		t := m.Tags
		if len(t) > 15 {
			t = t[:15]
		}
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(t, ", "))
	}
	if len(m.Chapters) > 0 {
		fmt.Fprintf(&b, "Chapters:\n")
		for _, ch := range m.Chapters {
			fmt.Fprintf(&b, "  %s %s\n", proto.Clock(ch.Start), ch.Title)
		}
	}
	if d := strings.TrimSpace(m.Description); d != "" && maxDesc > 0 {
		if r := []rune(d); len(r) > maxDesc {
			d = string(r[:maxDesc]) + "…"
		}
		fmt.Fprintf(&b, "Description:\n%s\n", d)
	}
	return proto.OK("%s", strings.TrimRight(b.String(), "\n"))
}

// ----------------------------------------------------------- video_transcript

type transcriptReq struct {
	URL       string `json:"url"`
	Language  string `json:"language"`
	Format    string `json:"format"`
	SaveTo    string `json:"save_to"`
	MaxChars  int    `json:"max_chars"`
	Backend   string `json:"backend"`
	Overwrite bool   `json:"overwrite"`
}

func init() {
	register("video_transcript", "Get the transcript (subtitles or automatic captions) of an online video as clean text, timestamped text or SRT.", runTranscript)
}

var langCodeRe = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)

func runTranscript(ctx context.Context, c *proto.Ctx, r *transcriptReq) proto.Result {
	lang := orDefault(r.Language, "en")
	if !langCodeRe.MatchString(lang) {
		return proto.Fail("language must be a code such as en, hi, es or pt-BR")
	}
	format := orDefault(r.Format, "text")
	if err := oneOf("format", format, "text", "timestamped", "srt"); err != nil {
		return proto.Fail("%v", err)
	}
	maxChars := r.MaxChars
	if maxChars == 0 {
		maxChars = 40000
	}
	if maxChars < 500 || maxChars > 500000 {
		return proto.Fail("max_chars must be between 500 and 500000")
	}
	u, err := dl.CheckURL(r.URL)
	if err != nil {
		return proto.Fail("%v", err)
	}
	backend, err := dl.Backend(r.Backend, u)
	if err != nil {
		return proto.Fail("%v", err)
	}
	work, cleanup, err := ff.Workdir(c, nil)
	if err != nil {
		return proto.Fail("%v", err)
	}
	defer cleanup()
	capt, err := dl.Captions(ctx, backend, u.String(), lang, work)
	if err != nil {
		return proto.Fail("%v", err)
	}
	cues := dl.ParseCues(capt.Text)
	if len(cues) == 0 {
		return proto.Fail("the %s captions are empty", capt.Lang)
	}
	var text string
	ext := "txt"
	switch format {
	case "text":
		text = dl.PlainText(cues)
	case "timestamped":
		text = dl.Timestamped(cues)
	case "srt":
		text, ext = dl.SRT(cues), "srt"
	}
	kind := "subtitles"
	if capt.Auto {
		kind = "automatic captions"
	}
	header := fmt.Sprintf("Transcript of %q (%s, %s, %d words)", capt.Title, capt.Lang, kind, len(strings.Fields(dl.PlainText(cues))))

	saved := ""
	if r.SaveTo != "" || len(text) > maxChars {
		target := r.SaveTo
		if target == "" {
			target = filepath.Join("transcripts", safeName(orDefault(capt.Title, "transcript"), orDefault(capt.ID, "video"))+"."+ext)
			if p := filepath.Join(c.Workspace, target); fileExists(p) {
				target = c.Rel(uniquePath(p))
			}
		}
		out, err := c.Output(target, "", "transcript", ext, r.Overwrite)
		if err != nil {
			return proto.Fail("%v", err)
		}
		if err := os.WriteFile(out, []byte(text+"\n"), 0o644); err != nil {
			return proto.Fail("%v", err)
		}
		saved = c.Rel(out)
	}
	if len(text) > maxChars {
		cut := text[:maxChars]
		if i := strings.LastIndexAny(cut, ".\n"); i > maxChars/2 {
			cut = cut[:i+1]
		}
		return proto.OK("%s — the first %d of %d characters are shown; the full transcript is saved at %s\n\n%s\n\n[… truncated; read %s for the rest]",
			header, len(cut), len(text), saved, cut, saved)
	}
	if saved != "" {
		header += "; saved to " + saved
	}
	return proto.OK("%s:\n\n%s", header, text)
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
