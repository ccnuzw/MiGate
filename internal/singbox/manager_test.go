package singbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("SINGBOX_MANAGER_TEST_HELPER") == "1" {
		if hasTestHelperArg("check-fail") {
			os.Stderr.WriteString("outbounds[3]: dns outbound is deprecated\n")
			os.Exit(1)
		}
		if hasTestHelperArg("restart-fail") {
			os.Stderr.WriteString("restart failed\n")
			os.Exit(1)
		}
		if out := os.Getenv("SINGBOX_MANAGER_TEST_STDOUT"); out != "" {
			os.Stdout.WriteString(out)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func hasTestHelperArg(want string) bool {
	for _, arg := range os.Args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestNormalizeVersionDropsTagsSuffix(t *testing.T) {
	raw := "sing-box version 1.13.13\nEnvironment: go1.25 linux/amd64\nTags: with_quic,with_gvisor\n"
	got := NormalizeVersion(raw)
	if got != "sing-box version 1.13.13" {
		t.Fatalf("expected first version line without Tags suffix, got %q", got)
	}
	if strings.Contains(got, "Tags:") {
		t.Fatalf("normalized version must not include Tags suffix: %q", got)
	}
}

func TestServiceNameUsesUpstreamSystemdUnit(t *testing.T) {
	if got := ServiceName(); got != "sing-box" {
		t.Fatalf("expected sing-box service name, got %q", got)
	}
}

func TestRuntimeServiceNamePrefersNewService(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	var calls []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
		cs := []string{"-test.run=TestMain", "--", "ok"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1", "SINGBOX_MANAGER_TEST_STDOUT=loaded\n")
		return cmd
	}

	if got := RuntimeServiceName(); got != "sing-box" {
		t.Fatalf("expected sing-box service, got %q", got)
	}
	want := []string{"systemctl show sing-box --property=LoadState --value"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestRuntimeServiceNameFallsBackToLegacyService(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	var calls []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		call := strings.TrimSpace(name + " " + strings.Join(args, " "))
		calls = append(calls, call)
		cs := []string{"-test.run=TestMain", "--", "ok"}
		cmd := exec.Command(os.Args[0], cs...)
		out := "loaded\n"
		if strings.Contains(call, "show sing-box ") {
			out = "not-found\n"
		}
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1", "SINGBOX_MANAGER_TEST_STDOUT="+out)
		return cmd
	}

	if got := RuntimeServiceName(); got != "migate-singbox" {
		t.Fatalf("expected legacy service, got %q", got)
	}
	want := []string{
		"systemctl show sing-box --property=LoadState --value",
		"systemctl show migate-singbox --property=LoadState --value",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestManagementReportsLegacyServiceAsManaged(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	var calls []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		call := strings.TrimSpace(name + " " + strings.Join(args, " "))
		calls = append(calls, call)
		cs := []string{"-test.run=TestMain", "--", "ok"}
		cmd := exec.Command(os.Args[0], cs...)
		out := "loaded\n"
		if strings.Contains(call, "show sing-box ") {
			out = "not-found\n"
		}
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1", "SINGBOX_MANAGER_TEST_STDOUT="+out)
		return cmd
	}

	got := Management()
	if !got.Managed || got.Service != "migate-singbox" {
		t.Fatalf("expected legacy service to be managed, got %+v", got)
	}
	want := []string{
		"systemctl show sing-box --property=LoadState --value",
		"systemctl show migate-singbox --property=LoadState --value",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestManagementReportsUnmanagedWhenNoServiceExists(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestMain", "--", "ok"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1", "SINGBOX_MANAGER_TEST_STDOUT=not-found\n")
		return cmd
	}

	got := Management()
	if got.Managed || got.Service != "sing-box" {
		t.Fatalf("expected unmanaged primary service fallback, got %+v", got)
	}
	if status := Status(); status != "not_managed" {
		t.Fatalf("expected not_managed status, got %q", status)
	}
}

func TestManagementDoesNotTreatSystemctlErrorsAsManaged(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestMain", "--", "restart-fail"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1", "SINGBOX_MANAGER_TEST_STDOUT=Failed to connect to bus: permission denied\n")
		return cmd
	}

	got := Management()
	if got.Managed {
		t.Fatalf("systemctl errors must not be treated as managed: %+v", got)
	}
	if status := Status(); status != "not_managed" {
		t.Fatalf("expected not_managed when systemctl fails, got %q", status)
	}
}

func TestRestartServiceFallsBackToLegacyService(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	var calls []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		call := strings.TrimSpace(name + " " + strings.Join(args, " "))
		calls = append(calls, call)
		cs := []string{"-test.run=TestMain", "--", "ok"}
		cmd := exec.Command(os.Args[0], cs...)
		out := "loaded\n"
		if strings.Contains(call, "show sing-box ") {
			out = "not-found\n"
		}
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1", "SINGBOX_MANAGER_TEST_STDOUT="+out)
		return cmd
	}

	if _, err := RestartService(); err != nil {
		t.Fatalf("restart legacy service: %v", err)
	}
	want := []string{
		"systemctl show sing-box --property=LoadState --value",
		"systemctl show migate-singbox --property=LoadState --value",
		"systemctl restart migate-singbox",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestApplyConfigDoesNotOverwriteExistingConfigWhenCheckFails(t *testing.T) {
	origDir := DefaultConfigDir
	origPath := DefaultConfigPath
	origExec := execCommand
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
		execCommand = origExec
	}()

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	oldConfig := []byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"old"}]}`)
	if err := os.WriteFile(DefaultConfigPath, oldConfig, 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestMain", "--", "check-fail"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1")
		return cmd
	}

	err := ApplyConfig([]byte(`{"outbounds":[{"type":"dns","tag":"bad"}]}`))
	if err == nil || !strings.Contains(err.Error(), "dns outbound is deprecated") {
		t.Fatalf("expected sing-box check error, got %v", err)
	}
	got, readErr := os.ReadFile(DefaultConfigPath)
	if readErr != nil {
		t.Fatalf("read config after failed apply: %v", readErr)
	}
	if string(got) != string(oldConfig) {
		t.Fatalf("failed check must not overwrite config, got %s want %s", got, oldConfig)
	}
}

func TestApplyConfigRestoresExistingConfigWhenRestartFails(t *testing.T) {
	origDir := DefaultConfigDir
	origPath := DefaultConfigPath
	origExec := execCommand
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
		execCommand = origExec
	}()

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	oldConfig := []byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"old"}]}`)
	if err := os.WriteFile(DefaultConfigPath, oldConfig, 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	execCommand = func(name string, args ...string) *exec.Cmd {
		helperArg := "ok"
		if name == "systemctl" && len(args) > 0 && args[0] == "restart" {
			helperArg = "restart-fail"
		}
		cs := []string{"-test.run=TestMain", "--", helperArg}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1")
		return cmd
	}

	err := ApplyConfig([]byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"new"}]}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl restart sing-box failed") {
		t.Fatalf("expected restart error, got %v", err)
	}
	got, readErr := os.ReadFile(DefaultConfigPath)
	if readErr != nil {
		t.Fatalf("read config after failed restart: %v", readErr)
	}
	if string(got) != string(oldConfig) {
		t.Fatalf("failed restart must restore config, got %s want %s", got, oldConfig)
	}
}

