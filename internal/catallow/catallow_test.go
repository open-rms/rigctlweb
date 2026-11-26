package catallow

import (
	"encoding/json"
	"os"
	"testing"
)

// The openRMS conformance cases for the allow-list (shared with openRMS-Rigs).
func TestCATVectors(t *testing.T) {
	b, err := os.ReadFile("../presets/testdata/check-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Cat struct {
			Cases []struct {
				Protocol, Use, Command, Safety, Why string
				Allowed                             bool
			} `json:"cases"`
		} `json:"cat"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cat.Cases) < 30 {
		t.Fatalf("only %d CAT cases", len(v.Cat.Cases))
	}
	for _, c := range v.Cat.Cases {
		r := Check(c.Protocol, c.Use, c.Command)
		if r.Allowed != c.Allowed {
			t.Errorf("%s %s %q: allowed=%v (%s), want %v (%s)", c.Protocol, c.Use, c.Command, r.Allowed, r.Why, c.Allowed, c.Why)
			continue
		}
		if c.Allowed && c.Safety != "" && r.Safety != c.Safety {
			t.Errorf("%s %q: safety %s, want %s", c.Protocol, c.Command, r.Safety, c.Safety)
		}
	}
}

func TestEdges(t *testing.T) {
	for _, c := range []struct {
		proto, use, cmd string
		ok              bool
	}{
		{"ascii", "set", ";", false},
		{"ascii", "set", "EX010415;;", true},
		{"ascii", "send", "EX010415;", false},
		{"ascii", "read", "ex010415;", false},
		{"ascii", "set", "PS0;", false},
		{"ascii", "set", "EX010415\x00;", false},
		{"rtty", "read", "EX010415;", false},
		{"civ", "set", "1A0001", false},
		{"civ", "set", "1A0501", true},
	} {
		if r := Check(c.proto, c.use, c.cmd); r.Allowed != c.ok {
			t.Errorf("%s %s %q = %v (%s)", c.proto, c.use, c.cmd, r.Allowed, r.Why)
		}
	}
	if !Get("ascii").ForManufacturer("yaesu") || Get("ascii").ForManufacturer("Icom") {
		t.Error("manufacturer match")
	}
	if !ValueOK("050") || ValueOK("05 0") || ValueOK("") {
		t.Error("value pattern")
	}
}

// The openRMS step_rules conformance cases (check-vectors "steps").
func TestStepVectors(t *testing.T) {
	b, err := os.ReadFile("../presets/testdata/check-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Steps struct {
			Cases []struct {
				Name  string
				Cat   struct{ Protocol, Read, Reply, Set string }
				Valid bool
				Field string
			} `json:"cases"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Steps.Cases) < 19 {
		t.Fatalf("only %d step cases", len(v.Steps.Cases))
	}
	for _, c := range v.Steps.Cases {
		field, why := ValidateStep(c.Cat.Protocol, c.Cat.Read, c.Cat.Reply, c.Cat.Set)
		if (field == "") != c.Valid || !c.Valid && field != c.Field {
			t.Errorf("%s: field %q (%s), want valid=%v field %q", c.Name, field, why, c.Valid, c.Field)
		}
	}
	if Version() != "1.1.0" {
		t.Errorf("allow-list version %s", Version())
	}
}
