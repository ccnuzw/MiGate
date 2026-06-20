package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	outsub "github.com/imzyb/MiGate/internal/subscription"
)

type SubscriptionFetcher interface {
	Fetch(ctx context.Context, rawURL string, allowPrivate bool) ([]byte, error)
}

type OutboundSubscriptionRefreshResult struct {
	SubscriptionID int64  `json:"subscription_id"`
	Count          int    `json:"count"`
	SkippedCount   int    `json:"skipped_count"`
	LastFetchedAt  string `json:"last_fetched_at,omitempty"`
	Error          string `json:"error,omitempty"`
}

func outboundSubscriptionsHandler(cfg *routerConfig) http.HandlerFunc {
	store := cfg.store
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
			return
		}
		switch r.Method {
		case http.MethodGet:
			subs, err := store.ListOutboundSubscriptions(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_outbound_subscriptions_failed")
				return
			}
			writeJSON(w, http.StatusOK, subs)
		case http.MethodPost:
			if strings.TrimSuffix(r.URL.Path, "/") == "/api/outbound-subscriptions/refresh" {
				refreshAllOutboundSubscriptionsHandler(cfg, w, r)
				return
			}
			if strings.TrimSuffix(r.URL.Path, "/") == "/api/outbound-subscriptions/preview" {
				previewOutboundSubscriptionHandler(cfg, w, r)
				return
			}
			if strings.TrimSuffix(r.URL.Path, "/") == "/api/outbound-subscriptions/reorder" {
				reorderOutboundSubscriptionsHandler(cfg, w, r)
				return
			}
			var params db.CreateOutboundSubscriptionParams
			if err := decodeJSONBody(r, &params); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			sub, err := store.CreateOutboundSubscription(r.Context(), params)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "create_outbound_subscription_failed")
				return
			}
			writeJSON(w, http.StatusCreated, map[string]interface{}{"subscription": sub})
		default:
			methodNotAllowed(w)
		}
	}
}

func outboundSubscriptionChildrenHandler(cfg *routerConfig) http.HandlerFunc {
	store := cfg.store
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
			return
		}
		path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/outbound-subscriptions/"), "/")
		switch path {
		case "refresh":
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			refreshAllOutboundSubscriptionsHandler(cfg, w, r)
			return
		case "preview":
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			previewOutboundSubscriptionHandler(cfg, w, r)
			return
		case "reorder":
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			reorderOutboundSubscriptionsHandler(cfg, w, r)
			return
		}
		refresh := strings.HasSuffix(path, "/refresh")
		if refresh {
			path = strings.TrimSuffix(path, "/refresh")
		}
		id, err := strconv.ParseInt(strings.TrimSpace(path), 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_id")
			return
		}
		if refresh {
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			refreshOneOutboundSubscriptionHandler(cfg, id, w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			var params db.UpdateOutboundSubscriptionParams
			if err := decodeJSONBody(r, &params); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			previous, previousFound, err := store.GetOutboundSubscription(r.Context(), id)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "get_outbound_subscription_failed")
				return
			}
			sub, err := store.UpdateOutboundSubscription(r.Context(), id, params)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSONError(w, http.StatusNotFound, "not_found")
				} else {
					writeJSONError(w, http.StatusBadRequest, "update_outbound_subscription_failed")
				}
				return
			}
			includeXray, includeSingbox, err := xrayAndSingboxForAllOutbounds(r.Context(), store)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
				return
			}
			needsRefresh := previousFound && !previous.Enabled && sub.Enabled
			writeCoreWriteResult(w, r, cfg, store, http.StatusOK, map[string]interface{}{"subscription": sub, "needs_refresh": needsRefresh}, includeXray, includeSingbox)
		case http.MethodDelete:
			includeXray, includeSingbox, err := xrayAndSingboxForAllOutbounds(r.Context(), store)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
				return
			}
			if err := store.DeleteOutboundSubscription(r.Context(), id); err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSONError(w, http.StatusNotFound, "not_found")
				} else {
					writeJSONError(w, http.StatusInternalServerError, "delete_outbound_subscription_failed")
				}
				return
			}
			writeCoreWriteResult(w, r, cfg, store, http.StatusOK, map[string]interface{}{"status": "deleted"}, includeXray, includeSingbox)
		default:
			methodNotAllowed(w)
		}
	}
}

func refreshOneOutboundSubscriptionHandler(cfg *routerConfig, id int64, w http.ResponseWriter, r *http.Request) {
	result, includeXray, includeSingbox, err := refreshOutboundSubscription(r.Context(), cfg.store, id)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "refresh_outbound_subscription_failed", map[string]interface{}{"detail": err.Error()})
		return
	}
	writeCoreWriteResult(w, r, cfg, cfg.store, http.StatusOK, map[string]interface{}{"result": result}, includeXray, includeSingbox)
}

func refreshAllOutboundSubscriptionsHandler(cfg *routerConfig, w http.ResponseWriter, r *http.Request) {
	subs, err := cfg.store.ListOutboundSubscriptions(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list_outbound_subscriptions_failed")
		return
	}
	results := []map[string]interface{}{}
	includeXray := false
	includeSingbox := false
	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}
		result, xrayChanged, singboxChanged, err := refreshOutboundSubscription(r.Context(), cfg.store, sub.ID)
		if err != nil {
			results = append(results, map[string]interface{}{"subscription_id": sub.ID, "error": err.Error()})
			continue
		}
		results = append(results, result)
		includeXray = includeXray || xrayChanged
		includeSingbox = includeSingbox || singboxChanged
	}
	writeCoreWriteResult(w, r, cfg, cfg.store, http.StatusOK, map[string]interface{}{"results": results}, includeXray, includeSingbox)
}

