package models

import "time"

// ---- Database rows ----

type DownloadSource struct {
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	SourceType string            `json:"source_type"`
	Enabled    bool              `json:"enabled"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxSpeed   string            `json:"max_speed,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type GlobalSetting struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TrafficRecord struct {
	ID            int       `json:"id"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
	PeriodType    string    `json:"period_type"`
	TotalBytes    int64     `json:"total_bytes"`
	DownloadCount int       `json:"download_count"`
	IsCurrent     bool      `json:"is_current"`
	CreatedAt     time.Time `json:"created_at"`
}

// ---- API request / response ----

type SourceCreate struct {
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	SourceType string            `json:"source_type"`
	Enabled    bool              `json:"enabled"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxSpeed   string            `json:"max_speed,omitempty"`
}

type SourceUpdate struct {
	Name       *string           `json:"name,omitempty"`
	URL        *string           `json:"url,omitempty"`
	SourceType *string           `json:"source_type,omitempty"`
	Enabled    *bool             `json:"enabled,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxSpeed   *string           `json:"max_speed,omitempty"`
}

// SourceResponse extends DownloadSource with runtime fields.
type SourceResponse struct {
	DownloadSource
	Downloading bool   `json:"downloading"`
	Bytes       int64  `json:"bytes"`
	TotalBytes  int64  `json:"total_bytes"`
	InCooldown  bool   `json:"in_cooldown"`
	MaxSpeed    string `json:"max_speed,omitempty"`
}

// ---- Stats ----

type StatsResponse struct {
	Speed          int64            `json:"speed"`
	Traffic        int64            `json:"traffic"`
	TrafficCap     int64            `json:"traffic_cap"`
	CapEnabled     bool             `json:"cap_enabled"`
	StreamCount    int              `json:"stream_count"`
	StreamBytes    map[int]int64    `json:"stream_bytes"`
	Tasks          []TaskInfo       `json:"tasks"`
	MaxConcurrent  string           `json:"max_concurrent"`
	InWindow       bool             `json:"in_window"`
	WindowEnabled  bool             `json:"window_enabled"`
	WindowStart    string           `json:"window_start"`
	WindowEnd      string           `json:"window_end"`
	PausedCap      bool             `json:"paused_cap"`
	PausedWindow   bool             `json:"paused_window"`
	CooldownIDs    []int            `json:"cooldown_ids"`
	AllFailed      bool             `json:"all_failed"`
	AllTimeBytes   int64            `json:"all_time_bytes"`
}

type TaskInfo struct {
	Key      string `json:"key"`
	SourceID int    `json:"source_id"`
	Bytes    int64  `json:"bytes"`
}

type TrafficHistoryItem struct {
	Start   string `json:"start"`
	End     string `json:"end"`
	Type    string `json:"type"`
	Bytes   int64  `json:"bytes"`
	Count   int    `json:"count"`
	Current bool   `json:"current"`
}
