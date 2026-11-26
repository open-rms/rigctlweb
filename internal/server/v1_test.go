package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/open-rms/rigctlweb/internal/integration"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

func TestHealthIsOpenAndSaysOnlyOK(t *testing.T) {
	ts, _ := newTestServer(t)
	res, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || strings.TrimSpace(string(b)) != `{"status":"ok"}` {
		t.Fatalf("health = %d %s", res.StatusCode, b)
	}
}

func TestIntegrationHandshake(t *testing.T) {
	ts, tok := newTestServer(t)
	call := func(method, path, body string, auth bool) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		if auth {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	get := func() integration.Info {
		t.Helper()
		res := call("GET", "/api/v1/integration", "", true)
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("handshake = %d", res.StatusCode)
		}
		var info integration.Info
		if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
			t.Fatal(err)
		}
		return info
	}

	// It reveals rig ids, so it needs the token like the rest of the API.
	if res := call("GET", "/api/v1/integration", "", false); res.StatusCode != 401 {
		t.Fatalf("without a token = %d, want 401", res.StatusCode)
	}

	info := get()
	if info.Name != "rigctlweb" || info.Version != "test" || info.Mode != "standalone" ||
		len(info.API) != 1 || info.API[0] != "v1" || info.Transmits || info.Rigs == nil || len(info.Rigs) != 0 {
		t.Fatalf("no rigs: %+v", info)
	}

	if res := call("POST", "/api/rigs", `{"name":"RO","addr":"127.0.0.1:1","readOnly":true}`, true); res.StatusCode != 201 {
		t.Fatalf("add read-only rig = %d", res.StatusCode)
	}
	if info = get(); info.Transmits || len(info.Rigs) != 1 || !info.Rigs[0].ReadOnly {
		t.Fatalf("read-only rig only: %+v", info)
	}

	if res := call("POST", "/api/rigs", `{"name":"TX","addr":"127.0.0.1:2"}`, true); res.StatusCode != 201 {
		t.Fatalf("add transmitting rig = %d", res.StatusCode)
	}
	if info = get(); !info.Transmits || len(info.Rigs) != 2 {
		t.Fatalf("with a transmitting rig: %+v", info)
	}
}

// The handshake's field names are a published contract (hamstack decodes exactly these).
func TestIntegrationFieldNamesAreStable(t *testing.T) {
	b, _ := json.Marshal(integration.Info{Rigs: []integration.InfoRig{{}}})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"name", "version", "api", "mode", "capabilities", "transmits", "rigs"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing field %q in %s", k, b)
		}
	}
	rig := m["rigs"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "readOnly"} {
		if _, ok := rig[k]; !ok {
			t.Errorf("missing rig field %q in %s", k, b)
		}
	}
}

// docs/openapi.json is the committed description. Regenerate it with:
//
//	go run ./cmd/rigctlweb openapi > docs/openapi.json
func TestOpenAPIMatchesCommittedFile(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := OpenAPIJSON("", DefaultServerURL); string(got) != string(want) {
		t.Fatal("docs/openapi.json is out of date: run go run ./cmd/rigctlweb openapi > docs/openapi.json")
	}
}

// Published routes are registered only from the route table, so the document lists exactly
// what is served. This catches a /api/v1 route registered by hand elsewhere.
func TestV1RoutesOnlyFromTable(t *testing.T) {
	for _, f := range []string{"server.go", "audio.go", "auth.go"} {
		b, _ := os.ReadFile(f)
		if regexp.MustCompile(`Handle(Func)?\("([A-Z]+ )?/api/v1`).Match(b) {
			t.Errorf("%s registers a /api/v1 route outside v1Routes", f)
		}
	}
	spec := openAPI("", "/")
	paths := spec["paths"].(map[string]any)
	for _, r := range v1Routes(nil) {
		if _, ok := paths[r.Path].(map[string]any)[strings.ToLower(r.Method)]; !ok {
			t.Errorf("%s %s missing from the document", r.Method, r.Path)
		}
	}
}

// Every published field says what it is.
func TestEverySchemaFieldDocumented(t *testing.T) {
	schemas := openAPI("", "/")["components"].(map[string]any)["schemas"].(map[string]any)
	for name, s := range schemas {
		for field, p := range s.(map[string]any)["properties"].(map[string]any) {
			if d, _ := p.(map[string]any)["description"].(string); d == "" {
				t.Errorf("%s.%s has no description (add a doc tag)", name, field)
			}
		}
	}
}

