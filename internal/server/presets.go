package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/open-rms/rigctlweb/internal/presets"
	"github.com/open-rms/rigctlweb/internal/rig"
	"github.com/open-rms/rigctlweb/internal/rigctl"
)

// openRMS Rigs presets (private UI routes). Browsing and checking work on read-only radios,
// since they change nothing; applying and restoring are refused there.

// SetPresets turns on the presets routes.
func (s *Server) SetPresets(p *presets.Service) { s.presets = p }

func (s *Server) presetsOn(w http.ResponseWriter, r *http.Request) bool {
	if s.presets == nil {
		http.NotFound(w, r)
		return false
	}
	return true
}

// capsFor returns the selected radio's capabilities, or nil when none is chosen or connected.
func (s *Server) capsFor(id string) *rigctl.Caps {
	if id == "" {
		return nil
	}
	if rg, ok := s.mgr.Get(id); ok {
		return rg.Caps()
	}
	return nil
}

func (s *Server) handlePresetList(w http.ResponseWriter, r *http.Request) {
	if !s.presetsOn(w, r) {
		return
	}
	caps := s.capsFor(r.URL.Query().Get("rig"))
	out := []presets.Summary{}
	for _, e := range s.presets.Lib.Profiles() {
		out = append(out, presets.Summarize(e, caps))
	}
	var radio any
	if caps != nil {
		radio = map[string]string{"mfg": caps.Mfg, "model": caps.Model}
	}
	writeJSON(w, 200, map[string]any{"library": s.presets.Lib.Status(), "radio": radio, "profiles": out})
}

func (s *Server) handlePresetUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.presetsOn(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := s.presets.Lib.Update(ctx); err != nil {
		s.log.Warn("presets library update failed", "err", err)
		writeJSON(w, http.StatusBadGateway, s.presets.Lib.Status())
		return
	}
	writeJSON(w, 200, s.presets.Lib.Status())
}

func (s *Server) handlePresetGet(w http.ResponseWriter, r *http.Request) {
	if !s.presetsOn(w, r) {
		return
	}
	e, ok := s.presets.Lib.Get(r.PathValue("pid"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, 200, presets.Plan(e, s.capsFor(r.URL.Query().Get("rig"))))
}

func presetErr(w http.ResponseWriter, err error) {
	code := http.StatusConflict
	switch {
	case errors.Is(err, presets.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, rig.ErrReadOnly):
		code = http.StatusForbidden
	case errors.Is(err, rig.ErrNotConnected):
		code = http.StatusServiceUnavailable
	}
	http.Error(w, err.Error(), code)
}

func (s *Server) presetRig(w http.ResponseWriter, r *http.Request) (*rig.Rig, bool) {
	if !s.presetsOn(w, r) {
		return nil, false
	}
	rg, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
	}
	return rg, ok
}

func (s *Server) handlePresetCheck(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.presetRig(w, r)
	if !ok {
		return
	}
	res, err := s.presets.Check(r.Context(), rg, r.PathValue("pid"))
	if err != nil {
		presetErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handlePresetApply(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.presetRig(w, r)
	if !ok {
		return
	}
	var in struct {
		Steps []string `json:"steps"`
		// Cat is the exact raw CAT commands the operator confirmed; it must match what would be sent.
		Cat []string `json:"cat"`
	}
	if err := decode(w, r, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, err := s.presets.Apply(r.Context(), rg, r.PathValue("pid"), in.Steps, in.Cat)
	if err != nil {
		presetErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handlePresetSnapshot(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.presetRig(w, r)
	if !ok {
		return
	}
	sn, err := s.presets.LoadSnapshot(rg.Config().ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, 200, map[string]any{"snapshot": sn})
}

func (s *Server) handlePresetRestore(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.presetRig(w, r)
	if !ok {
		return
	}
	res, err := s.presets.Restore(r.Context(), rg)
	if err != nil {
		presetErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}
