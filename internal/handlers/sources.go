package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tideflow/internal/enforcer"
	"tideflow/internal/models"

	"github.com/go-chi/chi/v5"
)

// SourcesHandler groups source-related HTTP handlers.
type SourcesHandler struct {
	DB     *sql.DB
	Engine *enforcer.Engine
}

// ListSources returns all download sources with runtime status.
func (h *SourcesHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query("SELECT id, name, url, source_type, enabled, created_at, updated_at FROM download_sources ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []models.SourceResponse
	for rows.Next() {
		var s models.DownloadSource
		if err := rows.Scan(&s.ID, &s.Name, &s.URL, &s.SourceType, &s.Enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := models.SourceResponse{DownloadSource: s}

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

	res, err := h.DB.Exec(
		"INSERT INTO download_sources (name, url, source_type, enabled) VALUES (?, ?, ?, ?)",
		body.Name, body.URL, body.SourceType, body.Enabled,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, _ := res.LastInsertId()

	var s models.DownloadSource
	h.DB.QueryRow(
		"SELECT id, name, url, source_type, enabled, created_at, updated_at FROM download_sources WHERE id = ?", id,
	).Scan(&s.ID, &s.Name, &s.URL, &s.SourceType, &s.Enabled, &s.CreatedAt, &s.UpdatedAt)

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

	// check exists
	var s models.DownloadSource
	err = h.DB.QueryRow(
		"SELECT id, name, url, source_type, enabled, created_at, updated_at FROM download_sources WHERE id = ?", sid,
	).Scan(&s.ID, &s.Name, &s.URL, &s.SourceType, &s.Enabled, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"detail": "源不存在"})
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if body.Name != nil {
		s.Name = *body.Name
	}
	if body.URL != nil {
		s.URL = *body.URL
	}
	if body.SourceType != nil {
		s.SourceType = *body.SourceType
	}
	if body.Enabled != nil {
		s.Enabled = *body.Enabled
	}

	_, err = h.DB.Exec(
		"UPDATE download_sources SET name=?, url=?, source_type=?, enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
		s.Name, s.URL, s.SourceType, s.Enabled, sid,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// re-read to get updated timestamp
	h.DB.QueryRow(
		"SELECT id, name, url, source_type, enabled, created_at, updated_at FROM download_sources WHERE id = ?", sid,
	).Scan(&s.ID, &s.Name, &s.URL, &s.SourceType, &s.Enabled, &s.CreatedAt, &s.UpdatedAt)

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

	res, err := h.DB.Exec("DELETE FROM download_sources WHERE id = ?", sid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
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
