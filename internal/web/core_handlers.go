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
			commands = []string{"download Xray release", "verify Xray release checksum", "systemctl stop xray", "atomic install /usr/local/bin/xray", "write /etc/systemd/system/xray.service", "systemctl restart xray"}
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
install_tmp=""
trap 'rm -rf "$tmp"; [ -z "${install_tmp:-}" ] || rm -f "$install_tmp"' EXIT
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
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl stop xray 2>/dev/null || true
fi
install_tmp="/usr/local/bin/.xray.new.$$"
rm -f "$install_tmp"
cp "$tmp/xray/xray" "$install_tmp"
chmod +x "$install_tmp"
mv -f "$install_tmp" /usr/local/bin/xray
install_tmp=""
mkdir -p /usr/local/share/xray /usr/local/migate /usr/local/etc/xray
[ -f "$tmp/xray/geosite.dat" ] && cp "$tmp/xray/geosite.dat" /usr/local/share/xray/geosite.dat
[ -f "$tmp/xray/geoip.dat" ] && cp "$tmp/xray/geoip.dat" /usr/local/share/xray/geoip.dat
write_migate_default_xray_config() {
  cat > /usr/local/migate/xray.json <<'JSON'
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "tag": "api",
      "listen": "127.0.0.1",
      "port": 10085,
      "protocol": "dokodemo-door",
      "settings": {
        "address": "127.0.0.1"
      }
    }
  ],
  "outbounds": [
    {
      "tag": "xray-out-1",
      "protocol": "freedom",
      "settings": {}
    },
    {
      "tag": "xray-out-2",
      "protocol": "blackhole",
      "settings": {}
    },
    {
      "tag": "xray-out-3",
      "protocol": "dns",
      "settings": {}
    }
  ],
  "routing": {
    "domainStrategy": "AsIs",
    "rules": [
      {
        "inboundTag": [
          "api"
        ],
        "outboundTag": "api"
      }
    ]
  },
  "stats": {},
  "policy": {
    "levels": {
      "0": {
        "statsUserUplink": true,
        "statsUserDownlink": true
      }
    }
  },
  "api": {
    "tag": "api",
    "services": [
      "StatsService"
    ]
  }
}
JSON
}
backup_migate_invalid_core_config() {
  path="$1"
  if [ ! -e "$path" ]; then
    return 0
  fi
  backup="${path}.migate-backup.$(date +%Y%m%d%H%M%S)"
  mv -f "$path" "$backup"
  echo "backed up invalid config: $backup" >&2
}
if [ ! -f /usr/local/migate/xray.json ]; then
  write_migate_default_xray_config
fi
mkdir -p /etc/migate
if [ -e /etc/migate/xray.json ] && [ ! -L /etc/migate/xray.json ]; then
  echo "/etc/migate/xray.json exists and is not a symlink; keeping it unchanged" >&2
  echo "Move it aside or replace it with a symlink to /usr/local/migate/xray.json, then rerun install." >&2
  exit 1
fi
ln -sf /usr/local/migate/xray.json /etc/migate/xray.json
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/xray.json
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/config.json
if ! /usr/local/bin/xray run -test -c /usr/local/etc/xray/config.json; then
  echo "existing Xray config check failed; backing it up and writing MiGate default config" >&2
  backup_migate_invalid_core_config /usr/local/migate/xray.json
  write_migate_default_xray_config
  /usr/local/bin/xray run -test -c /usr/local/etc/xray/config.json
fi
if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "systemd is unavailable; skipped xray.service"
  /usr/local/bin/xray version | sed -n '1p'
  exit 0
fi
rm -f /etc/systemd/system/xray.service.d/10-donot_touch_single_conf.conf
rmdir /etc/systemd/system/xray.service.d 2>/dev/null || true
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
systemctl restart xray
systemctl is-active --quiet xray
/usr/local/bin/xray version | sed -n '1p'`
		case "singbox":
			commands = []string{"download sing-box release", "verify sing-box release checksum", "systemctl stop sing-box and migate-singbox", "atomic install /usr/local/bin/sing-box", "write /etc/systemd/system/sing-box.service", "systemctl restart sing-box"}
			script = `set -euo pipefail
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch=amd64 ;;
  aarch64|arm64) asset_arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
version="${SINGBOX_VERSION:-1.13.13}"
tmp="$(mktemp -d)"
install_tmp=""
trap 'rm -rf "$tmp"; [ -z "${install_tmp:-}" ] || rm -f "$install_tmp"' EXIT
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
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl stop sing-box 2>/dev/null || true
  systemctl stop migate-singbox 2>/dev/null || true
