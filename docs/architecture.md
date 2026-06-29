# MiGate Architecture

MiGate keeps runtime behavior behind stable package boundaries. New code must follow these ownership rules.

## Package Boundaries

- `cmd/migate` only starts the process, parses CLI flags, selects CLI/server mode, and assembles dependencies.
- `internal/web` only owns HTTP routing, request decoding, authentication/CSRF middleware, and response writing.
- `internal/config` owns panel configuration shape, defaults, loading, saving, validation, and normalization.
- `internal/runtime` owns system command execution, systemd/journalctl integration, file operations, and host runtime adaptation.
- `internal/db` only owns persistence and database migrations/queries.
- `internal/xray` and `internal/singbox` only own core configuration generation, validation, stats/capability probing, and core-specific model translation.
- `internal/paths` owns the MiGate Runtime Contract constants for paths, service names, binaries, data, logs, run files, and locks.

The MiGate Runtime Contract remains:

- panel config: `/etc/migate/panel.json`
- certificate assets: `/etc/migate/certs`
- core configs: `/etc/migate/cores/xray.json`, `/etc/migate/cores/sing-box.json`
- database: `/var/lib/migate/migate.db`
- services: `migate`, `migate-xray`, `migate-sing-box`
- panel config permissions: `0640`, owned by `root:migate` when running as root

## Configuration

Panel configuration is centralized in `internal/config`.

- `Config` is the only Go struct for `panel.json`.
- `Default()` is the only source for install/create defaults.
- `Load(path)` reads existing config without inventing omitted server behavior.
- `Load(path)` uses a strict schema and rejects unknown fields.
- `Normalize(cfg)` applies defaults for write/create paths.
- `Validate(cfg)` rejects invalid persisted config.
- `Save(path, cfg)` writes through `internal/panelconfig.WriteFile`, preserving `0640` and `root:migate`.
- `Update(path, mutate)` is typed; settings and cert updates must not use arbitrary `map[string]interface{}` persistence.

`cmd/migate` and `internal/web/settings.go` must not write `panel.json` directly.

The field-level contract is documented in `docs/config-contract.md`.

## Runtime Commands

System commands go through runtime adapters.

- `CommandRunner` is the testable interface.
- `RealCommandRunner` uses context-aware execution.
- `Run` and `RunOutput` are package-level defaults.
- command output is bounded by a common truncation limit before it can enter API responses or logs.
- `internal/runtime/script` owns bash stdin script execution for core install/uninstall/service-control workflows.
- Core scripts run as `bash -s`; when `systemd-run` and `/run/systemd/system` are available, the script adapter uses `systemd-run --wait --pipe` with a root oneshot unit, timeout, stdin forwarding, combined stdout/stderr, and bounded output.

Direct exec exceptions are explicitly guarded:

- `internal/runtime/command/command.go` owns generic command execution and `LookPath`.
- `internal/runtime/script/script.go` owns bash stdin and `systemd-run --pipe` execution for core scripts.
- tests may use `exec.Command`.

`internal/web`, `internal/xray`, `internal/singbox`, and `cmd/migate` must not call `exec.Command`, `exec.CommandContext`, or `exec.LookPath` directly. Xray stats probing, sing-box capability checks, sing-box manager operations, and WebUI core script execution are all behind runtime adapters.

Xray REALITY key generation and public-key derivation use local X25519 crypto and no longer call the `xray x25519` CLI.

## MiGate Ops Contract

MiGate operations use one MiGate Runtime Contract across CLI, Web services,
installer, uninstaller, tests, and docs.

Standard paths:

- config dir: `/etc/migate`
- panel config: `/etc/migate/panel.json`
- core config dir: `/etc/migate/cores`
- certificate dir: `/etc/migate/certs`
- Xray config: `/etc/migate/cores/xray.json`
- sing-box config: `/etc/migate/cores/sing-box.json`
- data dir: `/var/lib/migate`
- database: `/var/lib/migate/migate.db`
- versions file: `/var/lib/migate/versions.json`
- backup dir: `/var/lib/migate/backups`
- log dir: `/var/log/migate`
- run dir: `/run/migate`
- installer lock: `/run/migate/install.lock`
- binaries: `/usr/local/bin/migate`, `/usr/local/bin/mg`,
  `/usr/local/bin/migate-install`, `/usr/local/bin/migate-uninstall`,
  `/usr/local/bin/xray`, `/usr/local/bin/sing-box`

