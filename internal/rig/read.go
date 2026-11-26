package rig

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Setting is a value read on demand from the radio. Mode readings carry the passband.
type Setting struct {
	Mode     string  `json:"mode,omitempty"`
	Passband int     `json:"passband,omitempty"`
	Number   float64 `json:"number,omitempty"`
	On       bool    `json:"on,omitempty"`
	Text     string  `json:"text,omitempty"`
}

// ReadSetting reads one setting straight from the radio, outside the poller: kind is "mode",
// "level", "func" or "parm". The name must be one the radio's capabilities can read. Reading
// changes nothing on the radio.
func (r *Rig) ReadSetting(ctx context.Context, kind, name string) (Setting, error) {
	caps := r.Caps()
	if caps == nil {
		return Setting{}, ErrNotConnected
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r.pending.Add(1)
	defer r.pending.Add(-1)
	c := r.client
	switch kind {
	case "mode":
		m, pb, err := c.GetMode(ctx)
		return Setting{Mode: m, Passband: pb}, err
	case "level":
		if !caps.HasLevel(name) && !hasExt(caps.ExtLevels, name) {
			return Setting{}, fmt.Errorf("this radio cannot read level %s", name)
		}
		v, err := c.GetLevel(ctx, name)
		return Setting{Number: v}, err
	case "func":
		if !contains(caps.GetFuncs, name) && !hasExt(caps.ExtFuncs, name) {
			return Setting{}, fmt.Errorf("this radio cannot read function %s", name)
		}
		on, err := c.GetFunc(ctx, name)
		return Setting{On: on}, err
	case "parm":
		if !contains(caps.GetParms, name) && !hasExt(caps.ExtParms, name) {
			return Setting{}, fmt.Errorf("this radio cannot read parameter %s", name)
		}
		v, err := c.GetParm(ctx, name)
		s := Setting{Text: v}
		if f, perr := strconv.ParseFloat(v, 64); perr == nil {
			s.Number = f
		}
		return s, err
	}
	return Setting{}, fmt.Errorf("cannot read a %q setting", kind)
}
