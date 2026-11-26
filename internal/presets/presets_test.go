package presets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/open-rms/rigctlweb/internal/catallow"
	"github.com/open-rms/rigctlweb/internal/rig"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

func ftdx10Caps(t *testing.T) *rigctl.Caps {
	t.Helper()
	b, err := os.ReadFile("../rigctl/testdata/ftdx10.caps")
	if err != nil {
		t.Fatal(err)
	}
	return rigctl.ParseCaps(strings.Split(string(b), "\n"))
}

// A synthetic FT-DX10 profile shaped like the published ones (not copied from them).
const sampleDoc = `{
  "schema": "openrms-rigs/profile/v1", "title": "Test FT8", "rig": {"macro": "RIG_MODEL_FTDX10", "hamlib_model": 1042, "mfg": "Yaesu", "name": "FTDX-10"},
  "operation": "ft8", "connection": "usb", "hamlib": ">=4.6", "future_field": {"ignored": true},
  "sources": [{"id": "S1", "title": "Manual", "author": "Yaesu", "url": "https://example.org/m.pdf"}],
  "steps": [
    {"id": "mode", "kind": "mode", "name": "MODE", "value": "PKTUSB", "passband_hz": 3000, "sources": ["S1"], "confidence": "high"},
    {"id": "agc", "kind": "level", "name": "AGC", "value": 6, "expect": {"not": [0]}, "display": "AUTO"},
    {"id": "nb-off", "kind": "func", "name": "NB", "value": false},
    {"id": "comp-off", "kind": "func", "name": "COMP", "value": false},
    {"id": "power", "kind": "level", "name": "RFPOWER", "value": 0.5, "safety": "rx"},
    {"id": "rear", "kind": "cat", "name": "REAR SELECT", "value": 1, "safety": "tx", "cat": {"protocol": "ascii", "read": "EX010416;", "reply": "^EX010416(\\d);$", "set": "EX010416{value};", "values": {"0": "DATA", "1": "USB"}}},
    {"id": "shift", "kind": "cat", "name": "DATA SHIFT", "value": 1500, "cat": {"protocol": "ascii", "read": "EX010408;", "reply": "^EX010408(\\d{4});$", "set": "EX010408{value:04};"}},
    {"id": "noread", "kind": "cat", "name": "NO READ", "value": 1, "cat": {"protocol": "ascii", "set": "EX010420{value};"}},
    {"id": "gtmd", "kind": "cat", "name": "TWO AT ONCE", "value": 4, "cat": {"protocol": "ascii", "read": "GT0;", "reply": "^GT0(\\d);$", "set": "GT0{value};MD0C;"}},
    {"id": "outlvl2", "kind": "cat", "name": "DATA OUT LEVEL", "check_only": true, "cat": {"protocol": "ascii", "read": "EX010413;", "reply": "^EX010413(\\d{3});$", "set": "EX010413{value:03};"}},
    {"id": "outlvl", "kind": "cat", "name": "DATA OUT LEVEL", "check_only": true, "expect": {"min": 30, "max": 70}, "cat": {"protocol": "ascii", "read": "EX010413;", "reply": "^EX010413(\\d{3});$"}},
    {"id": "agc-check", "kind": "level", "name": "AGC", "value": 6, "check_only": true},
    {"id": "ext", "kind": "level", "name": "SOMETHING_EXT", "value": 1},
    {"id": "width", "kind": "manual", "name": "Width", "manual": {"menu": "WIDTH", "value": "3000 Hz"}},
    {"id": "hologram", "kind": "hologram", "name": "X", "value": 1}
  ]}`

func envelope(t *testing.T, id, doc string) Envelope {
	t.Helper()
	var p Profile
	if err := json.Unmarshal([]byte(doc), &p); err != nil {
		t.Fatal(err)
	}
	return Envelope{ID: id, Revision: 1, Status: "published", Title: p.Title, Operation: p.Operation, Document: p}
}

func stepOf(v View, id string) PlanStep {
	for _, s := range v.Steps {
		if s.ID == id {
			return s
		}
	}
	return PlanStep{}
}

