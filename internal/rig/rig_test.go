package rig

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// startDummy runs Hamlib's dummy radio under rigctld and returns its address.
func startDummy(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("rigctld")
	if err != nil {
		t.Skip("rigctld not installed")
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	cmd := exec.Command(bin, "-m", "1", "-P", "RIG", "-t", strconv.Itoa(port), "-T", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for i := 0; i < 50; i++ {
		if rigctldAnswers(addr) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return addr
}

// runRig starts a rig against addr and waits until it is connected.
func runRig(t *testing.T, cfg Config) *Rig {
	t.Helper()
	r := New(cfg, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go r.Run(ctx)
	for i := 0; i < 100 && r.Caps() == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if r.Caps() == nil {
		t.Fatal("rig never connected")
	}
	return r
}

func dummyRig(t *testing.T) *Rig {
	t.Helper()
	return runRig(t, Config{ID: "t", Name: "t", Addr: startDummy(t)})
}

func TestTuneCooldown(t *testing.T) {
	r := dummyRig(t)
	ctx := context.Background()
	if err := r.Exec(ctx, Command{Name: "vfo_op", Args: map[string]any{"op": "TUNE"}}); err != nil {
		t.Fatalf("first tune: %v", err)
	}
	err := r.Exec(ctx, Command{Name: "vfo_op", Args: map[string]any{"op": "TUNE"}})
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("second tune should be refused, got %v", err)
	}
}

func TestReadOnlyRefusesCommands(t *testing.T) {
	r := New(Config{ID: "ro", Addr: "127.0.0.1:1", ReadOnly: true}, slog.Default())
	if err := r.Exec(context.Background(), Command{Name: "set_freq", Args: map[string]any{"hz": 7100000.0}}); err != ErrReadOnly {
		t.Fatalf("got %v, want ErrReadOnly", err)
	}
}

func TestCommandValidation(t *testing.T) {
	r := dummyRig(t)
	ctx := context.Background()
	for name, c := range map[string]Command{
		"low freq":    {Name: "set_freq", Args: map[string]any{"hz": 5.0}},
		"bad mode":    {Name: "set_mode", Args: map[string]any{"mode": "BOGUS"}},
		"bad level":   {Name: "set_level", Args: map[string]any{"name": "RFPOWER; set_ptt 1", "value": 0.5}},
		"unknown cmd": {Name: "rm_rf"},
	} {
		if err := r.Exec(ctx, c); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// slowProxy forwards to addr, delaying every client command line, to mimic a slow serial
// radio shared with other rigctld clients.
func slowProxy(t *testing.T, addr string, delayFor func(line string) time.Duration) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				up, err := net.Dial("tcp", addr)
				if err != nil {
					return
				}
				defer up.Close()
				go func() { _, _ = io.Copy(c, up) }()
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					time.Sleep(delayFor(line))
					if _, err := up.Write([]byte(line)); err != nil {
						return
					}
				}
			}()
		}
	}()
	return l.Addr().String()
}

