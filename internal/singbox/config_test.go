package singbox

import (
	"os"
	"testing"

	"github.com/imzyb/MiGate/internal/db"
)

func TestBuildConfig_Hysteria2Inbound(t *testing.T) {
	inbounds := []db.Inbound{
		{
			ID: 1, Protocol: "hysteria2", Port: 21001, Enabled: true,
			Hy2UpMbps: 100, Hy2DownMbps: 50,
			Hy2Obfs: "salamander", Hy2ObfsPassword: "obfs-pass",
			Clients: []db.Client{
				{ID: 1, UUID: "client-pass-1", Email: "user1@test", Enabled: true},
			},
		},
	}

	cfg := BuildConfig(inbounds)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
	}
	ib := cfg.Inbounds[0]
	if ib.Type != "hysteria2" {
		t.Errorf("expected type hysteria2, got %s", ib.Type)
	}
	if ib.ListenPort != SBBasePort {
		t.Errorf("expected port %d, got %d", SBBasePort, ib.ListenPort)
	}
	if ib.UpMbps != 100 {
		t.Errorf("expected up_mbps 100, got %d", ib.UpMbps)
	}
	if ib.DownMbps != 50 {
		t.Errorf("expected down_mbps 50, got %d", ib.DownMbps)
	}
	if ib.TLS == nil || !ib.TLS.Enabled {
		t.Error("expected TLS enabled")
	}
	if ib.Obfs == nil || ib.Obfs.Type != "salamander" {
		t.Errorf("expected obfs salamander, got %v", ib.Obfs)
	}
	if ib.Obfs.Password != "obfs-pass" {
		t.Errorf("expected obfs password obfs-pass, got %s", ib.Obfs.Password)
	}
	if len(ib.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(ib.Users))
	}
	if ib.Users[0].Password != "client-pass-1" {
		t.Errorf("expected password client-pass-1, got %s", ib.Users[0].Password)
	}
}

func TestBuildConfig_DisabledInboundSkipped(t *testing.T) {
	inbounds := []db.Inbound{
		{ID: 1, Protocol: "hysteria2", Port: 21001, Enabled: false},
		{ID: 2, Protocol: "hysteria2", Port: 21002, Enabled: true,
			Clients: []db.Client{{ID: 1, UUID: "p1", Enabled: true}}},
	}

	cfg := BuildConfig(inbounds)
	if len(cfg.Inbounds) != 1 {
		t.Errorf("expected 1 inbound (disabled skipped), got %d", len(cfg.Inbounds))
	}
}

func TestBuildConfig_NonHy2Skipped(t *testing.T) {
	inbounds := []db.Inbound{
		{ID: 1, Protocol: "vless", Port: 10001, Enabled: true,
			Clients: []db.Client{{ID: 1, UUID: "u1", Enabled: true}}},
		{ID: 2, Protocol: "hysteria2", Port: 21001, Enabled: true,
			Clients: []db.Client{{ID: 2, UUID: "p2", Enabled: true}}},
	}

	cfg := BuildConfig(inbounds)
	if len(cfg.Inbounds) != 1 {
		t.Errorf("expected 1 inbound (vless skipped), got %d", len(cfg.Inbounds))
	}
	if cfg.Inbounds[0].Type != "hysteria2" {
		t.Errorf("expected hysteria2, got %s", cfg.Inbounds[0].Type)
	}
}

func TestBuildConfig_HasDirectOutbound(t *testing.T) {
	cfg := BuildConfig(nil)
	found := false
	for _, o := range cfg.Outbounds {
		if o.Type == "direct" && o.Tag == "direct" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected direct outbound with tag direct")
	}
}

