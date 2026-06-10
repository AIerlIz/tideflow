package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
		Settings map[string]interface{} `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "无效的请求格式", "success": false})
		return
	}
	if body.Settings == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "无效的请求格式", "success": false})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"message": "无法开启数据库事务", "success": false})
		return
	}
	defer tx.Rollback()

	for key, val := range body.Settings {
		// Accept any JSON type, convert to string for storage
		value := fmt.Sprintf("%v", val)

		var existing string
		err := tx.QueryRow("SELECT value FROM global_settings WHERE key = ?", key).Scan(&existing)
		if err == sql.ErrNoRows {
			if _, err = tx.Exec("INSERT INTO global_settings (key, value) VALUES (?, ?)", key, value); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"message": "保存设置失败", "success": false})
				return
			}
		} else if err == nil {
			if _, err = tx.Exec("UPDATE global_settings SET value = ? WHERE key = ?", value, key); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"message": "保存设置失败", "success": false})
				return
			}
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"message": "读取设置失败", "success": false})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"message": "提交设置失败", "success": false})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "设置已更新", "success": true})
}

// HandleGetDefaults returns the default settings map.
func (h *SettingsHandler) HandleGetDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"defaults": config.DefaultSettings})
}