// Regression: the old poller read every level and function in one burst under a 5 s
// deadline, so on a slow shared radio it timed out and redialled forever.
func TestSlowRadioStaysConnected(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	base := dummyRig(t)
	addr := slowProxy(t, base.cfg.Addr, func(string) time.Duration { return 200 * time.Millisecond })
	r := New(Config{ID: "slow", Name: "slow", Addr: addr}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	deadline := time.Now().Add(26 * time.Second)
	connectedOnce := false
	for time.Now().Before(deadline) {
		st := r.Snapshot()
		if st.Connected {
			connectedOnce = true
		} else if connectedOnce {
			t.Fatalf("session dropped on a slow radio: %q", st.Error)
		}
		time.Sleep(200 * time.Millisecond)
	}
	st := r.Snapshot()
	if !st.Connected || len(st.Funcs) == 0 || len(st.Levels) < 3 {
		t.Fatalf("background refresh incomplete: connected=%v funcs=%d levels=%d", st.Connected, len(st.Funcs), len(st.Levels))
	}
}

func TestDynamicControlsOnDummy(t *testing.T) {
	r := dummyRig(t)
	ctx := context.Background()
	caps := r.Caps()
	if len(caps.ExtLevels) == 0 || len(caps.Parms) == 0 {
		t.Fatalf("dummy should advertise extras: %d ext levels, %d parms", len(caps.ExtLevels), len(caps.Parms))
	}
	cmd := func(name string, args map[string]any) error { return r.Exec(ctx, Command{Name: name, Args: args}) }

	// Extra numeric level is accepted in range, rejected out of range.
	if err := cmd("set_level", map[string]any{"name": "MGL", "value": 0.5}); err != nil {
		t.Errorf("ext level: %v", err)
	}
	if err := cmd("set_level", map[string]any{"name": "MGL", "value": 7.0}); err == nil {
		t.Error("ext level out of range should be rejected")
	}
	// Enumerated parameter: valid option accepted, unknown option and injection rejected.
	if err := cmd("set_parm", map[string]any{"name": "KEYERTYPE", "value": "BUG"}); err != nil {
		t.Errorf("enum parm: %v", err)
	}
	for _, bad := range []string{"NOPE", "BUG; set_ptt 1", "BUG\nset_ptt 1"} {
		if err := cmd("set_parm", map[string]any{"name": "KEYERTYPE", "value": bad}); err == nil {
			t.Errorf("parm value %q should be rejected", bad)
		}
	}
	// Tone not in the rig's list is refused before reaching the radio.
	if err := cmd("set_ctcss_tone", map[string]any{"tenths": 1234.0}); err == nil {
		t.Error("unsupported CTCSS tone should be rejected")
	}
	if err := cmd("set_rptr_shift", map[string]any{"shift": "sideways"}); err == nil {
		t.Error("bad shift should be rejected")
	}
}

// Regression: a radio that takes seconds to answer one background read (an FT-DX10 and
// "get_func COMP" took 2.4 s every time) used to stall every user command queued behind it.
func TestSlowBackgroundReadDoesNotStallCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	base := dummyRig(t)
	var hits atomic.Int32
	addr := slowProxy(t, base.cfg.Addr, func(line string) time.Duration {
		if strings.Contains(line, "get_func COMP") {
			hits.Add(1)
			return 2400 * time.Millisecond
		}
		return 2 * time.Millisecond
	})
	r := New(Config{ID: "slowread", Name: "slowread", Addr: addr}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)
	for i := 0; i < 100 && r.Caps() == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if r.Caps() == nil {
		t.Fatal("never connected")
	}
	// Let the poller reach the slow read and bench it. How long the first cycle takes depends on
	// the Hamlib version (4.6's dummy times out on some parms), so wait for the read itself.
	for end := time.Now().Add(40 * time.Second); hits.Load() == 0 && time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
	}
	if hits.Load() == 0 {
		t.Fatal("the poller never read COMP")
	}
	time.Sleep(4 * time.Second) // the slow read finishes and the next cycles skip it
	if n := hits.Load(); n > 2 {
		t.Errorf("get_func COMP was polled %d times; a slow read should be benched after one or two", n)
	}
	// A user command now must complete quickly instead of queueing behind the slow read.
	start := time.Now()
	if err := r.Exec(context.Background(), Command{Name: "set_freq", Args: map[string]any{"hz": 14074000.0}}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 800*time.Millisecond {
		t.Errorf("set_freq took %v behind background polling", d)
	}
}

// rigctldAnswers reports whether rigctld at addr completes a real command. Accepting a
// connection is not enough: right after startup the first client can see EOF.
func rigctldAnswers(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte("+\\get_freq\n")); err != nil {
		return false
	}
	buf := make([]byte, 256)
	n, err := c.Read(buf)
	return err == nil && n > 0
}
