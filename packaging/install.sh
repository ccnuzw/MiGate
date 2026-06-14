#!/usr/bin/env bash
set -euo pipefail

REPO="${MIGATE_REPO:-imzyb/MiGate}"
VERSION="${MIGATE_VERSION:-latest}"
INSTALL_DIR="${MIGATE_INSTALL_DIR:-/usr/local/migate}"
CONFIG_DIR="${MIGATE_CONFIG_DIR:-/etc/migate}"
CONFIG_PATH="${MIGATE_CONFIG_PATH:-/etc/migate/panel.json}"
SERVICE_PATH="${MIGATE_SERVICE_PATH:-/etc/systemd/system/migate.service}"
MIGATE_BIN="${MIGATE_BIN:-/usr/local/bin/migate}"
MIGATE_LINK="${MIGATE_LINK:-/usr/local/bin/mg}"
INSTALLER_BIN="${INSTALLER_BIN:-/usr/local/bin/migate-install}"
UNINSTALLER_BIN="${UNINSTALLER_BIN:-/usr/local/bin/migate-uninstall}"
SINGBOX_SERVICE_PATH="${SINGBOX_SERVICE_PATH:-/etc/systemd/system/migate-singbox.service}"

ACTION="auto"
ASSUME_YES=0
DRY_RUN=0
REGENERATE_CONFIG=0
INSTALL_XRAY=0
INSTALL_SINGBOX=0
SKIP_CORE_PROMPTS=0
EXTRA_ARGS_COUNT=0
XRAY_FOUND=0
SINGBOX_FOUND=0
CORE_PROMPTS_CONFIRMED=0

OS_NAME="unknown"
ARCH="unknown"
SYSTEMD_AVAILABLE=0
IS_ROOT=0
PANEL_PORT=9999
PANEL_USERNAME="admin"
PANEL_PASSWORD=""
WEB_BASE_PATH="/panel"
PANEL_BIND_HOST="${MIGATE_PANEL_BIND_HOST:-127.0.0.1}"
GENERATED_PASSWORD=0

on_error() {
  local code="$?"
  section "安装未完成"
  log_error "脚本在执行过程中失败，退出码：${code}"
  log_info "如 MiGate 服务启动失败，请查看：journalctl -u migate -n 80 --no-pager"
  log_info "如下载失败，请检查服务器网络、DNS 和 GitHub 访问。"
  log_info "可以使用 --dry-run 预览安装步骤。"
  exit "$code"
}
trap on_error ERR

line() { printf '%s\n' '----------------------------------------------------------------'; }
log_info() { printf '  [INFO] %s\n' "$*"; }
log_ok() { printf '  [ OK ] %s\n' "$*"; }
log_warn() { printf '  [WARN] %s\n' "$*"; }
log_error() { printf '  [ERR ] %s\n' "$*" >&2; }
section() {
  printf '\n'
  line
  printf '%s\n' "$*"
  line
}
kv() {
  printf '  %-22s %s\n' "$1:" "$2"
}
prompt_line() {
  printf '  ? %s' "$1"
}
confirm_yes() {
  local prompt="$1"
  local answer
  prompt_line "${prompt} [Y/n]: "
  read -r answer
  case "$answer" in n|N|no|NO) return 1 ;; *) return 0 ;; esac
}
confirm_no() {
  local prompt="$1"
  local answer
  prompt_line "${prompt} [y/N]: "
  read -r answer
  case "$answer" in y|Y|yes|YES) return 0 ;; *) return 1 ;; esac
}
is_valid_port() {
  local port="$1"
  case "$port" in ''|*[!0-9]*) return 1 ;; esac
  [ "$port" -ge 1 ] && [ "$port" -le 65535 ]
}
print_banner() {
  section "MiGate 一键安装器"
  kv "仓库" "$REPO"
  kv "版本" "$VERSION"
  kv "安装目录" "$INSTALL_DIR"
  kv "配置文件" "$CONFIG_PATH"
  if [ "$DRY_RUN" -eq 1 ]; then
    log_warn "当前为 dry-run 模式，只打印计划执行的操作。"
  fi
}

usage() {
  cat <<'EOF'
MiGate installer

Usage:
  install.sh [--install|--upgrade|--uninstall|--repair-service|--install-xray|--install-singbox] [options]

Options:
  --yes, -y             Run non-interactively for MiGate operations.
  --install            Install MiGate. If already installed, upgrade/reinstall binary and keep config.
  --upgrade, --update   Upgrade MiGate and keep existing config.
  --reinstall          Reinstall MiGate binary and keep existing config.
  --fresh-config       Regenerate panel.json during install/reinstall.
  --uninstall          Run MiGate uninstaller. Keeps config/data unless --purge is passed after --.
  --repair-service     Rewrite/repair migate.service only.
  --install-xray       Install/repair Xray only, or include Xray when used with --install.
  --install-singbox    Install/repair sing-box only, or include sing-box when used with --install.
  --dry-run            Print planned commands without changing the system.
  --check              Check latest MiGate release only.
  --version vX.Y.Z     Install or upgrade to a specific release tag.
  -h, --help           Show this help.

Environment:
  MIGATE_VERSION=vX.Y.Z
  MIGATE_REPO=owner/repo
  MIGATE_PANEL_BIND_HOST=127.0.0.1
  SINGBOX_VERSION=1.13.13
EOF
}

