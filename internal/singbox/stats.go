package singbox

import (
	"context"
	"strings"

	"github.com/imzyb/MiGate/internal/xray"
)

type StatsClient interface {
	QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error)
	Close() error
}

type StubStatsClient struct{}

type UnavailableStatsClient struct {
	Err error
}

type DisabledStatsClient struct {
	Status  string
	Message string
}

type GRPCStatsClient struct {
	client *xray.GRPCStatsClient
}

func NewStubStatsClient() *StubStatsClient {
	return &StubStatsClient{}
}

func NewUnavailableStatsClient(err error) *UnavailableStatsClient {
	return &UnavailableStatsClient{Err: err}
}

func NewDisabledStatsClient(status, message string) *DisabledStatsClient {
	return &DisabledStatsClient{Status: strings.TrimSpace(status), Message: strings.TrimSpace(message)}
}

func NewGRPCStatsClient(ctx context.Context, server string) (*GRPCStatsClient, error) {
	if strings.TrimSpace(server) == "" {
		server = "127.0.0.1:10086"
	}
	client, err := xray.NewGRPCStatsClientWithEngineAndService(ctx, server, "singbox", "experimental.v2rayapi.StatsService")
	if err != nil {
		return nil, err
	}
	return &GRPCStatsClient{client: client}, nil
}

func (c *StubStatsClient) QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error) {
	return []xray.TrafficStat{}, nil
}

func (c *StubStatsClient) Close() error { return nil }

func (c *UnavailableStatsClient) QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	return nil, context.Canceled
}

func (c *UnavailableStatsClient) Close() error { return nil }

func (c *DisabledStatsClient) QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error) {
	return []xray.TrafficStat{}, nil
}

func (c *DisabledStatsClient) Close() error { return nil }

func (c *GRPCStatsClient) QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error) {
	stats, err := c.client.QueryTrafficStats(ctx)
	if err != nil {
		return nil, err
	}
	for i := range stats {
		stats[i].Engine = "singbox"
	}
	return stats, nil
}

func (c *GRPCStatsClient) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}
