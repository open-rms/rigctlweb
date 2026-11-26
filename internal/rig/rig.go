// Package rig owns one rigctld connection: it probes capabilities, polls state at an
// adaptive rate, applies commands, and fans state changes out to subscribers.
package rig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// Meters are levels polled on the fast path; everything else in Levels is polled slowly.
var meterNames = map[string]bool{
	"STRENGTH": true, "RAWSTR": true, "SWR": true, "ALC": true, "COMP_METER": true,
	"RFPOWER_METER": true, "RFPOWER_METER_WATTS": true, "TEMP_METER": true,
	"ID_METER": true, "VD_METER": true,
}

// maxPollFailures is how many consecutive transport errors end a session.
const maxPollFailures = 4

var rxMeters = []string{"STRENGTH"}
var txMeters = []string{"RFPOWER_METER", "RFPOWER_METER_WATTS", "SWR", "ALC", "COMP_METER", "ID_METER", "VD_METER", "TEMP_METER"}

// State is the live, JSON-serialised view of a rig sent to the UI.
type hold struct {
	until time.Time
	fn    func(*State)
}

type State struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Connected bool               `json:"connected"`
	Error     string             `json:"error,omitempty"`
	Freq      int64              `json:"freq"`
	Mode      string             `json:"mode"`
	Passband  int                `json:"passband"`
	VFO       string             `json:"vfo"`
	Split     bool               `json:"split"`
	TXVFO     string             `json:"txVfo,omitempty"`
	TXFreq    int64              `json:"txFreq,omitempty"`
	PTT       bool               `json:"ptt"`
	RIT       int                `json:"rit"`
	XIT       int                `json:"xit"`
	CTCSS     int                `json:"ctcss"`    // tenths of Hz, 0 = off
	CTCSSSql  int                `json:"ctcssSql"` // tenths of Hz, 0 = off
	RptrShift string             `json:"rptrShift,omitempty"`
	RptrOffs  int                `json:"rptrOffs"`
	Parms     map[string]string  `json:"parms"`
	Levels    map[string]float64 `json:"levels"`
	Funcs     map[string]bool    `json:"funcs"`
	Updated   time.Time          `json:"updated"`
}

func (s State) clone() State {
	s.Parms = maps.Clone(s.Parms)
	s.Levels = maps.Clone(s.Levels)
	s.Funcs = maps.Clone(s.Funcs)
	return s
}

// Config describes one rig connection.
type Config struct {
	ID        string        `toml:"id" json:"id"`
	Name      string        `toml:"name" json:"name"`
	Addr      string        `toml:"addr" json:"addr"`          // rigctld host:port
	PollFast  time.Duration `toml:"poll_fast" json:"-"`        // default 250ms
	PollSlow  time.Duration `toml:"poll_slow" json:"-"`        // default 2s
	TXTimeout time.Duration `toml:"tx_timeout" json:"-"`       // hard PTT limit, default 3m; 0 = default
	ReadOnly  bool          `toml:"read_only" json:"readOnly"` // refuse all set commands
	// StateDir is where the "we keyed the radio" marker lives; empty disables the marker.
	StateDir string `toml:"-" json:"-"`
}

func (c *Config) defaults() {
	if c.PollFast <= 0 {
		c.PollFast = 250 * time.Millisecond
	}
	if c.PollSlow <= 0 {
		c.PollSlow = 2 * time.Second
	}
	if c.TXTimeout <= 0 {
		c.TXTimeout = 3 * time.Minute
	}
}

// Rig is a managed rigctld connection.
type Rig struct {
	cfg    Config
	client *rigctl.Client
	log    *slog.Logger

	mu       sync.RWMutex
	state    State
	caps     *rigctl.Caps
	skip     map[string]bool // commands the rig doesn't support; never retried
	subs     map[chan State]struct{}
	txTimer  *time.Timer
	pending  atomic.Int32         // user commands waiting or running; background polling yields to them
	bench    map[string]time.Time // reads paused until the given time, see try
	strikes  map[string]int
	holds    map[string]hold // recently confirmed changes, see patch
	lastTune time.Time       // last accepted TUNE cycle (it keys the transmitter)
	kick     chan struct{}   // wake the poller early after a command

	// Keying: whether this program keyed the transmitter (see keying.go).
	kmu       sync.Mutex
	held      bool
	keyedBy   uint64 // client connection that keyed it; 0 = none/unknown
	keyedAt   time.Time
	recovered bool

	mem memories // memory channels read so far (see memory.go)
}

