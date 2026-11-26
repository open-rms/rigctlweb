package rig

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// Memory channels, phase 1: read them and tune the VFO to one. Nothing here writes the
// radio's memories or switches it into memory mode: "recall" copies a channel's frequency,
// mode and repeater settings onto the current VFO through the ordinary validated commands.

// memoryBankTypes are the memory kinds that hold frequencies. VOICE and MORSE slots (and
// anything Hamlib adds later) are never listed: "using" them means transmitting.
var memoryBankTypes = map[string]bool{"MEM": true, "CALL": true, "EDGE": true, "BAND": true, "PRIO": true, "MEMOPAD": true}

// maxMemoryRead caps one read job, so a mistake cannot tie up a shared serial line for long.
const maxMemoryRead = 200

// memoryReadGap is the pause between channels, so the poller and other programs sharing
// rigctld keep getting turns.
var memoryReadGap = 150 * time.Millisecond

// MemoryBank is one range of memory channels the radio reports.
type MemoryBank struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Type  string `json:"type"`
}

// MemoryView is what the UI shows: the banks, what has been read so far, and the read job.
type MemoryView struct {
	Supported bool             `json:"supported"`
	Reason    string           `json:"reason,omitempty"`
	Banks     []MemoryBank     `json:"banks"`
	Channels  []rigctl.Channel `json:"channels"`
	Reading   bool             `json:"reading"`
	Done      int              `json:"done"`
	Total     int              `json:"total"`
	Error     string           `json:"error,omitempty"`
}

type memories struct {
	mu       sync.Mutex
	channels map[int]rigctl.Channel
	reading  bool
	done     int
	total    int
	err      string
	cancel   context.CancelFunc
}

// MemoryBanks lists the radio's frequency memory ranges, or why there are none.
func (r *Rig) MemoryBanks() ([]MemoryBank, string) {
	caps := r.Caps()
	switch {
	case caps == nil:
		return nil, "the radio is not connected yet"
	case !caps.Can["get Channel"]:
		return nil, "this radio's Hamlib backend cannot read memory channels"
	}
	var out []MemoryBank
	for _, m := range caps.Memories {
		if memoryBankTypes[m.Type] && m.End >= m.Start {
			out = append(out, MemoryBank{Start: m.Start, End: m.End, Type: m.Type})
		}
	}
	if len(out) == 0 {
		return nil, "this radio reports no memory channels"
	}
	return out, ""
}

func (r *Rig) inBank(n int) bool {
	banks, _ := r.MemoryBanks()
	for _, b := range banks {
		if n >= b.Start && n <= b.End {
			return true
		}
	}
	return false
}

// Memories returns the banks, the channels read so far and the state of any read job.
func (r *Rig) Memories() MemoryView {
	banks, reason := r.MemoryBanks()
	r.mem.mu.Lock()
	defer r.mem.mu.Unlock()
	v := MemoryView{Supported: reason == "", Reason: reason, Banks: banks, Channels: []rigctl.Channel{},
		Reading: r.mem.reading, Done: r.mem.done, Total: r.mem.total, Error: r.mem.err}
	if v.Banks == nil {
		v.Banks = []MemoryBank{}
	}
	for _, ch := range r.mem.channels {
		v.Channels = append(v.Channels, ch)
	}
	sort.Slice(v.Channels, func(i, j int) bool { return v.Channels[i].Number < v.Channels[j].Number })
	return v
}

