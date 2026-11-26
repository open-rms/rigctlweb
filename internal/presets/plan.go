package presets

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/open-rms/rigctlweb/internal/rig"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// License is the licence of profile content; the UI credits the author and links the source.
const License = "CC-BY-SA-4.0"

// txLevels and txFuncs change transmit behaviour. A step naming one is treated as safety "tx"
// (the operator opts in to it) whatever the profile says.
// The list follows profile.md's "Safety" rule.
var txLevels = map[string]bool{"RFPOWER": true, "MICGAIN": true, "COMP": true, "VOXGAIN": true, "VOXDELAY": true,
	"ANTIVOX": true, "BKINDL": true, "KEYSPD": true, "BKIN_DLYMS": true, "USB_AF_INPUT": true, "TXEQ": true}
var txFuncs = map[string]bool{"COMP": true}

// refusedNames can never appear in a profile rigctlWeb applies.
var refusedNames = map[string]bool{"PTT": true, "TUNE": true}

// Summary is one entry in the presets list.
type Summary struct {
	ID         string   `json:"id"`
	Revision   int      `json:"revision"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary,omitempty"`
	Operation  string   `json:"operation"`
	RigMfg     string   `json:"rigMfg"`
	RigName    string   `json:"rigName"`
	Author     string   `json:"author,omitempty"`
	Tags       []string `json:"tags"`
	Stats      Stats    `json:"stats"`
	UpdatedAt  string   `json:"updatedAt,omitempty"`
	Matches    bool     `json:"matches"` // made for the selected radio's model
	Refused    string   `json:"refused,omitempty"`
	Settings   int      `json:"settings"` // steps rigctlWeb could set
	Manual     int      `json:"manual"`   // steps for the operator to do on the radio
	TxSettings int      `json:"txSettings"`
}

// View is a profile laid out like a manual, with each step marked for the selected radio.
type View struct {
	Summary
	Connection string            `json:"connection,omitempty"`
	Interface  string            `json:"interface,omitempty"`
	Hamlib     string            `json:"hamlib,omitempty"`
	Firmware   string            `json:"firmware,omitempty"`
	Audio      *Audio            `json:"audio,omitempty"`
	Software   []Software        `json:"software"`
	Sources    []Source          `json:"sources"`
	Notes      string            `json:"notes,omitempty"`
	License    string            `json:"license"`
	Links      map[string]string `json:"links"`
	ForkedFrom string            `json:"forkedFrom,omitempty"`
	Steps      []PlanStep        `json:"steps"`
	Connected  bool              `json:"connected"` // a radio's capabilities were used
}

// PlanStep is one step with what rigctlWeb can do with it on this radio.
type PlanStep struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Value      any               `json:"value,omitempty"`
	PassbandHz int               `json:"passbandHz,omitempty"`
	Expect     *Expect           `json:"expect,omitempty"`
	Display    string            `json:"display,omitempty"`
	Unit       string            `json:"unit,omitempty"`
	Note       string            `json:"note,omitempty"`
	Confidence string            `json:"confidence,omitempty"`
	Sources    []string          `json:"sources"`
	Safety     string            `json:"safety"` // "rx" or "tx"
	CheckOnly  bool              `json:"checkOnly"`
	Readable   bool              `json:"readable"`   // rigctlWeb can read it from this radio
	Applicable bool              `json:"applicable"` // rigctlWeb can set it on this radio
	Reason     string            `json:"reason,omitempty"`
	Manual     *Manual           `json:"manual,omitempty"`
	Choices    map[string]string `json:"choices,omitempty"`  // raw CAT value meanings
	Hamlib     string            `json:"hamlib,omitempty"`   // Hamlib versions this step needs
	Firmware   string            `json:"firmware,omitempty"` // firmware or menu-table versions it applies to
	Menu       []string          `json:"menu,omitempty"`     // path in the radio's own menus, outermost first
	Tolerance  *float64          `json:"tolerance,omitempty"`
	CatRead    string            `json:"catRead,omitempty"` // raw CAT read command, for display
	Command    string            `json:"command,omitempty"` // the exact raw CAT command applying sends
	Text       string            `json:"text"`
}

// Matches reports whether a profile was made for the radio these capabilities describe.
// rigctld reports the model and manufacturer names, not the Hamlib macro.
func Matches(p Profile, caps *rigctl.Caps) bool {
	if caps == nil {
		return false
	}
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	return norm(p.Rig.Name) != "" && norm(p.Rig.Name) == norm(caps.Model) && norm(p.Rig.Mfg) == norm(caps.Mfg)
}

