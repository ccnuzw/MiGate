package xray_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/xray"
)

func TestBuildConfigIncludesSupportedProtocolInboundsAndFreedomOutbound(t *testing.T) {
	inbounds := []db.Inbound{
		{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Remark: "vless-reality", Protocol: "vless", Port: 443, Network: "tcp", Security: "reality", Enabled: true, Clients: []db.Client{{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Email: "a@example.com", Enabled: true}}},
		{ID: 2, UUID: "22222222-2222-4222-8222-222222222222", Remark: "vmess-ws", Protocol: "vmess", Port: 8443, Network: "ws", Security: "tls", Enabled: true, Clients: []db.Client{{UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Email: "b@example.com", Enabled: true}}},
		{ID: 3, UUID: "33333333-3333-4333-8333-333333333333", Remark: "trojan", Protocol: "trojan", Port: 9443, Network: "tcp", Security: "tls", Enabled: true, Clients: []db.Client{{UUID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Email: "c@example.com", Enabled: true}}},
		{ID: 4, UUID: "44444444-4444-4444-8444-444444444444", Remark: "ss", Protocol: "shadowsocks", Port: 1080, Network: "tcp", Security: "none", Enabled: true, Clients: []db.Client{{UUID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", Email: "d@example.com", Enabled: true}}},
		{ID: 5, UUID: "55555555-5555-4555-8555-555555555555", Remark: "disabled", Protocol: "vless", Port: 1443, Network: "tcp", Security: "none", Enabled: false, Clients: []db.Client{{UUID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", Email: "disabled@example.com", Enabled: true}}},
		{ID: 6, UUID: "66666666-6666-4666-8666-666666666666", Remark: "trojan-reality", Protocol: "trojan", Port: 30030, Network: "tcp", Security: "reality", RealityDest: "www.microsoft.com:443", RealityServerNames: "www.microsoft.com", RealityShortID: "", RealityPrivateKey: "uNisYErm5wwrV9t9EP2P3VB0g3CpS5m70bdG7gwShXg", Enabled: true, Clients: []db.Client{{UUID: "ffffffff-ffff-4fff-8fff-ffffffffffff", Email: "trojan-reality@test.com", Enabled: true}}},
	}

	config, err := xray.BuildConfig(inbounds)
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	if len(config.Inbounds) != 5 {
		t.Fatalf("expected five enabled inbounds, got %+v", config.Inbounds)
	}
	if len(config.Outbounds) != 1 || config.Outbounds[0].Protocol != "freedom" {
		t.Fatalf("expected direct freedom outbound, got %+v", config.Outbounds)
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	text := string(encoded)
	for _, want := range []string{"vless", "vmess", "trojan", "shadowsocks", "a@example.com", "b@example.com", "c@example.com", "d@example.com", "trojan-reality@test.com", "trojan-reality"} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "disabled@example.com") {
		t.Fatalf("disabled inbound leaked into xray config: %s", text)
	}
	// Verify Trojan+REALITY has realitySettings with privateKey and shortIds
	if !strings.Contains(text, "uNisYErm5wwrV9t9EP2P3VB0g3CpS5m70bdG7gwShXg") {
		t.Fatalf("Trojan+REALITY config missing privateKey: %s", text)
	}
	if !strings.Contains(text, "realitySettings") {
		t.Fatalf("Trojan+REALITY config missing realitySettings: %s", text)
	}
	if !strings.Contains(text, "shortIds") {
		t.Fatalf("Trojan+REALITY config missing shortIds: %s", text)
	}
	if !strings.Contains(text, "password") {
		t.Fatalf("Trojan+REALITY config missing password field: %s", text)
	}
}

func TestBuildConfigRejectsUnsupportedProtocol(t *testing.T) {
	_, err := xray.BuildConfig([]db.Inbound{{Protocol: "openvpn", Port: 1194, Enabled: true}})
	if err == nil {
		t.Fatal("expected unsupported protocol error")
	}
}

func TestBuildConfigIncludesXHTTPSettingsForVLESSReality(t *testing.T) {
	config, err := xray.BuildConfig([]db.Inbound{{
		ID:                 7,
		UUID:               "77777777-7777-4777-8777-777777777777",
		Remark:             "vless-xhttp-reality",
		Protocol:           "vless",
		Port:               30040,
		Network:            "xhttp",
		Security:           "reality",
		RealityDest:        "www.cloudflare.com:443",
		RealityServerNames: "www.cloudflare.com",
		RealityPrivateKey:  "uNisYErm5wwrV9t9EP2P3VB0g3CpS5m70bdG7gwShXg",
		XHTTPPath:          "/migate-xhttp",
		XHTTPMode:          "stream-one",
		Enabled:            true,
		Clients:            []db.Client{{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Email: "xhttp@test.com", Enabled: true}},
	}})
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	text := string(encoded)
	for _, want := range []string{`"network":"xhttp"`, `"xhttpSettings"`, `"path":"/migate-xhttp"`, `"mode":"stream-one"`, `"realitySettings"`, `"shortIds"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("XHTTP config missing %q: %s", want, text)
		}
	}
}

func TestBuildConfigVLESSRealityHasFlowInClients(t *testing.T) {
	inbounds := []db.Inbound{
		{
			ID: 9, UUID: "99999999-9999-4999-8999-999999999999",
			Remark: "vless-tcp-reality-flow", Protocol: "vless", Port: 30110,
			Network: "tcp", Security: "reality",
			RealityDest:        "www.cloudflare.com:443",
			RealityServerNames: "www.cloudflare.com",
			RealityPrivateKey:  "uNisYErm5wwrV9t9EP2P3VB0g3CpS5m70bdG7gwShXg",
			Enabled: true,
			Clients: []db.Client{{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Email: "flow-test@test.com", Enabled: true}},
		},
		{
			ID: 10, UUID: "10101010-1010-4010-8010-101010101010",
			Remark: "vless-xhttp-reality-flow", Protocol: "vless", Port: 30120,
			Network: "xhttp", Security: "reality",
			XHTTPPath:          "/migate",
			XHTTPMode:          "stream-one",
			RealityDest:        "www.cloudflare.com:443",
			RealityServerNames: "www.cloudflare.com",
			RealityPrivateKey:  "uNisYErm5wwrV9t9EP2P3VB0g3CpS5m70bdG7gwShXg",
			Enabled: true,
			Clients: []db.Client{{UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Email: "xhttp-flow@test.com", Enabled: true}},
		},
	}
	config, err := xray.BuildConfig(inbounds)
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	encoded, _ := json.Marshal(config)
	text := string(encoded)
	for _, want := range []string{`"flow":"xtls-rprx-vision"`, `"network":"xhttp"`, `"network":"tcp"`, `xhttpSettings`, `realitySettings`} {
		if !strings.Contains(text, want) {
			t.Fatalf("VLESS+REALITY config missing %q: %s", want, text)
		}
	}
	// Verify non-REALITY inbounds don't get flow
	if strings.Contains(text, `"flow":"`) && !strings.Contains(text, `"flow":"xtls-rprx-vision"`) {
		t.Fatalf("unexpected flow value in config: %s", text)
	}
}

func TestBuildConfigGeneratesMissingRealityPrivateKey(t *testing.T) {
	inbounds := []db.Inbound{
		{
			ID: 8, UUID: "88888888-8888-4888-8888-888888888888",
			Remark: "auto-key-reality", Protocol: "vless", Port: 30050,
			Network: "tcp", Security: "reality",
			RealityDest: "www.example.com:443", RealityServerNames: "www.example.com",
			Enabled: true,
			Clients: []db.Client{{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Email: "auto-key@test.com", Enabled: true}},
		},
	}
	config, err := xray.BuildConfig(inbounds)
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	if len(config.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(config.Inbounds))
	}
	encoded, _ := json.Marshal(config)
	text := string(encoded)
	if !strings.Contains(text, "realitySettings") {
		t.Fatalf("auto-key inbound missing realitySettings: %s", text)
	}
	if !strings.Contains(text, "privateKey") {
		t.Fatalf("auto-key inbound missing auto-generated privateKey: %s", text)
	}
}
