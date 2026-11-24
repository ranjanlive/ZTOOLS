// SPDX-License-Identifier: Apache-2.0

package dl

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed pytube_helper.py
var pytubeScript []byte

// Options describes a download.
type Options struct {
	URL           string
	Kind          string // video | audio
	MaxHeight     int    // 0 = best
	AudioFormat   string // mp3 | m4a | opus | wav | flac
	OutputDir     string
	Start, End    float64 // section to download; End 0 = to the end
	Subtitles     bool
	SubLang       string
	PlaylistLimit int // 0 = single item only
	MaxFilesizeMB int
	FFmpeg        string
	WorkDir       string // private scratch directory
}

// Item is one downloaded entry.
type Item struct {
	ID       string
	Title    string
	Uploader string
	Duration float64
	Files    []string
	// Pending are raw streams the caller must still merge or convert
	// (pytubefix downloads video and audio separately).
	PendingVideo, PendingAudio string
}

// Download fetches media with the chosen backend.
func Download(ctx context.Context, backend string, o Options, progress Progress) ([]Item, error) {
	if backend == "pytubefix" {
		return pytubeDownload(ctx, o, progress)
	}
	return ytdlpDownload(ctx, o, progress)
}

func ytdlpDownload(ctx context.Context, o Options, progress Progress) ([]Item, error) {
	args := append([]string{}, ytdlp...)
	args = append(args, "--encoding", "utf-8", "--no-color", "--newline", "--progress", "--no-mtime", "--no-part",
		"-P", o.OutputDir, "-P", "temp:"+o.WorkDir, "-o", "%(title).150B [%(id)s].%(ext)s",
		"--print", "after_move:ZTFILE\t%(id)s\t%(duration)s\t%(filepath)s\t%(uploader)s\t%(title)s")
	if o.FFmpeg != "" {
		args = append(args, "--ffmpeg-location", o.FFmpeg)
	}
	if o.PlaylistLimit > 0 {
		args = append(args, "--yes-playlist", "--playlist-items", fmt.Sprintf("1:%d", o.PlaylistLimit))
	} else {
		args = append(args, "--no-playlist")
	}
	if o.MaxFilesizeMB > 0 {
		args = append(args, "--max-filesize", fmt.Sprintf("%dM", o.MaxFilesizeMB))
	}
	if o.Start > 0 || o.End > 0 {
		end := "inf"
		if o.End > 0 {
			end = strconv.FormatFloat(o.End, 'f', 3, 64)
		}
		args = append(args, "--download-sections", fmt.Sprintf("*%s-%s", strconv.FormatFloat(o.Start, 'f', 3, 64), end), "--force-keyframes-at-cuts")
	}
	if o.Kind == "audio" {
		args = append(args, "-f", "ba/b", "-x", "--audio-format", o.AudioFormat, "--audio-quality", "0", "--embed-metadata")
	} else {
		h := ""
		if o.MaxHeight > 0 {
			h = fmt.Sprintf("[height<=%d]", o.MaxHeight)
		}
		args = append(args, "-f", fmt.Sprintf("bv*%s+ba/b%s/bv*+ba/b", h, h), "-S", "vcodec:h264,res,acodec:m4a", "--merge-output-format", "mp4", "--embed-metadata")
	}
	if o.Subtitles {
		lang := orDefault(o.SubLang, "en")
		args = append(args, "--write-subs", "--write-auto-subs", "--sub-langs", lang+","+lang+"-orig,"+lang+".*", "--convert-subs", "srt")
	}
	args = append(args, "--", o.URL)
	stdout, stderr, err := run(ctx, args, o.WorkDir, nil, progress)
	var items []Item
	for _, line := range strings.Split(stdout, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "ZTFILE\t")
		if !ok {
			continue
		}
		f := strings.SplitN(rest, "\t", 5)
		if len(f) < 5 {
			continue
		}
		d, _ := strconv.ParseFloat(f[1], 64)
		it := Item{ID: f[0], Duration: d, Files: []string{f[2]}, Uploader: na(f[3]), Title: f[4]}
		if o.Subtitles {
			stem := strings.TrimSuffix(f[2], filepath.Ext(f[2]))
			if subs, _ := filepath.Glob(globEscape(stem) + ".*.srt"); len(subs) > 0 {
				it.Files = append(it.Files, subs...)
			}
		}
		items = append(items, it)
	}
	if err != nil && len(items) == 0 {
		return nil, cleanErr(stderr, err)
	}
	if len(items) == 0 {
		if strings.Contains(stderr, "larger than max-filesize") || strings.Contains(stdout, "larger than max-filesize") {
			return nil, fmt.Errorf("the file is larger than max_filesize_mb (%d MB), so it was not downloaded", o.MaxFilesizeMB)
		}
		return nil, cleanErr(stderr, fmt.Errorf("nothing was downloaded"))
	}
	return items, nil
}

func na(s string) string {
	if s == "NA" {
		return ""
	}
	return s
}