// Refusal says why rigctlWeb will not apply any part of a profile, or "" when it may.
func Refusal(p Profile) string {
	ids := map[string]bool{}
	for _, s := range p.Steps {
		if ids[s.ID] {
			return fmt.Sprintf("step id %q appears twice", s.ID)
		}
		ids[s.ID] = true
		name := strings.ToUpper(strings.TrimSpace(s.Name))
		switch s.Kind {
		case "func":
			if rig.IsKeyingFunc(name) {
				return fmt.Sprintf("step %q changes %s, which can make the radio transmit", s.ID, s.Name)
			}
			if !rig.SafeFuncs[s.Name] {
				return fmt.Sprintf("step %q changes %s, which is not on rigctlWeb's list of functions other programs may change", s.ID, s.Name)
			}
			if _, ok := s.Value.(bool); s.Value != nil && !ok {
				return fmt.Sprintf("step %q: a function's value must be true or false", s.ID)
			}
		case "level", "parm":
			switch s.Value.(type) {
			case nil, float64, string:
			default:
				return fmt.Sprintf("step %q: a %s value must be a number", s.ID, s.Kind)
			}
		case "mode":
			if _, ok := s.Value.(string); s.Value != nil && !ok {
				return fmt.Sprintf("step %q: a mode must be a name", s.ID)
			}
		case "cat":
			if why := catRefusal(s); why != "" {
				return why
			}
		}
		if refusedNames[name] {
			return fmt.Sprintf("step %q names %s", s.ID, s.Name)
		}
	}
	return ""
}

func author(e Envelope) string {
	if e.Author.Callsign != nil {
		return *e.Author.Callsign
	}
	return ""
}

// Summarize lists a profile for the selected radio (caps may be nil).
func Summarize(e Envelope, caps *rigctl.Caps) Summary {
	v := Plan(e, caps)
	return v.Summary
}

