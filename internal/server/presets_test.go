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

	"github.com/open-rms/rigctlweb/internal/config"
	"github.com/open-rms/rigctlweb/internal/presets"
	"github.com/open-rms/rigctlweb/internal/rig"
)

// Presets end to end against Hamlib's dummy radio and a stand-in Rigs server.
func TestPresetsAgainstDummyRig(t *testing.T) {
	bin, err := exec.LookPath("rigctld")
	if err != nil {
		t.Skip("rigctld not installed")
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	rc := exec.Command(bin, "-m", "1", "-t", strconv.Itoa(port), "-T", "127.0.0.1")
	if err := rc.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rc.Process.Kill(); _ = rc.Wait() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	doc := func(steps string) json.RawMessage {
		return json.RawMessage(`{"schema":"openrms-rigs/profile/v1","title":"t","rig":{"macro":"RIG_MODEL_DUMMY","hamlib_model":1,"mfg":"Hamlib","name":"Dummy"},
			"operation":"ft8","connection":"usb","hamlib":">=4","steps":[` + steps + `]}`)
	}
	dump, _ := json.Marshal(map[string]any{"license": "CC-BY-SA-4.0", "profiles": []any{
		map[string]any{"id": "dummy-ft8", "status": "published", "title": "Dummy FT8", "author": map[string]any{"callsign": "N0CALL"}, "document": doc(`
			{"id":"mode","kind":"mode","name":"MODE","value":"USB","passband_hz":3000},
			{"id":"nb","kind":"func","name":"NB","value":true},
			{"id":"rf","kind":"level","name":"RF","value":0.25},
			{"id":"menu","kind":"cat","name":"MENU","value":1,"cat":{"protocol":"ascii","read":"EX01;","set":"EX01{value};"}}`)},
		map[string]any{"id": "dummy-vox", "status": "published", "title": "Bad", "document": doc(`
			{"id":"mode","kind":"mode","name":"MODE","value":"USB"},
			{"id":"vox","kind":"func","name":"VOX","value":true}`)},
	}})
	rigs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/dump.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(dump)
	}))
	defer rigs.Close()

	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := rig.NewManager(ctx, slog.Default())
	srv := New(store, mgr, fstest.MapFS{"index.html": {Data: []byte(`<html></html>`)}}, "test", slog.Default())
	lib, err := presets.NewLibrary(dir, rigs.URL+"/api/v1", "rigctlweb/test")
	if err != nil {
		t.Fatal(err)
	}
	srv.SetPresets(presets.NewService(lib, dir, slog.Default()))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	tok := store.Get().Token

	call := func(method, path, body string, out any) int {
		t.Helper()
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if out != nil {
			_ = json.NewDecoder(res.Body).Decode(out)
		}
		return res.StatusCode
	}
	if err := mgr.Add(rig.Config{ID: "d", Name: "D", Addr: addr}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add(rig.Config{ID: "ro", Name: "RO", Addr: addr, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	d, _ := mgr.Get("d")
	ro, _ := mgr.Get("ro")
	for end := time.Now().Add(10 * time.Second); d.Caps() == nil || ro.Caps() == nil; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("dummy rig never came online")
		}
	}

	// Every presets route needs the token.
	for _, rt := range [][2]string{{"GET", "/api/presets"}, {"POST", "/api/presets/update"}, {"GET", "/api/presets/x"},
		{"POST", "/api/rigs/d/presets/x/check"}, {"POST", "/api/rigs/d/presets/x/apply"},
		{"GET", "/api/rigs/d/presets-snapshot"}, {"POST", "/api/rigs/d/presets-restore"}} {
		req, _ := http.NewRequest(rt[0], ts.URL+rt[1], strings.NewReader(`{"steps":["nb"]}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Errorf("%s %s without a token = %d", rt[0], rt[1], res.StatusCode)
		}
	}

	// Empty until the operator asks for the library.
	var list struct {
		Library  presets.LibraryStatus `json:"library"`
		Profiles []presets.Summary     `json:"profiles"`
	}
	if call("GET", "/api/presets?rig=d", "", &list); list.Library.Available || len(list.Profiles) != 0 {
		t.Fatalf("library before update: %+v", list)
	}
	if code := call("POST", "/api/presets/update", "", nil); code != 200 {
		t.Fatalf("update = %d", code)
	}
	call("GET", "/api/presets?rig=d", "", &list)
	if len(list.Profiles) != 2 {
		t.Fatalf("profiles: %+v", list.Profiles)
	}
	for _, p := range list.Profiles {
		switch p.ID {
		case "dummy-ft8":
			if !p.Matches || p.Refused != "" || p.Settings != 3 || p.Manual != 1 || p.Author != "N0CALL" {
				t.Errorf("ft8 summary %+v", p)
			}
		case "dummy-vox":
			if p.Refused == "" || p.Settings != 0 {
				t.Errorf("VOX profile not refused: %+v", p)
			}
		}
	}

	var check presets.CheckResult
	if code := call("POST", "/api/rigs/ro/presets/dummy-ft8/check", "", &check); code != 200 || check.Summary["manual"] != 1 {
		t.Fatalf("check on read-only = %d %+v", code, check)
	}
	if code := call("POST", "/api/rigs/ro/presets/dummy-ft8/apply", `{"steps":["nb"]}`, nil); code != 403 {
		t.Errorf("apply on read-only = %d", code)
	}
	if code := call("POST", "/api/rigs/d/presets/dummy-vox/apply", `{"steps":["mode"]}`, nil); code != 409 {
		t.Errorf("apply refused profile = %d", code)
	}
	if code := call("POST", "/api/rigs/d/presets/dummy-ft8/apply", `{"steps":["menu"]}`, nil); code != 409 {
		t.Errorf("apply raw CAT step = %d", code)
	}

	before := d.Snapshot()
	var res presets.ApplyResult
	if code := call("POST", "/api/rigs/d/presets/dummy-ft8/apply", `{"steps":["mode","nb","rf"]}`, &res); code != 200 || len(res.Applied) != 3 {
		t.Fatalf("apply = %d %+v", code, res)
	}
	call("POST", "/api/rigs/d/presets/dummy-ft8/check", "", &check)
	if check.Summary["ok"] != 3 {
		t.Errorf("after apply: %+v", check)
	}
	var snap struct {
		Snapshot *presets.Snapshot `json:"snapshot"`
	}
	if call("GET", "/api/rigs/d/presets-snapshot", "", &snap); snap.Snapshot == nil || len(snap.Snapshot.Saved) != 3 {
		t.Fatalf("snapshot %+v", snap.Snapshot)
	}
	var rr presets.RestoreResult
	if code := call("POST", "/api/rigs/d/presets-restore", "", &rr); code != 200 || len(rr.Restored) != 3 || rr.Remaining != 0 {
		t.Fatalf("restore = %d %+v", code, rr)
	}
	m, _ := d.ReadSetting(ctx, "mode", "MODE")
	if m.Mode != before.Mode {
		t.Errorf("mode after restore = %s, want %s", m.Mode, before.Mode)
	}
	if d.Snapshot().PTT {
		t.Fatal("the radio was keyed")
	}
}