// ReadMemories starts reading channels from..to in the background. It refuses while the radio
// is transmitting and stops if it starts.
func (r *Rig) ReadMemories(from, to int) error {
	if from > to {
		from, to = to, from
	}
	if to-from+1 > maxMemoryRead {
		return fmt.Errorf("read at most %d channels at a time", maxMemoryRead)
	}
	for n := from; n <= to; n++ {
		if !r.inBank(n) {
			return fmt.Errorf("channel %d is not a memory channel on this radio", n)
		}
	}
	if r.Snapshot().PTT {
		return errors.New("the radio is transmitting; try again when it is not")
	}
	r.mem.mu.Lock()
	if r.mem.reading {
		r.mem.mu.Unlock()
		return errors.New("already reading memories")
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.mem.reading, r.mem.done, r.mem.total, r.mem.err, r.mem.cancel = true, 0, to-from+1, "", cancel
	if r.mem.channels == nil {
		r.mem.channels = map[int]rigctl.Channel{}
	}
	r.mem.mu.Unlock()

	go func() {
		defer cancel()
		fail := ""
		for n := from; n <= to && fail == ""; n++ {
			if ctx.Err() != nil {
				fail = "stopped"
				break
			}
			if r.Snapshot().PTT {
				fail = "stopped: the radio started transmitting"
				break
			}
			cctx, c := context.WithTimeout(ctx, 5*time.Second)
			ch, err := r.client.GetChannel(cctx, n)
			c()
			r.mem.mu.Lock()
			switch {
			case err == nil:
				r.mem.channels[n] = ch
			case rigctl.IsUnsupported(err):
				fail = "this radio refused to read channel " + fmt.Sprint(n) + ": " + err.Error()
			default:
				fail = "reading channel " + fmt.Sprint(n) + " failed: " + err.Error()
			}
			r.mem.done++
			r.mem.mu.Unlock()
			select {
			case <-time.After(memoryReadGap):
			case <-ctx.Done():
			}
		}
		r.mem.mu.Lock()
		r.mem.reading, r.mem.err, r.mem.cancel = false, fail, nil
		r.mem.mu.Unlock()
	}()
	return nil
}

// StopMemoryRead cancels a running read job.
func (r *Rig) StopMemoryRead() {
	r.mem.mu.Lock()
	if r.mem.cancel != nil {
		r.mem.cancel()
	}
	r.mem.mu.Unlock()
}

// recallChannel tunes the current VFO to channel n: frequency and mode, then repeater shift,
// offset and tones when the channel has them and the radio can set them. It reads the channel
// fresh so the radio's own memory is the source of truth.
func (r *Rig) recallChannel(ctx context.Context, n int) error {
	if !r.inBank(n) {
		return fmt.Errorf("channel %d is not a memory channel on this radio", n)
	}
	ch, err := r.client.GetChannel(ctx, n)
	if err != nil {
		return err
	}
	r.mem.mu.Lock()
	if r.mem.channels == nil {
		r.mem.channels = map[int]rigctl.Channel{}
	}
	r.mem.channels[n] = ch
	r.mem.mu.Unlock()
	if ch.Empty {
		return fmt.Errorf("channel %d is empty", n)
	}
	if err := r.Exec(ctx, Command{Name: "set_freq", Args: map[string]any{"hz": float64(ch.FreqHz)}}); err != nil {
		return err
	}
	if ch.Mode != "" {
		args := map[string]any{"mode": ch.Mode}
		if ch.WidthHz > 0 {
			args["passband"] = float64(ch.WidthHz)
		}
		if err := r.Exec(ctx, Command{Name: "set_mode", Args: args}); err != nil {
			return fmt.Errorf("tuned to %d Hz, but the mode was not set: %w", ch.FreqHz, err)
		}
	}
	caps := r.Caps()
	// Repeater settings are best effort: the frequency and mode are what matter most.
	if caps != nil && ch.RptrShift != "" && caps.Can["set Repeater Duplex"] {
		_ = r.Exec(ctx, Command{Name: "set_rptr_shift", Args: map[string]any{"shift": ch.RptrShift}})
		if ch.RptrOffsHz > 0 {
			_ = r.Exec(ctx, Command{Name: "set_rptr_offs", Args: map[string]any{"hz": float64(ch.RptrOffsHz)}})
		}
	}
	if caps != nil && ch.CTCSS > 0 {
		_ = r.Exec(ctx, Command{Name: "set_ctcss_tone", Args: map[string]any{"tenths": float64(ch.CTCSS)}})
	}
	if caps != nil && ch.CTCSSSql > 0 {
		_ = r.Exec(ctx, Command{Name: "set_ctcss_sql", Args: map[string]any{"tenths": float64(ch.CTCSSSql)}})
	}
	return nil
}
