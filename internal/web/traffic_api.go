package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

const trafficStreamInterval = 5 * time.Second

func trafficSummaryHandler(store Store, cache *trafficViewCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		view, err := cache.get(r.Context(), store)
		if err != nil {
			writeTrafficViewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, buildTrafficSummaryPayload(view))
	}
}

func trafficInboundsHandler(store Store, cache *trafficViewCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		view, err := cache.get(r.Context(), store)
		if err != nil {
			writeTrafficViewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"inbounds": buildTrafficInboundPayloads(view)})
	}
}

func trafficClientsHandler(store Store, cache *trafficViewCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		view, err := cache.get(r.Context(), store)
		if err != nil {
			writeTrafficViewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"clients": buildTrafficClientPayloads(view)})
	}
}

func trafficStreamHandler(store Store, cache *trafficViewCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSONError(w, http.StatusInternalServerError, "streaming_unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		send := func() bool {
			payload, err := buildTrafficSnapshot(r.Context(), store, cache)
			if err != nil {
				encoded, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
				_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", encoded)
				flusher.Flush()
				return true
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				return false
			}
			_, _ = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", encoded)
			flusher.Flush()
			return true
		}
		if !send() {
			return
		}
		ticker := time.NewTicker(trafficStreamInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if !send() {
					return
				}
			}
		}
	}
}

func buildTrafficSnapshot(ctx context.Context, store Store, cache *trafficViewCache) (map[string]interface{}, error) {
	view, err := cache.get(ctx, store)
	if err != nil {
		return nil, err
	}
	summary := buildTrafficSummaryPayload(view)
	inbounds := buildTrafficInboundPayloads(view)
	clients := buildTrafficClientPayloads(view)
	return map[string]interface{}{
		"summary":      summary,
		"inbounds":     inbounds,
		"clients":      clients,
		"generated_at": summary["generated_at"],
	}, nil
}

func buildTrafficSummaryPayload(view trafficView) map[string]interface{} {
	metrics := buildTrafficMetricSet(view)
	lastSampledAt := metrics.TotalRealtime.ObservedAt
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	coverage := buildTrafficCoverage(view.trafficByInbound)
	totalCumulative := cumulativeMetricPayload(metrics.TotalCumulative)
	totalRealtime := withTrafficCoverage(metrics.TotalRealtime, coverage)
	return map[string]interface{}{
		"total_up": metrics.TotalCumulative.Up, "total_down": metrics.TotalCumulative.Down, "total": metrics.TotalCumulative.Total,
		"rate_up": metrics.TotalRealtime.RateUp, "rate_down": metrics.TotalRealtime.RateDown, "rate_total": metrics.TotalRealtime.RateTotal,
		"delta_up": metrics.TotalRealtime.DeltaUp, "delta_down": metrics.TotalRealtime.DeltaDown, "delta_total": metrics.TotalRealtime.DeltaTotal,
		"window_seconds": metrics.TotalRealtime.WindowSeconds, "observed_at": lastSampledAt,
		"total_traffic":          totalCumulative,
		"realtime_traffic":       totalRealtime,
		"total_cumulative":       totalCumulative,
		"total_realtime":         metrics.TotalRealtime.RateTotal,
		"total_realtime_traffic": totalRealtime,
		"total_realtime_up":      metrics.TotalRealtime.RateUp, "total_realtime_down": metrics.TotalRealtime.RateDown, "total_realtime_rate": metrics.TotalRealtime.RateTotal,
		"status": coverage,
		"engine": trafficViewEngine(view.trafficByInbound),
		"source": "migate", "last_sampled_at": lastSampledAt, "generated_at": generatedAt,
	}
}

func buildTrafficInboundPayloads(view trafficView) []map[string]interface{} {
	metrics := buildTrafficMetricSet(view)
	items := make([]map[string]interface{}, 0, len(view.inbounds))
	for _, inbound := range view.inbounds {
		traffic := view.trafficByInbound[inbound.ID]
		cumulative := cumulativeMetricPayload(metrics.InboundCumulative[inbound.ID])
		realtime := realtimeMetricPayload(metrics.InboundRealtime[inbound.ID])
		items = append(items, map[string]interface{}{
			"id": inbound.ID, "remark": inbound.Remark, "protocol": inbound.Protocol, "port": inbound.Port,
			"total_up": traffic.Up, "total_down": traffic.Down, "total": traffic.Total,
			"rate_up": traffic.RateUp, "rate_down": traffic.RateDown, "rate_total": traffic.RateTotal,
			"delta_up": traffic.DeltaUp, "delta_down": traffic.DeltaDown, "delta_total": traffic.DeltaUp + traffic.DeltaDown,
			"window_seconds": traffic.WindowSeconds, "observed_at": traffic.LastSampledAt,
			"cumulative":         cumulative,
			"realtime":           realtime,
			"inbound_cumulative": cumulative,
			"inbound_realtime":   realtime,
			"status":             traffic.Status, "message": traffic.Message, "engine": traffic.Engine, "source": traffic.Source, "last_sampled_at": traffic.LastSampledAt,
		})
	}
	return items
}