The `migate.service` sandbox uses `ProtectSystem=strict` and grants
`ReadWritePaths=/etc/migate ...`; therefore managed certificate files must live
under `/etc/migate/certs`, not `/etc/xray/certs`. Xray and sing-box read those
paths from generated inbound TLS configuration.

Standard services:

- panel: `migate`
- Xray: `migate-xray`
- sing-box: `migate-sing-box`

Standard CLI commands:

- read-only checks: `mg status`, `mg doctor`, `mg info`, `mg ports`,
  `mg logs`, `mg url`, `mg url --public`, `mg update --check`,
  `mg version`, `mg hash-password`
- dangerous writes: `mg start`, `mg stop`, `mg restart`, `mg restart all`,
  `mg reset-password`, `mg update`, `mg update vX.Y.Z`, `mg backup`,
  `mg restore <file>`, `mg uninstall`

Exit codes are stable across CLI operations:

- `0`: command completed successfully
- `1`: runtime, filesystem, service, validation, update, backup, restore, or
  installer/uninstaller failure
- `2`: command-line usage or unsupported CLI language

Dangerous operation confirmation:

- Web API dangerous operations must be `POST` and require both `confirm: true`
  and `allow_system_changes: true`.
- CLI dangerous operations are explicit subcommands. Non-interactive installer
  operations use `--yes`; uninstall asks for one of three scopes when no mode is
  supplied: panel-only, panel+managed-cores, or full removal including all
  MiGate config/data/logs. Non-interactive uninstall requires an explicit scope
  for the uninstaller itself (`--panel-only`, `--with-cores`, or `--purge`),
  while `migate-install --uninstall --yes` preserves backward-compatible
  panel-only behavior.
- Installer dry-runs must not write real system paths. When the default lock is
  `/run/migate/install.lock`, dry-run uses a temporary lock path.

Backup and restore scope:

- `mg backup` defaults to `/var/lib/migate/backups/migate-backup-YYYYMMDD-HHMMSS.tar.gz`.
- Backup archives include only `/etc/migate`,
  `/var/lib/migate/migate.db`, and `/var/lib/migate/versions.json` by default.
- Backup archives do not include `/run/migate` or unrelated system files.
- `mg restore <file>` extracts to `/` and restarts `migate`. Core services use
  their standard names and can be restarted with `mg restart all` when needed.

Update semantics:

- CLI `mg update` delegates to `migate-install --update`.
- CLI `mg update vX.Y.Z` delegates to
  `migate-install --update --version vX.Y.Z`.
- CLI `mg update --check` delegates to `migate-install --check` and is read-only.
- Web update service delegates update execution to the same installer entrypoint
  through the service layer; handlers do not own update logic.

## Web/API Boundary

`internal/web` must use the shared response helpers in `response.go`:

- `WriteJSON`
- `WriteError`
- `DecodeJSONBody`
- compatibility wrappers `writeJSON`, `writeJSONError`, and `decodeJSONBody`

Handlers must not call `json.NewEncoder(w).Encode(...)` directly. After the third-round API cleanup, `internal/web/response.go` is the only allowed direct `json.NewEncoder(w).Encode(...)` site in non-test Web code.

The frontend API boundary is `web/src/api`:

- `web/src/api/client.ts` is the only browser HTTP client entrypoint. It owns `fetch`, base-path handling, credentials, JSON/text decode, unauthorized redirect behavior, and `ApiError`.
- `web/src/api/types.ts` owns shared API DTOs consumed by pages.
- Domain API modules own endpoint strings and response unwrapping: `session.ts`, `core.ts`, `inbounds.ts`, `outbounds.ts`, `routing.ts`, `settings.ts`, and `traffic.ts`.
- `web/src/api/endpoints.ts` is a thin compatibility aggregator over those modules.
- Route/page components must not call `fetch` directly and must not embed `/api/` or `/sub/` endpoint strings.
- UI-facing API error messages must use the standard error object through `getAPIErrorMessage`/`formatAPIError`; frontend code must not read removed compatibility fields such as top-level message or legacy error shapes.
- Mutations that affect topology or traffic views refresh through `web/src/lib/queryInvalidation.ts`; explicit query refresh calls should use its helpers rather than open-coded page-local loops.

