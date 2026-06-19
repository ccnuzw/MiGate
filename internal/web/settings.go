package web

import (
	"net/http"
	"path/filepath"
	"strings"

	settingssvc "github.com/imzyb/MiGate/internal/service/settings"
)

func settingsHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.configDir == "" {
			writeJSONError(w, http.StatusNotFound, "settings_not_available")
			return
		}
		configPath := filepath.Join(cfg.configDir, "panel.json")
		service := settingsService(configPath)
		switch r.Method {
		case http.MethodGet:
			settings, err := service.Get()
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "read_config_failed")
				return
			}
			writeJSON(w, http.StatusOK, settings)
		case http.MethodPut:
			var req settingssvc.Request
			if err := decodeJSONBody(r, &req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			if err := service.Update(req); err != nil {
				writeJSONError(w, http.StatusInternalServerError, panelSettingsErrorCode(err))
				return
			}
			writeJSON(w, http.StatusOK, StatusResponse{Status: "ok"})
		default:
			methodNotAllowed(w)
		}
	}
}

func panelSettingsErrorCode(err error) string {
	if err != nil && strings.Contains(err.Error(), "hash password") {
		return "hash_password_failed"
	}
	return "write_config_failed"
}

func settingsService(configPath string) settingssvc.Service {
	return settingssvc.Service{
		ConfigPath:     configPath,
		HashPassword:   HashPanelPassword,
		IsPasswordHash: IsPanelPasswordHash,
	}
}

func writePanelPasswordToConfig(configPath, hashedPassword string) error {
	return settingsService(configPath).SetPasswordHash(hashedPassword)
}
