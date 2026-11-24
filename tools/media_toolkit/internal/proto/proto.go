// SPDX-License-Identifier: Apache-2.0

// Package proto implements the ZAgent / ZEngine external tool protocol:
// arguments arrive as one JSON object on stdin, the result is one JSON object
// on stdout, and progress lines can be appended to $ZAGENT_NOTIFY_FILE.
//
// It also owns path policy. Every file a tool reads or writes must resolve
// inside the agent's workspace (or the engine's scratch area), so a request
// cannot reach outside the folder the user gave the agent.
package proto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Media is a file attached to the result (an image the model can look at).
type Media struct {
	Path string `json:"path"`
	MIME string `json:"mime,omitempty"`
}

// Result is what the tool prints on stdout.
type Result struct {
	Output  string  `json:"output"`
	IsError bool    `json:"is_error"`
	Media   []Media `json:"media"`
}

// Fail builds an error result.
func Fail(format string, a ...any) Result {
	return Result{Output: fmt.Sprintf(format, a...), IsError: true}
}

// OK builds a success result.
func OK(format string, a ...any) Result {
	return Result{Output: fmt.Sprintf(format, a...)}
}

// Ctx is the environment a tool runs in.
type Ctx struct {
	Workspace string // absolute, symlinks resolved
	Scratch   string // engine scratch dir (may be empty)
	OutputDir string // per-call output dir for temporary files (always set)
	Vision    bool   // the model can look at images

	notifyFile string
	mu         sync.Mutex
	lastNote   time.Time
}

// FromEnv builds a Ctx from the ZAGENT_* environment, falling back to the
// current directory when the tool is run by hand.
func FromEnv() (*Ctx, error) {
	ws := os.Getenv("ZAGENT_WORKSPACE")
	if ws == "" {
		var err error
		if ws, err = os.Getwd(); err != nil {
			return nil, err
		}
	}
	c := &Ctx{notifyFile: os.Getenv("ZAGENT_NOTIFY_FILE")}
	var err error
	if c.Workspace, err = realDir(ws); err != nil {
		return nil, fmt.Errorf("workspace %s: %w", ws, err)
	}
	if s := os.Getenv("ZAGENT_SCRATCH"); s != "" {
		if r, err := realDir(s); err == nil {
			c.Scratch = r
		}
	}
	c.OutputDir = os.Getenv("ZAGENT_OUTPUT_DIR")
	if c.OutputDir == "" {
		if c.OutputDir, err = os.MkdirTemp("", "ztools-media-"); err != nil {
			return nil, err
		}
	}
	c.Vision = strings.Contains(os.Getenv("ZAGENT_CAPS"), "vision=true")
	return c, nil
}

// NewCtx builds a Ctx for tests and embedding.
func NewCtx(workspace, outputDir string) (*Ctx, error) {
	ws, err := realDir(workspace)
	if err != nil {
		return nil, err
	}
	return &Ctx{Workspace: ws, OutputDir: outputDir}, nil
}

func realDir(p string) (string, error) {
	a, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	r, err := filepath.EvalSymlinks(a)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(r)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("not a directory")
	}
	return r, nil
}

// Notify sends a progress line to the running agent. Lines are rate-limited
// so long jobs report progress without flooding the conversation.
func (c *Ctx) Notify(force bool, format string, a ...any) {
	if c.notifyFile == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && time.Since(c.lastNote) < 20*time.Second {
		return
	}
	c.lastNote = time.Now()
	f, err := os.OpenFile(c.notifyFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()
	line := strings.ReplaceAll(fmt.Sprintf(format, a...), "\n", " ")
	fmt.Fprintln(f, line)
}

// Decode strictly parses the request into v. Unknown fields are an error so a
// misspelled parameter is reported instead of silently ignored.
func Decode(raw []byte, v any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		msg := err.Error()
		msg = strings.Replace(msg, "json: unknown field", "unknown parameter", 1)
		return fmt.Errorf("invalid arguments: %s", msg)
	}
	return nil
}

// inside reports whether p is root or below it.
func inside(root, p string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func (c *Ctx) allowed(p string) bool {
	return inside(c.Workspace, p) || inside(c.Scratch, p) || inside(c.OutputDir, p)
}

func (c *Ctx) abs(p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(c.Workspace, p)
	}
	return filepath.Clean(p)
}

// Input resolves an existing input file.
func (c *Ctx) Input(name, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	a := c.abs(p)
	r, err := filepath.EvalSymlinks(a)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s: file not found: %s", name, p)
		}
		return "", fmt.Errorf("%s: %v", name, err)
	}
	if !c.allowed(r) {
		return "", fmt.Errorf("%s: %s is outside the workspace; only files inside %s can be used", name, p, c.Workspace)
	}
	st, err := os.Stat(r)
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("%s: %s is a directory, not a file", name, p)
	}
	return r, nil
}

