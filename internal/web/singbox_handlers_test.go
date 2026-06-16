package web

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/singbox"
)

func TestValidateSingboxUnsupportedStatsWarnsWithoutFailing(t *testing.T) {
	store := &countingSummaryStore{
		inbounds: []db.Inbound{{
			ID: 1, Protocol: "hysteria2", Enabled: true,
			Clients: []db.Client{{ID: 2, StatsKey: "c_hy2", Email: "hy2@example.com", Enabled: true}},
		}},
	}
	cfg := &routerConfig{
		store: store,
		singboxRuntime: fixedSingboxRuntime{capability: singbox.Capability{
			Checked: true, Unsupported: true, Message: singbox.StatsUnsupportedMessage,
		}},
	}

	result := validateSingboxConfig(context.Background(), cfg)
	if !result.Valid {
		t.Fatalf("unsupported stats should not fail validation: %+v", result)
	}
	if !containsString(result.Warnings, "singbox_stats_unsupported") {
		t.Fatalf("expected unsupported warning, got %+v", result.Warnings)
	}
}

func TestBuildSingboxConfigUnsupportedStatsOmitsV2RayAPI(t *testing.T) {
	inbounds := []db.Inbound{{
		ID: 1, Protocol: "hysteria2", Port: 21001, Enabled: true,
		Clients: []db.Client{{ID: 2, UUID: "pass", StatsKey: "c_hy2", Email: "hy2@example.com", Enabled: true}},
	}}
	cfg := &routerConfig{
		singboxRuntime: fixedSingboxRuntime{capability: singbox.Capability{
			Checked: true, Unsupported: true, Message: singbox.StatsUnsupportedMessage,
		}},
	}

	built := buildSingboxConfigForRuntime(context.Background(), cfg, inbounds)
	raw, err := json.Marshal(built.config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if strings.Contains(string(raw), "v2ray_api") {
		t.Fatalf("unsupported stats config must not include v2ray_api: %s", raw)
	}
	if !containsString(built.warnings, "singbox_stats_unsupported") {
		t.Fatalf("expected unsupported warning, got %+v", built.warnings)
	}
}

func TestBuildSingboxConfigCapabilityFailureDoesNotWarnUnsupported(t *testing.T) {
	inbounds := []db.Inbound{{
		ID: 1, Protocol: "hysteria2", Port: 21001, Enabled: true,
		Clients: []db.Client{{ID: 2, UUID: "pass", StatsKey: "c_hy2", Email: "hy2@example.com", Enabled: true}},
	}}
	cfg := &routerConfig{
		singboxRuntime: fixedSingboxRuntime{capability: singbox.Capability{
			Checked: true, Message: "sing-box check permission denied",
		}},
	}

	built := buildSingboxConfigForRuntime(context.Background(), cfg, inbounds)
	if containsString(built.warnings, "singbox_stats_unsupported") {
		t.Fatalf("capability check failure must not be reported as unsupported: %+v", built.warnings)
	}
	if !containsString(built.warnings, "singbox_stats_capability_check_failed") {
		t.Fatalf("expected capability failure warning, got %+v", built.warnings)
	}
}

func TestBuildSingboxConfigSkipsCapabilityWhenNoSingboxInbound(t *testing.T) {
	var calls int32
	cfg := &routerConfig{
		singboxRuntime: countingSingboxRuntime{calls: &calls},
	}

	built := buildSingboxConfigForRuntime(context.Background(), cfg, []db.Inbound{{ID: 1, Protocol: "vless", Enabled: true}})
	if len(built.warnings) != 0 {
		t.Fatalf("expected no warnings without singbox inbound, got %+v", built.warnings)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected capability detection to be skipped, got %d calls", got)
	}
}

type countingSingboxRuntime struct {
	calls *int32
}

func (r countingSingboxRuntime) Capability(ctx context.Context) singbox.Capability {
	atomic.AddInt32(r.calls, 1)
	return singbox.Capability{V2RayAPIStats: true, Checked: true}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
