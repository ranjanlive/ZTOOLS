// SPDX-License-Identifier: Apache-2.0

// Package dl downloads online media through yt-dlp (any of its supported
// sites) or, for YouTube, pytubefix. Both are optional external programs;
// whichever is installed is used.
package dl

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// InstallHint explains how to get a download backend.
func InstallHint() string {
	hint := "Install yt-dlp (recommended, supports YouTube and 1000+ other sites): `pip install -U yt-dlp`"
	switch runtime.GOOS {
	case "windows":
		hint += " or `winget install yt-dlp.yt-dlp`"
	case "darwin":
		hint += " or `brew install yt-dlp`"
	}
	return hint + ". For YouTube only, `pip install -U pytubefix` also works. Set ZTOOLS_YTDLP or ZTOOLS_PYTHON if they are not on PATH."
}

// CheckURL accepts only public http(s) URLs, so an agent cannot be steered
// into fetching from the local machine or the private network.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("url must be a full web address such as https://www.youtube.com/watch?v=…")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are supported")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return nil, fmt.Errorf("local addresses are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
			return nil, fmt.Errorf("private and local network addresses are not allowed")
		}
	}
	return u, nil
}

var ytHost = regexp.MustCompile(`(^|\.)(youtube\.com|youtu\.be|youtube-nocookie\.com)$`)

// IsYouTube reports whether a URL is a YouTube URL.
func IsYouTube(u *url.URL) bool { return ytHost.MatchString(strings.ToLower(u.Hostname())) }

var (
	toolsOnce sync.Once
	ytdlp     []string
	python    []string
	pytube    bool
)

func probe(argv ...string) bool {
	cmd := exec.Command(argv[0], argv[1:]...)
	// Run outside the workspace so a planted yt_dlp.py or pytubefix.py
	// there cannot shadow the real module.
	cmd.Dir = os.TempDir()
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	hideWindow(cmd)
	return cmd.Run() == nil
}

func detect() {
	toolsOnce.Do(func() {
		if p := os.Getenv("ZTOOLS_YTDLP"); p != "" && probe(p, "--version") {
			ytdlp = []string{p}
		} else if p, err := exec.LookPath("yt-dlp"); err == nil && probe(p, "--version") {
			ytdlp = []string{p}
		}
		var cands [][]string
		if p := os.Getenv("ZTOOLS_PYTHON"); p != "" {
			cands = append(cands, []string{p})
		}
		if runtime.GOOS == "windows" {
			cands = append(cands, []string{"py", "-3"}, []string{"python"})
		} else {
			cands = append(cands, []string{"python3"}, []string{"python"})
		}
		for _, c := range cands {
			if _, err := exec.LookPath(c[0]); err != nil {
				continue
			}
			if probe(append(c, "-c", "import sys; sys.exit(0 if sys.version_info >= (3, 8) else 1)")...) {
				python = c
				break
			}
		}
		if python != nil {
			if ytdlp == nil && probe(append(python, "-m", "yt_dlp", "--version")...) {
				ytdlp = append(append([]string{}, python...), "-m", "yt_dlp")
			}
			pytube = probe(append(python, "-c", "import pytubefix")...)
		}
	})
}

// Backend picks a backend for a URL.
func Backend(choice string, u *url.URL) (string, error) {
	detect()
	switch choice {
	case "", "auto":
		if ytdlp != nil {
			return "yt-dlp", nil
		}
		if pytube && IsYouTube(u) {
			return "pytubefix", nil
		}
		if pytube {
			return "", fmt.Errorf("pytubefix only supports YouTube, and yt-dlp is not installed. %s", InstallHint())
		}
		return "", fmt.Errorf("no download backend is installed. %s", InstallHint())
	case "yt-dlp":
		if ytdlp == nil {
			return "", fmt.Errorf("yt-dlp is not installed. %s", InstallHint())
		}
		return "yt-dlp", nil
	case "pytubefix":
		if !pytube {
			return "", fmt.Errorf("pytubefix is not installed (`pip install -U pytubefix`), or Python 3.8+ was not found")
		}
		if !IsYouTube(u) {
			return "", fmt.Errorf("pytubefix only supports YouTube URLs")
		}
		return "pytubefix", nil
	}
	return "", fmt.Errorf("backend must be auto, yt-dlp or pytubefix")
}

// Progress receives progress lines.
type Progress func(pct float64, line string)

var pctRe = regexp.MustCompile(`(\d{1,3}(?:\.\d+)?)%`)

// run executes a backend command, streaming progress from stdout/stderr.
// It returns stdout (minus progress lines) and stderr.
func run(ctx context.Context, argv []string, dir string, env []string, progress Progress) (string, string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	hideWindow(cmd)
	so, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	se, err := cmd.StderrPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	var out, errb bytes.Buffer
	var wg sync.WaitGroup
	scan := func(r io.Reader, buf *bytes.Buffer) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "[download]") || strings.HasPrefix(line, "PROGRESS ") {
				if m := pctRe.FindStringSubmatch(line); m != nil && progress != nil {
					var v float64
					fmt.Sscanf(m[1], "%g", &v)
					progress(v, line)
				}
				if strings.HasPrefix(line, "PROGRESS ") {
					continue
				}
			}
			if buf.Len() < 32<<20 {
				buf.WriteString(line)
				buf.WriteByte('\n')
			}
		}
	}
	wg.Add(2)
	go scan(so, &out)
	go scan(se, &errb)
	wg.Wait()
	err = cmd.Wait()
	if ctx.Err() != nil {
		return out.String(), errb.String(), ctx.Err()
	}
	return out.String(), errb.String(), err
}

// cleanErr extracts the useful part of a backend's error output.
func cleanErr(stderr string, err error) error {
	var lines []string
	for _, l := range strings.Split(stderr, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "ERROR:") || strings.Contains(l, "Error") || strings.Contains(l, "error:") {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		all := strings.Split(strings.TrimSpace(stderr), "\n")
		if len(all) > 6 {
			all = all[len(all)-6:]
		}
		lines = all
	}
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	msg := strings.TrimSpace(strings.Join(lines, "\n"))
	if msg == "" {
		if err == nil {
			return errors.New("the download failed")
		}
		return err
	}
	return errors.New(msg)
}
