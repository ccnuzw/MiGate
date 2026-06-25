package coreadmin

func InstallPlan(core string) (Plan, error) {
	switch core {
	case "xray":
		return Plan{
			Core:     core,
			Commands: []string{"download Xray release", "verify Xray release checksum", "systemctl stop migate-xray", "atomic install /usr/local/bin/xray", "write /etc/systemd/system/migate-xray.service", "systemctl restart migate-xray"},
			Script:   xrayInstallScript,
		}, nil
	case "singbox":
		return Plan{
			Core:     core,
			Commands: []string{"download sing-box release", "verify sing-box release checksum", "systemctl stop migate-sing-box", "atomic install /usr/local/bin/sing-box", "write /etc/systemd/system/migate-sing-box.service", "systemctl restart migate-sing-box"},
			Script:   singboxInstallScript,
		}, nil
	default:
		return Plan{}, ErrUnknownCore
	}
}

const xrayInstallScript = `set -euo pipefail
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
awk -F'= ' '/^SHA2-256=/{print $2}' "$tmp/$asset_name.dgst" | grep -E '^[0-9a-fA-F]{64}$' > "$tmp/$asset_name.digest"
if [ "$(wc -l < "$tmp/$asset_name.digest" | tr -d '[:space:]')" != "1" ]; then
  echo "invalid Xray checksum file" >&2
  exit 1
fi
digest="$(cat "$tmp/$asset_name.digest")"
printf '%s  %s\n' "$digest" "$asset_name" > "$tmp/$asset_name.sha256"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$tmp" && sha256sum -c "$asset_name.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$tmp" && shasum -a 256 -c "$asset_name.sha256")
else
  echo "sha256sum or shasum is required" >&2; exit 1
fi
unzip -oq "$tmp/$asset_name" -d "$tmp/xray"
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl stop migate-xray 2>/dev/null || true
fi
install_tmp="/usr/local/bin/.xray.new.$$"
rm -f "$install_tmp"
cp "$tmp/xray/xray" "$install_tmp"
chmod +x "$install_tmp"
mv -f "$install_tmp" /usr/local/bin/xray
install_tmp=""
mkdir -p /usr/local/share/xray /etc/migate/cores /var/lib/migate/backups
[ -f "$tmp/xray/geosite.dat" ] && cp "$tmp/xray/geosite.dat" /usr/local/share/xray/geosite.dat
[ -f "$tmp/xray/geoip.dat" ] && cp "$tmp/xray/geoip.dat" /usr/local/share/xray/geoip.dat
atomic_write_file() {
  path="$1"
  mode="$2"
  owner="${3:-}"
  dir="$(dirname "$path")"
  base="$(basename "$path")"
  tmp_file="$(mktemp "${dir}/.${base}.tmp.XXXXXX")"
  if ! cat > "$tmp_file"; then
    rm -f "$tmp_file"
    return 1
  fi
  if [ -n "$owner" ]; then
    chown "$owner" "$tmp_file" 2>/dev/null || true
  fi
  chmod "$mode" "$tmp_file"
  mv -f "$tmp_file" "$path"
}
write_migate_default_xray_config() {
  path="$1"
  atomic_write_file "$path" 0640 root:migate <<'JSON'
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
    },
    "system": {
      "statsInboundUplink": true,
      "statsInboundDownlink": true
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
install_migate_default_xray_config() {
  tmp_config="$(mktemp /etc/migate/cores/.xray-default.XXXXXX.json)"
  write_migate_default_xray_config "$tmp_config"
  /usr/local/bin/xray run -test -c "$tmp_config"
  mv -f "$tmp_config" /etc/migate/cores/xray.json
  chown root:migate /etc/migate/cores/xray.json 2>/dev/null || true
  chmod 0640 /etc/migate/cores/xray.json
}
backup_migate_invalid_core_config() {
  path="$1"
  if [ ! -e "$path" ]; then
    return 0
  fi
  backup="/var/lib/migate/backups/xray-config-invalid-$(date +%Y%m%d-%H%M%S).json"
  mv -f "$path" "$backup"
  echo "backed up invalid config: $backup" >&2
}
if [ ! -f /etc/migate/cores/xray.json ]; then
  install_migate_default_xray_config
fi
if ! /usr/local/bin/xray run -test -c /etc/migate/cores/xray.json; then
  echo "existing Xray config check failed; backing it up and writing MiGate default config" >&2
  backup_migate_invalid_core_config /etc/migate/cores/xray.json
  install_migate_default_xray_config
  /usr/local/bin/xray run -test -c /etc/migate/cores/xray.json
fi
if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "systemd is unavailable; skipped migate-xray.service"
  /usr/local/bin/xray version | sed -n '1p'
  exit 0
fi
atomic_write_file /etc/systemd/system/migate-xray.service 0644 root:root <<'UNIT'
[Unit]
Description=MiGate managed Xray service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/xray run -c /etc/migate/cores/xray.json
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
systemctl enable migate-xray
systemctl restart migate-xray
systemctl is-active --quiet migate-xray
/usr/local/bin/xray version | sed -n '1p'`