func previewOutboundSubscriptionHandler(cfg *routerConfig, w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL          string `json:"url"`
		AllowPrivate bool   `json:"allow_private"`
		Body         string `json:"body"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	var body []byte
	var err error
	if strings.TrimSpace(req.Body) != "" {
		body = []byte(req.Body)
	} else {
		body, err = outsub.HTTPFetcher{}.Fetch(r.Context(), req.URL, req.AllowPrivate)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "fetch_outbound_subscription_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
	}
	result, err := outsub.ParseLinks(outsub.DecodeBody(body))
	if err != nil {
		if len(result.Nodes) == 0 && len(result.Skipped) > 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{"nodes": result.Nodes, "count": 0, "skipped_count": len(result.Skipped), "skipped": result.Skipped})
			return
		}
		writeJSONError(w, http.StatusBadRequest, "parse_outbound_subscription_failed", map[string]interface{}{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"nodes": result.Nodes, "count": len(result.Nodes), "skipped_count": len(result.Skipped), "skipped": result.Skipped})
}

func reorderOutboundSubscriptionsHandler(cfg *routerConfig, w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSONBody(r, &req); err != nil || len(req.IDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_payload")
		return
	}
	includeXray, includeSingbox, err := xrayAndSingboxForAllOutbounds(r.Context(), cfg.store)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list_failed")
		return
	}
	if err := cfg.store.ReorderOutboundSubscriptions(r.Context(), req.IDs); err != nil {
		writeJSONError(w, http.StatusBadRequest, "reorder_outbound_subscriptions_failed")
		return
	}
	writeCoreWriteResult(w, r, cfg, cfg.store, http.StatusOK, map[string]interface{}{"status": "reordered"}, includeXray, includeSingbox)
}

func refreshOutboundSubscription(ctx context.Context, store Store, id int64) (map[string]interface{}, bool, bool, error) {
	result, includeXray, includeSingbox, err := RefreshOutboundSubscription(ctx, store, nil, id)
	if result == nil {
		return nil, includeXray, includeSingbox, err
	}
	payload := map[string]interface{}{
		"subscription_id": result.SubscriptionID,
		"count":           result.Count,
		"skipped_count":   result.SkippedCount,
		"last_fetched_at": result.LastFetchedAt,
	}
	if result.Error != "" {
		payload["error"] = result.Error
	}
	return payload, includeXray, includeSingbox, err
}

func RefreshOutboundSubscription(ctx context.Context, store Store, fetcher SubscriptionFetcher, id int64) (*OutboundSubscriptionRefreshResult, bool, bool, error) {
	sub, ok, err := store.GetOutboundSubscription(ctx, id)
	if err != nil {
		return nil, false, false, err
	}
	if !ok {
		return nil, false, false, fmt.Errorf("outbound subscription not found: %d", id)
	}
	if !sub.Enabled {
		return nil, false, false, fmt.Errorf("outbound subscription disabled: %d", id)
	}
	if fetcher == nil {
		fetcher = outsub.HTTPFetcher{}
	}
	body, err := fetcher.Fetch(ctx, sub.URL, sub.AllowPrivate)
	if err != nil {
		_ = store.MarkOutboundSubscriptionFetch(ctx, id, time.Now(), err.Error(), nil)
		return nil, false, false, err
	}
	parsed, err := outsub.ParseLinks(outsub.DecodeBody(body))
	if err != nil {
		_ = store.MarkOutboundSubscriptionFetch(ctx, id, time.Now(), err.Error(), nil)
		return nil, false, false, err
	}
	existing, err := store.ListOutbounds(ctx)
	if err != nil {
		return nil, false, false, err
	}
	nodes, identities := outsub.Materialize(id, parsed.Nodes, existing, sub.TagPrefix)
	scope, err := loadCoreInboundScope(ctx, store)
	if err != nil {
		return nil, false, false, err
	}
	_, err = store.MaterializeSubscriptionOutbounds(ctx, id, nodes, identities)
	if err != nil {
		_ = store.MarkOutboundSubscriptionFetch(ctx, id, time.Now(), err.Error(), nil)
		return nil, false, false, err
	}
	lastFetchedAt := time.Now().UTC().Format(time.RFC3339)
	lastErr := ""
	if len(parsed.Skipped) > 0 {
		lastErr = fmt.Sprintf("部分节点跳过：%d 个", len(parsed.Skipped))
		if err := store.MarkOutboundSubscriptionFetch(ctx, id, time.Now(), lastErr, identities); err != nil {
			log.Printf("outbound subscription refresh: failed to record skipped summary for %d: %v", id, err)
		}
	}
	includeXray := scope.hasCore(db.CoreXray)
	includeSingbox := scope.hasCore(db.CoreSingbox)
	return &OutboundSubscriptionRefreshResult{SubscriptionID: id, Count: len(nodes), SkippedCount: len(parsed.Skipped), LastFetchedAt: lastFetchedAt, Error: lastErr}, includeXray, includeSingbox, nil
}

func xrayAndSingboxForAllOutbounds(ctx context.Context, store Store) (bool, bool, error) {
	scope, err := loadCoreInboundScope(ctx, store)
	if err != nil {
		return false, false, err
	}
	return scope.hasCore(db.CoreXray), scope.hasCore(db.CoreSingbox), nil
}
