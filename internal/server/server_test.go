package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/open-rms/rigctlweb/internal/audio"
	"github.com/open-rms/rigctlweb/internal/config"
	"github.com/open-rms/rigctlweb/internal/rig"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ui := fstest.MapFS{"index.html": {Data: []byte(`<html><script>boot()</script></html>`)}}
	srv := New(store, rig.NewManager(ctx, slog.Default()), ui, "test", slog.Default())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, store.Get().Token
}

func TestAuthRequired(t *testing.T) {
	ts, token := newTestServer(t)

	res, _ := http.Get(ts.URL + "/api/rigs")
	if res.StatusCode != 401 {
		t.Fatalf("unauthenticated /api/rigs = %d, want 401", res.StatusCode)
	}
	res, _ = http.Get(ts.URL + "/ws?rig=x")
	if res.StatusCode != 401 {
		t.Fatalf("unauthenticated /ws = %d, want 401", res.StatusCode)
	}

	req, _ := http.NewRequest("GET", ts.URL+"/api/rigs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if res, _ = http.DefaultClient.Do(req); res.StatusCode != 200 {
		t.Fatalf("bearer /api/rigs = %d, want 200", res.StatusCode)
	}
	req.Header.Set("Authorization", "Bearer wrong")
	if res, _ = http.DefaultClient.Do(req); res.StatusCode != 401 {
		t.Fatalf("wrong bearer = %d, want 401", res.StatusCode)
	}
}

func TestLoginSetsStrictCookieAndRateLimits(t *testing.T) {
	ts, token := newTestServer(t)
	post := func(tok string) *http.Response {
		res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"token":"`+tok+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := post(token)
	if res.StatusCode != 200 {
		t.Fatalf("good login = %d", res.StatusCode)
	}
	var c *http.Cookie
	for _, k := range res.Cookies() {
		if k.Name == cookieName {
			c = k
		}
	}
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Value == token {
		t.Fatalf("bad session cookie: %+v", c)
	}
	// The cookie alone must authenticate.
	req, _ := http.NewRequest("GET", ts.URL+"/api/rigs", nil)
	req.AddCookie(c)
	if r2, _ := http.DefaultClient.Do(req); r2.StatusCode != 200 {
		t.Fatalf("cookie auth = %d", r2.StatusCode)
	}
	// Brute force is throttled (8 attempts/min already used 1).
	var limited bool
	for i := 0; i < 10; i++ {
		if post("nope").StatusCode == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("expected 429 after repeated failures")
	}
}

func TestCSPHashesInlineBootstrapScript(t *testing.T) {
	ts, _ := newTestServer(t)
	res, _ := http.Get(ts.URL + "/")
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' 'sha256-") || strings.Contains(csp, "unsafe-inline'; img") {
		t.Fatalf("csp = %q", csp)
	}
}

func TestAddRigValidation(t *testing.T) {
	ts, token := newTestServer(t)
	for _, body := range []string{
		`{"name":"x","addr":"not-an-address"}`,
		`{"name":"","addr":"127.0.0.1:4532"}`,
		`{"name":"x","addr":"127.0.0.1:4532; rm -rf /"}`,
	} {
		req, _ := http.NewRequest("POST", ts.URL+"/api/rigs", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if res, _ := http.DefaultClient.Do(req); res.StatusCode != 400 {
			t.Fatalf("%s = %d, want 400", body, res.StatusCode)
		}
	}
}

func TestDevNoAuth(t *testing.T) {
	store, _ := config.Open(filepath.Join(t.TempDir(), "config.toml"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := New(store, rig.NewManager(ctx, slog.Default()), fstest.MapFS{"index.html": {Data: []byte("x")}}, "t", slog.Default())
	srv.DisableAuth()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	if res, _ := http.Get(ts.URL + "/api/rigs"); res.StatusCode != 200 {
		t.Fatalf("no-auth /api/rigs = %d, want 200", res.StatusCode)
	}
}

func TestIsLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8480": true, "[::1]:8480": true, "localhost:8480": true,
		"0.0.0.0:8480": false, ":8480": false, "10.0.0.5:8480": false,
	} {
		if IsLoopback(addr) != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", addr, !want, want)
		}
	}
}

