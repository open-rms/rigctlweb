package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/open-rms/rigctlweb/internal/integration"
	"github.com/open-rms/rigctlweb/internal/rig"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// The versioned API. Everything under /api/v1/ is a published contract: within v1 fields are
// only ever added, never renamed, removed or given a new meaning, and clients ignore fields they
// do not know. A breaking change is /api/v2, with v1 kept alongside it. The unversioned routes
// (/api/rigs, /api/views, /ws ...) are private to rigctlWeb's own UI and change freely.
// See docs/API.md; the OpenAPI document is generated from v1Routes (openapi.go).
//
// v1 has its own types rather than serialising internal ones, so internal changes never leak
// into the contract. v1 never keys the transmitter (see keyingFuncs).

// apiVersions lists the versioned API majors this build serves.
var apiVersions = []string{"v1"}

// capabilities are the features this build offers, as reported in the handshake.
var capabilities = []string{"rig.control", "audio.monitor"}

// ---- types (published: add fields only) ----

type v1Info struct {
	Name    string   `json:"name" doc:"Always rigctlweb."`
	Version string   `json:"version" doc:"Program version."`
	API     []string `json:"api" doc:"Versioned API majors served, e.g. [\"v1\"]."`
}

type v1Health struct {
	Status string `json:"status" enum:"ok" doc:"ok when the program can serve. Says nothing about radios."`
}

type v1Rig struct {
	ID       string `json:"id" doc:"Stable id used in every per-radio route."`
	Name     string `json:"name" doc:"Display name."`
	Online   bool   `json:"online" doc:"True while rigctld and the radio answer."`
	ReadOnly bool   `json:"readOnly" doc:"True when rigctlWeb never changes this radio. Control routes answer 403."`
}

type v1State struct {
	ID            string             `json:"id" doc:"Radio id."`
	Online        bool               `json:"online" doc:"True while rigctld and the radio answer."`
	Error         string             `json:"error,omitempty" doc:"Last connection problem, when offline."`
	FrequencyHz   int64              `json:"frequencyHz" doc:"Current VFO frequency in hertz."`
	Mode          string             `json:"mode" doc:"Hamlib mode name, e.g. USB, LSB, CW, FM, PKTUSB."`
	PassbandHz    int                `json:"passbandHz" doc:"Filter passband in hertz (0 when unknown)."`
	VFO           string             `json:"vfo" doc:"Current VFO, e.g. VFOA, VFOB, Main."`
	Split         bool               `json:"split" doc:"True when transmitting on a different VFO."`
	TxVFO         string             `json:"txVfo" doc:"Transmit VFO when split is on, otherwise empty."`
	TxFrequencyHz int64              `json:"txFrequencyHz" doc:"Transmit frequency in hertz when split is on, otherwise 0."`
	Transmitting  bool               `json:"transmitting" doc:"True while the radio is keyed (PTT on), by anyone."`
	RitHz         int                `json:"ritHz" doc:"Receive incremental tuning offset in hertz (0 = off)."`
	XitHz         int                `json:"xitHz" doc:"Transmit incremental tuning offset in hertz (0 = off)."`
	Levels        map[string]float64 `json:"levels" doc:"Hamlib levels by name, e.g. AF, RFPOWER, STRENGTH (dB relative to S9), SWR."`
	Functions     map[string]bool    `json:"functions" doc:"Hamlib functions by name, e.g. NB, NR, COMP."`
	Parameters    map[string]string  `json:"parameters" doc:"Hamlib parameters by name, as text."`
	Updated       time.Time          `json:"updated" doc:"When the state last changed."`
}

type v1Level struct {
	Name     string  `json:"name" doc:"Hamlib level name."`
	Min      float64 `json:"min" doc:"Lowest value (min = max = 0 when the range is unknown)."`
	Max      float64 `json:"max" doc:"Highest value."`
	Step     float64 `json:"step" doc:"Step between values (0 = continuous)."`
	Readable bool    `json:"readable" doc:"The radio reports this level."`
	Settable bool    `json:"settable" doc:"PUT /levels/{name} can change it."`
}

