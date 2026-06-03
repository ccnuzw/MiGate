#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/dist}"
VERSION="${VERSION:-dev}"

mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR"/migate-linux-*.tar.gz "$DIST_DIR"/checksums.txt

build_one() {
  local arch="$1"
  local work_dir
  work_dir="$(mktemp -d)"

  echo "Building MiGate ${VERSION} linux/${arch}"
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$work_dir/migate" ./cmd/migate

  mkdir -p "$work_dir/packaging"
  cp "$ROOT_DIR/packaging/migate.service" "$work_dir/packaging/migate.service"
  cp "$ROOT_DIR/packaging/install.sh" "$work_dir/packaging/install.sh"
  chmod +x "$work_dir/migate" "$work_dir/packaging/install.sh"

  tar -C "$work_dir" -czf "$DIST_DIR/migate-linux-${arch}.tar.gz" migate packaging/migate.service packaging/install.sh
  rm -rf "$work_dir"
}

main() {
  cd "$ROOT_DIR"
  build_one amd64
  build_one arm64
  (
    cd "$DIST_DIR"
    sha256sum migate-linux-amd64.tar.gz migate-linux-arm64.tar.gz > checksums.txt
  )
  echo "Release artifacts written to $DIST_DIR"
}

main "$@"