// New creates a rig; call Run to start polling.
func New(cfg Config, log *slog.Logger) *Rig {
	cfg.defaults()
	if log == nil {
		log = slog.Default()
	}
	cl := rigctl.NewClient(cfg.Addr)
	cl.OnSlow = func(cmd string, wait, rtt time.Duration) {
		log.With("rig", cfg.ID).Warn("slow rigctld command", "cmd", cmd, "queued_ms", wait.Milliseconds(), "rtt_ms", rtt.Milliseconds())
	}
	return &Rig{
		cfg:     cfg,
		client:  cl,
		log:     log.With("rig", cfg.ID),
		skip:    map[string]bool{},
		strikes: map[string]int{},
		subs:    map[chan State]struct{}{},
		kick:    make(chan struct{}, 1),
		state: State{ID: cfg.ID, Name: cfg.Name,
			Levels: map[string]float64{}, Funcs: map[string]bool{}, Parms: map[string]string{}},
	}
}

func (r *Rig) Config() Config { return r.cfg }

// Snapshot returns a copy of the current state.
func (r *Rig) Snapshot() State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.clone()
}

// Caps returns the probed capabilities, or nil until the first successful probe.
func (r *Rig) Caps() *rigctl.Caps {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.caps
}

// Subscribe returns a channel of state updates (latest-wins; slow readers drop frames).
func (r *Rig) Subscribe() (<-chan State, func()) {
	ch := make(chan State, 1)
	r.mu.Lock()
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		delete(r.subs, ch)
		r.mu.Unlock()
	}
}

func (r *Rig) publish() {
	r.mu.RLock()
	snap := r.state.clone()
	for ch := range r.subs {
		select {
		case ch <- snap:
		default: // drop the stale frame, keep latest
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- snap:
			default:
			}
		}
	}
	r.mu.RUnlock()
}

