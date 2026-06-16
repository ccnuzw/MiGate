package xray

import (
	"context"
	"fmt"
	"testing"
)

type flakyStatsClient struct {
	calls int
}

func (c *flakyStatsClient) QueryAllStats(ctx context.Context) (map[string]*ClientStats, error) {
	c.calls++
	if c.calls == 1 {
		return nil, fmt.Errorf("not ready")
	}
	return map[string]*ClientStats{
		"sam@example.com": {Email: "sam@example.com", Uplink: 100, Downlink: 200},
	}, nil
}

func (c *flakyStatsClient) QueryTrafficStats(ctx context.Context) ([]TrafficStat, error) {
	stats, err := c.QueryAllStats(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]TrafficStat, 0, len(stats))
	for _, stat := range stats {
		result = append(result, TrafficStat{Engine: "xray", ScopeType: "client", ScopeKey: stat.Email, Uplink: stat.Uplink, Downlink: stat.Downlink})
	}
	return result, nil
}

func (c *flakyStatsClient) Close() error { return nil }

func TestStubStatsClientReturnsEmptyStats(t *testing.T) {
	client := NewStubStatsClient()
	defer client.Close()

	stats, err := client.QueryAllStats(context.Background())
	if err != nil {
		t.Fatalf("QueryAllStats returned error: %v", err)
	}

	if len(stats) != 0 {
		t.Errorf("Expected empty stats map, got %d entries", len(stats))
	}
}

func TestStubStatsClientCloseIsNoOp(t *testing.T) {
	client := NewStubStatsClient()
	err := client.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
	// Second close should also be safe
	err = client.Close()
	if err != nil {
		t.Errorf("Second Close returned error: %v", err)
	}
}

func TestResilientStatsClientRetriesAfterInitialFailure(t *testing.T) {
	primary := &flakyStatsClient{}
	client := NewResilientStatsClient(primary, NewStubStatsClient())
	defer client.Close()

	first, err := client.QueryAllStats(context.Background())
	if err != nil {
		t.Fatalf("first query should use fallback without error: %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("first query should return fallback empty stats, got %#v", first)
	}

	second, err := client.QueryAllStats(context.Background())
	if err != nil {
		t.Fatalf("second query should recover primary stats: %v", err)
	}
	got := second["sam@example.com"]
	if got == nil || got.Uplink != 100 || got.Downlink != 200 {
		t.Fatalf("second query did not recover live stats: %#v", second)
	}
	if primary.calls != 2 {
		t.Fatalf("primary should be retried, got %d calls", primary.calls)
	}
}

func TestClientStatsStruct(t *testing.T) {
	stats := &ClientStats{
		Email:    "test@example.com",
		Uplink:   1024,
		Downlink: 2048,
	}

	if stats.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", stats.Email, "test@example.com")
	}
	if stats.Uplink != 1024 {
		t.Errorf("Uplink = %d, want %d", stats.Uplink, 1024)
	}
	if stats.Downlink != 2048 {
		t.Errorf("Downlink = %d, want %d", stats.Downlink, 2048)
	}
}

func TestGRPCStatsClientRequiresBuildTag(t *testing.T) {
	// Without -tags grpc, NewGRPCStatsClient should return an error
	client, err := NewGRPCStatsClient(context.Background(), "tcp:127.0.0.1:1080")
	if err == nil {
		t.Errorf("Expected error when grpc not enabled, got nil")
	}
	if client != nil {
		t.Errorf("Expected nil client when grpc not enabled, got %v", client)
	}
}

func TestParseCommandStatsQueryOutput(t *testing.T) {
	raw := []byte(`{"stat":[{"name":"user>>>sam@example.com>>>traffic>>>uplink","value":60300000},{"name":"user>>>sam@example.com>>>traffic>>>downlink","value":202400000}]}`)
	stats, err := ParseStatsQueryOutput(raw)
	if err != nil {
		t.Fatalf("parse stats: %v", err)
	}
	got := stats["sam@example.com"]
	if got == nil || got.Uplink != 60300000 || got.Downlink != 202400000 {
		t.Fatalf("unexpected stats: %#v", got)
	}
}

func TestParseTrafficStatsQueryOutputAllScopes(t *testing.T) {
	raw := []byte(`{"stat":[
		{"name":"user>>>c_stats>>>traffic>>>uplink","value":10},
		{"name":"user>>>c_stats>>>traffic>>>downlink","value":20},
		{"name":"inbound>>>inbound-1-vless>>>traffic>>>uplink","value":30},
		{"name":"inbound>>>inbound-1-vless>>>traffic>>>downlink","value":40},
		{"name":"outbound>>>direct>>>traffic>>>uplink","value":50},
		{"name":"outbound>>>direct>>>traffic>>>downlink","value":60}
	]}`)
	stats, err := ParseTrafficStatsQueryOutput("xray", raw)
	if err != nil {
		t.Fatalf("parse traffic stats: %v", err)
	}
	byScope := map[string]TrafficStat{}
	for _, stat := range stats {
		byScope[stat.ScopeType+"/"+stat.ScopeKey] = stat
	}
	if got := byScope["client/c_stats"]; got.Uplink != 10 || got.Downlink != 20 {
		t.Fatalf("unexpected client stats: %+v", got)
	}
	if got := byScope["inbound/inbound-1-vless"]; got.Uplink != 30 || got.Downlink != 40 {
		t.Fatalf("unexpected inbound stats: %+v", got)
	}
	if got := byScope["outbound/direct"]; got.Uplink != 50 || got.Downlink != 60 {
		t.Fatalf("unexpected outbound stats: %+v", got)
	}
}