fi
install_tmp="/usr/local/bin/.sing-box.new.$$"
rm -f "$install_tmp"
cp "$tmp"/sing-box-*/sing-box "$install_tmp"
chmod +x "$install_tmp"
mv -f "$install_tmp" /usr/local/bin/sing-box
install_tmp=""
mkdir -p /etc/sing-box
write_migate_default_singbox_config() {
  cat > /etc/sing-box/config.json <<'JSON'
{
  "log": {
    "level": "warn"
  },
  "inbounds": [],
  "outbounds": [
    {
      "type": "direct",
      "tag": "singbox-out-1"
    },
    {
      "type": "block",
      "tag": "singbox-out-2"
    }
  ]
}
JSON
}
backup_migate_invalid_core_config() {
  path="$1"
  if [ ! -e "$path" ]; then
    return 0
  fi
  backup="${path}.migate-backup.$(date +%Y%m%d%H%M%S)"
  mv -f "$path" "$backup"
  echo "backed up invalid config: $backup" >&2
}
if [ ! -f /etc/sing-box/config.json ]; then
  write_migate_default_singbox_config
fi
if ! /usr/local/bin/sing-box check -c /etc/sing-box/config.json; then
  echo "existing sing-box config check failed; backing it up and writing MiGate default config" >&2
  backup_migate_invalid_core_config /etc/sing-box/config.json
  write_migate_default_singbox_config
  /usr/local/bin/sing-box check -c /etc/sing-box/config.json
fi
if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "systemd is unavailable; skipped sing-box.service"
  /usr/local/bin/sing-box version | sed -n '1p'
  exit 0
fi
cat > /etc/systemd/system/sing-box.service <<'UNIT'
[Unit]
Description=sing-box service managed by MiGate
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
systemctl stop migate-singbox 2>/dev/null || true
systemctl disable migate-singbox 2>/dev/null || true
rm -f /etc/systemd/system/migate-singbox.service
rm -rf /etc/systemd/system/migate-singbox.service.d
systemctl daemon-reload
systemctl reset-failed migate-singbox 2>/dev/null || true
systemctl enable sing-box
systemctl restart sing-box
systemctl is-active --quiet sing-box
/usr/local/bin/sing-box version | sed -n '1p'`
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
		service := ""
		switch core {
		case "xray":
			service = "xray"
		case "singbox":
			service = "sing-box"
		default:
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		if action != "restart" && action != "stop" {
			writeJSONError(w, http.StatusBadRequest, "unknown_action")
			return
		}
		commands := []string{fmt.Sprintf("systemctl %s %s", action, service)}
		script := fmt.Sprintf(`set -euo pipefail
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
`, action, service, service, action, service, service, action, action, service, service, service, action, service)
		out, err := runner(script)
		status := action + "ed"
		if action == "stop" {
			status = "stopped"
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"core": core, "status": "failed", "error": action + "_failed", "output": string(out), "commands_executed": commands})
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
			commands = []string{"systemctl stop xray", "systemctl disable xray", "remove MiGate xray service and symlinks", "systemctl daemon-reload"}
			script = `set -euo pipefail
systemctl stop xray 2>/dev/null || true
systemctl disable xray 2>/dev/null || true
rm -f /etc/systemd/system/xray.service
rm -f /etc/systemd/system/xray.service.d/10-donot_touch_single_conf.conf
rmdir /etc/systemd/system/xray.service.d 2>/dev/null || true
rm -f /usr/local/etc/xray/xray.json /usr/local/etc/xray/config.json
if [ -L /etc/migate/xray.json ]; then rm -f /etc/migate/xray.json; fi
systemctl daemon-reload 2>/dev/null || true
systemctl reset-failed xray 2>/dev/null || true
printf 'Xray service disabled and MiGate systemd unit/symlinks removed. Remove the Xray binary/package manually if it was installed outside MiGate.\n'`
		case "singbox":
			commands = []string{"systemctl stop sing-box", "systemctl disable sing-box", "remove sing-box binary and service", "systemctl daemon-reload"}
			script = `set -euo pipefail
systemctl stop sing-box 2>/dev/null || true
systemctl disable sing-box 2>/dev/null || true
systemctl stop migate-singbox 2>/dev/null || true
systemctl disable migate-singbox 2>/dev/null || true
rm -f /etc/systemd/system/sing-box.service /etc/systemd/system/migate-singbox.service /usr/local/bin/sing-box
rm -rf /etc/systemd/system/migate-singbox.service.d
systemctl daemon-reload 2>/dev/null || true
systemctl reset-failed sing-box 2>/dev/null || true
systemctl reset-failed migate-singbox 2>/dev/null || true
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
