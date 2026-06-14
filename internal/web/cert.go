package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
)

var validDomain = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
var validEmail = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func certStatusHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		domain := ""
		email := ""
		certPath := ""
		keyPath := ""
		issued := false

		if cfg.configDir != "" {
			configPath := cfg.configDir + "/panel.json"
			data, err := os.ReadFile(configPath)
			if err == nil {
				var raw map[string]interface{}
				if err := json.Unmarshal(data, &raw); err == nil {
					if d, ok := raw["cert_domain"].(string); ok {
						domain = d
					}
					if e, ok := raw["cert_email"].(string); ok {
						email = e
					}
				}
			}
			if domain != "" {
				// Check /etc/xray/certs/{domain}.pem and .key first
				certPath = "/etc/xray/certs/" + domain + ".pem"
				keyPath = "/etc/xray/certs/" + domain + ".key"
				if _, err := os.Stat(certPath); err == nil {
					if _, err := os.Stat(keyPath); err == nil {
						issued = true
					}
				}
				// Fallback to config dir for tests
				if !issued && cfg.configDir != "" {
					certDir := cfg.configDir + "/certs/" + domain
					certPath = certDir + "/fullchain.pem"
					keyPath = certDir + "/privkey.pem"
					if _, err := os.Stat(certPath); err == nil {
						if _, err := os.Stat(keyPath); err == nil {
							issued = true
						}
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"domain":    domain,
			"email":     email,
			"issued":    issued,
			"cert_path": certPath,
			"key_path":  keyPath,
		})
	}
}

func installACMESh(email string) (string, error) {
	return "acme.sh is not installed. Install acme.sh from a pinned release and verify its checksum or signature before retrying.", fmt.Errorf("refusing to download and execute unverified acme.sh installer for %s", email)
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
		if req.Domain == "" || req.Email == "" {
			writeJSONError(w, http.StatusBadRequest, "domain_and_email_required")
			return
		}
		if !validDomain.MatchString(req.Domain) {
			writeJSONError(w, http.StatusBadRequest, "invalid_domain")
			return
		}
		if !validEmail.MatchString(req.Email) {
			writeJSONError(w, http.StatusBadRequest, "invalid_email")
			return
		}
		if cfg.configDir == "" {
			writeJSONError(w, http.StatusNotFound, "cert_not_available")
			return
		}

		// Issue cert via acme.sh directly to /etc/xray/certs/
		certDir := "/etc/xray/certs"
		if err := os.MkdirAll(certDir, 0755); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "mkdir_cert_dir_failed")
			return
		}

		certFile := certDir + "/" + req.Domain + ".pem"
		keyFile := certDir + "/" + req.Domain + ".key"

		// Check if acme.sh is installed; if not, install it without interpolating
		// request data into a shell command string.
		if _, err := exec.LookPath("acme.sh"); err != nil {
			installOut, err := installACMESh(req.Email)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "install_acme_failed", map[string]interface{}{"detail": installOut})
				return
			}
		}

		// Run acme.sh --issue --standalone
		out, err := exec.Command("acme.sh",
			"--issue", "--standalone", "-d", req.Domain,
			"--keylength", "ec-256",
			"--fullchain-file", certFile,
			"--key-file", keyFile,
			"--reloadcmd", "systemctl restart xray || true",
		).CombinedOutput()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "issue_cert_failed", map[string]interface{}{"detail": string(out)})
			return
		}

		// Set permissions for xray user
		exec.Command("chmod", "644", certFile, keyFile).Run()

		// Update panel.json with cert domain/email
		configPath := cfg.configDir + "/panel.json"
		existing, err := os.ReadFile(configPath)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "read_panel_config_failed")
			return
		}
		var raw map[string]interface{}
		if err := json.Unmarshal(existing, &raw); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "parse_panel_config_failed")
			return
		}
		raw["cert_domain"] = req.Domain
		raw["cert_email"] = req.Email
		updated, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "serialize_failed")
			return
		}
		if err := os.WriteFile(configPath, updated, 0o600); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "write_panel_config_failed")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "issued",
			"domain":    req.Domain,
			"cert_path": certFile,
			"key_path":  keyFile,
		})
	}
}
