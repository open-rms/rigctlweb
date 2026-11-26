package rigctl

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// hamlibNames maps the names in rig.h's enum rig_errcode_e (without the RIG_ prefix) to our
// constants, so the table can be checked against the real header.
var hamlibNames = map[string]int{
	"EINVAL": ErrInval, "ECONF": ErrConf, "ENOMEM": ErrNoMem, "ENIMPL": ErrNotImpl,
	"ETIMEOUT": ErrTimeout, "EIO": ErrIO, "EINTERNAL": ErrInternal, "EPROTO": ErrProto,
	"ERJCTED": ErrReject, "ETRUNC": ErrTrunc, "ENAVAIL": ErrNotAvail, "ENTARGET": ErrNotTarget,
	"BUSERROR": ErrBusError, "BUSBUSY": ErrBusBusy, "EARG": ErrArg, "EVFO": ErrVfo,
	"EDOM": ErrDomain, "EDEPRECATED": ErrDeprecated, "ESECURITY": ErrSecurity,
	"EPOWER": ErrPower, "ELIMIT": ErrLimit, "EACCESS": ErrAccess,
}

// The numbers as they stand in Hamlib 4.7's rig.h, so the test still guards the table on
// machines without the header.
var hamlibNumbers = map[string]int{
	"EINVAL": 1, "ECONF": 2, "ENOMEM": 3, "ENIMPL": 4, "ETIMEOUT": 5, "EIO": 6, "EINTERNAL": 7,
	"EPROTO": 8, "ERJCTED": 9, "ETRUNC": 10, "ENAVAIL": 11, "ENTARGET": 12, "BUSERROR": 13,
	"BUSBUSY": 14, "EARG": 15, "EVFO": 16, "EDOM": 17, "EDEPRECATED": 18, "ESECURITY": 19,
	"EPOWER": 20, "ELIMIT": 21, "EACCESS": 22,
}

func TestErrorCodesMatchHamlib(t *testing.T) {
	for name, want := range hamlibNumbers {
		if got := hamlibNames[name]; got != want {
			t.Errorf("RIG_%s = %d in this package, Hamlib has %d", name, got, want)
		}
	}
	for code := 1; code <= 22; code++ {
		if hamlibText[code] == "" {
			t.Errorf("no description for code %d", code)
		}
	}
}

// When Hamlib's header is installed, check the table against it directly.
func TestErrorCodesMatchInstalledHeader(t *testing.T) {
	var src []byte
	for _, p := range []string{"/usr/include/hamlib/rig.h", "/usr/local/include/hamlib/rig.h", "/opt/homebrew/include/hamlib/rig.h"} {
		if b, err := os.ReadFile(p); err == nil {
			src = b
			break
		}
	}
	if src == nil {
		t.Skip("hamlib/rig.h not installed")
	}
	re := regexp.MustCompile(`(?m)^\s*RIG_(E[A-Z]+|BUSERROR|BUSBUSY),?\s*/\*!<\s*\\c\s+(\d+)`)
	seen := 0
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		want, _ := strconv.Atoi(m[2])
		got, ok := hamlibNames[m[1]]
		if m[1] == "EEND" {
			continue
		}
		if !ok {
			t.Errorf("rig.h has RIG_%s = %d, which this package does not know", m[1], want)
			continue
		}
		seen++
		if got != want {
			t.Errorf("RIG_%s: header says %d, package has %d", m[1], want, got)
		}
	}
	if seen < 17 {
		t.Fatalf("only matched %d codes in rig.h; the parser or header format changed", seen)
	}
}

func TestErrorWordsAreRight(t *testing.T) {
	for code, word := range map[int]string{
		-9:  "rejected",
		-8:  "protocol",
		-10: "truncated",
		-14: "busy",
		-4:  "not implemented",
		-11: "not available",
		-5:  "timeout",
		-6:  "I/O",
	} {
		if got := hamlibErr(code); !strings.Contains(got, word) {
			t.Errorf("hamlibErr(%d) = %q, want it to mention %q", code, got, word)
		}
	}
	if got := hamlibErr(-99); got != "error" {
		t.Errorf("unknown code: %q", got)
	}
}

func TestIsUnsupportedMatchesHamlibSemantics(t *testing.T) {
	unsupported := map[int]bool{ErrInval: true, ErrNotImpl: true, ErrNotAvail: true, ErrNotTarget: true, ErrVfo: true, ErrDomain: true}
	for code := 1; code <= 22; code++ {
		got := IsUnsupported(&Error{Code: -code, Cmd: "x"})
		if got != unsupported[code] {
			t.Errorf("IsUnsupported(RPRT -%d) = %v, want %v", code, got, unsupported[code])
		}
	}
	// A rejected command, a truncated one and a busy bus must not permanently disable a control.
	for _, code := range []int{ErrReject, ErrTrunc, ErrBusBusy, ErrBusError, ErrTimeout, ErrIO} {
		if IsUnsupported(&Error{Code: -code}) {
			t.Errorf("code %d must not count as unsupported", code)
		}
	}
	if IsUnsupported(nil) || IsUnsupported(os.ErrClosed) {
		t.Error("non-rigctld errors are never 'unsupported'")
	}
}