func globEscape(s string) string {
	r := strings.NewReplacer("[", "[[]", "*", "[*]", "?", "[?]")
	return r.Replace(s)
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// runHelper runs the helper script with a JSON request.
func runHelper(ctx context.Context, work string, req map[string]any, progress Progress) (map[string]any, error) {
	script := filepath.Join(work, "ztools_pytube_helper.py")
	if err := os.WriteFile(script, pytubeScript, 0o600); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(req)
	reqFile := filepath.Join(work, "request.json")
	if err := os.WriteFile(reqFile, b, 0o600); err != nil {
		return nil, err
	}
	args := append(append([]string{}, python...), script, reqFile)
	stdout, stderr, err := run(ctx, args, work, nil, progress)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	var res map[string]any
	if len(lines) > 0 {
		_ = json.Unmarshal([]byte(lines[len(lines)-1]), &res)
	}
	if res == nil {
		return nil, cleanErr(stderr, err)
	}
	if e, _ := res["error"].(string); e != "" {
		return nil, fmt.Errorf("%s", e)
	}
	return res, nil
}

func pytubeDownload(ctx context.Context, o Options, progress Progress) ([]Item, error) {
	// pytubefix cannot fetch a section; the caller trims afterwards.
	res, err := runHelper(ctx, o.WorkDir, map[string]any{
		"op": "download", "url": o.URL, "kind": o.Kind, "max_height": o.MaxHeight, "out": o.WorkDir,
		"playlist_limit": o.PlaylistLimit, "subtitles": o.Subtitles, "sub_lang": orDefault(o.SubLang, "en"),
		"max_filesize_mb": o.MaxFilesizeMB,
	}, progress)
	if err != nil {
		return nil, err
	}
	raw, _ := res["items"].([]any)
	var items []Item
	for _, x := range raw {
		m, _ := x.(map[string]any)
		it := Item{ID: str(m["id"]), Title: str(m["title"]), Uploader: str(m["author"]), Duration: num(m["length"]),
			PendingVideo: str(m["video"]), PendingAudio: str(m["audio"])}
		if s := str(m["single"]); s != "" {
			it.Files = append(it.Files, s)
		}
		if s := str(m["subtitles"]); s != "" {
			it.Files = append(it.Files, s)
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("nothing was downloaded")
	}
	return items, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

// Meta is what Lookup returns.
type Meta struct {
	Kind        string // video | playlist
	ID          string
	Title       string
	Uploader    string
	Duration    float64
	Views       int64
	Likes       int64
	UploadDate  string
	Description string
	URL         string
	Tags        []string
	Chapters    []Chapter
	Heights     []int
	Captions    []string
	AutoCaption []string
	Live        bool
	Entries     []Entry
	EntryCount  int
}

// Chapter is a chapter marker.
type Chapter struct {
	Start float64
	Title string
}

// Entry is a playlist entry.
type Entry struct {
	Title    string
	URL      string
	Duration float64
}

// Lookup fetches metadata without downloading.
func Lookup(ctx context.Context, backend, rawURL string, limit int, work string) (*Meta, error) {
	if backend == "pytubefix" {
		res, err := runHelper(ctx, work, map[string]any{"op": "info", "url": rawURL, "playlist_limit": limit}, nil)
		if err != nil {
			return nil, err
		}
		m := &Meta{Kind: orDefault(str(res["kind"]), "video"), ID: str(res["id"]), Title: str(res["title"]), Uploader: str(res["author"]),
			Duration: num(res["length"]), Views: int64(num(res["views"])), UploadDate: str(res["publish_date"]), Description: str(res["description"]),
			URL: rawURL, EntryCount: int(num(res["count"]))}
		for _, k := range anyList(res["keywords"]) {
			m.Tags = append(m.Tags, str(k))
		}
		for _, h := range anyList(res["heights"]) {
			m.Heights = append(m.Heights, int(num(h)))
		}
		for _, cpt := range anyList(res["captions"]) {
			code := str(cpt)
			if strings.HasPrefix(code, "a.") {
				m.AutoCaption = append(m.AutoCaption, strings.TrimPrefix(code, "a."))
			} else {
				m.Captions = append(m.Captions, code)
			}
		}
		for _, ch := range anyList(res["chapters"]) {
			cm, _ := ch.(map[string]any)
			m.Chapters = append(m.Chapters, Chapter{Start: num(cm["start"]), Title: str(cm["title"])})
		}
		for _, e := range anyList(res["entries"]) {
			em, _ := e.(map[string]any)
			m.Entries = append(m.Entries, Entry{Title: str(em["title"]), URL: str(em["url"]), Duration: num(em["length"])})
		}
		return m, nil
	}
	args := append(append([]string{}, ytdlp...), "--encoding", "utf-8", "-J", "--no-warnings", "--flat-playlist", "--playlist-items", fmt.Sprintf("1:%d", max(limit, 1)), "--", rawURL)
	stdout, stderr, err := run(ctx, args, work, nil, nil)
	if err != nil {
		return nil, cleanErr(stderr, err)
	}
	var j struct {
		Type        string   `json:"_type"`
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Uploader    string   `json:"uploader"`
		Channel     string   `json:"channel"`
		Duration    float64  `json:"duration"`
		Views       int64    `json:"view_count"`
		Likes       int64    `json:"like_count"`
		UploadDate  string   `json:"upload_date"`
		Description string   `json:"description"`
		WebpageURL  string   `json:"webpage_url"`
		Tags        []string `json:"tags"`
		IsLive      bool     `json:"is_live"`
		Count       int      `json:"playlist_count"`
		Chapters    []struct {
			Start float64 `json:"start_time"`
			Title string  `json:"title"`
		} `json:"chapters"`
		Formats []struct {
			Height int    `json:"height"`
			VCodec string `json:"vcodec"`
		} `json:"formats"`
		Subtitles    map[string]any `json:"subtitles"`
		AutoCaptions map[string]any `json:"automatic_captions"`
		Entries      []struct {
			Title    string  `json:"title"`
			URL      string  `json:"url"`
			Duration float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(stdout), &j); err != nil {
		return nil, fmt.Errorf("could not read yt-dlp output: %v", err)
	}
	m := &Meta{Kind: "video", ID: j.ID, Title: j.Title, Uploader: orDefault(j.Uploader, j.Channel), Duration: j.Duration, Views: j.Views, Likes: j.Likes,
		UploadDate: j.UploadDate, Description: j.Description, URL: orDefault(j.WebpageURL, rawURL), Tags: j.Tags, Live: j.IsLive}
	if j.Type == "playlist" {
		m.Kind = "playlist"
		m.EntryCount = j.Count
		for _, e := range j.Entries {
			m.Entries = append(m.Entries, Entry{Title: e.Title, URL: e.URL, Duration: e.Duration})
		}
	}
	for _, ch := range j.Chapters {
		m.Chapters = append(m.Chapters, Chapter{Start: ch.Start, Title: ch.Title})
	}
	seen := map[int]bool{}
	for _, f := range j.Formats {
		if f.Height > 0 && f.VCodec != "none" && !seen[f.Height] {
			seen[f.Height] = true
			m.Heights = append(m.Heights, f.Height)
		}
	}
	for k := range j.Subtitles {
		if k != "live_chat" {
			m.Captions = append(m.Captions, k)
		}
	}
	for k := range j.AutoCaptions {
		if !strings.Contains(k, "-") || strings.HasSuffix(k, "-orig") {
			m.AutoCaption = append(m.AutoCaption, k)
		}
	}
	return m, nil
}

func anyList(v any) []any {
	l, _ := v.([]any)
	return l
}

// Caption is a fetched subtitle track.
type Caption struct {
	Lang  string
	Auto  bool
	Text  string // SRT or WebVTT
	Title string
	ID    string
}

// Captions fetches subtitles in the requested language.
func Captions(ctx context.Context, backend, rawURL, lang, work string) (*Caption, error) {
	if backend == "pytubefix" {
		res, err := runHelper(ctx, work, map[string]any{"op": "captions", "url": rawURL, "lang": lang}, nil)
		if err != nil {
			return nil, err
		}
		code := str(res["lang"])
		return &Caption{Lang: strings.TrimPrefix(code, "a."), Auto: strings.HasPrefix(code, "a."), Text: str(res["text"]), Title: str(res["title"]), ID: str(res["id"])}, nil
	}
	args := append(append([]string{}, ytdlp...), "--encoding", "utf-8", "--no-warnings", "--skip-download", "--no-playlist",
		"--write-subs", "--write-auto-subs", "--sub-langs", lang+","+lang+"-orig,"+lang+"-.*", "--sub-format", "vtt/srt/best",
		"-P", work, "-o", "subs.%(ext)s", "--print", "after_video:ZTMETA\t%(id)s\t%(title)s", "--no-simulate", "--", rawURL)
	stdout, stderr, err := run(ctx, args, work, nil, nil)
	if err != nil {
		return nil, cleanErr(stderr, err)
	}
	c := &Caption{}
	for _, line := range strings.Split(stdout, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "ZTMETA\t"); ok {
			c.ID, c.Title, _ = strings.Cut(rest, "\t")
		}
	}
	files, _ := filepath.Glob(filepath.Join(work, "subs.*"))
	best := ""
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if ext != ".vtt" && ext != ".srt" {
			continue
		}
		code := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "subs."), ext)
		// prefer the exact language, then the original-language auto track
		if best == "" || code == lang {
			best = f
			c.Lang = code
		}
	}
	if best == "" {
		return nil, fmt.Errorf("no %q subtitles or automatic captions are available for this video; use media_lookup to see which languages exist", lang)
	}
	b, err := os.ReadFile(best)
	if err != nil {
		return nil, err
	}
	c.Text = string(b)
	c.Auto = strings.HasSuffix(c.Lang, "-orig")
	c.Lang = strings.TrimSuffix(c.Lang, "-orig")
	return c, nil
}
