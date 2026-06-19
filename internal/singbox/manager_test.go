package singbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeCommandRunner struct {
	runOutput func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (r fakeCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	_, err := r.RunOutput(ctx, name, args...)
	return err
}

func (r fakeCommandRunner) RunOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.runOutput == nil {
		return nil, nil
	}
	return r.runOutput(ctx, name, args...)
}

func setCommandRunner(t *testing.T, runner fakeCommandRunner) {
	t.Helper()
	origRunner := commandRunner
	commandRunner = runner
	t.Cleanup(func() { commandRunner = origRunner })
}

func commandCall(name string, args ...string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func setTempSingboxBinary(t *testing.T, mode os.FileMode) string {
	t.Helper()
	origBinary := DefaultBinaryPath
	path := filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	DefaultBinaryPath = path
	t.Cleanup(func() { DefaultBinaryPath = origBinary })
	return path
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

func TestServiceNameUsesMiGateSystemdUnit(t *testing.T) {
	if got := ServiceName(); got != "migate-sing-box" {
		t.Fatalf("expected MiGate sing-box service name, got %q", got)
	}
}

func TestManagementUsesOnlyStandardService(t *testing.T) {
	var calls []string
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, commandCall(name, args...))
		return []byte("loaded\n"), nil
	}})

	got := Management()
	if !got.Managed || got.Service != "migate-sing-box" {
		t.Fatalf("expected standard service to be managed, got %+v", got)
	}
	want := []string{"systemctl show migate-sing-box --property=LoadState --value"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestManagementDoesNotFallbackToLegacyService(t *testing.T) {
	var calls []string
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, commandCall(name, args...))
		return []byte("not-found\n"), nil
	}})

	got := Management()
	if got.Managed || got.Service != "migate-sing-box" {
		t.Fatalf("expected unmanaged standard service, got %+v", got)
	}
	want := []string{"systemctl show migate-sing-box --property=LoadState --value"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestManagementReportsUnmanagedWhenNoServiceExists(t *testing.T) {
	origBinary := DefaultBinaryPath
	defer func() {
		DefaultBinaryPath = origBinary
	}()
	DefaultBinaryPath = filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(DefaultBinaryPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("not-found\n"), nil
	}})

	got := Management()
	if got.Managed || got.Service != "migate-sing-box" {
		t.Fatalf("expected unmanaged primary service fallback, got %+v", got)
	}
	if status := Status(); status != "not_managed" {
		t.Fatalf("expected not_managed status, got %q", status)
	}
}

func TestStatusPreservesSystemdStates(t *testing.T) {
	for _, tc := range []struct {
		active string
		want   string
	}{
		{active: "active", want: "running"},
		{active: "activating", want: "activating"},
		{active: "inactive", want: "inactive"},
		{active: "failed", want: "failed"},
		{active: "deactivating", want: "deactivating"},
	} {
		t.Run(tc.active, func(t *testing.T) {
			origBinary := DefaultBinaryPath
			defer func() {
				DefaultBinaryPath = origBinary
			}()
			DefaultBinaryPath = filepath.Join(t.TempDir(), "sing-box")
			if err := os.WriteFile(DefaultBinaryPath, []byte("#!/bin/sh\n"), 0755); err != nil {
				t.Fatalf("write fake binary: %v", err)
			}
			setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				call := commandCall(name, args...)
				out := "loaded\n"
				if strings.Contains(call, "is-active") {
					out = tc.active + "\n"
				}
				return []byte(out), nil
			}})
			if got := Status(); got != tc.want {
				t.Fatalf("Status()=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckBinaryRequiresExecutableRegularFile(t *testing.T) {
	dir := t.TempDir()
	origBinary := DefaultBinaryPath
	defer func() { DefaultBinaryPath = origBinary }()

	DefaultBinaryPath = filepath.Join(dir, "missing")
	if got := CheckBinary(); got.OK() || got.Code != "not_exists" || !strings.Contains(got.Error(), DefaultBinaryPath) {
		t.Fatalf("missing binary check = %+v error=%q", got, got.Error())
	}

	DefaultBinaryPath = dir
	if got := CheckBinary(); got.OK() || got.Code != "not_file" {
		t.Fatalf("directory binary check = %+v", got)
	}

	DefaultBinaryPath = filepath.Join(dir, "sing-box")
	if err := os.WriteFile(DefaultBinaryPath, []byte("#!/bin/sh\n"), 0644); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	if got := CheckBinary(); got.OK() || got.Code != "execute_permission_denied" {
		t.Fatalf("non-executable binary check = %+v", got)
	}
	if err := os.Chmod(DefaultBinaryPath, 0755); err != nil {
		t.Fatalf("chmod fake binary: %v", err)
	}
	if got := CheckBinary(); !got.OK() || !got.Executable {
		t.Fatalf("executable binary check = %+v", got)
	}
}

func TestCheckConfigPathReportsReadableFileAndBinaryPath(t *testing.T) {
	setTempSingboxBinary(t, 0755)
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	setCommandRunner(t, fakeCommandRunner{})
	if err := CheckConfigPath(configPath); err != nil {
		t.Fatalf("check config path: %v", err)
	}
	if err := CheckConfigPath(configDir); err == nil || !strings.Contains(err.Error(), "not_file") || !strings.Contains(err.Error(), configDir) {
		t.Fatalf("expected directory path error with path, got %v", err)
	}
}

func TestApplyConfigUsesFinalConfigParentForTempFiles(t *testing.T) {
	origDir := DefaultConfigDir
	origPath := DefaultConfigPath
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
	}()
	setTempSingboxBinary(t, 0755)

	base := t.TempDir()
	DefaultConfigDir = filepath.Join(base, "default-dir")
	DefaultConfigPath = filepath.Join(base, "final-dir", "config.json")
	setCommandRunner(t, fakeCommandRunner{})
	if err := ApplyConfig([]byte(`{"outbounds":[{"type":"direct","tag":"new"}]}`)); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	if _, err := os.Stat(DefaultConfigPath); err != nil {
		t.Fatalf("expected config at final path %s: %v", DefaultConfigPath, err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(DefaultConfigPath), ".config-*.json"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files should be cleaned from final config dir: %+v", matches)
	}
}

func TestManagementDoesNotTreatSystemctlErrorsAsManaged(t *testing.T) {
	origBinary := DefaultBinaryPath
	defer func() {
		DefaultBinaryPath = origBinary
	}()
	DefaultBinaryPath = filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(DefaultBinaryPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Failed to connect to bus: permission denied\n"), errors.New("restart failed")
	}})

	got := Management()
	if got.Managed {
		t.Fatalf("systemctl errors must not be treated as managed: %+v", got)
	}
	if status := Status(); status != "not_managed" {
		t.Fatalf("expected not_managed when systemctl fails, got %q", status)
	}
}

