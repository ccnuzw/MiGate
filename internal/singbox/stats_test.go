package singbox

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/imzyb/MiGate/internal/trafficstats"
)

func TestGRPCStatsClientQueriesAndMarksSingboxEngine(t *testing.T) {
	addr, closeServer := startFakeSingboxStatsService(t, []trafficstats.Stat{
		{ScopeType: "client", ScopeKey: "c_1", Uplink: 10, Downlink: 20},
		{ScopeType: "inbound", ScopeKey: "hy2-inbound-1", Uplink: 30, Downlink: 40},
	})
	defer closeServer()

	client, err := NewGRPCStatsClient(context.Background(), addr)
	if err != nil {
		t.Fatalf("new sing-box grpc stats client: %v", err)
	}
	defer client.Close()

	stats, err := client.QueryTrafficStats(context.Background())
	if err != nil {
		t.Fatalf("query stats: %v", err)
	}
	byScope := map[string]trafficstats.Stat{}
	for _, stat := range stats {
		byScope[stat.ScopeType+"/"+stat.ScopeKey] = stat
	}
	if got := byScope["client/c_1"]; got.Engine != "singbox" || got.Uplink != 10 || got.Downlink != 20 {
		t.Fatalf("unexpected client stats: %+v", got)
	}
	if got := byScope["inbound/hy2-inbound-1"]; got.Engine != "singbox" || got.Uplink != 30 || got.Downlink != 40 {
		t.Fatalf("unexpected inbound stats: %+v", got)
	}
}

func TestGRPCStatsClientReturnsErrorWhenStatsServiceUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen temporary port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	client, err := NewGRPCStatsClient(context.Background(), addr)
	if err != nil {
		t.Fatalf("new sing-box grpc stats client: %v", err)
	}
	defer client.Close()

	_, err = client.QueryTrafficStats(context.Background())
	if err == nil || !strings.Contains(err.Error(), "stats grpc query") {
		t.Fatalf("expected unavailable stats service error, got %v", err)
	}
}

func TestUnavailableStatsClientReturnsConstructionError(t *testing.T) {
	want := errors.New("invalid stats service")
	client := NewUnavailableStatsClient(want)
	_, err := client.QueryTrafficStats(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("expected construction error, got %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close unavailable client: %v", err)
	}
}

func startFakeSingboxStatsService(t *testing.T, stats []trafficstats.Stat) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake stats service: %v", err)
	}
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Protocols: protocols,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/experimental.v2rayapi.StatsService/QueryStats" {
				t.Errorf("unexpected stats service path: %s", r.URL.Path)
				http.NotFound(w, r)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !strings.Contains(string(body), ">>>traffic>>>") {
				t.Errorf("request body missing traffic pattern: %x", body)
				http.Error(w, "missing pattern", http.StatusBadRequest)
				return
			}
			w.Header().Set("Trailer", "Grpc-Status")
			w.Header().Set("Content-Type", "application/grpc")
			_, _ = w.Write(encodeGRPCFrameForTest(encodeQueryStatsResponseForTest(stats)))
			w.Header().Set("Grpc-Status", "0")
		}),
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Errorf("fake stats service: %v", err)
		}
	}()
	return listener.Addr().String(), func() {
		_ = server.Close()
	}
}

func encodeGRPCFrameForTest(message []byte) []byte {
	frame := make([]byte, 5+len(message))
	size := len(message)
	frame[1] = byte(size >> 24)
	frame[2] = byte(size >> 16)
	frame[3] = byte(size >> 8)
	frame[4] = byte(size)
	copy(frame[5:], message)
	return frame
}

func encodeQueryStatsResponseForTest(stats []trafficstats.Stat) []byte {
	var out []byte
	for _, stat := range stats {
		if stat.Uplink != 0 {
			out = append(out, encodeStatForTest(stat.ScopeType, stat.ScopeKey, "uplink", stat.Uplink)...)
		}
		if stat.Downlink != 0 {
			out = append(out, encodeStatForTest(stat.ScopeType, stat.ScopeKey, "downlink", stat.Downlink)...)
		}
	}
	return out
}

func encodeStatForTest(scopeType, scopeKey, direction string, value int64) []byte {
	scope := scopeType
	if scope == "client" {
		scope = "user"
	}
	name := strings.Join([]string{scope, scopeKey, "traffic", direction}, ">>>")
	var stat []byte
	stat = append(stat, 0x0a)
	stat = appendVarintForTest(stat, uint64(len(name)))
	stat = append(stat, name...)
	stat = append(stat, 0x10)
	stat = appendVarintForTest(stat, uint64(value))
	var out []byte
	out = append(out, 0x0a)
	out = appendVarintForTest(out, uint64(len(stat)))
	out = append(out, stat...)
	return out
}

func appendVarintForTest(out []byte, value uint64) []byte {
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}
