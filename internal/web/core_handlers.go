package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
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

func coreInstallHandler(core string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		var script string
		var commands []string
		switch core {
		case "xray":
			commands = []string{"download Xray-install script", "run installed script", "mkdir -p /usr/local/etc/xray", "ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/xray.json", "systemctl enable --now xray"}
			script = `set -euo pipefail
if ! command -v curl >/dev/null 2>&1; then echo 'curl is required' >&2; exit 1; fi
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fL "https://github.com/XTLS/Xray-install/raw/main/install-release.sh" -o "$tmp/install-release.sh"
bash "$tmp/install-release.sh"
mkdir -p /usr/local/etc/xray
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/xray.json
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/config.json
systemctl enable xray
systemctl restart xray || true
xray --version | head -1`
		case "singbox":
			commands = []string{"download sing-box release", "install /usr/local/bin/sing-box", "write /etc/systemd/system/migate-singbox.service", "systemctl enable --now migate-singbox"}
			script = `set -euo pipefail
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch=amd64 ;;
  aarch64|arm64) asset_arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
version="${SINGBOX_VERSION:-1.13.13}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
url="https://github.com/SagerNet/sing-box/releases/download/v${version}/sing-box-${version}-linux-${asset_arch}.tar.gz"
checksums_url="https://github.com/SagerNet/sing-box/releases/download/v${version}/sing-box-${version}-checksums.txt"
curl -fL "$url" -o "$tmp/sing-box.tar.gz"
curl -fL "$checksums_url" -o "$tmp/checksums.txt"
grep "sing-box-${version}-linux-${asset_arch}.tar.gz" "$tmp/checksums.txt" > "$tmp/sing-box.tar.gz.sha256"
(cd "$tmp" && sha256sum -c "sing-box.tar.gz.sha256")
tar -xzf "$tmp/sing-box.tar.gz" -C "$tmp"
cp "$tmp"/sing-box-*/sing-box /usr/local/bin/sing-box
chmod +x /usr/local/bin/sing-box
mkdir -p /etc/sing-box
if [ ! -f /etc/sing-box/config.json ]; then
  printf '%s\n' '{"log":{"level":"warn"},"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}]}' > /etc/sing-box/config.json
fi
cat > /etc/systemd/system/migate-singbox.service <<'UNIT'
[Unit]
Description=MiGate managed sing-box service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sing-box run -c /etc/sing-box/config.json
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable migate-singbox
systemctl restart migate-singbox || true
sing-box version | head -1`
		default:
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		out, err := runCoreScript(script)
		status := "installed"
		if err != nil {
			status = "failed"
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"core": core, "status": status, "output": string(out), "commands_executed": commands})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"core": core, "status": status, "output": string(out), "commands_executed": commands})
	}
}

func runCoreScript(script string) ([]byte, error) {
	cmd := exec.Command("bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	return cmd.CombinedOutput()
}

func coreUninstallHandler(core string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		var script string
		var commands []string
		switch core {
		case "xray":
			commands = []string{"systemctl disable --now xray", "bash Xray-install remove", "remove MiGate xray symlinks"}
			script = `set -euo pipefail
systemctl disable --now xray 2>/dev/null || true
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" -- remove --purge 2>&1 || true
rm -f /usr/local/etc/xray/xray.json /usr/local/etc/xray/config.json
printf 'Xray removed or disabled\n'`
		case "singbox":
			commands = []string{"systemctl disable --now migate-singbox", "remove sing-box binary and service"}
			script = `set -euo pipefail
systemctl disable --now migate-singbox 2>/dev/null || true
rm -f /etc/systemd/system/migate-singbox.service /usr/local/bin/sing-box
systemctl daemon-reload 2>/dev/null || true
printf 'sing-box removed\n'`
		default:
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		out, err := runCoreScript(script)
		status := "uninstalled"
		if err != nil {
			status = "failed"
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"core": core, "status": status, "output": string(out), "commands_executed": commands})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"core": core, "status": status, "output": string(out), "commands_executed": commands})
	}
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
		out, err := exec.Command("journalctl", "-u", "xray", "-n", lines, "--no-pager", "-o", "short-iso").CombinedOutput()
		if err != nil {
			// Fallback: try reading from syslog
			out, err = exec.Command("tail", "-n", lines, "/var/log/syslog").CombinedOutput()
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"logs": "无法读取 Xray 日志：journalctl 和 syslog 均不可用。"})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"logs": string(out)})
	}
}

func xrayVersionHandler(controller XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		ver := controller.Version(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": ver})
	}
}
