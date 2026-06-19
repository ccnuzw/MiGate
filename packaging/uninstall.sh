#!/usr/bin/env bash
set -euo pipefail

MIGATE_SERVICE="migate"
XRAY_SERVICE="migate-xray"
SINGBOX_SERVICE="migate-sing-box"
MIGATE_BINARY="/usr/local/bin/migate"
MIGATE_LINK="/usr/local/bin/mg"
INSTALLER_BINARY="/usr/local/bin/migate-install"
UNINSTALLER_BINARY="/usr/local/bin/migate-uninstall"
MIGATE_SERVICE_PATH="/etc/systemd/system/migate.service"
XRAY_SERVICE_PATH="/etc/systemd/system/migate-xray.service"
SINGBOX_SERVICE_PATH="/etc/systemd/system/migate-sing-box.service"
MIGATE_CONFIG_DIR="/etc/migate"
MIGATE_DATA_DIR="/var/lib/migate"
MIGATE_LOG_DIR="/var/log/migate"
MIGATE_RUN_DIR="/run/migate"

PURGE=0
ASSUME_YES=0
DRY_RUN=0

usage() {
  cat <<'EOF'
MiGate uninstaller

Usage:
  uninstall.sh [--purge] [--yes] [--dry-run]

Options:
  --purge   Also remove MiGate config, data, logs, backups, and runtime files.
  --yes     Do not ask for confirmation when --purge is used.
  --dry-run Print planned commands without changing the system.
  -h,--help Show this help.

Default uninstall keeps:
  - /etc/migate
  - /var/lib/migate
  - /var/log/migate

Purge removes:
  - /etc/migate
  - /var/lib/migate
  - /var/log/migate
  - /run/migate
EOF
}

require_root() {
  [ "$(id -u)" -eq 0 ] || [ "$DRY_RUN" -eq 1 ] || { echo "MiGate uninstaller must run as root" >&2; exit 1; }
}

run_cmd() {
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] %s\n' "$*"
    return 0
  fi
  "$@"
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --purge) PURGE=1 ;;
      --yes|-y) ASSUME_YES=1 ;;
      --dry-run) DRY_RUN=1 ;;
      -h|--help) usage; exit 0 ;;
      *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
    shift
  done
}

confirm_purge() {
  if [ "$PURGE" -ne 1 ] || [ "$ASSUME_YES" -eq 1 ]; then
    return
  fi
  echo "--purge will permanently remove MiGate configuration, data, logs, and backups."
  read -r -p "Type 'PURGE' to continue: " answer
  if [ "$answer" != "PURGE" ]; then
    echo "Purge cancelled. Re-run without --purge for service-only uninstall."
    exit 1
  fi
}

stop_disable_remove_service() {
  local service="$1"
  local unit_path="$2"
  run_cmd systemctl stop "$service" 2>/dev/null || true
  run_cmd systemctl disable "$service" 2>/dev/null || true
  run_cmd rm -f "$unit_path"
}

main() {
  parse_args "$@"
  require_root
  confirm_purge

  echo "Stopping MiGate services..."
  stop_disable_remove_service "$MIGATE_SERVICE" "$MIGATE_SERVICE_PATH"
  stop_disable_remove_service "$XRAY_SERVICE" "$XRAY_SERVICE_PATH"
  stop_disable_remove_service "$SINGBOX_SERVICE" "$SINGBOX_SERVICE_PATH"

  echo "Removing MiGate command binaries..."
  run_cmd rm -f "$MIGATE_BINARY"
  run_cmd rm -f "$MIGATE_LINK"
  run_cmd rm -f "$INSTALLER_BINARY"
  run_cmd rm -f "$UNINSTALLER_BINARY"

  if [ "$PURGE" -eq 1 ]; then
    echo "Purging MiGate config, data, logs, backups, and runtime files..."
    run_cmd rm -rf "$MIGATE_CONFIG_DIR"
    run_cmd rm -rf "$MIGATE_DATA_DIR"
    run_cmd rm -rf "$MIGATE_LOG_DIR"
    run_cmd rm -rf "$MIGATE_RUN_DIR"
  else
    echo "Keeping MiGate config/data/logs. Use --purge --yes to remove them."
  fi

  run_cmd systemctl daemon-reload 2>/dev/null || true
  run_cmd systemctl reset-failed "$MIGATE_SERVICE" 2>/dev/null || true
  run_cmd systemctl reset-failed "$XRAY_SERVICE" 2>/dev/null || true
  run_cmd systemctl reset-failed "$SINGBOX_SERVICE" 2>/dev/null || true
  run_cmd systemctl reset-failed 2>/dev/null || true

  echo "MiGate uninstalled."
}

main "$@"