func buildTrafficClientPayloads(view trafficView) []map[string]interface{} {
	metrics := buildTrafficMetricSet(view)
	items := []map[string]interface{}{}
	for _, inbound := range view.inbounds {
		for _, client := range inbound.Clients {
			traffic := view.trafficByClient[client.ID]
			cumulative := cumulativeMetricPayload(metrics.ClientCumulative[client.ID])
			realtime := realtimeMetricPayload(metrics.ClientRealtime[client.ID])
			items = append(items, map[string]interface{}{
				"id": client.ID, "inbound_id": inbound.ID, "email": client.Email, "protocol": inbound.Protocol,
				"total_up": traffic.Up, "total_down": traffic.Down, "total": traffic.Up + traffic.Down,
				"rate_up": traffic.RateUp, "rate_down": traffic.RateDown, "rate_total": traffic.RateTotal,
				"delta_up": traffic.DeltaUp, "delta_down": traffic.DeltaDown, "delta_total": traffic.DeltaUp + traffic.DeltaDown,
				"window_seconds": traffic.WindowSeconds, "observed_at": traffic.LastSampledAt,
				"cumulative":        cumulative,
				"realtime":          realtime,
				"client_cumulative": cumulative,
				"client_realtime":   realtime,
				"traffic_limit":     client.TrafficLimit, "expiry_at": client.ExpiryAt,
				"status": traffic.Status, "message": traffic.Message, "engine": traffic.Engine, "source": traffic.Source, "last_sampled_at": traffic.LastSampledAt,
			})
		}
	}
	return items
}

func withTrafficCoverage(realtime TrafficRealtimeMetric, coverage map[string]interface{}) map[string]interface{} {
	payload := realtimeMetricPayload(realtime)
	payload["coverage"] = coverage
	return payload
}

func trafficViewEngine(byInbound map[int64]inboundTrafficSummary) string {
	seen := map[string]struct{}{}
	for _, traffic := range byInbound {
		engine := normalizeTrafficEngine(traffic.Engine)
		if engine != "" {
			seen[engine] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return "migate"
	}
	if len(seen) > 1 {
		return "mixed"
	}
	for engine := range seen {
		return engine
	}
	return "migate"
}

type trafficView struct {
	inbounds         []db.Inbound
	trafficByInbound map[int64]inboundTrafficSummary
	trafficByClient  map[int64]clientTrafficSummary
}

type trafficViewCache struct {
	ttl       time.Duration
	mu        sync.Mutex
	expiresAt time.Time
	value     trafficView
	hasValue  bool
	now       func() time.Time
}

func newTrafficViewCache(ttl time.Duration) *trafficViewCache {
	return &trafficViewCache{ttl: ttl, now: time.Now}
}

func (c *trafficViewCache) get(ctx context.Context, store Store) (trafficView, error) {
	if store == nil {
		return trafficView{}, fmt.Errorf("store_unavailable")
	}
	if c == nil || c.ttl <= 0 {
		return buildTrafficView(ctx, store)
	}
	now := c.now()
	c.mu.Lock()
	if c.hasValue && now.Before(c.expiresAt) {
		value := c.value
		c.mu.Unlock()
		return value, nil
	}
	c.mu.Unlock()

	value, err := buildTrafficView(ctx, store)
	if err != nil {
		return trafficView{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hasValue && c.now().Before(c.expiresAt) {
		return c.value, nil
	}
	c.value = value
	c.hasValue = true
	c.expiresAt = c.now().Add(c.ttl)
	return value, nil
}

func buildTrafficView(ctx context.Context, store Store) (trafficView, error) {
	inbounds, err := store.ListInboundTraffic(ctx)
	if err != nil {
		return trafficView{}, fmt.Errorf("list_inbounds_failed")
	}
	states, err := store.ListTrafficStates(ctx)
	if err != nil {
		return trafficView{}, fmt.Errorf("list_traffic_states_failed")
	}
	trafficByInbound, trafficByClient := summarizeTrafficFromStates(states, inbounds)
	return trafficView{inbounds: inbounds, trafficByInbound: trafficByInbound, trafficByClient: trafficByClient}, nil
}

func writeTrafficViewError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "store_unavailable":
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		inbounds, err := store.ListInboundTraffic(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed")
			return
		}
		points := trafficSamplesToSeries(samples, scopeType, inbounds)
		writeJSON(w, http.StatusOK, map[string]interface{}{"series": points})
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
