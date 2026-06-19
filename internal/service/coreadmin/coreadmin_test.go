package coreadmin

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestControlPlanBuildsServiceCommandAndScript(t *testing.T) {
	plan, err := ControlPlan("xray", "restart")
	if err != nil {
		t.Fatalf("control plan: %v", err)
	}
	if plan.Core != "xray" {
		t.Fatalf("core = %q", plan.Core)
	}
	if !reflect.DeepEqual(plan.Commands, []string{"systemctl restart migate-xray"}) {
		t.Fatalf("commands = %+v", plan.Commands)
	}
	for _, want := range []string{"systemd is unavailable; cannot restart migate-xray.service", "systemctl restart migate-xray"} {
		if !strings.Contains(plan.Script, want) {
			t.Fatalf("script missing %q:\n%s", want, plan.Script)
		}
	}
}

func TestControlPlanRejectsUnknownCoreAndAction(t *testing.T) {
	if _, err := ControlPlan("bad", "restart"); !errors.Is(err, ErrUnknownCore) {
		t.Fatalf("expected ErrUnknownCore, got %v", err)
	}
	if _, err := ControlPlan("xray", "reload"); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", err)
	}
}

func TestInstallPlanBuildsXrayScript(t *testing.T) {
	plan, err := InstallPlan("xray")
	if err != nil {
		t.Fatalf("install plan: %v", err)
	}
	if plan.Core != "xray" {
		t.Fatalf("core = %q", plan.Core)
	}
	for _, want := range []string{
		"download Xray release",
		"verify Xray release checksum",
		"Xray-linux-${asset_arch}.zip",
		"sha256sum -c",
		"/etc/migate/cores/xray.json",
		"/var/lib/migate/backups/xray-config-invalid-",
		"migate-xray.service",
		"/usr/local/bin/xray run -test -c",
	} {
		if !strings.Contains(strings.Join(append([]string{plan.Script}, plan.Commands...), "\n"), want) {
			t.Fatalf("xray install plan missing %q", want)
		}
	}
}

func TestInstallPlanBuildsSingboxScript(t *testing.T) {
	plan, err := InstallPlan("singbox")
	if err != nil {
		t.Fatalf("install plan: %v", err)
	}
	if plan.Core != "singbox" {
		t.Fatalf("core = %q", plan.Core)
	}
	for _, want := range []string{
		"download sing-box release",
		"verify sing-box release checksum",
		"sing-box-${version}-linux",
		"digest",
		"sha256sum -c",
		"/etc/migate/cores/sing-box.json",
		"/var/lib/migate/backups/sing-box-config-invalid-",
		"migate-sing-box.service",
		"/usr/local/bin/sing-box check -c",
	} {
		if !strings.Contains(strings.Join(append([]string{plan.Script}, plan.Commands...), "\n"), want) {
			t.Fatalf("sing-box install plan missing %q", want)
		}
	}
}

func TestInstallPlanRejectsUnknownCore(t *testing.T) {
	if _, err := InstallPlan("bad"); !errors.Is(err, ErrUnknownCore) {
		t.Fatalf("expected ErrUnknownCore, got %v", err)
	}
}

func TestServiceControlWrapsRunnerResult(t *testing.T) {
	var scriptSeen string
	result, err := (Service{Runner: func(script string) ([]byte, error) {
		scriptSeen = script
		return []byte("ok"), nil
	}}).Control("singbox", "stop")
	if err != nil {
		t.Fatalf("control: %v", err)
	}
	if scriptSeen == "" {
		t.Fatal("runner was not called")
	}
	if result.Core != "singbox" || result.Status != "stopped" || result.Output != "ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(result.CommandsExecuted, []string{"systemctl stop migate-sing-box"}) {
		t.Fatalf("commands = %+v", result.CommandsExecuted)
	}
}

func TestServiceInstallWrapsRunnerResult(t *testing.T) {
	var scriptSeen string
	result, err := (Service{Runner: func(script string) ([]byte, error) {
		scriptSeen = script
		return []byte("installed ok"), nil
	}}).Install("xray")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(scriptSeen, "Xray-linux-${asset_arch}.zip") {
		t.Fatalf("runner received unexpected script: %s", scriptSeen)
	}
	if result.Core != "xray" || result.Status != "installed" || result.Output != "installed ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(result.CommandsExecuted, []string{"download Xray release", "verify Xray release checksum", "systemctl stop migate-xray", "atomic install /usr/local/bin/xray", "write /etc/systemd/system/migate-xray.service", "systemctl restart migate-xray"}) {
		t.Fatalf("commands = %+v", result.CommandsExecuted)
	}
}

func TestServiceInstallFailureKeepsCommands(t *testing.T) {
	result, err := (Service{Runner: func(script string) ([]byte, error) {
		return []byte("download failed"), errors.New("boom")
	}}).Install("singbox")
	if err == nil {
		t.Fatal("expected runner error")
	}
	if result.Status != "failed" || result.Error != "install_failed" || result.Output != "download failed" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(result.CommandsExecuted, []string{"download sing-box release", "verify sing-box release checksum", "systemctl stop migate-sing-box", "atomic install /usr/local/bin/sing-box", "write /etc/systemd/system/migate-sing-box.service", "systemctl restart migate-sing-box"}) {
		t.Fatalf("commands = %+v", result.CommandsExecuted)
	}
}

func TestServiceControlFailureKeepsCommands(t *testing.T) {
	result, err := (Service{Runner: func(script string) ([]byte, error) {
		return []byte("restart failed"), errors.New("boom")
	}}).Control("xray", "restart")
	if err == nil {
		t.Fatal("expected runner error")
	}
	if result.Status != "failed" || result.Error != "restart_failed" || result.Output != "restart failed" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(result.CommandsExecuted, []string{"systemctl restart migate-xray"}) {
		t.Fatalf("commands = %+v", result.CommandsExecuted)
	}
}

func TestUninstallPlanBuildsScript(t *testing.T) {
	plan, err := UninstallPlan("singbox")
	if err != nil {
		t.Fatalf("uninstall plan: %v", err)
	}
	for _, want := range []string{"systemctl stop migate-sing-box", "rm -f /etc/systemd/system/migate-sing-box.service", "sing-box service disabled"} {
		if !strings.Contains(plan.Script, want) {
			t.Fatalf("script missing %q:\n%s", want, plan.Script)
		}
	}
	if len(plan.Commands) != 4 {
		t.Fatalf("commands = %+v", plan.Commands)
	}
}