## Route Contracts

`internal/web/routes_contract.go` owns the Route table used by `router.go` to register API routes. `RouteContracts()` is derived from this same table, so the route table is the API contract source rather than a sidecar list.

- method
- path
- auth policy
- CSRF policy
- handler name
- handler registration function

Non-API routes such as static assets, `/login`, `/sub/`, and the SPA fallback remain registered in `router.go`.

## Database Layer

`internal/db` is the persistence boundary. It owns SQLite schema initialization, migrations, seed data, and repository-style methods for MiGate domain data. It must not depend on `internal/web`, must not know about HTTP response writing, and must preserve the public `Store` method contracts used by the Web and service layers.

The database package is split by responsibility:

- `store.go` owns `Store`, `Open`, `Close`, SQLite connection setup, foreign-key verification, and package-wide token/UUID helpers.
- `schema.go` owns `migrate`, `CREATE TABLE`, `CREATE INDEX`, PRAGMA/schema initialization, and schema-compatible migrations.
- `models.go` owns exported persistence models and create/update parameter structs such as `Inbound`, `Client`, `Outbound`, `RoutingRule`, and traffic DTOs.
- `outbounds.go`, `routing.go`, `inbounds.go`, and `clients.go` own repository methods for those domain tables, including validation and lookup helpers that are tied to persistence.
- `traffic.go` owns `traffic_states`, `traffic_samples`, raw stat application, traffic reset baselines, cleanup throttling, and traffic usage queries.
- `sessions.go` owns `token_blacklist` and active session persistence.
- `sqlutil.go` owns small shared SQL helpers such as placeholders, nullable values, and common query/exec interfaces.

Schema behavior is part of the API contract for existing installations: table names, column names, indexes, defaults, foreign keys, and uniqueness constraints should only change through explicit compatible migrations. The db layer returns Go values and errors; `internal/web/response.go` remains responsible for translating those outcomes into HTTP status codes and JSON responses.

## Service Layer

The service layer keeps business logic outside HTTP handlers. Handlers should only check methods, decode requests, enforce confirmation gates, call services, and write responses.

- `internal/service/settings` owns typed settings reads/updates and password hashing orchestration.
- `internal/service/cert` owns certificate preflight diagnostics, ACME HTTP-01 issuance through a Go native issuer, import validation, certificate metadata parsing, renewal decisions, operation logs, and applying managed certificate paths to TLS inbounds.
- `internal/service/update` owns release checks, updater availability validation, update runtime state, update logs, and detached `systemd-run` updater execution.
- `internal/service/coreadmin` owns core install, uninstall, restart, and stop management logic. It builds the plans, shell scripts, command lists, service-name resolution, runner invocation, and `ActionResult` shaping for those workflows.
- `internal/web/core_handlers.go` is a thin HTTP layer for core management actions: method checks, JSON decoding, confirmation gates, service calls, and response writing.
- Core install scripts still execute through `internal/runtime/script` and follow the MiGate Runtime Contract paths and services: `/etc/migate/cores/xray.json`, `/etc/migate/cores/sing-box.json`, `/var/lib/migate/backups`, `migate-xray`, and `migate-sing-box`.

Service packages must not depend on `http.ResponseWriter` or inbound handler types. `internal/service/update` may use `net/http` and `*http.Request` only as an outbound GitHub release-check client.

Update start requests validate and accept work in the request path, then create a detached background context with a five-minute timeout for the `systemd-run` command. Request cancellation does not immediately kill an accepted update command.

## Architecture Guards

`architecture/architecture_test.go` guards:

- MiGate Runtime Contract paths and service names
- route table as the API contract source
- direct command execution outside approved runtime/core boundaries
- direct `panel.json` writes outside `internal/config` and `internal/panelconfig`
- direct Web JSON responses outside `internal/web/response.go`
- `internal/db` schema and repository logic remains split across schema, traffic, routing, session, inbound, client, and outbound files
- no direct command execution from `internal/web`
- no HTTP handler dependencies from `internal/service`
- core install script markers live in `internal/service/coreadmin`, not `internal/web/core_handlers.go`
- strict settings/config persistence
- frontend API calls stay behind `web/src/api`, endpoint strings stay in API modules, and removed legacy error markers stay absent
- presence of this architecture document, the API contract document, and the config contract document
