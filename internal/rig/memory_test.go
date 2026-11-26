package rig

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

// storeOnDummy writes a memory on Hamlib's dummy radio (test only: the app never stores).
func storeOnDummy(t *testing.T, addr string, n int, hz int64, mode string) {
	t.Helper()
	c := other(t, addr)
	ctx := context.Background()
	if err := c.SetFreq(ctx, hz); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMode(ctx, mode, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(ctx, `\set_mem `+strconv.Itoa(n)); err != nil {
		t.Fatal(err)
	}
	if err := c.VFOOp(ctx, "FROM_VFO"); err != nil {
		t.Fatal(err)
	}
}

func TestMemoriesOnDummy(t *testing.T) {
	addr := startDummy(t)
	storeOnDummy(t, addr, 3, 7074000, "USB")
	r := runRig(t, Config{ID: "t", Name: "t", Addr: addr})
	memoryReadGap = 10 * time.Millisecond

	v := r.Memories()
	if !v.Supported || len(v.Banks) == 0 || v.Banks[0].Type != "MEM" {
		t.Fatalf("banks = %+v (%s)", v.Banks, v.Reason)
	}
	// Bounds: outside the radio's banks, and too many at once, are refused.
	if err := r.ReadMemories(500, 501); err == nil {
		t.Error("reading outside the banks should fail")
	}
	if err := r.ReadMemories(0, maxMemoryRead+5); err == nil {
		t.Error("an over-long read should fail")
	}

	if err := r.ReadMemories(0, 5); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, "memory read", func() bool { m := r.Memories(); return !m.Reading && m.Done == 6 })
	v = r.Memories()
	if v.Error != "" || len(v.Channels) != 6 {
		t.Fatalf("after read: %+v", v)
	}
	var ch3 bool
	for _, ch := range v.Channels {
		if ch.Number == 3 {
			ch3 = !ch.Empty && ch.FreqHz == 7074000 && ch.Mode == "USB"
		} else if !ch.Empty {
			t.Errorf("channel %d should be empty: %+v", ch.Number, ch)
		}
	}
	if !ch3 {
		t.Fatalf("channel 3 not read correctly: %+v", v.Channels)
	}

	// Recall tunes the VFO; it does not leave the radio in memory mode.
	if err := r.Exec(context.Background(), Command{Name: "set_freq", Args: map[string]any{"hz": 14074000.0}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Exec(context.Background(), Command{Name: "recall_channel", Args: map[string]any{"channel": 3.0}}); err != nil {
		t.Fatal(err)
	}
	c := other(t, addr)
	if hz, _ := c.GetFreq(context.Background()); hz != 7074000 {
		t.Errorf("after recall the radio is on %d Hz", hz)
	}
	if vfo, _ := c.GetVFO(context.Background()); vfo == "MEM" {
		t.Error("recall left the radio in memory mode")
	}
	if err := r.Exec(context.Background(), Command{Name: "recall_channel", Args: map[string]any{"channel": 4.0}}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("recalling an empty channel: %v", err)
	}
	if ptt(t, c) {
		t.Fatal("PTT went on")
	}
}

func TestMemoryReadRefusedWhileTransmitting(t *testing.T) {
	r := dummyRig(t)
	r.mu.Lock()
	r.state.PTT = true
	r.mu.Unlock()
	if err := r.ReadMemories(0, 1); err == nil || !strings.Contains(err.Error(), "transmitting") {
		t.Fatalf("read while keyed: %v", err)
	}
}

func TestRecallRefusedOnReadOnlyRig(t *testing.T) {
	r := runRig(t, Config{ID: "t", Name: "t", Addr: startDummy(t), ReadOnly: true})
	if err := r.Exec(context.Background(), Command{Name: "recall_channel", Args: map[string]any{"channel": 1.0}}); err != ErrReadOnly {
		t.Fatalf("recall on read-only rig: %v", err)
	}
	if err := r.ReadMemories(0, 1); err != nil {
		t.Fatalf("reading is allowed on a read-only rig: %v", err)
	}
}
