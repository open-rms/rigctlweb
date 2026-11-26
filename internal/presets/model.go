// Package presets reads openRMS Rigs settings profiles (https://rigs.openrms.dev), shows them
// like a manual, checks them against the connected radio, and applies the steps rigctlWeb can
// set safely. The format is openRMS-Spec rigs/profile.md (v1).
//
// Safety rules, which hold whatever a profile says:
//   - nothing here keys the transmitter: a profile naming a keying function, a function outside
//     rig.SafeFuncs, PTT or TUNE is refused as a whole;
//   - every change goes through rig.Exec, which validates it against the radio's capabilities;
//     the plan's rigctld lines are never sent;
//   - raw CAT steps go only through the narrow path in cat.go: the openRMS allow-list (embedded,
//     internal/catallow), ASCII CAT only, read before and after each change, the exact set
//     commands confirmed by the operator, never while transmitting, every command (reads too)
//     logged; any other raw CAT is shown for the operator to set on the radio. ASCII CAT can't
//     tell a read from a set by its syntax, so nothing is read (by Check too) unless the step
//     passes the allow-list's step_rules;
//   - nothing about the station leaves it: checks run locally, and the library is fetched only
//     when the operator asks.
//
// Profiles are crowd-sourced text. Callers render every string as text, never HTML.
package presets

// Rigs v1 is additive, so decoding ignores fields it does not know.

// Dump is GET /dump.json: every published profile with its document.
type Dump struct {
	GeneratedAt string     `json:"generated_at"`
	License     string     `json:"license"`
	Profiles    []Envelope `json:"profiles"`
}

// Envelope is a profile's server-side record around the document.
type Envelope struct {
	ID         string            `json:"id"`
	Revision   int               `json:"revision"`
	Status     string            `json:"status"`
	Title      string            `json:"title"`
	RigMacro   string            `json:"rig_macro"`
	Operation  string            `json:"operation"`
	Author     Author            `json:"author"`
	Stats      Stats             `json:"stats"`
	ForkedFrom *Fork             `json:"forked_from"`
	UpdatedAt  string            `json:"updated_at"`
	Links      map[string]string `json:"links"`
	Document   Profile           `json:"document"`
	// Plan is the server's apply-plan for this revision (dump.json, Rigs API 1.1). It gives
	// the raised safety, level tolerance and menu path without Hamlib's catalog. May be nil.
	Plan *ServerPlan `json:"plan"`
}

// ServerPlan is the part of Rigs' apply-plan rigctlWeb uses. rigctlWeb never sends the plan's
// rigctld lines; it builds its own validated commands.
type ServerPlan struct {
	Steps []ServerPlanStep `json:"steps"`
}

type ServerPlanStep struct {
	ID        string   `json:"id"`
	Safety    string   `json:"safety"`
	Tolerance *float64 `json:"tolerance"`
	Menu      []string `json:"menu"`
}

type Author struct {
	Callsign *string `json:"callsign"`
}

type Stats struct {
	Stars     int `json:"stars"`
	Worked    int `json:"worked"`
	NotWorked int `json:"not_worked"`
}

type Fork struct {
	ID       string `json:"id"`
	Revision *int   `json:"revision"`
}

// Profile is the document (profile.schema.json).
type Profile struct {
	Schema     string     `json:"schema"`
	Title      string     `json:"title"`
	Summary    string     `json:"summary"`
	Rig        RigRef     `json:"rig"`
	Operation  string     `json:"operation"`
	Connection string     `json:"connection"`
	Interface  string     `json:"interface"`
	Hamlib     string     `json:"hamlib"`
	Audio      *Audio     `json:"audio,omitempty"`
	Steps      []Step     `json:"steps"`
	Software   []Software `json:"software"`
	Sources    []Source   `json:"sources"`
	Notes      string     `json:"notes"`
	Tags       []string   `json:"tags"`
}

type RigRef struct {
	Macro       string `json:"macro"`
	HamlibModel int    `json:"hamlib_model"`
	Mfg         string `json:"mfg"`
	Name        string `json:"name"`
	Firmware    string `json:"firmware,omitempty"`
}

type Audio struct {
	RxTargetDBFS *float64 `json:"rx_target_dbfs,omitempty"`
	RxNote       string   `json:"rx_note,omitempty"`
	TxNote       string   `json:"tx_note,omitempty"`
}

type Software struct {
	App     string `json:"app"`
	Setting string `json:"setting"`
	Value   string `json:"value"`
	Note    string `json:"note,omitempty"`
}

type Source struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author,omitempty"`
	URL    string `json:"url,omitempty"`
	Date   string `json:"date,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Step is one setting. Value is a JSON number (float64), boolean or string.
type Step struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Value      any      `json:"value"`
	PassbandHz int      `json:"passband_hz"`
	Expect     *Expect  `json:"expect"`
	CheckOnly  bool     `json:"check_only"`
	Safety     string   `json:"safety"`
	Unit       string   `json:"unit"`
	Display    string   `json:"display"`
	Hamlib     string   `json:"hamlib"`
	Firmware   string   `json:"firmware"`
	Cat        *Cat     `json:"cat"`
	Manual     *Manual  `json:"manual"`
	Menu       []string `json:"menu"`
	Note       string   `json:"note"`
	Sources    []string `json:"sources"`
	Confidence string   `json:"confidence"`
}

type Expect struct {
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	OneOf []any    `json:"one_of,omitempty"`
	Not   []any    `json:"not,omitempty"`
}

type Cat struct {
	Protocol string            `json:"protocol"`
	Read     string            `json:"read"`
	Reply    string            `json:"reply"`
	Set      string            `json:"set"`
	Values   map[string]string `json:"values"`
}

type Manual struct {
	Menu  string `json:"menu"`
	Value string `json:"value"`
}
