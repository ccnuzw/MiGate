package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

type TrafficV2Snapshot struct {
	GeneratedAt   string             `json:"generated_at"`
	ObservedAt    string             `json:"observed_at"`
	WindowSeconds float64            `json:"window_seconds"`
	Total         TrafficV2Total     `json:"total"`
	Inbounds      []TrafficV2Inbound `json:"inbounds"`
	Clients       []TrafficV2Client  `json:"clients"`
	Coverage      TrafficV2Coverage  `json:"coverage"`
}

type TrafficV2Patch struct {
	GeneratedAt       string             `json:"generated_at"`
	ObservedAt        string             `json:"observed_at"`
	WindowSeconds     float64            `json:"window_seconds"`
	Total             *TrafficV2Total    `json:"total,omitempty"`
	Inbounds          []TrafficV2Inbound `json:"inbounds,omitempty"`
	RemovedInboundIDs []int64            `json:"removed_inbound_ids,omitempty"`
	Clients           []TrafficV2Client  `json:"clients,omitempty"`
	RemovedClientIDs  []int64            `json:"removed_client_ids,omitempty"`
	Coverage          *TrafficV2Coverage `json:"coverage,omitempty"`
}

type TrafficV2SeriesPoint struct {
	Time      string  `json:"time"`
	Up        int64   `json:"up"`
	Down      int64   `json:"down"`
	Total     int64   `json:"total"`
	RateUp    float64 `json:"rate_up,omitempty"`
	RateDown  float64 `json:"rate_down,omitempty"`
	RateTotal float64 `json:"rate_total,omitempty"`
}

type TrafficV2Total struct {
	Cumulative TrafficCumulativeMetric `json:"cumulative"`
	Realtime   TrafficRealtimeMetric   `json:"realtime"`
}

type TrafficV2Inbound struct {
	ID         int64                   `json:"id"`
	Remark     string                  `json:"remark"`
	Protocol   string                  `json:"protocol"`
	Port       int                     `json:"port"`
	Enabled    bool                    `json:"enabled"`
	Cumulative TrafficCumulativeMetric `json:"cumulative"`
	Realtime   TrafficRealtimeMetric   `json:"realtime"`
}

type TrafficV2Client struct {
	ID           int64                   `json:"id"`
	InboundID    int64                   `json:"inbound_id"`
	Email        string                  `json:"email"`
	Enabled      bool                    `json:"enabled"`
	TrafficLimit int64                   `json:"traffic_limit"`
	ExpiryAt     int64                   `json:"expiry_at"`
	Cumulative   TrafficCumulativeMetric `json:"cumulative"`
	Realtime     TrafficRealtimeMetric   `json:"realtime"`
}

type TrafficV2Coverage struct {
	Overall       string            `json:"overall"`
	Engines       map[string]string `json:"engines"`
	OK            int               `json:"ok"`
	Waiting       int               `json:"waiting"`
	Stale         int               `json:"stale"`
	Unavailable   int               `json:"unavailable"`
	Unsupported   int               `json:"unsupported"`
	Partial       int               `json:"partial"`
	NotConfigured int               `json:"not_configured,omitempty"`
}

var trafficV2StreamInterval = trafficStreamInterval
var trafficV2StreamResyncEvery = 12

func trafficV2SnapshotHandler(store Store, cache *trafficViewCache) http.HandlerFunc {
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
		writeJSON(w, http.StatusOK, buildTrafficV2Snapshot(view))
	}
}

