// SPDX-License-Identifier: Apache-2.0

package proto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseTime(t *testing.T) {
	for in, want := range map[string]float64{"90": 90, "12.5": 12.5, "1:30": 90, "01:02:03.5": 3723.5, "0:05": 5, "45s": 45} {
		got, err := ParseTime(in)
		if err != nil || got != want {
			t.Errorf("ParseTime(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "1:75", "-3", "1:2:3:4"} {
		if _, err := ParseTime(bad); err == nil {
			t.Errorf("ParseTime(%q) should fail", bad)
		}
	}
	var v struct{ A, B Time }
	if err := json.Unmarshal([]byte(`{"A": 7.5, "B": "1:00"}`), &v); err != nil || v.A.Seconds != 7.5 || v.B.Seconds != 60 || !v.A.Set {
		t.Errorf("Time JSON: %+v %v", v, err)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	var v struct {
		Input string `json:"input"`
	}
	if err := Decode([]byte(`{"inptu":"x"}`), &v); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := Decode(nil, &v); err != nil {
		t.Fatalf("empty input: %v", err)
	}
}

func TestOutputNaming(t *testing.T) {
	ws := t.TempDir()
	c, err := NewCtx(ws, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(c.Workspace, "clip.mp4")
	_ = os.WriteFile(in, []byte("x"), 0o644)
	p1, _ := c.Output("", in, "trim", "mp4", false)
	_ = os.WriteFile(p1, []byte("x"), 0o644)
	p2, _ := c.Output("", in, "trim", "mp4", false)
	if filepath.Base(p1) != "clip_trim.mp4" || filepath.Base(p2) != "clip_trim_2.mp4" {
		t.Errorf("names %s, %s", p1, p2)
	}
	if _, err := c.Output("clip.mp4", in, "x", "mp4", true); err == nil {
		t.Error("writing over the input must be refused even with overwrite")
	}
	if p, err := c.Output("sub/dir/out", in, "x", "mp3", false); err != nil || filepath.Ext(p) != ".mp3" {
		t.Errorf("extension not added: %s %v", p, err)
	}
}
