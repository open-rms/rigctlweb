// Package audio finds out what sound capture is available on this machine so the
// UI can offer receive-audio monitoring. Detection is read-only: it lists tools and
// devices and never changes any sound setting. No cgo; everything goes through the
// platform's own command line tools so the binary stays a single static build.
package audio

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Capture methods, in the order they are preferred on Linux.
const (
	MethodPipeWire     = "pipewire"
	MethodPulse        = "pulse"
	MethodALSA         = "alsa"
	MethodAVFoundation = "avfoundation" // macOS, via ffmpeg
	MethodDShow        = "dshow"        // Windows, via ffmpeg
	MethodNone         = "none"
)

// SampleRate is the capture rate sent to the browser: plenty for SSB/FM audio and
// digital-mode passbands, and light enough for a LAN or a Pi (about 190 kbps).
const SampleRate = 12000

// Device is one capture source.
type Device struct {
	ID   string `json:"id"`   // what the capture command takes (node name, hw:2,0, ...)
	Name string `json:"name"` // human description
}

// Report is the result of a detection run.
type Report struct {
	OS      string            `json:"os"`
	Arch    string            `json:"arch"`
	Tools   map[string]bool   `json:"tools"`
	Method  string            `json:"method"`
	Devices []Device          `json:"devices"`
	Command []string          `json:"command,omitempty"` // capture command for the first device, as an example
	Notes   []string          `json:"notes,omitempty"`
	Server  map[string]string `json:"server,omitempty"` // which sound servers answered
}

// runner runs a command and returns its combined output (ffmpeg prints lists on stderr).
type runner func(ctx context.Context, name string, args ...string) (string, error)

func execRunner(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

var toolNames = []string{"pw-record", "pw-cli", "wpctl", "parec", "pactl", "arecord", "ffmpeg"}

// Detect inspects this machine.
func Detect(ctx context.Context) Report {
	has := func(n string) bool { _, err := exec.LookPath(n); return err == nil }
	return detect(ctx, runtime.GOOS, runtime.GOARCH, has, execRunner, func(p string) bool { _, err := os.Stat(p); return err == nil })
}

func detect(ctx context.Context, goos, arch string, has func(string) bool, run runner, exists func(string) bool) Report {
	r := Report{OS: goos, Arch: arch, Tools: map[string]bool{}, Server: map[string]string{}, Method: MethodNone}
	for _, t := range toolNames {
		r.Tools[t] = has(t)
	}

	switch goos {
	case "linux":
		detectLinux(ctx, &r, has, run, exists)
	case "darwin":
		if !has("ffmpeg") {
			r.note("macOS capture needs ffmpeg (for example: brew install ffmpeg).")
			break
		}
		out, _ := run(ctx, "ffmpeg", "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
		r.Devices = parseAVFoundation(out)
		r.Method = MethodAVFoundation
		r.note("macOS support is untested on real hardware. The first capture asks for microphone permission.")
	case "windows":
		if !has("ffmpeg") {
			r.note("Windows capture needs ffmpeg on the PATH (for example: winget install ffmpeg).")
			break
		}
		out, _ := run(ctx, "ffmpeg", "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
		r.Devices = parseDShow(out)
		r.Method = MethodDShow
		r.note("Windows support is untested on real hardware.")
	default:
		r.note("Audio capture is not supported on " + goos + " yet.")
	}

	if r.Method != MethodNone && len(r.Devices) == 0 {
		r.note("No capture devices were found.")
	}
	if len(r.Devices) > 0 {
		r.Command = CaptureCommand(r.Method, r.Devices[0].ID, has)
	}
	return r
}

func detectLinux(ctx context.Context, r *Report, has func(string) bool, run runner, exists func(string) bool) {
	pwUp, pulseUp := false, false
	if has("pw-cli") {
		if _, err := run(ctx, "pw-cli", "info", "0"); err == nil {
			pwUp = true
			r.Server["pipewire"] = "running"
		}
	}
	var pulseDevs []Device
	if has("pactl") {
		if out, err := run(ctx, "pactl", "list", "short", "sources"); err == nil {
			pulseUp = true
			pulseDevs = parsePactlSources(out)
			if info, err := run(ctx, "pactl", "info"); err == nil {
				if name := serverName(info); name != "" {
					r.Server["pulse"] = name
				}
			}
		}
	}
	if len(pulseDevs) == 0 && has("ffmpeg") && (pwUp || pulseUp || !has("pactl")) {
		if out, err := run(ctx, "ffmpeg", "-hide_banner", "-sources", "pulse"); err == nil {
			pulseDevs = parseFFmpegSources(out)
			if len(pulseDevs) > 0 {
				pulseUp = true
			}
		}
	}

	switch {
	case pwUp && has("pw-record") && len(pulseDevs) > 0:
		r.Method, r.Devices = MethodPipeWire, pulseDevs
		r.note("PipeWire shares capture devices between programs, so no sound configuration is needed.")
	case pulseUp && len(pulseDevs) > 0 && (has("parec") || has("ffmpeg")):
		r.Method, r.Devices = MethodPulse, pulseDevs
		r.note("PulseAudio shares capture devices between programs, so no sound configuration is needed.")
	case has("arecord"):
		out, _ := run(ctx, "arecord", "-l")
		devs := parseArecord(out)
		if len(devs) > 0 || exists("/proc/asound/cards") {
			r.Method, r.Devices = MethodALSA, devs
			r.note("Plain ALSA gives one program exclusive use of a card. If pat, direwolf or WSJT-X already uses it, " +
				"capture here will fail until a shared (dsnoop) device is set up.")
		}
	}
	if r.Method == MethodNone {
		r.note("No usable capture tool found. Install PipeWire (pw-record), PulseAudio (parec), ALSA utils (arecord) or ffmpeg.")
	}
}

func (r *Report) note(s string) { r.Notes = append(r.Notes, s) }

// CaptureCommand returns the command that writes raw signed 16-bit little-endian stereo
// PCM at SampleRate to stdout for the given device. Quiet, no files written.
func CaptureCommand(method, device string, has func(string) bool) []string {
	rate, ch := itoa(SampleRate), itoa(Channels)
	switch method {
	case MethodPipeWire:
		return []string{"pw-record", "--target", device, "--rate", rate, "--channels", ch, "--format", "s16", "-"}
	case MethodPulse:
		if has("parec") {
			return []string{"parec", "--device=" + device, "--format=s16le", "--rate=" + rate, "--channels=" + ch, "--latency-msec=40"}
		}
		return []string{"ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "pulse", "-i", device, "-ac", ch, "-ar", rate, "-f", "s16le", "-"}
	case MethodALSA:
		return []string{"arecord", "-q", "-D", device, "-f", "S16_LE", "-r", rate, "-c", ch, "-t", "raw"}
	case MethodAVFoundation:
		return []string{"ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "avfoundation", "-i", ":" + device, "-ac", ch, "-ar", rate, "-f", "s16le", "-"}
	case MethodDShow:
		return []string{"ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "dshow", "-i", "audio=" + device, "-ac", ch, "-ar", rate, "-f", "s16le", "-"}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func serverName(info string) string {
	for _, l := range strings.Split(info, "\n") {
		if v, ok := strings.CutPrefix(l, "Server Name:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
