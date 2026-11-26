// Package rigctl is a small client for Hamlib's rigctld network protocol.
//
// It uses the "extended response" protocol (commands prefixed with '+'), which
// echoes the command and ends every reply with "RPRT <code>", making replies
// unambiguous to parse on a persistent connection.
package rigctl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Error is a non-zero RPRT result from rigctld.
type Error struct {
	Code int
	Cmd  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("rigctld: %s: %s (RPRT %d)", e.Cmd, hamlibErr(e.Code), e.Code)
}

// Hamlib error codes (the negated RPRT values), numbered exactly as Hamlib's rig.h
// (enum rig_errcode_e). A test checks every one against rig.h when it is installed.
const (
	ErrInval      = 1  // RIG_EINVAL: invalid parameter
	ErrConf       = 2  // RIG_ECONF: invalid configuration (serial, ...)
	ErrNoMem      = 3  // RIG_ENOMEM: memory shortage
	ErrNotImpl    = 4  // RIG_ENIMPL: function not implemented
	ErrTimeout    = 5  // RIG_ETIMEOUT: communication timed out
	ErrIO         = 6  // RIG_EIO: I/O error, including open failed
	ErrInternal   = 7  // RIG_EINTERNAL: internal Hamlib error
	ErrProto      = 8  // RIG_EPROTO: protocol error
	ErrReject     = 9  // RIG_ERJCTED: command rejected by the rig
	ErrTrunc      = 10 // RIG_ETRUNC: command performed, but the argument was truncated
	ErrNotAvail   = 11 // RIG_ENAVAIL: function not available
	ErrNotTarget  = 12 // RIG_ENTARGET: VFO not targetable
	ErrBusError   = 13 // RIG_BUSERROR: error talking on the bus
	ErrBusBusy    = 14 // RIG_BUSBUSY: collision on the bus
	ErrArg        = 15 // RIG_EARG: NULL handle or invalid pointer argument
	ErrVfo        = 16 // RIG_EVFO: invalid VFO
	ErrDomain     = 17 // RIG_EDOM: argument out of domain
	ErrDeprecated = 18 // RIG_EDEPRECATED: function deprecated
	ErrSecurity   = 19 // RIG_ESECURITY: security error
	ErrPower      = 20 // RIG_EPOWER: rig not powered on
	ErrLimit      = 21 // RIG_ELIMIT: limit exceeded
	ErrAccess     = 22 // RIG_EACCESS: access denied, e.g. port already in use
)

// hamlibText describes each code in plain words for messages shown to people.
var hamlibText = map[int]string{
	ErrInval:      "invalid parameter",
	ErrConf:       "invalid configuration",
	ErrNoMem:      "out of memory",
	ErrNotImpl:    "not implemented",
	ErrTimeout:    "timeout",
	ErrIO:         "I/O error",
	ErrInternal:   "internal Hamlib error",
	ErrProto:      "protocol error",
	ErrReject:     "command rejected by the rig",
	ErrTrunc:      "performed, but the value was truncated",
	ErrNotAvail:   "not available on this rig",
	ErrNotTarget:  "VFO not targetable",
	ErrBusError:   "bus error",
	ErrBusBusy:    "bus busy",
	ErrArg:        "invalid argument",
	ErrVfo:        "invalid VFO",
	ErrDomain:     "value out of range",
	ErrDeprecated: "deprecated function",
	ErrSecurity:   "security error",
	ErrPower:      "rig not powered on",
	ErrLimit:      "limit exceeded",
	ErrAccess:     "access denied (is the port in use?)",
}

func hamlibErr(code int) string {
	if t, ok := hamlibText[-code]; ok {
		return t
	}
	return "error"
}

