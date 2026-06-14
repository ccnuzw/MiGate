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

func (s *countingSummaryStore) AddToBlacklist(ctx context.Context, tokenHash string, expiresAt time.Time, revoked bool) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) IsBlacklisted(ctx context.Context, tokenHash string) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *countingSummaryStore) RecordSessionTouch(ctx context.Context, tokenHash string) error {
	return errors.New("not implemented")
}

func (s *countingSummaryStore) ListActiveSessions(ctx context.Context) ([]db.BlacklistedSession, error) {
	return nil, errors.New("not implemented")
}

func (s *countingSummaryStore) RevokeSession(ctx context.Context, id int64) error {
	return errors.New("not implemented")
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
	if store.listInboundsCalls != 3 {
		t.Fatalf("expected first build only before expiry; validation calls included, got %d ListInbounds calls", store.listInboundsCalls)
	}
	if first["generated_at"] != second["generated_at"] {
		t.Fatalf("expected cached generated_at to be reused: first=%v second=%v", first["generated_at"], second["generated_at"])
	}

	now = now.Add(6 * time.Second)
	if _, err := cache.get(context.Background(), store, nil); err != nil {
		t.Fatalf("expired summary: %v", err)
	}
	if store.listInboundsCalls != 6 {
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
