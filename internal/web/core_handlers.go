package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
		var script string
		var commands []string
		switch core {
		case "xray":
			commands = []string{"download Xray release", "verify Xray release checksum", "install /usr/local/bin/xray", "write /etc/systemd/system/xray.service", "systemctl enable --now xray"}
			script = `set -euo pipefail
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch=64 ;;
  aarch64|arm64) asset_arch=arm64-v8a ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
for dep in curl unzip; do
  if ! command -v "$dep" >/dev/null 2>&1; then
    echo "$dep is required to install Xray" >&2
    exit 1
  fi
done
version="${XRAY_VERSION:-26.3.27}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
asset_name="Xray-linux-${asset_arch}.zip"
url="https://github.com/XTLS/Xray-core/releases/download/v${version}/${asset_name}"
dgst_url="${url}.dgst"
curl -fL "$url" -o "$tmp/$asset_name"
curl -fL "$dgst_url" -o "$tmp/$asset_name.dgst"
awk -F'= ' -v asset="$asset_name" '/^SHA2-256=/{print $2 "  " asset}' "$tmp/$asset_name.dgst" > "$tmp/$asset_name.sha256"
if ! grep -Eq '^[0-9a-fA-F]{64}[[:space:]]+' "$tmp/$asset_name.sha256"; then
  echo "invalid Xray checksum file" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$tmp" && sha256sum -c "$asset_name.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$tmp" && shasum -a 256 -c "$asset_name.sha256")
else
  echo "sha256sum or shasum is required" >&2; exit 1
fi
unzip -oq "$tmp/$asset_name" -d "$tmp/xray"
cp "$tmp/xray/xray" /usr/local/bin/xray
chmod +x /usr/local/bin/xray
mkdir -p /usr/local/share/xray /usr/local/migate /usr/local/etc/xray
[ -f "$tmp/xray/geosite.dat" ] && cp "$tmp/xray/geosite.dat" /usr/local/share/xray/geosite.dat
[ -f "$tmp/xray/geoip.dat" ] && cp "$tmp/xray/geoip.dat" /usr/local/share/xray/geoip.dat
if [ ! -f /usr/local/migate/xray.json ]; then
  printf '%s\n' '{"log":{"loglevel":"warning"},"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct"},{"protocol":"blackhole","tag":"blocked"}]}' > /usr/local/migate/xray.json
fi
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/xray.json
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/config.json
if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "systemd is unavailable; skipped xray.service"
  /usr/local/bin/xray version | head -1
  exit 0
fi
cat > /etc/systemd/system/xray.service <<'UNIT'
[Unit]
Description=MiGate managed Xray service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/xray run -config /usr/local/etc/xray/config.json
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576
StandardOutput=journal
StandardError=journal
LogRateLimitIntervalSec=30s
LogRateLimitBurst=200

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable xray
if ! systemctl restart xray; then
  echo "Xray installed, but xray.service did not start. Apply config or check journalctl -u xray." >&2
fi
/usr/local/bin/xray version | head -1`
		case "singbox":
			commands = []string{"download sing-box release", "verify sing-box release checksum", "install /usr/local/bin/sing-box", "write /etc/systemd/system/migate-singbox.service", "systemctl enable --now migate-singbox"}
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
asset_name="sing-box-${version}-linux-${asset_arch}.tar.gz"
url="https://github.com/SagerNet/sing-box/releases/download/v${version}/${asset_name}"
release_api_url="https://api.github.com/repos/SagerNet/sing-box/releases/tags/v${version}"
curl -fL "$url" -o "$tmp/$asset_name"
curl -fsSL "$release_api_url" -o "$tmp/release.json"
digest="$(awk -v asset="$asset_name" '
  /"name": "/ { in_asset=0 }
  index($0, "\"name\": \"" asset "\"") { in_asset=1 }
  in_asset && index($0, "\"digest\": \"sha256:") {
    line=$0
    sub(/^.*"digest": "sha256:/, "", line)
    sub(/".*$/, "", line)
    print line
    exit
  }
' "$tmp/release.json")"
if ! printf '%s\n' "$digest" | grep -Eq '^[0-9a-fA-F]{64}$'; then
  echo "invalid sing-box release digest for $asset_name" >&2
  exit 1
fi
printf '%s  %s\n' "$digest" "$asset_name" > "$tmp/sing-box.tar.gz.sha256"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$tmp" && sha256sum -c "sing-box.tar.gz.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$tmp" && shasum -a 256 -c "sing-box.tar.gz.sha256")
else
  echo "sha256sum or shasum is required" >&2; exit 1
fi
tar --no-same-owner -xzf "$tmp/$asset_name" -C "$tmp"
cp "$tmp"/sing-box-*/sing-box /usr/local/bin/sing-box
chmod +x /usr/local/bin/sing-box
mkdir -p /etc/sing-box
if [ ! -f /etc/sing-box/config.json ]; then
  printf '%s\n' '{"log":{"level":"warn"},"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}]}' > /etc/sing-box/config.json
fi
if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "systemd is unavailable; skipped migate-singbox.service"
  /usr/local/bin/sing-box version | head -1
  exit 0
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
if ! systemctl restart migate-singbox; then
  echo "sing-box installed, but migate-singbox.service did not start. Apply config or check journalctl -u migate-singbox." >&2
fi
/usr/local/bin/sing-box version | head -1`
		default:
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		out, err := runner(script)
		status := "installed"
		if err != nil {
			status = "failed"
			writeJSON(w, http.StatusOK, map[string]interface{}{"core": core, "status": status, "error": "install_failed", "output": string(out), "commands_executed": commands})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"core": core, "status": status, "output": string(out), "commands_executed": commands})
	}
}

func runCoreScript(script string) ([]byte, error) {
	if coreSystemdRunAvailable() {
		unit := fmt.Sprintf("migate-core-%d-%d", os.Getpid(), time.Now().UnixNano())
		cmd := exec.Command(
			"systemd-run",
			"--wait",
			"--pipe",
			"--quiet",
			"--unit="+unit,
			"--collect",
			"--property=Type=oneshot",
			"--property=User=root",
			"--property=TimeoutSec=300",
			"bash",
			"-s",
		)
		cmd.Stdin = strings.NewReader(script)
		return cmd.CombinedOutput()
	}
	cmd := exec.Command("bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	return cmd.CombinedOutput()
}

func coreSystemdRunAvailable() bool {
	if _, err := exec.LookPath("systemd-run"); err != nil {
		return false
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false
	}
	return true
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
		var script string
		var commands []string
		switch core {
		case "xray":
			commands = []string{"systemctl disable --now xray", "remove MiGate xray symlinks"}
			script = `set -euo pipefail
systemctl disable --now xray 2>/dev/null || true
rm -f /usr/local/etc/xray/xray.json /usr/local/etc/xray/config.json
printf 'Xray service disabled and MiGate symlinks removed. Remove the Xray binary/package manually if it was installed outside MiGate.\n'`
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
		out, err := runner(script)
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