func trafficV2StreamHandler(store Store, cache *trafficViewCache) http.HandlerFunc {
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

		sendSnapshot := func() (TrafficV2Snapshot, bool) {
			view, err := cache.get(r.Context(), store)
			if err != nil {
				encoded, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
				_, _ = fmt.Fprintf(w, "event: stream-error\ndata: %s\n\n", encoded)
				flusher.Flush()
				return TrafficV2Snapshot{}, false
			}
			snapshot := buildTrafficV2Snapshot(view)
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				return TrafficV2Snapshot{}, false
			}
			_, _ = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", encoded)
			flusher.Flush()
			return snapshot, true
		}
		previous, ok := sendSnapshot()
		if !ok {
			return
		}
		ticker := time.NewTicker(trafficV2StreamInterval)
		defer ticker.Stop()
		resyncCountdown := trafficV2StreamResyncEvery
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				view, err := cache.get(r.Context(), store)
				if err != nil {
					encoded, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
					_, _ = fmt.Fprintf(w, "event: stream-error\ndata: %s\n\n", encoded)
					flusher.Flush()
					continue
				}
				current := buildTrafficV2Snapshot(view)
				if resyncCountdown <= 0 {
					encoded, err := json.Marshal(current)
					if err != nil {
						return
					}
					_, _ = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", encoded)
					flusher.Flush()
					previous = current
					resyncCountdown = trafficV2StreamResyncEvery
					continue
				}
				patch, changed := buildTrafficV2Patch(previous, current)
				previous = current
				resyncCountdown--
				if !changed {
					continue
				}
				encoded, err := json.Marshal(patch)
				if err != nil {
					return
				}
				_, _ = fmt.Fprintf(w, "event: patch\ndata: %s\n\n", encoded)
				flusher.Flush()
			}
		}
	}
}

func trafficV2SeriesHandler(store Store) http.HandlerFunc {
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
			scopeType = "inbound"
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
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"series": trafficV2SeriesFromSamples(samples, scopeType, inbounds),
		})
	}
}

func buildTrafficV2Snapshot(view trafficView) TrafficV2Snapshot {
	metrics := buildTrafficMetricSet(view)
	coverage := trafficV2Coverage(buildTrafficCoverage(view.trafficByInbound))
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	inbounds := make([]TrafficV2Inbound, 0, len(view.inbounds))
	clients := []TrafficV2Client{}
	for _, inbound := range view.inbounds {
		inbounds = append(inbounds, TrafficV2Inbound{
			ID:         inbound.ID,
			Remark:     inbound.Remark,
			Protocol:   inbound.Protocol,
			Port:       inbound.Port,
			Enabled:    inbound.Enabled,
			Cumulative: metricOrZero(metrics.InboundCumulative, inbound.ID),
			Realtime:   realtimeMetricOrZero(metrics.InboundRealtime, inbound.ID),
		})
		for _, client := range inbound.Clients {
			clients = append(clients, TrafficV2Client{
				ID:           client.ID,
				InboundID:    inbound.ID,
				Email:        client.Email,
				Enabled:      client.Enabled,
				TrafficLimit: client.TrafficLimit,
				ExpiryAt:     client.ExpiryAt,
				Cumulative:   metricOrZero(metrics.ClientCumulative, client.ID),
				Realtime:     realtimeMetricOrZero(metrics.ClientRealtime, client.ID),
			})
		}
	}
	return TrafficV2Snapshot{
		GeneratedAt:   generatedAt,
		ObservedAt:    metrics.TotalRealtime.ObservedAt,
		WindowSeconds: metrics.TotalRealtime.WindowSeconds,
		Total: TrafficV2Total{
			Cumulative: metrics.TotalCumulative,
			Realtime:   metrics.TotalRealtime,
		},
		Inbounds: inbounds,
		Clients:  clients,
		Coverage: coverage,
	}
}

func trafficV2SeriesFromSamples(samples []db.TrafficSample, scopeType string, inbounds []db.Inbound) []TrafficV2SeriesPoint {
	allowed := selectedTrafficSeriesEngines(samples, scopeType, inbounds)
	byTime := map[string]*TrafficV2SeriesPoint{}
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
			point = &TrafficV2SeriesPoint{Time: sample.SampledAt}
			byTime[sample.SampledAt] = point
			order = append(order, sample.SampledAt)
		}
		point.Up += sample.TotalUp
		point.Down += sample.TotalDown
		point.Total += sample.TotalUp + sample.TotalDown
		point.RateUp += sample.RateUp
		point.RateDown += sample.RateDown
		point.RateTotal += sample.RateUp + sample.RateDown
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339Nano, order[i])
		right, rightErr := time.Parse(time.RFC3339Nano, order[j])
		if leftErr == nil && rightErr == nil {
			return left.Before(right)
		}
		return order[i] < order[j]
	})
	points := make([]TrafficV2SeriesPoint, 0, len(order))
	for _, sampledAt := range order {
		points = append(points, *byTime[sampledAt])
	}
	return points
}

