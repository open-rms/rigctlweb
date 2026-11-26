package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/open-rms/rigctlweb/internal/audio"
	"github.com/open-rms/rigctlweb/internal/config"
)

// Receive-audio monitor. Off unless the user enables it for a rig; nothing is captured
// until a browser asks to listen, and capture stops when the last listener leaves.

// inputMixer reads and sets the sound input's volume (satisfied by *audio.Mixer).
type inputMixer interface {
	Get(ctx context.Context, method, device string) (float64, error)
	Set(ctx context.Context, method, device string, v float64) error
}

type audioState struct {
	hub    *audio.Hub
	mixer  inputMixer
	detect func(context.Context) audio.Report // audio.Detect; replaced in tests

	mu  sync.Mutex
	rep audio.Report
	at  time.Time
}

// report returns the detection result, cached briefly because detection runs programs.
func (s *Server) audioReport(fresh bool) audio.Report {
	a := s.audio
	a.mu.Lock()
	defer a.mu.Unlock()
	if fresh || a.at.IsZero() || time.Since(a.at) > 20*time.Second {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		a.rep, a.at = a.detect(ctx), time.Now()
	}
	return a.rep
}

type audioView struct {
	Config    config.AudioConfig `json:"config"`
	Available audio.Report       `json:"available"`
	Listeners int                `json:"listeners"`
}

func audioDefaults(c config.AudioConfig) config.AudioConfig {
	if c.Channel != "left" && c.Channel != "right" {
		c.Channel = "mix"
	}
	return c
}

func (s *Server) handleGetAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.mgr.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	rep := s.audioReport(false)
	rep.Command = nil // the UI doesn't need the command line
	writeJSON(w, 200, audioView{
		Config:    audioDefaults(s.cfg.Get().Audio[id]),
		Available: rep,
		Listeners: s.audio.hub.Listeners(id),
	})
}

func (s *Server) handlePutAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.mgr.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	var in config.AudioConfig
	if err := decode(w, r, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if in.Channel != "left" && in.Channel != "right" && in.Channel != "mix" {
		http.Error(w, "invalid channel", http.StatusBadRequest)
		return
	}
	rep := s.audioReport(true)
	if in.Device != "" && !knownDevice(rep, in.Device) {
		http.Error(w, "unknown audio device", http.StatusBadRequest)
		return
	}
	if in.Enabled && rep.Method == audio.MethodNone {
		http.Error(w, "audio capture is not available on this machine", http.StatusBadRequest)
		return
	}
	err := s.cfg.Update(func(c *config.Config) error {
		old := c.Audio[id]
		in.RestoreDevice, in.RestoreVolume = old.RestoreDevice, old.RestoreVolume // not editable here
		c.Audio[id] = in
		return nil
	})
	if err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	s.audio.hub.Stop(id) // listeners reconnect with the new settings
	s.handleGetAudio(w, r)
}

func knownDevice(rep audio.Report, id string) bool {
	for _, d := range rep.Devices {
		if d.ID == id {
			return true
		}
	}
	return false
}

