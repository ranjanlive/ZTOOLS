// SPDX-License-Identifier: Apache-2.0

package dl

import (
	"strings"
	"testing"
)

// YouTube automatic captions repeat the previous line in every cue.
const rollingVTT = `WEBVTT
Kind: captions
Language: en

00:00:00.160 --> 00:00:02.310 align:start position:0%

hello<00:00:00.640><c> everyone</c><00:00:01.040><c> and</c>

00:00:02.310 --> 00:00:02.320 align:start position:0%
hello everyone and


00:00:02.320 --> 00:00:04.950 align:start position:0%
hello everyone and
welcome<00:00:02.800><c> to</c><00:00:03.000><c> the</c><00:00:03.200><c> show</c>

00:00:04.950 --> 00:00:07.000 align:start position:0%
welcome to the show
today &amp; tomorrow
`

func TestParseCuesRemovesRollingDuplicates(t *testing.T) {
	cues := ParseCues(rollingVTT)
	got := PlainText(cues)
	if got != "hello everyone and welcome to the show today & tomorrow" {
		t.Fatalf("plain text = %q", got)
	}
	if len(cues) != 3 || cues[1].Start != 2.32 {
		t.Fatalf("cues = %+v", cues)
	}
	if ts := Timestamped(cues); !strings.HasPrefix(ts, "[0:00] hello everyone") {
		t.Errorf("timestamped = %q", ts)
	}
	srt := SRT(cues)
	if !strings.Contains(srt, "1\n00:00:00,160 --> 00:00:02,310\nhello everyone and\n") {
		t.Errorf("srt = %q", srt)
	}
}

func TestParseSRT(t *testing.T) {
	cues := ParseCues("1\r\n00:00:01,000 --> 00:00:02,500\r\n<i>First</i> line\r\nsecond part\r\n\r\n2\r\n01:00:00,000 --> 01:00:01,000\r\nLast\r\n")
	if len(cues) != 2 || cues[0].Text != "First line second part" || cues[1].Start != 3600 {
		t.Fatalf("cues = %+v", cues)
	}
}

func TestCheckURL(t *testing.T) {
	for _, ok := range []string{"https://www.youtube.com/watch?v=x", "http://vimeo.com/1", "https://youtu.be/abc"} {
		if _, err := CheckURL(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"file:///etc/passwd", "ftp://x.com/a", "http://localhost:8080/", "http://127.0.0.1/", "http://10.0.0.5/v", "http://192.168.1.1/", "http://[::1]/", "http://169.254.169.254/latest", "notaurl", "http://printer.local/"} {
		if _, err := CheckURL(bad); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}
}
