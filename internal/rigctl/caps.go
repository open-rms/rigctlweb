package rigctl

import (
	"context"
	"strconv"
	"strings"
)

// Range is a numeric capability range as printed by dump_caps: NAME(min..max/step).
type Range struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step"`
}

// LevelCap describes one level (slider or meter) the rig can get and/or set.
type LevelCap struct {
	Name string `json:"name"`
	Range
	Get bool `json:"get"`
	Set bool `json:"set"`
	// Assumed is true when dump_caps gave no range and one was filled in from Hamlib's
	// conventions (float levels are 0..1; a few integer levels have customary ranges).
	Assumed bool `json:"assumed,omitempty"`
}

// Caps is the subset of `dump_caps` the UI needs. The UI renders only what is here.
type Caps struct {
	Model      string     `json:"model"`
	Mfg        string     `json:"mfg"`
	Hamlib     string     `json:"hamlib"`
	RigType    string     `json:"rigType"`
	PTTType    string     `json:"pttType"`
	Modes      []string   `json:"modes"`
	VFOs       []string   `json:"vfos"`
	VFOOps     []string   `json:"vfoOps"`
	GetFuncs   []string   `json:"getFuncs"`
	SetFuncs   []string   `json:"setFuncs"`
	Levels     []LevelCap `json:"levels"`
	GetParms   []string   `json:"getParms"`
	SetParms   []string   `json:"setParms"`
	Preamps    []int      `json:"preamps"`
	Attens     []int      `json:"attenuators"`
	AGCLevels  []NamedInt `json:"agcLevels"`
	MaxRIT     int        `json:"maxRit"`
	MaxXIT     int        `json:"maxXit"`
	CanSplit   bool       `json:"canSplit"`
	CanPTT     bool       `json:"canPtt"`
	HasMemory  bool       `json:"hasMemory"`
	Targetable bool       `json:"targetableVfo"`
	// Richer capabilities (see caps_ext.go).
	Can         map[string]bool       `json:"can"` // "set PTT", "get Frequency", ...
	ExtLevels   []ExtField            `json:"extLevels"`
	ExtFuncs    []ExtField            `json:"extFuncs"`
	ExtParms    []ExtField            `json:"extParms"`
	Parms       []ParmCap             `json:"parms"`
	TuningSteps []ModeSet             `json:"tuningSteps"`
	Filters     []ModeSet             `json:"filters"`
	Bandwidths  map[string]Bandwidths `json:"bandwidths"`
	Memories    []MemRange            `json:"memories"`
	ScanOps     []string              `json:"scanOps"`
	CTCSS       []int                 `json:"ctcss"` // tenths of Hz
	DCS         []int                 `json:"dcs"`
	// Raw is the untouched dump_caps text, kept for the "report a radio" workflow.
	Raw string `json:"-"`
}

// NamedInt is an enum value like AGC "3=SLOW".
type NamedInt struct {
	Value int    `json:"value"`
	Name  string `json:"name"`
}

// HasLevel reports whether the rig can read the named level.
func (c *Caps) HasLevel(name string) bool {
	for _, l := range c.Levels {
		if l.Name == name && l.Get {
			return true
		}
	}
	return false
}

// HasFunc reports whether the rig can read the named function.
func (c *Caps) HasFunc(name string) bool {
	for _, f := range c.GetFuncs {
		if f == name {
			return true
		}
	}
	return false
}

// FetchCaps runs dump_caps and parses it.
func (c *Client) FetchCaps(ctx context.Context) (*Caps, error) {
	lines, err := c.Raw(ctx, `\dump_caps`)
	if err != nil {
		return nil, err
	}
	return ParseCaps(lines), nil
}