run_cmd() {
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] %s\n' "$*"
    return 0
  fi
  "$@"
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

detect_os() {
  case "$(uname -s 2>/dev/null || true)" in
    Linux) OS_NAME="linux" ;;
    Darwin) OS_NAME="macos" ;;
    *) OS_NAME="other" ;;
  esac
}

detect_arch() {
  case "$(uname -m 2>/dev/null || true)" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) ARCH="unsupported" ;;
  esac
}

arch() {
  detect_arch
  if [ "$ARCH" = "unsupported" ]; then
    log_error "unsupported architecture: $(uname -m). MiGate release assets support linux/amd64 and linux/arm64."
    exit 1
  fi
  printf '%s' "$ARCH"
}

detect_systemd() {
  if command_exists systemctl && [ -d /run/systemd/system ]; then
    SYSTEMD_AVAILABLE=1
  else
    SYSTEMD_AVAILABLE=0
  fi
}

detect_root() {
  if [ "$(id -u)" -eq 0 ]; then
    IS_ROOT=1
  else
    IS_ROOT=0
  fi
}

require_root() {
  detect_root
  if [ "$IS_ROOT" -ne 1 ] && [ "$DRY_RUN" -ne 1 ]; then
    log_error "MiGate installer must run as root. Re-run with sudo or root."
    exit 1
  fi
}

require_linux_for_release() {
  detect_os
  if [ "$OS_NAME" != "linux" ] && [ "$DRY_RUN" -ne 1 ]; then
    log_error "One-click release install supports Linux VPS only. macOS can run MiGate manually with a local build."
    exit 1
  fi
}

dependency_status() {
  local missing=0
  for dep in curl tar openssl; do
    if command_exists "$dep"; then
      log_ok "依赖 ${dep}: 已找到 ($(command -v "$dep"))"
    else
      log_warn "依赖 ${dep}: 未找到"
      missing=1
    fi
  done
  if command_exists sha256sum || command_exists shasum; then
    log_ok "依赖 checksum: 已找到"
  else
    log_warn "依赖 checksum: 未找到 sha256sum/shasum"
    missing=1
  fi
  if command_exists wget; then
    log_ok "可选依赖 wget: 已找到 ($(command -v wget))"
  else
    log_info "可选依赖 wget: 未找到"
  fi
  if command_exists unzip; then
    log_ok "可选依赖 unzip: 已找到 ($(command -v unzip))"
  else
    log_info "可选依赖 unzip: 未找到"
  fi
  return "$missing"
}

