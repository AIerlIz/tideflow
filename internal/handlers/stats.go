package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"tideflow/internal/enforcer"
	"tideflow/internal/models"
)

// StatsHandler groups stats-related HTTP handlers.
type StatsHandler struct {
	DB     *sql.DB
	Engine *enforcer.Engine
}

// HandleStats returns the current enforcer state.
func (h *StatsHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	e := h.Engine

	capEnabled := e.GetSetting("traffic_cap_enabled")
	capStr := e.GetSetting("traffic_cap_bytes")
	cap, _ := strconv.ParseInt(capStr, 10, 64)
	if capEnabled != "true" {
		cap = 0
	}

	e.Mu().RLock()
	traffic := e.TrafficThisPeriod
	if capEnabled == "true" && cap > 0 && traffic > cap {
		traffic = cap
	}

	streamBytes := make(map[int]int64)
	tasks := make([]models.TaskInfo, 0)
	for key, sid := range e.TaskSource {
		b := e.StreamBytes[key]
		streamBytes[sid] = streamBytes[sid] + b
		tasks = append(tasks, models.TaskInfo{
			Key:      key,
			SourceID: sid,
			Bytes:    b,
		})
	}

	resp := models.StatsResponse{
		Speed:         e.DownloadSpeed,
		Traffic:       traffic,
		TrafficCap:    cap,
		CapEnabled:    capEnabled == "true",
		StreamCount:   len(e.TaskSource),
		StreamBytes:   streamBytes,
		Tasks:         tasks,
		MaxConcurrent: e.GetSetting("max_concurrent"),
		InWindow:      e.InWindow,
		WindowEnabled: e.GetSetting("time_window_enabled") == "true",
		WindowStart:   e.GetSetting("time_window_start"),
		WindowEnd:     e.GetSetting("time_window_end"),
		PausedCap:     e.PausedByCap,
		PausedWindow:  e.PausedByWindow,
		CooldownIDs:   e.CooldownIDs,
		AllFailed:     e.AllFailed,
	}
	e.Mu().RUnlock()

	writeJSON(w, http.StatusOK, resp)
}

// HandleTrafficHistory returns historical traffic records.
func (h *StatsHandler) HandleTrafficHistory(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d >= 1 && d <= 365 {
			days = d
		}
	}

	rows, err := h.DB.Query(
		"SELECT period_start, period_end, period_type, total_bytes, download_count, is_current FROM traffic_records ORDER BY period_start DESC LIMIT ?",
		days,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []models.TrafficHistoryItem
	for rows.Next() {
		var item models.TrafficHistoryItem
		var start, end any
		if err := rows.Scan(&start, &end, &item.Type, &item.Bytes, &item.Count, &item.Current); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Format times as ISO 8601 strings (matching Python's .isoformat())
		item.Start = formatTime(start)
		item.End = formatTime(end)
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}
