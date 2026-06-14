package web

import (
	"encoding/json"
	"net/http"
	"os"
)

func settingsHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.configDir == "" {
			writeJSONError(w, http.StatusNotFound, "settings_not_available")
			return
		}
		configPath := cfg.configDir + "/panel.json"
		switch r.Method {
		case http.MethodGet:
			data, err := os.ReadFile(configPath)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "read_config_failed")
				return
			}
			// Mask password for GET
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "parse_config_failed")
				return
			}
			if _, exists := raw["panel_password"]; exists {
				raw["has_password"] = true
				delete(raw, "panel_password")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(raw)
		case http.MethodPut:
			var updated map[string]interface{}
			if err := decodeJSONBody(r, &updated); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			// Read existing to preserve password if not provided
			existing, err := os.ReadFile(configPath)
			if err == nil {
				var existingMap map[string]interface{}
				if err := json.Unmarshal(existing, &existingMap); err == nil {
					if pw, has := updated["panel_password"]; !has || pw == "" {
						if oldPW, ok := existingMap["panel_password"]; ok {
							updated["panel_password"] = oldPW
						}
					}
					// Preserve database_path if not in update
					if _, has := updated["database_path"]; !has {
						if oldDP, ok := existingMap["database_path"]; ok {
							updated["database_path"] = oldDP
						}
					}
				}
			}
			data, err := json.MarshalIndent(updated, "", "  ")
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "serialize_failed")
				return
			}
			if err := os.WriteFile(configPath, data, 0o600); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "write_config_failed")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			methodNotAllowed(w)
		}
	}
}
