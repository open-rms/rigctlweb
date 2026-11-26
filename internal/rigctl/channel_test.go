package rigctl

import (
	"os"
	"strings"
	"testing"
)

func readLines(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// Captured from a real FT-DX10 (Hamlib 4.7) with get_channel N 1.
func TestParseChannelFTDX10(t *testing.T) {
	ch, err := ParseChannel(readLines(t, "ftdx10-channel1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := Channel{Number: 1, FreqHz: 7000000, Mode: "LSB"}
	if ch != want {
		t.Fatalf("got %+v, want %+v", ch, want)
	}
	ch, err = ParseChannel(readLines(t, "ftdx10-channel2-empty.txt"))
	if err != nil || ch.Number != 2 || !ch.Empty || ch.Mode != "" {
		t.Fatalf("empty channel: %+v %v", ch, err)
	}
}

func TestParseChannelRepeater(t *testing.T) {
	lines := []string{
		"Channel: 12, Name: 'W7AAA rptr'",
		"VFO: MEM, Antenna: 0, Split: OFF",
		"Freq:   146.940000 MHz\tMode:   FM\tWidth:   15.000 kHz",
		"txFreq: 0.0 Hz\ttxMode: \ttxWidth: 0.0 Hz",
		"Shift: -, Offset: +600.0 kHz, Step: 5.0 kHz, RIT: 0.0 Hz, XIT: 0.0 Hz",
		"CTCSS: 100.0Hz, CTCSSsql: 88.5Hz, DCS: 0.0, DCSsql: 0.0",
		"Functions: TONE",
		"Levels: AF: 0%,\t RF: 0%",
	}
	ch, err := ParseChannel(lines)
	if err != nil {
		t.Fatal(err)
	}
	want := Channel{Number: 12, Name: "W7AAA rptr", FreqHz: 146940000, Mode: "FM", WidthHz: 15000,
		RptrShift: "-", RptrOffsHz: 600000, CTCSS: 1000, CTCSSSql: 885}
	if ch != want {
		t.Fatalf("got %+v\nwant %+v", ch, want)
	}
}

func TestParseChannelSplit(t *testing.T) {
	lines := []string{
		"Channel: 3, Name: ''",
		"VFO: MEM, Antenna: 0, Split: ON",
		"Freq:   14.195000 MHz\tMode:   USB\tWidth:   2.4000 kHz",
		"txFreq: 14.225000 MHz\ttxMode: USB\ttxWidth: 2.4000 kHz",
		"Shift: None, Offset: 0.0 Hz, Step: 0.0 Hz, RIT: 0.0 Hz, XIT: 0.0 Hz",
		"CTCSS: 0.0Hz, CTCSSsql: 0.0Hz, DCS: 0.0, DCSsql: 0.0",
	}
	ch, err := ParseChannel(lines)
	if err != nil || !ch.Split || ch.TxFreqHz != 14225000 || ch.WidthHz != 2400 || ch.RptrShift != "" {
		t.Fatalf("split: %+v %v", ch, err)
	}
}

func TestParseChannelRejectsOtherReplies(t *testing.T) {
	for _, lines := range [][]string{nil, {"Frequency: 7074000"}, {"Channel: 1, Name: ''", "Freq: lots"}} {
		if _, err := ParseChannel(lines); err == nil {
			t.Errorf("ParseChannel(%q) should fail", lines)
		}
	}
}