// Dir resolves (and creates) an output directory.
func (c *Ctx) Dir(name, p, fallback string) (string, error) {
	if strings.TrimSpace(p) == "" {
		p = fallback
	}
	a := c.abs(p)
	if err := c.checkWritable(a); err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	if err := os.MkdirAll(a, 0o755); err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	r, err := filepath.EvalSymlinks(a)
	if err != nil || !c.allowed(r) {
		return "", fmt.Errorf("%s: %s is outside the workspace", name, p)
	}
	return r, nil
}

// checkWritable verifies that the deepest existing ancestor of p resolves
// inside the allowed roots, which defeats symlinked parent directories.
func (c *Ctx) checkWritable(p string) error {
	probe := p
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	r, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return err
	}
	rest, _ := filepath.Rel(probe, p)
	if !c.allowed(filepath.Join(r, rest)) {
		return fmt.Errorf("%s is outside the workspace; outputs must be written inside %s", p, c.Workspace)
	}
	return nil
}

// Output resolves the path for a new output file.
//
// When requested is empty the file is placed next to input as
// <input name>_<suffix>.<ext>, numbered so nothing is overwritten. When a
// path is given and the file exists, overwrite must be true.
func (c *Ctx) Output(requested, input, suffix, ext string, overwrite bool) (string, error) {
	if strings.TrimSpace(requested) == "" {
		dir := c.Workspace
		base := "output"
		if input != "" {
			dir = filepath.Dir(input)
			base = strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
			if !inside(c.Workspace, dir) {
				dir = c.Workspace // inputs from scratch write their results into the workspace
			}
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		for i := 1; ; i++ {
			name := base + "_" + suffix + ext
			if i > 1 {
				name = fmt.Sprintf("%s_%s_%d%s", base, suffix, i, ext)
			}
			p := filepath.Join(dir, name)
			if _, err := os.Lstat(p); os.IsNotExist(err) {
				return p, nil
			}
		}
	}
	p := c.abs(requested)
	if filepath.Ext(p) == "" && ext != "" {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		p += ext
	}
	if err := c.checkWritable(p); err != nil {
		return "", fmt.Errorf("output: %v", err)
	}
	if st, err := os.Lstat(p); err == nil {
		if st.IsDir() {
			return "", fmt.Errorf("output: %s is a directory", requested)
		}
		if !overwrite {
			return "", fmt.Errorf("output: %s already exists; choose another name or set overwrite to true", requested)
		}
	}
	if input != "" && sameFile(p, input) {
		return "", errors.New("output: the output cannot be the same file as the input")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("output: %v", err)
	}
	return p, nil
}

func sameFile(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

// Rel renders a path relative to the workspace for messages.
func (c *Ctx) Rel(p string) string {
	if r, err := filepath.Rel(c.Workspace, p); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return p
}

// Attach adds images to the result when the model can see them.
func (c *Ctx) Attach(r *Result, paths []string, limit int) {
	if !c.Vision {
		return
	}
	for i, p := range paths {
		if i >= limit {
			break
		}
		mime := "image/jpeg"
		if strings.EqualFold(filepath.Ext(p), ".png") {
			mime = "image/png"
		}
		r.Media = append(r.Media, Media{Path: p, MIME: mime})
	}
}

// Time is a timestamp or duration given as seconds (90, 12.5) or clock time
// ("1:30", "00:01:30.250"). It accepts JSON numbers and strings.
type Time struct {
	Set     bool
	Seconds float64
}

func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	}
	v, err := ParseTime(s)
	if err != nil {
		return err
	}
	t.Set, t.Seconds = true, v
	return nil
}

// ParseTime parses seconds or [[hh:]mm:]ss[.fff].
func ParseTime(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty time")
	}
	s = strings.TrimSuffix(s, "s")
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("invalid time %q (use seconds like 90 or clock time like 1:30 or 00:01:30.5)", s)
	}
	total := 0.0
	for i, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil || v < 0 || (i > 0 && v >= 60) {
			return 0, fmt.Errorf("invalid time %q (use seconds like 90 or clock time like 1:30 or 00:01:30.5)", s)
		}
		total = total*60 + v
	}
	return total, nil
}

// Clock formats seconds as h:mm:ss.mmm (or m:ss.mmm under an hour).
func Clock(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int64(sec*1000 + 0.5)
	h := ms / 3600000
	m := ms / 60000 % 60
	s := ms / 1000 % 60
	f := ms % 1000
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d.%03d", h, m, s, f)
	}
	return fmt.Sprintf("%d:%02d.%03d", m, s, f)
}

// Size formats a byte count.
func Size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// FileSize returns a file's size, or 0.
func FileSize(p string) int64 {
	if st, err := os.Stat(p); err == nil {
		return st.Size()
	}
	return 0
}