const singboxInstallScript = `set -euo pipefail
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
  systemctl stop migate-sing-box 2>/dev/null || true
fi
install_tmp="/usr/local/bin/.sing-box.new.$$"
rm -f "$install_tmp"
cp "$tmp"/sing-box-*/sing-box "$install_tmp"
chmod +x "$install_tmp"
mv -f "$install_tmp" /usr/local/bin/sing-box
install_tmp=""
mkdir -p /etc/migate/cores /var/lib/migate/backups
atomic_write_file() {
  path="$1"
  mode="$2"
  owner="${3:-}"
  dir="$(dirname "$path")"
  base="$(basename "$path")"
  tmp_file="$(mktemp "${dir}/.${base}.tmp.XXXXXX")"
  if ! cat > "$tmp_file"; then
    rm -f "$tmp_file"
    return 1
  fi
  if [ -n "$owner" ]; then
    chown "$owner" "$tmp_file" 2>/dev/null || true
  fi
  chmod "$mode" "$tmp_file"
  mv -f "$tmp_file" "$path"
}
write_migate_default_singbox_config() {
  path="$1"
  atomic_write_file "$path" 0640 root:migate <<'JSON'
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
install_migate_default_singbox_config() {
  tmp_config="$(mktemp /etc/migate/cores/.sing-box-default.XXXXXX.json)"
  write_migate_default_singbox_config "$tmp_config"
  /usr/local/bin/sing-box check -c "$tmp_config"
  mv -f "$tmp_config" /etc/migate/cores/sing-box.json
  chown root:migate /etc/migate/cores/sing-box.json 2>/dev/null || true
  chmod 0640 /etc/migate/cores/sing-box.json
}
backup_migate_invalid_core_config() {
  path="$1"
  if [ ! -e "$path" ]; then
    return 0
  fi
  backup="/var/lib/migate/backups/sing-box-config-invalid-$(date +%Y%m%d-%H%M%S).json"
  mv -f "$path" "$backup"
  echo "backed up invalid config: $backup" >&2
}
if [ ! -f /etc/migate/cores/sing-box.json ]; then
  install_migate_default_singbox_config
fi
if ! /usr/local/bin/sing-box check -c /etc/migate/cores/sing-box.json; then
  echo "existing sing-box config check failed; backing it up and writing MiGate default config" >&2
  backup_migate_invalid_core_config /etc/migate/cores/sing-box.json
  install_migate_default_singbox_config
  /usr/local/bin/sing-box check -c /etc/migate/cores/sing-box.json
fi
if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "systemd is unavailable; skipped migate-sing-box.service"
  /usr/local/bin/sing-box version | sed -n '1p'
  exit 0
fi
atomic_write_file /etc/systemd/system/migate-sing-box.service 0644 root:root <<'UNIT'
[Unit]
Description=sing-box service managed by MiGate
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sing-box run -c /etc/migate/cores/sing-box.json
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable migate-sing-box
systemctl restart migate-sing-box
systemctl is-active --quiet migate-sing-box
/usr/local/bin/sing-box version | sed -n '1p'`
