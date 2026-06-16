package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/xray"
)

type mockStore struct {
	traffic map[string]*xray.ClientStats
	raw     []db.TrafficRawStat
	unavail []db.TrafficRawStat
}

func (m *mockStore) UpdateClientTraffic(ctx context.Context, email string, uplink, downlink int64) error {
	if m.traffic == nil {
		m.traffic = make(map[string]*xray.ClientStats)
	}
	m.traffic[email] = &xray.ClientStats{Email: email, Uplink: uplink, Downlink: downlink}
	return nil
}

func (m *mockStore) ApplyTrafficRawStats(ctx context.Context, stats []db.TrafficRawStat, observedAt time.Time) error {
	m.raw = append(m.raw, stats...)
	if m.traffic == nil {
		m.traffic = make(map[string]*xray.ClientStats)
	}
	for _, stat := range stats {
		if stat.ScopeType == "client" {
			m.traffic[stat.ScopeKey] = &xray.ClientStats{Email: stat.ScopeKey, Uplink: stat.RawUp, Downlink: stat.RawDown}
		}
	}
	return nil
}

func (m *mockStore) MarkTrafficUnavailable(ctx context.Context, engine, status, message string, observedAt time.Time) error {
	m.unavail = append(m.unavail, db.TrafficRawStat{Engine: engine, Status: status, Message: message})
	return nil
}

type mockStatsClient struct {
	stats map[string]*xray.ClientStats
	raw   []xray.TrafficStat
	err   error
}

func (m *mockStatsClient) QueryAllStats(ctx context.Context) (map[string]*xray.ClientStats, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.stats, nil
}

func (m *mockStatsClient) QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.raw != nil {
		return m.raw, nil
	}
	result := make([]xray.TrafficStat, 0, len(m.stats))
	for _, stat := range m.stats {
		result = append(result, xray.TrafficStat{Engine: "xray", ScopeType: "client", ScopeKey: stat.Email, Uplink: stat.Uplink, Downlink: stat.Downlink})
	}
	return result, nil
}

func (m *mockStatsClient) Close() error {
	return nil
}

func TestTrafficSyncSchedulerSync(t *testing.T) {
	store := &mockStore{}
	client := &mockStatsClient{
		stats: map[string]*xray.ClientStats{
			"client1@test.com": {Email: "client1@test.com", Uplink: 1024, Downlink: 2048},
			"client2@test.com": {Email: "client2@test.com", Uplink: 512, Downlink: 1024},
		},
	}

	scheduler := NewTrafficSyncScheduler(store, client, 1*time.Minute)
	scheduler.sync()

	if len(store.traffic) != 2 {
		t.Errorf("Expected 2 clients updated, got %d", len(store.traffic))
	}

	c1 := store.traffic["client1@test.com"]
	if c1.Uplink != 1024 || c1.Downlink != 2048 {
		t.Errorf("client1 traffic mismatch: up=%d down=%d", c1.Uplink, c1.Downlink)
	}

	c2 := store.traffic["client2@test.com"]
	if c2.Uplink != 512 || c2.Downlink != 1024 {
		t.Errorf("client2 traffic mismatch: up=%d down=%d", c2.Uplink, c2.Downlink)
	}
}

func TestTrafficSyncSchedulerWithEmptyStats(t *testing.T) {
	store := &mockStore{}
	client := &mockStatsClient{stats: make(map[string]*xray.ClientStats)}

	scheduler := NewTrafficSyncScheduler(store, client, 1*time.Minute)
	scheduler.sync()

	if len(store.traffic) != 0 {
		t.Errorf("Expected 0 clients with empty stats, got %d", len(store.traffic))
	}
}

func TestTrafficSyncSchedulerKeepsXrayWhenSingboxUnavailable(t *testing.T) {
	store := &mockStore{}
	xrayClient := &mockStatsClient{raw: []xray.TrafficStat{
		{Engine: "xray", ScopeType: "client", ScopeKey: "client1@test.com", Uplink: 1024, Downlink: 2048},
	}}
	singboxClient := &mockStatsClient{err: errors.New("connect 127.0.0.1:10086: connection refused")}

	scheduler := NewTrafficSyncSchedulerWithSingbox(store, xrayClient, singboxClient, time.Minute)
	scheduler.sync()

	if len(store.raw) != 1 {
		t.Fatalf("expected xray raw stat to be applied despite sing-box failure, got %+v", store.raw)
	}
	if store.raw[0].Engine != "xray" || store.raw[0].ScopeKey != "client1@test.com" {
		t.Fatalf("unexpected raw stat: %+v", store.raw[0])
	}
	if len(store.unavail) != 1 || store.unavail[0].Engine != "singbox" || store.unavail[0].Status != "unavailable" {
		t.Fatalf("expected singbox unavailable marker, got %+v", store.unavail)
	}
}

func TestTrafficSyncSchedulerWritesSingboxEngineStats(t *testing.T) {
	store := &mockStore{}
	xrayClient := &mockStatsClient{raw: []xray.TrafficStat{}}
	singboxClient := &mockStatsClient{raw: []xray.TrafficStat{
		{Engine: "singbox", ScopeType: "client", ScopeKey: "c_singbox", Uplink: 10, Downlink: 20},
		{Engine: "singbox", ScopeType: "inbound", ScopeKey: "hy2-inbound-1", Uplink: 30, Downlink: 40},
	}}

	scheduler := NewTrafficSyncSchedulerWithSingbox(store, xrayClient, singboxClient, time.Minute)
	scheduler.sync()

	if len(store.raw) != 2 {
		t.Fatalf("expected singbox stats to be applied, got %+v", store.raw)
	}
	for _, stat := range store.raw {
		if stat.Engine != "singbox" || stat.Status != "ok" {
			t.Fatalf("unexpected singbox stat: %+v", stat)
		}
	}
}
