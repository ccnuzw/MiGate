package web

import (
	"context"
	"encoding/json"
	"errors"
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

type TrafficV2AnalyticsResponse struct {
	GeneratedAt   string                    `json:"generated_at"`
	Range         string                    `json:"range"`
	Metric        string                    `json:"metric"`
	ScopeType     string                    `json:"scope_type"`
	Semantics     string                    `json:"semantics,omitempty"`
	BucketSeconds int                       `json:"bucket_seconds"`
	Summary       TrafficV2AnalyticsSummary `json:"summary"`
	Series        []TrafficV2AnalyticsPoint `json:"series"`
	TopClients    []TrafficV2AnalyticsRank  `json:"top_clients"`
	TopInbounds   []TrafficV2AnalyticsRank  `json:"top_inbounds"`
	Heatmap       []TrafficV2HeatmapPoint   `json:"heatmap,omitempty"`
}

type TrafficV2AnalyticsSummary struct {
	Up          int64   `json:"up"`
	Down        int64   `json:"down"`
	Total       int64   `json:"total"`
	RateUp      float64 `json:"rate_up"`
	RateDown    float64 `json:"rate_down"`
	RateTotal   float64 `json:"rate_total"`
	PeakUp      int64   `json:"peak_up"`
	PeakDown    int64   `json:"peak_down"`
	PeakTotal   int64   `json:"peak_total"`
	PeakRate    float64 `json:"peak_rate"`
	PeakAt      string  `json:"peak_at,omitempty"`
	Points      int     `json:"points"`
	HasData     bool    `json:"has_data"`
	EmptyReason string  `json:"empty_reason,omitempty"`
}

type TrafficV2AnalyticsPoint struct {
	Time      string  `json:"time"`
	Up        int64   `json:"up"`
	Down      int64   `json:"down"`
	Total     int64   `json:"total"`
	RateUp    float64 `json:"rate_up"`
	RateDown  float64 `json:"rate_down"`
	RateTotal float64 `json:"rate_total"`
}

type TrafficV2AnalyticsRank struct {
	ID        int64   `json:"id"`
	Label     string  `json:"label"`
	ScopeKey  string  `json:"scope_key,omitempty"`
	Protocol  string  `json:"protocol,omitempty"`
	Up        int64   `json:"up"`
	Down      int64   `json:"down"`
	Total     int64   `json:"total"`
	RateTotal float64 `json:"rate_total"`
}

type TrafficV2HeatmapPoint struct {
	Day   string `json:"day"`
	Hour  int    `json:"hour"`
	Total int64  `json:"total"`
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

type trafficV2AnalyticsParams struct {
	Range         string
	Metric        string
	ScopeType     string
	BucketSeconds int
	Since         time.Time
	Until         time.Time
	Limit         int
	Top           int
}

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

func trafficV2AnalyticsHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
			return
		}
		params, err := parseTrafficV2AnalyticsParams(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		inbounds, err := store.ListInboundTraffic(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed")
			return
		}
		samples, err := loadTrafficAnalyticsSamples(r.Context(), store, params, inbounds)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "traffic_samples_failed")
			return
		}
		writeJSON(w, http.StatusOK, buildTrafficV2Analytics(params, samples, inbounds))
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

