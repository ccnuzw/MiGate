# Frontend Split Architecture

MiGate now separates the WebUI source from the Go API server while keeping the release artifact as a single Go binary.

## Layout

- `web/` contains the Vite + React + TypeScript + Tailwind frontend.
- `internal/web/router.go` keeps API routes, auth/session handling, subscription routes, static asset serving, and SPA fallback.
- `internal/web/static/dist/` is the Vite build output embedded by Go with `go:embed`.
- `internal/web/static/static.go` exposes embedded `dist`, `assets`, and `index.html`.

## Runtime Model

End users still run only `migate`. Node.js and npm are build-time tools for maintainers only.

Release builds run the frontend build first, then `go build` embeds `internal/web/static/dist` into the binary. The VPS installer and systemd service do not run `npm install` and do not require Node.

## Current WebUI Scope

The Vite + React panel is the only management interface. Go owns the API,
subscription output, embedded static files, configuration generation, and
system operations; the frontend owns screens, forms, validation feedback,
navigation, theme, language, and operator interaction state.

- Overview: server resources, inbound/client/outbound/routing counts, active/expired/limited client summary, total traffic, realtime traffic when Xray stats are available, Xray/sing-box status, manual refresh, request error notices, and recent config generation status.
- Inbounds: list, search, sort, create, edit, delete, enable/disable, client management, reset traffic, subscription copy, and advanced fields for VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, and ShadowTLS.
- Outbounds: default `direct` and `blocked` display, SOCKS5/HTTP/freedom/blackhole CRUD, enable/disable, ping, batch speed test, reorder, and SOCKS5 pool import with cache metadata.
- Routing: list, create, edit, delete, enable/disable via full PUT, reorder, inbound tag suggestions, `domain`, `ip`, `rule_set`, `protocol` match fields, and outbound options loaded from `/api/outbounds`.
- Core pages: Xray and sing-box status, version, config preview, structured config generation validation, apply/install/uninstall result details, install, uninstall, and logs. System-changing actions send `confirm` and `allow_system_changes`.
- Settings: panel port, username, password preservation when empty, base path, database path, Xray config path, service status, restart, TLS certificate status/issue, update check/status/update, and active session revoke.
- UI basics: dark/light theme persistence, Chinese/English navigation persistence, responsive sidebar, non-blocking toast, and modal confirmation for destructive operations.

## Routes

The backend keeps clear runtime boundaries:

- `/api/*` is handled by Go API routes and never falls through to the SPA.
- `/sub/*` remains public for client subscriptions.
- `/assets/*` serves Vite static assets from the embedded dist.
- `/`, `/login`, and other non-API paths fall back to `index.html` for React routing.
- `POST /login` is accepted as a compatibility login alias for deployments and tests that still post to the visible login URL; the React app uses `POST /api/login`.
- `web_base_path` such as `/panel` is handled by Go before routing, so `/panel`, `/panel/login`, `/panel/assets/*`, `/panel/api/*`, and `/panel/sub/*` keep working.

## Web API Additions

- `GET /api/xray/validate` builds the current Xray config from stored inbounds, outbounds, and routing rules without writing files, running core test commands, or restarting services. It returns `{target, valid, error, warnings, inbounds, outbounds, rules}`.
- `GET /api/singbox/validate` builds and marshals the current sing-box config without writing files, running core check commands, or restarting services. It returns the same structured validation shape.
- `GET /api/dashboard/summary` returns a read-only management summary for the React overview: inbound/client/outbound/routing counts, active/expired/limited clients, stored and realtime traffic totals, protocol distribution, and the current build-only validation results for Xray and sing-box.
- Routing rules now persist and return `ip` and `rule_set` fields in addition to `inbound_tag`, `domain`, `protocol`, `outbound_tag`, and `enabled`. `rule_set` is currently stored as a reserved field and is not emitted into Xray config because the supported Xray rule fields are still `domain`, `ip`, `inboundTag`, `protocol`, and outbound routing targets.

## Development

Install frontend dependencies:

```bash
make web-install
```

Run the frontend dev server:

```bash
make web-dev
```

Run the Go backend locally:

```bash
go run ./cmd/migate serve --config /path/to/panel.json
```

Build frontend assets:

```bash
make web-build
```

Build the final Go binary with embedded frontend:

```bash
make go-build
```

Run all checks:

```bash
make test
```

Focused checks used during WebUI development:

```bash
cd web && npm test -- --run
cd web && npm run build
go test ./...
```

Equivalent scripts are available in `scripts/build-web.sh`, `scripts/dev-web.sh`, and `scripts/check.sh`.

## Local Test Config

For local smoke tests, create a temporary config like:

```json
{
  "panel_port": 9999,
  "panel_username": "admin",
  "panel_password": "admin123",
  "web_base_path": "/panel",
  "database_path": "/tmp/migate-dev.db"
}
```

Then run:

```bash
npm --prefix web run build
go build ./cmd/migate
./migate serve --config /tmp/migate-panel.json
```

Open `http://127.0.0.1:9999/panel/`, log in with the configured account, and verify `/panel/api/session`, inbounds, outbounds, routing, Xray, sing-box, settings, and subscription paths.