require_dependencies() {
  local missing=0
  for dep in curl tar; do
    if ! command_exists "$dep"; then
      log_error "required dependency missing: ${dep}"
      missing=1
    fi
  done
  if ! command_exists sha256sum && ! command_exists shasum; then
    log_error "required dependency missing: sha256sum or shasum"
    missing=1
  fi
  if [ "$missing" -ne 0 ] && [ "$DRY_RUN" -ne 1 ]; then
    exit 1
  fi
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

generate_password() {
  if command_exists openssl; then
    openssl rand -base64 24 | tr -d '\n'
  else
    LC_ALL=C tr -dc 'A-Za-z0-9_@%+=:,.-' < /dev/urandom | head -c 32
  fi
}

normalize_web_base_path() {
  local path="$1"
  if [ -z "$path" ] || [ "$path" = "/" ]; then
    printf ''
    return
  fi
  path="/${path#/}"
  path="${path%/}"
  printf '%s' "$path"
}

json_number_value() {
  local key="$1"
  local file="$2"
  sed -nE "s/.*\"${key}\"[[:space:]]*:[[:space:]]*([0-9]+).*/\\1/p" "$file" 2>/dev/null | head -1
}

json_string_value() {
  local key="$1"
  local file="$2"
  sed -nE "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"([^\"]*)\".*/\\1/p" "$file" 2>/dev/null | head -1
}

read_existing_config_defaults() {
  if [ ! -f "$CONFIG_PATH" ]; then
    return
  fi
  PANEL_PORT="$(json_number_value panel_port "$CONFIG_PATH")"
  PANEL_PORT="${PANEL_PORT:-9999}"
  PANEL_USERNAME="$(json_string_value panel_username "$CONFIG_PATH")"
  PANEL_USERNAME="${PANEL_USERNAME:-admin}"
  WEB_BASE_PATH="$(json_string_value web_base_path "$CONFIG_PATH")"
  WEB_BASE_PATH="${WEB_BASE_PATH:-/panel}"
}

port_in_use() {
  local port="$1"
  if command_exists ss; then
    ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${port}$"
  elif command_exists lsof; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1
  elif command_exists netstat; then
    netstat -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${port}$"
  else
    return 1
  fi
}

service_exists() {
  [ -f "$SERVICE_PATH" ] || { [ "$SYSTEMD_AVAILABLE" -eq 1 ] && systemctl list-unit-files migate.service >/dev/null 2>&1; }
}

service_status() {
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then
    systemctl is-active migate 2>/dev/null || printf 'unknown'
  else
    printf 'systemd_unavailable'
  fi
}

binary_version() {
  local bin="$1"
  if [ -x "$bin" ]; then
    "$bin" version 2>/dev/null | head -1 || true
  elif command_exists "$bin"; then
    "$bin" version 2>/dev/null | head -1 || true
  fi
}

detect_existing_install() {
  local found=0
  section "已安装检测"
  if [ -x "$MIGATE_BIN" ]; then
    found=1
    log_ok "MiGate 二进制：$MIGATE_BIN ($(binary_version "$MIGATE_BIN"))"
  else
    log_info "MiGate 二进制：未找到 ($MIGATE_BIN)"
  fi
  if [ -L "$MIGATE_LINK" ] || [ -e "$MIGATE_LINK" ]; then
    found=1
    log_ok "MiGate CLI 链接：$MIGATE_LINK"
  else
    log_info "MiGate CLI 链接：未找到"
  fi
  if service_exists; then
    found=1
    log_ok "systemd 服务：migate.service ($(service_status))"
  else
    log_info "systemd 服务：未找到"
  fi
  if [ -d "$CONFIG_DIR" ]; then
    found=1
    log_ok "配置目录：$CONFIG_DIR"
  else
    log_info "配置目录：未找到"
  fi
  if [ -f "$CONFIG_PATH" ]; then
    found=1
    read_existing_config_defaults
    log_ok "面板配置：$CONFIG_PATH"
  else
    log_info "面板配置：未找到"
  fi
  if [ -f "${INSTALL_DIR}/migate.db" ]; then
    found=1
    log_ok "数据库：${INSTALL_DIR}/migate.db"
  else
    log_info "数据库：未找到 (${INSTALL_DIR}/migate.db)"
  fi
  if pgrep -x migate >/dev/null 2>&1; then
    found=1
    log_ok "进程：migate 正在运行"
  else
    log_info "进程：migate 未运行"
  fi
  if [ -n "${PANEL_PORT:-}" ] && port_in_use "$PANEL_PORT"; then
    log_warn "端口 ${PANEL_PORT}: 已被监听"
  else
    log_info "端口 ${PANEL_PORT:-9999}: 未检测到监听"
  fi
  log_info "WebUI：Release 二进制已内嵌前端静态资源，无需在服务器上安装 Node。"
  [ "$found" -eq 1 ]
}

core_version() {
  local core="$1"
  local name
  name="$(basename "$core")"
  case "$name" in
    xray) "$core" version 2>/dev/null | head -1 || true ;;
    sing-box) "$core" version 2>/dev/null | head -1 || true ;;
  esac
}

core_binary_path() {
  local command_name="$1"
  for path in "/usr/local/bin/${command_name}" "/usr/bin/${command_name}"; do
    if [ -x "$path" ]; then
      printf '%s' "$path"
      return 0
    fi
  done
  command -v "$command_name" 2>/dev/null || true
}

core_service_status() {
  local svc="$1"
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then
    systemctl is-active "$svc" 2>/dev/null || printf 'unknown'
  else
    printf 'systemd_unavailable'
  fi
}

detect_core() {
  local label="$1"
  local command_name="$2"
  local service_name="$3"
  local found=0
  log_info "${label}: 检测二进制路径"
  for path in "/usr/local/bin/${command_name}" "/usr/bin/${command_name}"; do
    if [ -x "$path" ]; then
      found=1
      log_ok "${label} binary: ${path} ($(core_version "$path"))"
    else
      log_info "${label} binary: not found at ${path}"
    fi
  done
  if command_exists "$command_name"; then
    found=1
    log_ok "${label} command: $(command -v "$command_name") ($(core_version "$command_name"))"
  fi
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ] && systemctl list-unit-files "${service_name}.service" >/dev/null 2>&1; then
    log_ok "${label} service: ${service_name}.service ($(core_service_status "$service_name"))"
  else
    log_info "${label} service: ${service_name}.service not found"
  fi
  [ "$found" -eq 1 ]
}

environment_report() {
  section "环境检测"
  detect_os
  detect_arch
  detect_root
  detect_systemd
  kv "系统" "${OS_NAME}"
  kv "架构" "${ARCH}"
  if [ "$IS_ROOT" -eq 1 ]; then log_ok "权限：root"; else log_warn "权限：非 root，实际安装需要 sudo/root。"; fi
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then log_ok "systemd：可用"; else log_warn "systemd：不可用，将跳过服务写入。"; fi
  dependency_status || true
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --install) ACTION="install"; shift ;;
      --upgrade|--update) ACTION="upgrade"; shift ;;
      --reinstall) ACTION="reinstall"; shift ;;
      --fresh-config) REGENERATE_CONFIG=1; shift ;;
      --uninstall) ACTION="uninstall"; shift ;;
      --repair-service) ACTION="repair-service"; shift ;;
      --install-xray) INSTALL_XRAY=1; ACTION="${ACTION:-auto}"; shift ;;
      --install-singbox) INSTALL_SINGBOX=1; ACTION="${ACTION:-auto}"; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      --yes|-y) ASSUME_YES=1; SKIP_CORE_PROMPTS=1; shift ;;
      --check) ACTION="check"; shift ;;
      --version)
        [ "$#" -ge 2 ] || { log_error "--version requires a value"; exit 2; }
        VERSION="$2"
        shift 2
        ;;
      -h|--help) usage; exit 0 ;;
      --)
        shift
        EXTRA_ARGS=("$@")
        EXTRA_ARGS_COUNT="$#"
        break
        ;;
      *)
        log_error "unknown argument: $1"
        usage >&2
        exit 2
        ;;
    esac
  done
}

