// Package server is the HTTP/WebSocket front end: REST for configuration, a WebSocket
// per rig for live state and commands, and the embedded Svelte UI.
package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/open-rms/rigctlweb/internal/audio"
	"github.com/open-rms/rigctlweb/internal/config"
	"github.com/open-rms/rigctlweb/internal/integration"
	"github.com/open-rms/rigctlweb/internal/presets"
	"github.com/open-rms/rigctlweb/internal/rig"
)

type Server struct {
	cfg     *config.Store
	mgr     *rig.Manager
	log     *slog.Logger
	ui      fs.FS
	version string
	login   *limiter
	noAuth  bool
	audio   *audioState

	basePath string // "" or "/a/b": where the app is served
	secret   string // set in hamstack integration mode: the proxy secret replaces token sign-in
	index    []byte // index.html with the base path applied

	presets *presets.Service // openRMS Rigs presets; nil turns the routes off
}

func New(cfg *config.Store, mgr *rig.Manager, ui fs.FS, version string, log *slog.Logger) *Server {
	return &Server{cfg: cfg, mgr: mgr, log: log, ui: ui, version: version, login: newLimiter(8, time.Minute), audio: &audioState{hub: audio.NewHub(), mixer: audio.NewMixer(), detect: audio.Detect}}
}

// DisableAuth turns off the access token. For local development only; callers must
// ensure the listener is loopback-only.
func (s *Server) DisableAuth() { s.noAuth = true }

// SetBasePath serves everything under p (already normalised, see integration.NormalizeBasePath).
func (s *Server) SetBasePath(p string) { s.basePath = p }

// SetIntegration turns on hamstack integration mode: every request except health must carry the
// proxy secret, token sign-in is off, the rig list is managed by hamstack, and hamstack's pages
// may frame this one.
func (s *Server) SetIntegration(secret string) { s.secret = secret }

func (s *Server) integrated() bool { return s.secret != "" }

func (s *Server) mode() string {
	if s.integrated() {
		return integration.ModeHamstack
	}
	return integration.ModeStandalone
}

// cookiePath scopes the session cookie to the app's own base path.
func (s *Server) cookiePath() string { return s.basePath + "/" }

func (s *Server) token() string { return s.cfg.Get().Token }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Health needs no token and says nothing beyond "I can serve" (part of the published API, docs/API.md).
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, v1Health{Status: "ok"})
	})
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		// mode tells the UI whether sign-in, sign-out and adding/removing radios are its job.
		writeJSON(w, 200, map[string]any{"authenticated": s.authed(r), "version": s.version, "mode": s.mode()})
	})
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: cookieName, Path: s.cookiePath(), MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w.WriteHeader(http.StatusNoContent)
	})

	api := http.NewServeMux()
	s.registerV1(api)
	api.HandleFunc("GET /api/rigs", s.handleListRigs)
	api.HandleFunc("POST /api/rigs", s.handleAddRig)
	api.HandleFunc("DELETE /api/rigs/{id}", s.handleDeleteRig)
	api.HandleFunc("GET /api/rigs/{id}/caps", s.handleCaps)
	api.HandleFunc("GET /api/rigs/{id}/caps/raw", s.handleCapsRaw)
	api.HandleFunc("GET /api/views", s.handleGetViews)
	api.HandleFunc("PUT /api/views", s.handlePutViews)
	api.HandleFunc("GET /api/settings", s.handleGetSettings)
	api.HandleFunc("PUT /api/settings", s.handlePutSettings)
	api.HandleFunc("GET /api/rigs/{id}/audio", s.handleGetAudio)
	api.HandleFunc("PUT /api/rigs/{id}/audio", s.handlePutAudio)
	api.HandleFunc("GET /api/rigs/{id}/audio/input", s.handleGetAudioInput)
	api.HandleFunc("PUT /api/rigs/{id}/audio/input", s.handlePutAudioInput)
	api.HandleFunc("GET /api/rigs/{id}/memories", s.handleGetMemories)
	api.HandleFunc("POST /api/rigs/{id}/memories/read", s.handleReadMemories)
	api.HandleFunc("POST /api/rigs/{id}/memories/stop", s.handleStopMemories)
	api.HandleFunc("GET /api/presets", s.handlePresetList)
	api.HandleFunc("POST /api/presets/update", s.handlePresetUpdate)
	api.HandleFunc("GET /api/presets/{pid}", s.handlePresetGet)
	api.HandleFunc("POST /api/rigs/{id}/presets/{pid}/check", s.handlePresetCheck)
	api.HandleFunc("POST /api/rigs/{id}/presets/{pid}/apply", s.handlePresetApply)
	api.HandleFunc("GET /api/rigs/{id}/presets-snapshot", s.handlePresetSnapshot)
	api.HandleFunc("POST /api/rigs/{id}/presets-restore", s.handlePresetRestore)
	api.HandleFunc("GET /ws", s.handleWS)
	api.HandleFunc("GET /ws/audio", s.handleAudioWS)
	mux.Handle("/api/", s.requireAuth(api))
	mux.Handle("/ws", s.requireAuth(api))
	mux.Handle("/ws/audio", s.requireAuth(api))

	mux.Handle("/", s.spa())

	s.index = s.buildIndex()
	var h http.Handler = mux
	if s.integrated() {
		h = integration.RequireSecret(s.secret, h)
	}
	return s.secureHeaders(integration.MountAt(s.basePath, h))
}