type v1Function struct {
	Name     string `json:"name" doc:"Hamlib function name."`
	Readable bool   `json:"readable" doc:"The radio reports this function."`
	Settable bool   `json:"settable" doc:"PUT /functions/{name} can change it. v1 changes only receive-side and setup functions, never ones that can make the radio transmit."`
}

type v1Capabilities struct {
	Model        string       `json:"model" doc:"Radio model as Hamlib names it."`
	Manufacturer string       `json:"manufacturer" doc:"Manufacturer as Hamlib names it."`
	Hamlib       string       `json:"hamlib" doc:"Hamlib backend version."`
	Modes        []string     `json:"modes" doc:"Modes PUT /mode accepts."`
	VFOs         []string     `json:"vfos" doc:"VFOs PUT /vfo accepts."`
	Levels       []v1Level    `json:"levels" doc:"Standard Hamlib levels."`
	Functions    []v1Function `json:"functions" doc:"Standard Hamlib functions."`
	MaxRitHz     int          `json:"maxRitHz" doc:"Largest RIT offset in hertz (0 = no RIT)."`
	MaxXitHz     int          `json:"maxXitHz" doc:"Largest XIT offset in hertz (0 = no XIT)."`
	CanSplit     bool         `json:"canSplit" doc:"The radio supports split operation."`
}

type v1Problem struct {
	Type   string `json:"type" doc:"Always about:blank for now; use status and title."`
	Title  string `json:"title" doc:"Short summary of the problem class."`
	Status int    `json:"status" doc:"HTTP status code."`
	Detail string `json:"detail,omitempty" doc:"What went wrong, for a person."`
}

type v1Frequency struct {
	Hz int64 `json:"hz" doc:"Frequency in hertz."`
}

type v1Mode struct {
	Mode       string `json:"mode" doc:"A mode from the radio's capabilities."`
	PassbandHz int    `json:"passbandHz,omitempty" doc:"Passband in hertz; 0 or absent keeps the radio's default for the mode."`
}

type v1VFO struct {
	VFO string `json:"vfo" doc:"A VFO from the radio's capabilities."`
}

type v1Split struct {
	Enabled bool   `json:"enabled" doc:"Turn split on or off."`
	TxVFO   string `json:"txVfo,omitempty" doc:"Transmit VFO; defaults to VFOB."`
}

type v1Offset struct {
	Hz int `json:"hz" doc:"Offset in hertz, within the radio's limit (0 = off)."`
}

type v1LevelValue struct {
	Value float64 `json:"value" doc:"New value, within the level's range."`
}

type v1Switch struct {
	Enabled bool `json:"enabled" doc:"On or off."`
}

var typeDocs = map[reflect.Type]string{
	reflect.TypeOf(v1Problem{}):        "RFC 9457 problem details.",
	reflect.TypeOf(integration.Info{}): "Who this program is and what it may do. hamstack reads it as its handshake.",
}

// schemaNames are the published component names (part of the contract, like field names).
var schemaNames = map[reflect.Type]string{
	reflect.TypeOf(integration.Info{}):    "IntegrationInfo",
	reflect.TypeOf(integration.InfoRig{}): "IntegrationRig",
	reflect.TypeOf(v1State{}):             "RigState",
	reflect.TypeOf(v1Capabilities{}):      "RigCapabilities",
	reflect.TypeOf(v1Level{}):             "LevelCapability",
	reflect.TypeOf(v1Function{}):          "FunctionCapability",
	reflect.TypeOf(v1Frequency{}):         "FrequencyRequest",
	reflect.TypeOf(v1Mode{}):              "ModeRequest",
	reflect.TypeOf(v1VFO{}):               "VFORequest",
	reflect.TypeOf(v1Split{}):             "SplitRequest",
	reflect.TypeOf(v1Offset{}):            "OffsetRequest",
	reflect.TypeOf(v1LevelValue{}):        "LevelRequest",
	reflect.TypeOf(v1Switch{}):            "SwitchRequest",
}

// v1Funcs are the Hamlib functions v1 may change (rig.SafeFuncs, shared with openRMS Rigs
// presets); keyingFunc names the ones that transmit by themselves.
var v1Funcs = rig.SafeFuncs

func keyingFunc(name string) bool { return rig.IsKeyingFunc(name) }

// ---- routes ----

