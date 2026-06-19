package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	panelcfg "github.com/imzyb/MiGate/internal/config"
)

func TestServiceUpdatesPasswordWithoutPersistingPlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := panelcfg.Save(path, panelcfg.Config{PanelUsername: "admin"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	service := Service{
		ConfigPath:     path,
		HashPassword:   func(password string) (string, error) { return "$migate$argon2id$v=19$hash-for-" + password, nil },
		IsPasswordHash: func(value string) bool { return strings.HasPrefix(value, "$migate$argon2id$v=19$") },
	}
	password := "secret"
	if err := service.Update(Request{PanelPassword: &password}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(raw), `"secret"`) {
		t.Fatalf("plain password leaked into config: %s", string(raw))
	}
	if !strings.Contains(string(raw), "$migate$argon2id$v=19$hash-for-secret") {
		t.Fatalf("hashed password missing from config: %s", string(raw))
	}
	response, err := service.Get()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if !response.HasPassword {
		t.Fatal("settings response should report password presence")
	}
}

func TestServiceNormalizesWebBasePathAndSavesCertFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := panelcfg.Save(path, panelcfg.Config{PanelUsername: "admin"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	service := Service{ConfigPath: path, HashPassword: func(password string) (string, error) { return password, nil }}
	webPath := "console/"
	if err := service.Update(Request{WebPath: &webPath}); err != nil {
		t.Fatalf("update web path: %v", err)
	}
	if err := service.SaveCert("example.com", "admin@example.com"); err != nil {
		t.Fatalf("save cert: %v", err)
	}
	response, err := service.Get()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if response.WebPath != "/console" {
		t.Fatalf("web path = %q, want /console", response.WebPath)
	}
	if response.CertDomain != "example.com" || response.CertEmail != "admin@example.com" {
		t.Fatalf("unexpected cert fields: %+v", response)
	}
}
