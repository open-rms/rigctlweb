package audio

import (
	"regexp"
	"strings"
)

// parsePactlSources reads `pactl list short sources` (tab separated: index, name, driver,
// sample spec, state). Monitor sources are what plays out of the speakers, not inputs.
func parsePactlSources(out string) []Device {
	var devs []Device
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimSpace(l), "\t")
		if len(f) < 2 || strings.HasSuffix(f[1], ".monitor") {
			continue
		}
		devs = append(devs, Device{ID: f[1], Name: friendly(f[1])})
	}
	return devs
}

// ffmpegSrcRe matches "name [description]" with the optional trailing media type
// ffmpeg 7 appends, e.g. "(none)".
var ffmpegSrcRe = regexp.MustCompile(`^(\S+)\s+\[(.*)\](?:\s+\([^)]*\))?$`)

// parseFFmpegSources reads `ffmpeg -sources pulse` (seen on ffmpeg 7.1):
//
//	Auto-detected sources for pulse:
//	* alsa_input.usb-...analog-stereo [USB Audio CODEC Analog Stereo] (none)
//	  alsa_output....monitor [Monitor of ...] (none)
func parseFFmpegSources(out string) []Device {
	var devs []Device
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "*"))
		if l == "" || strings.HasPrefix(l, "Auto-detected") {
			continue
		}
		id, desc := l, ""
		if m := ffmpegSrcRe.FindStringSubmatch(l); m != nil {
			id, desc = m[1], m[2]
		}
		if strings.ContainsAny(id, " \t") || strings.HasSuffix(id, ".monitor") {
			continue
		}
		if desc == "" {
			desc = friendly(id)
		}
		devs = append(devs, Device{ID: id, Name: desc})
	}
	return devs
}

var arecordRe = regexp.MustCompile(`^card (\d+): (\S+) \[(.*?)\], device (\d+): (.*?) \[(.*?)\]`)

// parseArecord reads `arecord -l` into hw:CARD,DEV ids.
func parseArecord(out string) []Device {
	var devs []Device
	for _, l := range strings.Split(out, "\n") {
		m := arecordRe.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		devs = append(devs, Device{ID: "hw:" + m[1] + "," + m[4], Name: strings.Join(strings.Fields(m[3]), " ") + " (" + m[6] + ")"})
	}
	return devs
}

var avfRe = regexp.MustCompile(`\[(\d+)\]\s+(.+)$`)

// parseAVFoundation reads the audio section of ffmpeg's avfoundation device list.
func parseAVFoundation(out string) []Device {
	var devs []Device
	audio := false
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(l, "AVFoundation audio devices"):
			audio = true
		case strings.Contains(l, "AVFoundation video devices"):
			audio = false
		case audio:
			if m := avfRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
				devs = append(devs, Device{ID: m[1], Name: strings.TrimSpace(m[2])})
			}
		}
	}
	return devs
}

var dshowRe = regexp.MustCompile(`"([^"]+)"`)

// parseDShow reads ffmpeg's DirectShow device list. Newer ffmpeg tags each line
// "(audio)"; older versions group them under "DirectShow audio devices". Alternative
// names (@device_cm_...) are skipped.
func parseDShow(out string) []Device {
	var devs []Device
	section := ""
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(l, "DirectShow audio devices"):
			section = "audio"
			continue
		case strings.Contains(l, "DirectShow video devices"):
			section = "video"
			continue
		}
		if strings.Contains(l, "Alternative name") {
			continue
		}
		m := dshowRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		if strings.Contains(l, "(audio)") || (section == "audio" && !strings.Contains(l, "(video)")) {
			devs = append(devs, Device{ID: m[1], Name: m[1]})
		}
	}
	return devs
}

// friendly turns a PulseAudio/PipeWire node name into something readable.
func friendly(id string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(id, "alsa_input."), "alsa_output.")
	s = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
