package handlers

import (
	"encoding/json"
	"net/http"

	"tideflow/internal/config"
)

// HandleAuth verifies the admin password.
func HandleAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": false})
		return
	}
	if body.Password == config.AdminPassword() {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	} else {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": false})
	}
}
