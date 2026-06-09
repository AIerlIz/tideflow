package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WriteJSON serializes v as JSON and writes it to the response.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeJSON is the package-private alias.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	WriteJSON(w, status, v)
}

// formatTime formats a scanned database time value as ISO 8601.
func formatTime(v interface{}) string {
	switch t := v.(type) {
	case time.Time:
		return t.Format("2006-01-02T15:04:05")
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}
