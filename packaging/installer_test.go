package packaging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestInstallerIs轻量面板StyleInteractiveReleaseInstaller(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"read -r -p \"Panel port",
		"read -r -p \"Panel username",
		"read -r -s -p \"Panel password",
		"read -r -p \"Web base path",
		"/etc/migate/panel.json",
		"panel_port",
		"panel_username",
		"panel_password",
		"web_base_path",
		"migate-linux-${ARCH}.tar.gz",
		"systemctl enable migate",
		"systemctl start migate",
		"WebUI",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer missing %q", want)
		}
	}

	forbidden := []string{"git clone", "pip install", "uv ", "python3 -m", "npm install", "go build", "openvpn", "migate-proxy", "rollout", "leak", "egress"}
	lower := strings.ToLower(script)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("installer must not contain %q", word)
		}
	}
}

func TestServiceUsesGeneratedPanelConfigAndSingleBinary(t *testing.T) {
	service := read(t, "packaging", "migate.service")
	for _, want := range []string{
		"ExecStart=/usr/local/migate/migate",
		"--config /etc/migate/panel.json",
		"User=root",
		"Restart=on-failure",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("service missing %q: %s", want, service)
		}
	}
	forbidden := []string{"python", "uv", "pip", "npm", "openvpn", "tun", "egress", "remote", "leak", "rollout"}
	lower := strings.ToLower(service)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("service must not contain %q: %s", word, service)
		}
	}
}
