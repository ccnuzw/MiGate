package singbox

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeCommandRunner struct {
	name string
	args []string
	out  []byte
	err  error
}

func (r *fakeCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = append([]string{}, args...)
	return r.out, r.err
}

func TestCommandStatsClientQueriesAndParsesTrafficStats(t *testing.T) {
	runner := &fakeCommandRunner{out: []byte(`{"stat":[{"name":"user>>>c_1>>>traffic>>>uplink","value":10},{"name":"user>>>c_1>>>traffic>>>downlink","value":20}]}`)}
	client := &CommandStatsClient{BinaryPath: "/usr/bin/sing-box", Server: "127.0.0.1:10086", Runner: runner}
	stats, err := client.QueryTrafficStats(context.Background())
	if err != nil {
		t.Fatalf("query stats: %v", err)
	}
	if runner.name != "/usr/bin/sing-box" {
		t.Fatalf("unexpected command name: %q", runner.name)
	}
	wantArgs := []string{"api", "statsquery", "--server", "127.0.0.1:10086", "-pattern", ">>>traffic>>>"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("unexpected args: got %+v want %+v", runner.args, wantArgs)
	}
	if len(stats) != 1 || stats[0].Engine != "singbox" || stats[0].ScopeType != "client" || stats[0].ScopeKey != "c_1" || stats[0].Uplink != 10 || stats[0].Downlink != 20 {
		t.Fatalf("unexpected parsed stats: %+v", stats)
	}
}

func TestCommandStatsClientReturnsClearErrorOnCommandFailure(t *testing.T) {
	runner := &fakeCommandRunner{err: errors.New("not supported")}
	client := &CommandStatsClient{BinaryPath: "/usr/bin/sing-box", Server: "127.0.0.1:10086", Runner: runner}
	_, err := client.QueryTrafficStats(context.Background())
	if err == nil || !strings.Contains(err.Error(), "sing-box statsquery") || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected wrapped statsquery error, got %v", err)
	}
}