type v1Route struct {
	Method, Path string
	ID, Tag      string
	Summary      string
	Description  string
	Body         any // zero value of the request body type, or nil
	Resp         any // zero value of the response type
	RespDoc      string
	Stream       bool // Server-Sent Events of Resp
	Public       bool // no auth
	Errors       []int
	H            http.HandlerFunc
}

var ctlErrors = []int{400, 401, 403, 404, 422, 502, 503, 504}

// v1Routes is the single list of published routes: it registers the handlers and generates the
// OpenAPI document. s may be nil when only the description is needed.
func v1Routes(s *Server) []v1Route {
	h := func(f func(*Server, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		if s == nil {
			return nil
		}
		return func(w http.ResponseWriter, r *http.Request) { f(s, w, r) }
	}
	return []v1Route{
		{Method: "GET", Path: "/api/health", ID: "health", Tag: "About", Public: true,
			Summary: "Health", Description: "200 when rigctlWeb can serve. Needs no token and does not depend on any radio.",
			Resp: v1Health{}, RespDoc: "Serving."},
		{Method: "GET", Path: "/api/v1/info", ID: "getInfo", Tag: "About",
			Summary: "Program and API versions", Resp: v1Info{}, RespDoc: "Versions.", Errors: []int{401},
			H: h((*Server).v1GetInfo)},
		{Method: "GET", Path: "/api/v1/integration", ID: "getIntegration", Tag: "About",
			Summary: "Integration handshake", Description: "Mode, features and radios, with whether each may transmit. hamstack calls this to check an integration.",
			Resp: integration.Info{}, RespDoc: "Handshake.", Errors: []int{401, 403},
			H: h(func(s *Server, w http.ResponseWriter, r *http.Request) {
				integration.Handler(s.integrationInfo).ServeHTTP(w, r)
			})},
		{Method: "GET", Path: "/api/v1/openapi.json", ID: "getOpenAPI", Tag: "About",
			Summary: "This OpenAPI document", Resp: map[string]any{}, RespDoc: "OpenAPI 3.1 document.", Errors: []int{401},
			H: h((*Server).v1GetOpenAPI)},
		{Method: "GET", Path: "/api/v1/rigs", ID: "listRigs", Tag: "Radios",
			Summary: "List radios", Resp: []v1Rig{}, RespDoc: "Every configured radio.", Errors: []int{401},
			H: h((*Server).v1ListRigs)},
		{Method: "GET", Path: "/api/v1/rigs/{id}", ID: "getRig", Tag: "Radios",
			Summary: "One radio", Resp: v1Rig{}, RespDoc: "The radio.", Errors: []int{401, 404},
			H: h((*Server).v1GetRig)},
		{Method: "GET", Path: "/api/v1/rigs/{id}/state", ID: "getRigState", Tag: "Radios",
			Summary: "Current state", Description: "Frequency, mode, meters and settings as last read from the radio (it is polled several times a second).",
			Resp: v1State{}, RespDoc: "State.", Errors: []int{401, 404},
			H: h((*Server).v1GetState)},
		{Method: "GET", Path: "/api/v1/rigs/{id}/events", ID: "streamRigState", Tag: "Radios", Stream: true,
			Summary: "Stream state changes", Description: "Server-Sent Events. Sends `event: state` with the full RigState as `data` right away and after every change (latest wins, so a slow reader skips frames), and a `: ping` comment every 15 seconds.",
			Resp: v1State{}, RespDoc: "An endless text/event-stream; each event's data is a RigState.", Errors: []int{401, 404},
			H: h((*Server).v1Events)},
		{Method: "GET", Path: "/api/v1/rigs/{id}/capabilities", ID: "getRigCapabilities", Tag: "Radios",
			Summary: "What the radio can do", Description: "From Hamlib's dump_caps. Lists what the control routes accept.",
			Resp: v1Capabilities{}, RespDoc: "Capabilities.", Errors: []int{401, 404, 503},
			H: h((*Server).v1GetCaps)},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/frequency", ID: "setFrequency", Tag: "Control",
			Summary: "Set the frequency", Body: v1Frequency{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h(ctl(func(b v1Frequency) rig.Command { return cmd("set_freq", "hz", float64(b.Hz)) }))},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/mode", ID: "setMode", Tag: "Control",
			Summary: "Set the mode and passband", Body: v1Mode{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h(ctl(func(b v1Mode) rig.Command {
				c := cmd("set_mode", "mode", b.Mode)
				if b.PassbandHz > 0 {
					c.Args["passband"] = float64(b.PassbandHz)
				}
				return c
			}))},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/vfo", ID: "setVfo", Tag: "Control",
			Summary: "Select the VFO", Body: v1VFO{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h(ctl(func(b v1VFO) rig.Command { return cmd("set_vfo", "vfo", b.VFO) }))},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/split", ID: "setSplit", Tag: "Control",
			Summary: "Turn split on or off", Body: v1Split{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h(ctl(func(b v1Split) rig.Command {
				c := cmd("set_split", "on", b.Enabled)
				if b.TxVFO != "" {
					c.Args["txVfo"] = b.TxVFO
				}
				return c
			}))},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/rit", ID: "setRit", Tag: "Control",
			Summary: "Set the RIT offset", Body: v1Offset{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h(ctl(func(b v1Offset) rig.Command { return cmd("set_rit", "hz", float64(b.Hz)) }))},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/xit", ID: "setXit", Tag: "Control",
			Summary: "Set the XIT offset", Body: v1Offset{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h(ctl(func(b v1Offset) rig.Command { return cmd("set_xit", "hz", float64(b.Hz)) }))},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/levels/{name}", ID: "setLevel", Tag: "Control",
			Summary: "Set a level", Description: "Standard Hamlib levels only (see capabilities, settable).",
			Body: v1LevelValue{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h((*Server).v1SetLevel)},
		{Method: "PUT", Path: "/api/v1/rigs/{id}/functions/{name}", ID: "setFunction", Tag: "Control",
			Summary: "Turn a function on or off", Description: "Receive-side and setup functions only (noise blanker, noise reduction, notch, AGC, squelch, RIT/XIT, monitor, scope and similar); the capabilities' settable flag says which. Anything else answers 403, including every function that can make the radio transmit (VOX, break-in, tone burst, sending Morse or voice memories, the automatic tuner).",
			Body: v1Switch{}, Resp: v1State{}, RespDoc: "Applied; the state after the change.", Errors: ctlErrors,
			H: h((*Server).v1SetFunction)},
	}
}

// registerV1 adds the authenticated v1 routes to mux (health is registered separately, open).
// Anything else under /api/v1/ gets a problem-details 404, or 405 when the path exists with
// another method.
func (s *Server) registerV1(mux *http.ServeMux) {
	for _, r := range v1Routes(s) {
		if r.H != nil {
			mux.HandleFunc(r.Method+" "+r.Path, r.H)
		}
	}
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		var allow []string
		for _, m := range []string{"GET", "PUT", "POST", "DELETE"} {
			probe := r.Clone(r.Context())
			probe.Method = m
			if _, pat := mux.Handler(probe); pat != "/api/v1/" && pat != "" {
				allow = append(allow, m)
			}
		}
		if len(allow) > 0 {
			w.Header().Set("Allow", strings.Join(allow, ", "))
			problem(w, http.StatusMethodNotAllowed, "use "+strings.Join(allow, " or "))
			return
		}
		problem(w, http.StatusNotFound, "no such API route; see /api/v1/openapi.json")
	})
}

