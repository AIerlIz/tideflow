package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"tideflow/internal/config"
)

// SettingsHandler groups settings-related HTTP handlers.
type SettingsHandler struct {
	DB *sql.DB
}

// HandleGetSettings returns all settings merged with defaults.
func (h *SettingsHandler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings := make(map[string]string)
	for k, v := range config.DefaultSettings {
		settings[k] = v
	}

	rows, err := h.DB.Query("SELECT key, value FROM global_settings")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		settings[k] = v
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"settings": settings})
}

// HandleUpdateSettings bulk-updates settings.
func (h *SettingsHandler) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Settings map[string]string `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "无效的请求格式", "success": false})
		return
	}
	if body.Settings == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "无效的请求格式", "success": false})
		return
	}

	for key, value := range body.Settings {
		var existing string
		err := h.DB.QueryRow("SELECT value FROM global_settings WHERE key = ?", key).Scan(&existing)
		if err == sql.ErrNoRows {
			h.DB.Exec("INSERT INTO global_settings (key, value) VALUES (?, ?)", key, value)
		} else {
			h.DB.Exec("UPDATE global_settings SET value = ? WHERE key = ?", value, key)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "设置已更新", "success": true})
}

// HandleGetDefaults returns the default settings map.
func (h *SettingsHandler) HandleGetDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"defaults": config.DefaultSettings})
}
