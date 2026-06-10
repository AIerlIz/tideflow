package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tideflow/internal/config"
)

// Store is a thread-safe, file-backed key/document store.
// All data is kept in memory and persisted to JSON files on every write.
type Store struct {
	mu      sync.RWMutex
	dataDir string

	// download sources
	sources      []SourceRecord
	nextSourceID int

	// global settings (key → value, all strings)
	settings map[string]string

	// traffic records
	traffic       []TrafficRecord
	nextTrafficID int
}

// New creates or opens a Store rooted at dataDir.
func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	s := &Store{
		dataDir:  dataDir,
		settings: make(map[string]string),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	// Seed default download sources on first run.
	if len(s.sources) == 0 {
		s.seedDefaults()
	}

	return s, nil
}

// ---- file paths ----

func (s *Store) sourcesPath() string  { return filepath.Join(s.dataDir, "sources.json") }
func (s *Store) settingsPath() string { return filepath.Join(s.dataDir, "settings.json") }
func (s *Store) trafficPath() string  { return filepath.Join(s.dataDir, "traffic.json") }

// ---- load / save ----

func (s *Store) load() error {
	s.loadJSON(s.sourcesPath(), &sourcesJSON{}, func(v any) {
		d := v.(*sourcesJSON)
		s.sources = d.Items
		s.nextSourceID = d.NextID
	})
	s.loadJSON(s.settingsPath(), &s.settings, func(v any) {
		s.settings = *(v.(*map[string]string))
	})
	s.loadJSON(s.trafficPath(), &trafficJSON{}, func(v any) {
		d := v.(*trafficJSON)
		s.traffic = d.Items
		s.nextTrafficID = d.NextID
	})
	return nil
}

func (s *Store) loadJSON(path string, target any, assign func(any)) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file missing is OK on first run
	}
	if err := json.Unmarshal(data, target); err != nil {
		log.Printf("storage: failed to parse %s (starting fresh): %v", path, err)
		return
	}
	assign(target)
}

func (s *Store) writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---- defaults ----

func (s *Store) seedDefaults() {
	defaults := []struct{ name, url, stype string }{
		{"100MB 测速文件", "http://speedtest.tele2.net/100MB.zip", "http"},
		{"10MB 测速文件", "http://speedtest.tele2.net/10MB.zip", "http"},
		{"1MB 测速文件", "http://speedtest.tele2.net/1MB.zip", "http"},
	}
	now := time.Now()
	for _, d := range defaults {
		s.nextSourceID++
		s.sources = append(s.sources, SourceRecord{
			ID:         s.nextSourceID,
			Name:       d.name,
			URL:        d.url,
			SourceType: d.stype,
			Enabled:    true,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	s.saveSources()
	log.Printf("Seeded %d default download sources", len(defaults))
}

func (s *Store) saveSources() {
	if err := s.writeJSON(s.sourcesPath(), &sourcesJSON{NextID: s.nextSourceID, Items: s.sources}); err != nil {
		log.Printf("storage: save sources: %v", err)
	}
}
func (s *Store) saveSettings() {
	if err := s.writeJSON(s.settingsPath(), s.settings); err != nil {
		log.Printf("storage: save settings: %v", err)
	}
}
func (s *Store) saveTraffic() {
	if err := s.writeJSON(s.trafficPath(), &trafficJSON{NextID: s.nextTrafficID, Items: s.traffic}); err != nil {
		log.Printf("storage: save traffic: %v", err)
	}
}

// ========================================================================
// Sources
// ========================================================================

// ListSources returns all download sources ordered by ID.
func (s *Store) ListSources() []SourceRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SourceRecord, len(s.sources))
	copy(out, s.sources)
	return out
}

// ListEnabledSources returns only enabled sources.
func (s *Store) ListEnabledSources() []SourceRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []SourceRecord
	for _, src := range s.sources {
		if src.Enabled {
			out = append(out, src)
		}
	}
	return out
}

// GetSource returns a single source by ID.
func (s *Store) GetSource(id int) (SourceRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, src := range s.sources {
		if src.ID == id {
			return src, true
		}
	}
	return SourceRecord{}, false
}

// CreateSource adds a new download source and returns it.
func (s *Store) CreateSource(name, url, sourceType string, enabled bool) SourceRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.nextSourceID++
	rec := SourceRecord{
		ID:         s.nextSourceID,
		Name:       name,
		URL:        url,
		SourceType: sourceType,
		Enabled:    enabled,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.sources = append(s.sources, rec)
	s.saveSources()
	return rec
}