func TestPlanMarksSteps(t *testing.T) {
	caps := ftdx10Caps(t)
	v := Plan(envelope(t, "p", sampleDoc), caps)
	if !v.Matches || v.Refused != "" {
		t.Fatalf("matches=%v refused=%q", v.Matches, v.Refused)
	}
	for id, want := range map[string]bool{"mode": true, "agc": true, "nb-off": true, "comp-off": true, "power": true, "rear": true, "shift": true,
		"noread": false, "gtmd": false, "outlvl": false, "width": false, "hologram": false, "agc-check": false, "ext": false} {
		if got := stepOf(v, id).Applicable; got != want {
			t.Errorf("%s applicable = %v, want %v (%s)", id, got, want, stepOf(v, id).Reason)
		}
	}
	// Transmit-side names are tx whatever the profile says.
	if stepOf(v, "power").Safety != "tx" || stepOf(v, "comp-off").Safety != "tx" || stepOf(v, "nb-off").Safety != "rx" {
		t.Error("safety not raised for RFPOWER/COMP")
	}
	if c := stepOf(v, "rear"); !c.Readable || c.Command != "EX0104161;" || c.Safety != "tx" || c.Choices["1"] != "USB" {
		t.Errorf("cat step: %+v", c)
	}
	if c := stepOf(v, "shift"); c.Command != "EX0104081500;" || c.Safety != "rx" {
		t.Errorf("padded cat step: %+v", c)
	}
	if c := stepOf(v, "noread"); c.Readable || !strings.Contains(c.Reason, "Restore") {
		t.Errorf("cat step without a read: %+v", c)
	}
	if c := stepOf(v, "gtmd"); c.Readable || !strings.Contains(c.Reason, "won't send") {
		t.Errorf("cat step setting two things: %+v", c)
	}
	if c := stepOf(v, "outlvl"); c.Readable {
		t.Errorf("check-only cat step without a set template was readable: %+v", c)
	}
	if c := stepOf(v, "outlvl2"); !c.Readable || !c.CheckOnly {
		t.Errorf("check-only cat step: %+v", c)
	}
	if v.Settings != 7 || v.TxSettings != 3 || v.Manual != 4 {
		t.Errorf("counts settings=%d tx=%d manual=%d", v.Settings, v.TxSettings, v.Manual)
	}
	// Another radio: readable as a manual, nothing applicable.
	other := Plan(envelope(t, "p", sampleDoc), &rigctl.Caps{Model: "IC-7300", Mfg: "Icom"})
	if other.Matches || other.Settings != 0 {
		t.Errorf("other radio: matches=%v settings=%d", other.Matches, other.Settings)
	}
	// No radio at all.
	if none := Plan(envelope(t, "p", sampleDoc), nil); none.Settings != 0 || none.Connected {
		t.Error("nothing should be applicable without a radio")
	}
}

func TestRefusal(t *testing.T) {
	bad := map[string]string{
		"keying func":  `{"id":"a","kind":"func","name":"VOX","value":true}`,
		"send_ func":   `{"id":"a","kind":"func","name":"SEND_MORSE","value":true}`,
		"tuner":        `{"id":"a","kind":"func","name":"TUNER","value":true}`,
		"unlisted":     `{"id":"a","kind":"func","name":"TRANSCEIVE","value":true}`,
		"ptt level":    `{"id":"a","kind":"level","name":"PTT","value":1}`,
		"tune parm":    `{"id":"a","kind":"parm","name":"TUNE","value":1}`,
		"func type":    `{"id":"a","kind":"func","name":"NB","value":"yes"}`,
		"dupe ids":     `{"id":"a","kind":"func","name":"NB","value":true},{"id":"a","kind":"func","name":"NR","value":true}`,
		"cat tx":       `{"id":"a","kind":"cat","name":"x","value":1,"cat":{"protocol":"ascii","read":"EX010416;","set":"TX{value};"}}`,
		"cat chained":  `{"id":"a","kind":"cat","name":"x","value":1,"cat":{"protocol":"ascii","read":"EX010416;","set":"EX010416{value};TX1;"}}`,
		"cat read":     `{"id":"a","kind":"cat","name":"x","check_only":true,"cat":{"protocol":"ascii","read":"FA;"}}`,
		"cat value":    `{"id":"a","kind":"cat","name":"x","value":"1;TX","cat":{"protocol":"ascii","read":"EX010416;","set":"EX010416{value};"}}`,
		"cat wide":     `{"id":"a","kind":"cat","name":"x","value":1000,"cat":{"protocol":"ascii","read":"EX010417;","set":"EX010417{value:03};"}}`,
		"cat negative": `{"id":"a","kind":"cat","name":"x","value":-1,"cat":{"protocol":"ascii","read":"EX010417;","set":"EX010417{value};"}}`,
		"cat space":    `{"id":"a","kind":"cat","name":"x","value":1,"cat":{"protocol":"ascii","read":"EX010416;","set":"EX010416 {value};"}}`,
		"cat no cmds":  `{"id":"a","kind":"cat","name":"x","value":1}`,
		"cat proto":    `{"id":"a","kind":"cat","name":"x","value":1,"cat":{"protocol":"rtty","read":"EX010416;"}}`,
		"civ tx":       `{"id":"a","kind":"cat","name":"x","value":1,"cat":{"protocol":"civ","read":"1A0601","set":"1C00{value:02}"}}`,
	}
	for name, steps := range bad {
		doc := `{"schema":"openrms-rigs/profile/v1","title":"x","rig":{"mfg":"Yaesu","name":"FTDX-10"},"steps":[` + steps + `]}`
		e := envelope(t, "bad", doc)
		if Refusal(e.Document) == "" {
			t.Errorf("%s: not refused", name)
		}
		if v := Plan(e, ftdx10Caps(t)); v.Settings != 0 {
			t.Errorf("%s: %d steps still applicable", name, v.Settings)
		}
	}
}