// IsUnsupported reports whether err means this rig or backend can't do the command, so it
// is pointless to ask again: not implemented, not available, VFO not targetable, invalid
// parameter, invalid VFO, or out of domain. These are the parts of Hamlib's own "soft error"
// set that mean "never going to work".
//
// Deliberately not included: a rejection (ERJCTED, which can be transient, such as a control
// that only exists in some modes; the poller's strike-and-bench logic deals with those),
// truncation (the command worked), and bus busy or bus error (transient).
func IsUnsupported(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		switch -e.Code {
		case ErrNotImpl, ErrNotAvail, ErrNotTarget, ErrInval, ErrVfo, ErrDomain:
			return true
		}
	}
	return false
}

// Reply is one parsed rigctld response.
type Reply struct {
	Cmd    string            // echoed command name, e.g. "get_freq"
	Fields map[string]string // "Key: value" lines (keys as sent by rigctld)
	Lines  []string          // all body lines, in order (including key:value ones)
}

// Value returns the first field value, or the first bare line (e.g. a level).
func (r *Reply) Value(key string) string {
	if v, ok := r.Fields[key]; ok {
		return v
	}
	return ""
}

// Bare returns the single un-keyed body line (as used by get_level / get_func).
func (r *Reply) Bare() string {
	if len(r.Lines) == 0 {
		return ""
	}
	return r.Lines[0]
}

// Client is a single persistent rigctld connection. It is safe for concurrent use;
// commands are serialised.
type Client struct {
	Addr    string
	Timeout time.Duration
	// OnSlow, if set, is called when a command's lock wait or round trip is slow.
	OnSlow func(cmd string, wait, rtt time.Duration)

	mu   sync.Mutex
	conn net.Conn
	rd   *bufio.Reader
}

// NewClient returns a client for host:port. It connects lazily.
func NewClient(addr string) *Client {
	return &Client{Addr: addr, Timeout: 3 * time.Second}
}

func (c *Client) connect(ctx context.Context) error {
	d := net.Dialer{Timeout: c.Timeout}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetKeepAlive(true)
	}
	c.conn = conn
	c.rd = bufio.NewReaderSize(conn, 64*1024)
	return nil
}

// Close drops the connection; the next command reconnects.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn, c.rd = nil, nil
	return err
}

// Do sends one command (without the leading '+'; backslash form like `\get_freq` or
// short form like `f` both work) and returns the parsed reply. A non-zero RPRT is
// returned as *Error. Transport errors close the connection so the next call redials.
func (c *Client) Do(ctx context.Context, cmd string) (*Reply, error) {
	rep, err, doa := c.do(ctx, cmd)
	var re *Error
	for attempt := 0; err != nil && !errors.As(err, &re) && ctx.Err() == nil && attempt < 3; attempt++ {
		switch {
		case doa && safeToRepeat(cmd):
			// Hamlib's rigctld sometimes accepts a connection that arrives within milliseconds
			// of another client disconnecting and then closes it without reading anything.
			// Nothing ran, so redial after a short pause (measured: ~13% of rapid reconnects).
		case attempt == 0 && isRead(cmd):
			// A stale connection (rigctld restart, idle timeout) fails the first read.
		default:
			return rep, err
		}
		time.Sleep([]time.Duration{25, 75, 200}[attempt] * time.Millisecond)
		rep, err, doa = c.do(ctx, cmd)
	}
	return rep, err
}

// isRead reports whether cmd only reads state, so repeating it is harmless.
func isRead(cmd string) bool {
	return strings.HasPrefix(cmd, `\get_`) || cmd == `\dump_caps` || cmd == `\chk_vfo`
}

// safeToRepeat reports whether cmd may be resent after a connection that was dead on
// arrival. Anything that triggers an action rather than setting a value is excluded.
func safeToRepeat(cmd string) bool {
	for _, p := range []string{`\vfo_op`, `\send_`, `\scan`, `\reset`, `\power`, `\set_powerstat`, `\stop_`, `\wait_`} {
		if strings.HasPrefix(cmd, p) {
			return false
		}
	}
	return true
}

