package rigctl

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Typed helpers over Do. Names follow rigctl long commands.

func (c *Client) GetFreq(ctx context.Context) (int64, error) {
	r, err := c.Do(ctx, `\get_freq`)
	if err != nil {
		return 0, err
	}
	f, perr := strconv.ParseFloat(r.Value("Frequency"), 64)
	if perr != nil {
		return 0, fmt.Errorf("rigctl: bad frequency %q", r.Value("Frequency"))
	}
	return int64(f), nil
}

func (c *Client) SetFreq(ctx context.Context, hz int64) error {
	if hz <= 0 {
		return fmt.Errorf("rigctl: invalid frequency %d", hz)
	}
	_, err := c.Do(ctx, fmt.Sprintf(`\set_freq %d`, hz))
	return err
}

func (c *Client) GetMode(ctx context.Context) (mode string, passband int, err error) {
	r, err := c.Do(ctx, `\get_mode`)
	if err != nil {
		return "", 0, err
	}
	pb, _ := strconv.Atoi(r.Value("Passband"))
	return r.Value("Mode"), pb, nil
}

// SetMode sets mode and passband in Hz. Passband 0 means the rig default, -1 leaves it unchanged.
func (c *Client) SetMode(ctx context.Context, mode string, passband int) error {
	if !safeToken(mode) {
		return fmt.Errorf("rigctl: invalid mode %q", mode)
	}
	_, err := c.Do(ctx, fmt.Sprintf(`\set_mode %s %d`, mode, passband))
	return err
}

func (c *Client) GetPTT(ctx context.Context) (bool, error) {
	r, err := c.Do(ctx, `\get_ptt`)
	if err != nil {
		return false, err
	}
	return r.Value("PTT") != "0", nil
}

func (c *Client) SetPTT(ctx context.Context, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := c.Do(ctx, fmt.Sprintf(`\set_ptt %d`, v))
	return err
}

func (c *Client) GetVFO(ctx context.Context) (string, error) {
	r, err := c.Do(ctx, `\get_vfo`)
	if err != nil {
		return "", err
	}
	return r.Value("VFO"), nil
}

func (c *Client) SetVFO(ctx context.Context, vfo string) error {
	if !safeToken(vfo) {
		return fmt.Errorf("rigctl: invalid vfo %q", vfo)
	}
	_, err := c.Do(ctx, `\set_vfo `+vfo)
	return err
}

func (c *Client) GetSplit(ctx context.Context) (on bool, txVFO string, err error) {
	r, err := c.Do(ctx, `\get_split_vfo`)
	if err != nil {
		return false, "", err
	}
	return r.Value("Split") == "1", r.Value("TX VFO"), nil
}

func (c *Client) SetSplit(ctx context.Context, on bool, txVFO string) error {
	if !safeToken(txVFO) {
		return fmt.Errorf("rigctl: invalid vfo %q", txVFO)
	}
	v := 0
	if on {
		v = 1
	}
	_, err := c.Do(ctx, fmt.Sprintf(`\set_split_vfo %d %s`, v, txVFO))
	return err
}

func (c *Client) GetSplitFreq(ctx context.Context) (int64, error) {
	r, err := c.Do(ctx, `\get_split_freq`)
	if err != nil {
		return 0, err
	}
	f, _ := strconv.ParseFloat(r.Value("TX Frequency"), 64)
	return int64(f), nil
}

func (c *Client) SetSplitFreq(ctx context.Context, hz int64) error {
	_, err := c.Do(ctx, fmt.Sprintf(`\set_split_freq %d`, hz))
	return err
}

func (c *Client) GetLevel(ctx context.Context, name string) (float64, error) {
	if !safeToken(name) {
		return 0, fmt.Errorf("rigctl: invalid level %q", name)
	}
	r, err := c.Do(ctx, `\get_level `+name)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(r.Bare()), 64)
}

func (c *Client) SetLevel(ctx context.Context, name string, v float64) error {
	if !safeToken(name) {
		return fmt.Errorf("rigctl: invalid level %q", name)
	}
	_, err := c.Do(ctx, fmt.Sprintf(`\set_level %s %s`, name, strconv.FormatFloat(v, 'f', -1, 64)))
	return err
}

func (c *Client) GetFunc(ctx context.Context, name string) (bool, error) {
	if !safeToken(name) {
		return false, fmt.Errorf("rigctl: invalid function %q", name)
	}
	r, err := c.Do(ctx, `\get_func `+name)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(r.Bare()) != "0", nil
}

func (c *Client) SetFunc(ctx context.Context, name string, on bool) error {
	if !safeToken(name) {
		return fmt.Errorf("rigctl: invalid function %q", name)
	}
	v := 0
	if on {
		v = 1
	}
	_, err := c.Do(ctx, fmt.Sprintf(`\set_func %s %d`, name, v))
	return err
}

func (c *Client) GetRIT(ctx context.Context) (int, error) {
	r, err := c.Do(ctx, `\get_rit`)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(r.Value("RIT"))
}

func (c *Client) SetRIT(ctx context.Context, hz int) error {
	_, err := c.Do(ctx, fmt.Sprintf(`\set_rit %d`, hz))
	return err
}

