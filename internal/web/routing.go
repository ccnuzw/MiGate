package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/imzyb/MiGate/internal/db"
)

func routingRulesHandler(cfg *routerConfig) http.HandlerFunc {
	store := cfg.store
	ctrl := cfg.xrayController
	if ctrl == nil {
		ctrl = defaultXrayController{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rules, err := store.ListRoutingRules(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rules)
		case http.MethodPost:
			var params db.CreateRoutingRuleParams
			if err := decodeJSONBody(r, &params); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			rule, err := store.CreateRoutingRule(r.Context(), params)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "create_failed")
				return
			}
			applyResult := ctrl.Apply(r.Context())
			payload := map[string]interface{}{"rule": rule, "xray": applyResult}
			affected, checkErr := routingChangeAffectsSingboxStrict(r.Context(), store, rule)
			if checkErr != nil {
				attachSingboxResult(payload, failedSingboxListSummary(checkErr))
			} else if affected {
				attachSingboxResult(payload, strictSingboxApply(r.Context(), cfg, store))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		default:
			methodNotAllowed(w)
		}
	}
}

func routingRuleChildrenHandler(cfg *routerConfig) http.HandlerFunc {
	store := cfg.store
	ctrl := cfg.xrayController
	if ctrl == nil {
		ctrl = defaultXrayController{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/routing-rules/")
		if path == "reorder" {
			if r.Method != http.MethodPost {
				writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
				return
			}
			var req struct {
				IDs []int64 `json:"ids"`
			}
			if err := decodeJSONBody(r, &req); err != nil || len(req.IDs) == 0 {
				writeJSONError(w, http.StatusBadRequest, "invalid_payload")
				return
			}
			if err := store.ReorderRoutingRules(r.Context(), req.IDs); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "reorder_failed")
				return
			}
			payload := map[string]interface{}{"status": "reordered"}
			hasSingboxInbounds, checkErr := storeHasSingboxInboundsStrict(r.Context(), store)
			if checkErr != nil {
				attachSingboxResult(payload, failedSingboxListSummary(checkErr))
			} else if hasSingboxInbounds {
				attachSingboxResult(payload, strictSingboxApply(r.Context(), cfg, store))
			}
			writeJSON(w, http.StatusOK, payload)
			return
		}
		idStr := strings.TrimSuffix(path, "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_id")
			return
		}
		switch r.Method {
		case http.MethodPut:
			var params db.UpdateRoutingRuleParams
			if err := decodeJSONBody(r, &params); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			previousRule, hadPreviousRule, err := findRoutingRuleStrict(r.Context(), store, id)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
				return
			}
			rule, err := store.UpdateRoutingRule(r.Context(), id, params)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSONError(w, http.StatusNotFound, "not_found")
				} else {
					writeJSONError(w, http.StatusBadRequest, "update_failed")
				}
				return
			}
			applyResult := ctrl.Apply(r.Context())
			payload := map[string]interface{}{"rule": rule, "xray": applyResult}
			updatedAffected, updatedCheckErr := routingChangeAffectsSingboxStrict(r.Context(), store, rule)
			previousAffected := false
			var previousCheckErr error
			if hadPreviousRule {
				previousAffected, previousCheckErr = routingChangeAffectsSingboxStrict(r.Context(), store, previousRule)
			}
			if updatedCheckErr != nil {
				attachSingboxResult(payload, failedSingboxListSummary(updatedCheckErr))
			} else if previousCheckErr != nil {
				attachSingboxResult(payload, failedSingboxListSummary(previousCheckErr))
			} else if updatedAffected || previousAffected {
				attachSingboxResult(payload, strictSingboxApply(r.Context(), cfg, store))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case http.MethodDelete:
			deletedRule, found, err := findRoutingRuleStrict(r.Context(), store, id)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
				return
			}
			if !found {
				writeJSONError(w, http.StatusNotFound, "not_found")
				return
			}
			err = store.DeleteRoutingRule(r.Context(), id)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSONError(w, http.StatusNotFound, "not_found")
				} else {
					writeJSONError(w, http.StatusInternalServerError, "delete_failed")
				}
				return
			}
			applyResult := ctrl.Apply(r.Context())
			payload := map[string]interface{}{"status": "deleted", "xray": applyResult}
			affected, checkErr := routingChangeAffectsSingboxStrict(r.Context(), store, deletedRule)
			if checkErr != nil {
				attachSingboxResult(payload, failedSingboxListSummary(checkErr))
			} else if deletedRule.ID > 0 && affected {
				attachSingboxResult(payload, strictSingboxApply(r.Context(), cfg, store))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case http.MethodGet:
			writeJSONError(w, http.StatusNotFound, "not_found")
		default:
			methodNotAllowed(w)
		}
	}
}

func findRoutingRule(ctx context.Context, store Store, id int64) (db.RoutingRule, bool) {
	rule, found, _ := findRoutingRuleStrict(ctx, store, id)
	return rule, found
}

func findRoutingRuleStrict(ctx context.Context, store Store, id int64) (db.RoutingRule, bool, error) {
	if store == nil {
		return db.RoutingRule{}, false, nil
	}
	rules, err := store.ListRoutingRules(ctx)
	if err != nil {
		return db.RoutingRule{}, false, err
	}
	for _, rule := range rules {
		if rule.ID == id {
			return rule, true, nil
		}
	}
	return db.RoutingRule{}, false, nil
}

func storeHasSingboxInbounds(ctx context.Context, store Store) bool {
	hasSingboxInbounds, _ := storeHasSingboxInboundsStrict(ctx, store)
	return hasSingboxInbounds
}

func storeHasSingboxInboundsStrict(ctx context.Context, store Store) (bool, error) {
	if store == nil {
		return false, nil
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return false, err
	}
	for _, inbound := range inbounds {
		if db.InboundCore(inbound) == db.CoreSingbox {
			return true, nil
		}
	}
	return false, nil
}

func routingChangeAffectsSingbox(ctx context.Context, store Store, rule db.RoutingRule) bool {
	affected, _ := routingChangeAffectsSingboxStrict(ctx, store, rule)
	return affected
}

func routingChangeAffectsSingboxStrict(ctx context.Context, store Store, rule db.RoutingRule) (bool, error) {
	if store == nil {
		return false, nil
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return false, err
	}
	outbounds, err := store.ListOutbounds(ctx)
	if err != nil {
		return false, err
	}
	return db.RoutingRuleAppliesToCore(rule, inbounds, db.CoreSingbox) && db.RuleTargetSupportsCore(rule, outbounds, db.CoreSingbox), nil
}

func failedSingboxListSummary(err error) SingboxApplySummary {
	return SingboxApplySummary{
		Applied:          false,
		Service:          "sing-box",
		ConfigPath:       "/etc/sing-box/config.json",
		CommandsExecuted: []string{},
		Error:            "list_failed",
		Detail:           err.Error(),
	}
}
