package integration

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeBasePath(t *testing.T) {
	ok := map[string]string{
		"":                        "",
		"/":                       "",
		"/integration/rigctlweb":  "/integration/rigctlweb",
		"/integration/rigctlweb/": "/integration/rigctlweb",
		"rigctlweb":               "/rigctlweb",
		" /a/b-c_d.e~ ":           "/a/b-c_d.e~",
	}
	for in, want := range ok {
		got, err := NormalizeBasePath(in)
		if err != nil || got != want {
			t.Errorf("NormalizeBasePath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"/a//b", "/a/../b", "/./a", "/a b", `/a"b`, "/a?b", "/a#b", "/a%2fb", "//evil.example"} {
		if got, err := NormalizeBasePath(in); err == nil {
			t.Errorf("NormalizeBasePath(%q) = %q, want an error", in, got)
		}
	}
}

func TestMountAt(t *testing.T) {
	h := MountAt("/base", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	cases := []struct {
		path, want string
		code       int
	}{
		{"/base/api/health", "/api/health", 200},
		{"/base/", "/", 200},
		{"/base", "", 301},
		{"/api/health", "", 404},
		{"/basement/x", "", 404},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.code || (c.code == 200 && rec.Body.String() != c.want) {
			t.Errorf("%s = %d %q, want %d %q", c.path, rec.Code, rec.Body.String(), c.code, c.want)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/base?x=1", nil))
	if loc := rec.Header().Get("Location"); loc != "/base/?x=1" {
		t.Errorf("redirect to %q", loc)
	}
}

func TestRequireSecret(t *testing.T) {
	secret := strings.Repeat("a", 64)
	h := RequireSecret(secret, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	do := func(method, path, sent string) int {
		req := httptest.NewRequest(method, path, nil)
		if sent != "" {
			req.Header.Set(ProxyHeader, sent)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if do("GET", "/api/health", "") != 200 {
		t.Error("health must not need the secret")
	}
	if do("POST", "/api/health", "") != 403 {
		t.Error("only GET/HEAD health is open")
	}
	for _, p := range []string{"/", "/api/v1/integration", "/ws", "/_app/x.js", "/api/healthz"} {
		if do("GET", p, "") != 403 || do("GET", p, secret[:63]) != 403 || do("GET", p, secret+"x") != 403 {
			t.Errorf("%s must refuse a missing or wrong secret", p)
		}
		if do("GET", p, secret) != 200 {
			t.Errorf("%s must accept the secret", p)
		}
	}
}

func TestReadSecret(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "s")
	_ = os.WriteFile(good, []byte(strings.Repeat("ab", 32)+"\n"), 0o600)
	if s, err := ReadSecret(good); err != nil || s != strings.Repeat("ab", 32) {
		t.Fatalf("ReadSecret = %q, %v", s, err)
	}
	open := filepath.Join(dir, "open")
	_ = os.WriteFile(open, []byte(strings.Repeat("ab", 32)), 0o644)
	if _, err := ReadSecret(open); err == nil {
		t.Error("a secret readable by others must be refused")
	}
	short := filepath.Join(dir, "short")
	_ = os.WriteFile(short, []byte("abc\n"), 0o600)
	if _, err := ReadSecret(short); err == nil {
		t.Error("a short secret must be refused")
	}
	for _, p := range []string{"", filepath.Join(dir, "missing"), dir} {
		if _, err := ReadSecret(p); err == nil {
			t.Errorf("ReadSecret(%q) must fail", p)
		}
	}
}

func TestParseRigs(t *testing.T) {
	// The exact form hamstack writes (cmd/hamstack/integration.go, integrationRigs).
	got, err := ParseRigs("ftdx10=127.0.0.1:4532,ro;ic705=127.0.0.1:4533")
	if err != nil || len(got) != 2 ||
		got[0] != (Rig{ID: "ftdx10", Addr: "127.0.0.1:4532", ReadOnly: true}) ||
		got[1] != (Rig{ID: "ic705", Addr: "127.0.0.1:4533"}) {
		t.Fatalf("ParseRigs = %+v, %v", got, err)
	}
	if got, err := ParseRigs(""); err != nil || len(got) != 0 {
		t.Errorf("empty = %+v, %v", got, err)
	}
	for _, bad := range []string{
		"a=127.0.0.1:1,rw", // unknown option: never guess whether it may transmit
		"a=127.0.0.1:1,RO", // case matters
		"a=127.0.0.1",      // no port
		"a=127.0.0.1:0",    // bad port
		"=127.0.0.1:1",     // no id
		"A=127.0.0.1:1",    // id form
		"a=127.0.0.1:1;a=127.0.0.1:2",
		"a127.0.0.1:1",
	} {
		if _, err := ParseRigs(bad); err == nil {
			t.Errorf("ParseRigs(%q) must fail", bad)
		}
	}
}

func TestFromEnv(t *testing.T) {
	if _, ok := FromEnv(func(string) string { return "" }); ok {
		t.Error("integration mode must be off without HAMSTACK_INTEGRATION_ID")
	}
	env := map[string]string{"HAMSTACK_INTEGRATION_ID": "rigctlweb", "HAMSTACK_BASE_PATH": "/integration/rigctlweb"}
	e, ok := FromEnv(func(k string) string { return env[k] })
	if !ok || e.ID != "rigctlweb" || e.BasePath != "/integration/rigctlweb" {
		t.Errorf("FromEnv = %+v, %v", e, ok)
	}
}