func TestRestartServiceUsesStandardService(t *testing.T) {
	var calls []string
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, commandCall(name, args...))
		return []byte("loaded\n"), nil
	}})

	if _, err := RestartService(); err != nil {
		t.Fatalf("restart service: %v", err)
	}
	want := []string{"systemctl restart migate-sing-box"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestApplyConfigDoesNotOverwriteExistingConfigWhenCheckFails(t *testing.T) {
	origDir := DefaultConfigDir
	origPath := DefaultConfigPath
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
	}()
	setTempSingboxBinary(t, 0755)

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	oldConfig := []byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"old"}]}`)
	if err := os.WriteFile(DefaultConfigPath, oldConfig, 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("outbounds[3]: dns outbound is deprecated\n"), errors.New("check failed")
	}})

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
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
	}()
	setTempSingboxBinary(t, 0755)

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	oldConfig := []byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"old"}]}`)
	if err := os.WriteFile(DefaultConfigPath, oldConfig, 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "systemctl" && len(args) > 0 && args[0] == "restart" {
			return []byte("restart failed\n"), errors.New("restart failed")
		}
		return nil, nil
	}})

	err := ApplyConfig([]byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"new"}]}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl restart migate-sing-box failed") {
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
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
	}()
	setTempSingboxBinary(t, 0755)

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
	setCommandRunner(t, fakeCommandRunner{})

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
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
	}()
	setTempSingboxBinary(t, 0755)

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	newConfig := []byte(`{"log":{"level":"warn"},"outbounds":[{"type":"direct","tag":"new"}]}`)
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "systemctl" && len(args) > 0 && args[0] == "restart" {
			return []byte("restart failed\n"), errors.New("restart failed")
		}
		return nil, nil
	}})

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
	defer func() {
		DefaultConfigDir = origDir
		DefaultConfigPath = origPath
	}()
	setTempSingboxBinary(t, 0755)

	dir := t.TempDir()
	DefaultConfigDir = dir
	DefaultConfigPath = filepath.Join(dir, "config.json")
	if err := os.WriteFile(DefaultConfigPath, []byte(`{"outbounds":[{"type":"direct","tag":"old"}]}`), 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	var restarts int32
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "systemctl" && len(args) > 0 && args[0] == "restart" {
			if atomic.AddInt32(&restarts, 1) == 1 {
				return []byte("restart failed\n"), errors.New("restart failed")
			}
		}
		return nil, nil
	}})

	err := ApplyConfig([]byte(`{"outbounds":[{"type":"direct","tag":"new"}]}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl restart migate-sing-box failed") {
		t.Fatalf("expected first restart error, got %v", err)
	}
	if got := atomic.LoadInt32(&restarts); got != 2 {
		t.Fatalf("expected restart of new then restored config, got %d restarts", got)
	}
}

func TestListeningUDPPortsParsesSSOutput(t *testing.T) {
	setCommandRunner(t, fakeCommandRunner{runOutput: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("UNCONN 0 0 0.0.0.0:20001 0.0.0.0:*\nUNCONN 0 0 [::]:20002 [::]:*\n"), nil
	}})
	got := ListeningUDPPorts([]int{20001, 20002, 20003})
	if !got[20001] || !got[20002] || got[20003] {
		t.Fatalf("unexpected UDP listening map: %+v", got)
	}
}
