package rigctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, name string) *Caps {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return ParseCaps(strings.Split(strings.TrimRight(string(b), "\n"), "\n"))
}

// FTDX-10 captured from a real radio through rigctld (Hamlib 4.7.2).
func TestParseFTDX10(t *testing.T) {
	c := load(t, "ftdx10.caps")
	if c.Model != "FTDX-10" || c.Mfg != "Yaesu" {
		t.Fatalf("model %q %q", c.Model, c.Mfg)
	}
	if !c.CanPTT {
		t.Error("FTDX-10 keys over CAT (Can set PTT: Y) even though PTT type is None")
	}
	if !c.Can["set RIT"] || c.Can["set Param"] || c.Can["set Ant"] {
		t.Errorf("capability matrix wrong: %v", c.Can)
	}
	if len(c.ExtLevels) == 0 || c.ExtLevels[0].Name != "ROOFINGFILTER" || c.ExtLevels[0].Type != "COMBO" || len(c.ExtLevels[0].Values) != 5 {
		t.Fatalf("ext levels: %+v", c.ExtLevels)
	}
	if got := c.WidthsFor("USB"); len(got) < 3 || got[0] != 1800 {
		t.Errorf("USB widths = %v", got)
	}
	if got := c.WidthsFor("CW"); got[0] != 300 {
		t.Errorf("CW widths = %v", got)
	}
	if len(c.TuningSteps) == 0 || c.TuningSteps[0].Hz != 10 {
		t.Errorf("tuning steps = %+v", c.TuningSteps)
	}
	if len(c.CTCSS) != 50 || c.CTCSS[0] != 670 {
		t.Errorf("ctcss = %d tones, first %v", len(c.CTCSS), c.CTCSS[:1])
	}
	var mem *MemRange
	for i := range c.Memories {
		if c.Memories[i].Type == "MEM" {
			mem = &c.Memories[i]
		}
	}
	if mem == nil || mem.Start != 1 || mem.End != 99 || len(mem.Caps) == 0 {
		t.Errorf("memories = %+v", c.Memories)
	}
	if len(c.Attens) != 3 || len(c.Preamps) != 2 || len(c.AGCLevels) != 5 {
		t.Errorf("atten/preamp/agc = %v %v %v", c.Attens, c.Preamps, c.AGCLevels)
	}
}

func TestParseDummyExtras(t *testing.T) {
	c := load(t, "dummy.caps")
	if len(c.ExtLevels) == 0 || len(c.ExtFuncs) == 0 || len(c.ExtParms) == 0 {
		t.Fatalf("ext: %d levels, %d funcs, %d parms", len(c.ExtLevels), len(c.ExtFuncs), len(c.ExtParms))
	}
	kinds := map[string]bool{}
	for _, l := range c.ExtLevels {
		kinds[l.Type] = true
	}
	for _, want := range []string{"NUMERIC", "CHECKBUTTON", "BUTTON", "COMBO"} {
		if !kinds[want] {
			t.Errorf("missing ext level type %s: %+v", want, c.ExtLevels)
		}
	}
	var enum, num *ParmCap
	for i := range c.Parms {
		p := &c.Parms[i]
		if p.Name == "KEYERTYPE" {
			enum = p
		}
		if p.Name == "BACKLIGHT" {
			num = p
		}
	}
	if enum == nil || len(enum.Options) != 3 || !enum.Set {
		t.Errorf("KEYERTYPE = %+v", enum)
	}
	if num == nil || num.Range.Max != 1 {
		t.Errorf("BACKLIGHT = %+v", num)
	}
}

func TestToHz(t *testing.T) {
	for in, want := range map[string]float64{"2.4000 kHz": 2400, "600.0 Hz": 600, "1.5 MHz": 1.5e6, "9.0000 kHz,": 9000} {
		if got := toHz(in); got != want {
			t.Errorf("toHz(%q) = %v, want %v", in, got, want)
		}
	}
}

