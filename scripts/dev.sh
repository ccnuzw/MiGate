#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_DIR="${MIGATE_DEV_DIR:-$ROOT_DIR/.dev}"
CONFIG_PATH="${MIGATE_CONFIG:-$DEV_DIR/panel.json}"
PANEL_PORT="${MIGATE_PANEL_PORT:-9999}"
WEB_PORT="${MIGATE_WEB_PORT:-5173}"
PANEL_USERNAME="${MIGATE_PANEL_USERNAME:-admin}"
PANEL_PASSWORD="${MIGATE_PANEL_PASSWORD:-admin123}"
WEB_BASE_PATH="${MIGATE_WEB_BASE_PATH:-/panel}"
DATABASE_PATH="${MIGATE_DATABASE_PATH:-$DEV_DIR/migate-dev.db}"
XRAY_CONFIG_DIR="${MIGATE_XRAY_CONFIG_DIR:-$DEV_DIR/xray}"

mkdir -p "$DEV_DIR" "$XRAY_CONFIG_DIR"

if [ ! -f "$CONFIG_PATH" ]; then
  cat > "$CONFIG_PATH" <<JSON
{
  "panel_port": $PANEL_PORT,
  "panel_username": "$PANEL_USERNAME",
  "panel_password": "$PANEL_PASSWORD",
  "web_base_path": "$WEB_BASE_PATH",
  "database_path": "$DATABASE_PATH",
  "xray_config_path": "$XRAY_CONFIG_DIR"
}
JSON
  chmod 600 "$CONFIG_PATH"
elif grep -q '"database_path": "'"$DEV_DIR"'/migate-dev.db"' "$CONFIG_PATH" && grep -q '"web_base_path": "/"' "$CONFIG_PATH"; then
  sed -i.bak 's#"web_base_path": "/"#"web_base_path": "'"$WEB_BASE_PATH"'"#' "$CONFIG_PATH"
fi

if [ ! -d "$ROOT_DIR/web/node_modules" ]; then
  npm --prefix "$ROOT_DIR/web" install
fi

cleanup() {
  if [ "${api_pid:-}" != "" ] && kill -0 "$api_pid" 2>/dev/null; then
    kill "$api_pid" 2>/dev/null || true
  fi
  if [ "${web_pid:-}" != "" ] && kill -0 "$web_pid" 2>/dev/null; then
    kill "$web_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

cd "$ROOT_DIR"

echo "MiGate dev config: $CONFIG_PATH"
echo "Backend API:       http://127.0.0.1:$PANEL_PORT"
echo "Frontend WebUI:    http://127.0.0.1:$WEB_PORT$WEB_BASE_PATH/"
echo "Login account:     $PANEL_USERNAME / $PANEL_PASSWORD"
echo

go run ./cmd/migate serve --host 127.0.0.1 --port "$PANEL_PORT" --config "$CONFIG_PATH" &
api_pid=$!

npm --prefix "$ROOT_DIR/web" run dev -- --port "$WEB_PORT" &
web_pid=$!

set +e
exit_code=0
while kill -0 "$api_pid" 2>/dev/null && kill -0 "$web_pid" 2>/dev/null; do
  sleep 1
done

if ! kill -0 "$api_pid" 2>/dev/null; then
  wait "$api_pid"
  exit_code=$?
elif ! kill -0 "$web_pid" 2>/dev/null; then
  wait "$web_pid"
  exit_code=$?
fi

exit "$exit_code"
