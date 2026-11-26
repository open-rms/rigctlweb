package presets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/open-rms/rigctlweb/internal/rig"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// Radio is what the service needs from a rig (*rig.Rig implements it).
type Radio interface {
	Config() rig.Config
	Caps() *rigctl.Caps
	Snapshot() rig.State
	ReadSetting(ctx context.Context, kind, name string) (rig.Setting, error)
	Exec(ctx context.Context, cmd rig.Command) error
	// CATRead and CATSet send allow-listed raw ASCII CAT (rig.Rig refuses anything else).
	CATRead(ctx context.Context, cmd string) (string, error)
	CATSet(ctx context.Context, cmd string) error
}

// Service checks, applies and restores presets.
type Service struct {
	Lib *Library
	dir string // snapshots live in dir/presets/<rig id>.json
	log *slog.Logger
	mu  sync.Mutex // one apply or restore at a time
}

// ErrNotFound means the profile is not in the local library.
var ErrNotFound = errors.New("no such preset in the library")

// ErrConfirmCAT means the raw CAT commands the operator confirmed are not the ones that would
// be sent (the preset changed, or the request was not built from the confirmation dialog).
var ErrConfirmCAT = errors.New("confirm the exact raw CAT commands before applying them")

// NewService uses lib for profiles and keeps restore snapshots under stateDir.
func NewService(lib *Library, stateDir string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{Lib: lib, dir: stateDir, log: log}
}

// ---- check ----

// CheckStep is one step compared with the radio.
type CheckStep struct {
	ID      string `json:"id"`
	Status  string `json:"status"` // ok, drift, unknown or manual
	Current any    `json:"current,omitempty"`
	Text    string `json:"currentText,omitempty"`
	Noop    bool   `json:"noop"` // applying it would change nothing
	Error   string `json:"error,omitempty"`
}

// CheckResult compares a whole profile with the radio.
type CheckResult struct {
	ProfileID string         `json:"profileId"`
	Revision  int            `json:"revision"`
	Summary   map[string]int `json:"summary"`
	Steps     []CheckStep    `json:"steps"`
}

// Check reads every step this radio can report and compares it with the profile. It changes
// nothing on the radio and sends nothing anywhere: the comparison runs here, offline.
func (s *Service) Check(ctx context.Context, r Radio, id string) (CheckResult, error) {
	e, ok := s.Lib.Get(id)
	if !ok {
		return CheckResult{}, ErrNotFound
	}
	caps := r.Caps()
	if caps == nil {
		return CheckResult{}, rig.ErrNotConnected
	}
	v := Plan(e, caps)
	if !v.Matches {
		return CheckResult{}, errors.New("this preset is for another radio model")
	}
	res := CheckResult{ProfileID: e.ID, Revision: e.Revision, Summary: map[string]int{"ok": 0, "drift": 0, "unknown": 0, "manual": 0}}
	steps := map[string]Step{}
	for _, st := range e.Document.Steps {
		steps[st.ID] = st
	}
	cache := map[string]rig.Setting{}
	catCache := map[string]string{}
	for _, ps := range v.Steps {
		cs := CheckStep{ID: ps.ID}
		switch {
		case ps.Kind == "manual" || ps.Kind == "cat" && !ps.Readable:
			cs.Status = "manual"
		case ps.Kind == "cat":
			s.checkCat(ctx, r, steps[ps.ID], caps, catCache, &cs)
		case !ps.Readable:
			cs.Status = "unknown"
		default:
			key := ps.Kind + ":" + ps.Name
			cur, seen := cache[key]
			var err error
			if !seen {
				cur, err = r.ReadSetting(ctx, ps.Kind, ps.Name)
				if err == nil {
					cache[key] = cur
				}
			}
			if err != nil {
				cs.Status, cs.Error = "unknown", err.Error()
				break
			}
			cs.Current, cs.Text = currentValue(ps.Kind, ps.Name, cur, caps)
			cs.Status, cs.Noop = compare(steps[ps.ID], cs.Current, levelTolerance(steps[ps.ID], caps, ps.Tolerance))
			// Rigs compares the mode name only, but applying also sets the passband, so it is
			// not a no-op when the passband differs.
			if st := steps[ps.ID]; ps.Kind == "mode" && st.PassbandHz > 0 && cur.Passband != st.PassbandHz {
				cs.Noop = false
			}
		}
		res.Summary[cs.Status]++
		res.Steps = append(res.Steps, cs)
	}
	if res.Steps == nil {
		res.Steps = []CheckStep{}
	}
	return res, nil
}