// ParseCaps parses dump_caps output lines.
func ParseCaps(lines []string) *Caps {
	caps := &Caps{Raw: strings.Join(lines, "\n")}
	levels := map[string]*LevelCap{}
	var order []string

	addLevels := func(rest string, get bool) {
		for _, tok := range splitTokens(rest) {
			name, rng, ok := parseRangeTok(tok)
			if !ok {
				continue
			}
			l, exists := levels[name]
			if !exists {
				l = &LevelCap{Name: name, Range: rng}
				levels[name] = l
				order = append(order, name)
			}
			if get {
				l.Get = true
			} else {
				l.Set = true
			}
			if l.Range == (Range{}) {
				l.Range = rng
			}
		}
	}

	for _, ln := range lines {
		key, val, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "Model name":
			caps.Model = val
		case "Mfg name":
			caps.Mfg = val
		case "Hamlib version":
			caps.Hamlib = val
		case "Rig type":
			caps.RigType = val
		case "PTT type":
			caps.PTTType = val
			caps.CanPTT = val != "" && !strings.EqualFold(val, "None")
		case "Has targetable VFO":
			caps.Targetable = strings.HasPrefix(val, "Y")
		case "Mode list":
			caps.Modes = strings.Fields(val)
		case "VFO list":
			caps.VFOs = strings.Fields(val)
		case "VFO Ops":
			caps.VFOOps = strings.Fields(val)
		case "Get functions":
			caps.GetFuncs = strings.Fields(val)
		case "Set functions":
			caps.SetFuncs = strings.Fields(val)
		case "Get level":
			addLevels(val, true)
		case "Set level":
			addLevels(val, false)
		case "Get parameters":
			caps.GetParms = nameList(val)
		case "Set parameters":
			caps.SetParms = nameList(val)
		case "Preamp":
			caps.Preamps = dbList(val)
		case "Attenuator":
			caps.Attens = dbList(val)
		case "AGC levels":
			for _, t := range strings.Fields(val) {
				if v, n, ok := strings.Cut(t, "="); ok {
					if iv, err := strconv.Atoi(v); err == nil {
						caps.AGCLevels = append(caps.AGCLevels, NamedInt{iv, n})
					}
				}
			}
		case "Max RIT":
			caps.MaxRIT = parseKHzMax(val)
		case "Max XIT":
			caps.MaxXIT = parseKHzMax(val)
		case "Number of banks":
			// handled via memories below
		}
	}
	for _, n := range order {
		l := *levels[n]
		if l.Max <= l.Min {
			if r, ok := defaultLevelRange(l.Name); ok {
				l.Range, l.Assumed = r, true
			}
		}
		caps.Levels = append(caps.Levels, l)
	}
	parseExtended(lines, caps)
	// The "PTT type" line only describes a separate PTT port; many radios key over CAT.
	if v, ok := caps.Can["set PTT"]; ok {
		caps.CanPTT = v
	}
	caps.CanSplit = len(caps.VFOs) > 1 && (caps.Can["set Split VFO"] || len(caps.Can) == 0)
	caps.HasMemory = strings.Contains(caps.Raw, "Memories:") && strings.Contains(caps.Raw, "MEM")
	return caps
}

// splitTokens splits "A(0..1/0.1) B(0..0/0)" on spaces that are outside parentheses.
func splitTokens(s string) []string {
	var out []string
	depth, start := 0, -1
	for i, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ' ' && depth == 0:
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

func parseRangeTok(tok string) (string, Range, bool) {
	open := strings.IndexByte(tok, '(')
	if open < 0 || !strings.HasSuffix(tok, ")") {
		return "", Range{}, false
	}
	name := tok[:open]
	body := tok[open+1 : len(tok)-1]
	mm, step, _ := strings.Cut(body, "/")
	lo, hi, ok := strings.Cut(mm, "..")
	if !ok {
		return name, Range{}, true // enum-style parameter such as KEYERTYPE(STRAIGHT,BUG)
	}
	var r Range
	r.Min, _ = strconv.ParseFloat(lo, 64)
	r.Max, _ = strconv.ParseFloat(hi, 64)
	r.Step, _ = strconv.ParseFloat(step, 64)
	return name, r, true
}

func nameList(s string) []string {
	var out []string
	for _, t := range splitTokens(s) {
		if i := strings.IndexByte(t, '('); i > 0 {
			out = append(out, t[:i])
		}
	}
	return out
}

func dbList(s string) []int {
	var out []int
	for _, t := range strings.Fields(s) {
		if v, err := strconv.Atoi(strings.TrimSuffix(t, "dB")); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// parseKHzMax turns "-9.990kHz/+9.990kHz" into 9990 (Hz).
func parseKHzMax(s string) int {
	_, hi, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}
	hi = strings.TrimPrefix(hi, "+")
	switch {
	case strings.HasSuffix(hi, "kHz"):
		f, _ := strconv.ParseFloat(strings.TrimSuffix(hi, "kHz"), 64)
		return int(f*1000 + 0.5)
	case strings.HasSuffix(hi, "Hz"):
		f, _ := strconv.ParseFloat(strings.TrimSuffix(hi, "Hz"), 64)
		return int(f + 0.5)
	}
	return 0
}
