package web

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

type countingSummaryStore struct {
	inbounds          []db.Inbound
	outbounds         []db.Outbound
	rules             []db.RoutingRule
	states            []db.TrafficState
	listInboundsErr   error
	listOutboundsErr  error
	listRulesErr      error
	listInboundsCalls int
}

func (s *countingSummaryStore) ListInbounds(ctx context.Context) ([]db.Inbound, error) {
	s.listInboundsCalls++
	if s.listInboundsErr != nil {
		return nil, s.listInboundsErr
	}
	return s.inbounds, nil
}

func (s *countingSummaryStore) ListInboundTraffic(ctx context.Context) ([]db.Inbound, error) {
	return s.ListInbounds(ctx)
}

func (s *countingSummaryStore) InboundExists(ctx context.Context, id int64) (bool, error) {
	for _, inbound := range s.inbounds {
		if inbound.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (s *countingSummaryStore) FindInboundByPort(ctx context.Context, port int, excludeID int64) (db.Inbound, bool, error) {
	for _, inbound := range s.inbounds {
		if inbound.Port == port && inbound.ID != excludeID {
			return inbound, true, nil
		}
	}
	return db.Inbound{}, false, nil
}

func (s *countingSummaryStore) ListOutbounds(ctx context.Context) ([]db.Outbound, error) {
	if s.listOutboundsErr != nil {
		return nil, s.listOutboundsErr
	}
	return s.outbounds, nil
}

func (s *countingSummaryStore) ListRoutingRules(ctx context.Context) ([]db.RoutingRule, error) {
	if s.listRulesErr != nil {
		return nil, s.listRulesErr
	}
	return s.rules, nil
}

func (s *countingSummaryStore) GetSubscriptionByClientUUID(ctx context.Context, uuid string) (db.Inbound, db.Client, bool, error) {
	return db.Inbound{}, db.Client{}, false, nil
}
func (s *countingSummaryStore) GetSubscriptionByToken(ctx context.Context, token string) (db.Inbound, db.Client, bool, error) {
	return db.Inbound{}, db.Client{}, false, nil
}

func (s *countingSummaryStore) CreateInbound(ctx context.Context, params db.CreateInboundParams) (db.Inbound, error) {
	return db.Inbound{}, errors.New("not implemented")
}

func (s *countingSummaryStore) CreateOutbound(ctx context.Context, params db.CreateOutboundParams) (db.Outbound, error) {
	return db.Outbound{}, errors.New("not implemented")
}

func (s *countingSummaryStore) UpdateOutbound(ctx context.Context, id int64, params db.UpdateOutboundParams) (db.Outbound, error) {
	return db.Outbound{}, errors.New("not implemented")
}

func (s *countingSummaryStore) DeleteOutbound(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) ReorderOutbounds(ctx context.Context, ids []int64) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) CreateRoutingRule(ctx context.Context, params db.CreateRoutingRuleParams) (db.RoutingRule, error) {
	return db.RoutingRule{}, errors.New("not implemented")
}

func (s *countingSummaryStore) UpdateRoutingRule(ctx context.Context, id int64, params db.UpdateRoutingRuleParams) (db.RoutingRule, error) {
	return db.RoutingRule{}, errors.New("not implemented")
}

func (s *countingSummaryStore) DeleteRoutingRule(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) ReorderRoutingRules(ctx context.Context, ids []int64) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) CreateClient(ctx context.Context, params db.CreateClientParams) (db.Client, error) {
	return db.Client{}, errors.New("not implemented")
}

func (s *countingSummaryStore) DeleteInbound(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) DeleteClient(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) UpdateInbound(ctx context.Context, id int64, params db.UpdateInboundParams) (db.Inbound, error) {
	return db.Inbound{}, errors.New("not implemented")
}

func (s *countingSummaryStore) UpdateClient(ctx context.Context, id int64, params db.UpdateClientParams) (db.Client, error) {
	return db.Client{}, errors.New("not implemented")
}

