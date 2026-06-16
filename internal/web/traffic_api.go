package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

func trafficSummaryHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		inbounds, trafficByInbound, _, ok := loadTrafficView(w, r, store)
		if !ok {
			return
		}
		var totalUp int64
		var totalDown int64
		var rateUp float64
		var rateDown float64
		for _, inbound := range inbounds {
			traffic := trafficByInbound[inbound.ID]
			totalUp += traffic.Up
			totalDown += traffic.Down
			rateUp += traffic.RateUp
			rateDown += traffic.RateDown
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"total_up":        totalUp,
			"total_down":      totalDown,
			"total":           totalUp + totalDown,
			"rate_up":         rateUp,
			"rate_down":       rateDown,
			"rate_total":      rateUp + rateDown,
			"status":          buildTrafficCoverage(trafficByInbound),
			"last_updated_at": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func trafficInboundsHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		inbounds, trafficByInbound, _, ok := loadTrafficView(w, r, store)
		if !ok {
			return
		}
		items := make([]map[string]interface{}, 0, len(inbounds))
		for _, inbound := range inbounds {
			traffic := trafficByInbound[inbound.ID]
			items = append(items, map[string]interface{}{
				"id":              inbound.ID,
				"remark":          inbound.Remark,
				"protocol":        inbound.Protocol,
				"port":            inbound.Port,
				"total_up":        traffic.Up,
				"total_down":      traffic.Down,
				"total":           traffic.Total,
				"rate_up":         traffic.RateUp,
				"rate_down":       traffic.RateDown,
				"status":          traffic.Status,
				"message":         traffic.Message,
				"engine":          traffic.Engine,
				"last_updated_at": time.Now().UTC().Format(time.RFC3339),
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"inbounds": items})
	}
}

func trafficClientsHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		inbounds, _, trafficByClient, ok := loadTrafficView(w, r, store)
		if !ok {
			return
		}
		items := []map[string]interface{}{}
		for _, inbound := range inbounds {
			for _, client := range inbound.Clients {
				traffic := trafficByClient[client.ID]
				items = append(items, map[string]interface{}{
					"id":              client.ID,
					"inbound_id":      inbound.ID,
					"email":           client.Email,
					"protocol":        inbound.Protocol,
					"total_up":        traffic.Up,
					"total_down":      traffic.Down,
					"total":           traffic.Up + traffic.Down,
					"rate_up":         traffic.RateUp,
					"rate_down":       traffic.RateDown,
					"traffic_limit":   client.TrafficLimit,
					"expiry_at":       client.ExpiryAt,
					"status":          traffic.Status,
					"message":         traffic.Message,
					"engine":          traffic.Engine,
					"last_updated_at": time.Now().UTC().Format(time.RFC3339),
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"clients": items})
	}
}

func trafficSeriesHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
			return
		}
		scopeType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope_type")))
		if scopeType == "" {
			scopeType = "client"
		}
		if scopeType != "client" && scopeType != "inbound" && scopeType != "outbound" && scopeType != "core" {
			writeJSONError(w, http.StatusBadRequest, "invalid_scope_type")
			return
		}
		since := time.Now().UTC().Add(-24 * time.Hour)
		if rawSince := strings.TrimSpace(r.URL.Query().Get("since")); rawSince != "" {
			parsed, err := time.Parse(time.RFC3339, rawSince)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_since")
				return
			}
			since = parsed.UTC()
		}
		limit := 2000
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed <= 0 || parsed > 2000 {
				writeJSONError(w, http.StatusBadRequest, "invalid_limit")
				return
			}
			limit = parsed
		}
		samples, err := store.ListTrafficSamples(r.Context(), scopeType, since, limit)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "traffic_samples_failed")
			return
		}
		inbounds, err := store.ListInbounds(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed")
			return
		}
		points := trafficSamplesToSeries(samples, scopeType, inbounds)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"series": points})
	}
}

