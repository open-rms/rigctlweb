package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Channels is how many channels the capture commands record. Stereo is captured so the
// user can pick left, right or a mix; some interfaces put the radio on one side only.
const Channels = 2

// FrameSamples is the number of mono samples per frame sent to the browser (40 ms).
const FrameSamples = 480

// MaxListeners caps simultaneous browsers per rig.
const MaxListeners = 4

// Hub runs at most one capture process per key (rig ID) and fans its audio out to every
// listener. The process only runs while someone is listening, so the sound card is not
// held open and nothing is captured when no one is using the monitor.
type Hub struct {
	mu   sync.Mutex
	sess map[string]*session
}

func NewHub() *Hub { return &Hub{sess: map[string]*session{}} }

type session struct {
	cancel context.CancelFunc
	subs   map[*Sub]struct{}
	// stopReason is shown to listeners when capture is stopped on purpose (settings changed).
	stopReason string
}

// Sub is one listener. C yields mono signed 16-bit little-endian PCM frames at SampleRate
// and is closed when capture ends; Err then says why (empty if the listener left).
type Sub struct {
	C       <-chan []byte
	ch      chan []byte
	channel string
	hub     *Hub
	key     string
	sess    *session
	err     string
}

// Err returns the reason capture ended. Call after C is closed.
func (s *Sub) Err() string {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return s.err
}

// Subscribe joins the capture for key, starting command if it isn't running. channel is
// "left", "right" or anything else for a mix.
func (h *Hub) Subscribe(key string, command []string, channel string) (*Sub, error) {
	if len(command) == 0 {
		return nil, errors.New("no capture command")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sess[key]
	if s != nil && len(s.subs) >= MaxListeners {
		return nil, errors.New("too many listeners")
	}
	if s == nil {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		cmd.Stdin = nil
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			return nil, err
		}
		var tail tailBuf
		cmd.Stderr = &tail
		if err := cmd.Start(); err != nil {
			cancel()
			return nil, err
		}
		s = &session{cancel: cancel, subs: map[*Sub]struct{}{}}
		h.sess[key] = s
		go h.pump(key, s, ctx, cmd, stdout, &tail)
	}
	ch := make(chan []byte, 16)
	sub := &Sub{C: ch, ch: ch, channel: channel, hub: h, key: key, sess: s}
	s.subs[sub] = struct{}{}
	return sub, nil
}

// Close removes the listener and stops capture when it was the last one.
func (s *Sub) Close() {
	h := s.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := s.sess.subs[s]; !ok {
		return
	}
	delete(s.sess.subs, s)
	close(s.ch)
	if len(s.sess.subs) == 0 {
		s.sess.cancel()
		if h.sess[s.key] == s.sess {
			delete(h.sess, s.key)
		}
	}
}

// Stop ends capture for key and closes all its listeners, for when its settings change.
func (h *Hub) Stop(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s := h.sess[key]; s != nil {
		s.stopReason = "audio settings changed"
		s.cancel()
	}
}

// Listeners reports how many browsers are listening to key.
func (h *Hub) Listeners(key string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s := h.sess[key]; s != nil {
		return len(s.subs)
	}
	return 0
}

func (h *Hub) pump(key string, s *session, ctx context.Context, cmd *exec.Cmd, out io.Reader, tail *tailBuf) {
	buf := make([]byte, FrameSamples*Channels*2)
	for {
		if _, err := io.ReadFull(out, buf); err != nil {
			break
		}
		h.mu.Lock()
		for sub := range s.subs {
			frame := toMono(buf, sub.channel)
			select {
			case sub.ch <- frame:
			default: // slow listener: drop its oldest frame rather than stall the capture
				select {
				case <-sub.ch:
				default:
				}
				select {
				case sub.ch <- frame:
				default:
				}
			}
		}
		h.mu.Unlock()
	}
	stoppedOnPurpose := ctx.Err() != nil // checked before our own cancel below
	s.cancel()
	_ = cmd.Wait()

	h.mu.Lock()
	reason := ""
	if stoppedOnPurpose {
		reason = s.stopReason
	} else if reason = strings.TrimSpace(tail.String()); reason == "" {
		reason = "the capture program stopped"
	}
	for sub := range s.subs {
		sub.err = reason
		close(sub.ch)
	}
	s.subs = map[*Sub]struct{}{}
	if h.sess[key] == s {
		delete(h.sess, key)
	}
	h.mu.Unlock()
}

// toMono converts one interleaved stereo s16le frame to mono: the left channel, the right
// channel, or their average.
func toMono(stereo []byte, channel string) []byte {
	n := len(stereo) / 4
	out := make([]byte, n*2)
	for i := 0; i < n; i++ {
		l := int16(binary.LittleEndian.Uint16(stereo[i*4:]))
		r := int16(binary.LittleEndian.Uint16(stereo[i*4+2:]))
		var v int16
		switch channel {
		case "left":
			v = l
		case "right":
			v = r
		default:
			v = int16((int32(l) + int32(r)) / 2)
		}
		binary.LittleEndian.PutUint16(out[i*2:], uint16(v))
	}
	return out
}

// tailBuf keeps the last few hundred bytes written to it (the capture tool's error text).
type tailBuf struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuf) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > 400 {
		t.b = t.b[len(t.b)-400:]
	}
	return len(p), nil
}

func (t *tailBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