func (s *countingSummaryStore) SetInboundEnabled(ctx context.Context, id int64, enabled bool) (db.Inbound, error) {
	return db.Inbound{}, errors.New("not implemented")
}

func (s *countingSummaryStore) SetOutboundEnabled(ctx context.Context, id int64, enabled bool) (db.Outbound, error) {
	return db.Outbound{}, errors.New("not implemented")
}

func (s *countingSummaryStore) SetClientEnabled(ctx context.Context, inboundID int64, id int64, enabled bool) (db.Client, error) {
	return db.Client{}, errors.New("not implemented")
}

func (s *countingSummaryStore) ResetClientTraffic(ctx context.Context, id int64) (db.Client, error) {
	return db.Client{}, errors.New("not implemented")
}

func (s *countingSummaryStore) ResetClientTrafficBaseline(ctx context.Context, id int64, baselines []db.TrafficRawStat) (db.Client, error) {
	return db.Client{}, errors.New("not implemented")
}

func (s *countingSummaryStore) GetClientTrafficUsage(ctx context.Context, statsKey string) (db.ClientTrafficUsage, bool, error) {
	return db.ClientTrafficUsage{}, false, nil
}

func (s *countingSummaryStore) GetClientTrafficUsageForClient(ctx context.Context, clientID int64) (db.ClientTrafficUsage, bool, error) {
	return db.ClientTrafficUsage{}, false, nil
}

func (s *countingSummaryStore) ListTrafficStates(ctx context.Context) ([]db.TrafficState, error) {
	return s.states, nil
}

func (s *countingSummaryStore) ListTrafficSamples(ctx context.Context, scopeType string, since time.Time, limit int) ([]db.TrafficSample, error) {
	return nil, nil
}

func (s *countingSummaryStore) ApplyTrafficRawStats(ctx context.Context, stats []db.TrafficRawStat, observedAt time.Time) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) MarkTrafficUnavailable(ctx context.Context, engine, status, message string, observedAt time.Time) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) AddToBlacklist(ctx context.Context, tokenHash string, expiresAt time.Time, revoked bool) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) IsBlacklisted(ctx context.Context, tokenHash string) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *countingSummaryStore) RecordSessionTouch(ctx context.Context, tokenHash string) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) PruneActiveSessions(ctx context.Context, maxActive int) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) ListActiveSessions(ctx context.Context) ([]db.BlacklistedSession, error) {
	return nil, errors.New("not implemented")
}

func (s *countingSummaryStore) RevokeSession(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) RevokeOtherSessions(ctx context.Context, currentTokenHash string) (int64, error) {
	return 0, errors.New("not implemented")
}

func TestDashboardSummaryCacheHitsExpiresAndRetriesErrors(t *testing.T) {
	store := &countingSummaryStore{
		inbounds: []db.Inbound{{
			ID:       1,
			Remark:   "cached",
			Protocol: "vless",
			Port:     443,
			Network:  "tcp",
			Security: "none",
			Enabled:  true,
			Clients:  []db.Client{{ID: 10, InboundID: 1, UUID: "uuid", Email: "a@example.com", Enabled: true}},
		}},
		outbounds: []db.Outbound{{ID: 1, Tag: "direct", Protocol: "freedom", Enabled: true}},
	}
	cache := newDashboardSummaryCache(5 * time.Second)
	now := time.Unix(100, 0)
	cache.now = func() time.Time { return now }

	first, err := cache.get(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("first summary: %v", err)
	}
	second, err := cache.get(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("cached summary: %v", err)
	}
	if store.listInboundsCalls != 1 {
		t.Fatalf("expected first build only before expiry, got %d ListInbounds calls", store.listInboundsCalls)
	}
	if first["generated_at"] != second["generated_at"] {
		t.Fatalf("expected cached generated_at to be reused: first=%v second=%v", first["generated_at"], second["generated_at"])
	}

	now = now.Add(6 * time.Second)
	if _, err := cache.get(context.Background(), store, nil); err != nil {
		t.Fatalf("expired summary: %v", err)
	}
	if store.listInboundsCalls != 2 {
		t.Fatalf("expected cache expiry to rebuild summary, got %d ListInbounds calls", store.listInboundsCalls)
	}

	failing := &countingSummaryStore{listInboundsErr: errors.New("boom")}
	failingCache := newDashboardSummaryCache(5 * time.Second)
	if _, err := failingCache.get(context.Background(), failing, nil); err == nil {
		t.Fatal("expected error from empty cache")
	}
	if _, err := failingCache.get(context.Background(), failing, nil); err == nil {
		t.Fatal("expected retry to call failing store again")
	}
	if failing.listInboundsCalls != 2 {
		t.Fatalf("expected failed summaries not to be cached, got %d calls", failing.listInboundsCalls)
	}
}