func trafficSamplesToSeries(samples []db.TrafficSample, scopeType string, inbounds []db.Inbound) []trafficSeriesPoint {
	allowed := selectedTrafficSeriesEngines(samples, scopeType, inbounds)
	byTime := map[string]*trafficSeriesPoint{}
	order := []string{}
	for _, sample := range samples {
		if scopeType == "client" || scopeType == "inbound" {
			engines, ok := allowed[sample.ScopeKey]
			if !ok {
				continue
			}
			if _, ok := engines[normalizeTrafficEngine(sample.Engine)]; !ok {
				continue
			}
		}
		point := byTime[sample.SampledAt]
		if point == nil {
			point = &trafficSeriesPoint{Name: sample.SampledAt, Time: sample.SampledAt}
			byTime[sample.SampledAt] = point
			order = append(order, sample.SampledAt)
		}
		point.Up += sample.TotalUp
		point.Down += sample.TotalDown
		point.RateUp += sample.RateUp
		point.RateDown += sample.RateDown
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339Nano, order[i])
		right, rightErr := time.Parse(time.RFC3339Nano, order[j])
		if leftErr == nil && rightErr == nil {
			return left.Before(right)
		}
		return order[i] < order[j]
	})
	points := make([]trafficSeriesPoint, 0, len(order))
	for _, sampledAt := range order {
		points = append(points, *byTime[sampledAt])
	}
	return points
}

func selectedTrafficSeriesEngines(samples []db.TrafficSample, scopeType string, inbounds []db.Inbound) map[string]map[string]struct{} {
	expectedByScope := expectedTrafficSeriesEngines(scopeType, inbounds)
	sampleEngines := map[string]map[string]struct{}{}
	for _, sample := range samples {
		if strings.TrimSpace(sample.ScopeKey) == "" {
			continue
		}
		engine := normalizeTrafficEngine(sample.Engine)
		if engine == "" {
			continue
		}
		engines := sampleEngines[sample.ScopeKey]
		if engines == nil {
			engines = map[string]struct{}{}
			sampleEngines[sample.ScopeKey] = engines
		}
		engines[engine] = struct{}{}
	}
	selected := map[string]map[string]struct{}{}
	for scopeKey, expectedEngines := range expectedByScope {
		if len(expectedEngines) == 0 {
			continue
		}
		engines := sampleEngines[scopeKey]
		for expectedEngine := range expectedEngines {
			if _, ok := engines[expectedEngine]; ok {
				selected[scopeKey] = map[string]struct{}{expectedEngine: {}}
				break
			}
			for _, fallback := range fallbackTrafficEngines(expectedEngine) {
				if _, ok := engines[fallback]; ok {
					selected[scopeKey] = map[string]struct{}{fallback: {}}
					break
				}
			}
			break
		}
	}
	return selected
}

func expectedTrafficSeriesEngines(scopeType string, inbounds []db.Inbound) map[string]map[string]struct{} {
	allowed := map[string]map[string]struct{}{}
	add := func(scopeKey, engine string) {
		scopeKey = strings.TrimSpace(scopeKey)
		engine = normalizeTrafficEngine(engine)
		if scopeKey == "" || engine == "" {
			return
		}
		allowed[scopeKey] = map[string]struct{}{engine: {}}
	}
	switch scopeType {
	case "client":
		for _, inbound := range inbounds {
			engine := expectedTrafficEngine(inbound.Protocol)
			for _, client := range inbound.Clients {
				add(client.StatsKey, engine)
			}
		}
	case "inbound":
		for _, inbound := range inbounds {
			add(inboundStatsKey(inbound), expectedTrafficEngine(inbound.Protocol))
		}
	}
	return allowed
}

func loadTrafficView(w http.ResponseWriter, r *http.Request, store Store) ([]db.Inbound, map[int64]inboundTrafficSummary, map[int64]clientTrafficSummary, bool) {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return nil, nil, nil, false
	}
	inbounds, err := store.ListInbounds(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed")
		return nil, nil, nil, false
	}
	trafficByInbound, trafficByClient := summarizeTraffic(r.Context(), store, inbounds)
	return inbounds, trafficByInbound, trafficByClient, true
}
