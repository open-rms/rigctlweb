package presets

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/open-rms/rigctlweb/internal/catallow"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// Raw CAT steps (kind "cat") are applied only on a narrow path:
//   - every command form passes the openRMS allow-list (internal/catallow), or the whole
//     profile is refused;
//   - only ASCII CAT (Kenwood, Yaesu, Elecraft) to a radio of one of those makes is sent;
//   - the step has a read command and a reply pattern, and its set template is exactly the read
//     command with one value placeholder, so the value can be read before the change (for
//     Restore) and read back after it;
//   - the operator confirms the exact commands, and nothing is sent while the radio transmits.
//
// Everything else stays a manual step for the operator to do on the radio.

// catValue is a step value as CAT text: a non-negative whole number or plain letters and digits.
func catValue(v any) (string, bool) {
	switch x := v.(type) {
	case float64:
		if x < 0 || x != math.Trunc(x) || x > 1e15 {
			return "", false
		}
		return strconv.FormatInt(int64(x), 10), true
	case string:
		return x, catallow.ValueOK(x)
	}
	return "", false
}

// renderCat fills every placeholder in a template with val, padding it to the placeholder's
// width with zeros. A value longer than the width, or a padded value that isn't digits, is an
// error: it would change which command is sent.
func renderCat(template, val string) (string, error) {
	var bad error
	out := catallow.Placeholder().ReplaceAllStringFunc(template, func(m string) string {
		sub := catallow.Placeholder().FindStringSubmatch(m)
		if sub[1] == "" {
			return val
		}
		w, _ := strconv.Atoi(sub[1])
		if len(val) > w {
			bad = fmt.Errorf("value %s is longer than %d characters", val, w)
		} else if strings.Trim(val, "0123456789") != "" {
			bad = fmt.Errorf("value %s must be digits to pad to %d", val, w)
		}
		return strings.Repeat("0", max(0, w-len(val))) + val
	})
	return out, bad
}

// zeroFill fills each placeholder with zeros of its width (one zero without a width).
func zeroFill(template string) string {
	return catallow.Placeholder().ReplaceAllStringFunc(template, func(m string) string {
		sub := catallow.Placeholder().FindStringSubmatch(m)
		w, _ := strconv.Atoi(sub[1])
		return strings.Repeat("0", max(1, w))
	})
}

// catRefusal checks every command a cat step could send against the allow-list, as openRMS
// does. Any failure refuses the whole profile: a published profile never has one.
func catRefusal(s Step) string {
	c := s.Cat
	if c == nil {
		return fmt.Sprintf("step %q is a raw CAT step without commands", s.ID)
	}
	if catallow.Get(c.Protocol) == nil {
		return fmt.Sprintf("step %q uses an unknown CAT protocol %q", s.ID, c.Protocol)
	}
	check := func(use, cmd string) string {
		if r := catallow.Check(c.Protocol, use, cmd); !r.Allowed {
			return fmt.Sprintf("step %q: %s", s.ID, r.Why)
		}
		return ""
	}
	if c.Read != "" {
		if why := check("read", c.Read); why != "" {
			return why
		}
	}
	if c.Set != "" {
		if why := check("set", zeroFill(c.Set)); why != "" {
			return why
		}
	}
	if s.Value != nil {
		val, ok := catValue(s.Value)
		if !ok {
			return fmt.Sprintf("step %q: a raw CAT value must be plain letters and digits", s.ID)
		}
		if c.Set != "" && !s.CheckOnly {
			cmd, err := renderCat(c.Set, val)
			if err != nil {
				return fmt.Sprintf("step %q: %v", s.ID, err)
			}
			if why := check("set", cmd); why != "" {
				return why
			}
		}
	}
	return ""
}

// catSpec is a cat step that rigctlWeb can read on this radio.
type catSpec struct {
	Read     string
	Reply    *regexp.Regexp
	Template string // "" when it can't be set
	Set      string // the rendered set command
	Safety   string
}

// planCat works out what rigctlWeb can do with a cat step on this radio. readWhy and setWhy
// say why it can't read or set it ("" when it can).
func planCat(s Step, caps *rigctl.Caps) (spec *catSpec, readWhy, setWhy string) {
	c := s.Cat
	if c == nil {
		return nil, "the step has no commands", "the step has no commands"
	}
	if c.Protocol != "ascii" {
		why := "rigctlWeb doesn't send Icom CI-V commands yet"
		return nil, why, why
	}
	if caps == nil {
		return nil, "no radio connected", "no radio connected"
	}
	if !catallow.Get("ascii").ForManufacturer(caps.Mfg) {
		why := "this radio doesn't use Kenwood/Yaesu-style CAT"
		return nil, why, why
	}
	// openRMS step_rules (ASCII CAT can't tell a read from a set by its syntax): the read is the
	// set template without its value, the reply is anchored to the read, and a read without a
	// set is allowed only for families that can't be set (meters). Checked before anything is
	// read, by Check too.
	if field, why := catallow.ValidateStep(c.Protocol, c.Read, c.Reply, c.Set); field != "" {
		why = "rigctlWeb won't send its commands: " + why
		return nil, why, why
	}
	if c.Read == "" {
		why := "the step has no read command, so its current value can't be saved for Restore"
		return nil, why, why
	}
	if !catallow.Check("ascii", "read", c.Read).Allowed {
		return nil, "its read command isn't allowed", "its read command isn't allowed"
	}
	re, err := regexp.Compile(c.Reply)
	if len(c.Reply) > 200 || err != nil || re.NumSubexp() != 1 {
		why := "its reply pattern can't be used"
		return nil, why, why
	}
	spec = &catSpec{Read: c.Read, Reply: re, Safety: "rx"}
	switch {
	case s.CheckOnly || s.Value == nil:
		return spec, "", "checked only, never set"
	case c.Set == "":
		return spec, "", "the step has no set command"
	}
	val, ok := catValue(s.Value)
	if !ok {
		return spec, "", "its value isn't plain letters and digits"
	}
	cmd, err := renderCat(c.Set, val)
	if err != nil {
		return spec, "", err.Error()
	}
	r := catallow.Check("ascii", "set", cmd)
	if !r.Allowed {
		return spec, "", r.Why
	}
	spec.Template, spec.Set = c.Set, cmd
	if zr := catallow.Check("ascii", "set", zeroFill(c.Set)); r.Safety == "tx" || zr.Safety == "tx" {
		spec.Safety = "tx"
	}
	return spec, "", ""
}

// catCapture reads the value out of a reply with the step's pattern. The reply must be exactly
// the read command with the value in place of its ';' terminator, so a loose pattern can't pick
// digits out of the wrong place (Restore would then write a wrong value back).
func catCapture(re *regexp.Regexp, read, reply string) (string, error) {
	m := re.FindStringSubmatch(reply)
	if m == nil {
		return "", fmt.Errorf("the radio answered %q, which doesn't match the profile's reply pattern", reply)
	}
	if !catallow.ValueOK(m[1]) || reply != strings.TrimSuffix(read, ";")+m[1]+";" {
		return "", fmt.Errorf("the radio answered %q, which isn't the read command followed by a value", reply)
	}
	return m[1], nil
}

// sameCAT compares two CAT values: numerically when both are numbers ("050" equals 50), else
// ignoring case.
func sameCAT(a, b string) bool {
	fa, ea := strconv.ParseFloat(a, 64)
	fb, eb := strconv.ParseFloat(b, 64)
	if ea == nil && eb == nil {
		return fa == fb
	}
	return strings.EqualFold(a, b)
}
