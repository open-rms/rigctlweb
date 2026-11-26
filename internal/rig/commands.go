package rig

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// Command is a UI-originated action. Args are validated per command.
type Command struct {
	Name string         `json:"cmd"`
	Args map[string]any `json:"args"`
}

// tuneCooldown is the minimum gap between accepted TUNE cycles.
const tuneCooldown = 5 * time.Second

// tuneMaxCarrier is the longest a tune carrier may stay up before the server drops it.
const tuneMaxCarrier = 30 * time.Second

var ErrReadOnly = errors.New("rig is read-only")

// ErrNotConnected means the rig has not been reached yet (or the connection dropped).
var ErrNotConnected = errors.New("rig not connected")

func num(a map[string]any, k string) (float64, error) {
	v, ok := a[k].(float64)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("argument %q must be a number", k)
	}
	return v, nil
}

func str(a map[string]any, k string) (string, error) {
	v, ok := a[k].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("argument %q must be a string", k)
	}
	return v, nil
}

func boolean(a map[string]any, k string) (bool, error) {
	v, ok := a[k].(bool)
	if !ok {
		return false, fmt.Errorf("argument %q must be a boolean", k)
	}
	return v, nil
}

// Exec applies a command. It validates against probed capabilities so the UI can't
// send something the rig never advertised.
func (r *Rig) Exec(ctx context.Context, cmd Command) error {
	if r.cfg.ReadOnly {
		return ErrReadOnly
	}
	caps := r.Caps()
	if caps == nil {
		return ErrNotConnected
	}
	budget := 5 * time.Second
	if cmd.Name == "recall_channel" {
		budget = 15 * time.Second // reads the channel, then up to six settings over a slow serial line
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	c := r.client
	r.pending.Add(1)
	defer r.pending.Add(-1)
	defer r.wake()

	switch cmd.Name {
	case "set_freq":
		hz, err := num(cmd.Args, "hz")
		if err != nil {
			return err
		}
		if hz < 1000 || hz > 1e12 {
			return fmt.Errorf("frequency %.0f Hz out of range", hz)
		}
		if err := c.SetFreq(ctx, int64(hz)); err != nil {
			return err
		}
		r.patch("freq", func(st *State) { st.Freq = int64(hz) })
		return nil

	case "set_mode":
		m, err := str(cmd.Args, "mode")
		if err != nil {
			return err
		}
		if !contains(caps.Modes, m) {
			return fmt.Errorf("mode %q not supported by this rig", m)
		}
		pb := 0
		if v, err := num(cmd.Args, "passband"); err == nil {
			pb = int(v)
		}
		if err := c.SetMode(ctx, m, pb); err != nil {
			return err
		}
		r.patch("mode", func(st *State) { st.Mode = m })
		return nil

	case "set_ptt":
		on, err := boolean(cmd.Args, "on")
		if err != nil {
			return err
		}
		if !caps.CanPTT {
			return errors.New("rig has no PTT control configured in rigctld")
		}
		if on {
			r.markKeyed(ctx) // before the command: a crash in between must err toward unkeying
		}
		if err := c.SetPTT(ctx, on); err != nil {
			if on {
				r.abortKey()
			}
			return err
		}
		r.patch("ptt", func(st *State) { st.PTT = on })
		r.armTXTimer(on)
		if !on {
			r.clearKeyed()
		}
		return nil

	case "set_vfo":
		v, err := str(cmd.Args, "vfo")
		if err != nil {
			return err
		}
		if !contains(caps.VFOs, v) {
			return fmt.Errorf("vfo %q not supported", v)
		}
		if err := c.SetVFO(ctx, v); err != nil {
			return err
		}
		r.patch("vfo", func(st *State) { st.VFO = v })
		return nil

	case "set_split":
		on, err := boolean(cmd.Args, "on")
		if err != nil {
			return err
		}
		tx, _ := str(cmd.Args, "txVfo")
		if tx == "" {
			tx = "VFOB"
		}
		if err := c.SetSplit(ctx, on, tx); err != nil {
			return err
		}
		r.patch("split", func(st *State) { st.Split, st.TXVFO = on, tx })
		return nil

	case "set_split_freq":
		hz, err := num(cmd.Args, "hz")
		if err != nil {
			return err
		}
		if err := c.SetSplitFreq(ctx, int64(hz)); err != nil {
			return err
		}
		r.patch("txfreq", func(st *State) { st.TXFreq = int64(hz) })
		return nil

	case "set_rit", "set_xit":
		hz, err := num(cmd.Args, "hz")
		if err != nil {
			return err
		}
		limit := caps.MaxRIT
		if cmd.Name == "set_xit" {
			limit = caps.MaxXIT
		}
		if limit == 0 || math.Abs(hz) > float64(limit) {
			return fmt.Errorf("%s out of range (±%d Hz)", cmd.Name, limit)
		}
		if cmd.Name == "set_rit" {
			if err := c.SetRIT(ctx, int(hz)); err != nil {
				return err
			}
			r.patch("rit", func(st *State) { st.RIT = int(hz) })
			return nil
		}
		if err := c.SetXIT(ctx, int(hz)); err != nil {
			return err
		}
		r.patch("xit", func(st *State) { st.XIT = int(hz) })
		return nil

	case "set_level":
		name, err := str(cmd.Args, "name")
		if err != nil {
			return err
		}
		v, err := num(cmd.Args, "value")
		if err != nil {
			return err
		}
		for _, l := range caps.Levels {
			if l.Name == name && l.Set {
				if l.Max > l.Min && (v < l.Min || v > l.Max) {
					return fmt.Errorf("%s out of range (%g..%g)", name, l.Min, l.Max)
				}
				if err := c.SetLevel(ctx, name, v); err != nil {
					return err
				}
				r.patch("lvl:"+name, func(st *State) { st.Levels[name] = v })
				return nil
			}
		}
		for _, l := range caps.ExtLevels {
			if l.Name != name || l.Type == "BUTTON" || l.Type == "STRING" {
				continue
			}
			if l.Range != nil && l.Range.Max > l.Range.Min && (v < l.Range.Min || v > l.Range.Max) {
				return fmt.Errorf("%s out of range (%g..%g)", name, l.Range.Min, l.Range.Max)
			}
			if err := c.SetLevel(ctx, name, v); err != nil {
				return err
			}
			r.patch("lvl:"+name, func(st *State) { st.Levels[name] = v })
			return nil
		}
		return fmt.Errorf("level %q not settable on this rig", name)

	case "set_func":
		name, err := str(cmd.Args, "name")
		if err != nil {
			return err
		}
		on, err := boolean(cmd.Args, "on")
		if err != nil {
			return err
		}
		if !contains(caps.SetFuncs, name) && !hasExt(caps.ExtFuncs, name) {
			return fmt.Errorf("function %q not settable on this rig", name)
		}
		if err := c.SetFunc(ctx, name, on); err != nil {
			return err
		}
		r.patch("fn:"+name, func(st *State) { st.Funcs[name] = on })
		return nil

	case "set_ctcss_tone", "set_ctcss_sql":
		v, err := num(cmd.Args, "tenths")
		if err != nil {
			return err
		}
		t := int(v)
		if t != 0 && !containsInt(caps.CTCSS, t) {
			return fmt.Errorf("CTCSS tone %.1f Hz not supported by this rig", float64(t)/10)
		}
		if cmd.Name == "set_ctcss_tone" {
			if !caps.Can["set CTCSS"] {
				return errors.New("this rig can't set a CTCSS tone")
			}
			if err := c.SetCTCSSTone(ctx, t); err != nil {
				return err
			}
			r.patch("ctcss", func(st *State) { st.CTCSS = t })
			return nil
		}
		if !caps.Can["set CTCSS Squelch"] {
			return errors.New("this rig can't set a CTCSS squelch tone")
		}
		if err := c.SetCTCSSSql(ctx, t); err != nil {
			return err
		}
		r.patch("ctcsssql", func(st *State) { st.CTCSSSql = t })
		return nil

	case "set_rptr_shift":
		sh, err := str(cmd.Args, "shift")
		if err != nil {
			return err
		}
		if !caps.Can["set Repeater Duplex"] {
			return errors.New("this rig can't set a repeater shift")
		}
		if err := c.SetRptrShift(ctx, sh); err != nil {
			return err
		}
		r.patch("rptrshift", func(st *State) { st.RptrShift = sh })
		return nil

	case "set_rptr_offs":
		hz, err := num(cmd.Args, "hz")
		if err != nil {
			return err
		}
		if !caps.Can["set Repeater Offset"] || hz < 0 || hz > 100e6 {
			return errors.New("repeater offset not supported or out of range")
		}
		if err := c.SetRptrOffs(ctx, int(hz)); err != nil {
			return err
		}
		r.patch("rptroffs", func(st *State) { st.RptrOffs = int(hz) })
		return nil

	case "set_parm":
		name, err := str(cmd.Args, "name")
		if err != nil {
			return err
		}
		val, err := str(cmd.Args, "value")
		if err != nil {
			return err
		}
		for _, p := range caps.Parms {
			if p.Name != name || !p.Set {
				continue
			}
			if len(p.Options) > 0 && !contains(p.Options, val) {
				return fmt.Errorf("%s: %q is not one of %v", name, val, p.Options)
			}
			if len(p.Options) == 0 && p.Range.Max > p.Range.Min {
				f, perr := strconv.ParseFloat(val, 64)
				if perr != nil || f < p.Range.Min || f > p.Range.Max {
					return fmt.Errorf("%s out of range (%g..%g)", name, p.Range.Min, p.Range.Max)
				}
			}
			if err := c.SetParm(ctx, name, val); err != nil {
				return err
			}
			r.patch("parm:"+name, func(st *State) { st.Parms[name] = val })
			return nil
		}
		for _, e := range caps.ExtParms {
			if e.Name != name || e.Type == "BUTTON" || e.Type == "STRING" || e.Type == "BINARY" {
				continue
			}
			f, perr := strconv.ParseFloat(val, 64)
			if perr != nil {
				return fmt.Errorf("%s needs a number", name)
			}
			if e.Range != nil && e.Range.Max > e.Range.Min && (f < e.Range.Min || f > e.Range.Max) {
				return fmt.Errorf("%s out of range (%g..%g)", name, e.Range.Min, e.Range.Max)
			}
			if len(e.Values) > 0 {
				ok := false
				for _, v := range e.Values {
					ok = ok || float64(v.Value) == f
				}
				if !ok {
					return fmt.Errorf("%s: %q is not one of its choices", name, val)
				}
			}
			if err := c.SetParm(ctx, name, val); err != nil {
				return err
			}
			r.patch("parm:"+name, func(st *State) { st.Parms[name] = val })
			return nil
		}
		return fmt.Errorf("parameter %q not settable on this rig", name)

	case "recall_channel":
		n, err := num(cmd.Args, "channel")
		if err != nil {
			return err
		}
		return r.recallChannel(ctx, int(n))

	case "vfo_op":
		op, err := str(cmd.Args, "op")
		if err != nil {
			return err
		}
		if !contains(caps.VFOOps, op) {
			return fmt.Errorf("vfo op %q not supported", op)
		}
		if op == "TUNE" {
			// A tune cycle transmits a carrier; refuse rapid repeats from any client.
			r.mu.Lock()
			if since := time.Since(r.lastTune); since < tuneCooldown {
				r.mu.Unlock()
				return fmt.Errorf("tuner is busy; wait %.0f s between tune cycles", (tuneCooldown-since).Seconds()+1)
			}
			r.lastTune = time.Now()
			r.mu.Unlock()
			r.log.Info("tune cycle requested")
		}
		if op == "TUNE" {
			r.markKeyed(ctx)
		}
		if err := c.VFOOp(ctx, op); err != nil {
			if op == "TUNE" {
				r.abortKey()
			}
			return err
		}
		if op == "TUNE" {
			// Hard cap: if the carrier is still up, and is still ours, it is dropped.
			time.AfterFunc(tuneMaxCarrier, func() {
				if !r.HoldsKey() {
					return // the radio reported itself unkeyed since; whatever is on air now isn't ours
				}
				if !r.Snapshot().PTT {
					r.clearKeyed()
					return
				}
				r.log.Warn("tune carrier exceeded limit; unkeying", "limit", tuneMaxCarrier)
				uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = r.Unkey(uctx)
			})
		}
		return nil
	}
	return fmt.Errorf("unknown command %q", cmd.Name)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// armTXTimer enforces a hard transmit limit: if PTT stays on longer than TXTimeout
// the rig is unkeyed regardless of what any client does.
func (r *Rig) armTXTimer(on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.txTimer != nil {
		r.txTimer.Stop()
		r.txTimer = nil
	}
	if !on {
		return
	}
	r.txTimer = time.AfterFunc(r.cfg.TXTimeout, func() {
		r.log.Warn("TX timeout reached; unkeying", "limit", r.cfg.TXTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.Unkey(ctx)
	})
}

// holdFor is how long a confirmed change is kept authoritative against in-flight reads.
const holdFor = 800 * time.Millisecond

// patch applies a confirmed change to the live state immediately, so the UI doesn't wait
// for the next background read. The change is re-applied over polled values for a short
// hold window, so a read that raced the command can't flash the old value back.
func (r *Rig) patch(key string, fn func(*State)) {
	r.mu.Lock()
	fn(&r.state)
	r.state.Updated = time.Now()
	if r.holds == nil {
		r.holds = map[string]hold{}
	}
	r.holds[key] = hold{until: time.Now().Add(holdFor), fn: fn}
	r.mu.Unlock()
	r.publish()
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func hasExt(list []rigctl.ExtField, name string) bool {
	for _, f := range list {
		if f.Name == name && f.Type != "BUTTON" {
			return true
		}
	}
	return false
}