// Field names are part of the v1 contract: only additions are allowed. If this fails because a
// name changed or went away, that is a breaking change and needs /api/v2.
func TestV1PublishedNamesArePinned(t *testing.T) {
	pinned := map[string][]string{
		"RigState":           {"id", "online", "frequencyHz", "mode", "passbandHz", "vfo", "split", "txVfo", "txFrequencyHz", "transmitting", "ritHz", "xitHz", "levels", "functions", "parameters", "updated"},
		"Rig":                {"id", "name", "online", "readOnly"},
		"RigCapabilities":    {"model", "manufacturer", "hamlib", "modes", "vfos", "levels", "functions", "maxRitHz", "maxXitHz", "canSplit"},
		"LevelCapability":    {"name", "min", "max", "step", "readable", "settable"},
		"FunctionCapability": {"name", "readable", "settable"},
		"Info":               {"name", "version", "api"},
		"IntegrationInfo":    {"name", "version", "api", "mode", "capabilities", "transmits", "rigs"},
		"IntegrationRig":     {"id", "readOnly"},
		"Problem":            {"type", "title", "status"},
		"Health":             {"status"},
		"FrequencyRequest":   {"hz"},
		"ModeRequest":        {"mode"},
		"VFORequest":         {"vfo"},
		"SplitRequest":       {"enabled"},
		"OffsetRequest":      {"hz"},
		"LevelRequest":       {"value"},
		"SwitchRequest":      {"enabled"},
	}
	schemas := openAPI("", "/")["components"].(map[string]any)["schemas"].(map[string]any)
	for name, fields := range pinned {
		s, ok := schemas[name].(map[string]any)
		if !ok {
			t.Errorf("schema %s is gone", name)
			continue
		}
		props := s["properties"].(map[string]any)
		for _, f := range fields {
			if _, ok := props[f]; !ok {
				t.Errorf("%s.%s is gone", name, f)
			}
		}
	}
	paths := openAPI("", "/")["paths"].(map[string]any)
	for _, p := range []string{"/api/health", "/api/v1/info", "/api/v1/integration", "/api/v1/rigs", "/api/v1/rigs/{id}",
		"/api/v1/rigs/{id}/state", "/api/v1/rigs/{id}/events", "/api/v1/rigs/{id}/capabilities", "/api/v1/rigs/{id}/frequency",
		"/api/v1/rigs/{id}/mode", "/api/v1/rigs/{id}/vfo", "/api/v1/rigs/{id}/split", "/api/v1/rigs/{id}/rit",
		"/api/v1/rigs/{id}/xit", "/api/v1/rigs/{id}/levels/{name}", "/api/v1/rigs/{id}/functions/{name}"} {
		if _, ok := paths[p]; !ok {
			t.Errorf("route %s is gone", p)
		}
	}
}

