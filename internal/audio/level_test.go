package audio

import (
	"context"
	"errors"
	"testing"
)

const pwDump = `[{"id":57,"type":"PipeWire:Interface:Node","info":{"props":{"node.name":"alsa_output.usb-x"}}},
{"id":58,"type":"PipeWire:Interface:Node","info":{"props":{"node.name":"alsa_input.usb-x"}}},
{"id":9,"type":"PipeWire:Interface:Device","info":{"props":{"node.name":"alsa_input.usb-x"}}}]`

// recorder runs canned outputs and remembers every command line.
func recorder(m map[string]string) (runner, *[]string) {
	var log []string
	f := fake(m)
	return func(ctx context.Context, name string, args ...string) (string, error) {
		log = append(log, name+" "+join(args))
		return f(ctx, name, args...)
	}, &log
}

func join(a []string) string {
	s := ""
	for i, x := range a {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}

func TestPipeWireGetSetByNodeName(t *testing.T) {
	run, log := recorder(map[string]string{
		"pw-dump ":                 pwDump,
		"wpctl get-volume 58":      "Volume: 0.80 [MUTED]\n",
		"wpctl set-volume 58 0.50": "",
	})
	m := &Mixer{run: run, has: hasSet("wpctl", "pw-dump")}
	v, err := m.Get(context.Background(), MethodPipeWire, "alsa_input.usb-x")
	if err != nil || v != 0.80 {
		t.Fatalf("get %v %v", v, err)
	}
	if err := m.Set(context.Background(), MethodPipeWire, "alsa_input.usb-x", 0.5); err != nil {
		t.Fatal(err)
	}
	// The device node (type Device) with the same name must not be picked.
	for _, c := range *log {
		if c == "wpctl set-volume 9 0.50" {
			t.Fatal("used the wrong object")
		}
	}
}

func TestSetClampsAndNeverBoosts(t *testing.T) {
	run, log := recorder(map[string]string{
		"pw-dump ":                 pwDump,
		"wpctl set-volume 58 0.05": "",
		"wpctl set-volume 58 1.00": "",
	})
	m := &Mixer{run: run, has: hasSet("wpctl", "pw-dump")}
	if err := m.Set(context.Background(), MethodPipeWire, "alsa_input.usb-x", 0); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(context.Background(), MethodPipeWire, "alsa_input.usb-x", 3); err != nil {
		t.Fatal(err)
	}
	got := (*log)[len(*log)-1]
	if got != "wpctl set-volume 58 1.00" {
		t.Fatalf("boost not capped: %s", got)
	}
}

func TestPulseVolumeParsing(t *testing.T) {
	run, log := recorder(map[string]string{
		"pactl get-source-volume src":     "Volume: front-left: 52428 /  80% / -5.81 dB,   front-right: 52428 /  80% / -5.81 dB\n        balance 0.00\n",
		"pactl set-source-volume src 35%": "",
	})
	m := &Mixer{run: run, has: hasSet("pactl")}
	v, err := m.Get(context.Background(), MethodPulse, "src")
	if err != nil || v != 0.8 {
		t.Fatalf("get %v %v", v, err)
	}
	if err := m.Set(context.Background(), MethodPulse, "src", 0.35); err != nil {
		t.Fatalf("set: %v (%v)", err, *log)
	}
}

func TestALSAFindsCaptureControlOrExplains(t *testing.T) {
	m := &Mixer{run: fake(map[string]string{
		"amixer -c 2 scontrols": "Simple mixer control 'PCM',0\n",
		"amixer -c 2 sget PCM":  "Capabilities: pvolume pswitch\n",
	}), has: hasSet("amixer")}
	_, err := m.Get(context.Background(), MethodALSA, "hw:2,0")
	var u ErrUnsupported
	if !errors.As(err, &u) {
		t.Fatalf("want unsupported, got %v", err)
	}

	m = &Mixer{run: fake(map[string]string{
		"amixer -c 1 scontrols": "Simple mixer control 'Mic',0\n",
		"amixer -c 1 sget Mic":  "Capabilities: cvolume cswitch\n  Front Left: Capture 30 [60%] [on]\n",
	}), has: hasSet("amixer")}
	if v, err := m.Get(context.Background(), MethodALSA, "hw:1,0"); err != nil || v != 0.6 {
		t.Fatalf("got %v %v", v, err)
	}
}

func TestUnsupportedPlatforms(t *testing.T) {
	m := &Mixer{run: fake(nil), has: hasSet()}
	for _, method := range []string{MethodAVFoundation, MethodDShow, MethodNone} {
		var u ErrUnsupported
		if _, err := m.Get(context.Background(), method, "x"); !errors.As(err, &u) {
			t.Errorf("%s: %v", method, err)
		}
	}
}
