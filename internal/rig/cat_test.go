package rig

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

// Raw CAT refusals at the rig, whatever the caller.
func TestCATRefusals(t *testing.T) {
	ctx := context.Background()
	ro := New(Config{ID: "ro", Addr: "127.0.0.1:1", ReadOnly: true}, slog.Default())
	if _, err := ro.CATRead(ctx, "EX010416;"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("read on a read-only radio: %v", err)
	}
	if err := ro.CATSet(ctx, "EX0104161;"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("set on a read-only radio: %v", err)
	}

	r := dummyRig(t)
	for _, c := range []string{"TX1;", "MX1;", "EX0104161;TX1;", "FA00014074000;", "EX01 0416;", "AC002;", "ZZTX1;", "SM0050;"} {
		if err := r.CATSet(ctx, c); !errors.Is(err, ErrCATRefused) {
			t.Errorf("set %q: %v", c, err)
		}
	}
	// The dummy radio is made by "Hamlib", not a maker that speaks Kenwood/Yaesu-style CAT.
	if _, err := r.CATRead(ctx, "EX010416;"); !errors.Is(err, ErrCATRefused) {
		t.Errorf("read on a non-ASCII-CAT radio: %v", err)
	}
	// Pretend it is a Yaesu: allowed commands go through (the dummy echoes them).
	r.mu.Lock()
	c := *r.caps
	c.Mfg = "Yaesu"
	r.caps = &c
	r.mu.Unlock()
	if got, err := r.CATRead(ctx, "EX010416;"); err != nil || got != "EX010416;" {
		t.Errorf("allowed read: %q %v", got, err)
	}
	if err := r.CATSet(ctx, "EX0104161;"); err != nil {
		t.Errorf("allowed set: %v", err)
	}
	if err := r.CATSet(ctx, "PS0;"); !errors.Is(err, ErrCATRefused) {
		t.Errorf("power off: %v", err)
	}
	if r.Snapshot().PTT {
		t.Fatal("the radio was keyed")
	}
}
