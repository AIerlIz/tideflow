package database

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"tideflow/internal/config"
)

var db *sql.DB

// GetDB returns the shared database connection.
func GetDB() *sql.DB {
	return db
}

// Init opens (or creates) the SQLite database, runs migrations, and seeds defaults.
func Init() error {
	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return err
	}

	dsn := config.DatabaseURL + "?_journal_mode=WAL&_busy_timeout=5000"
	var err error
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1) // SQLite works best with a single writer

	if err := migrate(); err != nil {
		return err
	}
	if err := seedDefaults(); err != nil {
		return err
	}
	return nil
}

// Close shuts down the database connection.
func Close() {
	if db != nil {
		db.Close()
	}
}

func migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS download_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'http',
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS global_settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key TEXT UNIQUE NOT NULL,
		value TEXT DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS traffic_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		period_start DATETIME NOT NULL,
		period_end DATETIME NOT NULL,
		period_type TEXT NOT NULL,
		total_bytes INTEGER DEFAULT 0,
		download_count INTEGER DEFAULT 0,
		is_current INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(schema)
	return err
}

// seedDefaults inserts default download sources if the table is empty.
func seedDefaults() error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM download_sources").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaults := []struct {
		name, url, stype string
	}{
		{"100MB 测速文件", "http://speedtest.tele2.net/100MB.zip", "http"},
		{"10MB 测速文件", "http://speedtest.tele2.net/10MB.zip", "http"},
		{"1MB 测速文件", "http://speedtest.tele2.net/1MB.zip", "http"},
	}

	for _, d := range defaults {
		if _, err := db.Exec(
			"INSERT INTO download_sources (name, url, source_type, enabled) VALUES (?, ?, ?, 1)",
			d.name, d.url, d.stype,
		); err != nil {
			return err
		}
	}
	log.Printf("Seeded %d default download sources", len(defaults))
	return nil
}

// EnsureDataDir creates the data directory relative to the binary or cwd.
func EnsureDataDir() {
	os.MkdirAll(filepath.Dir(config.DatabaseURL), 0755)
}
