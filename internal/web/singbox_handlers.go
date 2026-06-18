package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/singbox"
)

var errSingboxNotInstalled = errors.New("singbox_not_installed")

func singboxStatusHandler(cfg ...*routerConfig) http.HandlerFunc {
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
		management := singbox.Management()
		status := singbox.Status()
		ver, _ := singbox.Version()
		ports := singboxExpectedUDPPorts(r.Context(), firstRouterConfig(cfg))
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"installed":          true,
			"managed":            management.Managed,
			"service":            management.Service,
			"status":             status,
			"version":            strings.TrimSpace(ver),
			"memory_rss_bytes":   singbox.MemoryRSS(),
			"uptime":             singbox.Uptime(),
			"active_connections": singbox.ActiveConnections(),
			"config_path":        singbox.DefaultConfigPath,
			"commands_executed":  []string{},
			"listening_ports":    ports,
		})
	}
}

func firstRouterConfig(cfg []*routerConfig) *routerConfig {
	if len(cfg) == 0 {
		return nil
	}
	return cfg[0]
}

func singboxExpectedUDPPorts(ctx context.Context, cfg *routerConfig) []map[string]interface{} {
	if cfg == nil || cfg.store == nil {
		return []map[string]interface{}{}
	}
	inbounds, err := cfg.store.ListInbounds(ctx)
	if err != nil {
		return []map[string]interface{}{}
	}
	expected := []int{}
	records := []db.Inbound{}
	for _, inbound := range inbounds {
		if !inbound.Enabled || db.InboundCore(inbound) != db.CoreSingbox {
			continue
		}
		switch db.NormalizeInboundProtocol(inbound.Protocol) {
		case "hysteria2", "tuic":
			if inbound.Port > 0 {
				expected = append(expected, inbound.Port)
				records = append(records, inbound)
			}
		}
	}
	listening := singbox.ListeningUDPPorts(expected)
	result := make([]map[string]interface{}, 0, len(records))
	for _, inbound := range records {
		result = append(result, map[string]interface{}{
			"inbound_id": inbound.ID,
			"protocol":   inbound.Protocol,
			"port":       inbound.Port,
			"network":    "udp",
			"listening":  listening[inbound.Port],
		})
	}
	return result
}

