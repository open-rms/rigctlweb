package audio

import (
	"encoding/binary"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestMain doubles as a fake capture program: re-run the test binary with
// AUDIO_FAKE=<mode> and it writes stereo frames (left = 1000, right = -2000) to stdout.
func TestMain(m *testing.M) {
	switch os.Getenv("AUDIO_FAKE") {
	case "tone":
		frame := make([]byte, FrameSamples*Channels*2)
		for i := 0; i < FrameSamples; i++ {
			binary.LittleEndian.PutUint16(frame[i*4:], uint16(int16(1000)))
			neg := int16(-2000)
			binary.LittleEndian.PutUint16(frame[i*4+2:], uint16(neg))
		}
		for {
			if _, err := os.Stdout.Write(frame); err != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	case "fail":
		os.Stderr.WriteString("device is busy")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func fakeCmd(mode string) []string {
	exe, _ := os.Executable()
	// env is inherited, so set it for the child through a wrapper.
	return []string{"env", "AUDIO_FAKE=" + mode, exe}
}

func first(t *testing.T, s *Sub) []byte {
	t.Helper()
	select {
	case f, ok := <-s.C:
		if !ok {
			t.Fatalf("channel closed: %q", s.Err())
		}
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no audio")
	}
	return nil
}

func sample(f []byte) int16 { return int16(binary.LittleEndian.Uint16(f)) }

func TestChannelsAndSharedCapture(t *testing.T) {
	if _, err := exec.LookPath("env"); err != nil {
		t.Skip("no env")
	}
	h := NewHub()
	l, err := h.Subscribe("r", fakeCmd("tone"), "left")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := h.Subscribe("r", fakeCmd("tone"), "right")
	m, _ := h.Subscribe("r", fakeCmd("tone"), "mix")
	if got := h.Listeners("r"); got != 3 {
		t.Fatalf("listeners %d", got)
	}
	if v := sample(first(t, l)); v != 1000 {
		t.Errorf("left %d", v)
	}
	if v := sample(first(t, r)); v != -2000 {
		t.Errorf("right %d", v)
	}
	if v := sample(first(t, m)); v != -500 {
		t.Errorf("mix %d", v)
	}
	if n := len(first(t, l)); n != FrameSamples*2 {
		t.Errorf("frame bytes %d", n)
	}
	l.Close()
	r.Close()
	m.Close()
	time.Sleep(100 * time.Millisecond)
	if got := h.Listeners("r"); got != 0 {
		t.Errorf("capture should stop with no listeners, have %d", got)
	}
}

func TestFailureReasonReachesListener(t *testing.T) {
	h := NewHub()
	s, err := h.Subscribe("r", fakeCmd("fail"), "mix")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-s.C:
		if ok {
			t.Fatal("expected close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if got := s.Err(); got != "device is busy" {
		t.Errorf("reason %q", got)
	}
}

func TestStopTellsListenersWhy(t *testing.T) {
	h := NewHub()
	s, _ := h.Subscribe("r", fakeCmd("tone"), "mix")
	first(t, s)
	h.Stop("r")
	for range s.C {
	}
	if got := s.Err(); got != "audio settings changed" {
		t.Errorf("reason %q", got)
	}
}

func TestListenerLimitAndMissingProgram(t *testing.T) {
	h := NewHub()
	var subs []*Sub
	for i := 0; i < MaxListeners; i++ {
		s, err := h.Subscribe("r", fakeCmd("tone"), "mix")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, s)
	}
	if _, err := h.Subscribe("r", fakeCmd("tone"), "mix"); err == nil {
		t.Error("expected listener limit")
	}
	for _, s := range subs {
		s.Close()
	}
	if _, err := h.Subscribe("x", []string{"/nonexistent/capture-tool"}, "mix"); err == nil {
		t.Error("expected error for missing program")
	}
}

func TestToMono(t *testing.T) {
	st := []byte{0x00, 0x7f, 0x00, 0x7f} // both channels 32512
	if v := sample(toMono(st, "mix")); v != 32512 {
		t.Errorf("mix of equal channels must not overflow: %d", v)
	}
}