func TestSummarizeTrafficSelectsExpectedEngineForSharedStatsKey(t *testing.T) {
	store := &countingSummaryStore{
		states: []db.TrafficState{
			{Engine: "xray", ScopeType: "client", ScopeKey: "c_state", TotalUp: 10, TotalDown: 20, RateUp: 1, RateDown: 2, Status: "ok", LastSeenAt: "2026-06-16T00:00:00Z"},
			{Engine: "singbox", ScopeType: "client", ScopeKey: "c_state", TotalUp: 30, TotalDown: 40, RateUp: 3, RateDown: 4, Status: "ok", LastSeenAt: "2026-06-16T00:01:00Z"},
		},
	}
	inbounds := []db.Inbound{
		{ID: 1, Protocol: "vless", Enabled: true, Clients: []db.Client{{ID: 10, StatsKey: "c_state", Email: "xray@example.com", Enabled: true}}},
		{ID: 2, Protocol: "hysteria2", Enabled: true, Clients: []db.Client{{ID: 20, StatsKey: "c_state", Email: "hy2@example.com", Enabled: true}}},
	}
	trafficByInbound, trafficByClient := summarizeTraffic(context.Background(), store, inbounds)
	xrayClient := trafficByClient[10]
	if xrayClient.Status != "ok" || xrayClient.Up != 10 || xrayClient.Down != 20 || xrayClient.Engine != "xray" {
		t.Fatalf("expected xray inbound to select xray state, got %+v", xrayClient)
	}
	singboxClient := trafficByClient[20]
	if singboxClient.Status != "ok" || singboxClient.Up != 30 || singboxClient.Down != 40 || singboxClient.Engine != "singbox" {
		t.Fatalf("expected sing-box inbound to select singbox state, got %+v", singboxClient)
	}
	if trafficByInbound[1].Engine != "xray" || trafficByInbound[2].Engine != "singbox" {
		t.Fatalf("expected inbound summaries to keep expected engines, got xray=%+v singbox=%+v", trafficByInbound[1], trafficByInbound[2])
	}
}

func TestSummarizeTrafficKeepsExpectedEngineEvenWhenUnavailable(t *testing.T) {
	store := &countingSummaryStore{
		states: []db.TrafficState{
			{Engine: "singbox", ScopeType: "client", ScopeKey: "c_state", TotalUp: 10, TotalDown: 20, Status: "unavailable", LastSeenAt: "2026-06-16T00:01:00Z"},
			{Engine: "xray", ScopeType: "client", ScopeKey: "c_state", TotalUp: 30, TotalDown: 40, Status: "ok", LastSeenAt: "2026-06-16T00:00:00Z"},
		},
	}
	inbounds := []db.Inbound{{ID: 1, Protocol: "hysteria2", Enabled: true, Clients: []db.Client{{ID: 10, StatsKey: "c_state", Email: "user@example.com", Enabled: true}}}}
	_, trafficByClient := summarizeTraffic(context.Background(), store, inbounds)
	client := trafficByClient[10]
	if client.Status != "unavailable" || client.Up != 10 || client.Down != 20 || client.Engine != "singbox" {
		t.Fatalf("expected expected singbox unavailable state, got %+v", client)
	}
}