release_base_url() {
  if [ "$VERSION" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download' "$REPO"
  else
    printf 'https://github.com/%s/releases/download/%s' "$REPO" "$VERSION"
  fi
}

download_file() {
  local url="$1"
  local dest="$2"
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] curl -fL %q -o %q\n' "$url" "$dest"
    return 0
  fi
  curl -fL "$url" -o "$dest"
}

verify_sha256() {
  local sha_file="$1"
  local work_dir="$2"
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] verify sha256 with %s in %s\n' "$sha_file" "$work_dir"
    return 0
  fi
  if command_exists sha256sum; then
    (cd "$work_dir" && sha256sum -c "$sha_file")
  else
    (cd "$work_dir" && shasum -a 256 -c "$sha_file")
  fi
}

download_release_asset() {
  BASE_URL="$(release_base_url)"
  URL="${BASE_URL}/${ARTIFACT}"
  CHECKSUM_URL="${BASE_URL}/checksums.txt"

  log_info "下载 Release 包：${URL}"
  download_file "$URL" "$TMP/${ARTIFACT}"
  log_info "下载校验文件：${CHECKSUM_URL}"
  download_file "$CHECKSUM_URL" "$TMP/checksums.txt"
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] grep "migate-linux-${ARCH}.tar.gz" %q > %q\n' "$TMP/checksums.txt" "$TMP/${ARTIFACT}.sha256"
    printf '[DRY-RUN] tar -xzf %q -C %q\n' "$TMP/migate-linux-${ARCH}.tar.gz" "$TMP"
    return 0
  fi
  grep "migate-linux-${ARCH}.tar.gz" "$TMP/checksums.txt" > "$TMP/${ARTIFACT}.sha256"
  log_info "校验 Release 包 sha256"
  verify_sha256 "${ARTIFACT}.sha256" "$TMP"
  log_info "解压 Release 包"
  tar -xzf "$TMP/migate-linux-${ARCH}.tar.gz" -C "$TMP"
}

install_migate_binary_from_tmp() {
  run_cmd mkdir -p "$INSTALL_DIR"
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] install %q to %q using atomic temp file\n' "$TMP/migate" "$MIGATE_BIN"
    printf '[DRY-RUN] ln -sf %q %q\n' "$MIGATE_BIN" "$MIGATE_LINK"
    printf '[DRY-RUN] install packaged installer/uninstaller when present\n'
    return 0
  fi
  local migate_tmp
  migate_tmp="$(mktemp /usr/local/bin/.migate.XXXXXX)"
  cat "$TMP/migate" > "$migate_tmp"
  chmod +x "$migate_tmp"
  mv -f "$migate_tmp" "$MIGATE_BIN"
  ln -sf "$MIGATE_BIN" "$MIGATE_LINK"
  log_ok "MiGate 二进制已安装：$MIGATE_BIN"
  log_ok "CLI 快捷命令已安装：$MIGATE_LINK"
  if [ -f "$TMP/packaging/install.sh" ]; then
    local installer_tmp
    installer_tmp="$(mktemp /usr/local/bin/.migate-install.XXXXXX)"
    cat "$TMP/packaging/install.sh" > "$installer_tmp"
    chmod +x "$installer_tmp"
    mv -f "$installer_tmp" "$INSTALLER_BIN"
    log_ok "安装器已安装：$INSTALLER_BIN"
  fi
  if [ -f "$TMP/packaging/uninstall.sh" ]; then
    local uninstaller_tmp
    uninstaller_tmp="$(mktemp /usr/local/bin/.migate-uninstall.XXXXXX)"
    cat "$TMP/packaging/uninstall.sh" > "$uninstaller_tmp"
    chmod +x "$uninstaller_tmp"
    mv -f "$uninstaller_tmp" "$UNINSTALLER_BIN"
    log_ok "卸载器已安装：$UNINSTALLER_BIN"
  fi
}

check_update() {
  latest="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": "([^"]+)".*/\1/' || true)"
  current="$("$MIGATE_BIN" version 2>/dev/null | awk '{print $NF}' || true)"
  [ -n "$current" ] || current="unknown"
  [ -n "$latest" ] || latest="unknown"
  echo "Current version: ${current}"
  echo "Latest version: ${latest}"
  if [ "$current" != "$latest" ] && [ "$latest" != "unknown" ]; then
    echo "Update available: yes"
    echo "Run: mg update"
  else
    echo "Update available: no"
  fi
}

print_config_summary() {
  section "安装配置摘要"
  kv "面板监听" "${PANEL_BIND_HOST}:${PANEL_PORT}"
  kv "Web base path" "${WEB_BASE_PATH:-/}"
  kv "管理员用户" "${PANEL_USERNAME}"
  if [ -f "$CONFIG_PATH" ] && [ "$REGENERATE_CONFIG" -ne 1 ]; then
    kv "管理员密码" "保留已有配置"
  elif [ "$GENERATED_PASSWORD" -eq 1 ]; then
    kv "管理员密码" "随机生成，完成后仅显示一次"
  else
    kv "管理员密码" "使用刚才输入的密码"
  fi
  kv "配置文件" "$CONFIG_PATH"
  kv "数据库" "${INSTALL_DIR}/migate.db"
  kv "Xray 配置" "${INSTALL_DIR}/xray.json"
}