// checkCat reads a raw CAT setting (each read command once per check) and compares it.
func (s *Service) checkCat(ctx context.Context, r Radio, st Step, caps *rigctl.Caps, cache map[string]string, cs *CheckStep) {
	spec, _, _ := planCat(st, caps)
	if spec == nil {
		cs.Status = "manual"
		return
	}
	reply, seen := cache[spec.Read]
	if !seen {
		var err error
		reply, err = s.readCat(ctx, r, "check", st.ID, spec.Read)
		if errors.Is(err, rig.ErrReadOnly) {
			cs.Status, cs.Error = "manual", "raw CAT is off for read-only radios"
			return
		}
		if err != nil {
			cs.Status, cs.Error = "unknown", err.Error()
			return
		}
		cache[spec.Read] = reply
	}
	cur, err := catCapture(spec.Reply, spec.Read, reply)
	if err != nil {
		cs.Status, cs.Error = "unknown", err.Error()
		return
	}
	cs.Current, cs.Text = cur, cur
	keys := []string{cur}
	if n, err := strconv.Atoi(cur); err == nil {
		keys = append(keys, strconv.Itoa(n)) // "01" names the same choice as "1"
	}
	for _, k := range keys {
		if label, ok := st.Cat.Values[k]; ok {
			cs.Text = label
			break
		}
	}
	cs.Status, cs.Noop = compare(st, cur, 0)
}

func currentValue(kind, name string, cur rig.Setting, caps *rigctl.Caps) (any, string) {
	switch kind {
	case "mode":
		t := cur.Mode
		if cur.Passband > 0 {
			t += fmt.Sprintf(", %d Hz", cur.Passband)
		}
		return cur.Mode, t
	case "level":
		t := strconv.FormatFloat(cur.Number, 'f', -1, 64)
		if name == "AGC" {
			for _, a := range caps.AGCLevels {
				if float64(a.Value) == cur.Number {
					t = a.Name
				}
			}
		}
		return cur.Number, t
	case "func":
		return cur.On, map[bool]string{true: "on", false: "off"}[cur.On]
	case "parm":
		if _, err := strconv.ParseFloat(cur.Text, 64); err == nil {
			return cur.Number, cur.Text
		}
		return cur.Text, cur.Text
	}
	return nil, ""
}

// levelTolerance is how far from the target a level reading may be: the plan's tolerance when
// the library has one (so the result matches Rigs' /check), else the step of this radio's get
// range for the level. Other kinds compare exactly.
func levelTolerance(st Step, caps *rigctl.Caps, planTol *float64) float64 {
	if st.Kind != "level" {
		return 0
	}
	if planTol != nil {
		return *planTol
	}
	if caps != nil {
		for _, l := range caps.Levels {
			if l.Name == st.Name && l.Get {
				return l.Step
			}
		}
	}
	return 0
}

// truthy reads a function state: true only for true, 1, "1" or "true" (any case).
func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x == 1
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	}
	return false
}

// compare applies the openRMS Rigs check rules (profile.md, "Checks"; conformance cases in
// testdata/check-vectors.json). noop reports whether applying the value would change nothing.
func compare(st Step, cur any, tol float64) (status string, noop bool) {
	if st.Kind == "manual" {
		return "manual", false
	}
	if cur == nil {
		return "unknown", false
	}
	eq := func(want, got any) bool {
		if _, ok := want.(bool); ok || st.Kind == "func" {
			return truthy(want) == truthy(got)
		}
		if fa, ok := number(want); ok {
			if fb, ok := number(got); ok {
				return math.Abs(fa-fb) <= math.Max(tol, 1e-6)+1e-12
			}
		}
		sa, ok1 := want.(string)
		sb, ok2 := got.(string)
		return ok1 && ok2 && strings.EqualFold(sa, sb)
	}
	noop = st.Value != nil && eq(st.Value, cur)
	if x := st.Expect; x != nil && (x.Min != nil || x.Max != nil || len(x.OneOf) > 0 || len(x.Not) > 0) {
		for _, n := range x.Not {
			if eq(n, cur) {
				return "drift", noop
			}
		}
		if len(x.OneOf) > 0 {
			in := false
			for _, o := range x.OneOf {
				in = in || eq(o, cur)
			}
			if !in {
				return "drift", noop
			}
		}
		if x.Min != nil || x.Max != nil {
			f, ok := number(cur)
			if !ok || x.Min != nil && f < *x.Min-1e-12 || x.Max != nil && f > *x.Max+1e-12 {
				return "drift", noop
			}
		}
		return "ok", noop
	}
	if st.Value == nil || noop {
		return "ok", noop
	}
	return "drift", false
}

