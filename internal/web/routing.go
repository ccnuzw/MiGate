package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/imzyb/MiGate/internal/db"
)

func routingRulesHandler(store Store, ctrl XrayController) http.HandlerFunc {
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
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"rule": rule, "xray": applyResult})
		default:
			methodNotAllowed(w)
		}
	}
}

func routingRuleChildrenHandler(store Store, ctrl XrayController) http.HandlerFunc {
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
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"reordered"}`))
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
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"rule": rule, "xray": applyResult})
		case http.MethodDelete:
			err := store.DeleteRoutingRule(r.Context(), id)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSONError(w, http.StatusNotFound, "not_found")
				} else {
					writeJSONError(w, http.StatusInternalServerError, "delete_failed")
				}
				return
			}
			applyResult := ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "xray": applyResult})
		case http.MethodGet:
			writeJSONError(w, http.StatusNotFound, "not_found")
		default:
			methodNotAllowed(w)
		}
	}
}