write_config() {
  local panel_port="$1"
  local panel_username="$2"
  local panel_password="$3"
  local web_base_path="$4"
  if [ -f "$CONFIG_PATH" ] && [ "$REGENERATE_CONFIG" -ne 1 ]; then
    log_ok "保留已有配置：$CONFIG_PATH"
    return 0
  fi
  run_cmd mkdir -p "$CONFIG_DIR"
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] write panel config %q with mode 600\n' "$CONFIG_PATH"
    return 0
  fi
  cat > "$CONFIG_PATH" <<JSON
{
  "panel_port": ${panel_port},
  "panel_username": "$(json_escape "$panel_username")",
  "panel_password": "$(json_escape "$panel_password")",
  "web_base_path": "$(json_escape "$web_base_path")",
  "database_path": "$(json_escape "$INSTALL_DIR")/migate.db",
  "xray_config_path": "$(json_escape "$INSTALL_DIR")"
}
JSON
  chmod 600 "$CONFIG_PATH"
  log_ok "配置已写入：$CONFIG_PATH"
}

prompt_config() {
  if [ -f "$CONFIG_PATH" ] && [ "$REGENERATE_CONFIG" -ne 1 ]; then
    read_existing_config_defaults
    log_ok "使用已有配置，不重新生成 panel.json"
    print_config_summary
    return 0
  fi
  if [ "$ASSUME_YES" -eq 1 ]; then
    PANEL_PORT="${PANEL_PORT:-9999}"
    PANEL_USERNAME="${PANEL_USERNAME:-admin}"
    WEB_BASE_PATH="$(normalize_web_base_path "${WEB_BASE_PATH:-/panel}")"
    PANEL_PASSWORD="$(generate_password)"
    GENERATED_PASSWORD=1
    print_config_summary
    return 0
  fi

  section "面板配置"
  log_info "直接回车会使用方括号中的默认值。"
  while true; do
    prompt_line "面板端口 [${PANEL_PORT:-9999}]: "
    read -r input_panel_port
    PANEL_PORT="${input_panel_port:-${PANEL_PORT:-9999}}"
    if is_valid_port "$PANEL_PORT"; then
      break
    fi
    log_warn "端口必须是 1-65535 之间的数字，请重新输入。"
  done

  prompt_line "管理员用户名 [${PANEL_USERNAME:-admin}]: "
  read -r input_panel_username
  PANEL_USERNAME="${input_panel_username:-${PANEL_USERNAME:-admin}}"

  prompt_line "管理员密码 [留空则随机生成]: "
  read -r -s PANEL_PASSWORD
  printf '\n'
  if [ -z "$PANEL_PASSWORD" ]; then
    PANEL_PASSWORD="$(generate_password)"
    GENERATED_PASSWORD=1
    log_warn "未输入密码，已生成随机密码。安装完成时只显示一次，请保存。"
  fi

  prompt_line "Web base path [${WEB_BASE_PATH:-/panel}]: "
  read -r input_web_base_path
  WEB_BASE_PATH="${input_web_base_path:-${WEB_BASE_PATH:-/panel}}"
  WEB_BASE_PATH="$(normalize_web_base_path "$WEB_BASE_PATH")"

  print_config_summary
  if ! confirm_yes "确认使用以上配置继续安装？"; then
    log_error "用户取消安装。"
    exit 1
  fi
}

write_systemd_service() {
  if [ "$SYSTEMD_AVAILABLE" -ne 1 ]; then
    log_warn "systemd 不可用，跳过 migate.service 写入。"
    return 0
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] write %q\n' "$SERVICE_PATH"
    return 0
  fi
  cat > "$SERVICE_PATH" <<UNIT
[Unit]
Description=MiGate Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${MIGATE_BIN} serve --host ${PANEL_BIND_HOST} --config ${CONFIG_PATH}
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT
  log_ok "systemd service written: $SERVICE_PATH"
}

restart_migate_service() {
  if [ "$SYSTEMD_AVAILABLE" -ne 1 ]; then
    log_warn "systemd 不可用，手动运行：${MIGATE_BIN} serve --config ${CONFIG_PATH}"
    return 0
  fi
  run_cmd systemctl daemon-reload
  run_cmd systemctl enable migate
  run_cmd systemctl restart migate
  if [ "$DRY_RUN" -eq 0 ]; then
    if systemctl is-active migate >/dev/null 2>&1; then
      log_ok "MiGate service: running"
    else
      log_error "MiGate service failed to start. Run: journalctl -u migate -n 80 --no-pager"
      return 1
    fi
  fi
}