func parseTrafficV2AnalyticsParams(r *http.Request) (trafficV2AnalyticsParams, error) {
	query := r.URL.Query()
	rangeKey := strings.ToLower(strings.TrimSpace(query.Get("range")))
	if rangeKey == "" {
		rangeKey = "24h"
	}
	duration, bucketSeconds, err := trafficAnalyticsRange(rangeKey)
	if err != nil {
		return trafficV2AnalyticsParams{}, err
	}
	metric := strings.ToLower(strings.TrimSpace(query.Get("metric")))
	if metric == "" {
		metric = "usage"
	}
	if metric != "usage" && metric != "rate" && metric != "cumulative" {
		return trafficV2AnalyticsParams{}, errors.New("invalid_metric")
	}
	scopeType := strings.ToLower(strings.TrimSpace(query.Get("scope_type")))
	if scopeType == "" || scopeType == "total" {
		scopeType = "inbound"
	}
	if scopeType != "client" && scopeType != "inbound" {
		return trafficV2AnalyticsParams{}, errors.New("invalid_scope_type")
	}
	top := 5
	if rawTop := strings.TrimSpace(query.Get("top")); rawTop != "" {
		parsed, err := strconv.Atoi(rawTop)
		if err != nil || parsed <= 0 || parsed > 10 {
			return trafficV2AnalyticsParams{}, errors.New("invalid_top")
		}
		top = parsed
	}
	now := time.Now().UTC()
	return trafficV2AnalyticsParams{
		Range:         rangeKey,
		Metric:        metric,
		ScopeType:     scopeType,
		BucketSeconds: bucketSeconds,
		Since:         now.Add(-duration),
		Until:         now,
		Top:           top,
	}, nil
}

func trafficAnalyticsRange(rangeKey string) (time.Duration, int, error) {
	switch rangeKey {
	case "1h":
		return time.Hour, 60, nil
	case "24h":
		return 24 * time.Hour, 5 * 60, nil
	case "7d":
		return 7 * 24 * time.Hour, 60 * 60, nil
	case "30d":
		return 30 * 24 * time.Hour, 6 * 60 * 60, nil
	default:
		return 0, 0, errors.New("invalid_range")
	}
}

func loadTrafficAnalyticsSamples(ctx context.Context, store Store, params trafficV2AnalyticsParams, _ []db.Inbound) ([]db.TrafficSample, error) {
	samples := []db.TrafficSample{}
	for _, scopeType := range []string{"inbound", "client"} {
		scopeSamples, err := store.ListTrafficAnalyticsSamples(ctx, db.TrafficAnalyticsSampleParams{
			ScopeType:     scopeType,
			Since:         params.Since,
			Until:         params.Until,
			BucketSeconds: params.BucketSeconds,
		})
		if err != nil {
			return nil, err
		}
		samples = append(samples, scopeSamples...)
	}
	return samples, nil
}

func buildTrafficV2Analytics(params trafficV2AnalyticsParams, samples []db.TrafficSample, inbounds []db.Inbound) TrafficV2AnalyticsResponse {
	clientSamples := filterTrafficAnalyticsSamples(samples, "client", inbounds)
	inboundSamples := filterTrafficAnalyticsSamples(samples, "inbound", inbounds)
	effectiveInboundSamples := trafficAnalyticsInboundSamplesWithClientFallback(inboundSamples, clientSamples, inbounds)
	seriesSamples := clientSamples
	if params.ScopeType == "inbound" {
		seriesSamples = effectiveInboundSamples
	}
	series := bucketTrafficAnalyticsSeries(seriesSamples, params.BucketSeconds, params.Metric)
	summary := summarizeTrafficAnalyticsSeries(series, params.Metric)
	if len(series) == 0 {
		summary.EmptyReason = "waiting"
	}
	return TrafficV2AnalyticsResponse{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Range:         params.Range,
		Metric:        params.Metric,
		ScopeType:     params.ScopeType,
		Semantics:     "historical_samples",
		BucketSeconds: params.BucketSeconds,
		Summary:       summary,
		Series:        series,
		TopClients:    rankTrafficAnalyticsSamples(clientSamples, "client", inbounds, params.Top, params.Metric),
		TopInbounds:   rankTrafficAnalyticsSamples(effectiveInboundSamples, "inbound", inbounds, params.Top, params.Metric),
		Heatmap:       heatmapTrafficAnalytics(series),
	}
}

