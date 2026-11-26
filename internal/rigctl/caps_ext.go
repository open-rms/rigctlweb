package rigctl

import (
	"regexp"
	"strconv"
	"strings"
)

// ExtField is a rig-specific ("extra") level, function or parameter as described by
// dump_caps: a type, label, optional numeric range and optional enumerated values.
type ExtField struct {
	Name    string     `json:"name"`
	Type    string     `json:"type"` // NUMERIC, CHECKBUTTON, COMBO, BUTTON, STRING, INT
	Label   string     `json:"label"`
	Tooltip string     `json:"tooltip,omitempty"`
	Range   *Range     `json:"range,omitempty"`
	Values  []NamedInt `json:"values,omitempty"`
}

// ParmCap is a rig parameter (rigctl get_parm/set_parm).
type ParmCap struct {
	Name    string   `json:"name"`
	Get     bool     `json:"get"`
	Set     bool     `json:"set"`
	Range   Range    `json:"range"`
	Options []string `json:"options,omitempty"` // enumerated parameters like KEYERTYPE
}

// ModeSet pairs a width or step (Hz) with the modes it applies to.
type ModeSet struct {
	Hz    float64  `json:"hz"`
	Modes []string `json:"modes"`
}

// Bandwidths are a mode's normal/narrow/wide passbands in Hz (0 = not defined).
type Bandwidths struct {
	Normal float64 `json:"normal"`
	Narrow float64 `json:"narrow"`
	Wide   float64 `json:"wide"`
}

// MemRange is a block of memory channels of one type (MEM, BAND, CALL, VOICE, MORSE...).
type MemRange struct {
	Start int      `json:"start"`
	End   int      `json:"end"`
	Type  string   `json:"type"`
	Caps  []string `json:"caps"`
}

var (
	canRe    = regexp.MustCompile(`^Can ([a-zA-Z/ ]+?):\s*([YNE])\s*$`)
	extValRe = regexp.MustCompile(`(\d+)="([^"]*)"`)
)

// hz converts "2.4000 kHz", "600.0 Hz" or "1.2 MHz" to Hz.
func toHz(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ","))
	num, unit, _ := strings.Cut(s, " ")
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "khz":
		v *= 1e3
	case "mhz":
		v *= 1e6
	case "ghz":
		v *= 1e9
	}
	return v
}

// parseExtended fills the richer fields of Caps from dump_caps lines.
func parseExtended(lines []string, caps *Caps) {
	caps.Can = map[string]bool{}
	section := ""
	var cur *ExtField
	var curList *[]ExtField
	var mem *MemRange

	flush := func() {
		if cur != nil && curList != nil {
			*curList = append(*curList, *cur)
		}
		cur = nil
	}
	endSection := func() { flush(); section = ""; curList = nil; mem = nil }

	for _, raw := range lines {
		// Capability matrix lines: "Can set PTT:\tY"
		if m := canRe.FindStringSubmatch(raw); m != nil {
			caps.Can[strings.TrimSpace(m[1])] = m[2] != "N"
			continue
		}
		indented := strings.HasPrefix(raw, "\t") || strings.HasPrefix(raw, " ")
		if !indented {
			endSection()
			key, val, _ := strings.Cut(raw, ":")
			val = strings.TrimSpace(val)
			switch key {
			case "Extra functions":
				section, curList = "extf", &caps.ExtFuncs
			case "Extra levels":
				section, curList = "extl", &caps.ExtLevels
			case "Extra parameters":
				section, curList = "extp", &caps.ExtParms
			case "Memories":
				section = "mem"
			case "Tuning steps":
				section = "steps"
			case "Filters":
				section = "filters"
			case "Bandwidths":
				section = "bw"
			case "Scan Ops":
				caps.ScanOps = strings.Fields(val)
			case "CTCSS":
				caps.CTCSS = toneList(val)
			case "DCS":
				caps.DCS = codeList(val)
			}
			continue
		}
		line := strings.TrimRight(raw, " \t")
		trim := strings.TrimSpace(line)
		switch section {
		case "extf", "extl", "extp":
			depth := len(line) - len(strings.TrimLeft(line, "\t"))
			if depth == 1 { // a new field name
				flush()
				cur = &ExtField{Name: trim}
				continue
			}
			if cur == nil {
				continue
			}
			k, v, _ := strings.Cut(trim, ":")
			v = strings.TrimSpace(v)
			switch k {
			case "Type":
				cur.Type = v
			case "Label":
				cur.Label = v
			case "Tooltip":
				cur.Tooltip = v
			case "Range":
				if _, r, ok := parseRangeTok("x(" + v + ")"); ok {
					cur.Range = &r
				}
			case "Values":
				for _, m := range extValRe.FindAllStringSubmatch(v, -1) {
					n, _ := strconv.Atoi(m[1])
					cur.Values = append(cur.Values, NamedInt{n, m[2]})
				}
			}
		case "steps", "filters":
			hzs, modes, ok := strings.Cut(trim, ":")
			if !ok || strings.EqualFold(hzs, "ANY") {
				continue
			}
			ms := ModeSet{Hz: toHz(hzs), Modes: strings.Fields(modes)}
			if ms.Hz <= 0 || len(ms.Modes) == 0 {
				continue
			}
			if section == "steps" {
				caps.TuningSteps = append(caps.TuningSteps, ms)
			} else {
				caps.Filters = append(caps.Filters, ms)
			}
		case "bw":
			// "AM\tNormal: 9.0000 kHz,\tNarrow: 0.0 Hz,\tWide: 0.0 Hz"
			parts := strings.Split(trim, "\t")
			if len(parts) < 2 {
				continue
			}
			if caps.Bandwidths == nil {
				caps.Bandwidths = map[string]Bandwidths{}
			}
			var b Bandwidths
			for _, p := range parts[1:] {
				k, v, _ := strings.Cut(strings.TrimSpace(p), ":")
				switch strings.TrimSpace(k) {
				case "Normal":
					b.Normal = toHz(v)
				case "Narrow":
					b.Narrow = toHz(v)
				case "Wide":
					b.Wide = toHz(v)
				}
			}
			caps.Bandwidths[parts[0]] = b
		case "mem":
			if strings.HasPrefix(trim, "Mem caps:") {
				if mem != nil {
					mem.Caps = strings.Fields(strings.TrimPrefix(trim, "Mem caps:"))
				}
				continue
			}
			// "1..99:   \tMEM"
			rng, typ, ok := strings.Cut(trim, ":")
			if !ok {
				continue
			}
			lo, hi, ok := strings.Cut(rng, "..")
			if !ok {
				continue
			}
			s, _ := strconv.Atoi(lo)
			e, _ := strconv.Atoi(hi)
			caps.Memories = append(caps.Memories, MemRange{Start: s, End: e, Type: strings.TrimSpace(typ)})
			mem = &caps.Memories[len(caps.Memories)-1]
		}
	}
	endSection()

	// Parameters carry their option lists / ranges inline.
	caps.Parms = nil
	idx := map[string]int{}
	for _, ln := range lines {
		key, val, ok := strings.Cut(ln, ":")
		if !ok || (key != "Get parameters" && key != "Set parameters") {
			continue
		}
		for _, t := range splitTokens(strings.TrimSpace(val)) {
			open := strings.IndexByte(t, '(')
			if open <= 0 || !strings.HasSuffix(t, ")") {
				continue
			}
			name, body := t[:open], t[open+1:len(t)-1]
			i, seen := idx[name]
			if !seen {
				p := ParmCap{Name: name}
				if strings.Contains(body, "..") {
					_, p.Range, _ = parseRangeTok(t)
				} else if body != "" {
					p.Options = strings.Split(body, ",")
				}
				caps.Parms = append(caps.Parms, p)
				i = len(caps.Parms) - 1
				idx[name] = i
			}
			if key == "Get parameters" {
				caps.Parms[i].Get = true
			} else {
				caps.Parms[i].Set = true
			}
		}
	}
}