func TestBuildConfig_PortAllocation(t *testing.T) {
	inbounds := []db.Inbound{}
	for i := 0; i < 3; i++ {
		inbounds = append(inbounds, db.Inbound{
			ID: int64(i + 1), Protocol: "hysteria2", Port: 21000 + i, Enabled: true,
			Clients: []db.Client{{ID: 1, UUID: "p", Enabled: true}},
		})
	}

	cfg := BuildConfig(inbounds)
	if len(cfg.Inbounds) != 3 {
		t.Fatalf("expected 3 inbounds, got %d", len(cfg.Inbounds))
	}
	for i, ib := range cfg.Inbounds {
		expectedPort := SBBasePort + i
		if ib.ListenPort != expectedPort {
			t.Errorf("inbound %d: expected port %d, got %d", i, expectedPort, ib.ListenPort)
		}
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	// Use temp dir
	origCert := CertFile
	origKey := KeyFile
	origDir := DefaultConfigDir
	defer func() {
		CertFile = origCert
		KeyFile = origKey
		DefaultConfigDir = origDir
	}()

	certFile := t.TempDir() + "/server.crt"
	keyFile := t.TempDir() + "/server.key"
	DefaultConfigDir = t.TempDir()
	CertFile = certFile
	KeyFile = keyFile

	if err := GenerateSelfSignedCert(); err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}

	if _, err := os.Stat(certFile); err != nil {
		t.Errorf("cert file not created: %v", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Errorf("key file not created: %v", err)
	}
}

func TestNextPort(t *testing.T) {
	if p := NextPort(0); p != SBBasePort {
		t.Errorf("expected %d, got %d", SBBasePort, p)
	}
	if p := NextPort(1); p != SBBasePort+1 {
		t.Errorf("expected %d, got %d", SBBasePort+1, p)
	}
}

func TestBuildConfig_TUICInbound(t *testing.T) {
	inbounds := []db.Inbound{
		{
			ID: 1, Protocol: "tuic", Port: 21010, Enabled: true,
			TuicCongestionControl: "cubic",
			TuicZeroRTT:           true,
			Clients: []db.Client{
				{ID: 1, UUID: "tuic-pass-1", Email: "user1@test", Enabled: true},
			},
		},
	}

	cfg := BuildConfig(inbounds)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
	}
	ib := cfg.Inbounds[0]
	if ib.Type != "tuic" {
		t.Errorf("expected type tuic, got %s", ib.Type)
	}
	if ib.ListenPort != SBBasePort {
		t.Errorf("expected port %d, got %d", SBBasePort, ib.ListenPort)
	}
	if ib.TLS == nil || !ib.TLS.Enabled {
		t.Error("expected TLS enabled for TUIC")
	}
	if ib.CongestionControl != "cubic" {
		t.Errorf("expected congestion_control cubic, got %s", ib.CongestionControl)
	}
	if !ib.ZeroRTTHandshake {
		t.Error("expected zero_rtt_handshake true")
	}
	if len(ib.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(ib.Users))
	}
	if ib.Users[0].Password != "tuic-pass-1" {
		t.Errorf("expected password tuic-pass-1, got %s", ib.Users[0].Password)
	}
}

func TestBuildConfig_WireGuardInbound(t *testing.T) {
	inbounds := []db.Inbound{
		{
			ID: 1, Protocol: "wireguard", Port: 21020, Enabled: true,
			WgPrivateKey:    "server-private-key-abc",
			WgAddress:       "10.0.0.1/24",
			WgPeerPublicKey: "peer-public-key-xyz",
			WgAllowedIPs:    "0.0.0.0/0, ::/0",
			WgEndpoint:      "peer.example.com:51820",
			WgPresharedKey:  "preshared-key-123",
			WgMTU:           1420,
			Clients:         []db.Client{{ID: 1, UUID: "ignored", Enabled: true}},
		},
	}

	cfg := BuildConfig(inbounds)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
	}
	ib := cfg.Inbounds[0]
	if ib.Type != "wireguard" {
		t.Errorf("expected type wireguard, got %s", ib.Type)
	}
	if ib.PrivateKey != "server-private-key-abc" {
		t.Errorf("expected private_key, got %s", ib.PrivateKey)
	}
	if len(ib.Address) != 1 || ib.Address[0] != "10.0.0.1/24" {
		t.Errorf("expected address [10.0.0.1/24], got %v", ib.Address)
	}
	if ib.MTU != 1420 {
		t.Errorf("expected MTU 1420, got %d", ib.MTU)
	}
	if len(ib.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(ib.Peers))
	}
	if ib.Peers[0].PublicKey != "peer-public-key-xyz" {
		t.Errorf("expected peer public key, got %s", ib.Peers[0].PublicKey)
	}
	if ib.Peers[0].PreSharedKey != "preshared-key-123" {
		t.Errorf("expected preshared key, got %s", ib.Peers[0].PreSharedKey)
	}
	if ib.Peers[0].Endpoint != "peer.example.com:51820" {
		t.Errorf("expected endpoint, got %s", ib.Peers[0].Endpoint)
	}
	if len(ib.Peers[0].AllowedIPs) != 2 {
		t.Fatalf("expected 2 allowed IPs, got %v", ib.Peers[0].AllowedIPs)
	}
	if ib.Peers[0].AllowedIPs[0] != "0.0.0.0/0" {
		t.Errorf("expected allowedIPs[0] 0.0.0.0/0, got %s", ib.Peers[0].AllowedIPs[0])
	}
	// WireGuard should NOT have users
	if len(ib.Users) != 0 {
		t.Errorf("expected 0 users for wireguard, got %d", len(ib.Users))
	}
}