func cmd(name string, kv ...any) rig.Command {
	c := rig.Command{Name: name, Args: map[string]any{}}
	for i := 0; i+1 < len(kv); i += 2 {
		c.Args[kv[i].(string)] = kv[i+1]
	}
	return c
}

// ---- handlers ----

func problem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v1Problem{Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: detail})
}

func (s *Server) v1Rig(w http.ResponseWriter, r *http.Request) (*rig.Rig, bool) {
	rg, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		problem(w, http.StatusNotFound, "no radio with that id")
	}
	return rg, ok
}

func v1RigOf(rg *rig.Rig) v1Rig {
	c := rg.Config()
	return v1Rig{ID: c.ID, Name: c.Name, Online: rg.Snapshot().Connected, ReadOnly: c.ReadOnly}
}

func v1StateOf(st rig.State) v1State {
	out := v1State{
		ID: st.ID, Online: st.Connected, Error: st.Error, FrequencyHz: st.Freq, Mode: st.Mode, PassbandHz: st.Passband,
		VFO: st.VFO, Split: st.Split, Transmitting: st.PTT, RitHz: st.RIT, XitHz: st.XIT,
		Levels: st.Levels, Functions: st.Funcs, Parameters: st.Parms, Updated: st.Updated,
	}
	if st.Split {
		out.TxVFO, out.TxFrequencyHz = st.TXVFO, st.TXFreq
	}
	if out.Levels == nil {
		out.Levels = map[string]float64{}
	}
	if out.Functions == nil {
		out.Functions = map[string]bool{}
	}
	if out.Parameters == nil {
		out.Parameters = map[string]string{}
	}
	return out
}

