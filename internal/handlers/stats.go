package handlers

import (
	"net/http"
	"strconv"

	"tideflow/internal/enforcer"
	"tideflow/internal/models"
	"tideflow/internal/storage"
)

// StatsHandler groups stats-related HTTP handlers.
type StatsHandler struct {
	Store  *storage.Store
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

	items := h.Store.TrafficHistory(days)
	result := make([]models.TrafficHistoryItem, len(items))
	for i, item := range items {
		result[i] = models.TrafficHistoryItem{
			Start:   item.Start,
			End:     item.End,
			Type:    item.Type,
			Bytes:   item.Bytes,
			Count:   item.Count,
			Current: item.Current,
		}
	}
	writeJSON(w, http.StatusOK, result)
}
