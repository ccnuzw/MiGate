package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func join(parts ...string) string { return strings.Join(parts, "") }

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, ".."))
}

func read(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func readIfExists(t *testing.T, parts ...string) (string, bool) {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b), true
}

func TestServiceRunsSinglePrebuiltBinary(t *testing.T) {
	service := read(t, "packaging", "migate.service")
	if !strings.Contains(service, "ExecStart=/usr/local/bin/migate") {
		t.Fatalf("service must run single prebuilt binary:\n%s", service)
	}
	forbidden := []string{"python", "uv", "pip", "npm", "migate-proxy", join("open", "vpn"), "tun", "egress", "remote", "leak", "rollout"}
	lower := strings.ToLower(service)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("service must not contain %q:\n%s", word, service)
		}
	}
}

func TestInstallerDownloadsReleaseTarballOnly(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"migate-linux-${ARCH}.tar.gz",
		"/var/lib/migate",
		"/var/lib/migate/backups",
		"/var/lib/migate/versions.json",
		"/etc/migate/cores",
		"systemctl enable migate",
		"systemctl restart migate",
		"detect_existing_install()",
		"read_existing_config_defaults()",
		"REGENERATE_CONFIG",
		"--dry-run",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer missing %q:\n%s", want, script)
		}
	}
	forbidden := []string{"git clone", "pip install", "uv ", "python3 -m", "npm install", join("open", "vpn"), "migate-proxy", "rollout", "leak", "egress"}
	lower := strings.ToLower(script)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("installer must not contain %q:\n%s", word, script)
		}
	}
}

func TestRuntimeContractDoesNotAdvertiseLegacyPathsOrServices(t *testing.T) {
	for _, file := range []string{
		filepath.Join("packaging", "install.sh"),
		filepath.Join("packaging", "uninstall.sh"),
		filepath.Join("packaging", "migate.service"),
		filepath.Join("cmd", "migate", "main.go"),
		filepath.Join("internal", "web", "core_handlers.go"),
		filepath.Join("internal", "singbox", "manager.go"),
	} {
		content := read(t, file)
		for _, forbidden := range []string{
			"/usr/local/" + "migate",
			"/etc/sing-box/" + "config.json",
			"/usr/local/etc/" + "xray",
			"migate-" + "singbox",
			"ExecStart=/usr/local/bin/xray run -config",
			"ExecStart=/usr/local/bin/sing-box run -c /etc/sing-box/" + "config.json",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not contain legacy runtime contract %q", file, forbidden)
			}
		}
	}
}

func TestRemovedLegacyRuntimeCodeIsFullyRemoved(t *testing.T) {
	root := repoRoot(t)
	for _, removedDir := range []string{
		filepath.Join(root, "internal", join("vpn", "gate")),
	} {
		if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
			t.Fatalf("removed VPN feature implementation directory must be removed: %s", removedDir)
		}
	}

	if _, exists := readIfExists(t, "internal", "web", "static", "app.js"); exists {
		t.Fatal("removed internal/web/static/app.js must stay absent after frontend split")
	}

	for _, file := range []string{
		filepath.Join("internal", "db", "store.go"),
		filepath.Join("internal", "web", "router.go"),
		filepath.Join("internal", "web", "auth.go"),
		filepath.Join("internal", "xray", "config.go"),
		filepath.Join("cmd", "migate", "main.go"),
		filepath.Join("packaging", "install.sh"),
		filepath.Join("web", "src", "App.tsx"),
	} {
		content := strings.ToLower(read(t, file))
		for _, forbidden := range []string{join("vpn", "gate"), join("vpn", " gate"), join("soft", "ether"), join("micro", "socks"), join("vpn", "cmd"), join("vpn", "client")} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not contain removed VPN feature marker %q", file, forbidden)
			}
		}
	}
}

func TestReadmeIncludesSimpleInstallAndUsage(t *testing.T) {
	readme := read(t, "README.md")
	for _, want := range []string{
		"bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh)",
		"MIGATE_VERSION=",
		"http://127.0.0.1:9999/panel",
		"reverse proxy",
		"public_host",
		"Web path, default `/panel`",
		"systemctl status migate",
		"systemctl restart migate",
		"/etc/migate/panel.json",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README missing simple usage marker %q", want)
		}
	}
	for _, forbiddenName := range []string{join("MiGate Go", " Lite"), "Go Lite"} {
		if strings.Contains(readme, forbiddenName) {
			t.Fatalf("README should use MiGate as the product name, found %q", forbiddenName)
		}
	}
}
