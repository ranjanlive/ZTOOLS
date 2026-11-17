package browsercli

// Every tool embeds this loopback broker. The first command starts a short-lived
// copy of itself; no separately installed service, native host, or registry entry.
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const bridgeAddress = "127.0.0.1:19280"
const bridgeURL = "http://" + bridgeAddress
const bridgeProtocol = 2
const maxBridgeBody = 64 << 20

var pairingPattern = regexp.MustCompile(`^[a-p]{32}\.[a-f0-9]{64}$`)

type bridgeRoute struct {
	pair    string
	replies chan json.RawMessage
}

type bridgeProfile struct {
	instance string
	seen     time.Time
	queue    []json.RawMessage
	wake     chan struct{}
	polling  bool
}

type localBridge struct {
	mu       sync.Mutex
	profiles map[string]*bridgeProfile
	routes   map[string]*bridgeRoute
	lastCall time.Time
}

func newLocalBridge() *localBridge {
	return &localBridge{profiles: map[string]*bridgeProfile{}, routes: map[string]*bridgeRoute{}, lastCall: time.Now()}
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (b *localBridge) profile(pair string) *bridgeProfile {
	p := b.profiles[pair]
	if p == nil {
		p = &bridgeProfile{wake: make(chan struct{})}
		b.profiles[pair] = p
	}
	return p
}

func (b *localBridge) enqueue(p *bridgeProfile, msg any) {
	raw, _ := json.Marshal(msg)
	p.queue = append(p.queue, raw)
	close(p.wake)
	p.wake = make(chan struct{})
}

func bridgeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (b *localBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Exact Host and Origin checks reject DNS rebinding and requests from web pages.
	if r.Host != bridgeAddress {
		http.Error(w, "invalid host", 403)
		return
	}
	origin := r.Header.Get("Origin")
	if r.URL.Path == "/health" && r.Method == "GET" && origin == "" {
		bridgeJSON(w, map[string]any{"name": "zpilot-tool-bridge", "protocol": bridgeProtocol, "version": Version})
		return
	}
	pair := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !pairingPattern.MatchString(pair) {
		http.Error(w, "invalid connection ID", 401)
		return
	}
	extOrigin := "chrome-extension://" + strings.SplitN(pair, ".", 2)[0]
	if origin != "" && origin != extOrigin {
		http.Error(w, "invalid origin", 403)
		return
	}
	isExtension := origin == extOrigin
	if r.URL.Path == "/ready" && r.Method == "GET" && !isExtension {
		b.mu.Lock()
		p := b.profiles[pair]
		online := p != nil && time.Since(p.seen) < 25*time.Second
		b.lastCall = time.Now() // A CLI waiting for its profile is still active.
		b.mu.Unlock()
		bridgeJSON(w, map[string]bool{"connected": online})
		return
	}
	if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "JSON POST required", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBridgeBody)
	switch r.URL.Path {
	case "/poll":
		if !isExtension {
			http.Error(w, "extension origin required", 403)
			return
		}
		b.poll(w, r, pair)
	case "/reply":
		if !isExtension {
			http.Error(w, "extension origin required", 403)
			return
		}
		b.reply(w, r, pair)
	case "/disconnect":
		if !isExtension {
			http.Error(w, "extension origin required", 403)
			return
		}
		var req struct {
			Instance string `json:"instance"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		b.mu.Lock()
		if p := b.profiles[pair]; p != nil && p.instance == req.Instance {
			b.failProfile(pair)
			delete(b.profiles, pair)
			close(p.wake)
		}
		b.mu.Unlock()
		bridgeJSON(w, map[string]bool{"ok": true})
	case "/call":
		if isExtension {
			http.Error(w, "CLI endpoint", 403)
			return
		}
		b.call(w, r, pair)
	default:
		http.NotFound(w, r)
	}
}

// Caller holds b.mu. A restarted or reset profile must not inherit previous work.
func (b *localBridge) failProfile(pair string) {
	for _, route := range b.routes {
		if route.pair == pair {
			msg := json.RawMessage(`{"type":"result","ok":false,"error":{"code":"EXTENSION_DISCONNECTED","message":"The extension connection was reset or restarted. Retry the command."}}`)
			select {
			case route.replies <- msg:
			default:
			}
		}
	}
}

func (b *localBridge) poll(w http.ResponseWriter, r *http.Request, pair string) {
	var req struct {
		Instance string `json:"instance"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Instance == "" {
		http.Error(w, "missing instance", 400)
		return
	}
	b.mu.Lock()
	p := b.profile(pair)
	if p.instance != "" && p.instance != req.Instance {
		if time.Since(p.seen) < 25*time.Second {
			b.mu.Unlock()
			http.Error(w, "Connection ID already active in another profile. Reset its connection ID.", 409)
			return
		}
		b.failProfile(pair)
		p.queue = nil
	}
	if p.polling {
		b.mu.Unlock()
		http.Error(w, "poll already active", 409)
		return
	}
	p.instance, p.seen, p.polling = req.Instance, time.Now(), true
	wake := p.wake
	queued := len(p.queue) > 0
	b.mu.Unlock()
	if !queued {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-wake:
		case <-timer.C:
		case <-r.Context().Done():
		}
	}
	b.mu.Lock()
	p.polling = false
	if b.profiles[pair] != p {
		b.mu.Unlock()
		http.Error(w, "connection reset", 410)
		return
	}
	messages := p.queue
	p.queue = nil
	p.seen = time.Now()
	b.mu.Unlock()
	if messages == nil {
		messages = []json.RawMessage{}
	}
	bridgeJSON(w, map[string]any{"messages": messages})
}

func (b *localBridge) reply(w http.ResponseWriter, r *http.Request, pair string) {
	var req struct {
		Instance string          `json:"instance"`
		Message  json.RawMessage `json:"message"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad reply", 400)
		return
	}
	var msg struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if json.Unmarshal(req.Message, &msg) != nil {
		http.Error(w, "bad message", 400)
		return
	}
	b.mu.Lock()
	p := b.profiles[pair]
	route := b.routes[msg.ID]
	if p == nil || p.instance != req.Instance {
		b.mu.Unlock()
		http.Error(w, "stale connection", 409)
		return
	}
	if route != nil && route.pair == pair {
		select {
		case route.replies <- req.Message:
		default:
			b.mu.Unlock()
			http.Error(w, "reply buffer full", 429)
			return
		}
	}
	b.mu.Unlock()
	bridgeJSON(w, map[string]bool{"ok": true})
}

func (b *localBridge) call(w http.ResponseWriter, r *http.Request, pair string) {
	var req struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Tool == "" {
		http.Error(w, "tool and args required", 400)
		return
	}
	id := randomID()
	route := &bridgeRoute{pair: pair, replies: make(chan json.RawMessage, 64)}
	b.mu.Lock()
	p := b.profiles[pair]
	if p == nil || time.Since(p.seen) >= 25*time.Second {
		b.mu.Unlock()
		http.Error(w, "extension offline", 503)
		return
	}
	if len(b.routes) >= 128 {
		b.mu.Unlock()
		http.Error(w, "too many requests; retry later", 429)
		return
	}
	b.routes[id] = route
	b.lastCall = time.Now()
	b.enqueue(p, map[string]any{"type": "call", "id": id, "tool": req.Tool, "args": req.Args})
	b.mu.Unlock()
	finished := false
	defer func() {
		b.mu.Lock()
		delete(b.routes, id)
		b.lastCall = time.Now()
		if !finished && b.profiles[pair] == p {
			// Remove undelivered work and cancel work already delivered.
			kept := p.queue[:0]
			for _, raw := range p.queue {
				var m struct {
					ID string `json:"id"`
				}
				_ = json.Unmarshal(raw, &m)
				if m.ID != id {
					kept = append(kept, raw)
				}
			}
			p.queue = kept
			b.enqueue(p, map[string]any{"type": "cancel", "id": id})
		}
		b.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher := http.NewResponseController(w)
	_ = flusher.Flush()
	watch := time.NewTicker(5 * time.Second)
	defer watch.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-watch.C:
			b.mu.Lock()
			online := b.profiles[pair] == p && time.Since(p.seen) < 25*time.Second
			b.mu.Unlock()
			if !online {
				_ = json.NewEncoder(w).Encode(map[string]any{"type": "result", "ok": false, "error": &ToolError{Code: "EXTENSION_DISCONNECTED", Message: "The selected browser profile disconnected. Retry the command."}})
				finished = true
				return
			}
		case msg := <-route.replies:
			if _, err := w.Write(append(msg, '\n')); err != nil {
				return
			}
			if err := flusher.Flush(); err != nil {
				return
			}
			var meta struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(msg, &meta)
			if meta.Type == "result" {
				finished = true
				return
			}
		}
	}
}

func runEmbeddedBridge() int {
	ln, err := net.Listen("tcp4", bridgeAddress)
	if err != nil {
		return 0
	} // Another tool won the startup race.
	b := newLocalBridge()
	srv := &http.Server{Handler: b, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			b.mu.Lock()
			idle := len(b.routes) == 0 && time.Since(b.lastCall) > 60*time.Second
			b.mu.Unlock()
			if idle {
				_ = srv.Close()
				return
			}
		}
	}()
	_ = srv.Serve(ln)
	return 0
}

func bridgeHTTPClient() *http.Client {
	// Ignore HTTP_PROXY for private browser traffic.
	return &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 5 * time.Second}}
}

// Tests replace the client factory to exercise the same HTTP protocol on an
// isolated test listener, without starting a subprocess or using a fixed port.
var newBridgeHTTPClient = bridgeHTTPClient

func bridgeHealthy(ctx context.Context, client *http.Client) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", bridgeURL+"/health", nil)
	resp, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	var info struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
		Version  string `json:"version"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&info) != nil || info.Name != "zpilot-tool-bridge" || info.Protocol != bridgeProtocol || info.Version != Version {
		return false, fmt.Errorf("localhost port 19280 is occupied by another application or an incompatible tool version; close the older tool bridge and retry")
	}
	return true, nil
}

func ensureBridge(ctx context.Context, client *http.Client) error {
	if ok, err := bridgeHealthy(ctx, client); ok || err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--internal-bridge")
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start built-in connection: %w", err)
	}
	go cmd.Wait()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("could not start built-in connection on %s", bridgeAddress)
		case <-tick.C:
			if ok, err := bridgeHealthy(ctx, client); ok || err != nil {
				return err
			}
		}
	}
}