func TestSummarizeTrafficFallsBackWhenExpectedEngineMissing(t *testing.T) {
	store := &countingSummaryStore{
		states: []db.TrafficState{
			{Engine: "xray", ScopeType: "client", ScopeKey: "c_state", TotalUp: 30, TotalDown: 40, Status: "ok", LastSeenAt: "2026-06-16T00:00:00Z"},
		},
	}
	inbounds := []db.Inbound{{ID: 1, Protocol: "hysteria2", Enabled: true, Clients: []db.Client{{ID: 10, StatsKey: "c_state", Email: "user@example.com", Enabled: true}}}}
	_, trafficByClient := summarizeTraffic(context.Background(), store, inbounds)
	client := trafficByClient[10]
	if client.Status != "ok" || client.Up != 30 || client.Down != 40 || client.Engine != "xray" {
		t.Fatalf("expected deterministic fallback when singbox state is missing, got %+v", client)
	}
}

func TestBuildTrafficCoverageNormalizesSingBoxEngineKey(t *testing.T) {
	coverage := buildTrafficCoverage(map[int64]inboundTrafficSummary{
		1: {Engine: "sing-box", Status: "ok"},
	})
	engines, ok := coverage["engines"].(map[string]string)
	if !ok {
		t.Fatalf("expected engines map, got %#v", coverage["engines"])
	}
	if engines["singbox"] != "ok" {
		t.Fatalf("expected normalized singbox status, got %+v", engines)
	}
	if _, exists := engines["sing-box"]; exists {
		t.Fatalf("did not expect dashed sing-box key: %+v", engines)
	}
}

func TestBuildTrafficCoverageAggregatesEngineStatusDeterministically(t *testing.T) {
	for i := 0; i < 20; i++ {
		coverage := buildTrafficCoverage(map[int64]inboundTrafficSummary{
			1: {Engine: "xray", Status: "ok"},
			2: {Engine: "xray", Status: "waiting"},
			3: {Engine: "singbox", Status: "unsupported"},
		})
		engines, ok := coverage["engines"].(map[string]string)
		if !ok {
			t.Fatalf("expected engines map, got %#v", coverage["engines"])
		}
		if coverage["overall"] != "partial" || engines["xray"] != "partial" || engines["singbox"] != "unsupported" {
			t.Fatalf("unexpected coverage: %+v", coverage)
		}
	}
}

func TestBuildTrafficCoverageCountsPartialStatus(t *testing.T) {
	coverage := buildTrafficCoverage(map[int64]inboundTrafficSummary{
		1: {Engine: "xray", Status: "partial"},
		2: {Engine: "singbox", Status: "partial"},
	})
	if coverage["overall"] != "partial" || coverage["partial"] != 2 {
		t.Fatalf("expected all-partial coverage, got %+v", coverage)
	}
}

func TestTrafficSamplesToSeriesDropsUnknownKeysAndSortsByTime(t *testing.T) {
	inbounds := []db.Inbound{{
		ID: 1, Protocol: "vless",
		Clients: []db.Client{{ID: 10, StatsKey: "c_xray"}},
	}}
	samples := []db.TrafficSample{
		{SampledAt: "2026-06-16T00:02:00Z", Engine: "xray", ScopeType: "client", ScopeKey: "c_xray", TotalUp: 20, TotalDown: 30},
		{SampledAt: "2026-06-16T00:01:00Z", Engine: "xray", ScopeType: "client", ScopeKey: "old_deleted", TotalUp: 999, TotalDown: 999},
		{SampledAt: "2026-06-16T00:01:00Z", Engine: "singbox", ScopeType: "client", ScopeKey: "c_xray", TotalUp: 888, TotalDown: 888},
		{SampledAt: "2026-06-16T00:01:00Z", Engine: "xray", ScopeType: "client", ScopeKey: "c_xray", TotalUp: 10, TotalDown: 15},
	}
	points := trafficSamplesToSeries(samples, "client", inbounds)
	if len(points) != 2 {
		t.Fatalf("expected two sorted known-key points, got %+v", points)
	}
	if points[0].Time != "2026-06-16T00:01:00Z" || points[0].Up != 10 || points[0].Down != 15 {
		t.Fatalf("unexpected first point: %+v", points[0])
	}
	if points[1].Time != "2026-06-16T00:02:00Z" || points[1].Up != 20 || points[1].Down != 30 {
		t.Fatalf("unexpected second point: %+v", points[1])
	}
}