// Every function a profile may set must stay on the shared allow-list and never key.
func TestSafeFuncsNeverKey(t *testing.T) {
	for f := range rig.SafeFuncs {
		if rig.IsKeyingFunc(f) {
			t.Errorf("%s is on the allow-list but keys", f)
		}
	}
}

func TestCompare(t *testing.T) {
	caps := ftdx10Caps(t)
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		st     Step
		cur    any
		status string
		noop   bool
	}{
		{Step{Kind: "mode", Value: "PKTUSB"}, "pktusb", "ok", true},
		{Step{Kind: "mode", Value: "PKTUSB"}, "USB", "drift", false},
		{Step{Kind: "level", Name: "RFPOWER", Value: 0.5}, 0.505, "ok", true}, // within RFPOWER's 0.01 step
		{Step{Kind: "level", Name: "RFPOWER", Value: 0.5}, 0.52, "drift", false},
		{Step{Kind: "level", Name: "AGC", Value: 6.0, Expect: &Expect{Not: []any{0.0}}}, 3.0, "ok", false},
		{Step{Kind: "level", Name: "AGC", Value: 6.0, Expect: &Expect{Not: []any{0.0}}}, 0.0, "drift", false},
		{Step{Kind: "level", Name: "X", Expect: &Expect{Min: f(30), Max: f(70)}}, 50.0, "ok", false},
		{Step{Kind: "level", Name: "X", Expect: &Expect{Min: f(30), Max: f(70)}}, 5.0, "drift", false},
		{Step{Kind: "func", Value: false}, false, "ok", true},
		{Step{Kind: "func", Value: false}, true, "drift", false},
		{Step{Kind: "parm", Value: "ON", Expect: &Expect{OneOf: []any{"ON", "AUTO"}}}, "auto", "ok", false},
	}
	for i, c := range cases {
		s, n := compare(c.st, c.cur, levelTolerance(c.st, caps, nil))
		if s != c.status || n != c.noop {
			t.Errorf("case %d: got %s/%v, want %s/%v", i, s, n, c.status, c.noop)
		}
	}
}

// fakeRadio records commands and serves settings from a map.
type fakeRadio struct {
	mu     sync.Mutex
	caps   *rigctl.Caps
	ro     bool
	ptt    bool
	vals   map[string]rig.Setting
	sent   []rig.Command
	failOn string
	cat    map[string]string // raw CAT settings by read command without ';'
	catOut []string          // raw CAT sets sent
	catIn  []string          // raw CAT reads sent
	stuck  bool              // the radio ignores raw CAT sets
}

func (f *fakeRadio) catOK(use, cmd string) error {
	if f.ro {
		return rig.ErrReadOnly
	}
	if f.ptt {
		return rig.ErrTransmitting
	}
	if r := catallow.Check("ascii", use, cmd); !r.Allowed {
		return fmt.Errorf("%w: %s", rig.ErrCATRefused, r.Why)
	}
	return nil
}

