package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Input level control: read and set the capture volume of the sound input. This changes the
// computer's own input volume, which every program using that input shares, so callers
// must make it an explicit user action and offer a way back. Volume is a fraction of 100%.

// ErrUnsupported means the input volume can't be controlled on this system.
type ErrUnsupported struct{ Reason string }

func (e ErrUnsupported) Error() string { return e.Reason }

// Limits for volumes set from the app: never boost above 100%, never mute by accident.
const (
	MinInputVolume = 0.05
	MaxInputVolume = 1.0
)

// Mixer reads and sets input volume using the platform's command line tools.
type Mixer struct {
	run runner
	has func(string) bool
}

// NewMixer returns a Mixer that runs real programs.
func NewMixer() *Mixer {
	return &Mixer{run: execRunner, has: func(n string) bool { _, err := exec.LookPath(n); return err == nil }}
}

// Get returns the current input volume (1.0 = 100%) for device under method.
func (m *Mixer) Get(ctx context.Context, method, device string) (float64, error) {
	c, err := m.control(ctx, method, device)
	if err != nil {
		return 0, err
	}
	out, err := m.run(ctx, c.get[0], c.get[1:]...)
	if err != nil {
		return 0, fmt.Errorf("could not read the input volume: %w", err)
	}
	return c.parse(out)
}

// Set sets the input volume (clamped to MinInputVolume..MaxInputVolume).
func (m *Mixer) Set(ctx context.Context, method, device string, v float64) error {
	if v < MinInputVolume {
		v = MinInputVolume
	}
	if v > MaxInputVolume {
		v = MaxInputVolume
	}
	c, err := m.control(ctx, method, device)
	if err != nil {
		return err
	}
	args := c.set(v)
	if out, err := m.run(ctx, args[0], args[1:]...); err != nil {
		return fmt.Errorf("could not set the input volume: %s", strings.TrimSpace(out))
	}
	return nil
}

type control struct {
	get   []string
	parse func(string) (float64, error)
	set   func(float64) []string
}

var (
	wpctlVolRe = regexp.MustCompile(`Volume:\s+([0-9.]+)`)
	pctRe      = regexp.MustCompile(`(\d+)%`)
	cardRe     = regexp.MustCompile(`^hw:(\d+)`)
)

func (m *Mixer) control(ctx context.Context, method, device string) (*control, error) {
	switch method {
	case MethodPipeWire:
		if m.has("wpctl") && m.has("pw-dump") {
			id, err := m.pipewireNodeID(ctx, device)
			if err != nil {
				return nil, err
			}
			return &control{
				get: []string{"wpctl", "get-volume", id},
				parse: func(out string) (float64, error) {
					mm := wpctlVolRe.FindStringSubmatch(out)
					if mm == nil {
						return 0, fmt.Errorf("unexpected wpctl output %q", strings.TrimSpace(out))
					}
					return strconv.ParseFloat(mm[1], 64)
				},
				set: func(v float64) []string {
					return []string{"wpctl", "set-volume", id, strconv.FormatFloat(v, 'f', 2, 64)}
				},
			}, nil
		}
		fallthrough
	case MethodPulse:
		if !m.has("pactl") {
			return nil, ErrUnsupported{"Changing the input volume needs wpctl (PipeWire) or pactl (PulseAudio)."}
		}
		return &control{
			get: []string{"pactl", "get-source-volume", device},
			parse: func(out string) (float64, error) {
				mm := pctRe.FindStringSubmatch(out)
				if mm == nil {
					return 0, fmt.Errorf("unexpected pactl output %q", strings.TrimSpace(out))
				}
				p, _ := strconv.Atoi(mm[1])
				return float64(p) / 100, nil
			},
			set: func(v float64) []string {
				return []string{"pactl", "set-source-volume", device, strconv.Itoa(int(v*100+0.5)) + "%"}
			},
		}, nil
	case MethodALSA:
		cm := cardRe.FindStringSubmatch(device)
		if cm == nil || !m.has("amixer") {
			return nil, ErrUnsupported{"Changing the input volume needs amixer."}
		}
		name, err := m.alsaCaptureControl(ctx, cm[1])
		if err != nil {
			return nil, err
		}
		return &control{
			get: []string{"amixer", "-c", cm[1], "sget", name},
			parse: func(out string) (float64, error) {
				mm := pctRe.FindStringSubmatch(out)
				if mm == nil {
					return 0, fmt.Errorf("unexpected amixer output")
				}
				p, _ := strconv.Atoi(mm[1])
				return float64(p) / 100, nil
			},
			set: func(v float64) []string {
				return []string{"amixer", "-c", cm[1], "sset", name, strconv.Itoa(int(v*100+0.5)) + "%"}
			},
		}, nil
	}
	return nil, ErrUnsupported{"Changing the input volume isn't supported on this system. Use the sound settings of your operating system."}
}

// pipewireNodeID finds the PipeWire node ID for a capture node name via pw-dump.
func (m *Mixer) pipewireNodeID(ctx context.Context, name string) (string, error) {
	out, err := m.run(ctx, "pw-dump")
	if err != nil {
		return "", fmt.Errorf("pw-dump failed: %w", err)
	}
	var objs []struct {
		ID   int    `json:"id"`
		Type string `json:"type"`
		Info struct {
			Props map[string]any `json:"props"`
		} `json:"info"`
	}
	if err := json.Unmarshal([]byte(out), &objs); err != nil {
		return "", errors.New("could not read PipeWire's device list")
	}
	for _, o := range objs {
		if o.Type == "PipeWire:Interface:Node" && o.Info.Props["node.name"] == name {
			return strconv.Itoa(o.ID), nil
		}
	}
	return "", errors.New("the sound input was not found in PipeWire")
}

var scontrolRe = regexp.MustCompile(`Simple mixer control '([^']+)'`)

// alsaCaptureControl returns the first mixer control on card that has a capture volume.
func (m *Mixer) alsaCaptureControl(ctx context.Context, card string) (string, error) {
	out, err := m.run(ctx, "amixer", "-c", card, "scontrols")
	if err != nil {
		return "", ErrUnsupported{"This sound card has no mixer controls."}
	}
	for _, mm := range scontrolRe.FindAllStringSubmatch(out, -1) {
		info, err := m.run(ctx, "amixer", "-c", card, "sget", mm[1])
		if err == nil && strings.Contains(info, "cvolume") {
			return mm[1], nil
		}
	}
	return "", ErrUnsupported{"This sound card has no input volume control; lower the level at the radio instead."}
}
