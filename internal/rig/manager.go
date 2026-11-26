package rig

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
)

// Manager runs a set of rigs and supports adding/removing them at runtime.
type Manager struct {
	log *slog.Logger
	ctx context.Context

	stateDir string // see Config.StateDir

	mu   sync.RWMutex
	rigs map[string]*managed
}

type managed struct {
	rig    *Rig
	cancel context.CancelFunc
	done   chan struct{}
}

func NewManager(ctx context.Context, log *slog.Logger) *Manager {
	return &Manager{log: log, ctx: ctx, rigs: map[string]*managed{}}
}

// SetStateDir sets where rigs keep their "we keyed the radio" markers. Call before Add.
func (m *Manager) SetStateDir(dir string) { m.stateDir = dir }

// Add starts a rig. IDs must be unique.
func (m *Manager) Add(cfg Config) error {
	if cfg.ID == "" || cfg.Addr == "" {
		return errors.New("rig needs an id and an address")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rigs[cfg.ID]; ok {
		return errors.New("rig id already exists: " + cfg.ID)
	}
	cfg.StateDir = m.stateDir
	r := New(cfg, m.log)
	ctx, cancel := context.WithCancel(m.ctx)
	mg := &managed{rig: r, cancel: cancel, done: make(chan struct{})}
	m.rigs[cfg.ID] = mg
	go func() {
		defer close(mg.done)
		r.Run(ctx)
	}()
	return nil
}

// Remove stops a rig (unkeying it first) and waits for it to exit.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	mg, ok := m.rigs[id]
	delete(m.rigs, id)
	m.mu.Unlock()
	if !ok {
		return errors.New("no such rig: " + id)
	}
	mg.rig.StopMemoryRead()
	mg.rig.Unkey(context.Background())
	mg.cancel()
	<-mg.done
	return nil
}

func (m *Manager) Get(id string) (*Rig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mg, ok := m.rigs[id]
	if !ok {
		return nil, false
	}
	return mg.rig, true
}

// List returns rigs sorted by name.
func (m *Manager) List() []*Rig {
	m.mu.RLock()
	out := make([]*Rig, 0, len(m.rigs))
	for _, mg := range m.rigs {
		out = append(out, mg.rig)
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].cfg.Name < out[j].cfg.Name })
	return out
}

// UnkeyAll releases every rig this program is keying, concurrently, until ctx ends. Callers
// pass a context that a termination signal does not cancel, so the radio is dropped first.
func (m *Manager) UnkeyAll(ctx context.Context) {
	m.mu.RLock()
	rigs := make([]*Rig, 0, len(m.rigs))
	for _, mg := range m.rigs {
		rigs = append(rigs, mg.rig)
	}
	m.mu.RUnlock()
	var wg sync.WaitGroup
	for _, r := range rigs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Unkey(ctx)
		}()
	}
	wg.Wait()
}

// Shutdown unkeys every rig and waits for pollers to stop.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	all := m.rigs
	m.rigs = map[string]*managed{}
	m.mu.Unlock()
	for _, mg := range all {
		mg.rig.StopMemoryRead()
		mg.rig.Unkey(ctx)
		mg.cancel()
	}
	for _, mg := range all {
		<-mg.done
	}
}