func TestBuildConfig_ShadowTLSInbound(t *testing.T) {
	inbounds := []db.Inbound{
		{
			ID: 1, Protocol: "shadowtls", Port: 21030, Enabled: true,
			ShadowTLSPassword: "shadow-pass-1",
			ShadowTLSVersion:  2,
			TLSSNI:            "cloudflare.com",
			Clients: []db.Client{
				{ID: 1, UUID: "user-pass-1", Email: "user1@test", Enabled: true},
			},
		},
	}

	cfg := BuildConfig(inbounds)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
	}
	ib := cfg.Inbounds[0]
	if ib.Type != "shadowtls" {
		t.Errorf("expected type shadowtls, got %s", ib.Type)
	}
	if ib.Password != "shadow-pass-1" {
		t.Errorf("expected password, got %s", ib.Password)
	}
	if ib.Version != 2 {
		t.Errorf("expected version 2, got %d", ib.Version)
	}
	if ib.HandshakeServerName != "cloudflare.com" {
		t.Errorf("expected handshake_server_name cloudflare.com, got %s", ib.HandshakeServerName)
	}
	if ib.TLS != nil {
		t.Error("expected nil TLS for shadowtls (inbound has no TLS config)")
	}
	if len(ib.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(ib.Users))
	}
	if ib.Users[0].Password != "user-pass-1" {
		t.Errorf("expected user password, got %s", ib.Users[0].Password)
	}
}

func TestBuildConfig_MixedSingBoxProtocols(t *testing.T) {
	inbounds := []db.Inbound{
		{ID: 1, Protocol: "vless", Port: 10001, Enabled: true,
			Clients: []db.Client{{ID: 1, UUID: "u1", Enabled: true}}},
		{ID: 2, Protocol: "hysteria2", Port: 21001, Enabled: true,
			Clients: []db.Client{{ID: 2, UUID: "p2", Enabled: true}}},
		{ID: 3, Protocol: "tuic", Port: 21002, Enabled: true,
			Clients: []db.Client{{ID: 3, UUID: "tp3", Enabled: true}}},
		{ID: 4, Protocol: "wireguard", Port: 21003, Enabled: true,
			WgPrivateKey: "wg-key", WgAddress: "10.0.0.1/24", WgPeerPublicKey: "peer-key"},
		{ID: 5, Protocol: "shadowtls", Port: 21004, Enabled: true,
			ShadowTLSPassword: "st-pass",
			Clients:           []db.Client{{ID: 5, UUID: "st-user", Enabled: true}}},
		{ID: 6, Protocol: "shadowsocks", Port: 30001, Enabled: true,
			Clients: []db.Client{{ID: 6, UUID: "ss-u", Enabled: true}}},
	}

	cfg := BuildConfig(inbounds)

	// Expect 4 sing-box inbounds (hysteria2, tuic, wireguard, shadowtls)
	if len(cfg.Inbounds) != 4 {
		t.Fatalf("expected 4 sing-box inbounds, got %d", len(cfg.Inbounds))
	}
	types := make(map[string]bool)
	for _, ib := range cfg.Inbounds {
		types[ib.Type] = true
	}
	for _, proto := range []string{"hysteria2", "tuic", "wireguard", "shadowtls"} {
		if !types[proto] {
			t.Errorf("missing sing-box protocol: %s", proto)
		}
	}
	if types["vless"] || types["shadowsocks"] {
		t.Error("non-sing-box protocols should be skipped")
	}
}