func TestApplyConfigDoesNotOverwriteUserBakFile(t *testing.T) {
	origDir := DefaultConfigDir
	origPath := DefaultConfigPath
	origExec := execCommand
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
		execCommand = origExec
	}()

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	oldConfig := []byte(`{"outbounds":[{"type":"direct","tag":"old"}]}`)
	userBackup := []byte("user backup")
	if err := os.WriteFile(DefaultConfigPath, oldConfig, 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	if err := os.WriteFile(DefaultConfigPath+".bak", userBackup, 0644); err != nil {
		t.Fatalf("write user backup: %v", err)
	}
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestMain", "--", "ok"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1")
		return cmd
	}

	if err := ApplyConfig([]byte(`{"outbounds":[{"type":"direct","tag":"new"}]}`)); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	got, err := os.ReadFile(DefaultConfigPath + ".bak")
	if err != nil {
		t.Fatalf("read user backup: %v", err)
	}
	if string(got) != string(userBackup) {
		t.Fatalf("user backup must remain untouched, got %s want %s", got, userBackup)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".config-backup-*.json"))
	if err != nil {
		t.Fatalf("glob temp backups: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary backup should be removed after success, got %+v", matches)
	}
}

func TestApplyConfigReportsInstalledConfigWhenRestartFailsWithoutBackup(t *testing.T) {
	origDir := DefaultConfigDir
	origPath := DefaultConfigPath
	origExec := execCommand
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
		execCommand = origExec
	}()

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	newConfig := []byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"new"}]}`)
	execCommand = func(name string, args ...string) *exec.Cmd {
		helperArg := "ok"
		if name == "systemctl" && len(args) > 0 && args[0] == "restart" {
			helperArg = "restart-fail"
		}
		cs := []string{"-test.run=TestMain", "--", helperArg}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1")
		return cmd
	}

	err := ApplyConfig(newConfig)
	if err == nil || !strings.Contains(err.Error(), "new config was installed but service did not start because no previous config was available to restore") {
		t.Fatalf("expected explicit no-backup restart error, got %v", err)
	}
	got, readErr := os.ReadFile(DefaultConfigPath)
	if readErr != nil {
		t.Fatalf("read config after failed first apply: %v", readErr)
	}
	if string(got) != string(newConfig) {
		t.Fatalf("first checked config should remain installed when no backup exists, got %s want %s", got, newConfig)
	}
}

func TestApplyConfigRestartsRestoredConfigWhenNewConfigRestartFails(t *testing.T) {
	origDir := DefaultConfigDir
	origPath := DefaultConfigPath
	origExec := execCommand
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
		execCommand = origExec
	}()

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	if err := os.WriteFile(DefaultConfigPath, []byte(`{"outbounds":[{"type":"direct","tag":"old"}]}`), 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	var restarts int32
	execCommand = func(name string, args ...string) *exec.Cmd {
		helperArg := "ok"
		if name == "systemctl" && len(args) > 0 && args[0] == "restart" {
			if atomic.AddInt32(&restarts, 1) == 1 {
				helperArg = "restart-fail"
			}
		}
		cs := []string{"-test.run=TestMain", "--", helperArg}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1")
		return cmd
	}

	err := ApplyConfig([]byte(`{"outbounds":[{"type":"direct","tag":"new"}]}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl restart sing-box failed") {
		t.Fatalf("expected first restart error, got %v", err)
	}
	if got := atomic.LoadInt32(&restarts); got != 2 {
		t.Fatalf("expected restart of new then restored config, got %d restarts", got)
	}
}

func TestListeningUDPPortsParsesSSOutput(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestMain", "--", "ok"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "SINGBOX_MANAGER_TEST_HELPER=1", "SINGBOX_MANAGER_TEST_STDOUT=UNCONN 0 0 0.0.0.0:20001 0.0.0.0:*\nUNCONN 0 0 [::]:20002 [::]:*\n")
		return cmd
	}
	got := ListeningUDPPorts([]int{20001, 20002, 20003})
	if !got[20001] || !got[20002] || got[20003] {
		t.Fatalf("unexpected UDP listening map: %+v", got)
	}
}
