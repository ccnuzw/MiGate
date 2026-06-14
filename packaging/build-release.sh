#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/dist}"
VERSION="${VERSION:-dev}"
ARCHES="${ARCHES:-amd64 arm64}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

build_web() {
  echo "Building embedded WebUI"
  bash "$ROOT_DIR/scripts/build-web.sh"
  test -f "$ROOT_DIR/internal/web/static/dist/index.html"
}

build_one() {
  local arch="$1"
  local work_dir archive
  work_dir="$(mktemp -d)"
  archive="migate-linux-${arch}.tar.gz"

  echo "Building MiGate ${VERSION} linux/${arch}"
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X main.Version=${VERSION}" \
      -o "$work_dir/migate" ./cmd/migate

  mkdir -p "$work_dir/packaging"
  cp "$ROOT_DIR/packaging/migate.service" "$work_dir/packaging/migate.service"
  cp "$ROOT_DIR/packaging/install.sh" "$work_dir/packaging/install.sh"
  cp "$ROOT_DIR/packaging/uninstall.sh" "$work_dir/packaging/uninstall.sh"
  chmod +x "$work_dir/migate" "$work_dir/packaging/install.sh" "$work_dir/packaging/uninstall.sh"

  tar -C "$work_dir" -czf "$DIST_DIR/$archive" \
    migate packaging/migate.service packaging/install.sh packaging/uninstall.sh
  rm -rf "$work_dir"
}

main() {
  cd "$ROOT_DIR"
  require_cmd go
  require_cmd npm
  require_cmd tar
  require_cmd sha256sum

  rm -rf "$DIST_DIR"
  mkdir -p "$DIST_DIR"

  echo "Go: $(go version)"
  echo "Node: $(node --version 2>/dev/null || printf 'missing')"
  echo "npm: $(npm --version)"

  build_web

  for arch in $ARCHES; do
    case "$arch" in
      amd64|arm64) build_one "$arch" ;;
      *) echo "unsupported release arch: $arch" >&2; exit 1 ;;
    esac
  done

  (
    cd "$DIST_DIR"
    sha256sum migate-linux-*.tar.gz > checksums.txt
  )
  echo "Release artifacts written to $DIST_DIR"
}

main "$@"