// ---- apply and restore ----

// Saved is a setting's value before rigctlWeb changed it, kept on disk until restored.
type Saved struct {
	Kind    string      `json:"kind"`
	Name    string      `json:"name"`
	Before  rig.Setting `json:"before"`
	Known   bool        `json:"known"` // false: the radio could not report it, so it can't be restored
	Applied bool        `json:"applied"`
	Preset  string      `json:"preset"`
	Cat     *SavedCat   `json:"cat,omitempty"` // for kind "cat"; Name is then the read command
}

// SavedCat is what restoring a raw CAT setting needs. Restore renders Template with Before and
// the radio checks it against the allow-list again before sending.
type SavedCat struct {
	Read     string `json:"read"`
	Reply    string `json:"reply"`
	Template string `json:"template"`
	Before   string `json:"before"`
}

// Snapshot is what "Restore" puts back on one radio: the values from before the first preset
// was applied since the last restore.
type Snapshot struct {
	Rig     string    `json:"rig"`
	TakenAt time.Time `json:"takenAt"`
	Presets []string  `json:"presets"` // titles applied since
	Saved   []Saved   `json:"saved"`
}

// ApplyResult reports what was set.
type ApplyResult struct {
	Applied []string `json:"applied"`
	Failed  string   `json:"failed,omitempty"`
	Error   string   `json:"error,omitempty"`
	Skipped []string `json:"skipped"` // not reached after a failure
}

var safeRigID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func (s *Service) snapPath(rigID string) (string, error) {
	if !safeRigID.MatchString(rigID) || rigID == "." || rigID == ".." {
		return "", fmt.Errorf("rig id %q cannot name a file", rigID)
	}
	if s.dir == "" {
		return "", errors.New("no state folder for restore snapshots")
	}
	return filepath.Join(s.dir, "presets", rigID+".json"), nil
}

