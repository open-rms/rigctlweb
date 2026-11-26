package audio

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParsePactlSourcesSkipsMonitors(t *testing.T) {
	out := "56\talsa_output.pci-0000_00_1f.3.analog-stereo.monitor\tPipeWire\ts32le 2ch 48000Hz\tSUSPENDED\n" +
		"58\talsa_input.usb-BurrBrown_USB_AUDIO_CODEC-00.analog-stereo\tPipeWire\ts16le 2ch 48000Hz\tRUNNING\n"
	got := parsePactlSources(out)
	if len(got) != 1 || got[0].ID != "alsa_input.usb-BurrBrown_USB_AUDIO_CODEC-00.analog-stereo" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseFFmpegSources(t *testing.T) {
	out := "Auto-detected sources for pulse:\n" +
		"* alsa_input.usb-X_USB_AUDIO_CODEC-00.analog-stereo [USB AUDIO  CODEC Analog Stereo] (none)\n" +
		"  alsa_output.platform-hdmi.stereo-fallback.monitor [Monitor of HDMI] (none)\n"
	got := parseFFmpegSources(out)
	if len(got) != 1 || got[0].Name != "USB AUDIO  CODEC Analog Stereo" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseArecord(t *testing.T) {
	out := "**** List of CAPTURE Hardware Devices ****\n" +
		"card 2: CODEC [USB AUDIO  CODEC], device 0: USB Audio [USB Audio]\n" +
		"  Subdevices: 1/1\n"
	got := parseArecord(out)
	want := []Device{{ID: "hw:2,0", Name: "USB AUDIO CODEC (USB Audio)"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestParseAVFoundation(t *testing.T) {
	out := "[AVFoundation indev @ 0x1] AVFoundation video devices:\n" +
		"[AVFoundation indev @ 0x1] [0] FaceTime HD Camera\n" +
		"[AVFoundation indev @ 0x1] AVFoundation audio devices:\n" +
		"[AVFoundation indev @ 0x1] [0] MacBook Pro Microphone\n" +
		"[AVFoundation indev @ 0x1] [1] USB Audio CODEC\n"
	got := parseAVFoundation(out)
	if len(got) != 2 || got[1].ID != "1" || got[1].Name != "USB Audio CODEC" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseDShowBothFormats(t *testing.T) {
	newer := "[dshow @ 0x1] \"Integrated Camera\" (video)\n" +
		"[dshow @ 0x1]   Alternative name \"@device_pnp_x\"\n" +
		"[dshow @ 0x1] \"Microphone (USB Audio CODEC)\" (audio)\n" +
		"[dshow @ 0x1]   Alternative name \"@device_cm_y\"\n"
	if got := parseDShow(newer); len(got) != 1 || got[0].ID != "Microphone (USB Audio CODEC)" {
		t.Fatalf("newer: %+v", got)
	}
	older := "[dshow @ 0x1] DirectShow video devices\n[dshow @ 0x1]  \"Cam\"\n" +
		"[dshow @ 0x1] DirectShow audio devices\n[dshow @ 0x1]  \"Line In\"\n"
	if got := parseDShow(older); len(got) != 1 || got[0].ID != "Line In" {
		t.Fatalf("older: %+v", got)
	}
}

// fake builds a runner from canned outputs keyed by "name args...".
func fake(m map[string]string) runner {
	return func(_ context.Context, name string, args ...string) (string, error) {
		if out, ok := m[name+" "+strings.Join(args, " ")]; ok {
			return out, nil
		}
		return "", errors.New("not available")
	}
}

func hasSet(names ...string) func(string) bool {
	s := map[string]bool{}
	for _, n := range names {
		s[n] = true
	}
	return func(n string) bool { return s[n] }
}

func TestDetectPipeWireWithoutPactl(t *testing.T) {
	// Mirrors the Raspberry Pi: PipeWire running, pw-record + ffmpeg present, no pactl.
	run := fake(map[string]string{
		"pw-cli info 0":                      "id: 0",
		"ffmpeg -hide_banner -sources pulse": "Auto-detected sources for pulse:\n* alsa_input.usb-codec.analog-stereo [Codec] (none)\n",
	})
	r := detect(context.Background(), "linux", "arm64", hasSet("pw-cli", "pw-record", "ffmpeg", "arecord"), run, func(string) bool { return true })
	if r.Method != MethodPipeWire || len(r.Devices) != 1 {
		t.Fatalf("got %+v", r)
	}
	if r.Command[0] != "pw-record" || r.Command[2] != "alsa_input.usb-codec.analog-stereo" {
		t.Fatalf("command %v", r.Command)
	}
}

func TestDetectPlainALSAWarnsAboutSharing(t *testing.T) {
	run := fake(map[string]string{"arecord -l": "card 1: Dev [Dev], device 0: USB Audio [USB Audio]\n"})
	r := detect(context.Background(), "linux", "amd64", hasSet("arecord"), run, func(string) bool { return true })
	if r.Method != MethodALSA || len(r.Notes) == 0 || !strings.Contains(r.Notes[0], "exclusive") {
		t.Fatalf("got %+v", r)
	}
}

func TestDetectNothingInstalled(t *testing.T) {
	r := detect(context.Background(), "linux", "amd64", hasSet(), fake(nil), func(string) bool { return false })
	if r.Method != MethodNone || len(r.Notes) == 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestDetectOtherPlatforms(t *testing.T) {
	mac := detect(context.Background(), "darwin", "arm64", hasSet("ffmpeg"), fake(map[string]string{
		"ffmpeg -hide_banner -f avfoundation -list_devices true -i ": "[x] AVFoundation audio devices:\n[x] [0] Mic\n",
	}), nil)
	if mac.Method != MethodAVFoundation || len(mac.Devices) != 1 {
		t.Fatalf("mac %+v", mac)
	}
	win := detect(context.Background(), "windows", "amd64", hasSet(), fake(nil), nil)
	if win.Method != MethodNone {
		t.Fatalf("win without ffmpeg should be none: %+v", win)
	}
}

func TestCaptureCommandsAreStereo16At12k(t *testing.T) {
	for _, m := range []string{MethodPipeWire, MethodPulse, MethodALSA, MethodAVFoundation, MethodDShow} {
		cmd := strings.Join(CaptureCommand(m, "dev", hasSet("parec", "ffmpeg")), " ")
		if !strings.Contains(cmd, "12000") || !(strings.Contains(cmd, "s16") || strings.Contains(cmd, "S16")) {
			t.Errorf("%s: %s", m, cmd)
		}
	}
}