// Every fixture is a real `rigctl -m N -u` dump. These checks hold for any radio.
func TestFixturesAreWellFormed(t *testing.T) {
	files, _ := os.ReadDir("testdata")
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".caps") {
			continue // other fixtures (get_channel replies) are tested where they are used
		}
		t.Run(name, func(t *testing.T) {
			c := load(t, name)
			if c.Model == "" || c.Mfg == "" {
				t.Fatalf("no model/mfg: %q %q", c.Model, c.Mfg)
			}
			for _, kind := range [][]ExtField{c.ExtLevels, c.ExtFuncs, c.ExtParms} {
				for _, e := range kind {
					if e.Name == "" || e.Type == "" {
						t.Errorf("ext field without name/type: %+v", e)
					}
					if e.Type == "COMBO" && len(e.Values) == 0 {
						t.Errorf("%s: combo without values", e.Name)
					}
					if e.Type == "NUMERIC" && e.Range == nil {
						t.Errorf("%s: numeric without range", e.Name)
					}
				}
			}
			for _, l := range c.Levels {
				if l.Set && l.Max <= l.Min && (floatLevels[l.Name] || intLevelRanges[l.Name] != (Range{})) {
					t.Errorf("level %s should have a default range, has %+v", l.Name, l.Range)
				}
			}
		})
	}
}

// Backends that list levels without a range (Xiegu G90, many Icoms) get Hamlib's defaults.
func TestDefaultRangesForRangelessLevels(t *testing.T) {
	c := load(t, "g90.caps")
	var af, keyspd *LevelCap
	for i := range c.Levels {
		switch c.Levels[i].Name {
		case "AF":
			af = &c.Levels[i]
		case "KEYSPD":
			keyspd = &c.Levels[i]
		}
	}
	if af == nil || !af.Assumed || af.Min != 0 || af.Max != 1 {
		t.Fatalf("AF = %+v", af)
	}
	if keyspd == nil || !keyspd.Assumed || keyspd.Max < 30 {
		t.Fatalf("KEYSPD = %+v", keyspd)
	}
	// A level that does declare its own range keeps it and isn't marked assumed.
	ft := load(t, "ft991.caps")
	for _, l := range ft.Levels {
		if l.Name == "AF" && (l.Assumed || l.Max != 1) {
			t.Errorf("FT-991 AF = %+v", l)
		}
	}
}

func TestOtherRadios(t *testing.T) {
	cases := map[string]struct {
		model       string
		minLevels   int
		wantExtParm bool
		wantCTCSS   bool
	}{
		"ic7300.caps": {"IC-7300", 30, false, true},
		"ic7610.caps": {"IC-7610", 30, false, true},
		"g90.caps":    {"G90", 15, false, true},
		"ts590s.caps": {"TS-590S", 20, false, true},
		"k3.caps":     {"K3", 15, true, false},
		"ft991.caps":  {"FT-991", 25, false, true},
	}
	for file, w := range cases {
		c := load(t, file)
		if c.Model != w.model {
			t.Errorf("%s: model %q", file, c.Model)
		}
		if len(c.Levels) < w.minLevels {
			t.Errorf("%s: %d levels, want >= %d", file, len(c.Levels), w.minLevels)
		}
		if (len(c.ExtParms) > 0) != w.wantExtParm {
			t.Errorf("%s: ext parms = %d", file, len(c.ExtParms))
		}
		if (len(c.CTCSS) > 0) != w.wantCTCSS {
			t.Errorf("%s: ctcss tones = %d", file, len(c.CTCSS))
		}
		if len(c.Modes) < 5 || len(c.VFOs) == 0 {
			t.Errorf("%s: modes=%d vfos=%d", file, len(c.Modes), len(c.VFOs))
		}
		if len(c.WidthsFor("USB")) == 0 && file != "k3.caps" && file != "ts590s.caps" {
			t.Errorf("%s: no USB passband widths", file)
		}
	}
}

// GEN_CAPS_JSON=<dir> go test ./internal/rigctl -run TestGenerateCapsJSON writes the parsed
// capabilities of every fixture as JSON, for the web UI's tests (web/src/lib/testdata).
func TestGenerateCapsJSON(t *testing.T) {
	dir := os.Getenv("GEN_CAPS_JSON")
	if dir == "" {
		t.Skip("set GEN_CAPS_JSON to regenerate")
	}
	files, _ := os.ReadDir("testdata")
	for _, f := range files {
		c := load(t, f.Name())
		b, err := json.MarshalIndent(c, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, strings.TrimSuffix(f.Name(), ".caps")+".json")
		if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
