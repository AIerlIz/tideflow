package storage

import "time"

// ---- JSON file formats ----

// sourcesJSON is the on-disk format for data/sources.json.
type sourcesJSON struct {
	NextID int            `json:"next_id"`
	Items  []SourceRecord `json:"items"`
}

// trafficJSON is the on-disk format for data/traffic.json.
type trafficJSON struct {
	NextID int             `json:"next_id"`
	Items  []TrafficRecord `json:"items"`
}

// ---- Data records ----

// SourceRecord mirrors the download_sources table.
type SourceRecord struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	SourceType string    `json:"source_type"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TrafficRecord mirrors the traffic_records table.
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

// TrafficHistoryItem is used for the traffic history API response.
type TrafficHistoryItem struct {
	Start   string `json:"start"`
	End     string `json:"end"`
	Type    string `json:"type"`
	Bytes   int64  `json:"bytes"`
	Count   int    `json:"count"`
	Current bool   `json:"current"`
}