func TestViewsRoundTripAndValidation(t *testing.T) {
	ts, token := newTestServer(t)
	do := func(method, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+"/api/views", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	good := `[{"id":"mine","name":"My view","controls":["freq","lvl:AF","fn:NB"]},{"id":"essentials","name":"Essentials","builtin":true,"controls":["freq"]}]`
	if res := do("PUT", good); res.StatusCode != 200 {
		t.Fatalf("PUT good = %d", res.StatusCode)
	}
	res := do("GET", "")
	var got []config.ViewDef
	_ = json.NewDecoder(res.Body).Decode(&got)
	if len(got) != 2 || got[0].ID != "mine" || len(got[0].Controls) != 3 || !got[1].Builtin {
		t.Fatalf("round trip = %+v", got)
	}
	for name, body := range map[string]string{
		"bad id":      `[{"id":"Bad ID","name":"x","controls":[]}]`,
		"empty name":  `[{"id":"a","name":" ","controls":[]}]`,
		"duplicate":   `[{"id":"a","name":"x","controls":[]},{"id":"a","name":"y","controls":[]}]`,
		"bad control": `[{"id":"a","name":"x","controls":["ok","bad control; rm"]}]`,
		"long name":   `[{"id":"a","name":"` + strings.Repeat("n", 41) + `","controls":[]}]`,
	} {
		if res := do("PUT", body); res.StatusCode != 400 {
			t.Errorf("%s = %d, want 400", name, res.StatusCode)
		}
	}
	if res := do("GET", ""); res.StatusCode != 200 {
		t.Fatal("GET after rejected PUTs failed")
	}
}

func TestAudioEndpointsAreGuarded(t *testing.T) {
	ts, token := newTestServer(t)
	do := func(method, path, body string, auth bool) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	// Never reachable without the token.
	for _, p := range []string{"/ws/audio?rig=x", "/api/rigs/x/audio"} {
		if got := do("GET", p, "", false).StatusCode; got != 401 {
			t.Errorf("%s unauthenticated = %d, want 401", p, got)
		}
	}
	// Unknown rig.
	if got := do("GET", "/api/rigs/nope/audio", "", true).StatusCode; got != 404 {
		t.Errorf("unknown rig = %d, want 404", got)
	}

	if got := do("POST", "/api/rigs", `{"name":"R","addr":"127.0.0.1:1"}`, true).StatusCode; got != 200 && got != 201 {
		t.Fatalf("add rig = %d", got)
	}
	res, _ := http.DefaultClient.Do(func() *http.Request {
		req, _ := http.NewRequest("GET", ts.URL+"/api/rigs", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}())
	var rigs []struct{ ID string }
	_ = json.NewDecoder(res.Body).Decode(&rigs)
	res.Body.Close()
	if len(rigs) != 1 {
		t.Fatalf("rigs: %+v", rigs)
	}
	id := rigs[0].ID

	// Off by default: the stream is refused before anything is captured.
	if got := do("GET", "/ws/audio?rig="+id, "", true).StatusCode; got != 403 {
		t.Errorf("audio off = %d, want 403", got)
	}
	// Bad settings are rejected.
	if got := do("PUT", "/api/rigs/"+id+"/audio", `{"enabled":false,"channel":"both"}`, true).StatusCode; got != 400 {
		t.Errorf("bad channel = %d, want 400", got)
	}
	if got := do("PUT", "/api/rigs/"+id+"/audio", `{"enabled":false,"channel":"mix","device":"--evil;rm -rf"}`, true).StatusCode; got != 400 {
		t.Errorf("unknown device = %d, want 400", got)
	}
	// Valid settings with audio left off are accepted and round-trip.
	if got := do("PUT", "/api/rigs/"+id+"/audio", `{"enabled":false,"channel":"left"}`, true).StatusCode; got != 200 {
		t.Errorf("valid settings = %d, want 200", got)
	}
}

type fakeMixer struct {
	vol  float64
	sets []float64
}

func (f *fakeMixer) Get(context.Context, string, string) (float64, error) { return f.vol, nil }
func (f *fakeMixer) Set(_ context.Context, _, _ string, v float64) error {
	f.vol = v
	f.sets = append(f.sets, v)
	return nil
}

func TestAudioInputVolumeRemembersAndRestoresOriginal(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ui := fstest.MapFS{"index.html": {Data: []byte(`<html></html>`)}}
	srv := New(store, rig.NewManager(ctx, slog.Default()), ui, "test", slog.Default())
	fm := &fakeMixer{vol: 1.0}
	srv.audio.mixer = fm
	canned := audio.Report{Method: audio.MethodPulse, Devices: []audio.Device{{ID: "dev", Name: "Dev"}}}
	srv.audio.detect = func(context.Context) audio.Report { return canned }
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	token := store.Get().Token

	call := func(method, path, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		return res.StatusCode, m
	}
	if code, _ := call("POST", "/api/rigs", `{"name":"R","addr":"127.0.0.1:1"}`); code != 201 {
		t.Fatalf("add rig %d", code)
	}
	in := "/api/rigs/r/audio/input"

	if code, m := call("GET", in, ""); code != 200 || m["supported"] != true || m["volume"] != 1.0 || m["original"] != nil {
		t.Fatalf("initial: %d %v", code, m)
	}
	// Out-of-range volumes are refused and change nothing.
	for _, body := range []string{`{"volume":1.5}`, `{"volume":0.0}`, `{}`} {
		if code, _ := call("PUT", in, body); code != 400 {
			t.Errorf("%s = %d, want 400", body, code)
		}
	}
	if len(fm.sets) != 0 {
		t.Fatalf("refused requests must not touch the mixer: %v", fm.sets)
	}
	// First change records the original (100%); later changes keep it.
	if code, m := call("PUT", in, `{"volume":0.5}`); code != 200 || m["volume"] != 0.5 || m["original"] != 1.0 {
		t.Fatalf("first change: %d %v", code, m)
	}
	if code, m := call("PUT", in, `{"volume":0.3}`); code != 200 || m["original"] != 1.0 {
		t.Fatalf("second change: %d %v", code, m)
	}
	// Saving audio settings must not erase it.
	if code, _ := call("PUT", "/api/rigs/r/audio", `{"enabled":false,"channel":"mix"}`); code != 200 {
		t.Fatalf("save settings %d", code)
	}
	if _, m := call("GET", in, ""); m["original"] != 1.0 {
		t.Fatalf("original lost after saving settings: %v", m)
	}
	// Restore puts it back and forgets it.
	if code, m := call("PUT", in, `{"restore":true}`); code != 200 || m["volume"] != 1.0 || m["original"] != nil {
		t.Fatalf("restore: %d %v", code, m)
	}
	if code, _ := call("PUT", in, `{"restore":true}`); code != 409 {
		t.Errorf("second restore = %d, want 409", code)
	}
}

// A browser that keyed the radio and then vanishes (tab closed, network lost) must not
// leave it transmitting; a browser that never keyed must not unkey anything.
func TestWebSocketDisconnectUnkeysItsOwnKey(t *testing.T) {
	bin, err := exec.LookPath("rigctld")
	if err != nil {
		t.Skip("rigctld not installed")
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	cmd := exec.Command(bin, "-m", "1", "-t", strconv.Itoa(port), "-T", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	ts, token := newTestServer(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/rigs", strings.NewReader(`{"name":"D","addr":"`+addr+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != 201 {
		t.Fatalf("add rig: %v %v", res, err)
	}

	ext := rigctl.NewClient(addr)
	t.Cleanup(func() { _ = ext.Close() })
	waitUp := func(want bool, what string) {
		t.Helper()
		for end := time.Now().Add(8 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
			if on, err := ext.GetPTT(context.Background()); err == nil && on == want {
				return
			}
		}
		t.Fatalf("timed out: %s", what)
	}

	dial := func() *websocket.Conn {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws?rig=d", &websocket.DialOptions{
			HTTPHeader: http.Header{"Authorization": {"Bearer " + token}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	// Capabilities must be probed before commands are accepted, so retry until the key-up works.
	var a *websocket.Conn
	for i := 0; i < 50; i++ { // retry until the rig has probed capabilities
		a = dial()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = wsjson.Write(ctx, a, map[string]any{"id": 1, "cmd": "set_ptt", "args": map[string]any{"on": true}})
		var out struct {
			Type string `json:"type"`
			OK   bool   `json:"ok"`
		}
		for {
			if err := wsjson.Read(ctx, a, &out); err != nil {
				break
			}
			if out.Type == "result" {
				break
			}
		}
		cancel()
		if out.OK {
			break
		}
		a.CloseNow()
		time.Sleep(200 * time.Millisecond)
	}
	waitUp(true, "browser A to key the radio")

	// Browser B connects, never keys, and leaves: the radio stays keyed.
	b := dial()
	b.CloseNow()
	time.Sleep(700 * time.Millisecond)
	if on, _ := ext.GetPTT(context.Background()); !on {
		t.Fatal("an unrelated browser leaving unkeyed the radio")
	}

	// Browser A, which keyed it, drops: the radio is released.
	a.CloseNow()
	waitUp(false, "the radio to be unkeyed after the keying browser disconnected")
}

// Building from source without the web step leaves the embedded UI empty. The server must
// say so, not list the directory.
func TestMissingWebUIGivesAClearMessage(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	empty := fstest.MapFS{".gitkeep": {Data: nil}}
	ts := httptest.NewServer(New(store, rig.NewManager(ctx, slog.Default()), empty, "test", slog.Default()).Handler())
	t.Cleanup(ts.Close)

	for _, path := range []string{"/", "/settings", "/.gitkeep"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 400)
		n, _ := res.Body.Read(body)
		res.Body.Close()
		if res.StatusCode != 503 || !strings.Contains(string(body[:n]), "built without its web interface") {
			t.Errorf("%s = %d %q", path, res.StatusCode, body[:n])
		}
	}
	if res, _ := http.Get(ts.URL + "/api/health"); res == nil || res.StatusCode != 200 {
		t.Error("the API must keep working without the UI")
	}
}