// buildIndex applies the base path to the built index.html: SvelteKit's fallback page uses
// root-absolute asset URLs and an empty runtime base, so both are pointed at the base path.
// The CSP hash is computed from these rewritten bytes.
func (s *Server) buildIndex() []byte {
	b, err := fs.ReadFile(s.ui, "index.html")
	if err != nil || s.basePath == "" {
		return b
	}
	return rebase(b, s.basePath)
}

// rebase rewrites root-absolute URLs in index.html to sit under base. base is already
// validated as plain path segments, so it needs no escaping.
func rebase(b []byte, base string) []byte {
	out := string(b)
	for _, attr := range []string{`href="/`, `src="/`, `import("/`} {
		out = strings.ReplaceAll(out, attr, attr[:len(attr)-1]+base+"/")
	}
	out = strings.Replace(out, `base: ""`, `base: "`+base+`"`, 1)
	return []byte(out)
}

var inlineScript = regexp.MustCompile(`(?s)<script(?:\s[^>]*)?>(.*?)</script>`)

// csp builds the Content-Security-Policy. SvelteKit's static build ships one inline
// bootstrap script in index.html; instead of allowing 'unsafe-inline', its SHA-256
// hash is allowed, so any other injected script stays blocked.
func (s *Server) csp() string {
	script := "script-src 'self'"
	if b := s.index; b != nil {
		for _, m := range inlineScript.FindAllSubmatch(b, -1) {
			if len(m[1]) == 0 {
				continue
			}
			sum := sha256.Sum256(m[1])
			script += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		}
	}
	// Standalone nobody may frame the page; inside hamstack its own pages may (same origin).
	frame := "'none'"
	if s.integrated() {
		frame = "'self'"
	}
	return "default-src 'self'; " + script +
		"; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors " + frame + "; base-uri 'self'; form-action 'self'"
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	csp := s.csp()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		if s.integrated() {
			h.Set("X-Frame-Options", "SAMEORIGIN")
		} else {
			h.Set("X-Frame-Options", "DENY")
		}
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// spa serves the embedded UI, falling back to index.html for client-side routes.
func (s *Server) spa() http.Handler {
	files := http.FileServerFS(s.ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A build from source without the web step has an empty UI (only the .gitkeep that lets
		// go:embed compile). Say so plainly instead of serving a directory listing.
		if s.index == nil {
			http.Error(w, "rigctlWeb was built without its web interface.\n"+
				"Download a release, or build from source with `mise run build` (it builds the web UI first).\n"+
				"The API is still available.", http.StatusServiceUnavailable)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.ui, p); err != nil || p == "index.html" {
			// Client-side routes get the (rebased) index page.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(s.index)
			return
		}
		if strings.HasPrefix(p, "assets/") || strings.HasPrefix(p, "_app/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.integrated() {
		http.Error(w, "sign-in is handled by hamstack", http.StatusNotFound)
		return
	}
	if !s.login.allow(r) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, &body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Token), []byte(s.token())) != 1 {
		time.Sleep(400 * time.Millisecond)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	s.setSession(w, r)
	writeJSON(w, 200, map[string]any{"authenticated": true})
}

type rigView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Addr     string    `json:"addr"`
	ReadOnly bool      `json:"readOnly"`
	State    rig.State `json:"state"`
}

func view(r *rig.Rig) rigView {
	c := r.Config()
	return rigView{ID: c.ID, Name: c.Name, Addr: c.Addr, ReadOnly: c.ReadOnly, State: r.Snapshot()}
}

func (s *Server) handleListRigs(w http.ResponseWriter, _ *http.Request) {
	out := []rigView{}
	for _, r := range s.mgr.List() {
		out = append(out, view(r))
	}
	writeJSON(w, 200, out)
}

var idRe = regexp.MustCompile(`[^a-z0-9]+`)
var addrRe = regexp.MustCompile(`^[A-Za-z0-9.\-\[\]:]+:\d{1,5}$`)

// refuseManaged answers attempts to change the rig list when hamstack owns it.
func (s *Server) refuseManaged(w http.ResponseWriter) bool {
	if s.integrated() {
		http.Error(w, "the radios are managed by hamstack", http.StatusForbidden)
		return true
	}
	return false
}

func (s *Server) handleAddRig(w http.ResponseWriter, r *http.Request) {
	if s.refuseManaged(w) {
		return
	}
	var in struct {
		Name     string `json:"name"`
		Addr     string `json:"addr"`
		ReadOnly bool   `json:"readOnly"`
	}
	if err := decode(w, r, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 || !addrRe.MatchString(in.Addr) {
		http.Error(w, "need a name and a host:port address", http.StatusBadRequest)
		return
	}
	id := strings.Trim(idRe.ReplaceAllString(strings.ToLower(in.Name), "-"), "-")
	if id == "" {
		id = "rig"
	}
	cfg := rig.Config{ID: id, Name: in.Name, Addr: in.Addr, ReadOnly: in.ReadOnly}
	for n := 2; ; n++ {
		if _, exists := s.mgr.Get(cfg.ID); !exists {
			break
		}
		if n > 99 {
			http.Error(w, "too many rigs with that name", http.StatusConflict)
			return
		}
		cfg.ID = fmt.Sprintf("%s-%d", id, n)
	}
	if err := s.mgr.Add(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := s.cfg.Update(func(c *config.Config) error { c.Rigs = append(c.Rigs, cfg); return nil }); err != nil {
		_ = s.mgr.Remove(cfg.ID)
		http.Error(w, "could not save config", http.StatusInternalServerError)
		return
	}
	r2, _ := s.mgr.Get(cfg.ID)
	writeJSON(w, http.StatusCreated, view(r2))
}

func (s *Server) handleDeleteRig(w http.ResponseWriter, r *http.Request) {
	if s.refuseManaged(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.mgr.Remove(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	_ = s.cfg.Update(func(c *config.Config) error {
		keep := c.Rigs[:0]
		for _, rc := range c.Rigs {
			if rc.ID != id {
				keep = append(keep, rc)
			}
		}
		c.Rigs = keep
		delete(c.Audio, id)
		return nil
	})
	s.audio.hub.Stop(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCaps(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	caps := rg.Caps()
	if caps == nil {
		http.Error(w, "rig not connected yet", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, 200, caps)
}

// handleCapsRaw returns rigctld's untouched dump_caps text, for radio-support reports.
func (s *Server) handleCapsRaw(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.mgr.Get(r.PathValue("id"))
	if !ok || rg.Caps() == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="dump_caps.txt"`)
	_, _ = w.Write([]byte(rg.Caps().Raw))
}

type settings struct {
	DefaultView    string `json:"defaultView"`
	Theme          string `json:"theme"`
	FavoriteModels []int  `json:"favoriteModels"`
	Listen         string `json:"listen"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	c := s.cfg.Get()
	writeJSON(w, 200, settings{DefaultView: c.DefaultView, Theme: c.Theme, FavoriteModels: c.FavoriteModels, Listen: c.Listen})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in settings
	if err := decode(w, r, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch in.Theme {
	case "system", "light", "dark", "red":
	default:
		http.Error(w, "invalid theme", http.StatusBadRequest)
		return
	}
	if in.DefaultView != "" && !viewIDRe.MatchString(in.DefaultView) {
		http.Error(w, "invalid default view", http.StatusBadRequest)
		return
	}
	if len(in.FavoriteModels) > 200 {
		http.Error(w, "too many favorites", http.StatusBadRequest)
		return
	}
	// Listen address changes need a restart and are deliberately not editable over the API.
	err := s.cfg.Update(func(c *config.Config) error {
		c.Theme, c.FavoriteModels, c.DefaultView = in.Theme, in.FavoriteModels, in.DefaultView
		return nil
	})
	if err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	s.handleGetSettings(w, r)
}

// ---- WebSocket ----

// nextController hands out an ID per WebSocket connection (0 means "no client").
var nextController atomic.Uint64

type wsIn struct {
	ID   int64          `json:"id"`
	Cmd  string         `json:"cmd"`
	Args map[string]any `json:"args"`
}

type wsOut struct {
	Type  string     `json:"type"` // "state" | "result"
	State *rig.State `json:"state,omitempty"`
	ID    int64      `json:"id,omitempty"`
	OK    bool       `json:"ok,omitempty"`
	Error string     `json:"error,omitempty"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.mgr.Get(r.URL.Query().Get("rig"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Accept enforces same-origin by default, which blocks cross-site WebSocket hijacking.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// If this connection keyed the radio and goes away (tab closed, network lost), unkey.
	// Wait for its in-flight commands first so a late key-up can't slip past.
	ctrl := nextController.Add(1)
	var inflight sync.WaitGroup
	defer func() {
		cancel()
		inflight.Wait()
		uctx, ucancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ucancel()
		if err := rg.ControllerLeft(uctx, ctrl); err != nil {
			s.log.Error("unkey after disconnect failed", "rig", r.URL.Query().Get("rig"), "err", err)
		}
	}()

	out := make(chan wsOut, 32)
	send := func(m wsOut) {
		select {
		case out <- m:
		case <-ctx.Done():
		}
	}

	states, unsub := rg.Subscribe()
	defer unsub()
	snap := rg.Snapshot()
	send(wsOut{Type: "state", State: &snap})

	go func() { // state fan-out
		for {
			select {
			case st := <-states:
				send(wsOut{Type: "state", State: &st})
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { // single writer
		for {
			select {
			case m := <-out:
				wctx, c := context.WithTimeout(ctx, 5*time.Second)
				err := wsjson.Write(wctx, conn, m)
				c()
				if err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		var in wsIn
		if err := wsjson.Read(ctx, conn, &in); err != nil {
			return
		}
		inflight.Add(1)
		go func() {
			defer inflight.Done()
			err := rg.Exec(rig.WithController(ctx, ctrl), rig.Command{Name: in.Cmd, Args: in.Args})
			res := wsOut{Type: "result", ID: in.ID, OK: err == nil}
			if err != nil {
				res.Error = err.Error()
				if !errors.Is(err, rig.ErrReadOnly) {
					s.log.Debug("command failed", "cmd", in.Cmd, "err", err)
				}
			}
			send(res)
		}()
	}
}

// ---- helpers ----

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("content-type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ---- views ----

var (
	viewIDRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	controlIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:+-]{1,64}$`)
)

const maxViews = 50

func validateViews(views []config.ViewDef) error {
	if len(views) > maxViews {
		return fmt.Errorf("at most %d views", maxViews)
	}
	seen := map[string]bool{}
	for _, v := range views {
		switch {
		case !viewIDRe.MatchString(v.ID):
			return fmt.Errorf("invalid view id %q", v.ID)
		case seen[v.ID]:
			return fmt.Errorf("duplicate view id %q", v.ID)
		case strings.TrimSpace(v.Name) == "" || len(v.Name) > 40:
			return fmt.Errorf("view %q needs a name of 1-40 characters", v.ID)
		case len(v.Description) > 200:
			return fmt.Errorf("view %q description is too long", v.ID)
		case len(v.Controls) > 600:
			return fmt.Errorf("view %q has too many controls", v.ID)
		}
		seen[v.ID] = true
		for _, c := range v.Controls {
			if !controlIDRe.MatchString(c) {
				return fmt.Errorf("view %q has an invalid control id %q", v.ID, c)
			}
		}
	}
	return nil
}

func (s *Server) handleGetViews(w http.ResponseWriter, _ *http.Request) {
	v := s.cfg.Get().Views
	if v == nil {
		v = []config.ViewDef{}
	}
	for i := range v {
		if v[i].Controls == nil {
			v[i].Controls = []string{}
		}
	}
	writeJSON(w, 200, v)
}

func (s *Server) handlePutViews(w http.ResponseWriter, r *http.Request) {
	var in []config.ViewDef
	if err := decode(w, r, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := validateViews(in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.cfg.Update(func(c *config.Config) error { c.Views = in; return nil }); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	s.handleGetViews(w, r)
}
