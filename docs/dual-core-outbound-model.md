# Dual-Core Inbound and Shared Outbound Model

MiGate manages Xray and sing-box from one panel, but it does not bridge traffic
between cores. The panel stores logical resources and each config generator
compiles only the resources that belong to its core.

## Inbound Core Assignment

Inbound core is derived from protocol:

- `hysteria2`, `tuic`, `shadowtls` use sing-box.
- All other inbound protocols use Xray, including `vless`, `vmess`, `trojan`,
  `shadowsocks`, `socks`, and `http`.

Users choose an inbound protocol, not a core. Clients inherit the core of their
parent inbound.

## Shared Outbound Profiles

Outbounds are shared `OutboundProfile` records. They are not Xray outbound
objects and they are not sing-box outbound objects. The core-specific outbound
tags are generated only when compiling a config.

`supported_cores` is a derived display field. It is not stored in the database
and is not accepted as user configuration. The backend computes it from
`outbound.protocol` for API responses, routing validation, topology checks, and
config generation.

Protocol support:

- Shared by Xray and sing-box: `socks`, `socks5`, `http`, `https`, `vless`,
  `trojan`, `shadowsocks`, `freedom`, `blackhole`, `dns`, `direct`, `block`.
- sing-box only: `hysteria2`, `tuic`, `shadowtls`.

## Support Levels

The support level describes how complete the current profile model is:

- `builtin`: `freedom`, `blackhole`, `dns`, `direct`, `block`.
- `full`: `socks`, `socks5`, `http`, `https`.
- `basic`: `vless`, `trojan`, `shadowsocks`, `hysteria2`, `tuic`,
  `shadowtls`.

Basic protocols currently store only core connection parameters. Advanced
transport, TLS, REALITY, and other protocol-specific fields are intentionally
not modeled yet.

Credential fields:

- `vless`: UUID in `username`; `password` is hidden and cleared.
- `tuic`: UUID in `username` plus `password`.
- `shadowsocks`: method in `username` plus `password`.
- `trojan`, `hysteria2`, `shadowtls`: `password`.
- `socks`, `http`, `https`: `username` plus `password`.

## Routing Targets

Routing rules use `outbound_id` as the authoritative target. `outbound_tag` is
only a display snapshot and a fallback for manually constructed rules.

When creating or updating a route, the API resolves the selected outbound and
saves both:

- `outbound_id`: stable profile ID used by validation and config generation.
- `outbound_tag`: profile tag at the time the route was saved.

If an outbound profile is renamed, existing routes keep their snapshot tag but
continue to target the same profile through `outbound_id`.

## Generated Core Tags

Config generators instantiate core-specific outbound tags:

- Xray: `xray-out-{outbound_id}`
- sing-box: `singbox-out-{outbound_id}`

The same shared SOCKS, HTTP, HTTPS, VLESS, Trojan, or Shadowsocks profile can be
compiled into both configs with different generated tags. sing-box-only profiles
are not emitted into Xray configs.

Routes are compiled by resolving `outbound_id` first, falling back to
`outbound_tag` only when no ID is present. The generator then checks whether the
profile supports the current core before emitting the route.

## Topology and Traffic

The topology canvas displays Xray inbounds, sing-box inbounds, clients, and
shared outbound profiles together. Outbound nodes show their derived supported
cores and support level. Edges are valid only when the source core is supported
by the target outbound profile.

Traffic stats from generated tags are mapped back to the shared outbound
profile ID. If both cores report traffic for one profile, the API aggregates
upload, download, and rates, sets `traffic_engine` to `mixed`, and returns all
contributors in `traffic_engines`.
