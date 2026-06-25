#!/usr/bin/env bash
set -euo pipefail

STATE_FILE="${MIGATE_RUN_DIR:-/run/migate}/services.env"
mkdir -p "$(dirname "$STATE_FILE")" /var/log/migate
touch "$STATE_FILE"

service_key() {
  printf '%s' "$1" | tr '.-' '__'
}

read_pid() {
  local key
  key="$(service_key "$1")"
  awk -F= -v name="${key}_PID" '$1 == name { print $2 }' "$STATE_FILE" | tail -1
}

read_started() {
  local key
  key="$(service_key "$1")"
  awk -F= -v name="${key}_STARTED" '$1 == name { print $2 }' "$STATE_FILE" | tail -1
}

write_state() {
  local service="$1"
  local pid="$2"
  local started="$3"
  local key tmp
  key="$(service_key "$service")"
  tmp="$(mktemp "${STATE_FILE}.XXXXXX")"
  if [ -f "$STATE_FILE" ]; then
    awk -F= -v pid_key="${key}_PID" -v started_key="${key}_STARTED" '$1 != pid_key && $1 != started_key { print }' "$STATE_FILE" >"$tmp"
  fi
  {
    printf '%s_PID=%s\n' "$key" "$pid"
    printf '%s_STARTED=%s\n' "$key" "$started"
  } >>"$tmp"
  mv "$tmp" "$STATE_FILE"
}

clear_state() {
  local service="$1"
  local key tmp
  key="$(service_key "$service")"
  tmp="$(mktemp "${STATE_FILE}.XXXXXX")"
  if [ -f "$STATE_FILE" ]; then
    awk -F= -v pid_key="${key}_PID" -v started_key="${key}_STARTED" '$1 != pid_key && $1 != started_key { print }' "$STATE_FILE" >"$tmp"
  fi
  mv "$tmp" "$STATE_FILE"
}

is_running() {
  local pid="$1"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

service_command() {
  case "$1" in
    migate-xray) printf '/usr/local/bin/xray run -c /etc/migate/cores/xray.json' ;;
    migate-sing-box) printf '/usr/local/bin/sing-box run -c /etc/migate/cores/sing-box.json' ;;
    migate) printf '' ;;
    *) return 1 ;;
  esac
}

restart_service() {
  local service="$1"
  local command pid started
  command="$(service_command "$service")" || {
    echo "unknown service: $service" >&2
    return 5
  }
  if [ -z "$command" ]; then
    return 0
  fi
  pid="$(read_pid "$service")"
  if is_running "$pid"; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  started="$(date '+%a %Y-%m-%d %H:%M:%S %Z')"
  nohup bash -lc "exec $command" >>"/var/log/migate/${service}.log" 2>&1 &
  pid="$!"
  sleep 0.2
  if ! is_running "$pid"; then
    clear_state "$service"
    echo "$service failed to start; see /var/log/migate/${service}.log" >&2
    return 1
  fi
  write_state "$service" "$pid" "$started"
}

stop_service() {
  local service="$1"
  local pid
  pid="$(read_pid "$service")"
  if is_running "$pid"; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  clear_state "$service"
}

show_service() {
  local service="$1"
  shift || true
  local pid started load_only value_only prop
  pid="$(read_pid "$service")"
  started="$(read_started "$service")"
  load_only=0
  value_only=0
  for arg in "$@"; do
    [ "$arg" = "--property=LoadState" ] && load_only=1
    [ "$arg" = "--value" ] && value_only=1
  done
  if [ "$load_only" -eq 1 ]; then
    if [ "$value_only" -eq 1 ]; then
      echo loaded
    else
      echo LoadState=loaded
    fi
    return 0
  fi
  for arg in "$@"; do
    case "$arg" in
      --property=MemoryCurrent) prop=MemoryCurrent ;;
      --property=MainPID) prop=MainPID ;;
      --property=ActiveEnterTimestamp) prop=ActiveEnterTimestamp ;;
      --property=ActiveEnterTimestampMonotonic) prop=ActiveEnterTimestampMonotonic ;;
      *) prop="" ;;
    esac
    case "$prop" in
      MemoryCurrent) echo "MemoryCurrent=0" ;;
      MainPID) echo "MainPID=${pid:-0}" ;;
      ActiveEnterTimestamp) echo "ActiveEnterTimestamp=${started:-}" ;;
      ActiveEnterTimestampMonotonic) echo "ActiveEnterTimestampMonotonic=0" ;;
    esac
  done
}

cmd="${1:-}"
service="${2:-}"

case "$cmd" in
  restart)
    restart_service "$service"
    ;;
  stop)
    stop_service "$service"
    ;;
  is-active)
    pid="$(read_pid "$service")"
    if is_running "$pid"; then
      echo active
      exit 0
    fi
    echo inactive
    exit 3
    ;;
  show)
    show_service "$service" "${@:3}"
    ;;
  status)
    pid="$(read_pid "$service")"
    if is_running "$pid"; then
      echo "$service active (running)"
      exit 0
    fi
    echo "$service inactive"
    exit 3
    ;;
  enable|disable|daemon-reload|reset-failed)
    ;;
  list-unit-files)
    echo "migate.service enabled"
    echo "migate-xray.service enabled"
    echo "migate-sing-box.service enabled"
    ;;
  *)
    echo "docker systemctl shim: unsupported command: $*" >&2
    exit 1
    ;;
esac
