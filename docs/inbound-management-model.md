# Inbound Management Model

MiGate uses a backend-owned inbound capability model as the authority for
protocol ownership, valid transport/security combinations, credentials, config
generation, and share-link availability.

The authoritative capability table lives in
`internal/db/inbound_capability.go` and is exposed through
`GET /api/inbound-capabilities`. The Web UI loads that endpoint at runtime and
uses the returned capabilities to drive protocol choices, network/security
choices, defaults, advanced fields, credential fields, subscription/share
availability, and local-proxy inbound marking. A local fallback matrix remains
only to keep the page usable when the capability endpoint is unavailable.

## Core Ownership

| Protocol | Core |
| --- | --- |
| `vless`, `vmess`, `trojan`, `shadowsocks`, `socks`, `http` | Xray |
| `hysteria2`, `tuic`, `shadowtls` | sing-box |

Users choose an inbound protocol. The core is derived from the protocol and is
not an editable user field.

## Capability Matrix

| Protocol | Network | Security | Default | Share link |
| --- | --- | --- | --- | --- |
| `vless` | `tcp`, `ws`, `grpc`, `h2`, `xhttp` | `none`, `tls`; `reality` only on `tcp`, `grpc`, `xhttp` | `tcp` + `reality` | yes |
| `vmess` | `tcp`, `ws`, `grpc`, `h2`, `xhttp` | `none`, `tls` | `tcp` + `tls` | yes |
| `trojan` | `tcp`, `ws`, `grpc`, `h2`, `xhttp` | `none`, `tls`; `reality` only on `tcp`, `grpc`, `xhttp` | `tcp` + `tls` | yes |
| `shadowsocks` | `tcp` | `none` | `tcp` + `none` | yes |
| `socks` | `tcp` | `none` | `tcp` + `none` | no |
| `http` | `tcp` | `none` | `tcp` + `none` | no |
| `hysteria2` | `udp` | `tls` | `udp` + `tls` | yes |
| `tuic` | `udp` | `tls` | `udp` + `tls` | yes |
| `shadowtls` | `tcp` | `none` | `tcp` + `none` | no |

`socks` and `http` are supported as local/proxy inbound protocols. They can be
created, edited, validated, and emitted into Xray config, but they do not expose
client subscription or share links.

## Client Credentials

The database still keeps `clients.uuid` for continuity, but the business model
uses protocol-specific credential semantics:

| Protocol | Client credential |
| --- | --- |
| `vless`, `vmess` | UUID |
| `trojan` | password |
| `shadowsocks` | no client-level credential; the inbound password is `inbounds.uuid` |
| `socks`, `http` | username + password |
| `hysteria2` | password |
| `tuic` | UUID (`credential_id`) + password |
| `shadowtls` | password |

API responses include `credential_id` and `password` for protocols that need
them. The Web UI labels fields by protocol instead of showing a single generic
UUID field.

Creating an inbound with `initial_client` is atomic: the inbound row and the
initial client row commit together. If the initial client credentials fail the
target protocol validation or collide with an existing client credential, neither
row is persisted. Automatic port allocation, REALITY key persistence,
subscription token generation, and stats key generation remain part of the
normal create flow.

Changing an existing inbound protocol validates every existing client under that
inbound against the target protocol credential type before the update is written.
MiGate rejects the protocol change if any client cannot be interpreted by the
target protocol, and the error includes both the target protocol and the client
label. MiGate does not silently derive or fill hidden credential IDs/passwords
during a protocol change. Updating fields without changing the protocol keeps
the existing update behavior.

## REALITY Key Lifecycle

REALITY private/public keys are generated before an inbound is persisted when
they are missing. The public key is derived from the stored private key when
possible. Updates preserve existing REALITY keys and short IDs unless the user
explicitly changes them.

Config generation does not create transient REALITY keys. Subscription `pbk`
values come from the persisted `reality_public_key` field.

## TLS And SNI Fields

TLS uses `tls_sni`. REALITY uses `reality_server_names`. Hysteria2 and TUIC TLS
SNI also use `tls_sni`.

ShadowTLS uses `tls_sni` as the handshake server field in the current schema,
not as a generic TLS SNI setting. The UI labels this field as the handshake
server for ShadowTLS. The generated sing-box `handshake.server` comes from
`tls_sni`; the handshake port is currently fixed to `443` and is not exposed as
an editable field.

## Xray Transports

The supported Xray inbound transports are `tcp`, `ws`, `grpc`, `h2`, and
`xhttp`.

Generated stream settings:

| Network | Xray stream field |
| --- | --- |
| `tcp` | no extra settings |
| `ws` | `wsSettings` |
| `grpc` | `grpcSettings` |
| `h2` | `httpSettings` |
| `xhttp` | `xhttpSettings` |

`quic` and `kcp` are intentionally not exposed until their generator behavior
and tests are complete.

## sing-box Inbounds

Hysteria2 always emits TLS because sing-box requires TLS for server inbounds.
`users.password` comes from the client password. `up_mbps`, `down_mbps`, `obfs`,
and `obfs_password` are emitted from inbound fields.

TUIC emits `users.uuid` from `credential_id` and `users.password` from
`password`. `congestion_control` and `zero_rtt_handshake` are emitted from
inbound fields. TLS is required.

ShadowTLS is currently limited to v3. It uses per-user passwords and requires
`tls_sni` as the handshake server. The generated handshake port is fixed to
`443`.

## Subscription And Share UI

Protocols with `subscription: full` expose both "copy subscription link" and
"copy share link" actions after saving a client. Protocols with
`subscription: none` do not show those copy actions in the inbound card or the
client-save success panel; the UI instead states that the protocol does not
support subscription/share links. The backend subscription handler still returns
a clear unsupported error if a caller manually requests `/sub/{token}` for an
unsupported protocol.

## Current Non-Supported Capabilities

- Xray inbound `quic` and `kcp` transports.
- ShadowTLS subscription/share links.
- Multi-user Shadowsocks inbound credentials. Shadowsocks remains an
  inbound-level password model.
