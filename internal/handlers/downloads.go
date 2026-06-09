package handlers

import (
	"net/http"

	"tideflow/internal/enforcer"
)

// DownloadsHandler holds a reference to the enforcer engine.
type DownloadsHandler struct {
	Engine *enforcer.Engine
}

// HandlePause stops all active downloads.
func (h *DownloadsHandler) HandlePause(w http.ResponseWriter, r *http.Request) {
	h.Engine.StopAll()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleResume does nothing — the enforcer auto-fills on the next tick.
func (h *DownloadsHandler) HandleResume(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