func (c *Client) connectDirect(ctx context.Context) (*http.Client, error) {
	if !pairingPattern.MatchString(c.Config.ExtensionID) {
		return nil, fmt.Errorf("set extension_id in config.json: open the ZPilot Browser extension in your intended profile and copy its Connection ID (not the ID from chrome://extensions)")
	}
	client := newBridgeHTTPClient()
	if err := ensureBridge(ctx, client); err != nil {
		return nil, err
	}
	ready := func() bool {
		req, _ := http.NewRequestWithContext(ctx, "GET", bridgeURL+"/ready", nil)
		req.Header.Set("Authorization", "Bearer "+c.Config.ExtensionID)
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var state struct {
			Connected bool `json:"connected"`
		}
		return json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&state) == nil && state.Connected
	}
	if ready() {
		return client, nil
	}
	if c.Config.AutoLaunch {
		if err := c.launchBrowser(true); err != nil {
			return nil, err
		}
	}
	if c.Progress != nil {
		fmt.Fprintln(c.Progress, "Waiting for the selected extension profile…")
	}
	timer := time.NewTimer(time.Duration(c.Config.ConnectTimeout) * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, fmt.Errorf("extension did not connect within %ds: open the extension in profile %q and check extension_id; after Reset ID copy the new Connection ID into this config", c.Config.ConnectTimeout, c.Config.ChromeProfile)
		case <-tick.C:
			if ready() {
				return client, nil
			}
		}
	}
}

func (c *Client) launchBrowser(wake bool) error {
	path, err := findBrowser(c.Config)
	if err != nil {
		return err
	}
	args := []string{"--profile-directory=" + c.Config.ChromeProfile}
	if c.Config.UserDataDir != "" {
		args = append(args, "--user-data-dir="+c.Config.UserDataDir)
	}
	args = append(args, c.Config.BrowserArgs...)
	if wake {
		args = append(args, "chrome-extension://"+strings.SplitN(c.Config.ExtensionID, ".", 2)[0]+"/ui/connect.html")
	}
	cmd := exec.Command(path, args...)
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start browser: %w", err)
	}
	go cmd.Wait()
	return nil
}
