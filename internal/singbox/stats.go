package singbox

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/imzyb/MiGate/internal/xray"
)

type StatsClient interface {
	QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error)
	Close() error
}

type StubStatsClient struct{}

type CommandStatsClient struct {
	BinaryPath string
	Server     string
	Runner     CommandRunner
}

type CommandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func NewStubStatsClient() *StubStatsClient {
	return &StubStatsClient{}
}

func NewCommandStatsClient(binaryPath, server string) *CommandStatsClient {
	if strings.TrimSpace(binaryPath) == "" {
		binaryPath = DefaultBinaryPath
	}
	if strings.TrimSpace(server) == "" {
		server = "127.0.0.1:10086"
	}
	return &CommandStatsClient{BinaryPath: binaryPath, Server: server}
}

func (c *StubStatsClient) QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error) {
	return []xray.TrafficStat{}, nil
}

func (c *StubStatsClient) Close() error { return nil }

func (c *CommandStatsClient) QueryTrafficStats(ctx context.Context) ([]xray.TrafficStat, error) {
	runner := c.Runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	// This shells out to sing-box and is covered by command-runner tests; the
	// deployed stats API endpoint still needs validation in the target runtime.
	out, err := runner.Output(ctx, c.BinaryPath, "api", "statsquery", "--server", c.Server, "-pattern", ">>>traffic>>>")
	if err != nil {
		return nil, fmt.Errorf("sing-box statsquery: %w", err)
	}
	return xray.ParseTrafficStatsQueryOutput("singbox", out)
}

func (c *CommandStatsClient) Close() error { return nil }

func (execCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
