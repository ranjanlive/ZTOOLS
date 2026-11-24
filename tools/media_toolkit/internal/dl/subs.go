// SPDX-License-Identifier: Apache-2.0

package dl

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Cue is one subtitle entry.
type Cue struct {
	Start, End float64
	Text       string
}

var (
	tagRe    = regexp.MustCompile(`<[^>]*>`)
	timingRe = regexp.MustCompile(`^\s*((?:\d+:)?\d{1,2}:\d{2}[.,]\d{1,3})\s*-->\s*((?:\d+:)?\d{1,2}:\d{2}[.,]\d{1,3})`)
	spaceRe  = regexp.MustCompile(`\s+`)
)

func parseStamp(s string) float64 {
	s = strings.Replace(s, ",", ".", 1)
	parts := strings.Split(s, ":")
	total := 0.0
	for _, p := range parts {
		v, _ := strconv.ParseFloat(p, 64)
		total = total*60 + v
	}
	return total
}

// ParseCues reads SRT or WebVTT and removes the rolling duplicates that
// YouTube's automatic captions contain (each cue repeats the previous line).
func ParseCues(text string) []Cue {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(text, "\n")

	// Group text lines under the timing line before them. Working line by
	// line (not by blank-line blocks) keeps text that follows a blank line.
	type raw struct {
		start, end float64
		lines      []string
	}
	var groups []raw
	for i, l := range lines {
		if m := timingRe.FindStringSubmatch(l); m != nil {
			groups = append(groups, raw{start: parseStamp(m[1]), end: parseStamp(m[2])})
			continue
		}
		if len(groups) == 0 {
			continue // WEBVTT header, metadata
		}
		t := strings.TrimSpace(l)
		// an SRT index line directly before the next timing line
		if isIndex(t) && i+1 < len(lines) && timingRe.MatchString(lines[i+1]) {
			continue
		}
		if strings.HasPrefix(t, "NOTE") || strings.HasPrefix(t, "STYLE") {
			continue
		}
		t = strings.TrimSpace(spaceRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(t, "")), " "))
		if t != "" {
			g := &groups[len(groups)-1]
			g.lines = append(g.lines, t)
		}
	}

	var cues []Cue
	var prev map[string]bool
	for _, g := range groups {
		var kept []string
		cur := map[string]bool{}
		for _, l := range g.lines {
			cur[l] = true
			if !prev[l] {
				kept = append(kept, l)
			}
		}
		prev = cur
		if len(kept) == 0 {
			continue
		}
		cue := Cue{Start: g.start, End: g.end, Text: strings.Join(kept, " ")}
		// very short "transition" cues in auto captions repeat the next one
		if n := len(cues); n > 0 && cues[n-1].Text == cue.Text {
			cues[n-1].End = cue.End
			continue
		}
		cues = append(cues, cue)
	}
	return cues
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func clock(sec float64) string {
	s := int(sec)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// PlainText joins cues into readable paragraphs.
func PlainText(cues []Cue) string {
	var b strings.Builder
	para := 0
	for i, c := range cues {
		if i > 0 {
			gap := c.Start - cues[i-1].End
			endsSentence := strings.HasSuffix(cues[i-1].Text, ".") || strings.HasSuffix(cues[i-1].Text, "?") || strings.HasSuffix(cues[i-1].Text, "!")
			if gap > 2.5 || (para > 700 && endsSentence) {
				b.WriteString("\n\n")
				para = 0
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(c.Text)
		para += len(c.Text)
	}
	return strings.TrimSpace(b.String())
}

// Timestamped groups cues into roughly 20-second chunks with a time label.
func Timestamped(cues []Cue) string {
	var b strings.Builder
	start := -1.0
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			fmt.Fprintf(&b, "[%s] %s\n", clock(start), strings.Join(cur, " "))
		}
		cur, start = nil, -1
	}
	for _, c := range cues {
		if start < 0 {
			start = c.Start
		}
		cur = append(cur, c.Text)
		if c.End-start >= 20 {
			flush()
		}
	}
	flush()
	return strings.TrimRight(b.String(), "\n")
}

// SRT renders cues as a clean SRT file.
func SRT(cues []Cue) string {
	stamp := func(s float64) string {
		ms := int64(s*1000 + 0.5)
		return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
	}
	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, stamp(c.Start), stamp(c.End), c.Text)
	}
	return b.String()
}