func (f *fakeRadio) CATRead(_ context.Context, cmd string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.catOK("read", cmd); err != nil {
		return "", err
	}
	f.catIn = append(f.catIn, cmd)
	k := strings.TrimSuffix(cmd, ";")
	v, ok := f.cat[k]
	if !ok {
		return "?;", nil
	}
	return k + v + ";", nil
}

func (f *fakeRadio) CATSet(_ context.Context, cmd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.catOK("set", cmd); err != nil {
		return err
	}
	f.catOut = append(f.catOut, cmd)
	for k := range f.cat {
		if strings.HasPrefix(cmd, k) && !f.stuck {
			f.cat[k] = strings.TrimSuffix(cmd[len(k):], ";")
		}
	}
	return nil
}

func (f *fakeRadio) Config() rig.Config  { return rig.Config{ID: "ftdx10", ReadOnly: f.ro} }
func (f *fakeRadio) Caps() *rigctl.Caps  { return f.caps }
func (f *fakeRadio) Snapshot() rig.State { return rig.State{PTT: f.ptt} }
func (f *fakeRadio) ReadSetting(_ context.Context, kind, name string) (rig.Setting, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vals[kind+":"+name], nil
}
func (f *fakeRadio) Exec(_ context.Context, c rig.Command) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c.Name == "set_ptt" || c.Name == "vfo_op" {
		panic("a preset tried to key or tune the radio")
	}
	name, _ := c.Args["name"].(string)
	if name == f.failOn && name != "" {
		return errors.New("rig said no")
	}
	f.sent = append(f.sent, c)
	switch c.Name {
	case "set_mode":
		pb, _ := c.Args["passband"].(float64)
		f.vals["mode:MODE"] = rig.Setting{Mode: c.Args["mode"].(string), Passband: int(pb)}
	case "set_level":
		f.vals["level:"+name] = rig.Setting{Number: c.Args["value"].(float64)}
	case "set_func":
		f.vals["func:"+name] = rig.Setting{On: c.Args["on"].(bool)}
	}
	return nil
}

func newFake(t *testing.T) *fakeRadio {
	return &fakeRadio{caps: ftdx10Caps(t), vals: map[string]rig.Setting{
		"mode:MODE": {Mode: "USB", Passband: 2400}, "level:AGC": {Number: 0}, "func:NB": {On: true},
		"func:COMP": {On: false}, "level:RFPOWER": {Number: 1},
	}, cat: map[string]string{"EX010416": "0", "EX010408": "1500", "EX010413": "050", "GT0": "4"}}
}

func testService(t *testing.T, envs ...Envelope) *Service {
	t.Helper()
	dir := t.TempDir()
	lib := &Library{path: filepath.Join(dir, "lib.json"), base: "https://x"}
	lib.set(&cache{Dump: Dump{Profiles: envs}})
	return NewService(lib, dir, nil)
}