// singboxApplyHandler reads sing-box supported inbounds from the store, builds
// a sing-box config, generates a self-signed cert if missing, validates a temp
// config, atomically installs it, and restarts the sing-box service.
func singboxApplyHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}

		if !singbox.IsInstalled() {
			writeJSONError(w, http.StatusBadRequest, "singbox_not_installed")
			return
		}

		if cfg == nil || cfg.store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
			return
		}

		// Read sing-box inbounds
		inbounds, err := cfg.store.ListInbounds(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_failed", map[string]interface{}{"detail": err.Error()})
			return
		}

		outbounds, err := cfg.store.ListOutbounds(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_outbounds_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		rules, err := cfg.store.ListRoutingRules(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_routing_rules_failed", map[string]interface{}{"detail": err.Error()})
			return
		}

		// Build config
		built := buildSingboxConfigForRuntime(r.Context(), cfg, inbounds, outbounds, rules)
		if built.err != nil {
			writeJSONError(w, http.StatusBadRequest, "build_failed", map[string]interface{}{"detail": built.err.Error()})
			return
		}

		// Ensure self-signed cert exists
		if _, err := os.Stat(singbox.CertFile); os.IsNotExist(err) {
			if err := singbox.GenerateSelfSignedCert(); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "cert_failed", map[string]interface{}{"detail": err.Error()})
				return
			}
		}

		raw, err := json.MarshalIndent(built.config, "", "  ")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "marshal_failed", map[string]interface{}{"detail": err.Error()})
			return
		}

		applyErr := singbox.ApplyConfig(raw)

		result := map[string]interface{}{
			"applied":           applyErr == nil,
			"config_path":       singbox.DefaultConfigPath,
			"inbounds":          len(built.config.Inbounds),
			"commands_executed": []string{"sing-box check -c <temp>", "systemctl restart " + singbox.RuntimeServiceName()},
		}
		if len(built.warnings) > 0 {
			result["warnings"] = built.warnings
		}
		if applyErr != nil {
			result["error"] = applyErr.Error()
			writeJSON(w, http.StatusInternalServerError, result)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func singboxValidateHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		result := validateSingboxConfig(r.Context(), cfg)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func validateSingboxConfig(ctx context.Context, cfg *routerConfig) configValidationResult {
	result := configValidationResult{Target: "singbox", Valid: true, Warnings: []string{}}
	if cfg == nil || cfg.store == nil {
		result.Valid = false
		result.Error = "store_unavailable"
		return result
	}
	inbounds, err := cfg.store.ListInbounds(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_inbounds_failed"
		return result
	}
	outbounds, err := cfg.store.ListOutbounds(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_outbounds_failed"
		return result
	}
	rules, err := cfg.store.ListRoutingRules(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_routing_rules_failed"
		return result
	}
	return validateSingboxConfigSnapshotWithRuntime(ctx, validationSnapshot{inbounds: inbounds, outbounds: outbounds, rules: rules}, cfg)
}

func validateSingboxConfigSnapshot(snapshot validationSnapshot) configValidationResult {
	result := configValidationResult{Target: "singbox", Valid: true, Warnings: []string{}}
	cfg, err := singbox.BuildConfigWithOutbounds(snapshot.inbounds, snapshot.outbounds, snapshot.rules)
	if err != nil {
		result.Valid = false
		result.Error = err.Error()
		return result
	}
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

func validateSingboxConfigSnapshotWithRuntime(ctx context.Context, snapshot validationSnapshot, runtimeCfg *routerConfig) configValidationResult {
	result := configValidationResult{Target: "singbox", Valid: true, Warnings: []string{}}
	inbounds := snapshot.inbounds
	built := buildSingboxConfigForRuntime(ctx, runtimeCfg, inbounds, snapshot.outbounds, snapshot.rules)
	result.Inbounds = len(built.config.Inbounds)
	result.Outbounds = len(built.config.Outbounds)
	if built.config.Route != nil {
		result.Rules = len(built.config.Route.Rules)
	}
	if result.Inbounds == 0 {
		result.Warnings = append(result.Warnings, "no_enabled_singbox_inbounds")
	}
	result.Warnings = append(result.Warnings, built.warnings...)
	if built.err != nil {
		result.Valid = false
		result.Error = built.err.Error()
		return result
	}
	if _, err := json.Marshal(built.config); err != nil {
		result.Valid = false
		result.Error = err.Error()
	}
	return result
}

type builtSingboxConfig struct {
	config   singbox.Config
	warnings []string
	err      error
}

func buildSingboxConfigForRuntime(ctx context.Context, cfg *routerConfig, inbounds []db.Inbound, outbounds []db.Outbound, rules []db.RoutingRule) builtSingboxConfig {
	hasSingboxInbound := singbox.HasEnabledSingboxInbound(inbounds)
	if !hasSingboxInbound {
		built, err := singbox.BuildConfigWithOutbounds(inbounds, outbounds, rules)
		return builtSingboxConfig{config: built, err: err}
	}
	runtime := SingboxRuntime(defaultSingboxRuntime{})
	if cfg != nil && cfg.singboxRuntime != nil {
		runtime = cfg.singboxRuntime
	}
	capability := runtime.Capability(ctx)
	built, err := singbox.BuildConfigWithOutbounds(inbounds, outbounds, rules)
	result := builtSingboxConfig{
		config: built,
		err:    err,
	}
	if result.config.Experimental == nil && capability.V2RayAPIStats {
		result.config = singbox.BuildConfigWithOptions(inbounds, singbox.BuildOptions{EnableV2RayAPIStats: true})
		withOutbounds, buildErr := singbox.BuildConfigWithOutbounds(inbounds, outbounds, rules)
		if buildErr != nil {
			result.err = buildErr
		} else {
			withOutbounds.Experimental = result.config.Experimental
			result.config = withOutbounds
		}
	}
	if capability.Unsupported {
		result.warnings = append(result.warnings, "singbox_stats_unsupported")
	} else if !capability.V2RayAPIStats && strings.TrimSpace(capability.Message) != "" {
		result.warnings = append(result.warnings, "singbox_stats_capability_check_failed")
	}
	return result
}

// tryApplySingbox reads sing-box supported inbounds from the store, builds
// a sing-box config, validates a temp config, atomically installs it, and
// restarts sing-box. Errors are returned to the caller for UI/API visibility.
func tryApplySingbox(ctx context.Context, store Store) error {
	return tryApplySingboxWithRuntime(ctx, store, defaultSingboxRuntime{}, false)
}

func tryApplySingboxWithRuntime(ctx context.Context, store Store, runtime SingboxRuntime, strict bool) error {
	if !singbox.IsInstalled() {
		if strict {
			return errSingboxNotInstalled
		}
		return nil
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return fmt.Errorf("list inbounds: %w", err)
	}
	outbounds, err := store.ListOutbounds(ctx)
	if err != nil {
		return fmt.Errorf("list outbounds: %w", err)
	}
	rules, err := store.ListRoutingRules(ctx)
	if err != nil {
		return fmt.Errorf("list routing rules: %w", err)
	}
	built := buildSingboxConfigForRuntime(ctx, &routerConfig{singboxRuntime: runtime}, inbounds, outbounds, rules)
	if built.err != nil {
		return fmt.Errorf("build config: %w", built.err)
	}
	cfg := built.config
	if _, err := os.Stat(singbox.CertFile); os.IsNotExist(err) {
		if err := singbox.GenerateSelfSignedCert(); err != nil {
			return fmt.Errorf("generate cert: %w", err)
		}
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return singbox.ApplyConfig(raw)
}

// singboxConfigHandler returns the current sing-box config JSON.
func singboxConfigHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data, err := os.ReadFile(singbox.DefaultConfigPath)
		if err != nil {
			if os.IsNotExist(err) && cfg != nil && cfg.store != nil {
				inbounds, listErr := cfg.store.ListInbounds(r.Context())
				if listErr != nil {
					writeJSONError(w, http.StatusInternalServerError, "list_failed", map[string]interface{}{"detail": listErr.Error()})
					return
				}
				outbounds, outErr := cfg.store.ListOutbounds(r.Context())
				if outErr != nil {
					writeJSONError(w, http.StatusInternalServerError, "list_outbounds_failed", map[string]interface{}{"detail": outErr.Error()})
					return
				}
				rules, rulesErr := cfg.store.ListRoutingRules(r.Context())
				if rulesErr != nil {
					writeJSONError(w, http.StatusInternalServerError, "list_routing_rules_failed", map[string]interface{}{"detail": rulesErr.Error()})
					return
				}
				built := buildSingboxConfigForRuntime(r.Context(), cfg, inbounds, outbounds, rules)
				if built.err != nil {
					writeJSONError(w, http.StatusBadRequest, "build_failed", map[string]interface{}{"detail": built.err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, built.config)
				return
			}
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
		out, err := exec.Command("journalctl", "-u", singbox.RuntimeServiceName(), "-n", lines, "--no-pager", "-o", "short-iso").CombinedOutput()
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