// UpdateSource modifies an existing source. Returns the updated record
// and true if found, or zero value and false.
func (s *Store) UpdateSource(id int, name, url, sourceType *string, enabled *bool) (SourceRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sources {
		if s.sources[i].ID == id {
			if name != nil {
				s.sources[i].Name = *name
			}
			if url != nil {
				s.sources[i].URL = *url
			}
			if sourceType != nil {
				s.sources[i].SourceType = *sourceType
			}
			if enabled != nil {
				s.sources[i].Enabled = *enabled
			}
			s.sources[i].UpdatedAt = time.Now()
			rec := s.sources[i]
			s.saveSources()
			return rec, true
		}
	}
	return SourceRecord{}, false
}

// DeleteSource removes a source by ID. Returns true if deleted.
func (s *Store) DeleteSource(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sources {
		if s.sources[i].ID == id {
			s.sources = append(s.sources[:i], s.sources[i+1:]...)
			s.saveSources()
			return true
		}
	}
	return false
}

// SourceCount returns the number of download sources.
func (s *Store) SourceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sources)
}

// ========================================================================
// Settings
// ========================================================================

// GetSetting returns a single setting value. Falls back to hardcoded
// defaults when the key has not been persisted.
func (s *Store) GetSetting(key string) string {
	s.mu.RLock()
	v, ok := s.settings[key]
	s.mu.RUnlock()
	if ok {
		return v
	}
	if d, ok := config.DefaultSettings[key]; ok {
		return d
	}
	return ""
}

// GetAllSettings returns all settings merged with defaults.
func (s *Store) GetAllSettings() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(config.DefaultSettings)+len(s.settings))
	for k, v := range config.DefaultSettings {
		out[k] = v
	}
	for k, v := range s.settings {
		out[k] = v
	}
	return out
}

// UpdateSettings persists a batch of settings.
func (s *Store) UpdateSettings(in map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range in {
		s.settings[k] = v
	}
	s.saveSettings()
	return nil
}

// ========================================================================
// Traffic records
// ========================================================================

// CurrentRecord returns the traffic record with is_current=true, or nil.
func (s *Store) CurrentRecord() *TrafficRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.traffic {
		if s.traffic[i].IsCurrent {
			rec := s.traffic[i]
			return &rec
		}
	}
	return nil
}

// CreateRecord inserts a new current traffic record.
func (s *Store) CreateRecord(start, end time.Time, periodType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextTrafficID++
	s.traffic = append(s.traffic, TrafficRecord{
		ID:          s.nextTrafficID,
		PeriodStart: start,
		PeriodEnd:   end,
		PeriodType:  periodType,
		TotalBytes:  0,
		DownloadCount: 0,
		IsCurrent:   true,
		CreatedAt:   time.Now(),
	})
	s.saveTraffic()
}

// SyncTraffic updates total_bytes and increments download_count on the
// current record.
func (s *Store) SyncTraffic(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.traffic {
		if s.traffic[i].IsCurrent {
			s.traffic[i].TotalBytes = bytes
			s.traffic[i].DownloadCount++
			s.saveTraffic()
			return
		}
	}
}

// ResetPeriod marks the current record as finished and creates a new one.
func (s *Store) ResetPeriod(now time.Time, start, end time.Time, periodType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.traffic {
		if s.traffic[i].IsCurrent {
			s.traffic[i].IsCurrent = false
			s.traffic[i].PeriodEnd = now
		}
	}
	s.nextTrafficID++
	s.traffic = append(s.traffic, TrafficRecord{
		ID:          s.nextTrafficID,
		PeriodStart: start,
		PeriodEnd:   end,
		PeriodType:  periodType,
		TotalBytes:  0,
		DownloadCount: 0,
		IsCurrent:   true,
		CreatedAt:   now,
	})
	s.saveTraffic()
}

// TrafficHistory returns the most recent N records.
func (s *Store) TrafficHistory(days int) []TrafficHistoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var result []TrafficHistoryItem
	for i := len(s.traffic) - 1; i >= 0; i-- {
		r := s.traffic[i]
		if r.PeriodStart.Before(cutoff) && !r.IsCurrent {
			continue
		}
		result = append(result, TrafficHistoryItem{
			Start:   r.PeriodStart.Format("2006-01-02T15:04:05"),
			End:     r.PeriodEnd.Format("2006-01-02T15:04:05"),
			Type:    r.PeriodType,
			Bytes:   r.TotalBytes,
			Count:   r.DownloadCount,
			Current: r.IsCurrent,
		})
	}
	return result
}
