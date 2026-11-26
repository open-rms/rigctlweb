// Package integration holds what a program needs to run as a hamstack integration: the base path
// it is served under, the proxy secret check, the radios it is given, and the handshake answer.
//
// It imports only the standard library and nothing from the host program, so it can move to a
// shared library unchanged (planned as hamgo/integration).
//
// Contract (hamstack docs/integrations.md): hamstack starts the program with
//
//	HAMSTACK_INTEGRATION_ID     its id; integration mode is on when this is set
//	HAMSTACK_BASE_PATH          /integration/<id>  (also passed as -base-path)
//	HAMSTACK_RIGS               id=host:port[,ro] entries separated by ';'
//	HAMSTACK_DATA_DIR           its data folder (keying markers go in state/)
//	HAMSTACK_CONFIG_DIR         its settings folder
//	HAMSTACK_PROXY_SECRET_FILE  path of a 0600 file holding the proxy secret
//
// and its proxy adds X-Hamstack-Proxy-Auth: <secret> to every request it forwards.
package integration

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ProxyHeader carries the proxy secret on every request hamstack forwards.
const ProxyHeader = "X-Hamstack-Proxy-Auth"

// HealthPath answers without the secret, so a supervisor's readiness check stays simple.
const HealthPath = "/api/health"

// Env is what hamstack passes in the environment.
type Env struct {
	ID         string
	BasePath   string
	Rigs       string
	DataDir    string
	ConfigDir  string
	SecretFile string
}

// FromEnv reads the HAMSTACK_* variables. ok is false when integration mode is off.
func FromEnv(getenv func(string) string) (e Env, ok bool) {
	e = Env{
		ID:         getenv("HAMSTACK_INTEGRATION_ID"),
		BasePath:   getenv("HAMSTACK_BASE_PATH"),
		Rigs:       getenv("HAMSTACK_RIGS"),
		DataDir:    getenv("HAMSTACK_DATA_DIR"),
		ConfigDir:  getenv("HAMSTACK_CONFIG_DIR"),
		SecretFile: getenv("HAMSTACK_PROXY_SECRET_FILE"),
	}
	return e, e.ID != ""
}

var basePathRe = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)+$`)

// NormalizeBasePath returns a base path in the form "/a/b" (leading slash, no trailing slash),
// or "" for the root. It refuses anything that is not plain path segments.
func NormalizeBasePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "", nil
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !basePathRe.MatchString(p) {
		return "", fmt.Errorf("base path %q: use plain path segments such as /integration/rigctlweb", p)
	}
	for _, seg := range strings.Split(p[1:], "/") {
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("base path %q: no . or .. segments", p)
		}
	}
	return p, nil
}

// MountAt serves next under base: "/base/x" reaches next as "/x", "/base" redirects to "/base/",
// and anything outside base is not found. With base "" it returns next unchanged.
func MountAt(base string, next http.Handler) http.Handler {
	if base == "" {
		return next
	}
	strip := http.StripPrefix(base, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == base:
			u := base + "/"
			if r.URL.RawQuery != "" {
				u += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, u, http.StatusMovedPermanently)
		case strings.HasPrefix(r.URL.Path, base+"/"):
			strip.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// minSecret is the shortest secret accepted (hamstack writes 256 bits as 64 hex characters).
const minSecret = 32

// ReadSecret reads the proxy secret from a file that only its owner may read.
func ReadSecret(path string) (string, error) {
	if path == "" {
		return "", errors.New("HAMSTACK_PROXY_SECRET_FILE is not set")
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("proxy secret file: %w", err)
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("proxy secret file is not a regular file")
	}
	if st.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("proxy secret file is readable by others (mode %04o); it must be 0600", st.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("proxy secret file: %w", err)
	}
	s := strings.TrimSpace(string(b))
	if len(s) < minSecret {
		return "", errors.New("proxy secret is too short")
	}
	return s, nil
}

// RequireSecret refuses every request without the right proxy secret (403), except GET health.
// Paths are matched without the base path, so put it inside MountAt: MountAt(base, RequireSecret(s, h)).
func RequireSecret(secret string, next http.Handler) http.Handler {
	want := []byte(secret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthPath && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get(ProxyHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			// RFC 9457 problem details, like the rest of the published API.
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"type":"about:blank","title":"Forbidden","status":403,"detail":"this program only answers requests that come through hamstack"}` + "\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Rig is one radio hamstack hands over.
type Rig struct {
	ID       string
	Addr     string // host:port of its rigctld
	ReadOnly bool
}

var rigIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// ParseRigs reads HAMSTACK_RIGS. A malformed entry is an error: the caller should refuse to start
// rather than guess, because a misread ",ro" could let it transmit where the owner said no.
func ParseRigs(s string) ([]Rig, error) {
	var out []Rig
	seen := map[string]bool{}
	for _, e := range strings.Split(s, ";") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		id, rest, ok := strings.Cut(e, "=")
		if !ok || !rigIDRe.MatchString(id) {
			return nil, fmt.Errorf("HAMSTACK_RIGS entry %q: want id=host:port[,ro]", e)
		}
		parts := strings.Split(rest, ",")
		addr := parts[0]
		host, port, err := net.SplitHostPort(addr)
		if n, perr := strconv.Atoi(port); err != nil || host == "" || perr != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("HAMSTACK_RIGS entry %q: bad address", e)
		}
		r := Rig{ID: id, Addr: addr}
		for _, f := range parts[1:] {
			if f != "ro" {
				return nil, fmt.Errorf("HAMSTACK_RIGS entry %q: unknown option %q", e, f)
			}
			r.ReadOnly = true
		}
		if seen[id] {
			return nil, fmt.Errorf("HAMSTACK_RIGS: radio %q listed twice", id)
		}
		seen[id] = true
		out = append(out, r)
	}
	return out, nil
}

// Info is the handshake answer, GET <base>/api/v1/integration. Field names are a published
// contract: within API v1 they are only ever added to.
type Info struct {
	Name         string    `json:"name" doc:"Program name."`
	Version      string    `json:"version" doc:"Program version."`
	API          []string  `json:"api" doc:"Versioned API majors served."`
	Mode         string    `json:"mode" enum:"standalone,hamstack" doc:"hamstack when running as a hamstack integration."`
	Capabilities []string  `json:"capabilities" doc:"Features this build offers, e.g. rig.control, audio.monitor."`
	Transmits    bool      `json:"transmits" doc:"True when any radio may transmit (is not read-only)."`
	Rigs         []InfoRig `json:"rigs" doc:"Every radio and whether it is read-only."`
}

// InfoRig is one radio in the handshake.
type InfoRig struct {
	ID       string `json:"id" doc:"Radio id."`
	ReadOnly bool   `json:"readOnly" doc:"True when this program never changes the radio."`
}

// Mode names.
const (
	ModeStandalone = "standalone"
	ModeHamstack   = "hamstack"
)

// Handler answers the handshake from info, called per request so it reflects current rigs.
func Handler(info func() Info) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		in := info()
		if in.Rigs == nil {
			in.Rigs = []InfoRig{}
		}
		if in.API == nil {
			in.API = []string{}
		}
		if in.Capabilities == nil {
			in.Capabilities = []string{}
		}
		in.Transmits = false
		for _, r := range in.Rigs {
			if !r.ReadOnly {
				in.Transmits = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(in)
	})
}
