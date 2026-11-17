package browsercli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testPair = strings.Repeat("a", 32) + "." + strings.Repeat("b", 64)

func bridgeRequest(t *testing.T, ctx context.Context, url, path, pair, origin string, body any) *http.Response {
	t.Helper()
	method := "POST"
	var data []byte
	if body == nil {
		method = "GET"
	} else {
		data, _ = json.Marshal(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = bridgeAddress
	req.Header.Set("Content-Type", "application/json")
	if pair != "" {
		req.Header.Set("Authorization", "Bearer "+pair)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func waitBridge(t *testing.T, condition func() bool) {
	t.Helper()
	limit := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(limit) {
			t.Fatal("bridge condition timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDirectBridgeRoutesAndIsolatesProfiles(t *testing.T) {
	b := newLocalBridge()
	s := httptest.NewServer(b)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	origin := "chrome-extension://" + strings.Repeat("a", 32)
	pollDone := make(chan map[string]any, 1)
	go func() {
		resp := bridgeRequest(t, ctx, s.URL, "/poll", testPair, origin, map[string]string{"instance": "profile-one"})
		defer resp.Body.Close()
		var reply map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&reply)
		pollDone <- reply
	}()
	waitBridge(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.profiles[testPair] != nil })
	wrongPair := strings.Repeat("a", 32) + "." + strings.Repeat("c", 64)
	wrong := bridgeRequest(t, ctx, s.URL, "/call", wrongPair, "", map[string]any{"tool": "web_search", "args": map[string]any{}})
	wrong.Body.Close()
	if wrong.StatusCode != 503 {
		t.Fatalf("unpaired profile accepted: %d", wrong.StatusCode)
	}
	resp := bridgeRequest(t, ctx, s.URL, "/call", testPair, "", map[string]any{"tool": "web_search", "args": map[string]any{"query": "hello"}})
	defer resp.Body.Close()
	polled := <-pollDone
	messages := polled["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("poll: %v", polled)
	}
	call := messages[0].(map[string]any)
	if call["tool"] != "web_search" {
		t.Fatalf("wrong call: %v", call)
	}
	for _, msg := range []map[string]any{
		{"type": "progress", "id": call["id"], "message": "working"},
		{"type": "result", "id": call["id"], "ok": true, "result": map[string]any{"answer": "done"}},
	} {
		r := bridgeRequest(t, ctx, s.URL, "/reply", testPair, origin, map[string]any{"instance": "profile-one", "message": msg})
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatalf("reply: %d", r.StatusCode)
		}
	}
	decoder := json.NewDecoder(resp.Body)
	var progress, result map[string]any
	if decoder.Decode(&progress) != nil || decoder.Decode(&result) != nil || progress["message"] != "working" || result["ok"] != true {
		t.Fatalf("stream: %v %v", progress, result)
	}
	waitBridge(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return len(b.routes) == 0 })
}

func TestDirectBridgeCancellationAndReset(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "reset"}[reset], func(t *testing.T) {
			b := newLocalBridge()
			b.profiles[testPair] = &bridgeProfile{instance: "one", seen: time.Now(), wake: make(chan struct{})}
			s := httptest.NewServer(b)
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp := bridgeRequest(t, ctx, s.URL, "/call", testPair, "", map[string]any{"tool": "generate_image", "args": map[string]any{}})
			if reset {
				r := bridgeRequest(t, ctx, s.URL, "/disconnect", testPair, "chrome-extension://"+strings.Repeat("a", 32), map[string]string{"instance": "one"})
				r.Body.Close()
				var failure struct {
					Error *ToolError `json:"error"`
				}
				if json.NewDecoder(resp.Body).Decode(&failure) != nil || failure.Error == nil || failure.Error.Code != "EXTENSION_DISCONNECTED" {
					t.Fatalf("reset did not fail caller: %+v", failure)
				}
			}
			resp.Body.Close()
			waitBridge(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return len(b.routes) == 0 })
			b.mu.Lock()
			defer b.mu.Unlock()
			if reset {
				if b.profiles[testPair] != nil {
					t.Fatal("reset ID is still connected")
				}
			} else {
				for _, raw := range b.profiles[testPair].queue {
					var msg map[string]any
					_ = json.Unmarshal(raw, &msg)
					if msg["type"] == "call" {
						t.Fatal("cancelled request still queued")
					}
				}
			}
		})
	}
}

func TestDirectBridgeRejectsWebsitesAndDuplicateProfiles(t *testing.T) {
	b := newLocalBridge()
	b.profiles[testPair] = &bridgeProfile{instance: "first", seen: time.Now(), wake: make(chan struct{})}
	for _, tc := range []struct {
		path, origin, host, pair, instance string
		code                               int
	}{
		{"/poll", "https://evil.example", bridgeAddress, testPair, "first", 403},
		{"/poll", "", bridgeAddress, testPair, "first", 403},
		{"/poll", "chrome-extension://" + strings.Repeat("a", 32), "evil.example:19280", testPair, "first", 403},
		{"/poll", "chrome-extension://" + strings.Repeat("a", 32), bridgeAddress, "bad", "first", 401},
		{"/poll", "chrome-extension://" + strings.Repeat("a", 32), bridgeAddress, testPair, "second", 409},
		{"/call", "chrome-extension://" + strings.Repeat("a", 32), bridgeAddress, testPair, "first", 403},
	} {
		payload, _ := json.Marshal(map[string]string{"instance": tc.instance})
		r := httptest.NewRequest("POST", "http://"+tc.host+tc.path, bytes.NewReader(payload))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+tc.pair)
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s %s: got %d want %d (%s)", tc.path, tc.origin, w.Code, tc.code, w.Body.String())
		}
	}
}

func TestSimpleConfigAndMissingConnectionID(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if err := initConfig(path); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path, true)
	if err != nil || c.ChromeProfile != "Default" || c.ExtensionID != "" {
		t.Fatalf("config: %+v %v", c, err)
	}
	_, err = (&Client{Config: c, Progress: io.Discard}).Call(context.Background(), "web_search", map[string]any{"query": "hello"})
	if err == nil || !strings.Contains(err.Error(), "Connection ID") {
		t.Fatalf("missing ID: %v", err)
	}
}
