package web

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

func TestAddXrayPostApplyDiagnosticsRetriesListeners(t *testing.T) {
	origAttempts := xrayPostApplyListenerAttempts
	origDelay := xrayPostApplyListenerDelay
	xrayPostApplyListenerAttempts = 3
	xrayPostApplyListenerDelay = time.Millisecond
	defer func() {
		xrayPostApplyListenerAttempts = origAttempts
		xrayPostApplyListenerDelay = origDelay
	}()

	calls := 0
	cfg := &routerConfig{
		xrayListeners: func(ctx context.Context, cfg *routerConfig) []CoreListenerDiagnostic {
			calls++
			return []CoreListenerDiagnostic{{
				InboundID: 1,
				Protocol:  "vless",
				Port:      2443,
				Transport: "tcp",
				Listening: calls >= 2,
			}}
		},
	}

	result := addXrayPostApplyDiagnostics(context.Background(), cfg, XrayApplyResult{Applied: true})
	if calls != 2 {
		t.Fatalf("expected listener diagnostics to retry until second success, got %d calls", calls)
	}
	if len(result.PostApplyWarnings) != 0 {
		t.Fatalf("expected no post-apply warnings after retry success, got %#v", result.PostApplyWarnings)
	}
}

func TestXrayInboundSemanticIssues(t *testing.T) {
	cases := []struct {
		name    string
		inbound db.Inbound
		want    []string
	}{
		{
			name:    "ws path invalid",
			inbound: db.Inbound{ID: 1, Protocol: "vless", Core: db.CoreXray, Port: 2443, Network: "ws", Security: "none", Enabled: true, WsPath: "ws"},
			want:    []string{"xray_ws_path_invalid"},
		},
		{
			name:    "h2 path invalid",
			inbound: db.Inbound{ID: 2, Protocol: "vless", Core: db.CoreXray, Port: 2444, Network: "h2", Security: "none", Enabled: true},
			want:    []string{"xray_ws_path_invalid"},
		},
		{
			name:    "grpc empty service name",
			inbound: db.Inbound{ID: 3, Protocol: "vless", Core: db.CoreXray, Port: 2445, Network: "grpc", Security: "none", Enabled: true},
			want:    []string{"xray_grpc_service_name_default"},
		},
		{
			name:    "grpc invalid service name",
			inbound: db.Inbound{ID: 4, Protocol: "vless", Core: db.CoreXray, Port: 2446, Network: "grpc", Security: "none", Enabled: true, GrpcServiceName: "bad name"},
			want:    []string{"xray_grpc_service_name_invalid"},
		},
		{
			name:    "xhttp path invalid",
			inbound: db.Inbound{ID: 5, Protocol: "vless", Core: db.CoreXray, Port: 2447, Network: "xhttp", Security: "none", Enabled: true, XHTTPPath: "x"},
			want:    []string{"xray_xhttp_path_invalid"},
		},
		{
			name:    "reality incomplete",
			inbound: db.Inbound{ID: 6, Protocol: "vless", Core: db.CoreXray, Port: 2448, Network: "tcp", Security: "reality", Enabled: true, RealityDest: "example.com:443"},
			want:    []string{"xray_reality_settings_incomplete"},
		},
		{
			name:    "tls certificate missing",
			inbound: db.Inbound{ID: 7, Protocol: "vless", Core: db.CoreXray, Port: 2449, Network: "tcp", Security: "tls", Enabled: true, TLSCertFile: "/cert.pem"},
			want:    []string{"xray_tls_certificate_missing"},
		},
		{
			name:    "ss2022 credentials missing",
			inbound: db.Inbound{ID: 8, Protocol: "shadowsocks", Core: db.CoreXray, Port: 2450, Network: "tcp", Security: "none", Enabled: true, SSMethod: "2022-blake3-aes-128-gcm"},
			want:    []string{"xray_shadowsocks_credentials_missing"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := xrayInboundSemanticIssues(tc.inbound)
			got := make([]string, 0, len(issues))
			for _, issue := range issues {
				got = append(got, issue.Code)
			}
			for _, want := range tc.want {
				if !containsXrayTestString(got, want) {
					t.Fatalf("expected issue %q, got %#v", want, got)
				}
			}
		})
	}
}

func TestXrayLogAttributionSuggestions(t *testing.T) {
	missing := []CoreListenerDiagnostic{{Port: 2443, Transport: "tcp"}}
	suggestions := xrayLogAttributionSuggestions(
		"failed to load certificate: open /etc/xray/fullchain.pem: no such file",
		[]string{
			"failed to listen tcp 0.0.0.0:2443: bind: address already in use",
			"REALITY shortId privateKey error",
			"grpc service error",
			"xhttp path error",
			"permission denied",
		},
		missing,
	)
	for _, want := range []string{"端口可能被占用", "TLS 证书", "REALITY", "gRPC", "XHTTP", "权限不足"} {
		if !suggestionsContain(suggestions, want) {
			t.Fatalf("expected suggestion containing %q, got %#v", want, suggestions)
		}
	}
}

func TestRoutingScopeDoesNotFilterUnavailableOutboundTarget(t *testing.T) {
	inbound := db.Inbound{ID: 1, Remark: "xray", Protocol: "vless", Core: db.CoreXray, Enabled: true}
	rule := db.RoutingRule{ID: 9, InboundTag: db.GeneratedInboundTag(inbound), OutboundTag: "missing", Enabled: true}
	includeXray, includeSingbox := xrayAndSingboxForRoutingRuleWriteWithScope(coreInboundScope{inbounds: []db.Inbound{inbound}, hasXray: true}, db.RoutingRule{}, false, rule)
	if !includeXray || includeSingbox {
		t.Fatalf("bad xray routing target should still apply only xray, got xray=%v singbox=%v", includeXray, includeSingbox)
	}
}

func TestOutboundScopeIncludesCoreRulesReferencingBadTarget(t *testing.T) {
	inbound := db.Inbound{ID: 1, Remark: "xray", Protocol: "vless", Core: db.CoreXray, Enabled: true}
	outbound := db.Outbound{ID: 7, Tag: "hy2-only", Protocol: "hysteria2", Enabled: true}
	scope := coreInboundScope{
		inbounds: []db.Inbound{inbound},
		rules:    []db.RoutingRule{{ID: 9, InboundTag: db.GeneratedInboundTag(inbound), OutboundID: outbound.ID, OutboundTag: outbound.Tag, Enabled: true}},
		hasXray:  true,
	}
	includeXray, includeSingbox := xrayAndSingboxForOutboundDelete(scope, outbound)
	if !includeXray || includeSingbox {
		t.Fatalf("outbound referenced by xray rule should apply only xray, got xray=%v singbox=%v", includeXray, includeSingbox)
	}
}

func containsXrayTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func suggestionsContain(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
