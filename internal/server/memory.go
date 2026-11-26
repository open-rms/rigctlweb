package server

import "net/http"

// Memory channels (private UI routes). Reading is allowed on read-only rigs, since it changes
// nothing; tuning to a channel is the "recall_channel" WebSocket command, which Exec refuses on
// read-only rigs like any other change.

func (s *Server) handleGetMemories(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, 200, rg.Memories())
}

func (s *Server) handleReadMemories(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	var in struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := decode(w, r, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := rg.ReadMemories(in.From, in.To); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, rg.Memories())
}

func (s *Server) handleStopMemories(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	rg.StopMemoryRead()
	writeJSON(w, 200, rg.Memories())
}
