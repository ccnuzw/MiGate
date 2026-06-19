package web

import (
	"net/http"

	certsvc "github.com/imzyb/MiGate/internal/service/cert"
)

func certStatusHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, certService(cfg).Status())
	}
}

func certIssueHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req struct {
			Domain             string `json:"domain"`
			Email              string `json:"email"`
			Confirm            bool   `json:"confirm"`
			AllowSystemChanges bool   `json:"allow_system_changes"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if !req.Confirm || !req.AllowSystemChanges {
			writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
			return
		}
		result, err := certService(cfg).Issue(r.Context(), certsvc.IssueRequest{Domain: req.Domain, Email: req.Email})
		if err != nil {
			writeServiceError(w, certErrorStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func certService(cfg *routerConfig) certsvc.Service {
	service := certsvc.Service{ConfigDir: cfg.configDir}
	if cfg.configDir != "" {
		service.SaveConfig = settingsService(cfg.configDir + "/panel.json").SaveCert
	}
	return service
}

func certErrorStatus(err error) int {
	if serviceErr, ok := err.(certsvc.Error); ok {
		switch serviceErr.Code {
		case "domain_and_email_required", "invalid_domain", "invalid_email":
			return http.StatusBadRequest
		case "cert_not_available":
			return http.StatusNotFound
		default:
			return http.StatusInternalServerError
		}
	}
	return http.StatusInternalServerError
}