func TestCheckApplyRestore(t *testing.T) {
	ctx := context.Background()
	svc := testService(t, envelope(t, "p", sampleDoc))
	r := newFake(t)

	res, err := svc.Check(ctx, r, "p")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"mode": "drift", "agc": "drift", "nb-off": "drift", "comp-off": "ok", "power": "drift", "rear": "drift", "shift": "ok", "outlvl": "manual", "outlvl2": "ok", "noread": "manual", "gtmd": "manual", "width": "manual"}
	for _, s := range res.Steps {
		if w, ok := want[s.ID]; ok && s.Status != w {
			t.Errorf("check %s = %s, want %s", s.ID, s.Status, w)
		}
	}

	// Refusals: read-only, transmitting, unknown or non-applicable steps.
	r.ro = true
	if _, err := svc.Apply(ctx, r, "p", []string{"mode"}, nil); !errors.Is(err, rig.ErrReadOnly) {
		t.Errorf("read-only: %v", err)
	}
	r.ro, r.ptt = false, true
	if _, err := svc.Apply(ctx, r, "p", []string{"mode"}, nil); err == nil {
		t.Error("applied while transmitting")
	}
	r.ptt = false
	for _, bad := range [][]string{{"noread"}, {"gtmd"}, {"outlvl"}, {"width"}, {"agc-check"}, {"ext"}, {"hologram"}, {"nope"}, {}} {
		if _, err := svc.Apply(ctx, r, "p", bad, nil); err == nil {
			t.Errorf("applied %v", bad)
		}
	}
	if len(r.sent) != 0 || len(r.catOut) != 0 {
		t.Fatalf("commands sent during refusals: %v %v", r.sent, r.catOut)
	}

	out, err := svc.Apply(ctx, r, "p", []string{"mode", "agc", "nb-off"}, nil)
	if err != nil || len(out.Applied) != 3 {
		t.Fatalf("apply: %+v %v", out, err)
	}
	if r.vals["mode:MODE"].Mode != "PKTUSB" || r.vals["mode:MODE"].Passband != 3000 || r.vals["level:AGC"].Number != 6 || r.vals["func:NB"].On {
		t.Fatalf("radio after apply: %+v", r.vals)
	}
	sn, _ := svc.LoadSnapshot("ftdx10")
	if sn == nil || len(sn.Saved) != 3 {
		t.Fatalf("snapshot: %+v", sn)
	}
	// A second apply keeps the original "before" value.
	if _, err := svc.Apply(ctx, r, "p", []string{"agc", "power"}, nil); err != nil {
		t.Fatal(err)
	}

	rr, err := svc.Restore(ctx, r)
	if err != nil || rr.Remaining != 0 || len(rr.Restored) != 4 {
		t.Fatalf("restore: %+v %v", rr, err)
	}
	if r.vals["mode:MODE"].Mode != "USB" || r.vals["mode:MODE"].Passband != 2400 || r.vals["level:AGC"].Number != 0 || !r.vals["func:NB"].On || r.vals["level:RFPOWER"].Number != 1 {
		t.Fatalf("radio after restore: %+v", r.vals)
	}
	if sn, _ := svc.LoadSnapshot("ftdx10"); sn != nil {
		t.Errorf("snapshot left after restore: %+v", sn)
	}
}