func TestV1AgainstDummyRig(t *testing.T) {
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

	ts, tok := newTestServer(t)
	call := func(method, path, body string) (int, map[string]any, http.Header) {
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
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		return res.StatusCode, m, res.Header
	}
	for _, b := range []string{`{"name":"D","addr":"` + addr + `"}`, `{"name":"RO","addr":"` + addr + `","readOnly":true}`} {
		if code, _, _ := call("POST", "/api/rigs", b); code != 201 {
			t.Fatalf("add rig = %d", code)
		}
	}

	// Errors are problem details.
	res, _ := http.Get(ts.URL + "/api/v1/rigs")
	if res.StatusCode != 401 || res.Header.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("no token = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if code, m, h := call("GET", "/api/v1/rigs/nope/state", ""); code != 404 || m["status"] != 404.0 || h.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unknown rig = %d %v", code, m)
	}

	if code, _, h := call("GET", "/api/v1/nope", ""); code != 404 || h.Get("Content-Type") != "application/problem+json" {
		t.Errorf("unknown route = %d %s", code, h.Get("Content-Type"))
	}
	if code, _, h := call("POST", "/api/v1/rigs/d/frequency", `{"hz":1}`); code != 405 || h.Get("Allow") != "PUT" || h.Get("Content-Type") != "application/problem+json" {
		t.Errorf("wrong method = %d allow=%q %s", code, h.Get("Allow"), h.Get("Content-Type"))
	}
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		if _, m, _ := call("GET", "/api/v1/rigs/d", ""); m["online"] == true {
			if _, m, _ := call("GET", "/api/v1/rigs/ro", ""); m["online"] == true {
				break
			}
		}
		if time.Now().After(end) {
			t.Fatal("dummy rig never came online")
		}
	}

	code, caps, _ := call("GET", "/api/v1/rigs/d/capabilities", "")
	if code != 200 || !strings.Contains(fmt.Sprint(caps["modes"]), "USB") {
		t.Fatalf("capabilities = %d %v", code, caps)
	}
	for _, f := range caps["functions"].([]any) {
		f := f.(map[string]any)
		if keyingFunc(f["name"].(string)) && f["settable"] == true {
			t.Errorf("%s is advertised as settable", f["name"])
		}
	}

	if code, st, _ := call("PUT", "/api/v1/rigs/d/frequency", `{"hz":14074000}`); code != 200 || st["frequencyHz"] != 14074000.0 {
		t.Fatalf("set frequency = %d %v", code, st)
	}
	if code, st, _ := call("PUT", "/api/v1/rigs/d/mode", `{"mode":"USB"}`); code != 200 || st["mode"] != "USB" {
		t.Fatalf("set mode = %d %v", code, st)
	}
	if code, _, _ := call("PUT", "/api/v1/rigs/d/mode", `{"mode":"NOPE"}`); code != 400 {
		t.Errorf("bad mode = %d, want 400", code)
	}
	if code, _, _ := call("PUT", "/api/v1/rigs/d/mode", `{"mode":"USB","bogus":1}`); code != 400 {
		t.Errorf("unknown field = %d, want 400", code)
	}
	if code, _, _ := call("PUT", "/api/v1/rigs/d/levels/AF", `{"value":0.4}`); code != 200 {
		t.Errorf("set AF = %d", code)
	}
	if code, _, _ := call("PUT", "/api/v1/rigs/d/levels/NOPE", `{"value":1}`); code != 422 {
		t.Errorf("unknown level = %d, want 422", code)
	}
	for _, f := range []string{"VOX", "SBKIN", "FBKIN", "TBURST", "SEND_MORSE", "vox"} {
		if code, _, _ := call("PUT", "/api/v1/rigs/d/functions/"+f, `{"enabled":true}`); code != 403 {
			t.Errorf("function %s = %d, want 403", f, code)
		}
	}
	for _, f := range []string{"TUNER", "TRANSCEIVE", "SCEN", "NOT_A_FUNC"} {
		if code, _, _ := call("PUT", "/api/v1/rigs/d/functions/"+f, `{"enabled":true}`); code != 403 {
			t.Errorf("function %s = %d, want 403 (not on the allow-list)", f, code)
		}
	}
	// A missing required field is an error, never a silent zero or "off".
	for _, c := range []struct{ path, body string }{
		{"/api/v1/rigs/d/levels/AF", `{}`}, {"/api/v1/rigs/d/functions/NB", `{}`}, {"/api/v1/rigs/d/split", `{}`},
		{"/api/v1/rigs/d/rit", `{}`}, {"/api/v1/rigs/d/xit", `{"hz":null}`}, {"/api/v1/rigs/d/frequency", `{}`},
		{"/api/v1/rigs/d/mode", `{"passbandHz":2400}`}, {"/api/v1/rigs/d/vfo", `[]`},
	} {
		if code, m, _ := call("PUT", c.path, c.body); code != 400 {
			t.Errorf("PUT %s %s = %d %v, want 400", c.path, c.body, code, m)
		}
	}
	if _, st, _ := call("GET", "/api/v1/rigs/d/state", ""); st["levels"].(map[string]any)["AF"] == 0.0 {
		t.Error("an empty body changed AF")
	}
	if code, _, _ := call("PUT", "/api/v1/rigs/d/functions/NB", `{"enabled":true}`); code != 200 {
		t.Errorf("set NB = %d", code)
	}
	if code, _, _ := call("PUT", "/api/v1/rigs/ro/frequency", `{"hz":7074000}`); code != 403 {
		t.Errorf("read-only rig = %d, want 403", code)
	}

	// Server-Sent Events: the current state comes first.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/rigs/d/events", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err = http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := make([]byte, 4096)
	n, _ := io.ReadAtLeast(res.Body, buf, 20)
	if res.Header.Get("Content-Type") != "text/event-stream" || !strings.HasPrefix(string(buf[:n]), "event: state\ndata: {") ||
		!strings.Contains(string(buf[:n]), `"frequencyHz":14074000`) {
		t.Fatalf("events = %s %q", res.Header.Get("Content-Type"), buf[:n])
	}

	// Nothing in this test may have keyed the radio.
	ext := rigctl.NewClient(addr)
	defer ext.Close()
	if on, err := ext.GetPTT(context.Background()); err != nil || on {
		t.Fatalf("PTT = %v, %v", on, err)
	}
}

// The functions v1 may change are real Hamlib functions, and none of them can transmit.
func TestV1FunctionAllowList(t *testing.T) {
	for f := range v1Funcs {
		if keyingFunc(f) {
			t.Errorf("%s is on the allow-list but can make the radio transmit", f)
		}
	}
	b, err := os.ReadFile("/usr/include/hamlib/rig.h")
	if err != nil {
		t.Skip("hamlib headers not installed")
	}
	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`#define RIG_FUNC_([A-Z0-9_]+) `).FindAllSubmatch(b, -1) {
		defined[string(m[1])] = true
	}
	for f := range v1Funcs {
		if !defined[f] {
			t.Errorf("%s is not a Hamlib function in rig.h", f)
		}
	}
}
