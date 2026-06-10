package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"tideflow/internal/config"
	"tideflow/internal/storage"
)

// SettingsHandler groups settings-related HTTP handlers.
type SettingsHandler struct {
	Store *storage.Store
}

// HandleGetSettings returns all settings merged with defaults.
func (h *SettingsHandler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.Store.GetAllSettings()
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

	// Convert all values to strings
	kv := make(map[string]string, len(body.Settings))
	for key, val := range body.Settings {
		kv[key] = fmt.Sprintf("%v", val)
	}

	if err := h.Store.UpdateSettings(kv); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"message": "保存设置失败", "success": false})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "设置已更新", "success": true})
}

// HandleGetDefaults returns the default settings map.
func (h *SettingsHandler) HandleGetDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"defaults": config.DefaultSettings})
}