install_xray() {
  section "安装/修复 Xray"
  log_warn "将下载并执行官方 Xray-install 脚本：https://github.com/XTLS/Xray-install"
  log_warn "如服务器已有自定义 Xray 安装，请先确认是否允许修复/覆盖。"
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] xray_tmp="$(mktemp -d)"\n'
    printf '[DRY-RUN] curl -fL "https://github.com/XTLS/Xray-install/raw/main/install-release.sh" -o "$xray_tmp/install-release.sh"\n'
    printf '[DRY-RUN] bash "$xray_tmp/install-release.sh"\n'
    printf '[DRY-RUN] ln -sf %q /usr/local/etc/xray/xray.json\n' "${INSTALL_DIR}/xray.json"
    printf '[DRY-RUN] ln -sf %q /usr/local/etc/xray/config.json\n' "${INSTALL_DIR}/xray.json"
    printf '[DRY-RUN] systemctl enable xray && systemctl restart xray\n'
    return 0
  fi
  if [ "$ASSUME_YES" -ne 1 ] && [ "$CORE_PROMPTS_CONFIRMED" -ne 1 ]; then
    if ! confirm_no "确认安装/修复 Xray？"; then
      log_warn "跳过 Xray 安装。"
      return 0
    fi
  fi
  local xray_tmp
  xray_tmp="$(mktemp -d)"
  log_info "下载 Xray 官方安装脚本"
  curl -fL "https://github.com/XTLS/Xray-install/raw/main/install-release.sh" -o "$xray_tmp/install-release.sh"
  log_info "执行 Xray 官方安装脚本"
  bash "$xray_tmp/install-release.sh"
  rm -rf "$xray_tmp"
  mkdir -p /usr/local/etc/xray
  ln -sf "${INSTALL_DIR}/xray.json" /usr/local/etc/xray/xray.json
  ln -sf "${INSTALL_DIR}/xray.json" /usr/local/etc/xray/config.json
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then
    systemctl enable xray 2>/dev/null || true
    systemctl restart xray 2>/dev/null || true
  fi
  log_ok "Xray 安装/修复完成"
}

install_singbox() {
  section "安装/修复 sing-box"
  local sb_version="${SINGBOX_VERSION:-1.13.13}"
  local sb_asset_arch
  case "$(arch)" in
    amd64) sb_asset_arch="amd64" ;;
    arm64) sb_asset_arch="arm64" ;;
    *) log_error "unsupported sing-box architecture"; return 1 ;;
  esac
  local sb_artifact="sing-box-${sb_version}-linux-${sb_asset_arch}.tar.gz"
  local sb_url="https://github.com/SagerNet/sing-box/releases/download/v${sb_version}/sing-box-${sb_version}-linux-${sb_asset_arch}.tar.gz"
  local sb_checksums_url="https://github.com/SagerNet/sing-box/releases/download/v${sb_version}/sing-box-${sb_version}-checksums.txt"
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[DRY-RUN] curl -fL %q -o "$tmp_sb/%s"\n' "$sb_url" "$sb_artifact"
    printf '[DRY-RUN] curl -fL %q -o "$tmp_sb/checksums.txt"\n' "$sb_checksums_url"
    printf '[DRY-RUN] grep %q "$tmp_sb/checksums.txt" > "$tmp_sb/%s.sha256"\n' "$sb_artifact" "$sb_artifact"
    printf '[DRY-RUN] sha256sum -c "%s.sha256"\n' "$sb_artifact"
    printf '[DRY-RUN] install /usr/local/bin/sing-box\n'
    if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then
      printf '[DRY-RUN] write %q and restart migate-singbox\n' "$SINGBOX_SERVICE_PATH"
    else
      printf '[DRY-RUN] skip %q because systemd is unavailable\n' "$SINGBOX_SERVICE_PATH"
    fi
    return 0
  fi
  if [ "$ASSUME_YES" -ne 1 ] && [ "$CORE_PROMPTS_CONFIRMED" -ne 1 ]; then
    if ! confirm_no "确认安装/修复 sing-box ${sb_version}？"; then
      log_warn "跳过 sing-box 安装。"
      return 0
    fi
  fi
  log_info "下载 sing-box ${sb_version}: ${sb_url}"
  local tmp_sb
  tmp_sb="$(mktemp -d)"
  curl -fL "$sb_url" -o "$tmp_sb/$sb_artifact"
  log_info "下载 sing-box 校验文件"
  curl -fL "$sb_checksums_url" -o "$tmp_sb/checksums.txt"
  grep "$sb_artifact" "$tmp_sb/checksums.txt" > "$tmp_sb/$sb_artifact.sha256"
  log_info "校验 sing-box sha256"
  verify_sha256 "$sb_artifact.sha256" "$tmp_sb"
  log_info "解压并安装 sing-box"
  tar -xzf "$tmp_sb/$sb_artifact" -C "$tmp_sb"
  cp "$tmp_sb"/sing-box-*/sing-box /usr/local/bin/sing-box
  chmod +x /usr/local/bin/sing-box
  rm -rf "$tmp_sb"
  mkdir -p /etc/sing-box
  if [ ! -f /etc/sing-box/config.json ]; then
    printf '%s\n' '{"log":{"level":"warn"},"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}]}' > /etc/sing-box/config.json
  fi
  if [ "$SYSTEMD_AVAILABLE" -ne 1 ]; then
    log_warn "systemd 不可用，跳过 migate-singbox.service 写入。"
    log_info "Manual run: /usr/local/bin/sing-box run -c /etc/sing-box/config.json"
    log_ok "sing-box 安装/修复完成"
    return 0
  fi
  cat > "$SINGBOX_SERVICE_PATH" <<'UNIT'
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
  systemctl restart migate-singbox 2>/dev/null || true
  log_ok "sing-box 安装/修复完成"
}

