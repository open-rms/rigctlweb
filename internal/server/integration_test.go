package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/open-rms/rigctlweb/internal/config"
	"github.com/open-rms/rigctlweb/internal/integration"
	"github.com/open-rms/rigctlweb/internal/rig"
)

// A trimmed copy of SvelteKit's fallback page: root-absolute URLs and an empty runtime base.
const fallbackPage = `<html><head><link rel="icon" href="/favicon.svg" /><link href="/_app/immutable/entry/start.js" rel="modulepreload"></head>` +
	`<body><script>{ __sveltekit_x = { base: "", version: "1" }; import("/_app/immutable/entry/start.js"); }</script></body></html>`

const base = "/integration/rigctlweb"

func newMountedServer(t *testing.T, secret string, rigs ...rig.Config) *httptest.Server {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := rig.NewManager(ctx, slog.Default())
	for _, rc := range rigs {
		if err := mgr.Add(rc); err != nil {
			t.Fatal(err)
		}
	}
	ui := fstest.MapFS{
		"index.html":                    {Data: []byte(fallbackPage)},
		"_app/immutable/entry/start.js": {Data: []byte("export {}")},
	}
	srv := New(store, mgr, ui, "1.2.3", slog.Default())
	srv.SetBasePath(base)
	if secret != "" {
		srv.SetIntegration(secret)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// noRedirects matches hamstack's prober, which never follows redirects.
var noRedirects = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func TestHamstackHandshake(t *testing.T) {
	secret := strings.Repeat("5e", 32)
	ts := newMountedServer(t, secret,
		rig.Config{ID: "ftdx10", Name: "ftdx10", Addr: "127.0.0.1:1", ReadOnly: true},
		rig.Config{ID: "ic705", Name: "ic705", Addr: "127.0.0.1:2"})
	get := func(path, sent string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("User-Agent", "hamstack")
		if sent != "" {
			req.Header.Set(integration.ProxyHeader, sent)
		}
		res, err := noRedirects.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}

	// Step 1: health, under the full prefix, without the secret, answered directly.
	if res := get(base+"/api/health", ""); res.StatusCode != 200 {
		t.Fatalf("health = %d", res.StatusCode)
	}
	// A wrong or missing secret must be 401/403 so hamstack shows "refused".
	for _, sent := range []string{"", "nope", secret[:len(secret)-1]} {
		if res := get(base+"/api/v1/integration", sent); res.StatusCode != 403 {
			t.Fatalf("handshake with secret %q = %d, want 403", sent, res.StatusCode)
		}
	}
	// Step 2: the handshake with the secret.
	res := get(base+"/api/v1/integration", secret)
	if res.StatusCode != 200 {
		t.Fatalf("handshake = %d", res.StatusCode)
	}
	var rep struct {
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		API          []string `json:"api"`
		Mode         string   `json:"mode"`
		Capabilities []string `json:"capabilities"`
		Transmits    bool     `json:"transmits"`
		Rigs         []struct {
			ID       string `json:"id"`
			ReadOnly bool   `json:"readOnly"`
		} `json:"rigs"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&rep); err != nil {
		t.Fatal(err)
	}
	if rep.Name != "rigctlweb" || rep.Version != "1.2.3" || rep.Mode != "hamstack" || len(rep.API) != 1 || rep.API[0] != "v1" ||
		!rep.Transmits || len(rep.Rigs) != 2 || len(rep.Capabilities) == 0 {
		t.Fatalf("handshake = %+v", rep)
	}
	ro := map[string]bool{}
	for _, r := range rep.Rigs {
		ro[r.ID] = r.ReadOnly
	}
	if !ro["ftdx10"] || ro["ic705"] {
		t.Fatalf("readOnly flags = %v", ro)
	}
}

func TestIntegrationModeLocksDown(t *testing.T) {
	secret := strings.Repeat("5e", 32)
	ts := newMountedServer(t, secret)
	do := func(method, path, body string, withSecret bool) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		if withSecret {
			req.Header.Set(integration.ProxyHeader, secret)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := noRedirects.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	// Everything but health needs the secret: the page, assets, API and both WebSockets.
	for _, p := range []string{"/", "/settings", "/_app/immutable/entry/start.js", "/api/rigs", "/api/session", "/ws?rig=x", "/ws/audio?rig=x"} {
		if res := do("GET", base+p, "", false); res.StatusCode != 403 {
			t.Errorf("%s without the secret = %d, want 403", p, res.StatusCode)
		}
	}
	// The token is not a way in when hamstack owns sign-in.
	if res := do("POST", base+"/api/login", `{"token":"x"}`, true); res.StatusCode != 404 {
		t.Errorf("login = %d, want 404", res.StatusCode)
	}
	// hamstack owns the radio list.
	if res := do("POST", base+"/api/rigs", `{"name":"x","addr":"127.0.0.1:1"}`, true); res.StatusCode != 403 {
		t.Errorf("add rig = %d, want 403", res.StatusCode)
	}
	if res := do("DELETE", base+"/api/rigs/x", "", true); res.StatusCode != 403 {
		t.Errorf("delete rig = %d, want 403", res.StatusCode)
	}
	// The UI is told it is inside hamstack.
	var sess map[string]any
	_ = json.NewDecoder(do("GET", base+"/api/session", "", true).Body).Decode(&sess)
	if sess["authenticated"] != true || sess["mode"] != "hamstack" {
		t.Errorf("session = %v", sess)
	}
	// hamstack's own pages may frame it.
	res := do("GET", base+"/", "", true)
	if !strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-ancestors 'self'") || res.Header.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("frame headers: %q / %q", res.Header.Get("Content-Security-Policy"), res.Header.Get("X-Frame-Options"))
	}
}

func TestBasePathServesRebasedPage(t *testing.T) {
	ts := newMountedServer(t, "")
	for _, p := range []string{"/", "/settings", "/integrations"} {
		res, err := noRedirects.Get(ts.URL + base + p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		page := string(b)
		if res.StatusCode != 200 || !strings.Contains(page, `href="`+base+`/favicon.svg"`) ||
			!strings.Contains(page, `import("`+base+`/_app/`) || !strings.Contains(page, `base: "`+base+`"`) ||
			strings.Contains(page, `href="/_app`) {
			t.Fatalf("%s = %d %s", p, res.StatusCode, page)
		}
		// The CSP must allow exactly the rewritten inline script.
		m := inlineScript.FindStringSubmatch(page)
		sum := sha256.Sum256([]byte(m[1]))
		if want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"; !strings.Contains(res.Header.Get("Content-Security-Policy"), want) {
			t.Fatalf("CSP %q lacks the rewritten script's hash %s", res.Header.Get("Content-Security-Policy"), want)
		}
		if !strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") || res.Header.Get("X-Frame-Options") != "DENY" {
			t.Fatal("standalone must not be frameable")
		}
	}
	// Assets resolve under the prefix; nothing is served outside it.
	if res, _ := noRedirects.Get(ts.URL + base + "/_app/immutable/entry/start.js"); res.StatusCode != 200 {
		t.Errorf("asset = %d", res.StatusCode)
	}
	if res, _ := noRedirects.Get(ts.URL + "/api/health"); res.StatusCode != 404 {
		t.Errorf("outside the prefix = %d, want 404", res.StatusCode)
	}
	if res, _ := noRedirects.Get(ts.URL + base); res.StatusCode != 301 || res.Header.Get("Location") != base+"/" {
		t.Errorf("bare prefix = %d %q", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestSessionCookieScopedToBasePath(t *testing.T) {
	ts := newMountedServer(t, "")
	res, err := noRedirects.Post(ts.URL+base+"/api/logout", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	cs := res.Cookies()
	if len(cs) != 1 || cs[0].Name != cookieName || cs[0].Path != base+"/" {
		t.Fatalf("cookies = %+v, want %s scoped to %s/", cs, cookieName, base)
	}
}
