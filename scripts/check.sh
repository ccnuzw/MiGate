#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/web"

if [ -f package-lock.json ]; then
  npm ci --prefer-offline --no-audit
else
  npm install --no-audit
fi
npm test
npm run build

cd "$ROOT_DIR"
go test ./...
go vet ./...
go build ./cmd/migate
