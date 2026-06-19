package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/imzyb/MiGate/internal/paths"
	runtimecmd "github.com/imzyb/MiGate/internal/runtime/command"
	runtimescript "github.com/imzyb/MiGate/internal/runtime/script"
	"github.com/imzyb/MiGate/internal/service/coreadmin"
)

const maxXrayLogLines = 200

type coreActionPayload struct {
	Confirm            bool `json:"confirm"`
	AllowSystemChanges bool `json:"allow_system_changes"`
}

func decodeCoreActionPayload(w http.ResponseWriter, r *http.Request) (coreActionPayload, bool) {
	var payload coreActionPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return payload, false
	}
	if !payload.Confirm || !payload.AllowSystemChanges {
		writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
		return payload, false
	}
	return payload, true
}

func coreInstallHandler(core string, runner func(script string) ([]byte, error)) http.HandlerFunc {
	if runner == nil {
		runner = runCoreScript
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		result, err := (coreadmin.Service{Runner: runner}).Install(core)
		if err == coreadmin.ErrUnknownCore {
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		if err != nil {
			writeJSON(w, http.StatusOK, coreActionResultPayload(result))
			return
		}
		writeJSON(w, http.StatusOK, coreActionResultPayload(result))
	}
}

func coreServiceControlHandler(core, action string, runner func(script string) ([]byte, error)) http.HandlerFunc {
	if runner == nil {
		runner = runCoreScript
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		result, err := (coreadmin.Service{Runner: runner}).Control(core, action)
		if err == coreadmin.ErrUnknownCore {
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		if err == coreadmin.ErrUnknownAction {
			writeJSONError(w, http.StatusBadRequest, "unknown_action")
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, coreActionResultPayload(result))
			return
		}
		writeJSON(w, http.StatusOK, coreActionResultPayload(result))
	}
}

func runCoreScript(script string) ([]byte, error) {
	return (runtimescript.Runner{Timeout: 5 * time.Minute}).RunBash(context.Background(), script)
}

func coreUninstallHandler(core string, runner func(script string) ([]byte, error)) http.HandlerFunc {
	if runner == nil {
		runner = runCoreScript
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		result, err := (coreadmin.Service{Runner: runner}).Uninstall(core)
		if err == coreadmin.ErrUnknownCore {
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, coreActionResultPayload(result))
			return
		}
		writeJSON(w, http.StatusOK, coreActionResultPayload(result))
	}
}

func coreActionResultPayload(result coreadmin.ActionResult) map[string]interface{} {
	payload := map[string]interface{}{
		"core":              result.Core,
		"status":            result.Status,
		"output":            result.Output,
		"commands_executed": result.CommandsExecuted,
	}
	if result.Error != "" {
		payload["error"] = result.Error
	}
	return payload
}

func xrayLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		lines := r.URL.Query().Get("lines")
		if lines == "" {
			lines = "50"
		}
		if n, err := strconv.Atoi(lines); err != nil || n < 1 {
			lines = "50"
		} else if n > maxXrayLogLines {
			lines = strconv.Itoa(maxXrayLogLines)
		}
		out, err := runtimecmd.RunOutput(context.Background(), "journalctl", "-u", paths.XrayService, "-n", lines, "--no-pager", "-o", "short-iso")
		if err != nil {
			// Fallback: try reading from syslog
			out, err = runtimecmd.RunOutput(context.Background(), "tail", "-n", lines, "/var/log/syslog")
			if err != nil {
				writeJSON(w, http.StatusOK, map[string]string{"logs": "无法读取 Xray 日志：journalctl 和 syslog 均不可用。"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"logs": string(out)})
	}
}

func xrayVersionHandler(controller XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		ver := controller.Version(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"version": ver})
	}
}