func trafficAnalyticsInboundSamplesWithClientFallback(inboundSamples, clientSamples []db.TrafficSample, inbounds []db.Inbound) []db.TrafficSample {
	if len(clientSamples) == 0 {
		return inboundSamples
	}
	type aggregateKey struct {
		SampledAt string
		Engine    string
		ScopeKey  string
	}
	nativeInboundKeys := map[aggregateKey]bool{}
	for _, sample := range inboundSamples {
		if normalizeTrafficScopeType(sample.ScopeType) != "inbound" {
			continue
		}
		scopeKey := strings.TrimSpace(sample.ScopeKey)
		engine := normalizeTrafficEngine(sample.Engine)
		if scopeKey != "" && engine != "" && trafficSampleCanAggregate(sample) {
			nativeInboundKeys[aggregateKey{SampledAt: sample.SampledAt, Engine: engine, ScopeKey: scopeKey}] = true
		}
	}
	type clientInboundMeta struct {
		InboundKey string
		Engine     string
	}
	clientKeysByInbound := map[string][]string{}
	clientMetaByKey := map[string]clientInboundMeta{}
	for _, inbound := range inbounds {
		inboundKey := inboundStatsKey(inbound)
		engine := expectedTrafficEngine(inbound.Protocol)
		for _, client := range inbound.Clients {
			if !client.Enabled {
				continue
			}
			clientKey := clientTrafficStatsKey(client)
			if clientKey == "" {
				continue
			}
			clientMetaByKey[engine+"\x00"+clientKey] = clientInboundMeta{InboundKey: inboundKey, Engine: engine}
			clientKeysByInbound[engine+"\x00"+inboundKey] = append(clientKeysByInbound[engine+"\x00"+inboundKey], clientKey)
		}
	}
	clientStatusByPoint := map[aggregateKey]map[string]string{}
	aggregates := map[aggregateKey]*db.TrafficSample{}
	order := []aggregateKey{}
	for _, sample := range clientSamples {
		sampleEngine := normalizeTrafficEngine(sample.Engine)
		meta, ok := clientMetaByKey[sampleEngine+"\x00"+sample.ScopeKey]
		if !ok {
			continue
		}
		key := aggregateKey{SampledAt: sample.SampledAt, Engine: meta.Engine, ScopeKey: meta.InboundKey}
		if nativeInboundKeys[key] {
			continue
		}
		statusByClient := clientStatusByPoint[key]
		if statusByClient == nil {
			statusByClient = map[string]string{}
			clientStatusByPoint[key] = statusByClient
		}
		statusByClient[sample.ScopeKey] = sample.Status
		if !trafficSampleCanAggregate(sample) {
			continue
		}
		aggregate := aggregates[key]
		if aggregate == nil {
			aggregate = &db.TrafficSample{
				SampledAt: sample.SampledAt,
				Engine:    meta.Engine,
				ScopeType: "inbound",
				ScopeKey:  meta.InboundKey,
				Status:    sample.Status,
			}
			aggregates[key] = aggregate
			order = append(order, key)
		}
		aggregate.TotalUp += sample.TotalUp
		aggregate.TotalDown += sample.TotalDown
		aggregate.DeltaUp += sample.DeltaUp
		aggregate.DeltaDown += sample.DeltaDown
		aggregate.RateUp += sample.RateUp
		aggregate.RateDown += sample.RateDown
		aggregate.WindowSeconds = maxFloat64(aggregate.WindowSeconds, sample.WindowSeconds)
		aggregate.Status = combineTrafficStatuses(aggregate.Status, sample.Status)
	}
	if len(aggregates) == 0 {
		return inboundSamples
	}
	for key, aggregate := range aggregates {
		status := ""
		knownStatuses := clientStatusByPoint[key]
		for _, clientKey := range clientKeysByInbound[key.Engine+"\x00"+key.ScopeKey] {
			clientStatus := "waiting"
			if knownStatuses != nil && strings.TrimSpace(knownStatuses[clientKey]) != "" {
				clientStatus = knownStatuses[clientKey]
			}
			status = combineTrafficStatuses(status, clientStatus)
		}
		if status != "" {
			aggregate.Status = status
		}
	}
	aggregateScopes := map[string]bool{}
	for key := range aggregates {
		aggregateScopes[key.SampledAt+"\x00"+key.Engine+"\x00"+key.ScopeKey] = true
	}
	result := make([]db.TrafficSample, 0, len(inboundSamples)+len(aggregates))
	for _, sample := range inboundSamples {
		scopeKey := sample.SampledAt + "\x00" + normalizeTrafficEngine(sample.Engine) + "\x00" + strings.TrimSpace(sample.ScopeKey)
		if aggregateScopes[scopeKey] && !trafficSampleCanAggregate(sample) {
			continue
		}
		result = append(result, sample)
	}
	for _, key := range order {
		result = append(result, *aggregates[key])
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].SampledAt == result[j].SampledAt {
			if result[i].Engine == result[j].Engine {
				return result[i].ScopeKey < result[j].ScopeKey
			}
			return result[i].Engine < result[j].Engine
		}
		return result[i].SampledAt < result[j].SampledAt
	})
	return result
}

