package config

import (
	"os"
	"path/filepath"
)

var (
	// DataDir is where the SQLite database is stored.
	DataDir     = ""
	DatabaseURL = ""
)

// DefaultSettings mirrors the Python DEFAULT_SETTINGS dict.
var DefaultSettings = map[string]string{
	"traffic_cap_enabled":    "false",
	"traffic_cap_bytes":      "107374182400", // 100GB
	"traffic_cap_period":     "daily",
	"traffic_cap_reset_day":     "1",
	"traffic_cap_reset_hour":    "0",
	"traffic_cap_reset_weekday": "1", // 1=Mon … 7=Sun
	"time_window_enabled":    "false",
	"time_window_start":      "00:00",
	"time_window_end":        "23:59",
	"default_max_speed":      "0",
	"max_concurrent":         "3",
	"poll_interval":          "2",
}

// Timezone is read from the TZ env var (default "Asia/Shanghai").
func Timezone() string {
	tz := os.Getenv("TZ")
	if tz == "" {
		return "Asia/Shanghai"
	}
	return tz
}

// AdminPassword is read from the ADMIN_PASSWORD env var (default "admin").
func AdminPassword() string {
	pw := os.Getenv("ADMIN_PASSWORD")
	if pw == "" {
		return "admin"
	}
	return pw
}

// Init resolves paths relative to the working directory.
func Init() {
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		// Default: ./data relative to cwd
		dir = filepath.Join(".", "data")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	DataDir = abs
	DatabaseURL = filepath.Join(DataDir, "tideflow.db")
}
