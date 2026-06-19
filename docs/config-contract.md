# MiGate Config Contract

Panel configuration lives at `/etc/migate/panel.json`, is owned by
`internal/config.Config`, and is part of the MiGate Runtime Contract.

## strict schema

MiGate has not shipped a stable public config API yet, so the schema is strict:

- `internal/config.Config` is the only field source.
- `Load(path)` rejects unknown JSON fields.
- `Save(path, cfg)` writes only known `Config` fields.
- `Update(path, mutate)` loads typed `Config`, validates it, normalizes write defaults, and saves typed JSON.
- Settings and cert APIs must not preserve, pass through, or create unknown fields with arbitrary maps.
- `internal/service/cert` saves `cert_domain` and `cert_email` through typed settings/config persistence after successful issue.

## Fields

| JSON field | Go field | Default on Save | Validation |
| --- | --- | --- | --- |
| `panel_port` | `PanelPort` | `9999` | `1..65535`; explicit `0` in existing config is invalid |
| `panel_username` | `PanelUsername` | empty | trimmed |
| `panel_password` | `PanelPassword` | empty | stored as an Argon2id MiGate hash when updated through settings |
| `web_base_path` | `WebPath` | `/panel` | trimmed, leading slash added, trailing slash removed except `/` |
| `public_host` | `PublicHost` | empty | trimmed |
| `trust_proxy` | `TrustProxy` | `false` | boolean |
| `database_path` | `DatabasePath` | `/var/lib/migate/migate.db` | required on save, absolute path, no NUL byte |
| `cert_domain` | `CertDomain` | empty | trimmed; cert issue validates domain syntax before saving |
| `cert_email` | `CertEmail` | empty | trimmed; cert issue validates email syntax before saving |

## Read Versus Write Defaults

`Load(path)` normalizes existing string fields but does not inject omitted write defaults. This lets callers distinguish an old partial config from a newly saved full config.

`Normalize(cfg)` and `Save(path, cfg)` apply write defaults before persisting. A saved config should not contain unknown fields and should include normalized paths and defaults.

## Password Handling

The settings service never returns `panel_password`. Settings `GET` returns `has_password` only. Settings `PUT` hashes a non-empty plaintext password before saving; if a valid MiGate hash is supplied, it is preserved as a hash.
