package coreadmin

import (
	"errors"
	"fmt"

	"github.com/imzyb/MiGate/internal/paths"
)

var (
	ErrUnknownCore   = errors.New("unknown core")
	ErrUnknownAction = errors.New("unknown action")
)

type ScriptRunner func(script string) ([]byte, error)

type Service struct {
	Runner ScriptRunner
}

type Plan struct {
	Core     string
	Script   string
	Commands []string
}

type ActionResult struct {
	Core             string
	Status           string
	Error            string
	Output           string
	CommandsExecuted []string
}

func (s Service) Control(core, action string) (ActionResult, error) {
	plan, err := ControlPlan(core, action)
	if err != nil {
		return ActionResult{}, err
	}
	out, runErr := s.run(plan.Script)
	status := action + "ed"
	if action == "stop" {
		status = "stopped"
	}
	result := ActionResult{
		Core:             plan.Core,
		Status:           status,
		Output:           string(out),
		CommandsExecuted: plan.Commands,
	}
	if runErr != nil {
		result.Status = "failed"
		result.Error = action + "_failed"
		return result, runErr
	}
	return result, nil
}

func (s Service) Install(core string) (ActionResult, error) {
	plan, err := InstallPlan(core)
	if err != nil {
		return ActionResult{}, err
	}
	return s.runPlan(plan, "installed", "install_failed")
}

func (s Service) Uninstall(core string) (ActionResult, error) {
	plan, err := UninstallPlan(core)
	if err != nil {
		return ActionResult{}, err
	}
	return s.runPlan(plan, "unmanaged", "unmanage_failed")
}

func (s Service) Delete(core string) (ActionResult, error) {
	plan, err := DeletePlan(core)
	if err != nil {
		return ActionResult{}, err
	}
	return s.runPlan(plan, "deleted", "delete_failed")
}

func (s Service) runPlan(plan Plan, successStatus, failureError string) (ActionResult, error) {
	out, runErr := s.run(plan.Script)
	result := ActionResult{
		Core:             plan.Core,
		Status:           successStatus,
		Output:           string(out),
		CommandsExecuted: plan.Commands,
	}
	if runErr != nil {
		result.Status = "failed"
		result.Error = failureError
		return result, runErr
	}
	return result, nil
}

func (s Service) run(script string) ([]byte, error) {
	if s.Runner == nil {
		return nil, errors.New("coreadmin runner is nil")
	}
	return s.Runner(script)
}

func ServiceName(core string) (string, error) {
	switch core {
	case "xray":
		return paths.XrayService, nil
	case "singbox":
		return paths.SingboxService, nil
	default:
		return "", ErrUnknownCore
	}
}

func ControlPlan(core, action string) (Plan, error) {
	service, err := ServiceName(core)
	if err != nil {
		return Plan{}, err
	}
	if action != "restart" && action != "stop" {
		return Plan{}, ErrUnknownAction
	}
	return Plan{
		Core:     core,
		Commands: []string{fmt.Sprintf("systemctl %s %s", action, service)},
		Script: fmt.Sprintf(`set -euo pipefail
if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "systemd is unavailable; cannot %s %s.service" >&2
  exit 1
fi
load_state="$(systemctl show %s --property=LoadState --value 2>/dev/null || true)"
if [ "$load_state" != "loaded" ]; then
  if [ "%s" = "stop" ]; then
    echo "%s.service is not loaded; already stopped"
    exit 0
  fi
  echo "%s.service is not loaded; cannot %s" >&2
  exit 1
fi
if [ "%s" = "stop" ]; then
  active_state="$(systemctl is-active %s 2>/dev/null || true)"
  if [ "$active_state" = "inactive" ] || [ "$active_state" = "failed" ] || [ "$active_state" = "unknown" ] || [ "$active_state" = "" ]; then
    systemctl reset-failed %s 2>/dev/null || true
    echo "%s.service is already $active_state"
    exit 0
  fi
fi
systemctl %s %s
`, action, service, service, action, service, service, action, action, service, service, service, action, service),
	}, nil
}

func UninstallPlan(core string) (Plan, error) {
	switch core {
	case "xray":
		return Plan{
			Core:     core,
			Commands: []string{"systemctl stop migate-xray", "systemctl disable migate-xray", "remove MiGate Xray service", "systemctl daemon-reload"},
			Script: `set -euo pipefail
systemctl stop migate-xray 2>/dev/null || true
systemctl disable migate-xray 2>/dev/null || true
rm -f /etc/systemd/system/migate-xray.service
systemctl daemon-reload 2>/dev/null || true
systemctl reset-failed migate-xray 2>/dev/null || true
printf 'Xray service disabled. Configuration and binary were kept.\n'`,
		}, nil
	case "singbox":
		return Plan{
			Core:     core,
			Commands: []string{"systemctl stop migate-sing-box", "systemctl disable migate-sing-box", "remove MiGate sing-box service", "systemctl daemon-reload"},
			Script: `set -euo pipefail
systemctl stop migate-sing-box 2>/dev/null || true
systemctl disable migate-sing-box 2>/dev/null || true
rm -f /etc/systemd/system/migate-sing-box.service
systemctl daemon-reload 2>/dev/null || true
systemctl reset-failed migate-sing-box 2>/dev/null || true
printf 'sing-box service disabled. Configuration and binary were kept.\n'`,
		}, nil
	default:
		return Plan{}, ErrUnknownCore
	}
}

func DeletePlan(core string) (Plan, error) {
	switch core {
	case "xray":
		return Plan{
			Core:     core,
			Commands: []string{"systemctl stop migate-xray", "systemctl disable migate-xray", "remove MiGate Xray service", "remove /usr/local/bin/xray", "systemctl daemon-reload"},
			Script: `set -euo pipefail
systemctl stop migate-xray 2>/dev/null || true
systemctl disable migate-xray 2>/dev/null || true
rm -f /etc/systemd/system/migate-xray.service
rm -f /usr/local/bin/xray
systemctl daemon-reload 2>/dev/null || true
systemctl reset-failed migate-xray 2>/dev/null || true
printf 'Xray core binary removed. Configuration was kept.\n'`,
		}, nil
	case "singbox":
		return Plan{
			Core:     core,
			Commands: []string{"systemctl stop migate-sing-box", "systemctl disable migate-sing-box", "remove MiGate sing-box service", "remove /usr/local/bin/sing-box", "systemctl daemon-reload"},
			Script: `set -euo pipefail
systemctl stop migate-sing-box 2>/dev/null || true
systemctl disable migate-sing-box 2>/dev/null || true
rm -f /etc/systemd/system/migate-sing-box.service
rm -f /usr/local/bin/sing-box
systemctl daemon-reload 2>/dev/null || true
systemctl reset-failed migate-sing-box 2>/dev/null || true
printf 'sing-box core binary removed. Configuration was kept.\n'`,
		}, nil
	default:
		return Plan{}, ErrUnknownCore
	}
}
