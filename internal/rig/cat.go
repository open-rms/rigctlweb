package rig

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/open-rms/rigctlweb/internal/catallow"
)

// Raw CAT is deliberately not a Command: Exec is reachable from the WebSocket and API v1, and
// raw CAT must not be. Only the presets service calls these, for profile steps the operator
// reviewed and confirmed.

// ErrCATRefused means a raw CAT command was refused before it was sent.
var ErrCATRefused = errors.New("raw CAT refused")

// ErrTransmitting refuses raw CAT while the radio transmits.
var ErrTransmitting = errors.New("the radio is transmitting; try again when it is not")

func (r *Rig) catReady(use, cmd string) error {
	if r.cfg.ReadOnly {
		return ErrReadOnly // raw CAT, reads included, stays off on read-only radios
	}
	caps := r.Caps()
	if caps == nil {
		return ErrNotConnected
	}
	if r.Snapshot().PTT {
		return ErrTransmitting
	}
	if res := catallow.Check("ascii", use, cmd); !res.Allowed {
		return fmt.Errorf("%w: %s", ErrCATRefused, res.Why)
	}
	// Only ASCII CAT is sent: Icom CI-V needs framing with the radio's address, which rigctlWeb
	// doesn't do yet.
	if p := catallow.Get("ascii"); !p.ForManufacturer(caps.Mfg) {
		return fmt.Errorf("%w: rigctlWeb sends raw CAT only to Kenwood, Yaesu and Elecraft radios, not %s", ErrCATRefused, caps.Mfg)
	}
	return nil
}

// CATRead sends an allow-listed raw CAT read command and returns the radio's answer.
func (r *Rig) CATRead(ctx context.Context, cmd string) (string, error) {
	if err := r.catReady("read", cmd); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r.pending.Add(1)
	defer r.pending.Add(-1)
	return r.client.SendCAT(ctx, cmd, true)
}

// CATSet sends an allow-listed raw CAT set command. The radio does not answer a set, so the
// caller reads the value back to see whether it took.
func (r *Rig) CATSet(ctx context.Context, cmd string) error {
	if err := r.catReady("set", cmd); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r.pending.Add(1)
	defer r.pending.Add(-1)
	defer r.wake()
	_, err := r.client.SendCAT(ctx, cmd, false)
	return err
}