func (c *Client) GetXIT(ctx context.Context) (int, error) {
	r, err := c.Do(ctx, `\get_xit`)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(r.Value("XIT"))
}

func (c *Client) SetXIT(ctx context.Context, hz int) error {
	_, err := c.Do(ctx, fmt.Sprintf(`\set_xit %d`, hz))
	return err
}

// VFOOp runs a vfo_op such as CPY, XCHG, UP, DOWN, BAND_UP, TUNE.
func (c *Client) VFOOp(ctx context.Context, op string) error {
	if !safeToken(op) {
		return fmt.Errorf("rigctl: invalid vfo op %q", op)
	}
	_, err := c.Do(ctx, `\vfo_op `+op)
	return err
}

// safeToken guards against command injection: rigctld takes whitespace-separated
// arguments, so reject anything that could smuggle in a second argument or command.
func safeToken(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// CTCSS tone and squelch are in tenths of Hz (885 = 88.5 Hz); 0 means off.

func (c *Client) GetCTCSSTone(ctx context.Context) (int, error) {
	r, err := c.Do(ctx, `\get_ctcss_tone`)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(r.Value("CTCSS Tone"))
}

func (c *Client) SetCTCSSTone(ctx context.Context, tenths int) error {
	_, err := c.Do(ctx, fmt.Sprintf(`\set_ctcss_tone %d`, tenths))
	return err
}

func (c *Client) GetCTCSSSql(ctx context.Context) (int, error) {
	r, err := c.Do(ctx, `\get_ctcss_sql`)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(r.Value("CTCSS Squelch"))
}

func (c *Client) SetCTCSSSql(ctx context.Context, tenths int) error {
	_, err := c.Do(ctx, fmt.Sprintf(`\set_ctcss_sql %d`, tenths))
	return err
}

// Repeater shift is "+", "-" or "None".

func (c *Client) GetRptrShift(ctx context.Context) (string, error) {
	r, err := c.Do(ctx, `\get_rptr_shift`)
	if err != nil {
		return "", err
	}
	return r.Value("Repeater Shift"), nil
}

func (c *Client) SetRptrShift(ctx context.Context, shift string) error {
	switch shift {
	case "+", "-", "None":
	default:
		return fmt.Errorf("rigctl: invalid repeater shift %q", shift)
	}
	_, err := c.Do(ctx, `\set_rptr_shift `+shift)
	return err
}

func (c *Client) GetRptrOffs(ctx context.Context) (int, error) {
	r, err := c.Do(ctx, `\get_rptr_offs`)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(r.Value("Repeater Offset"))
}

func (c *Client) SetRptrOffs(ctx context.Context, hz int) error {
	_, err := c.Do(ctx, fmt.Sprintf(`\set_rptr_offs %d`, hz))
	return err
}

// GetParm reads a rig parameter as the raw text rigctld returns.
func (c *Client) GetParm(ctx context.Context, name string) (string, error) {
	if !safeToken(name) {
		return "", fmt.Errorf("rigctl: invalid parameter %q", name)
	}
	r, err := c.Do(ctx, `\get_parm `+name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Bare()), nil
}

// SetParm writes a parameter; value must be a plain number or token.
func (c *Client) SetParm(ctx context.Context, name, value string) error {
	if !safeToken(name) || !safeValue(value) {
		return fmt.Errorf("rigctl: invalid parameter %q=%q", name, value)
	}
	_, err := c.Do(ctx, fmt.Sprintf(`\set_parm %s %s`, name, value))
	return err
}

// safeValue allows numbers and simple tokens, never whitespace or control characters.
func safeValue(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-' || r == '+') {
			return false
		}
	}
	return true
}

// catChars is what a raw ASCII CAT command may contain: letters, digits and the ';' terminator.
// No spaces, so the command can't smuggle a second rigctld argument or command.
func catChars(s string) bool {
	if s == "" || len(s) > 64 || !strings.HasSuffix(s, ";") {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ';') {
			return false
		}
	}
	return true
}

// SendCAT sends one raw ASCII CAT command (Kenwood, Yaesu, Elecraft) through rigctld. With
// reply, it uses send_cmd and returns what the radio answered up to its ';' terminator. Without,
// it uses send_cmd_rx expecting no bytes, because a set gets no answer and send_cmd would wait
// out its timeout. Callers decide which commands may be sent; this only guards the syntax.
// It is never retried.
func (c *Client) SendCAT(ctx context.Context, cmd string, reply bool) (string, error) {
	if !catChars(cmd) {
		return "", fmt.Errorf("rigctl: invalid CAT command %q", cmd)
	}
	line := `\send_cmd ` + cmd
	if !reply {
		line = `\send_cmd_rx ` + cmd + ` 0`
	}
	rep, err := c.Do(ctx, line)
	if err != nil {
		return "", err
	}
	v, ok := rep.Fields["Reply"]
	if !ok {
		return "", fmt.Errorf("rigctl: no reply to %s", cmd)
	}
	return strings.TrimSpace(strings.ReplaceAll(v, "\x00", "")), nil
}
