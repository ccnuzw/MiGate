# MiGate API Contract

MiGate API handlers live in `internal/web`. New endpoints must follow this contract.

## Naming

- Resources use plural names: `/api/inbounds`, `/api/outbounds`, `/api/routing-rules`.
- Operations use sub-paths: `/api/xray/apply`, `/api/singbox/restart`, `/api/update/check`.
- Status reads are `GET`: `/api/xray/status`, `/api/singbox/status`, `/api/service/status`.
- Dangerous operations are `POST` and require explicit confirmation fields.
- Certificate assets use `/api/certificates`; legacy `/api/cert/status` and
  `/api/cert/issue` are compatibility endpoints only.

## Success Responses

Simple command success:

```json
{
  "status": "ok"
}
```

Write operations should include stable operation metadata when applicable:

```json
{
  "status": "applied",
  "applied": true,
  "warnings": []
}
```

List responses should use a named collection field for new endpoints:

```json
{
  "items": [],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 0
  }
}
```

Existing endpoints that return arrays directly are legacy-compatible and should be migrated when the frontend caller is updated.

## Error Responses

API errors use one standard object:

```json
{
  "error": {
    "code": "invalid_json",
    "message": "Invalid JSON body",
    "detail": "optional operator-facing detail",
    "fields": {}
  }
}
```

`detail` and `fields` are omitted when empty. Error codes use lowercase snake case. `fields` contains structured field or operation metadata such as `lock_path` or `commands_executed`.

Top-level compatibility error fields have been removed. API callers must read `error.code`, `error.message`, `error.detail`, and `error.fields`.

Frontend callers must go through `web/src/api/client.ts`. The client only promotes the standard error object into `ApiError` fields: `message`, `code`, `status`, and optional details. Page code should format API failures with `getAPIErrorMessage` or `formatAPIError` and must not parse unknown payloads or compatibility error fields itself.

## Dangerous Operations

Any operation that changes system services, installs/uninstalls cores, restarts services, issues certificates, writes runtime config, or starts an update must:

- use `POST`
- require `confirm: true`
- require `allow_system_changes: true`
- return `confirmation_required` when either field is missing
- include `commands_executed` when commands are part of the response

Core install, uninstall, restart, and stop handlers delegate non-HTTP planning, script construction, command lists, and runner result shaping to `internal/service/coreadmin`. `internal/web/core_handlers.go` is a thin HTTP layer for method checks, JSON decoding, confirmation gates, service calls, and response writing.

All core management scripts execute through `internal/runtime/script`, not direct `exec.Command` calls from Web handlers. Core install follows the MiGate Runtime Contract paths and service names: `/etc/migate/cores/xray.json`, `/etc/migate/cores/sing-box.json`, `/var/lib/migate/backups`, `migate-xray`, and `migate-sing-box`.

Update start accepts the request, records runtime state, and then runs the updater with a detached background context capped at five minutes. Canceling the HTTP request after acceptance does not immediately cancel the background update command.

Update execution must remain aligned with CLI and installer semantics:

- `mg update` and Web update both execute the installer update path.
- `mg update vX.Y.Z` maps to `migate-install --update --version vX.Y.Z`.
- `mg update --check` maps to `migate-install --check` and is read-only.
- HTTP handlers must not duplicate release download, installer argument, or
  service restart logic; those rules live in the update service and installer.

Example:

```json
{
  "confirm": true,
  "allow_system_changes": true
}
```

## Certificate Management

Certificate management endpoints expose durable certificate assets and
operation diagnostics:

- `GET /api/certificates` returns `{ "certificates": [...] }`.
- `POST /api/certificates/preflight` validates domain syntax, email syntax,
  DNS resolution, HTTP-01 port 80 availability, cert directory writability, and
  core apply impact. It returns `{ "preflight": { "ok": true, "checks": [...] } }`.
- `POST /api/certificates` issues an HTTP-01 ACME certificate through the Go
  native issuer. It requires confirmation fields and returns the certificate
  plus preflight checks.
- `POST /api/certificates/import` imports `fullchain` and `private_key`,
  validates the key pair, parses SANs, validity, fingerprint, and serial, and
  stores files under `/etc/migate/certs`.
- `POST /api/certificates/{id}/apply` writes `tls_cert_file` and `tls_key_file`
  to selected TLS inbounds and returns Xray/sing-box apply summaries.
- `POST /api/certificates/{id}/delete` deletes an unused managed certificate
  record. Certificates still referenced by inbounds return `certificate_in_use`.
- `GET /api/certificates/{id}/operations` returns recent issue, import, renew,
  apply, and delete operation records.
- `POST /api/certificates/renew-due` checks ACME-managed certificates and
  renews those expiring within the requested threshold, defaulting to 30 days.

Stable certificate error codes include `domain_not_resolved`,
`http_01_port_unavailable`, `cert_dir_not_writable`, `acme_issue_failed`,
`invalid_domain`, `invalid_email`, `invalid_certificate`,
`certificate_key_mismatch`, `certificate_not_found`, and
`certificate_in_use`.

DNS-01 is reserved in the service model for a future provider-backed
implementation. This version intentionally does not hard-code a single DNS
provider.

## Status Responses

Status reads must be `GET`. Core status responses should include:

- `core`
- `installed`
- `managed`
- `service`
- `status`
- `service_status`
- `binary_path`
- `binary_version`
- `config_path`
- `config_exists`
- `config_valid`
- `commands_executed`

## Route Audit

`internal/web/routes_contract.go` owns the route table. The route table is the only API contract source for registered API routes: `router.go` calls `registerAPIRoutes(...)`, and `RouteContracts()` is derived from the same table.

Every API route entry records:

- method
- path
- auth policy
- CSRF policy
- handler name
- handler registration function

Tests verify declared critical routes are reachable through the real router, every `/api/` contract is registered by the real router, dangerous operations are `POST` plus CSRF-required, all write methods require CSRF, and all `GET` routes are CSRF-free.

## Migration Backlog

Current migration backlog:

- some list endpoints still return arrays directly
- `internal/web/response.go` is the only allowed direct `json.NewEncoder(w).Encode(...)` site for Web API responses
- direct exec exceptions are limited to runtime adapters

Architecture tests prevent new spread and make the remaining backlog visible.
