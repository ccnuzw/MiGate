package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/imzyb/MiGate/internal/singbox"
)

func singboxStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")

		installed := singbox.IsInstalled()
		if !installed {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"installed": false,
				"status":    "not_installed",
			})
			return
		}
		status := singbox.Status()
		ver, _ := singbox.Version()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"installed":          true,
			"status":             status,
			"version":            strings.TrimSpace(ver),
			"memory_rss_bytes":   singbox.MemoryRSS(),
			"uptime":             singbox.Uptime(),
			"active_connections": singbox.ActiveConnections(),
		})
	}
}

// singboxApplyHandler reads sing-box supported inbounds from the store, builds
// a sing-box config, generates a self-signed cert if missing, writes
// the config to disk and restarts the sing-box service.
func singboxApplyHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}

		if !singbox.IsInstalled() {
			writeJSONError(w, http.StatusBadRequest, "singbox_not_installed")
			return
		}

		// Read sing-box inbounds
		inbounds, err := store.ListInbounds(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_failed", map[string]interface{}{"detail": err.Error()})
			return
		}

		// Build config
		cfg := singbox.BuildConfig(inbounds)

		// Ensure self-signed cert exists
		if _, err := os.Stat(singbox.CertFile); os.IsNotExist(err) {
			if err := singbox.GenerateSelfSignedCert(); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "cert_failed", map[string]interface{}{"detail": err.Error()})
				return
			}
		}

		// Encode and write config
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "marshal_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		if err := os.WriteFile(singbox.DefaultConfigPath, raw, 0644); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "write_failed", map[string]interface{}{"detail": err.Error()})
			return
		}

		// Restart sing-box
		applyErr := singbox.Apply()

		result := map[string]interface{}{
			"applied":     applyErr == nil,
			"config_path": singbox.DefaultConfigPath,
			"inbounds":    len(cfg.Inbounds),
		}
		if applyErr != nil {
			result["error"] = applyErr.Error()
			writeJSON(w, http.StatusInternalServerError, result)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func singboxValidateHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		result := validateSingboxConfig(r.Context(), store)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func validateSingboxConfig(ctx context.Context, store Store) configValidationResult {
	result := configValidationResult{Target: "singbox", Valid: true, Warnings: []string{}}
	if store == nil {
		result.Valid = false
		result.Error = "store_unavailable"
		return result
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_inbounds_failed"
		return result
	}
	cfg := singbox.BuildConfig(inbounds)
	result.Inbounds = len(cfg.Inbounds)
	if result.Inbounds == 0 {
		result.Warnings = append(result.Warnings, "no_enabled_singbox_inbounds")
	}
	if _, err := json.Marshal(cfg); err != nil {
		result.Valid = false
		result.Error = err.Error()
	}
	return result
}

// tryApplySingbox reads sing-box supported inbounds from the store, builds
// a sing-box config, writes it to disk and restarts sing-box. Errors are
// silently returned (not panicked) to avoid blocking the caller.
func tryApplySingbox(ctx context.Context, store Store) error {
	if !singbox.IsInstalled() {
		return nil // sing-box not available, skip silently
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return fmt.Errorf("list inbounds: %w", err)
	}
	cfg := singbox.BuildConfig(inbounds)
	if _, err := os.Stat(singbox.CertFile); os.IsNotExist(err) {
		if err := singbox.GenerateSelfSignedCert(); err != nil {
			return fmt.Errorf("generate cert: %w", err)
		}
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(singbox.DefaultConfigPath, raw, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return singbox.Apply()
}

// singboxConfigHandler returns the current sing-box config JSON.
func singboxConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data, err := os.ReadFile(singbox.DefaultConfigPath)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "read_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		// Parse and re-marshal so the client gets pretty-printed JSON
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			_, _ = w.Write(data)
			return
		}
		pretty, _ := json.MarshalIndent(parsed, "", "  ")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(pretty)
	}
}

// singboxVersionHandler returns the sing-box version.
func singboxVersionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !singbox.IsInstalled() {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": "not_installed"})
			return
		}
		ver, err := singbox.Version()
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": "unknown", "error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": strings.TrimSpace(ver)})
	}
}

// singboxLogsHandler returns recent sing-box service logs from journalctl.
func singboxLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		lines := r.URL.Query().Get("lines")
		if lines == "" {
			lines = "50"
		}
		if n, err := strconv.Atoi(lines); err != nil || n < 1 {
			lines = "50"
		} else if n > maxXrayLogLines {
			lines = strconv.Itoa(maxXrayLogLines)
		}
		out, err := exec.Command("journalctl", "-u", singbox.ServiceName(), "-n", lines, "--no-pager", "-o", "short-iso").CombinedOutput()
		if err != nil {
			out, err = exec.Command("tail", "-n", lines, "/var/log/syslog").CombinedOutput()
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"logs": "无法读取 Sing-box 日志：journalctl 和 syslog 均不可用。"})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"logs": string(out)})
	}
}
