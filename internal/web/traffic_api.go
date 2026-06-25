package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

const trafficStreamInterval = 5 * time.Second

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
		matched := map[string]struct{}{}
		for expectedEngine := range expectedEngines {
			if _, ok := engines[expectedEngine]; ok {
				matched[expectedEngine] = struct{}{}
			}
		}
		if len(matched) > 0 {
			selected[scopeKey] = matched
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