func (s *Server) v1GetInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, v1Info{Name: "rigctlweb", Version: s.version, API: apiVersions})
}

func (s *Server) v1GetOpenAPI(w http.ResponseWriter, _ *http.Request) {
	url := s.basePath
	if url == "" {
		url = "/"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(OpenAPIJSON(s.version, url))
}

func (s *Server) v1ListRigs(w http.ResponseWriter, _ *http.Request) {
	out := []v1Rig{}
	for _, rg := range s.mgr.List() {
		out = append(out, v1RigOf(rg))
	}
	writeJSON(w, 200, out)
}

func (s *Server) v1GetRig(w http.ResponseWriter, r *http.Request) {
	if rg, ok := s.v1Rig(w, r); ok {
		writeJSON(w, 200, v1RigOf(rg))
	}
}

func (s *Server) v1GetState(w http.ResponseWriter, r *http.Request) {
	if rg, ok := s.v1Rig(w, r); ok {
		writeJSON(w, 200, v1StateOf(rg.Snapshot()))
	}
}

func (s *Server) v1GetCaps(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.v1Rig(w, r)
	if !ok {
		return
	}
	c := rg.Caps()
	if c == nil {
		problem(w, http.StatusServiceUnavailable, "the radio has not been reached yet")
		return
	}
	out := v1Capabilities{
		Model: c.Model, Manufacturer: c.Mfg, Hamlib: c.Hamlib,
		Modes: nonNil(c.Modes), VFOs: nonNil(c.VFOs), Levels: []v1Level{}, Functions: []v1Function{},
		MaxRitHz: c.MaxRIT, MaxXitHz: c.MaxXIT, CanSplit: c.CanSplit,
	}
	for _, l := range c.Levels {
		out.Levels = append(out.Levels, v1Level{Name: l.Name, Min: l.Min, Max: l.Max, Step: l.Step, Readable: l.Get, Settable: l.Set})
	}
	get, set := map[string]bool{}, map[string]bool{}
	var names []string
	for _, list := range [][]string{c.GetFuncs, c.SetFuncs} {
		for _, n := range list {
			if !contains(names, n) {
				names = append(names, n)
			}
		}
	}
	for _, n := range c.GetFuncs {
		get[n] = true
	}
	for _, n := range c.SetFuncs {
		set[n] = true
	}
	for _, n := range names {
		out.Functions = append(out.Functions, v1Function{Name: n, Readable: get[n], Settable: set[n] && v1Funcs[n]})
	}
	writeJSON(w, 200, out)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ctl builds a control handler: decode the body into B, turn it into a command, run it.
func ctl[B any](build func(B) rig.Command) func(*Server, http.ResponseWriter, *http.Request) {
	return func(s *Server, w http.ResponseWriter, r *http.Request) {
		rg, ok := s.v1Rig(w, r)
		if !ok {
			return
		}
		var b B
		if err := decodeStrict(w, r, &b); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		s.v1Exec(w, r, rg, build(b))
	}
}

func (s *Server) v1SetLevel(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.v1Rig(w, r)
	if !ok {
		return
	}
	var b v1LevelValue
	if err := decodeStrict(w, r, &b); err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	name := r.PathValue("name")
	if c := rg.Caps(); c != nil && !standardSettableLevel(c, name) {
		problem(w, http.StatusUnprocessableEntity, "this radio has no settable standard level "+name)
		return
	}
	s.v1Exec(w, r, rg, cmd("set_level", "name", name, "value", b.Value))
}

func standardSettableLevel(c *rigctl.Caps, name string) bool {
	for _, l := range c.Levels {
		if l.Name == name && l.Set {
			return true
		}
	}
	return false
}

func (s *Server) v1SetFunction(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.v1Rig(w, r)
	if !ok {
		return
	}
	var b v1Switch
	if err := decodeStrict(w, r, &b); err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	name := r.PathValue("name")
	if keyingFunc(name) {
		problem(w, http.StatusForbidden, name+" can make the radio transmit, so API v1 never changes it")
		return
	}
	if !v1Funcs[name] {
		problem(w, http.StatusForbidden, name+" is not one of the functions API v1 changes (see the capabilities' settable flag)")
		return
	}
	if c := rg.Caps(); c != nil && !contains(c.SetFuncs, name) {
		problem(w, http.StatusUnprocessableEntity, "this radio has no settable standard function "+name)
		return
	}
	s.v1Exec(w, r, rg, cmd("set_func", "name", name, "on", b.Enabled))
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// v1Exec runs a command and answers with the new state or a problem.
func (s *Server) v1Exec(w http.ResponseWriter, r *http.Request, rg *rig.Rig, c rig.Command) {
	if c.Name == "set_ptt" || c.Name == "vfo_op" {
		problem(w, http.StatusForbidden, "API v1 never keys the transmitter")
		return
	}
	err := rg.Exec(r.Context(), c)
	switch {
	case err == nil:
		writeJSON(w, 200, v1StateOf(rg.Snapshot()))
	case errors.Is(err, rig.ErrReadOnly):
		problem(w, http.StatusForbidden, "this radio is read-only")
	case errors.Is(err, rig.ErrNotConnected):
		problem(w, http.StatusServiceUnavailable, "the radio is not connected")
	case errors.Is(err, context.DeadlineExceeded):
		problem(w, http.StatusGatewayTimeout, "the radio did not answer in time")
	case rigctl.IsUnsupported(err):
		problem(w, http.StatusUnprocessableEntity, err.Error())
	default:
		var he *rigctl.Error
		if errors.As(err, &he) {
			problem(w, http.StatusBadGateway, err.Error())
			return
		}
		problem(w, http.StatusBadRequest, err.Error()) // validation against the radio's capabilities
	}
}

// decodeStrict reads a JSON object body into v (a pointer to a struct). Unknown fields are
// refused, so a typo is an error rather than a no-op, and so is a missing required field (one
// without omitempty), so `{}` never quietly sets a level to 0 or switches something off.
func decodeStrict(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		return errors.New("body too large or unreadable")
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &present); err != nil {
		return errors.New("body must be a JSON object")
	}
	t := reflect.TypeOf(v).Elem()
	for i := range t.NumField() {
		name, opts, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" || strings.Contains(opts, "omitempty") {
			continue
		}
		if raw, ok := present[name]; !ok || string(raw) == "null" {
			return errors.New("missing required field " + strconvQuote(name))
		}
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("bad JSON body: " + err.Error())
	}
	return nil
}

func strconvQuote(s string) string { return `"` + s + `"` }

// v1Events streams state as Server-Sent Events.
func (s *Server) v1Events(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.v1Rig(w, r)
	if !ok {
		return
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // ask nginx-style proxies not to buffer
	states, unsub := rg.Subscribe()
	defer unsub()
	send := func(st rig.State) bool {
		b, _ := json.Marshal(v1StateOf(st))
		if _, err := w.Write([]byte("event: state\ndata: " + string(b) + "\n\n")); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send(rg.Snapshot()) {
		return
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case st := <-states:
			if !send(st) {
				return
			}
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil || rc.Flush() != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// integrationInfo is the handshake answer (GET /api/v1/integration).
func (s *Server) integrationInfo() integration.Info {
	info := integration.Info{
		Name:         "rigctlweb",
		Version:      s.version,
		API:          apiVersions,
		Mode:         s.mode(),
		Capabilities: capabilities,
	}
	for _, r := range s.mgr.List() {
		c := r.Config()
		info.Rigs = append(info.Rigs, integration.InfoRig{ID: c.ID, ReadOnly: c.ReadOnly})
	}
	return info
}
