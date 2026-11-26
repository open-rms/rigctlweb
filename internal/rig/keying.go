package rig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Keying safety. rigctld is shared: pat, ardopcf, WSJT-X and others key the same radio
// through it. So rigctlWeb unkeys the transmitter only when it keyed it itself. A blind
// "set_ptt 0" on start-up or when a browser leaves could cut another program's
// transmission mid-over.
//
// "Keyed by us" is tracked in memory and in a small marker file, written before the key-up
// command is sent and removed once the radio is confirmed unkeyed. If this program dies
// while keyed, the marker lets the next start recover (see recoverKey). A marker that is
// old is ignored, because by then whatever is keying the radio is more likely someone else.

const (
	unkeyAttempts = 3
	unkeyGap      = 150 * time.Millisecond
	// staleMarker is how old a marker may be and still be trusted at start-up.
	staleMarker = 15 * time.Minute
	// observeGrace is how long after keying a "PTT is off" reading is ignored (the radio
	// and rigctld need a moment to report the change).
	observeGrace = 3 * time.Second
)

var safeRigID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type ctrlKey struct{}

// WithController tags ctx with the ID of the client (a browser connection) issuing commands,
// so the rig knows who keyed it and can unkey when that client goes away.
func WithController(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, ctrlKey{}, id)
}

func controllerOf(ctx context.Context) uint64 {
	id, _ := ctx.Value(ctrlKey{}).(uint64)
	return id
}

func (r *Rig) markerPath() string {
	if r.cfg.StateDir == "" || !safeRigID.MatchString(r.cfg.ID) {
		return ""
	}
	return filepath.Join(r.cfg.StateDir, "keyed-"+r.cfg.ID)
}

// markKeyed records that this program is about to key the transmitter. Called before the
// key-up command so a crash in between errs on the side of unkeying.
func (r *Rig) markKeyed(ctx context.Context) {
	r.kmu.Lock()
	r.held = true
	r.keyedBy = controllerOf(ctx)
	r.keyedAt = time.Now()
	r.kmu.Unlock()
	p := r.markerPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err == nil {
		err = os.WriteFile(p, []byte(fmt.Sprintf("%d %d\n", os.Getpid(), time.Now().Unix())), 0o600)
		if err != nil {
			r.log.Warn("could not write keying marker", "err", err)
		}
	}
}

// clearKeyed forgets that we hold the key (the radio is unkeyed or was never keyed).
func (r *Rig) clearKeyed() {
	r.kmu.Lock()
	r.held = false
	r.keyedBy = 0
	r.kmu.Unlock()
	if p := r.markerPath(); p != "" {
		_ = os.Remove(p)
	}
}

// HoldsKey reports whether this program keyed the transmitter and hasn't unkeyed it.
func (r *Rig) HoldsKey() bool {
	r.kmu.Lock()
	defer r.kmu.Unlock()
	return r.held
}

// sendUnkey sends PTT off, retrying briefly. It does not look at who keyed the radio.
func (r *Rig) sendUnkey(ctx context.Context) error {
	var err error
	for i := 0; i < unkeyAttempts; i++ {
		if err = r.client.SetPTT(ctx, false); err == nil {
			return nil
		}
		if i < unkeyAttempts-1 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("unkey: %w (after %v)", err, ctx.Err())
			case <-time.After(unkeyGap):
			}
		}
	}
	return fmt.Errorf("unkey failed after %d attempts: %w", unkeyAttempts, err)
}

// Unkey drops PTT if, and only if, this program keyed the radio. It retries, and keeps the
// marker if every attempt fails so the next start can try again. Returns nil when there was
// nothing of ours to release.
func (r *Rig) Unkey(ctx context.Context) error {
	if r.cfg.ReadOnly || !r.HoldsKey() {
		return nil
	}
	if err := r.sendUnkey(ctx); err != nil {
		r.log.Error("could not unkey the radio", "err", err)
		return err
	}
	r.clearKeyed()
	r.armTXTimer(false)
	r.log.Info("unkeyed")
	r.wake()
	return nil
}

// ControllerLeft is called when a client connection ends. If that client keyed the radio
// and nothing has taken over since, unkey.
func (r *Rig) ControllerLeft(ctx context.Context, id uint64) error {
	if id == 0 {
		return nil
	}
	r.kmu.Lock()
	ours := r.held && r.keyedBy == id
	r.kmu.Unlock()
	if !ours {
		return nil
	}
	r.log.Warn("the client that keyed the radio disconnected; unkeying")
	return r.Unkey(ctx)
}

// abortKey is for a key-up command that failed: make sure the radio is off, then forget it.
func (r *Rig) abortKey() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = r.client.SetPTT(ctx, false)
	r.clearKeyed()
}

// observePTT is fed every PTT reading from the poller. Once the radio reports itself
// unkeyed (a tune cycle ended, or something else released it), we no longer hold the key,
// so a later shutdown won't cut whoever transmits next.
func (r *Rig) observePTT(on bool) {
	if on {
		return
	}
	r.kmu.Lock()
	release := r.held && time.Since(r.keyedAt) > observeGrace
	r.kmu.Unlock()
	if release {
		r.clearKeyed()
		r.armTXTimer(false)
	}
}

// recoverKey runs once after the first successful connection. If a previous run left a fresh
// marker and the radio is still transmitting, that run died while keyed: unkey it. Otherwise
// just clear the marker. Nothing is sent without a marker.
func (r *Rig) recoverKey(ctx context.Context) {
	r.kmu.Lock()
	done := r.recovered
	r.recovered = true
	r.kmu.Unlock()
	p := r.markerPath()
	if done || p == "" || r.cfg.ReadOnly {
		return
	}
	st, err := os.Stat(p)
	if err != nil {
		return
	}
	if age := time.Since(st.ModTime()); age > staleMarker {
		r.log.Info("ignoring an old keying marker", "age", age.Round(time.Second))
		_ = os.Remove(p)
		return
	}
	on, err := r.client.GetPTT(ctx)
	if err != nil {
		r.log.Warn("could not check PTT for crash recovery; keeping the marker", "err", err)
		return
	}
	if !on {
		_ = os.Remove(p)
		return
	}
	r.log.Warn("the previous run left the radio keyed; unkeying")
	r.kmu.Lock()
	r.held, r.keyedBy, r.keyedAt = true, 0, time.Now().Add(-observeGrace)
	r.kmu.Unlock()
	_ = r.Unkey(ctx)
}
