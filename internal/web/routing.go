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
			scope, checkErr := loadCoreInboundScope(r.Context(), store)
			if checkErr != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
				return
			}
			rule, err := store.CreateRoutingRule(r.Context(), params)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "create_failed")
				return
			}
			includeXray, includeSingbox := xrayAndSingboxForRoutingRuleWriteWithScope(scope, db.RoutingRule{}, false, rule)
			writeCoreWriteResult(w, r, cfg, store, http.StatusCreated, map[string]interface{}{"rule": rule}, includeXray, includeSingbox)
		default:
			methodNotAllowed(w)
		}
	}
}

func routingRuleChildrenHandler(cfg *routerConfig) http.HandlerFunc {
	store := cfg.store
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
			includeXray, includeSingbox, checkErr := xrayAndSingboxForReorder(r.Context(), store)
			if checkErr != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
				return
			}
			if err := store.ReorderRoutingRules(r.Context(), req.IDs); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "reorder_failed")
				return
			}
			writeCoreWriteResult(w, r, cfg, store, http.StatusOK, map[string]interface{}{"status": "reordered"}, includeXray, includeSingbox)
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
			scope, checkErr := loadCoreInboundScope(r.Context(), store)
			if checkErr != nil {
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
			includeXray, includeSingbox := xrayAndSingboxForRoutingRuleWriteWithScope(scope, previousRule, hadPreviousRule, rule)
			writeCoreWriteResult(w, r, cfg, store, http.StatusOK, map[string]interface{}{"rule": rule}, includeXray, includeSingbox)
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
			scope, checkErr := loadCoreInboundScope(r.Context(), store)
			if checkErr != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
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
			includeXray, includeSingbox := xrayAndSingboxForRoutingRuleDeleteWithScope(scope, deletedRule)
			writeCoreWriteResult(w, r, cfg, store, http.StatusOK, map[string]interface{}{"status": "deleted"}, includeXray, includeSingbox)
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
	return storeHasCoreInbounds(ctx, store, db.CoreSingbox)
}

func storeHasSingboxInboundsStrict(ctx context.Context, store Store) (bool, error) {
	return storeHasCoreInboundsStrict(ctx, store, db.CoreSingbox)
}

func storeHasCoreInbounds(ctx context.Context, store Store, core string) bool {
	hasCoreInbounds, _ := storeHasCoreInboundsStrict(ctx, store, core)
	return hasCoreInbounds
}

func storeHasCoreInboundsStrict(ctx context.Context, store Store, core string) (bool, error) {
	if store == nil {
		return false, nil
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return false, err
	}
	for _, inbound := range inbounds {
		if db.InboundCore(inbound) == core {
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
	return routingChangeAffectsCoreStrict(ctx, store, rule, db.CoreSingbox)
}

func routingChangeAffectsCoreStrict(ctx context.Context, store Store, rule db.RoutingRule, core string) (bool, error) {
	if store == nil {
		return false, nil
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return false, err
	}
	return db.RoutingRuleAppliesToCore(rule, inbounds, core), nil
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