prompt_core_installs() {
  if [ "$SKIP_CORE_PROMPTS" -eq 1 ]; then
    if [ "$INSTALL_XRAY" -ne 1 ]; then log_warn "未指定 --install-xray，跳过 Xray 安装。"; fi
    if [ "$INSTALL_SINGBOX" -ne 1 ]; then log_warn "未指定 --install-singbox，跳过 sing-box 安装。"; fi
    return 0
  fi
  CORE_PROMPTS_CONFIRMED=1
  if [ "$INSTALL_XRAY" -ne 1 ]; then
    if [ "$XRAY_FOUND" -eq 1 ]; then
      if confirm_no "检测到 Xray 已安装，是否重新安装/修复 Xray？"; then
        INSTALL_XRAY=1
      else
        log_info "保留现有 Xray 安装。"
      fi
    else
      if confirm_yes "未检测到 Xray，是否安装 Xray？"; then
        INSTALL_XRAY=1
      else
        log_warn "跳过 Xray。核心代理功能可能不可用。"
      fi
    fi
  fi
  if [ "$INSTALL_SINGBOX" -ne 1 ]; then
    if [ "$SINGBOX_FOUND" -eq 1 ]; then
      if confirm_no "检测到 sing-box 已安装，是否重新安装/修复 sing-box？"; then
        INSTALL_SINGBOX=1
      else
        log_info "保留现有 sing-box 安装。"
      fi
    else
      if confirm_yes "未检测到 sing-box，是否安装 sing-box？"; then
        INSTALL_SINGBOX=1
      else
        log_warn "跳过 sing-box。Hysteria2/TUIC/ShadowTLS 可能不可用。"
      fi
    fi
  fi
}

install_release_flow() {
  local mode="$1"
  require_linux_for_release
  require_root
  require_dependencies
  ARCH="$(arch)"
  ARTIFACT="migate-linux-${ARCH}.tar.gz"
  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT

  section "配置确认"
  if [ "$mode" = "fresh" ]; then
    REGENERATE_CONFIG=1
  fi
  prompt_config
  if [ -n "${PANEL_PORT:-}" ] && port_in_use "$PANEL_PORT" && ! pgrep -x migate >/dev/null 2>&1; then
    log_warn "端口 ${PANEL_PORT} 已被占用，服务启动可能失败。"
  fi
  section "安装计划"
  kv "动作" "$mode"
  kv "Release 资产" "$ARTIFACT"
  kv "安装 MiGate" "$MIGATE_BIN"
  kv "写入配置" "$CONFIG_PATH"
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then
    kv "写入服务" "$SERVICE_PATH"
  else
    kv "写入服务" "跳过，systemd 不可用"
  fi

  section "安装 MiGate"
  download_release_asset
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then
    run_cmd systemctl stop migate 2>/dev/null || true
  fi
  install_migate_binary_from_tmp
  write_config "$PANEL_PORT" "$PANEL_USERNAME" "$PANEL_PASSWORD" "$WEB_BASE_PATH"
  write_systemd_service

  section "核心检测"
  if detect_core "Xray" "xray" "xray"; then XRAY_FOUND=1; else XRAY_FOUND=0; fi
  if detect_core "sing-box" "sing-box" "migate-singbox"; then SINGBOX_FOUND=1; else SINGBOX_FOUND=0; fi
  prompt_core_installs
  [ "$INSTALL_XRAY" -eq 1 ] && install_xray
  [ "$INSTALL_SINGBOX" -eq 1 ] && install_singbox

  section "服务启动"
  restart_migate_service
  finish_message
}

repair_service_flow() {
  require_root
  detect_systemd
  section "修复 systemd 服务"
  write_systemd_service
  restart_migate_service
  log_ok "服务修复完成"
}

uninstall_flow() {
  require_root
  local args=()
  local args_count=0
  if [ "${EXTRA_ARGS_COUNT:-0}" -gt 0 ]; then
    args=("${EXTRA_ARGS[@]}")
    args_count="${EXTRA_ARGS_COUNT:-0}"
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    args[args_count]="--dry-run"
    args_count=$((args_count + 1))
  fi
  if [ "$ASSUME_YES" -eq 1 ]; then
    args[args_count]="--yes"
    args_count=$((args_count + 1))
  fi
  if [ -x "$UNINSTALLER_BIN" ]; then
    if [ "$args_count" -gt 0 ]; then
      "$UNINSTALLER_BIN" "${args[@]}"
    else
      "$UNINSTALLER_BIN"
    fi
  else
    if [ "$args_count" -gt 0 ]; then
      bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/uninstall.sh" "${args[@]}"
    else
      bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/uninstall.sh"
    fi
  fi
}

interactive_menu() {
  local installed=0
  detect_existing_install && installed=1 || installed=0
  if [ "$installed" -eq 0 ]; then
    ACTION="install"
    return
  fi
  section "操作选择"
  cat <<'EOF'
  1) 升级 MiGate，并保留现有配置
  2) 重装 MiGate，并保留现有配置
  3) 重装 MiGate，并重新生成面板配置
  4) 只修复 migate systemd 服务
  5) 只安装/修复 Xray
  6) 只安装/修复 sing-box
  7) 卸载 MiGate
  8) 退出
EOF
  prompt_line "请选择操作 [1-8]: "
  read -r choice
  case "$choice" in
    1) ACTION="upgrade" ;;
    2) ACTION="reinstall" ;;
    3) ACTION="reinstall"; REGENERATE_CONFIG=1 ;;
    4) ACTION="repair-service" ;;
    5) ACTION="install-xray-only"; INSTALL_XRAY=1 ;;
    6) ACTION="install-singbox-only"; INSTALL_SINGBOX=1 ;;
    7) ACTION="uninstall" ;;
    8) ACTION="exit" ;;
    *) log_error "无效选择"; exit 2 ;;
  esac
}