// Plan lays out a profile for the selected radio. Without capabilities (no radio, or not
// connected) nothing is applicable and the view is for reading only.
func Plan(e Envelope, caps *rigctl.Caps) View {
	d := e.Document
	v := View{
		Summary: Summary{ID: e.ID, Revision: e.Revision, Title: e.Title, Summary: d.Summary, Operation: e.Operation,
			RigMfg: d.Rig.Mfg, RigName: d.Rig.Name, Author: author(e), Tags: d.Tags, Stats: e.Stats, UpdatedAt: e.UpdatedAt,
			Matches: Matches(d, caps), Refused: Refusal(d)},
		Connection: d.Connection, Interface: d.Interface, Hamlib: d.Hamlib, Firmware: d.Rig.Firmware, Audio: d.Audio,
		Software: d.Software, Sources: d.Sources, Notes: d.Notes, License: License, Links: e.Links, Connected: caps != nil,
	}
	if v.Title == "" {
		v.Title = d.Title
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if v.Software == nil {
		v.Software = []Software{}
	}
	if v.Sources == nil {
		v.Sources = []Source{}
	}
	if e.ForkedFrom != nil {
		v.ForkedFrom = e.ForkedFrom.ID
	}
	server := map[string]ServerPlanStep{}
	if e.Plan != nil {
		for _, ss := range e.Plan.Steps {
			server[ss.ID] = ss
		}
	}
	for _, s := range d.Steps {
		ps := planStep(s, caps)
		if ss, ok := server[s.ID]; ok {
			// The server's safety only ever raises ours; its tolerance and menu are used as is.
			if ss.Safety != "" && ss.Safety != "rx" {
				ps.Safety = "tx"
			}
			ps.Tolerance = ss.Tolerance
			if len(ps.Menu) == 0 {
				ps.Menu = ss.Menu
			}
		}
		if !v.Matches || v.Refused != "" {
			if ps.Applicable {
				ps.Applicable = false
				if v.Refused != "" {
					ps.Reason = "rigctlWeb refuses this whole preset: " + v.Refused
				} else {
					ps.Reason = "this preset is for another radio model"
				}
			}
			if !v.Matches {
				ps.Readable = false
			}
		}
		switch {
		case ps.Applicable:
			v.Settings++
			if ps.Safety == "tx" {
				v.TxSettings++
			}
		case s.Kind == "manual" || s.Kind == "cat" && !ps.Readable:
			v.Manual++
		}
		v.Steps = append(v.Steps, ps)
	}
	if v.Steps == nil {
		v.Steps = []PlanStep{}
	}
	return v
}

func planStep(s Step, caps *rigctl.Caps) PlanStep {
	ps := PlanStep{ID: s.ID, Kind: s.Kind, Name: s.Name, Value: s.Value, PassbandHz: s.PassbandHz, Expect: s.Expect,
		Display: s.Display, Unit: s.Unit, Note: s.Note, Confidence: s.Confidence, Sources: s.Sources,
		Safety: "rx", CheckOnly: s.CheckOnly, Manual: s.Manual, Hamlib: s.Hamlib, Firmware: s.Firmware, Menu: s.Menu}
	if ps.Sources == nil {
		ps.Sources = []string{}
	}
	if s.Safety != "rx" && s.Safety != "" || txLevels[s.Name] && s.Kind == "level" || txFuncs[s.Name] && s.Kind == "func" {
		ps.Safety = "tx" // unknown safety values count as tx
	}
	if s.Cat != nil {
		ps.Choices = s.Cat.Values
	}
	ps.Text = stepText(s)
	switch s.Kind {
	case "manual":
		ps.Reason = "set this on the radio"
		return ps
	case "cat":
		spec, readWhy, setWhy := planCat(s, caps)
		if s.Cat != nil {
			ps.CatRead = s.Cat.Read
		}
		if spec == nil {
			ps.Reason = "set it on the radio: " + readWhy
			return ps
		}
		ps.Readable = true
		if spec.Safety == "tx" {
			ps.Safety = "tx"
		}
		switch {
		case s.CheckOnly || s.Value == nil:
			ps.CheckOnly, ps.Reason = true, "checked only, never set"
		case setWhy != "":
			ps.Reason = "set it on the radio: " + setWhy
		default:
			ps.Applicable, ps.Command = true, spec.Set
		}
		return ps
	case "mode", "level", "func", "parm":
	default:
		ps.Reason = "rigctlWeb does not know this kind of step"
		return ps
	}
	if caps == nil {
		ps.Reason = "no radio connected"
		return ps
	}
	ps.Readable = readable(s, caps)
	switch {
	case s.CheckOnly || s.Value == nil:
		ps.Reason = "checked only, never set"
		if !ps.Readable {
			ps.Reason = "checked only, and this radio cannot report it"
		}
		ps.CheckOnly = true
	default:
		if why := settable(s, caps); why != "" {
			ps.Reason = why
		} else {
			ps.Applicable = true
		}
	}
	return ps
}

func stepText(s Step) string {
	val := s.Display
	if val == "" && s.Value != nil {
		val = fmt.Sprint(s.Value)
		if b, ok := s.Value.(bool); ok {
			val = map[bool]string{true: "on", false: "off"}[b]
		}
		if s.Cat != nil {
			if m, ok := s.Cat.Values[val]; ok {
				val = m
			}
		}
	}
	switch s.Kind {
	case "mode":
		if s.PassbandHz > 0 {
			return fmt.Sprintf("Mode %s, %d Hz passband", val, s.PassbandHz)
		}
		return "Mode " + val
	case "manual":
		if s.Manual != nil {
			return fmt.Sprintf("On the radio: %s = %s", s.Manual.Menu, s.Manual.Value)
		}
	}
	if s.CheckOnly || s.Value == nil {
		return s.Name + " (check)"
	}
	return s.Name + " → " + val
}

func readable(s Step, caps *rigctl.Caps) bool {
	switch s.Kind {
	case "mode":
		return true // get_mode works on every backend rigctld drives
	case "level":
		return caps.HasLevel(s.Name) || extHas(caps.ExtLevels, s.Name)
	case "func":
		return caps.HasFunc(s.Name) || extHas(caps.ExtFuncs, s.Name)
	case "parm":
		return contains(caps.GetParms, s.Name) || extHas(caps.ExtParms, s.Name)
	}
	return false
}

// settable mirrors rig.Exec's checks so the UI can say ahead of time why a step can't be set.
// Exec checks again when the step is applied. Presets use only Hamlib's standard names:
// backend-specific (ext) controls are not reviewed for safety, so a shared profile can't set them.
func settable(s Step, caps *rigctl.Caps) string {
	switch s.Kind {
	case "mode":
		m, _ := s.Value.(string)
		if !contains(caps.Modes, m) {
			return "this radio has no mode " + m
		}
	case "level":
		f, ok := number(s.Value)
		if !ok {
			return "rigctlWeb sets levels only to numbers"
		}
		for _, l := range caps.Levels {
			if l.Name == s.Name && l.Set {
				if l.Max > l.Min && (f < l.Min || f > l.Max) {
					return fmt.Sprintf("%g is outside this radio's range (%g to %g)", f, l.Min, l.Max)
				}
				return ""
			}
		}
		return "this radio cannot set " + s.Name + " (presets set only Hamlib's standard levels)"
	case "func":
		if !contains(caps.SetFuncs, s.Name) {
			return "this radio cannot set " + s.Name
		}
	case "parm":
		for _, p := range caps.Parms {
			if p.Name == s.Name && p.Set {
				return ""
			}
		}
		return "this radio cannot set " + s.Name + " (presets set only Hamlib's standard parameters)"
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func extHas(list []rigctl.ExtField, name string) bool {
	for _, e := range list {
		if e.Name == name {
			return true
		}
	}
	return false
}

// number reads a JSON number, or a string holding one.
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return 0, false
}
