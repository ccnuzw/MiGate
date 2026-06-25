#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR="${MIGATE_CONFIG_DIR:-/etc/migate}"
CORE_CONFIG_DIR="${MIGATE_CORE_CONFIG_DIR:-/etc/migate/cores}"
CERT_DIR="${MIGATE_CERT_DIR:-/etc/migate/certs}"
DATA_DIR="${MIGATE_DATA_DIR:-/var/lib/migate}"
LOG_DIR="${MIGATE_LOG_DIR:-/var/log/migate}"
RUN_DIR="${MIGATE_RUN_DIR:-/run/migate}"
CONFIG_PATH="${MIGATE_CONFIG_PATH:-${CONFIG_DIR}/panel.json}"
PANEL_PORT="${MIGATE_PANEL_PORT:-9999}"
PANEL_USERNAME="${MIGATE_PANEL_USERNAME:-admin}"
PANEL_PASSWORD="${MIGATE_PANEL_PASSWORD:-admin123}"
WEB_BASE_PATH="${MIGATE_WEB_BASE_PATH:-/panel}"
DATABASE_PATH="${MIGATE_DATABASE_PATH:-${DATA_DIR}/migate.db}"

write_json_string() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

mkdir -p "$CONFIG_DIR" "$CORE_CONFIG_DIR" "$CERT_DIR" "$DATA_DIR" "$LOG_DIR" "$RUN_DIR"

if [ ! -f "$CONFIG_PATH" ]; then
  password_hash="$(migate hash-password "$PANEL_PASSWORD")"
  cat >"$CONFIG_PATH" <<JSON
{
  "panel_port": ${PANEL_PORT},
  "panel_username": "$(write_json_string "$PANEL_USERNAME")",
  "panel_password": "$(write_json_string "$password_hash")",
  "web_base_path": "$(write_json_string "$WEB_BASE_PATH")",
  "database_path": "$(write_json_string "$DATABASE_PATH")",
  "management_direct_enabled": true,
  "management_direct_auto_detect": false,
  "management_direct_hosts": [],
  "management_direct_ports": []
}
JSON
  chmod 0640 "$CONFIG_PATH"
fi

if [ ! -f "$CORE_CONFIG_DIR/xray.json" ]; then
  cat >"$CORE_CONFIG_DIR/xray.json" <<'JSON'
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
  chmod 0640 "$CORE_CONFIG_DIR/xray.json"
fi

if [ ! -f "$CORE_CONFIG_DIR/sing-box.json" ]; then
  cat >"$CORE_CONFIG_DIR/sing-box.json" <<'JSON'
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
  chmod 0640 "$CORE_CONFIG_DIR/sing-box.json"
fi

touch "$RUN_DIR/services.env"
chmod 0600 "$RUN_DIR/services.env"

systemctl restart migate-xray >/var/log/migate/bootstrap-xray.log 2>&1 || true
systemctl restart migate-sing-box >/var/log/migate/bootstrap-sing-box.log 2>&1 || true

exec "$@"
