package rigctl

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Channel is one memory channel as rigctld's get_channel reports it. Only the fields a
// "tune to this memory" needs are kept.
type Channel struct {
	Number     int    `json:"number"`
	Name       string `json:"name,omitempty"`
	Empty      bool   `json:"empty"` // no frequency stored
	FreqHz     int64  `json:"freqHz"`
	Mode       string `json:"mode,omitempty"`
	WidthHz    int    `json:"widthHz,omitempty"`
	Split      bool   `json:"split,omitempty"`
	TxFreqHz   int64  `json:"txFreqHz,omitempty"`
	RptrShift  string `json:"rptrShift,omitempty"` // "+", "-" or "" (none)
	RptrOffsHz int    `json:"rptrOffsHz,omitempty"`
	CTCSS      int    `json:"ctcss,omitempty"`    // tenths of Hz
	CTCSSSql   int    `json:"ctcssSql,omitempty"` // tenths of Hz
}

// GetChannel reads memory channel n without changing the radio (get_channel's read-only
// flag set). Some backends emulate it by switching to the memory and back; callers should
// not read while the radio is transmitting.
func (c *Client) GetChannel(ctx context.Context, n int) (Channel, error) {
	if n < 0 || n > 9999 {
		return Channel{}, fmt.Errorf("rigctl: channel %d out of range", n)
	}
	rep, err := c.Do(ctx, fmt.Sprintf(`\get_channel %d 1`, n))
	if err != nil {
		return Channel{}, err
	}
	ch, err := ParseChannel(rep.Lines)
	if err == nil && ch.Number != n {
		err = fmt.Errorf("rigctl: asked for channel %d, got %d", n, ch.Number)
	}
	return ch, err
}

var (
	chanHeadRe = regexp.MustCompile(`^Channel:\s*(\d+),\s*Name:\s*'(.*)'\s*$`)
	freqValRe  = regexp.MustCompile(`^([+-]?\d+(?:\.\d+)?)\s*(Hz|kHz|MHz|GHz)$`)
	toneValRe  = regexp.MustCompile(`^(\d+)(?:\.(\d))?\d*\s*Hz$`)
)

// ParseChannel parses the body of a get_channel reply (Hamlib's dump_chan format):
//
//	Channel: 1, Name: ''
//	VFO: MEM, Antenna: 0, Split: OFF
//	Freq:   7.0000000 MHz	Mode:   LSB	Width:   2.4000 kHz
//	txFreq: 0.0 Hz	txMode: 	txWidth: 0.0 Hz
//	Shift: +, Offset: +600.0 kHz, Step: 0.0 Hz, RIT: 0.0 Hz, XIT: 0.0 Hz
//	CTCSS: 88.5Hz, CTCSSsql: 0.0Hz, DCS: 0.0, DCSsql: 0.0
//	Functions: ...
//	Levels: ...
func ParseChannel(lines []string) (Channel, error) {
	var ch Channel
	head := false
	kv := map[string]string{}
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if m := chanHeadRe.FindStringSubmatch(line); m != nil {
			ch.Number, _ = strconv.Atoi(m[1])
			ch.Name = strings.TrimSpace(m[2])
			head = true
			continue
		}
		if strings.HasPrefix(line, "Functions:") || strings.HasPrefix(line, "Levels:") {
			continue // per-channel functions and levels are not used
		}
		for _, tab := range strings.Split(line, "\t") {
			for _, part := range strings.Split(tab, ", ") {
				k, v, ok := strings.Cut(part, ":")
				if ok {
					kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
				}
			}
		}
	}
	if !head {
		return ch, errors.New("rigctl: not a get_channel reply")
	}
	var err error
	if ch.FreqHz, err = parseFreq(kv["Freq"]); err != nil {
		return ch, err
	}
	ch.Empty = ch.FreqHz <= 0
	if ch.Empty {
		return ch, nil
	}
	ch.Mode = kv["Mode"]
	if w, err := parseFreq(kv["Width"]); err == nil && w > 0 {
		ch.WidthHz = int(w)
	}
	ch.Split = kv["Split"] == "ON"
	if ch.Split {
		ch.TxFreqHz, _ = parseFreq(kv["txFreq"])
	}
	switch kv["Shift"] {
	case "+", "-":
		ch.RptrShift = kv["Shift"]
		if o, err := parseFreq(kv["Offset"]); err == nil {
			ch.RptrOffsHz = int(math.Abs(float64(o)))
		}
	}
	ch.CTCSS = parseTone(kv["CTCSS"])
	ch.CTCSSSql = parseTone(kv["CTCSSsql"])
	return ch, nil
}

// parseFreq reads Hamlib's sprintf_freq output, e.g. "7.0000000 MHz", "600.0 kHz", "0.0 Hz".
func parseFreq(s string) (int64, error) {
	m := freqValRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("rigctl: bad frequency %q", s)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, err
	}
	mult := map[string]float64{"Hz": 1, "kHz": 1e3, "MHz": 1e6, "GHz": 1e9}[m[2]]
	return int64(math.Round(v * mult)), nil
}

// parseTone reads "88.5Hz" as 885 (tenths of Hz); anything else is 0 (off).
func parseTone(s string) int {
	m := toneValRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	whole, _ := strconv.Atoi(m[1])
	tenth := 0
	if m[2] != "" {
		tenth, _ = strconv.Atoi(m[2])
	}
	return whole*10 + tenth
}