// do runs one attempt. doa is true when a freshly dialed connection failed before returning
// any reply bytes, i.e. the command was almost certainly never executed.
func (c *Client) do(ctx context.Context, cmd string) (rep *Reply, err error, doa bool) {
	if strings.ContainsAny(cmd, "\r\n") {
		return nil, errors.New("rigctl: command contains newline"), false
	}
	queued := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	wait := time.Since(queued)
	fresh := false
	if c.conn == nil {
		if err := c.connect(ctx); err != nil {
			return nil, err, false
		}
		fresh = true
	}
	started := time.Now()
	defer func() {
		if rtt := time.Since(started); c.OnSlow != nil && (wait > 300*time.Millisecond || rtt > 300*time.Millisecond) {
			c.OnSlow(cmd, wait, rtt)
		}
	}()
	deadline := time.Now().Add(c.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.conn.SetDeadline(deadline)
	if _, err := c.conn.Write([]byte("+" + cmd + "\n")); err != nil {
		_ = c.closeLocked()
		return nil, err, fresh
	}
	rep, got, err := readReply(c.rd, cmd)
	if err != nil {
		var re *Error
		if !errors.As(err, &re) {
			_ = c.closeLocked()
			// EOF or reset before a single byte came back on a connection we just opened.
			doa = fresh && !got && (errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET))
		}
		return rep, err, doa
	}
	return rep, nil, false
}

func readReply(rd *bufio.Reader, cmd string) (rep *Reply, got bool, err error) {
	rep = &Reply{Fields: map[string]string{}}
	first := true
	for {
		line, rerr := rd.ReadString('\n')
		if rerr != nil {
			return nil, got || len(line) > 0, rerr
		}
		got = true
		line = strings.TrimRight(line, "\r\n")
		// send_cmd puts the radio's reply and the RPRT on one line, separated by a NUL:
		// "Reply: EX010413050;\x00RPRT 0". With no answer from the radio: "Reply: RPRT 0".
		if rest, ok := strings.CutPrefix(line, "Reply: RPRT "); ok && !strings.Contains(line, "\x00") {
			line = "Reply: \x00RPRT " + rest
		}
		if body, tail, ok := strings.Cut(line, "\x00"); ok {
			tail = strings.ReplaceAll(tail, "\x00", "")
			if !strings.HasPrefix(tail, "RPRT ") {
				return nil, got, fmt.Errorf("rigctl: unexpected reply %q", line)
			}
			rep.Lines = append(rep.Lines, body)
			if k, v, ok := strings.Cut(body, ": "); ok {
				rep.Fields[k] = v
			} else if k, ok := strings.CutSuffix(body, ":"); ok {
				rep.Fields[k] = ""
			}
			line = tail
		}
		if rest, ok := strings.CutPrefix(line, "RPRT "); ok {
			code, perr := strconv.Atoi(strings.TrimSpace(rest))
			if perr != nil {
				return nil, got, fmt.Errorf("rigctl: bad RPRT line %q", line)
			}
			if code != 0 {
				return rep, got, &Error{Code: code, Cmd: cmd}
			}
			return rep, got, nil
		}
		if first {
			first = false
			// Echo line looks like "get_freq:" or "get_level: STRENGTH".
			if name, _, ok := strings.Cut(line, ":"); ok && !strings.Contains(name, " ") {
				rep.Cmd = name
				continue
			}
		}
		rep.Lines = append(rep.Lines, line)
		if k, v, ok := strings.Cut(line, ": "); ok {
			if _, dup := rep.Fields[k]; !dup {
				rep.Fields[k] = v
			}
		} else if k, ok := strings.CutSuffix(line, ":"); ok {
			rep.Fields[k] = ""
		}
	}
}

// Raw sends a command and returns every body line (for dump_caps and similar).
func (c *Client) Raw(ctx context.Context, cmd string) ([]string, error) {
	rep, err := c.Do(ctx, cmd)
	if rep == nil {
		return nil, err
	}
	return rep.Lines, err
}
