// Package catallow is openRMS's raw-CAT allow-list (openRMS-Spec rigs/cat-allowlist.json),
// embedded unchanged so rigctlWeb and openRMS-Rigs enforce the same rules. It is never fetched
// at run time: a new list arrives only with a new rigctlWeb build.
package catallow

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed cat-allowlist.json
var raw []byte

// Family is an allowed command prefix.
type Family struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	Read   bool   `json:"read"`
	Set    bool   `json:"set"`
	Safety string `json:"safety"`
}

// Denied is a prefix refused even when a family matches.
type Denied struct {
	Prefix string `json:"prefix"`
	Reason string `json:"reason"`
}

// Protocol is one wire format: "ascii" (Kenwood, Yaesu, Elecraft) or "civ" (Icom CI-V body).
type Protocol struct {
	Manufacturers  []string `json:"manufacturers"`
	CommandPattern string   `json:"command_pattern"`
	AllowedChars   string   `json:"allowed_chars"`
	Terminator     string   `json:"terminator"`
	Families       []Family `json:"families"`
	Denied         []Denied `json:"denied"`

	command, chars *regexp.Regexp
}

type list struct {
	Version            string               `json:"version"`
	ValuePattern       string               `json:"value_pattern"`
	PlaceholderPattern string               `json:"placeholder_pattern"`
	Protocols          map[string]*Protocol `json:"protocols"`
}

var (
	l           list
	valueRe     *regexp.Regexp
	placeholder *regexp.Regexp
)

func init() {
	if err := json.Unmarshal(raw, &l); err != nil {
		panic("catallow: embedded list unreadable: " + err.Error())
	}
	valueRe = regexp.MustCompile(l.ValuePattern)
	placeholder = regexp.MustCompile(l.PlaceholderPattern)
	for _, p := range l.Protocols {
		p.command = regexp.MustCompile(p.CommandPattern)
		if p.AllowedChars != "" {
			p.chars = regexp.MustCompile(p.AllowedChars)
		}
	}
}

// Version is the embedded list's version.
func Version() string { return l.Version }

// ValueOK reports whether a CAT value is plain letters and digits, as the list requires.
func ValueOK(v string) bool { return valueRe.MatchString(v) }

// Placeholder matches {value} and {value:NN} in a set template; group 1 is the width.
func Placeholder() *regexp.Regexp { return placeholder }

// Get returns a protocol, or nil.
func Get(protocol string) *Protocol { return l.Protocols[protocol] }

// ForManufacturer reports whether the protocol is the one a manufacturer's radios speak.
func (p *Protocol) ForManufacturer(mfg string) bool {
	for _, m := range p.Manufacturers {
		if strings.EqualFold(strings.TrimSpace(mfg), m) {
			return true
		}
	}
	return false
}

// Result is the verdict on one command as it will be sent.
type Result struct {
	Allowed bool
	Safety  string // "rx" or "tx" for an allowed set
	Why     string // why it is refused
}

// Check applies the list's rules to a command exactly as it will be sent. use is "read" or
// "set". A denied prefix always wins; among families the longest matching prefix decides.
func Check(protocol, use, command string) Result {
	p := Get(protocol)
	if p == nil {
		return Result{Why: fmt.Sprintf("unknown CAT protocol %q", protocol)}
	}
	if use != "read" && use != "set" {
		return Result{Why: "use must be read or set"}
	}
	if command == "" || len(command) > 128 {
		return Result{Why: "empty or overlong command"}
	}
	var parts []string
	switch protocol {
	case "ascii":
		if p.chars != nil && !p.chars.MatchString(command) {
			return Result{Why: "only letters, digits and the terminator are allowed"}
		}
		if !strings.HasSuffix(command, p.Terminator) {
			return Result{Why: "the command must end with " + p.Terminator}
		}
		for _, s := range strings.Split(command, p.Terminator) {
			if s != "" {
				parts = append(parts, s)
			}
		}
	default:
		parts = []string{strings.ToUpper(command)}
	}
	if len(parts) == 0 {
		return Result{Why: "no command"}
	}
	res := Result{Allowed: true, Safety: "rx"}
	for _, c := range parts {
		r := p.checkOne(use, c)
		if !r.Allowed {
			return r
		}
		if r.Safety == "tx" {
			res.Safety = "tx"
		}
	}
	if use == "read" {
		res.Safety = ""
	}
	return res
}