func TestTrafficSamplesToSeriesFallsBackWhenExpectedEngineMissing(t *testing.T) {
	inbounds := []db.Inbound{{
		ID: 1, Protocol: "hysteria2",
		Clients: []db.Client{{ID: 10, StatsKey: "c_hy2"}},
	}}
	samples := []db.TrafficSample{
		{SampledAt: "2026-06-16T00:01:00Z", Engine: "xray", ScopeType: "client", ScopeKey: "c_hy2", TotalUp: 10, TotalDown: 15},
		{SampledAt: "2026-06-16T00:02:00Z", Engine: "xray", ScopeType: "client", ScopeKey: "c_hy2", TotalUp: 20, TotalDown: 30},
	}
	points := trafficSamplesToSeries(samples, "client", inbounds)
	if len(points) != 2 {
		t.Fatalf("expected fallback xray points, got %+v", points)
	}
	if points[0].Up != 10 || points[0].Down != 15 || points[1].Up != 20 || points[1].Down != 30 {
		t.Fatalf("unexpected fallback points: %+v", points)
	}
}

func TestCalculateCPUPercent(t *testing.T) {
	got := calculateCPUPercent(cpuSample{Idle: 40, Total: 100}, cpuSample{Idle: 50, Total: 140})
	if got != 75 {
		t.Fatalf("expected 75%% cpu, got %v", got)
	}
	for _, tc := range []struct {
		name     string
		previous cpuSample
		current  cpuSample
	}{
		{name: "no total delta", previous: cpuSample{Idle: 1, Total: 10}, current: cpuSample{Idle: 1, Total: 10}},
		{name: "counter reset", previous: cpuSample{Idle: 1, Total: 10}, current: cpuSample{Idle: 1, Total: 9}},
		{name: "idle exceeds total", previous: cpuSample{Idle: 1, Total: 10}, current: cpuSample{Idle: 12, Total: 15}},
	} {
		if got := calculateCPUPercent(tc.previous, tc.current); got != 0 {
			t.Fatalf("%s: expected 0, got %v", tc.name, got)
		}
	}
}

func TestCPUPercentSamplerUsesPreviousSampleWithoutSleeping(t *testing.T) {
	samples := []cpuSample{
		{Idle: 40, Total: 100},
		{Idle: 50, Total: 140},
	}
	idx := 0
	sampler := &cpuPercentSampler{read: func() (cpuSample, error) {
		if idx >= len(samples) {
			t.Fatal("unexpected extra cpu sample read")
		}
		sample := samples[idx]
		idx++
		return sample, nil
	}}
	if got := sampler.Sample(); got != 0 {
		t.Fatalf("first sample should seed baseline and return 0, got %v", got)
	}
	if got := sampler.Sample(); got != 75 {
		t.Fatalf("second sample should use previous sample, got %v", got)
	}

	failing := &cpuPercentSampler{read: func() (cpuSample, error) {
		return cpuSample{}, errors.New("read failed")
	}}
	if got := failing.Sample(); got != 0 {
		t.Fatalf("read failure should return 0, got %v", got)
	}
}