func buildTrafficV2Patch(previous, current TrafficV2Snapshot) (TrafficV2Patch, bool) {
	patch := TrafficV2Patch{
		GeneratedAt:   current.GeneratedAt,
		ObservedAt:    current.ObservedAt,
		WindowSeconds: current.WindowSeconds,
	}
	changed := false
	if !trafficV2TotalsEqual(previous.Total, current.Total) {
		total := current.Total
		patch.Total = &total
		changed = true
	}
	if !trafficV2CoverageEqual(previous.Coverage, current.Coverage) {
		coverage := current.Coverage
		patch.Coverage = &coverage
		changed = true
	}
	if inboundChanges := changedTrafficV2Inbounds(previous.Inbounds, current.Inbounds); len(inboundChanges) > 0 {
		patch.Inbounds = inboundChanges
		changed = true
	}
	if removedInboundIDs := removedTrafficV2InboundIDs(previous.Inbounds, current.Inbounds); len(removedInboundIDs) > 0 {
		patch.RemovedInboundIDs = removedInboundIDs
		changed = true
	}
	if clientChanges := changedTrafficV2Clients(previous.Clients, current.Clients); len(clientChanges) > 0 {
		patch.Clients = clientChanges
		changed = true
	}
	if removedClientIDs := removedTrafficV2ClientIDs(previous.Clients, current.Clients); len(removedClientIDs) > 0 {
		patch.RemovedClientIDs = removedClientIDs
		changed = true
	}
	return patch, changed
}

func changedTrafficV2Inbounds(previous, current []TrafficV2Inbound) []TrafficV2Inbound {
	previousByID := map[int64]TrafficV2Inbound{}
	for _, inbound := range previous {
		previousByID[inbound.ID] = inbound
	}
	changes := make([]TrafficV2Inbound, 0, len(current))
	for _, inbound := range current {
		if prior, ok := previousByID[inbound.ID]; ok && trafficV2InboundEqual(prior, inbound) {
			continue
		}
		changes = append(changes, inbound)
	}
	return changes
}

func changedTrafficV2Clients(previous, current []TrafficV2Client) []TrafficV2Client {
	previousByID := map[int64]TrafficV2Client{}
	for _, client := range previous {
		previousByID[client.ID] = client
	}
	changes := make([]TrafficV2Client, 0, len(current))
	for _, client := range current {
		if prior, ok := previousByID[client.ID]; ok && trafficV2ClientEqual(prior, client) {
			continue
		}
		changes = append(changes, client)
	}
	return changes
}

func removedTrafficV2InboundIDs(previous, current []TrafficV2Inbound) []int64 {
	currentByID := map[int64]struct{}{}
	for _, inbound := range current {
		currentByID[inbound.ID] = struct{}{}
	}
	removed := make([]int64, 0)
	for _, inbound := range previous {
		if _, ok := currentByID[inbound.ID]; !ok {
			removed = append(removed, inbound.ID)
		}
	}
	return removed
}

func removedTrafficV2ClientIDs(previous, current []TrafficV2Client) []int64 {
	currentByID := map[int64]struct{}{}
	for _, client := range current {
		currentByID[client.ID] = struct{}{}
	}
	removed := make([]int64, 0)
	for _, client := range previous {
		if _, ok := currentByID[client.ID]; !ok {
			removed = append(removed, client.ID)
		}
	}
	return removed
}

func trafficV2TotalsEqual(left, right TrafficV2Total) bool {
	return trafficV2MetricEqual(left.Cumulative, right.Cumulative) && trafficV2RealtimeEqual(left.Realtime, right.Realtime)
}