// Run polls until ctx is done, reconnecting with backoff.
func (r *Rig) Run(ctx context.Context) {
	defer r.client.Close()
	backoff := time.Second
	for ctx.Err() == nil {
		err := r.session(ctx)
		if ctx.Err() != nil {
			return
		}
		r.setDisconnected(err)
		r.log.Warn("rigctld session ended", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func (r *Rig) setDisconnected(err error) {
	r.mu.Lock()
	r.state.Connected = false
	if err != nil {
		r.state.Error = err.Error()
	}
	r.state.Updated = time.Now()
	r.mu.Unlock()
	r.publish()
}

func (r *Rig) session(ctx context.Context) error {
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	caps, err := r.client.FetchCaps(pctx)
	cancel()
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	r.mu.Lock()
	r.caps = caps
	r.state.Connected = true
	r.state.Error = ""
	r.skip = map[string]bool{}
	r.bench = nil
	r.mu.Unlock()
	r.log.Info("connected", "model", caps.Model, "mfg", caps.Mfg)
	r.recoverKey(ctx)

	fast := time.NewTicker(r.cfg.PollFast)
	defer fast.Stop()
	tasks := r.slowTasks(caps)
	idx := 0
	fails := 0
	for {
		if err := r.poll(ctx, tasks, &idx); err != nil {
			// One stalled read on a flaky link or a busy rig shouldn't flip the UI to
			// "offline": redial and only give up after several consecutive failures.
			if fails++; fails >= maxPollFailures {
				return err
			}
			r.log.Debug("poll failed; retrying", "n", fails, "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		fails = 0
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-fast.C:
		case <-r.kick:
		}
	}
}

// Background reads that are slow or keep failing are benched for a while: one stalled read
// (a radio that never answers a given query) otherwise blocks every other command, including
// the user's, for as long as rigctld waits on it.
const (
	slowReadLimit = 800 * time.Millisecond
	benchFor      = 10 * time.Minute
)

// protected keys are the reads the UI can't work without; they are never benched.
func protectedRead(key string) bool {
	if key == "freq" || key == "mode" || key == "ptt" {
		return true
	}
	if n, ok := strings.CutPrefix(key, "lvl:"); ok {
		return meterNames[n]
	}
	return false
}

// try runs fn, disabling the command key permanently if the rig reports it unsupported and
// benching it temporarily if it is slow or failing. Returns a non-nil error only for
// transport failures that should drop the session.
func (r *Rig) try(key string, fn func() error) error {
	r.mu.RLock()
	skipped := r.skip[key]
	until := r.bench[key]
	r.mu.RUnlock()
	if skipped || time.Now().Before(until) {
		return nil
	}
	start := time.Now()
	err := fn()
	took := time.Since(start)

	if !protectedRead(key) {
		r.mu.Lock()
		if took > slowReadLimit || (err != nil && !rigctl.IsUnsupported(err)) {
			r.strikes[key]++
			if took > slowReadLimit || r.strikes[key] >= 2 {
				if r.bench == nil {
					r.bench = map[string]time.Time{}
				}
				r.bench[key] = time.Now().Add(benchFor)
				r.strikes[key] = 0
				r.log.Warn("background read is slow or failing; pausing it", "cmd", key, "took_ms", took.Milliseconds(), "for", benchFor)
			}
		} else if err == nil {
			r.strikes[key] = 0
		}
		r.mu.Unlock()
	}
	if err == nil {
		return nil
	}
	var re *rigctl.Error
	if errors.As(err, &re) {
		if rigctl.IsUnsupported(err) {
			r.mu.Lock()
			r.skip[key] = true
			r.mu.Unlock()
			r.log.Debug("command unsupported; disabled", "cmd", key, "err", err)
		}
		return nil // rig-level failure (timeout, busy...): keep going
	}
	return err
}

// slowBatch is how many background reads run per poll cycle. The radio's serial port is
// shared with every other rigctld client (pat, wsjtx, hamstack...), so the full refresh of
// levels and functions is spread over many short cycles instead of one long burst.
const slowBatch = 3

type slowTask func(ctx context.Context, next *State) error

func (r *Rig) poll(ctx context.Context, tasks []slowTask, idx *int) error {
	c := r.client
	caps := r.Caps()
	next := r.Snapshot()
	next.Updated = time.Now()
	next.Connected = true

	pctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if err := r.try("freq", func() error {
		f, err := c.GetFreq(pctx)
		if err == nil {
			next.Freq = f
		}
		return err
	}); err != nil {
		return err
	}
	if err := r.try("mode", func() error {
		m, pb, err := c.GetMode(pctx)
		if err == nil {
			next.Mode, next.Passband = m, pb
		}
		return err
	}); err != nil {
		return err
	}
	if caps.CanPTT {
		if err := r.try("ptt", func() error {
			p, err := c.GetPTT(pctx)
			if err == nil {
				next.PTT = p
				r.observePTT(p)
			}
			return err
		}); err != nil {
			return err
		}
	}

	names := rxMeters
	if next.PTT {
		names = txMeters
	}
	for _, n := range names {
		if !caps.HasLevel(n) {
			continue
		}
		if err := r.try("lvl:"+n, func() error {
			v, err := c.GetLevel(pctx, n)
			if err == nil {
				next.Levels[n] = v
			}
			return err
		}); err != nil {
			return err
		}
	}

	for i := 0; i < slowBatch && len(tasks) > 0 && r.pending.Load() == 0; i++ {
		t := tasks[*idx%len(tasks)]
		*idx++
		if err := t(pctx, &next); err != nil {
			return err
		}
	}

	r.commit(next)
	return nil
}

// slowTasks lists the background reads for this rig, one small task each.
func (r *Rig) slowTasks(caps *rigctl.Caps) []slowTask {
	c := r.client
	var out []slowTask
	out = append(out, func(ctx context.Context, next *State) error {
		return r.try("vfo", func() error {
			v, err := c.GetVFO(ctx)
			if err == nil {
				next.VFO = v
			}
			return err
		})
	})
	if caps.CanSplit {
		out = append(out, func(ctx context.Context, next *State) error {
			if err := r.try("split", func() error {
				on, tx, err := c.GetSplit(ctx)
				if err == nil {
					next.Split, next.TXVFO = on, tx
				}
				return err
			}); err != nil {
				return err
			}
			if !next.Split {
				return nil
			}
			return r.try("splitfreq", func() error {
				f, err := c.GetSplitFreq(ctx)
				if err == nil {
					next.TXFreq = f
				}
				return err
			})
		})
	}
	if caps.MaxRIT > 0 {
		out = append(out, func(ctx context.Context, next *State) error {
			return r.try("rit", func() error {
				v, err := c.GetRIT(ctx)
				if err == nil {
					next.RIT = v
				}
				return err
			})
		})
	}
	if caps.MaxXIT > 0 {
		out = append(out, func(ctx context.Context, next *State) error {
			return r.try("xit", func() error {
				v, err := c.GetXIT(ctx)
				if err == nil {
					next.XIT = v
				}
				return err
			})
		})
	}
	for _, l := range caps.Levels {
		if !l.Get || meterNames[l.Name] || l.Range == (rigctl.Range{}) {
			continue
		}
		name := l.Name
		out = append(out, func(ctx context.Context, next *State) error {
			return r.try("lvl:"+name, func() error {
				v, err := c.GetLevel(ctx, name)
				if err == nil {
					next.Levels[name] = v
				}
				return err
			})
		})
	}
	for _, l := range caps.ExtLevels {
		if l.Type == "BUTTON" || l.Type == "STRING" {
			continue
		}
		name := l.Name
		out = append(out, func(ctx context.Context, next *State) error {
			return r.try("lvl:"+name, func() error {
				v, err := c.GetLevel(ctx, name)
				if err == nil {
					next.Levels[name] = v
				}
				return err
			})
		})
	}
	for _, l := range caps.ExtFuncs {
		name := l.Name
		out = append(out, func(ctx context.Context, next *State) error {
			return r.try("fn:"+name, func() error {
				v, err := c.GetFunc(ctx, name)
				if err == nil {
					next.Funcs[name] = v
				}
				return err
			})
		})
	}
	if caps.Can["get CTCSS"] && len(caps.CTCSS) > 0 {
		out = append(out, func(ctx context.Context, next *State) error {
			if !isFM(next.Mode) {
				return nil
			}
			return r.try("ctcss", func() error {
				v, err := c.GetCTCSSTone(ctx)
				if err == nil {
					next.CTCSS = v
				}
				return err
			})
		})
	}
	if caps.Can["get CTCSS Squelch"] && len(caps.CTCSS) > 0 {
		out = append(out, func(ctx context.Context, next *State) error {
			if !isFM(next.Mode) {
				return nil
			}
			return r.try("ctcsssql", func() error {
				v, err := c.GetCTCSSSql(ctx)
				if err == nil {
					next.CTCSSSql = v
				}
				return err
			})
		})
	}
	if caps.Can["get Repeater Duplex"] {
		out = append(out, func(ctx context.Context, next *State) error {
			if !isFM(next.Mode) {
				return nil
			}
			return r.try("rptrshift", func() error {
				v, err := c.GetRptrShift(ctx)
				if err == nil {
					next.RptrShift = v
				}
				return err
			})
		})
	}
	if caps.Can["get Repeater Offset"] {
		out = append(out, func(ctx context.Context, next *State) error {
			if !isFM(next.Mode) {
				return nil
			}
			return r.try("rptroffs", func() error {
				v, err := c.GetRptrOffs(ctx)
				if err == nil {
					next.RptrOffs = v
				}
				return err
			})
		})
	}
	if caps.Can["get Param"] {
		for _, e := range caps.ExtParms {
			if e.Type == "BUTTON" || e.Type == "STRING" || e.Type == "BINARY" {
				continue
			}
			name := e.Name
			out = append(out, func(ctx context.Context, next *State) error {
				return r.try("parm:"+name, func() error {
					v, err := c.GetParm(ctx, name)
					if err == nil {
						next.Parms[name] = v
					}
					return err
				})
			})
		}
		for _, p := range caps.Parms {
			if !p.Get {
				continue
			}
			name := p.Name
			out = append(out, func(ctx context.Context, next *State) error {
				return r.try("parm:"+name, func() error {
					v, err := c.GetParm(ctx, name)
					if err == nil {
						next.Parms[name] = v
					}
					return err
				})
			})
		}
	}
	for _, f := range caps.GetFuncs {
		name := f
		out = append(out, func(ctx context.Context, next *State) error {
			return r.try("fn:"+name, func() error {
				v, err := c.GetFunc(ctx, name)
				if err == nil {
					next.Funcs[name] = v
				}
				return err
			})
		})
	}
	return out
}

func (r *Rig) commit(next State) {
	r.mu.Lock()
	now := time.Now()
	for k, h := range r.holds {
		if now.After(h.until) {
			delete(r.holds, k)
			continue
		}
		h.fn(&next)
	}
	changed := !sameState(r.state, next)
	r.state = next
	r.mu.Unlock()
	if changed {
		r.publish()
	}
}

func sameState(a, b State) bool {
	a.Updated, b.Updated = time.Time{}, time.Time{}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func (r *Rig) wake() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// isFM reports whether a mode is one of the FM family, where tones and repeater offsets apply.
func isFM(mode string) bool {
	switch mode {
	case "FM", "WFM", "FMN", "FM-D", "PKTFM", "PKTFMN", "DFM":
		return true
	}
	return false
}
