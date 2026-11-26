package rigctl

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// startDummy launches `rigctld -m 1` on a free port, skipping the test if hamlib is absent.
func startDummy(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("rigctld")
	if err != nil {
		t.Skip("rigctld not installed")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	cmd := exec.Command(bin, "-m", "1", "-t", strconv.Itoa(port), "-T", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for i := 0; i < 50; i++ {
		if rigctldAnswers(addr) {
			return addr
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("dummy rigctld did not start")
	return ""
}

func TestDummyRoundTrip(t *testing.T) {
	c := NewClient(startDummy(t))
	defer c.Close()
	ctx := context.Background()

	if err := c.SetFreq(ctx, 14074000); err != nil {
		t.Fatal(err)
	}
	if f, err := c.GetFreq(ctx); err != nil || f != 14074000 {
		t.Fatalf("freq = %d, %v", f, err)
	}
	if err := c.SetMode(ctx, "USB", 2400); err != nil {
		t.Fatal(err)
	}
	if m, pb, err := c.GetMode(ctx); err != nil || m != "USB" || pb != 2400 {
		t.Fatalf("mode = %s %d %v", m, pb, err)
	}
	if err := c.SetLevel(ctx, "RFPOWER", 0.5); err != nil {
		t.Fatal(err)
	}
	if v, err := c.GetLevel(ctx, "RFPOWER"); err != nil || v < 0.49 || v > 0.51 {
		t.Fatalf("rfpower = %v, %v", v, err)
	}
	if err := c.SetFunc(ctx, "NB", true); err != nil {
		t.Fatal(err)
	}
	if on, err := c.GetFunc(ctx, "NB"); err != nil || !on {
		t.Fatalf("NB = %v, %v", on, err)
	}
	if err := c.SetPTT(ctx, true); err != nil {
		t.Fatal(err)
	}
	if on, _ := c.GetPTT(ctx); !on {
		t.Fatal("PTT should be on")
	}
	_ = c.SetPTT(ctx, false)
}

func TestDummyCaps(t *testing.T) {
	c := NewClient(startDummy(t))
	defer c.Close()
	caps, err := c.FetchCaps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Model != "Dummy" || caps.Mfg != "Hamlib" {
		t.Fatalf("model = %q / %q", caps.Model, caps.Mfg)
	}
	if !caps.HasLevel("RFPOWER") || !caps.HasFunc("NB") {
		t.Fatal("missing expected level/func")
	}
	if len(caps.Modes) == 0 || len(caps.VFOs) == 0 {
		t.Fatal("modes/vfos empty")
	}
	var rf *LevelCap
	for i := range caps.Levels {
		if caps.Levels[i].Name == "RFPOWER" {
			rf = &caps.Levels[i]
		}
	}
	if rf == nil || rf.Max != 1 || !rf.Set || !rf.Get {
		t.Fatalf("RFPOWER cap = %+v", rf)
	}
	if caps.MaxRIT != 9990 {
		t.Fatalf("MaxRIT = %d", caps.MaxRIT)
	}
}

func TestErrorsAndInjection(t *testing.T) {
	c := NewClient(startDummy(t))
	defer c.Close()
	ctx := context.Background()
	if err := c.SetMode(ctx, "USB\nset_ptt 1", 0); err == nil {
		t.Fatal("expected injection to be rejected")
	}
	if _, err := c.Do(ctx, "\\set_freq 1\n\\set_ptt 1"); err == nil {
		t.Fatal("expected newline rejection")
	}
	if _, err := c.GetLevel(ctx, "BOGUS"); err == nil || !IsUnsupported(err) {
		t.Fatalf("expected unsupported error, got %v", err)
	}
	// connection must still be usable after an RPRT error
	if _, err := c.GetFreq(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestReconnect(t *testing.T) {
	c := NewClient(startDummy(t))
	defer c.Close()
	ctx := context.Background()
	if _, err := c.GetFreq(ctx); err != nil {
		t.Fatal(err)
	}
	_ = c.Close() // simulate dropped connection
	if _, err := c.GetFreq(ctx); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
}

func TestParseTokens(t *testing.T) {
	got := splitTokens("A(0..1/0.1) B(x,y) C(0..0/0)")
	if len(got) != 3 || got[1] != "B(x,y)" {
		t.Fatalf("splitTokens = %v", got)
	}
}

func TestIsRead(t *testing.T) {
	for cmd, want := range map[string]bool{
		`\get_freq`: true, `\get_level AF`: true, `\dump_caps`: true,
		`\set_freq 7100000`: false, `\vfo_op TUNE`: false, `\set_ptt 1`: false,
	} {
		if isRead(cmd) != want {
			t.Errorf("isRead(%q) = %v", cmd, !want)
		}
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

// flakyServer is a stand-in rigctld that closes the first `drop` connections as soon as they
// are accepted (the Hamlib race measured against the real thing) and then answers every
// command line. It records connections and the commands it actually received.
type flakyServer struct {
	addr  string
	conns atomic.Int32
	mu    sync.Mutex
	cmds  []string
}

func newFlakyServer(t *testing.T, drop int32) *flakyServer {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	f := &flakyServer{addr: l.Addr().String()}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			n := f.conns.Add(1)
			if n <= drop {
				_ = c.Close()
				continue
			}
			go func() {
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					f.mu.Lock()
					f.cmds = append(f.cmds, strings.TrimSpace(line))
					f.mu.Unlock()
					_, _ = c.Write([]byte("cmd:\nFrequency: 7100000\nRPRT 0\n"))
				}
			}()
		}
	}()
	return f
}

func (f *flakyServer) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cmds...)
}

func TestDeadOnArrivalConnectionsAreRetried(t *testing.T) {
	ctx := context.Background()

	f := newFlakyServer(t, 2)
	c := NewClient(f.addr)
	defer c.Close()
	if _, err := c.GetFreq(ctx); err != nil {
		t.Fatalf("read after two dropped connections: %v", err)
	}

	f = newFlakyServer(t, 2)
	c = NewClient(f.addr)
	defer c.Close()
	if err := c.SetFreq(ctx, 14074000); err != nil {
		t.Fatalf("write after two dropped connections: %v", err)
	}
	if got := f.received(); len(got) != 1 || got[0] != `+\set_freq 14074000` {
		t.Fatalf("server should have run the write exactly once, got %v", got)
	}
}

func TestActionCommandsAreNotRetried(t *testing.T) {
	f := newFlakyServer(t, 1)
	c := NewClient(f.addr)
	defer c.Close()
	if err := c.VFOOp(context.Background(), "TUNE"); err == nil {
		t.Fatal("a dropped TUNE must be reported, not silently resent")
	}
	if n := f.conns.Load(); n != 1 {
		t.Fatalf("TUNE was retried: %d connections", n)
	}
	if got := f.received(); len(got) != 0 {
		t.Fatalf("server ran %v", got)
	}
}

func TestRetriesAreBounded(t *testing.T) {
	f := newFlakyServer(t, 1000)
	c := NewClient(f.addr)
	defer c.Close()
	if _, err := c.GetFreq(context.Background()); err == nil {
		t.Fatal("expected an error when every connection is dropped")
	}
	if n := f.conns.Load(); n > 5 {
		t.Fatalf("%d connection attempts; retries should be bounded", n)
	}
}

func TestSafeToRepeat(t *testing.T) {
	for cmd, want := range map[string]bool{
		`\set_freq 7100000`: true, `\get_freq`: true, `\set_ptt 0`: true,
		`\vfo_op TUNE`: false, `\send_morse CQ`: false, `\set_powerstat 0`: false,
	} {
		if safeToRepeat(cmd) != want {
			t.Errorf("safeToRepeat(%q) = %v", cmd, !want)
		}
	}
}

// rigctld puts send_cmd's reply and RPRT on one line, separated by a NUL.
func TestSendCATReplyFormat(t *testing.T) {
	for _, c := range []struct {
		in, want string
		code     int
	}{
		{"send_cmd: EX010413;\nReply: EX010413050;\x00RPRT 0\n", "EX010413050;", 0},
		{"send_cmd_rx: EX010413060; 0\nReply: \x00RPRT 0\n", "", 0},
		{"send_cmd: EX010413060;\nReply: RPRT 0\n", "", 0},
		{"send_cmd: EX010413;\nReply: ?;\x00RPRT 0\n", "?;", 0},
		{"send_cmd: EX010413;\nReply: \x00RPRT -5\n", "", -5},
	} {
		rep, _, err := readReply(bufio.NewReader(strings.NewReader(c.in)), "x")
		if c.code != 0 {
			var re *Error
			if !errors.As(err, &re) || re.Code != c.code {
				t.Errorf("%q: err %v", c.in, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got := rep.Fields["Reply"]; got != c.want {
			t.Errorf("%q: reply %q, want %q", c.in, got, c.want)
		}
	}
	if _, _, err := readReply(bufio.NewReader(strings.NewReader("send_cmd: X;\nReply: A;\x00junk\n")), "x"); err == nil {
		t.Error("junk after the NUL was accepted")
	}
}

func TestSendCATAgainstDummy(t *testing.T) {
	c := NewClient(startDummy(t))
	ctx := context.Background()
	// Hamlib's dummy radio echoes raw commands.
	if got, err := c.SendCAT(ctx, "EX010413;", true); err != nil || got != "EX010413;" {
		t.Fatalf("read: %q %v", got, err)
	}
	if _, err := c.SendCAT(ctx, "EX010413050;", false); err != nil {
		t.Fatalf("set: %v", err)
	}
	if f, err := c.GetFreq(ctx); err != nil || f == 0 {
		t.Fatalf("connection unusable after send_cmd: %d %v", f, err)
	}
	for _, bad := range []string{"EX01 0413;", "EX010413", "EX0104;\\get_freq", "", "EX0104;\nT"} {
		if _, err := c.SendCAT(ctx, bad, true); err == nil {
			t.Errorf("%q was sent", bad)
		}
	}
}
