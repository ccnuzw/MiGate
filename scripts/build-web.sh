#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/web"
npm ci
npm run build

find "$ROOT_DIR/internal/web/static/dist" -type f \
  \( -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.svg' -o -name '*.json' \) \
  -exec gzip -kf -9 {} \;

if command -v brotli >/dev/null 2>&1; then
  find "$ROOT_DIR/internal/web/static/dist" -type f \
    \( -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.svg' -o -name '*.json' \) \
    -exec brotli -f -q 11 {} \;
fi