func trafficSampleCanAggregate(sample db.TrafficSample) bool {
	switch strings.ToLower(strings.TrimSpace(sample.Status)) {
	case "", "ok", "partial":
		return true
	default:
		return false
	}
}

func maxFloat64(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}

func filterTrafficAnalyticsSamples(samples []db.TrafficSample, scopeType string, inbounds []db.Inbound) []db.TrafficSample {
	allowed := selectedTrafficSeriesEngines(samples, scopeType, inbounds)
	filtered := make([]db.TrafficSample, 0, len(samples))
	for _, sample := range samples {
		if sample.ScopeType != "" && normalizeTrafficScopeType(sample.ScopeType) != scopeType {
			continue
		}
		if scopeType == "client" || scopeType == "inbound" {
			engines, ok := allowed[sample.ScopeKey]
			if !ok {
				continue
			}
			if _, ok := engines[normalizeTrafficEngine(sample.Engine)]; !ok {
				continue
			}
		}
		filtered = append(filtered, sample)
	}
	return filtered
}

func bucketTrafficAnalyticsSeries(samples []db.TrafficSample, bucketSeconds int, metric string) []TrafficV2AnalyticsPoint {
	if bucketSeconds <= 0 {
		bucketSeconds = 300
	}
	type cumulativeValue struct {
		Up   int64
		Down int64
	}
	type bucketValue struct {
		Time       time.Time
		Up         int64
		Down       int64
		RateUp     float64
		RateDown   float64
		RateCount  int
		Cumulative map[string]cumulativeValue
	}
	buckets := map[int64]*bucketValue{}
	order := []int64{}
	for _, sample := range samples {
		if !trafficSampleCanAggregate(sample) {
			continue
		}
		sampledAt, err := time.Parse(time.RFC3339Nano, sample.SampledAt)
		if err != nil {
			continue
		}
		bucketUnix := sampledAt.UTC().Unix() / int64(bucketSeconds) * int64(bucketSeconds)
		bucket := buckets[bucketUnix]
		if bucket == nil {
			bucket = &bucketValue{Time: time.Unix(bucketUnix, 0).UTC(), Cumulative: map[string]cumulativeValue{}}
			buckets[bucketUnix] = bucket
			order = append(order, bucketUnix)
		}
		if metric == "cumulative" {
			bucket.Cumulative[sample.ScopeKey] = cumulativeValue{Up: sample.TotalUp, Down: sample.TotalDown}
		} else if metric == "usage" {
			bucket.Up += sample.DeltaUp
			bucket.Down += sample.DeltaDown
		}
		bucket.RateUp += sample.RateUp
		bucket.RateDown += sample.RateDown
		bucket.RateCount++
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	points := make([]TrafficV2AnalyticsPoint, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		up, down := bucket.Up, bucket.Down
		if metric == "cumulative" {
			for _, cumulative := range bucket.Cumulative {
				up += cumulative.Up
				down += cumulative.Down
			}
		}
		rateDivisor := float64(bucket.RateCount)
		if rateDivisor <= 0 {
			rateDivisor = 1
		}
		point := TrafficV2AnalyticsPoint{
			Time:      bucket.Time.Format(time.RFC3339),
			Up:        up,
			Down:      down,
			Total:     up + down,
			RateUp:    bucket.RateUp / rateDivisor,
			RateDown:  bucket.RateDown / rateDivisor,
			RateTotal: (bucket.RateUp + bucket.RateDown) / rateDivisor,
		}
		points = append(points, point)
	}
	return points
}

func summarizeTrafficAnalyticsSeries(series []TrafficV2AnalyticsPoint, metric string) TrafficV2AnalyticsSummary {
	summary := TrafficV2AnalyticsSummary{Points: len(series), HasData: len(series) > 0}
	if metric == "cumulative" {
		if len(series) == 0 {
			return summary
		}
		last := series[len(series)-1]
		summary.Up = last.Up
		summary.Down = last.Down
		summary.Total = last.Total
		summary.RateUp = last.RateUp
		summary.RateDown = last.RateDown
		summary.RateTotal = last.RateTotal
		for _, point := range series {
			if point.Total > summary.PeakTotal || (point.Total == summary.PeakTotal && point.RateTotal > summary.PeakRate) {
				summary.PeakUp = point.Up
				summary.PeakDown = point.Down
				summary.PeakTotal = point.Total
				summary.PeakRate = point.RateTotal
				summary.PeakAt = point.Time
			}
		}
		return summary
	}
	for _, point := range series {
		summary.Up += point.Up
		summary.Down += point.Down
		summary.Total += point.Total
		summary.RateUp += point.RateUp
		summary.RateDown += point.RateDown
		summary.RateTotal += point.RateTotal
		if point.Total > summary.PeakTotal || (point.Total == summary.PeakTotal && point.RateTotal > summary.PeakRate) {
			summary.PeakUp = point.Up
			summary.PeakDown = point.Down
			summary.PeakTotal = point.Total
			summary.PeakRate = point.RateTotal
			summary.PeakAt = point.Time
		}
	}
	if len(series) > 0 {
		divisor := float64(len(series))
		summary.RateUp /= divisor
		summary.RateDown /= divisor
		summary.RateTotal /= divisor
	}
	return summary
}

func rankTrafficAnalyticsSamples(samples []db.TrafficSample, scopeType string, inbounds []db.Inbound, limit int, metric string) []TrafficV2AnalyticsRank {
	if limit <= 0 {
		limit = 5
	}
	if scopeType == "client" {
		return rankTrafficAnalyticsClients(samples, inbounds, limit, metric)
	}
	return rankTrafficAnalyticsInbounds(samples, inbounds, limit, metric)
}

type trafficAnalyticsRankAggregate struct {
	rank      TrafficV2AnalyticsRank
	rateSum   float64
	rateCount int
	latestAt  string
}

func (aggregate *trafficAnalyticsRankAggregate) addSample(sample db.TrafficSample, metric string) {
	if metric == "cumulative" {
		if sample.SampledAt >= aggregate.latestAt {
			aggregate.latestAt = sample.SampledAt
			aggregate.rank.Up = sample.TotalUp
			aggregate.rank.Down = sample.TotalDown
			aggregate.rank.Total = sample.TotalUp + sample.TotalDown
		}
	} else {
		aggregate.rank.Up += sample.DeltaUp
		aggregate.rank.Down += sample.DeltaDown
		aggregate.rank.Total += sample.DeltaUp + sample.DeltaDown
	}
	aggregate.rateSum += sample.RateUp + sample.RateDown
	aggregate.rateCount++
	aggregate.rank.RateTotal = aggregate.rateSum / float64(aggregate.rateCount)
}

func rankTrafficAnalyticsClients(samples []db.TrafficSample, inbounds []db.Inbound, limit int, metric string) []TrafficV2AnalyticsRank {
	type clientMeta struct {
		ID       int64
		Label    string
		Protocol string
	}
	metaByKey := map[string]clientMeta{}
	for _, inbound := range inbounds {
		engine := expectedTrafficEngine(inbound.Protocol)
		for _, client := range inbound.Clients {
			clientKey := clientTrafficStatsKey(client)
			if clientKey == "" {
				continue
			}
			metaByKey[engine+"\x00"+clientKey] = clientMeta{ID: client.ID, Label: client.Email, Protocol: inbound.Protocol}
		}
	}
	byKey := map[string]*trafficAnalyticsRankAggregate{}
	for _, sample := range samples {
		if normalizeTrafficScopeType(sample.ScopeType) != "client" {
			continue
		}
		if !trafficSampleCanAggregate(sample) {
			continue
		}
		engine := normalizeTrafficEngine(sample.Engine)
		rankKey := engine + "\x00" + sample.ScopeKey
		meta, ok := metaByKey[rankKey]
		if !ok {
			continue
		}
		aggregate := byKey[rankKey]
		if aggregate == nil {
			aggregate = &trafficAnalyticsRankAggregate{rank: TrafficV2AnalyticsRank{ID: meta.ID, Label: meta.Label, ScopeKey: sample.ScopeKey, Protocol: meta.Protocol}}
			byKey[rankKey] = aggregate
		}
		aggregate.addSample(sample, metric)
	}
	return sortedTrafficAnalyticsRanks(byKey, limit, metric)
}

func rankTrafficAnalyticsInbounds(samples []db.TrafficSample, inbounds []db.Inbound, limit int, metric string) []TrafficV2AnalyticsRank {
	metaByKey := map[string]TrafficV2AnalyticsRank{}
	for _, inbound := range inbounds {
		metaByKey[inboundStatsKey(inbound)] = TrafficV2AnalyticsRank{ID: inbound.ID, Label: inbound.Remark, ScopeKey: inboundStatsKey(inbound), Protocol: inbound.Protocol}
	}
	byKey := map[string]*trafficAnalyticsRankAggregate{}
	for _, sample := range samples {
		if normalizeTrafficScopeType(sample.ScopeType) != "inbound" {
			continue
		}
		if !trafficSampleCanAggregate(sample) {
			continue
		}
		meta, ok := metaByKey[sample.ScopeKey]
		if !ok {
			continue
		}
		aggregate := byKey[sample.ScopeKey]
		if aggregate == nil {
			aggregate = &trafficAnalyticsRankAggregate{rank: meta}
			byKey[sample.ScopeKey] = aggregate
		}
		aggregate.addSample(sample, metric)
	}
	return sortedTrafficAnalyticsRanks(byKey, limit, metric)
}

func sortedTrafficAnalyticsRanks(byKey map[string]*trafficAnalyticsRankAggregate, limit int, metric string) []TrafficV2AnalyticsRank {
	ranks := make([]TrafficV2AnalyticsRank, 0, len(byKey))
	for _, aggregate := range byKey {
		ranks = append(ranks, aggregate.rank)
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		if metric == "rate" && ranks[i].RateTotal != ranks[j].RateTotal {
			return ranks[i].RateTotal > ranks[j].RateTotal
		}
		if ranks[i].Total == ranks[j].Total {
			return ranks[i].Label < ranks[j].Label
		}
		return ranks[i].Total > ranks[j].Total
	})
	if len(ranks) > limit {
		ranks = ranks[:limit]
	}
	return ranks
}

func heatmapTrafficAnalytics(series []TrafficV2AnalyticsPoint) []TrafficV2HeatmapPoint {
	type heatmapBucket struct {
		Day   string
		Hour  int
		Total int64
	}
	byKey := map[string]*heatmapBucket{}
	order := []string{}
	for _, point := range series {
		parsed, err := time.Parse(time.RFC3339Nano, point.Time)
		if err != nil {
			continue
		}
		day := parsed.UTC().Format("2006-01-02")
		hour := parsed.UTC().Hour()
		key := fmt.Sprintf("%s-%02d", day, hour)
		bucket := byKey[key]
		if bucket == nil {
			bucket = &heatmapBucket{Day: day, Hour: hour}
			byKey[key] = bucket
			order = append(order, key)
		}
		bucket.Total += point.Total
	}
	sort.Strings(order)
	points := make([]TrafficV2HeatmapPoint, 0, len(order))
	for _, key := range order {
		bucket := byKey[key]
		points = append(points, TrafficV2HeatmapPoint{Day: bucket.Day, Hour: bucket.Hour, Total: bucket.Total})
	}
	return points
}

func normalizeTrafficScopeType(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_")))
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
