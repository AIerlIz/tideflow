package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tideflow/internal/enforcer"
	"tideflow/internal/models"
	"tideflow/internal/storage"

	"github.com/go-chi/chi/v5"
)

// SourcesHandler groups source-related HTTP handlers.
type SourcesHandler struct {
	Store  *storage.Store
	Engine *enforcer.Engine
}

// ListSources returns all download sources with runtime status.
func (h *SourcesHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	sources := h.Store.ListSources()

	var result []models.SourceResponse
	for _, s := range sources {
		resp := models.SourceResponse{DownloadSource: models.DownloadSource{
			ID:         s.ID,
			Name:       s.Name,
			URL:        s.URL,
			SourceType: s.SourceType,
			Enabled:    s.Enabled,
			CreatedAt:  s.CreatedAt,
			UpdatedAt:  s.UpdatedAt,
		}, TotalBytes: s.TotalBytes, MaxSpeed: s.MaxSpeed}

		// Aggregate bytes and check if downloading
		h.Engine.Mu().RLock()
		for key, sid := range h.Engine.TaskSource {
			if sid == s.ID {
				resp.Bytes += h.Engine.StreamBytes[key]
			}
		}
		resp.Downloading = resp.Bytes > 0
		for _, sid := range h.Engine.TaskSource {
			if sid == s.ID {
				resp.Downloading = true
				break
			}
		}
		for _, cid := range h.Engine.CooldownIDs {
			if cid == s.ID {
				resp.InCooldown = true
				break
			}
		}
		h.Engine.Mu().RUnlock()

		result = append(result, resp)
	}
	writeJSON(w, http.StatusOK, result)
}

// TestURL sends a HEAD request to verify a URL is reachable.
func (h *SourcesHandler) TestURL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "URL 不能为空"})
		return
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}
	resp, err := client.Head(body.URL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	size := resp.Header.Get("Content-Length")
	if size == "" {
		size = "?"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"status": resp.StatusCode,
		"size":   size,
	})
}

// CreateSource adds a new download source.
func (h *SourcesHandler) CreateSource(w http.ResponseWriter, r *http.Request) {
	var body models.SourceCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.SourceType == "" {
		body.SourceType = "http"
	}
	if h.Store.URLExists(body.URL) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "该 URL 已存在"})
		return
	}

	rec := h.Store.CreateSource(body.Name, body.URL, body.SourceType, body.Enabled, body.Headers, body.MaxSpeed)
	s := models.DownloadSource{
		ID:         rec.ID,
		Name:       rec.Name,
		URL:        rec.URL,
		SourceType: rec.SourceType,
		Enabled:    rec.Enabled,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
	}
	writeJSON(w, http.StatusOK, s)
}

// UpdateSource modifies an existing download source.
func (h *SourcesHandler) UpdateSource(w http.ResponseWriter, r *http.Request) {
	sidStr := chi.URLParam(r, "id")
	sid, err := strconv.Atoi(sidStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var body models.SourceUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	rec, found := h.Store.UpdateSource(sid, body.Name, body.URL, body.SourceType, body.Enabled, body.Headers, body.MaxSpeed)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"detail": "源不存在"})
		return
	}

	s := models.DownloadSource{
		ID:         rec.ID,
		Name:       rec.Name,
		URL:        rec.URL,
		SourceType: rec.SourceType,
		Enabled:    rec.Enabled,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
	}
	writeJSON(w, http.StatusOK, s)
}

// DeleteSource removes a download source.
func (h *SourcesHandler) DeleteSource(w http.ResponseWriter, r *http.Request) {
	sidStr := chi.URLParam(r, "id")
	sid, err := strconv.Atoi(sidStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	if !h.Store.DeleteSource(sid) {
		writeJSON(w, http.StatusNotFound, map[string]string{"detail": "源不存在"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ClearCooldowns resets all failure trackers.
func (h *SourcesHandler) ClearCooldowns(w http.ResponseWriter, r *http.Request) {
	h.Engine.ResetFailures()
	n := h.Engine.FailureCount()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "cleared": n})
}
