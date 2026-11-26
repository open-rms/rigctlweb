package rig

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-rms/rigctlweb/internal/rigctl"
)

func pttCmd(on bool) Command { return Command{Name: "set_ptt", Args: map[string]any{"on": on}} }

// other is a second program (pat, ardopcf...) talking to the same rigctld.
func other(t *testing.T, addr string) *rigctl.Client {
	t.Helper()
	c := rigctl.NewClient(addr)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func ptt(t *testing.T, c *rigctl.Client) bool {
	t.Helper()
	on, err := c.GetPTT(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return on
}

func waitFor(t *testing.T, d time.Duration, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestUnkeyOnlyTouchesWhatWeKeyed(t *testing.T) {
	addr := startDummy(t)
	dir := t.TempDir()
	r := runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: dir})
	pat := other(t, addr)
	ctx := context.Background()

	// Another program is transmitting. We never keyed, so Unkey must leave it alone.
	if err := pat.SetPTT(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := r.Unkey(ctx); err != nil {
		t.Fatal(err)
	}
	if !ptt(t, pat) {
		t.Fatal("Unkey cut another program's transmission")
	}
	_ = pat.SetPTT(ctx, false)

	// Now we key it: marker appears, Unkey releases it and removes the marker.
	if err := r.Exec(ctx, pttCmd(true)); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "keyed-t")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("no marker while keyed: %v", err)
	}
	if !r.HoldsKey() || !ptt(t, pat) {
		t.Fatal("expected to hold the key and the radio to be keyed")
	}
	if err := r.Unkey(ctx); err != nil {
		t.Fatal(err)
	}
	if ptt(t, pat) || r.HoldsKey() {
		t.Fatal("our own key was not released")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker should be gone: %v", err)
	}
}

func TestClientDisconnectUnkeysOnlyItsOwnKey(t *testing.T) {
	addr := startDummy(t)
	r := runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: t.TempDir()})
	pat := other(t, addr)
	ctx := context.Background()

	if err := r.Exec(WithController(ctx, 7), pttCmd(true)); err != nil {
		t.Fatal(err)
	}
	// A different browser leaving must not unkey it.
	_ = r.ControllerLeft(ctx, 8)
	if !ptt(t, pat) {
		t.Fatal("an unrelated client leaving unkeyed the radio")
	}
	// The one that keyed it leaving must.
	if err := r.ControllerLeft(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if ptt(t, pat) {
		t.Fatal("the radio stayed keyed after the keying client left")
	}
}

func TestTXTimeoutStillUnkeysOurKey(t *testing.T) {
	addr := startDummy(t)
	r := runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: t.TempDir(), TXTimeout: 500 * time.Millisecond})
	pat := other(t, addr)
	if err := r.Exec(context.Background(), pttCmd(true)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, "the hard TX limit to drop the radio", func() bool { return !ptt(t, pat) })
	if r.HoldsKey() {
		t.Fatal("still marked as keyed after the limit unkeyed it")
	}
}

func TestStopsHoldingOnceRadioReportsUnkeyed(t *testing.T) {
	addr := startDummy(t)
	r := runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: t.TempDir()})
	pat := other(t, addr)
	ctx := context.Background()
	if err := r.Exec(ctx, pttCmd(true)); err != nil {
		t.Fatal(err)
	}
	// Someone else releases it (or a tune ended). After the poller sees that, the key is no
	// longer ours, so a later Unkey must not cut whoever transmits next.
	_ = pat.SetPTT(ctx, false)
	waitFor(t, 8*time.Second, "the poller to notice the radio is unkeyed", func() bool { return !r.HoldsKey() })
	_ = pat.SetPTT(ctx, true)
	_ = r.Unkey(ctx)
	if !ptt(t, pat) {
		t.Fatal("Unkey cut a transmission that started after ours ended")
	}
	_ = pat.SetPTT(ctx, false)
}

func TestCrashRecovery(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh marker and radio keyed: unkey", func(t *testing.T) {
		addr := startDummy(t)
		dir := t.TempDir()
		pat := other(t, addr)
		_ = pat.SetPTT(ctx, true)
		marker := filepath.Join(dir, "keyed-t")
		if err := os.WriteFile(marker, []byte("1 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: dir})
		waitFor(t, 5*time.Second, "crash recovery to unkey", func() bool { return !ptt(t, pat) })
		waitFor(t, 2*time.Second, "marker removal", func() bool { _, err := os.Stat(marker); return os.IsNotExist(err) })
	})

	t.Run("no marker: someone else's transmission is left alone", func(t *testing.T) {
		addr := startDummy(t)
		pat := other(t, addr)
		_ = pat.SetPTT(ctx, true)
		runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: t.TempDir()})
		time.Sleep(time.Second)
		if !ptt(t, pat) {
			t.Fatal("start-up unkeyed a radio that this program never keyed")
		}
	})

	t.Run("old marker: ignored and removed", func(t *testing.T) {
		addr := startDummy(t)
		dir := t.TempDir()
		pat := other(t, addr)
		_ = pat.SetPTT(ctx, true)
		marker := filepath.Join(dir, "keyed-t")
		_ = os.WriteFile(marker, []byte("1 1\n"), 0o600)
		old := time.Now().Add(-2 * staleMarker)
		_ = os.Chtimes(marker, old, old)
		runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: dir})
		time.Sleep(time.Second)
		if !ptt(t, pat) {
			t.Fatal("an old marker caused an unkey")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("old marker should be removed")
		}
	})

	t.Run("fresh marker but radio already off: just clear it", func(t *testing.T) {
		addr := startDummy(t)
		dir := t.TempDir()
		marker := filepath.Join(dir, "keyed-t")
		_ = os.WriteFile(marker, []byte("1 1\n"), 0o600)
		runRig(t, Config{ID: "t", Name: "t", Addr: addr, StateDir: dir})
		waitFor(t, 3*time.Second, "marker removal", func() bool { _, err := os.Stat(marker); return os.IsNotExist(err) })
	})
}

// A termination signal cancels the context the pollers run on. The radio must still be
// dropped, using a fresh context, and only if it was ours.
func TestUnkeyAllWorksAfterSignalCancelsPollers(t *testing.T) {
	addr := startDummy(t)
	root, cancel := context.WithCancel(context.Background())
	m := NewManager(root, slog.Default())
	m.SetStateDir(t.TempDir())
	if err := m.Add(Config{ID: "t", Name: "t", Addr: addr}); err != nil {
		t.Fatal(err)
	}
	rg, _ := m.Get("t")
	waitFor(t, 10*time.Second, "connect", func() bool { return rg.Caps() != nil })
	pat := other(t, addr)
	if err := rg.Exec(context.Background(), pttCmd(true)); err != nil {
		t.Fatal(err)
	}

	cancel() // what SIGTERM does to the manager's context
	time.Sleep(300 * time.Millisecond)
	uctx, ucancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ucancel()
	start := time.Now()
	m.UnkeyAll(uctx)
	if ptt(t, pat) {
		t.Fatal("radio still keyed after UnkeyAll")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("unkey took longer than 5 s")
	}
}