func TestApplyStopsAtFirstFailure(t *testing.T) {
	ctx := context.Background()
	svc := testService(t, envelope(t, "p", sampleDoc))
	r := newFake(t)
	r.failOn = "AGC"
	out, err := svc.Apply(ctx, r, "p", []string{"mode", "agc", "nb-off"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Applied, []string{"mode"}) || out.Failed != "agc" || !slices.Equal(out.Skipped, []string{"nb-off"}) {
		t.Fatalf("result %+v", out)
	}
	sn, _ := svc.LoadSnapshot("ftdx10")
	if sn == nil || len(sn.Saved) != 1 || sn.Saved[0].Kind != "mode" {
		t.Fatalf("snapshot should hold only what was applied: %+v", sn)
	}
}

func TestLibraryUpdate(t *testing.T) {
	doc := json.RawMessage(sampleDoc)
	body, _ := json.Marshal(map[string]any{"generated_at": "now", "license": "CC-BY-SA-4.0", "profiles": []any{
		map[string]any{"id": "good", "status": "published", "title": "Good", "document": doc},
		map[string]any{"id": "pending-one", "status": "pending", "document": doc},
		map[string]any{"id": "Bad/ID", "status": "published", "document": doc},
	}})
	var gotUA, gotINM string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotINM = r.UserAgent(), r.Header.Get("If-None-Match")
		if r.URL.Path != "/api/v1/dump.json" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if gotINM == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	lib, err := NewLibrary(dir, srv.URL+"/api/v1", "rigctlweb/test")
	if err != nil {
		t.Fatal(err)
	}
	if lib.Status().Available {
		t.Fatal("nothing should be cached yet")
	}
	if err := lib.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := lib.Status(); !st.Available || st.Count != 1 || gotUA != "rigctlweb/test" {
		t.Fatalf("status %+v ua %q", st, gotUA)
	}
	if err := lib.Update(context.Background()); err != nil || gotINM != `"v1"` {
		t.Fatalf("second update: %v, If-None-Match %q", err, gotINM)
	}
	// It survives a restart, offline.
	again, err := NewLibrary(dir, "https://offline.invalid/api/v1", "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Get("good"); !ok {
		t.Fatal("cached library not loaded")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "presets-library.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("library file mode %v", fi.Mode().Perm())
	}
}

func TestLibraryRefusesBadInput(t *testing.T) {
	for _, u := range []string{"http://example.org/api/v1", "ftp://x/", "https://u:p@x/", "https://x/?q=1"} {
		if CheckBaseURL(u) == nil {
			t.Errorf("%s accepted", u)
		}
	}
	for _, u := range []string{"https://rigs.openrms.dev/api/v1", "http://127.0.0.1:8790/api/v1", "http://localhost:1/api/v1"} {
		if err := CheckBaseURL(u); err != nil {
			t.Errorf("%s refused: %v", u, err)
		}
	}
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"profiles":[`))
		chunk := []byte(strings.Repeat(" ", 1<<20))
		for i := 0; i < maxDump>>20+2; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	defer big.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.2:1/elsewhere", http.StatusFound)
	}))
	defer redirect.Close()
	for name, base := range map[string]string{"too big": big.URL, "redirect": redirect.URL} {
		lib, _ := NewLibrary(t.TempDir(), base, "x")
		if err := lib.Update(context.Background()); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if lib.Status().Error == "" {
			t.Errorf("%s: no error shown", name)
		}
	}
}

// Rigs compares the mode name only; a different passband still needs applying.
func TestModePassbandIsNotNoop(t *testing.T) {
	svc := testService(t, envelope(t, "p", sampleDoc))
	r := newFake(t)
	r.vals["mode:MODE"] = rig.Setting{Mode: "PKTUSB", Passband: 2400}
	res, err := svc.Check(context.Background(), r, "p")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Steps {
		if s.ID == "mode" && (s.Status != "ok" || s.Noop) {
			t.Fatalf("mode = %+v, want ok and not a no-op", s)
		}
	}
	r.vals["mode:MODE"] = rig.Setting{Mode: "PKTUSB", Passband: 3000}
	res, _ = svc.Check(context.Background(), r, "p")
	for _, s := range res.Steps {
		if s.ID == "mode" && !s.Noop {
			t.Fatalf("mode = %+v, want a no-op", s)
		}
	}
}

// The openRMS Rigs conformance cases (profile.md, "Checks").
func TestCheckVectors(t *testing.T) {
	b, err := os.ReadFile("testdata/check-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Check struct {
			Cases []struct {
				Name      string  `json:"name"`
				Step      Step    `json:"step"`
				Current   any     `json:"current"`
				Tolerance float64 `json:"tolerance"`
				Status    string  `json:"status"`
			} `json:"cases"`
		} `json:"check"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Check.Cases) < 20 {
		t.Fatalf("only %d check cases", len(v.Check.Cases))
	}
	for _, c := range v.Check.Cases {
		tol := c.Tolerance
		got, _ := compare(c.Step, c.Current, levelTolerance(c.Step, nil, &tol))
		if got != c.Status {
			t.Errorf("%s: got %s, want %s", c.Name, got, c.Status)
		}
	}
}

// The dump's plan can raise safety (never lower it) and supplies tolerance and menu paths.
func TestServerPlanFields(t *testing.T) {
	e := envelope(t, "p", sampleDoc)
	tol := 0.2
	e.Plan = &ServerPlan{Steps: []ServerPlanStep{
		{ID: "nb-off", Safety: "tx", Menu: []string{"FUNC", "NB"}},
		{ID: "power", Safety: "rx"},
		{ID: "agc", Tolerance: &tol},
	}}
	v := Plan(e, ftdx10Caps(t))
	if s := stepOf(v, "nb-off"); s.Safety != "tx" || len(s.Menu) != 2 {
		t.Errorf("nb-off: %+v", s)
	}
	if stepOf(v, "power").Safety != "tx" {
		t.Error("the server lowered RFPOWER to rx")
	}
	if s := stepOf(v, "agc"); s.Tolerance == nil || *s.Tolerance != 0.2 {
		t.Errorf("agc tolerance: %+v", s.Tolerance)
	}
}

func TestCATApplyRestore(t *testing.T) {
	ctx := context.Background()
	svc := testService(t, envelope(t, "p", sampleDoc))
	r := newFake(t)

	// The operator must confirm exactly the commands that will be sent.
	for _, confirm := range [][]string{nil, {"EX0104160;"}, {"EX0104161;", "EX0104161;"}, {"EX0104161;", "TX1;"}} {
		if _, err := svc.Apply(ctx, r, "p", []string{"rear", "nb-off"}, confirm); !errors.Is(err, ErrConfirmCAT) {
			t.Errorf("confirm %v: %v", confirm, err)
		}
	}
	if _, err := svc.Apply(ctx, r, "p", []string{"nb-off"}, []string{"EX0104161;"}); !errors.Is(err, ErrConfirmCAT) {
		t.Errorf("a confirmation for a step not chosen was accepted: %v", err)
	}
	r.ptt = true
	if _, err := svc.Apply(ctx, r, "p", []string{"rear"}, []string{"EX0104161;"}); err == nil {
		t.Error("raw CAT applied while transmitting")
	}
	r.ptt = false
	if len(r.sent) != 0 || len(r.catOut) != 0 {
		t.Fatalf("sent during refusals: %v %v", r.sent, r.catOut)
	}

	out, err := svc.Apply(ctx, r, "p", []string{"rear", "shift", "nb-off"}, []string{"EX0104081500;", "EX0104161;"})
	if err != nil || len(out.Applied) != 3 {
		t.Fatalf("apply: %+v %v", out, err)
	}
	if r.cat["EX010416"] != "1" || !slices.Equal(r.catOut, []string{"EX0104161;", "EX0104081500;"}) {
		t.Fatalf("radio: %v sent %v", r.cat, r.catOut)
	}
	sn, _ := svc.LoadSnapshot("ftdx10")
	var saved *Saved
	for i := range sn.Saved {
		if sn.Saved[i].Name == "EX010416;" {
			saved = &sn.Saved[i]
		}
	}
	if saved == nil || saved.Cat == nil || saved.Cat.Before != "0" || !saved.Applied {
		t.Fatalf("snapshot: %+v", sn.Saved)
	}
	res, _ := svc.Check(ctx, r, "p")
	for _, s := range res.Steps {
		if s.ID == "rear" && (s.Status != "ok" || s.Text != "USB") {
			t.Errorf("check after apply: %+v", s)
		}
	}

	r.catOut = nil
	rr, err := svc.Restore(ctx, r)
	if err != nil || rr.Remaining != 0 || len(rr.Restored) != 3 {
		t.Fatalf("restore: %+v %v", rr, err)
	}
	if r.cat["EX010416"] != "0" || !slices.Contains(r.catOut, "EX0104160;") {
		t.Fatalf("after restore: %v sent %v", r.cat, r.catOut)
	}
	b, err := os.ReadFile(filepath.Join(svc.dir, "presets", "cat-log.jsonl"))
	if err != nil || strings.Count(string(b), `"use":"set"`) != 4 || !strings.Contains(string(b), `"command":"EX0104160;"`) ||
		!strings.Contains(string(b), `"use":"read"`) {
		t.Errorf("CAT log: %v %s", err, b)
	}
	if fi, err := os.Stat(filepath.Join(svc.dir, "presets", "cat-log.jsonl")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("CAT log mode: %v", err)
	}

	// A radio that ignores the set: the step fails on read-back, and Restore still covers it.
	r.stuck = true
	out, _ = svc.Apply(ctx, r, "p", []string{"rear"}, []string{"EX0104161;"})
	if out.Failed != "rear" || !strings.Contains(out.Error, "reads back") {
		t.Fatalf("stuck radio: %+v", out)
	}
	if sn, _ := svc.LoadSnapshot("ftdx10"); sn == nil || len(sn.Saved) != 1 || !sn.Saved[0].Applied {
		t.Fatalf("stuck radio snapshot: %+v", sn)
	}

	// Read-only radios: raw CAT stays off, reads included.
	r.ro = true
	res, _ = svc.Check(ctx, r, "p")
	for _, s := range res.Steps {
		if s.ID == "rear" && s.Status != "manual" {
			t.Errorf("read-only check of a cat step: %+v", s)
		}
	}
}

func TestRenderCat(t *testing.T) {
	for _, c := range []struct {
		tpl, val, want string
		bad            bool
	}{
		{"EX010417{value:03};", "50", "EX010417050;", false},
		{"EX010417{value:03};", "1000", "", true},
		{"EX010417{value:03};", "AB", "", true},
		{"EX010415{value};", "1", "EX0104151;", false},
	} {
		got, err := renderCat(c.tpl, c.val)
		if (err != nil) != c.bad || !c.bad && got != c.want {
			t.Errorf("%s %s = %q %v", c.tpl, c.val, got, err)
		}
	}
	if zeroFill("EX010417{value:03};") != "EX010417000;" || zeroFill("PC{value};") != "PC0;" {
		t.Error("zero fill")
	}
	if v, ok := catValue(1.5); ok {
		t.Errorf("fraction accepted: %s", v)
	}
}

// A reply pattern must not pick the value out of the wrong place, and every read Check sends is logged.
func TestCATReplyMustEchoTheRead(t *testing.T) {
	doc := `{"schema":"openrms-rigs/profile/v1","title":"x","rig":{"mfg":"Yaesu","name":"FTDX-10"},"steps":[
		{"id":"loose","kind":"cat","name":"RPORT GAIN","value":50,"cat":{"protocol":"ascii","read":"EX010417;","reply":"(\\d{3})","set":"EX010417{value:03};"}}]}`
	ctx := context.Background()
	svc := testService(t, envelope(t, "loose", doc))
	r := newFake(t)
	r.cat["EX010417"] = "050"
	res, err := svc.Check(ctx, r, "loose")
	if err != nil || res.Steps[0].Status != "manual" {
		t.Fatalf("loose pattern accepted: %+v %v", res, err)
	}
	if _, err := svc.Apply(ctx, r, "loose", []string{"loose"}, []string{"EX010417050;"}); err == nil || len(r.catOut) != 0 || len(r.catIn) != 0 {
		t.Fatalf("sent with a loose reply pattern: %v %v %v", err, r.catIn, r.catOut)
	}
	if _, err := catCapture(regexp.MustCompile(`^EX010417(\d{3});$`), "EX010417;", "EX010417050;"); err != nil {
		t.Error(err)
	}
}

// "PC100;" passes the allow-list as a read but sets the power: it must never be sent as one.
func TestSetShapedReadIsNeverSent(t *testing.T) {
	doc := `{"schema":"openrms-rigs/profile/v1","title":"x","rig":{"mfg":"Yaesu","name":"FTDX-10"},"steps":[
		{"id":"pc-check","kind":"cat","name":"POWER","check_only":true,"cat":{"protocol":"ascii","read":"PC100;","reply":"^PC100(\\d*);$"}},
		{"id":"pc-tpl","kind":"cat","name":"POWER","check_only":true,"cat":{"protocol":"ascii","read":"PC100;","reply":"^PC(\\d{3});$","set":"PC{value:03};"}},
		{"id":"pc-set","kind":"cat","name":"POWER","value":50,"cat":{"protocol":"ascii","read":"PC100;","reply":"^PC(\\d{3});$","set":"PC{value:03};"}},
		{"id":"pc-bare","kind":"cat","name":"POWER","check_only":true,"cat":{"protocol":"ascii","read":"PC;","reply":"^PC(\\d{3});$"}}]}`
	ctx := context.Background()
	e := envelope(t, "pc", doc)
	if Refusal(e.Document) != "" {
		t.Fatalf("refused as a whole (the allow-list allows PC reads): %s", Refusal(e.Document))
	}
	v := Plan(e, ftdx10Caps(t))
	for _, s := range v.Steps {
		if s.Readable || s.Applicable {
			t.Errorf("%s: readable=%v applicable=%v", s.ID, s.Readable, s.Applicable)
		}
	}
	svc := testService(t, e)
	r := newFake(t)
	if _, err := svc.Check(ctx, r, "pc"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, r, "pc", []string{"pc-set"}, []string{"PC050;"}); err == nil {
		t.Error("applied a step with a set-shaped read")
	}
	if len(r.catIn) != 0 || len(r.catOut) != 0 {
		t.Fatalf("sent reads %v sets %v", r.catIn, r.catOut)
	}
}

// Meters can't be set, so their read needs no set template (step_rules "read").
func TestMeterReadWithoutSet(t *testing.T) {
	doc := `{"schema":"openrms-rigs/profile/v1","title":"x","rig":{"mfg":"Yaesu","name":"FTDX-10"},"steps":[
		{"id":"s","kind":"cat","name":"S-METER","check_only":true,"cat":{"protocol":"ascii","read":"SM0;","reply":"^SM0(\\d{3});$"}}]}`
	ctx := context.Background()
	svc := testService(t, envelope(t, "m", doc))
	r := newFake(t)
	r.cat["SM0"] = "120"
	if s := stepOf(Plan(envelope(t, "m", doc), r.caps), "s"); !s.Readable || s.Applicable {
		t.Fatalf("meter step: %+v", s)
	}
	res, err := svc.Check(ctx, r, "m")
	if err != nil || res.Steps[0].Status != "ok" || !slices.Equal(r.catIn, []string{"SM0;"}) {
		t.Fatalf("meter check: %+v %v %v", res, err, r.catIn)
	}
}