func trafficV2InboundEqual(left, right TrafficV2Inbound) bool {
	return left.ID == right.ID &&
		left.Remark == right.Remark &&
		left.Protocol == right.Protocol &&
		left.Port == right.Port &&
		left.Enabled == right.Enabled &&
		trafficV2MetricEqual(left.Cumulative, right.Cumulative) &&
		trafficV2RealtimeEqual(left.Realtime, right.Realtime)
}

func trafficV2ClientEqual(left, right TrafficV2Client) bool {
	return left.ID == right.ID &&
		left.InboundID == right.InboundID &&
		left.Email == right.Email &&
		left.Enabled == right.Enabled &&
		left.TrafficLimit == right.TrafficLimit &&
		left.ExpiryAt == right.ExpiryAt &&
		trafficV2MetricEqual(left.Cumulative, right.Cumulative) &&
		trafficV2RealtimeEqual(left.Realtime, right.Realtime)
}

func trafficV2MetricEqual(left, right TrafficCumulativeMetric) bool {
	return left.Up == right.Up &&
		left.Down == right.Down &&
		left.Total == right.Total &&
		left.Status == right.Status &&
		left.Source == right.Source &&
		left.Message == right.Message
}

func trafficV2RealtimeEqual(left, right TrafficRealtimeMetric) bool {
	return left.DeltaUp == right.DeltaUp &&
		left.DeltaDown == right.DeltaDown &&
		left.DeltaTotal == right.DeltaTotal &&
		left.RateUp == right.RateUp &&
		left.RateDown == right.RateDown &&
		left.RateTotal == right.RateTotal &&
		left.ObservedAt == right.ObservedAt &&
		left.WindowSeconds == right.WindowSeconds &&
		left.Status == right.Status &&
		left.Source == right.Source &&
		left.Message == right.Message
}

func trafficV2CoverageEqual(left, right TrafficV2Coverage) bool {
	if left.Overall != right.Overall ||
		left.OK != right.OK ||
		left.Waiting != right.Waiting ||
		left.Stale != right.Stale ||
		left.Unavailable != right.Unavailable ||
		left.Unsupported != right.Unsupported ||
		left.Partial != right.Partial ||
		left.NotConfigured != right.NotConfigured ||
		len(left.Engines) != len(right.Engines) {
		return false
	}
	for engine, status := range left.Engines {
		if right.Engines[engine] != status {
			return false
		}
	}
	return true
}

func metricOrZero(metrics map[int64]TrafficCumulativeMetric, id int64) TrafficCumulativeMetric {
	if metric, ok := metrics[id]; ok {
		return metric
	}
	return newTrafficCumulativeMetric(0, 0, "waiting", "migate", "")
}

func realtimeMetricOrZero(metrics map[int64]TrafficRealtimeMetric, id int64) TrafficRealtimeMetric {
	if metric, ok := metrics[id]; ok {
		return metric
	}
	return newTrafficRealtimeMetric(0, 0, 0, 0, 0, "", "waiting", "migate", "")
}

func trafficV2Coverage(raw map[string]interface{}) TrafficV2Coverage {
	return TrafficV2Coverage{
		Overall:       stringValue(raw["overall"]),
		Engines:       trafficCoverageEngines(raw["engines"]),
		OK:            intValue(raw["ok"]),
		Waiting:       intValue(raw["waiting"]),
		Stale:         intValue(raw["stale"]),
		Unavailable:   intValue(raw["unavailable"]),
		Unsupported:   intValue(raw["unsupported"]),
		Partial:       intValue(raw["partial"]),
		NotConfigured: intValue(raw["not_configured"]),
	}
}

func trafficCoverageEngines(value interface{}) map[string]string {
	engines := map[string]string{}
	switch typed := value.(type) {
	case map[string]string:
		for key, status := range typed {
			engines[key] = status
		}
	case map[string]interface{}:
		for key, status := range typed {
			engines[key] = stringValue(status)
		}
	}
	return engines
}

func stringValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func intValue(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
