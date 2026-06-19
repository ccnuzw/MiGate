package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imzyb/MiGate/internal/panelconfig"
	"github.com/imzyb/MiGate/internal/paths"
)

type Config struct {
	PanelPort     int    `json:"panel_port"`
	PanelUsername string `json:"panel_username"`
	PanelPassword string `json:"panel_password"`
	WebPath       string `json:"web_base_path"`
	PublicHost    string `json:"public_host,omitempty"`
	TrustProxy    bool   `json:"trust_proxy,omitempty"`
	DatabasePath  string `json:"database_path"`
	CertDomain    string `json:"cert_domain,omitempty"`
	CertEmail     string `json:"cert_email,omitempty"`
}

func Default() Config {
	return Config{
		PanelPort:    paths.DefaultHTTPPort,
		WebPath:      "/panel",
		DatabasePath: paths.Database,
	}
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return Config{}, err
	}
	if err := rejectUnknownFields(raw); err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	cfg = NormalizeLoaded(cfg)
	if err := ValidateLoaded(cfg, raw); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return panelconfig.WriteFile(path, b)
}

func Normalize(cfg Config) Config {
	defaults := Default()
	if cfg.PanelPort == 0 {
		cfg.PanelPort = defaults.PanelPort
	}
	cfg.PanelUsername = strings.TrimSpace(cfg.PanelUsername)
	cfg.WebPath = normalizeWebPath(cfg.WebPath, defaults.WebPath)
	cfg.PublicHost = strings.TrimSpace(cfg.PublicHost)
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		cfg.DatabasePath = defaults.DatabasePath
	} else {
		cfg.DatabasePath = strings.TrimSpace(cfg.DatabasePath)
	}
	cfg.CertDomain = strings.TrimSpace(cfg.CertDomain)
	cfg.CertEmail = strings.TrimSpace(cfg.CertEmail)
	return cfg
}

func NormalizeLoaded(cfg Config) Config {
	cfg.PanelUsername = strings.TrimSpace(cfg.PanelUsername)
	if strings.TrimSpace(cfg.WebPath) != "" {
		cfg.WebPath = normalizeWebPath(cfg.WebPath, "")
	}
	cfg.PublicHost = strings.TrimSpace(cfg.PublicHost)
	cfg.DatabasePath = strings.TrimSpace(cfg.DatabasePath)
	cfg.CertDomain = strings.TrimSpace(cfg.CertDomain)
	cfg.CertEmail = strings.TrimSpace(cfg.CertEmail)
	return cfg
}

func Validate(cfg Config) error {
	if cfg.PanelPort < 1 || cfg.PanelPort > 65535 {
		return fmt.Errorf("panel_port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		return fmt.Errorf("database_path is required")
	}
	if strings.Contains(cfg.DatabasePath, "\x00") {
		return fmt.Errorf("database_path is invalid")
	}
	if !filepath.IsAbs(cfg.DatabasePath) {
		return fmt.Errorf("database_path must be absolute")
	}
	return nil
}

func ValidateLoaded(cfg Config, rawFields ...map[string]json.RawMessage) error {
	if len(rawFields) > 0 {
		if err := rejectUnknownFields(rawFields[0]); err != nil {
			return err
		}
	}
	panelPortPresent := false
	if len(rawFields) > 0 && rawFields[0] != nil {
		_, panelPortPresent = rawFields[0]["panel_port"]
	}
	if cfg.PanelPort < 0 || cfg.PanelPort > 65535 || (panelPortPresent && cfg.PanelPort == 0) {
		return fmt.Errorf("panel_port must be between 1 and 65535")
	}
	if cfg.DatabasePath != "" {
		if strings.Contains(cfg.DatabasePath, "\x00") {
			return fmt.Errorf("database_path is invalid")
		}
		if !filepath.IsAbs(cfg.DatabasePath) {
			return fmt.Errorf("database_path must be absolute")
		}
	}
	return nil
}

func Update(path string, mutate func(Config) (Config, error)) (Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return Config{}, err
	}
	cfg, err = mutate(cfg)
	if err != nil {
		return Config{}, err
	}
	cfg = Normalize(cfg)
	return cfg, Save(path, cfg)
}

func LoadCertFields(path string) (domain, email string, err error) {
	cfg, err := Load(path)
	if err != nil {
		return "", "", err
	}
	return cfg.CertDomain, cfg.CertEmail, nil
}

func normalizeWebPath(path, fallback string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = fallback
	}
	if path == "/" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}

func rejectUnknownFields(raw map[string]json.RawMessage) error {
	if raw == nil {
		return nil
	}
	allowed := allowedConfigFields()
	var unknown []string
	for key := range raw {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("unknown config field(s): %s", strings.Join(unknown, ", "))
}

func allowedConfigFields() map[string]bool {
	return map[string]bool{
		"panel_port":     true,
		"panel_username": true,
		"panel_password": true,
		"web_base_path":  true,
		"public_host":    true,
		"trust_proxy":    true,
		"database_path":  true,
		"cert_domain":    true,
		"cert_email":     true,
	}
}
