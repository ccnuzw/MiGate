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