func (p *Protocol) checkOne(use, c string) Result {
	if !p.command.MatchString(c) {
		return Result{Why: fmt.Sprintf("%q is not a well-formed command", c)}
	}
	for _, d := range p.Denied {
		if strings.HasPrefix(c, d.Prefix) {
			return Result{Why: fmt.Sprintf("%q is refused: %s", c, d.Reason)}
		}
	}
	best := p.family(c)
	if best == nil {
		return Result{Why: fmt.Sprintf("%q is not on the openRMS raw-CAT allow-list", c)}
	}
	if use == "read" && !best.Read || use == "set" && !best.Set {
		return Result{Why: fmt.Sprintf("%q (%s) may not be used to %s", c, best.Name, use)}
	}
	return Result{Allowed: true, Safety: best.Safety}
}

// replyPattern is what may sit inside a reply's capture group (step_rules.reply): no parentheses.
var replyPattern = regexp.MustCompile(`^[A-Za-z0-9\\\[\]{},+*?-]+$`)

// ValidateStep applies the list's step_rules, which bind a step's read, set and reply together
// because ASCII CAT can't tell a read from a set by its syntax. It returns the first field at
// fault ("set", "read" or "reply") and why, or "" when the step is sound. Empty strings mean
// the field is absent. Commands are checked against the list separately, with Check.
func ValidateStep(protocol, read, reply, set string) (field, why string) {
	p := Get(protocol)
	if p == nil {
		return "protocol", fmt.Sprintf("unknown CAT protocol %q", protocol)
	}
	term := p.Terminator
	single := func(cmd string) bool {
		return term == "" || strings.Count(cmd, term) == 1 && strings.HasSuffix(cmd, term)
	}
	if set != "" {
		locs := placeholder.FindAllStringIndex(set, -1)
		if len(locs) != 1 || set[locs[0][1]:] != term {
			return "set", "the set command needs exactly one value placeholder, at the end"
		}
		if !single(zeroFillLen(set)) {
			return "set", "the set must be a single command"
		}
	}
	if read != "" {
		if !single(read) {
			return "read", "the read must be a single command"
		}
		if set != "" {
			loc := placeholder.FindStringIndex(set)
			bare := set[:loc[0]] + set[loc[1]:]
			if read != bare && !(protocol == "civ" && strings.EqualFold(read, bare)) {
				return "read", fmt.Sprintf("the read %q is not the set without its value (%q)", read, bare)
			}
		} else if f := p.family(strings.ToUpper(strings.TrimSuffix(read, term))); f == nil || f.Set {
			return "read", fmt.Sprintf("the read %q has no set to bind it, and its family can be set", read)
		}
	}
	switch {
	case read == "" && reply != "":
		return "reply", "a reply needs a read"
	case read != "" && reply == "":
		return "reply", "a read needs a reply pattern"
	case read != "":
		pre, post := "^"+strings.TrimSuffix(read, term)+"(", ")"+term+"$"
		inner, ok := strings.CutPrefix(reply, pre)
		if ok {
			inner, ok = strings.CutSuffix(inner, post)
		}
		if !ok || !replyPattern.MatchString(inner) {
			return "reply", fmt.Sprintf("the reply must be ^%s(<value>)%s$", strings.TrimSuffix(read, term), term)
		}
	}
	return "", ""
}

// zeroFillLen fills placeholders with zeros of their width, so the template can be checked as
// a command.
func zeroFillLen(tpl string) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		w := 1
		if sub := placeholder.FindStringSubmatch(m); sub[1] != "" {
			fmt.Sscan(sub[1], &w)
		}
		return strings.Repeat("0", max(1, w))
	})
}

// family is the longest family whose prefix the command starts with, or nil.
func (p *Protocol) family(c string) *Family {
	var best *Family
	for i := range p.Families {
		f := &p.Families[i]
		if strings.HasPrefix(c, f.Prefix) && (best == nil || len(f.Prefix) > len(best.Prefix)) {
			best = f
		}
	}
	return best
}
