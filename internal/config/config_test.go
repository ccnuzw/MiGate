package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imzyb/MiGate/internal/paths"
)

func TestLoadNormalizesExistingValuesWithoutInjectingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(path, []byte(`{"panel_username":" admin ","web_base_path":"panel"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PanelUsername != "admin" || cfg.WebPath != "/panel" {
		t.Fatalf("unexpected normalized values: %+v", cfg)
	}
	if cfg.PanelPort != 0 || cfg.DatabasePath != "" {
		t.Fatalf("load must not inject write defaults into omitted fields: %+v", cfg)
	}
}

func TestSaveAppliesDefaultsAndPanelPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := Save(path, Config{PanelUsername: "admin"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if cfg.PanelPort != paths.DefaultHTTPPort || cfg.WebPath != "/panel" || cfg.DatabasePath != paths.Database {
		t.Fatalf("save should persist normalized defaults, got %+v", cfg)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("saved config mode = %03o, want 640", got)
	}
}

func TestLoadRejectsInvalidExplicitValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "explicit zero port", body: `{"panel_port":0}`},
		{name: "invalid port", body: `{"panel_port":70000}`},
		{name: "relative database", body: `{"database_path":"relative.db"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "panel.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected invalid explicit config to fail")
			}
		})
	}
}

func TestLoadIgnoresUnknownFieldsForUpgradeCompatibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(path, []byte(`{"panel_port":9999,"unknown_config_path":"/var/lib/migate"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config with unknown field: %v", err)
	}
	if cfg.PanelPort != 9999 {
		t.Fatalf("expected known fields to load, got %+v", cfg)
	}
}

func TestUpdateDropsUnknownFieldsWhenSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(path, []byte(`{"panel_port":9999,"database_path":"/var/lib/migate/migate.db","unknown":true}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Update(path, func(cfg Config) (Config, error) {
		cfg.CertDomain = "example.com"
		return cfg, nil
	}); err != nil {
		t.Fatalf("update config with unknown fields: %v", err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(updated), `"unknown"`) {
		t.Fatalf("saved config should drop unknown fields: %s", string(updated))
	}
	if !strings.Contains(string(updated), `"cert_domain": "example.com"`) {
		t.Fatalf("saved config should include updated known field: %s", string(updated))
	}
}

func TestUpdateNormalizesWebBasePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := Save(path, Config{PanelUsername: "admin", DatabasePath: "/var/lib/migate/migate.db"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	updated, err := Update(path, func(cfg Config) (Config, error) {
		cfg.WebPath = "admin/"
		return cfg, nil
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if updated.WebPath != "/admin" {
		t.Fatalf("web_base_path normalized to %q, want /admin", updated.WebPath)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.WebPath != "/admin" {
		t.Fatalf("persisted web_base_path = %q, want /admin", loaded.WebPath)
	}
}

func TestSaveRejectsRelativeDatabasePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := Save(path, Config{DatabasePath: "relative.db"}); err == nil || !strings.Contains(err.Error(), "database_path must be absolute") {
		t.Fatalf("expected relative database path error, got %v", err)
	}
}

func TestSaveRejectsPanelPortOutOfRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := Save(path, Config{PanelPort: 70000, DatabasePath: "/var/lib/migate/migate.db"}); err == nil || !strings.Contains(err.Error(), "panel_port") {
		t.Fatalf("expected panel_port range error, got %v", err)
	}
}

func TestUpdatePersistsCertFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := Save(path, Config{PanelUsername: "admin"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if _, err := Update(path, func(cfg Config) (Config, error) {
		cfg.CertDomain = "example.com"
		cfg.CertEmail = "admin@example.com"
		return cfg, nil
	}); err != nil {
		t.Fatalf("update cert fields: %v", err)
	}
	domain, email, err := LoadCertFields(path)
	if err != nil {
		t.Fatalf("load cert fields: %v", err)
	}
	if domain != "example.com" || email != "admin@example.com" {
		t.Fatalf("unexpected cert fields: %q %q", domain, email)
	}
}