finish_message() {
  local host_ip
  local xray_bin
  local singbox_bin
  host_ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  [ -n "$host_ip" ] || host_ip="SERVER_IP"
  xray_bin="$(core_binary_path xray)"
  singbox_bin="$(core_binary_path sing-box)"
  section "安装完成，请保存以下信息"
  kv "MiGate 二进制" "$MIGATE_BIN"
  kv "CLI 命令" "mg"
  kv "安装目录" "${INSTALL_DIR}"
  kv "面板监听" "${PANEL_BIND_HOST}:${PANEL_PORT}"
  kv "Web base path" "${WEB_BASE_PATH:-/}"
  kv "WebUI 地址" "http://${host_ip}:${PANEL_PORT}${WEB_BASE_PATH}"
  log_warn "默认仅监听 ${PANEL_BIND_HOST}。公网访问请通过 Nginx/Caddy 等反向代理并启用 HTTPS。"
  kv "管理员用户" "${PANEL_USERNAME}"
  if [ "$GENERATED_PASSWORD" -eq 1 ] || [ -n "$PANEL_PASSWORD" ]; then
    kv "管理员密码" "${PANEL_PASSWORD}"
    log_warn "密码仅在终端显示一次，请立即保存。"
  else
    kv "管理员密码" "保留已有配置中的密码"
  fi
  kv "面板配置" "${CONFIG_PATH}"
  kv "数据库" "${INSTALL_DIR}/migate.db"
  kv "Xray 配置" "${INSTALL_DIR}/xray.json"
  if [ -n "$xray_bin" ]; then
    kv "Xray 二进制" "${xray_bin} ($(core_version "$xray_bin"))"
    if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then kv "Xray 服务" "systemctl status xray"; fi
  else
    log_warn "Xray 二进制：未找到"
  fi
  kv "sing-box 配置" "/etc/sing-box/config.json"
  if [ -n "$singbox_bin" ]; then
    kv "sing-box 二进制" "${singbox_bin} ($(core_version "$singbox_bin"))"
    if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then kv "sing-box 服务" "systemctl status migate-singbox"; fi
  else
    log_warn "sing-box 二进制：未找到"
  fi
  kv "安装器" "${INSTALLER_BIN}"
  kv "卸载器" "${UNINSTALLER_BIN}"
  if [ "$SYSTEMD_AVAILABLE" -eq 1 ]; then
    kv "MiGate 服务文件" "${SERVICE_PATH}"
    kv "MiGate 服务状态" "systemctl status migate"
    kv "MiGate 实时日志" "journalctl -u migate -f"
  else
    kv "手动启动" "${MIGATE_BIN} serve --config ${CONFIG_PATH}"
  fi
  kv "常用命令" "mg status | mg doctor | mg logs -f | mg restart | mg update | mg uninstall"
  section "下一步"
  log_info "如果你需要公网访问面板，请用 Nginx/Caddy 反向代理到 ${PANEL_BIND_HOST}:${PANEL_PORT}${WEB_BASE_PATH}，并启用 HTTPS。"
  log_info "如果服务启动失败，请运行：journalctl -u migate -n 80 --no-pager"
  log_info "如果核心不可用，请运行：mg doctor"
}

main() {
  EXTRA_ARGS=()
  EXTRA_ARGS_COUNT=0
  parse_args "$@"
  print_banner
  environment_report
  if [ "$ACTION" = "check" ]; then
    check_update
    return
  fi
  if [ "$ACTION" = "auto" ] && [ "$ASSUME_YES" -eq 0 ] && [ "$INSTALL_XRAY" -eq 0 ] && [ "$INSTALL_SINGBOX" -eq 0 ]; then
    interactive_menu
  else
    detect_existing_install || true
    if [ "$ACTION" = "auto" ]; then
      if [ "$INSTALL_XRAY" -eq 1 ] && [ "$INSTALL_SINGBOX" -eq 0 ]; then
        ACTION="install-xray-only"
      elif [ "$INSTALL_SINGBOX" -eq 1 ] && [ "$INSTALL_XRAY" -eq 0 ]; then
        ACTION="install-singbox-only"
      elif [ "$INSTALL_XRAY" -eq 1 ] && [ "$INSTALL_SINGBOX" -eq 1 ]; then
        ACTION="install-cores-only"
      else
        ACTION="install"
      fi
    fi
  fi

  case "$ACTION" in
    install|upgrade|reinstall)
      install_release_flow "$([ "$REGENERATE_CONFIG" -eq 1 ] && printf 'fresh' || printf 'preserve')"
      ;;
    repair-service)
      repair_service_flow
      ;;
    install-xray-only)
      require_linux_for_release
      require_root
      detect_systemd
      install_xray
      ;;
    install-singbox-only)
      require_linux_for_release
      require_root
      detect_systemd
      install_singbox
      ;;
    install-cores-only)
      require_linux_for_release
      require_root
      detect_systemd
      install_xray
      install_singbox
      ;;
    uninstall)
      uninstall_flow
      ;;
    exit)
      log_info "退出。"
      ;;
    *)
      log_error "unknown action: ${ACTION}"
      exit 2
      ;;
  esac
}

main "$@"