// toneList parses "67.0 69.3 ... Hz, 50 tones" into tenths of Hz (the unit rigctld uses).
func toneList(s string) []int {
	s, _, _ = strings.Cut(s, "Hz")
	var out []int
	for _, f := range strings.Fields(s) {
		if v, err := strconv.ParseFloat(f, 64); err == nil {
			out = append(out, int(v*10+0.5))
		}
	}
	return out
}

func codeList(s string) []int {
	s, _, _ = strings.Cut(s, ",")
	var out []int
	for _, f := range strings.Fields(s) {
		if v, err := strconv.Atoi(f); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// can reports a "Can <verb> <thing>" capability.
func (c *Caps) CanDo(what string) bool { return c.Can[what] }

// WidthsFor returns the selectable passband widths (Hz, ascending) for a mode.
func (c *Caps) WidthsFor(mode string) []float64 {
	seen := map[float64]bool{}
	var out []float64
	add := func(hz float64) {
		if hz > 0 && !seen[hz] {
			seen[hz] = true
			out = append(out, hz)
		}
	}
	for _, f := range c.Filters {
		for _, m := range f.Modes {
			if m == mode {
				add(f.Hz)
			}
		}
	}
	if b, ok := c.Bandwidths[mode]; ok {
		add(b.Narrow)
		add(b.Normal)
		add(b.Wide)
	}
	for i := 1; i < len(out); i++ { // tiny insertion sort
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// floatLevels are the levels Hamlib defines as floating point 0.0..1.0 (RIG_LEVEL_FLOAT_LIST
// and friends). Many backends list them without a range, so 0..1 is the documented default.
var floatLevels = map[string]bool{
	"AF": true, "RF": true, "SQL": true, "APF": true, "NR": true, "NB": true, "COMP": true,
	"BAL": true, "VOXGAIN": true, "ANTIVOX": true, "MICGAIN": true, "RFPOWER": true,
	"MONITOR_GAIN": true, "PBT_IN": true, "PBT_OUT": true, "USB_AF": true,
}

// intLevelRanges are customary ranges for integer levels that backends list without one.
var intLevelRanges = map[string]Range{
	"KEYSPD":     {Min: 5, Max: 60, Step: 1},      // words per minute
	"CWPITCH":    {Min: 300, Max: 1000, Step: 10}, // Hz
	"VOXDELAY":   {Min: 0, Max: 20, Step: 1},      // tenths of a second
	"BKINDL":     {Min: 0, Max: 30, Step: 1},
	"IF":         {Min: -1500, Max: 1500, Step: 50}, // Hz
	"NOTCHF":     {Min: 0, Max: 4000, Step: 10},     // Hz
	"SLOPE_LOW":  {Min: 0, Max: 20, Step: 1},
	"SLOPE_HIGH": {Min: 0, Max: 20, Step: 1},
}

// defaultLevelRange returns a range for a level whose dump_caps entry had none.
func defaultLevelRange(name string) (Range, bool) {
	if floatLevels[name] {
		return Range{Min: 0, Max: 1, Step: 0.01}, true
	}
	r, ok := intLevelRanges[name]
	return r, ok
}