// LoadSnapshot returns the radio's restore snapshot, or nil when there is none.
func (s *Service) LoadSnapshot(rigID string) (*Snapshot, error) {
	p, err := s.snapPath(rigID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sn Snapshot
	if err := json.Unmarshal(b, &sn); err != nil {
		return nil, fmt.Errorf("restore snapshot unreadable: %w", err)
	}
	return &sn, nil
}

func (s *Service) saveSnapshot(sn *Snapshot) error {
	p, err := s.snapPath(sn.Rig)
	if err != nil {
		return err
	}
	keep := sn.Saved[:0:0]
	for _, x := range sn.Saved {
		if x.Applied {
			keep = append(keep, x)
		}
	}
	if len(keep) == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(Snapshot{Rig: sn.Rig, TakenAt: sn.TakenAt, Presets: sn.Presets, Saved: keep}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p, b)
}

func ready(r Radio) error {
	if r.Config().ReadOnly {
		return rig.ErrReadOnly
	}
	if r.Caps() == nil {
		return rig.ErrNotConnected
	}
	if r.Snapshot().PTT {
		return errors.New("the radio is transmitting; try again when it is not")
	}
	return nil
}

func command(kind, name string, value any, passband int) (rig.Command, error) {
	switch kind {
	case "mode":
		m, _ := value.(string)
		args := map[string]any{"mode": m}
		if passband > 0 {
			args["passband"] = float64(passband)
		}
		return rig.Command{Name: "set_mode", Args: args}, nil
	case "level":
		f, ok := number(value)
		if !ok {
			return rig.Command{}, fmt.Errorf("%s needs a number", name)
		}
		return rig.Command{Name: "set_level", Args: map[string]any{"name": name, "value": f}}, nil
	case "func":
		if rig.IsKeyingFunc(name) || !rig.SafeFuncs[name] {
			return rig.Command{}, fmt.Errorf("rigctlWeb never changes %s from a preset", name)
		}
		on, ok := value.(bool)
		if !ok {
			return rig.Command{}, fmt.Errorf("%s needs true or false", name)
		}
		return rig.Command{Name: "set_func", Args: map[string]any{"name": name, "on": on}}, nil
	case "parm":
		val := fmt.Sprint(value)
		if f, ok := value.(float64); ok {
			val = strconv.FormatFloat(f, 'f', -1, 64)
		}
		return rig.Command{Name: "set_parm", Args: map[string]any{"name": name, "value": val}}, nil
	}
	return rig.Command{}, fmt.Errorf("rigctlWeb cannot set a %q step", kind)
}

// Apply sets the chosen steps, in the profile's order, through the radio's validated commands.
// Every chosen step must be one the plan marks applicable; steps that change transmit
// behaviour are applied only when chosen, which the UI leaves to the operator. Values from
// before are saved first so Restore can put them back. It stops at the first failure.
//
// Raw CAT steps are sent only when confirmCAT lists exactly the commands that will be sent, as
// the confirmation dialog showed them. Each is read first (for Restore), sent, then read back.
func (s *Service) Apply(ctx context.Context, r Radio, id string, chosen, confirmCAT []string) (ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.Lib.Get(id)
	if !ok {
		return ApplyResult{}, ErrNotFound
	}
	if err := ready(r); err != nil {
		return ApplyResult{}, err
	}
	v := Plan(e, r.Caps())
	if v.Refused != "" {
		return ApplyResult{}, errors.New("rigctlWeb refuses this preset: " + v.Refused)
	}
	if !v.Matches {
		return ApplyResult{}, errors.New("this preset is for another radio model")
	}
	want := map[string]bool{}
	for _, c := range chosen {
		want[c] = true
	}
	if len(want) == 0 {
		return ApplyResult{}, errors.New("choose at least one setting")
	}
	raw := map[string]Step{}
	for _, st := range e.Document.Steps {
		raw[st.ID] = st
	}
	var todo []PlanStep
	for _, ps := range v.Steps {
		if !want[ps.ID] {
			continue
		}
		if !ps.Applicable {
			return ApplyResult{}, fmt.Errorf("step %q can't be set: %s", ps.ID, ps.Reason)
		}
		todo = append(todo, ps)
		delete(want, ps.ID)
	}
	for c := range want {
		return ApplyResult{}, fmt.Errorf("the preset has no step %q", c)
	}
	caps := r.Caps()
	specs := map[string]*catSpec{}
	var cmds []string
	for _, ps := range todo {
		if ps.Kind != "cat" {
			continue
		}
		spec, _, why := planCat(raw[ps.ID], caps)
		if spec == nil || why != "" || spec.Set != ps.Command {
			return ApplyResult{}, fmt.Errorf("step %q can't be set over CAT", ps.ID)
		}
		specs[ps.ID] = spec
		cmds = append(cmds, spec.Set)
	}
	if !sameStrings(cmds, confirmCAT) {
		return ApplyResult{}, ErrConfirmCAT
	}

	sn, err := s.LoadSnapshot(r.Config().ID)
	if err != nil {
		return ApplyResult{}, err
	}
	if sn == nil {
		sn = &Snapshot{Rig: r.Config().ID, TakenAt: time.Now().UTC()}
	}
	sn.Presets = append(sn.Presets, e.Title)
	idx := map[string]int{}
	for i, x := range sn.Saved {
		idx[x.Kind+":"+x.Name] = i
	}
	// Raw CAT settings are keyed by their read command, not the profile's free-text name.
	keyOf := func(ps PlanStep) string {
		if sp := specs[ps.ID]; sp != nil {
			return "cat:" + sp.Read
		}
		return ps.Kind + ":" + ps.Name
	}
	// Read every "before" value first, so nothing is changed if the radio stops answering.
	for _, ps := range todo {
		key := keyOf(ps)
		if _, ok := idx[key]; ok {
			continue // the value from before the first preset is already saved
		}
		x := Saved{Kind: ps.Kind, Name: ps.Name, Preset: e.Title}
		if sp := specs[ps.ID]; sp != nil {
			reply, err := s.readCat(ctx, r, e.ID, ps.ID, sp.Read)
			var before string
			if err == nil {
				before, err = catCapture(sp.Reply, sp.Read, reply)
			}
			if err == nil {
				_, err = renderCat(sp.Template, before)
			}
			if err != nil {
				return ApplyResult{}, fmt.Errorf("could not read %s before changing it: %w", ps.Name, err)
			}
			x.Name, x.Known = sp.Read, true
			x.Cat = &SavedCat{Read: sp.Read, Reply: sp.Reply.String(), Template: sp.Template, Before: before}
		} else if ps.Readable {
			b, err := r.ReadSetting(ctx, ps.Kind, ps.Name)
			if err != nil {
				return ApplyResult{}, fmt.Errorf("could not read %s before changing it: %w", ps.Name, err)
			}
			x.Before, x.Known = b, true
		}
		idx[key] = len(sn.Saved)
		sn.Saved = append(sn.Saved, x)
	}

	res := ApplyResult{Applied: []string{}, Skipped: []string{}}
	for i, ps := range todo {
		if res.Failed != "" {
			res.Skipped = append(res.Skipped, ps.ID)
			continue
		}
		var err error
		if r.Snapshot().PTT {
			err = errors.New("stopped: the radio started transmitting")
		} else if sp := specs[ps.ID]; sp != nil {
			var sent bool
			sent, err = s.setCat(ctx, r, e.ID, ps.ID, sp.Read, sp.Reply, sp.Set)
			if sent {
				sn.Saved[idx[keyOf(ps)]].Applied = true // it may have taken; Restore puts the old value back
				if serr := s.saveSnapshot(sn); serr != nil {
					s.log.Error("could not save the restore snapshot", "err", serr)
				}
			}
		} else {
			var cmd rig.Command
			cmd, err = command(ps.Kind, ps.Name, raw[ps.ID].Value, raw[ps.ID].PassbandHz)
			if err == nil {
				err = r.Exec(ctx, cmd)
			}
		}
		if err != nil {
			res.Failed, res.Error = ps.ID, err.Error()
			s.log.Warn("preset step failed", "rig", r.Config().ID, "preset", e.ID, "step", ps.ID, "err", err)
			continue
		}
		res.Applied = append(res.Applied, ps.ID)
		sn.Saved[idx[keyOf(ps)]].Applied = true
		s.log.Info("preset step applied", "rig", r.Config().ID, "preset", e.ID, "revision", e.Revision, "step", ps.ID, "n", i+1, "of", len(todo))
		if err := s.saveSnapshot(sn); err != nil {
			s.log.Error("could not save the restore snapshot", "err", err)
		}
	}
	return res, nil
}

// RestoreResult reports what was put back.
type RestoreResult struct {
	Restored  []string `json:"restored"` // "kind:name"
	Unknown   []string `json:"unknown"`  // changed, but the radio could not report the old value
	Failed    string   `json:"failed,omitempty"`
	Error     string   `json:"error,omitempty"`
	Remaining int      `json:"remaining"`
}

// Restore puts back every saved value, newest change first.
func (s *Service) Restore(ctx context.Context, r Radio) (RestoreResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ready(r); err != nil {
		return RestoreResult{}, err
	}
	sn, err := s.LoadSnapshot(r.Config().ID)
	if err != nil {
		return RestoreResult{}, err
	}
	res := RestoreResult{Restored: []string{}, Unknown: []string{}}
	if sn == nil {
		return res, nil
	}
	for i := len(sn.Saved) - 1; i >= 0; i-- {
		x := &sn.Saved[i]
		if !x.Applied {
			continue
		}
		if !x.Known {
			res.Unknown = append(res.Unknown, x.Kind+":"+x.Name)
			x.Applied = false
			continue
		}
		if x.Kind == "cat" {
			if err := s.restoreCat(ctx, r, x); err != nil {
				res.Failed, res.Error = x.Kind+":"+x.Name, err.Error()
				break
			}
			x.Applied = false
			res.Restored = append(res.Restored, x.Kind+":"+x.Name)
			continue
		}
		var val any
		switch x.Kind {
		case "mode":
			val = x.Before.Mode
		case "level":
			val = x.Before.Number
		case "func":
			val = x.Before.On
		case "parm":
			val = x.Before.Text
		}
		cmd, err := command(x.Kind, x.Name, val, x.Before.Passband)
		if err == nil {
			err = r.Exec(ctx, cmd)
		}
		if err != nil {
			res.Failed, res.Error = x.Kind+":"+x.Name, err.Error()
			break
		}
		x.Applied = false
		res.Restored = append(res.Restored, x.Kind+":"+x.Name)
	}
	if err := s.saveSnapshot(sn); err != nil {
		return res, err
	}
	for _, x := range sn.Saved {
		if x.Applied {
			res.Remaining++
		}
	}
	s.log.Info("preset values restored", "rig", r.Config().ID, "restored", len(res.Restored), "remaining", res.Remaining)
	return res, nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	n := map[string]int{}
	for _, x := range a {
		n[x]++
	}
	for _, x := range b {
		if n[x]--; n[x] < 0 {
			return false
		}
	}
	return true
}

// setCat sends one raw CAT set, logs it, and reads the setting back. sent reports whether the
// command may have reached the radio (so Restore must cover it).
func (s *Service) setCat(ctx context.Context, r Radio, preset, step, read string, reply *regexp.Regexp, cmd string) (sent bool, err error) {
	want := strings.TrimSuffix(strings.TrimPrefix(cmd, strings.TrimSuffix(read, ";")), ";")
	err = r.CATSet(ctx, cmd)
	s.logCAT(r.Config().ID, preset, step, cmd, err)
	if err != nil {
		refused := errors.Is(err, rig.ErrCATRefused) || errors.Is(err, rig.ErrReadOnly) ||
			errors.Is(err, rig.ErrTransmitting) || errors.Is(err, rig.ErrNotConnected)
		return !refused, err
	}
	got, err := s.readCat(ctx, r, preset, step, read)
	if err == nil {
		got, err = catCapture(reply, read, got)
	}
	if err != nil {
		return true, fmt.Errorf("sent %s but could not read it back: %w", cmd, err)
	}
	if !sameCAT(got, want) {
		return true, fmt.Errorf("sent %s but the radio reads back %s", cmd, got)
	}
	return true, nil
}

// readCat sends a raw CAT read and logs it. ASCII CAT can't tell a read from a set by its
// syntax, so reads are logged like sets.
func (s *Service) readCat(ctx context.Context, r Radio, preset, step, cmd string) (string, error) {
	reply, err := r.CATRead(ctx, cmd)
	if !errors.Is(err, rig.ErrReadOnly) {
		s.logCATUse("read", r.Config().ID, preset, step, cmd, reply, err)
	}
	return reply, err
}

func (s *Service) restoreCat(ctx context.Context, r Radio, x *Saved) error {
	c := x.Cat
	if c == nil {
		return errors.New("the saved value is incomplete")
	}
	re, err := regexp.Compile(c.Reply)
	if err != nil || re.NumSubexp() != 1 {
		return errors.New("the saved reply pattern is unusable")
	}
	cmd, err := renderCat(c.Template, c.Before)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(cmd, strings.TrimSuffix(c.Read, ";")) {
		return errors.New("the saved command doesn't match its read command")
	}
	_, err = s.setCat(ctx, r, "restore", x.Name, c.Read, re, cmd)
	return err
}

// logCAT records every raw CAT command sent (sets in the journal, reads and sets in an append-only file
// (presets/cat-log.jsonl in the state folder), so the operator can see exactly what went out.
func (s *Service) logCAT(rigID, preset, step, cmd string, err error) {
	s.logCATUse("set", rigID, preset, step, cmd, "", err)
}

func (s *Service) logCATUse(use, rigID, preset, step, cmd, reply string, err error) {
	entry := map[string]any{"time": time.Now().UTC().Format(time.RFC3339), "use": use, "rig": rigID, "preset": preset, "step": step, "command": cmd, "ok": err == nil}
	if use == "read" {
		entry["reply"] = reply
		if err != nil {
			entry["error"] = err.Error()
		}
		s.log.Debug("raw CAT read", "rig", rigID, "preset", preset, "step", step, "command", cmd, "reply", reply, "err", err)
	} else if err != nil {
		entry["error"] = err.Error()
		s.log.Warn("raw CAT not taken", "rig", rigID, "preset", preset, "step", step, "command", cmd, "err", err)
	} else {
		s.log.Info("raw CAT sent", "rig", rigID, "preset", preset, "step", step, "command", cmd)
	}
	if s.dir == "" {
		return
	}
	b, _ := json.Marshal(entry)
	p := filepath.Join(s.dir, "presets", "cat-log.jsonl")
	if mkErr := os.MkdirAll(filepath.Dir(p), 0o700); mkErr != nil {
		return
	}
	f, ferr := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if ferr != nil {
		s.log.Error("could not write the raw CAT log", "err", ferr)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}
