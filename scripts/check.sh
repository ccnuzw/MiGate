#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
cd web
npm install
npm test
npm run build
cd "$ROOT_DIR"
go test ./...
