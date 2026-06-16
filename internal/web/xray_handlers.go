package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/singbox"
	"github.com/imzyb/MiGate/internal/xray"
)

func xrayConfigHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		inbounds := []db.Inbound{}
		outbounds := []db.Outbound{}
		rules := []db.RoutingRule{}
		if store != nil {
			if loaded, err := store.ListInbounds(r.Context()); err == nil {
				inbounds = loaded
			}
			if loaded, err := store.ListOutbounds(r.Context()); err == nil {
				outbounds = loaded
			}
			if loaded, err := store.ListRoutingRules(r.Context()); err == nil {
				rules = loaded
			}
		}
		config, err := xray.BuildConfigWithOutbounds(inbounds, outbounds, rules)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "build_xray_config_failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}
}

func xrayStatusHandler(controller XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if controller == nil {
			controller = defaultXrayController{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(controller.Status(r.Context()))
	}
}

type configValidationResult struct {
	Target    string   `json:"target"`
	Valid     bool     `json:"valid"`
	Error     string   `json:"error,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
	Inbounds  int      `json:"inbounds"`
	Outbounds int      `json:"outbounds,omitempty"`
	Rules     int      `json:"rules,omitempty"`
}

func xrayValidateHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		result := validateXrayConfig(r.Context(), store)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func validateXrayConfig(ctx context.Context, store Store) configValidationResult {
	result := configValidationResult{Target: "xray", Valid: true, Warnings: []string{}}
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
	outbounds, err := store.ListOutbounds(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_outbounds_failed"
		return result
	}
	rules, err := store.ListRoutingRules(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_routing_rules_failed"
		return result
	}
	return validateXrayConfigSnapshot(validationSnapshot{inbounds: inbounds, outbounds: outbounds, rules: rules})
}

func validateXrayConfigSnapshot(snapshot validationSnapshot) configValidationResult {
	result := configValidationResult{Target: "xray", Valid: true, Warnings: []string{}}
	inbounds := snapshot.inbounds
	outbounds := snapshot.outbounds
	rules := snapshot.rules
	cfg, err := xray.BuildConfigWithOutbounds(inbounds, outbounds, rules)
	if err != nil {
		result.Valid = false
		result.Error = err.Error()
		return result
	}
	result.Inbounds = len(cfg.Inbounds)
	result.Outbounds = len(cfg.Outbounds)
	if cfg.Routing != nil {
		result.Rules = len(cfg.Routing.Rules)
	}
	for _, inbound := range inbounds {
		if inbound.Enabled && singbox.IsSingboxProtocol(inbound.Protocol) {
			result.Warnings = append(result.Warnings, inbound.Protocol+" is handled by sing-box")
		}
	}
	for _, rule := range rules {
		if strings.TrimSpace(rule.RuleSet) != "" {
			result.Warnings = append(result.Warnings, "rule_set is stored for future use but not emitted in Xray config")
			break
		}
	}
	return result
}

func xrayApplyHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var payload struct {
			Confirm            bool `json:"confirm"`
			AllowSystemChanges bool `json:"allow_system_changes"`
		}
		if err := decodeJSONBody(r, &payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if !payload.Confirm || !payload.AllowSystemChanges {
			writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
			return
		}
		var controller XrayController = defaultXrayController{}
		if cfg != nil && cfg.xrayController != nil {
			controller = cfg.xrayController
		}
		var store Store
		var singboxRuntime SingboxRuntime = defaultSingboxRuntime{}
		if cfg != nil {
			store = cfg.store
			if cfg.singboxRuntime != nil {
				singboxRuntime = cfg.singboxRuntime
			}
		}

		// 1. Apply Xray config
		xrayResult := controller.Apply(r.Context())

		// 2. Apply sing-box config if sing-box supported inbounds exist
		singboxResult := map[string]interface{}{
			"applied": false,
			"reason":  "not_needed",
		}
		if store != nil && singbox.IsInstalled() {
			inbounds, err := store.ListInbounds(r.Context())
			if err == nil {
				if singbox.HasEnabledSingboxInbound(inbounds) {
					applyErr := tryApplySingboxWithRuntime(r.Context(), store, singboxRuntime)
					if applyErr != nil {
						singboxResult = map[string]interface{}{
							"applied": false,
							"error":   applyErr.Error(),
						}
					} else {
						singboxResult = map[string]interface{}{
							"applied":  true,
							"inbounds": len(singbox.BuildConfigWithOptions(inbounds, singbox.BuildOptions{}).Inbounds),
						}
					}
				}
			}
		} else if store == nil {
			singboxResult["reason"] = "no_store"
		} else {
			singboxResult["reason"] = "singbox_not_installed"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"xray":    xrayResult,
			"singbox": singboxResult,
		})
	}
}