// handleAudioWS streams mono s16le PCM as binary frames after a JSON header. Errors are
// sent as a final JSON text message so the page can explain them.
func (s *Server) handleAudioWS(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("rig")
	if _, ok := s.mgr.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	cfg := audioDefaults(s.cfg.Get().Audio[id])
	if !cfg.Enabled {
		http.Error(w, "audio is off for this rig", http.StatusForbidden)
		return
	}
	rep := s.audioReport(false)
	device := cfg.Device
	if device == "" && len(rep.Devices) > 0 {
		device = rep.Devices[0].ID
	}
	// The device comes from the config file, so re-check it before it reaches a command line.
	if device == "" || !knownDevice(rep, device) {
		http.Error(w, "audio device not found", http.StatusConflict)
		return
	}
	has := func(n string) bool { _, err := exec.LookPath(n); return err == nil }
	cmd := audio.CaptureCommand(rep.Method, device, has)

	conn, err := websocket.Accept(w, r, nil) // same-origin only
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context())

	say := func(v any) error {
		b, _ := json.Marshal(v)
		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return conn.Write(wctx, websocket.MessageText, b)
	}
	sub, err := s.audio.hub.Subscribe(id, cmd, cfg.Channel)
	if err != nil {
		s.log.Warn("audio capture", "rig", id, "err", err)
		_ = say(map[string]string{"error": "Could not start audio capture: " + err.Error()})
		conn.Close(websocket.StatusInternalError, "capture failed")
		return
	}
	defer sub.Close()
	if err := say(map[string]any{"type": "audio", "rate": audio.SampleRate, "channels": 1, "format": "s16le", "channel": cfg.Channel}); err != nil {
		return
	}
	s.log.Info("audio listener joined", "rig", id, "device", device, "method", rep.Method)
	defer s.log.Info("audio listener left", "rig", id)

	for {
		select {
		case frame, ok := <-sub.C:
			if !ok {
				msg := sub.Err()
				if msg == "" {
					msg = "audio stopped"
				}
				_ = say(map[string]string{"error": msg})
				conn.Close(websocket.StatusNormalClosure, "audio stopped")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(wctx, websocket.MessageBinary, frame)
			cancel()
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// ---- input level ----

type inputView struct {
	Supported bool     `json:"supported"`
	Reason    string   `json:"reason,omitempty"`
	Volume    float64  `json:"volume"`             // 1.0 = 100%
	Original  *float64 `json:"original,omitempty"` // what it was before the app first changed it
}

// inputDevice resolves the sound input a rig's monitor uses, the same way the stream does.
func (s *Server) inputDevice(id string) (method, device string, cfg config.AudioConfig, err error) {
	cfg = audioDefaults(s.cfg.Get().Audio[id])
	rep := s.audioReport(false)
	device = cfg.Device
	if device == "" && len(rep.Devices) > 0 {
		device = rep.Devices[0].ID
	}
	if rep.Method == audio.MethodNone || device == "" || !knownDevice(rep, device) {
		return "", "", cfg, errors.New("audio input not found")
	}
	return rep.Method, device, cfg, nil
}

func (s *Server) inputView(ctx context.Context, method, device string, cfg config.AudioConfig) (inputView, error) {
	v, err := s.audio.mixer.Get(ctx, method, device)
	if err != nil {
		var u audio.ErrUnsupported
		if errors.As(err, &u) {
			return inputView{Supported: false, Reason: u.Reason}, nil
		}
		return inputView{}, err
	}
	out := inputView{Supported: true, Volume: v}
	if cfg.RestoreVolume != nil && cfg.RestoreDevice == device {
		o := *cfg.RestoreVolume
		out.Original = &o
	}
	return out, nil
}

func (s *Server) handleGetAudioInput(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.mgr.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	method, device, cfg, err := s.inputDevice(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	v, err := s.inputView(ctx, method, device, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, 200, v)
}

// handlePutAudioInput changes the sound input's volume. This is the computer's own input
// volume, shared with every program that records from it, so the first change remembers
// the original for a one-click restore. It can only lower or restore, never boost above 100%.
func (s *Server) handlePutAudioInput(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.mgr.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Volume  *float64 `json:"volume"`
		Restore bool     `json:"restore"`
	}
	if err := decode(w, r, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !in.Restore && (in.Volume == nil || *in.Volume < audio.MinInputVolume || *in.Volume > audio.MaxInputVolume) {
		http.Error(w, "volume must be between 5% and 100%", http.StatusBadRequest)
		return
	}
	method, device, cfg, err := s.inputDevice(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	target := 0.0
	switch {
	case in.Restore:
		if cfg.RestoreVolume == nil || cfg.RestoreDevice != device {
			http.Error(w, "nothing to restore", http.StatusConflict)
			return
		}
		target = *cfg.RestoreVolume
	default:
		target = *in.Volume
		if cfg.RestoreVolume == nil || cfg.RestoreDevice != device {
			cur, err := s.audio.mixer.Get(ctx, method, device)
			if err != nil {
				var u audio.ErrUnsupported
				if errors.As(err, &u) {
					http.Error(w, u.Reason, http.StatusNotImplemented)
				} else {
					http.Error(w, err.Error(), http.StatusBadGateway)
				}
				return
			}
			// Remember it before the first change, so Restore works even after a restart.
			if err := s.cfg.Update(func(c *config.Config) error {
				a := c.Audio[id]
				a.RestoreDevice, a.RestoreVolume = device, &cur
				c.Audio[id] = a
				return nil
			}); err != nil {
				http.Error(w, "could not save the original volume", http.StatusInternalServerError)
				return
			}
		}
	}
	if err := s.audio.mixer.Set(ctx, method, device, target); err != nil {
		var u audio.ErrUnsupported
		if errors.As(err, &u) {
			http.Error(w, u.Reason, http.StatusNotImplemented)
		} else {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	if in.Restore {
		_ = s.cfg.Update(func(c *config.Config) error {
			a := c.Audio[id]
			a.RestoreDevice, a.RestoreVolume = "", nil
			c.Audio[id] = a
			return nil
		})
	}
	s.log.Info("audio input volume changed", "rig", id, "device", device, "volume", target, "restore", in.Restore)
	cfg = audioDefaults(s.cfg.Get().Audio[id])
	v, err := s.inputView(ctx, method, device, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, 200, v)
}
