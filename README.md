# MiGate

MiGate is a **Go single-binary** lightweight VPS panel that uses local SQLite and embedded WebUI to manage Xray inbounds and clients.

Currently suitable for users familiar with VPS/Xray for testing.

## Features

- Single binary deployment, no Python/Node runtime required
- React WebUI for inbounds, clients, outbounds, SOCKS5 pool import, routing, Xray/sing-box core config, TLS certificates, settings, sessions, and updates
- Local SQLite database
- Generate and apply Xray configuration
- Supported inbound protocols: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, ShadowTLS
- systemd service management

## One-Click Install

Run as root on Linux VPS:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh)
```

Install specific version:

```bash
MIGATE_VERSION=v1.0.21 bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh)
```

The installer runs an environment check, detects existing MiGate installs, keeps
`/etc/migate/panel.json` by default, checks Xray and sing-box, and then asks what
to do. On an existing host it can upgrade, reinstall, regenerate config, repair
systemd, install/repair cores, uninstall, or exit.

Common non-interactive commands:

```bash
# Install or upgrade while keeping existing config.
bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh) --install --yes

# Preview install actions without changing the system.
bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh) --install --yes --dry-run

# Upgrade to latest release and keep config.
migate-install --upgrade --yes

# Repair the systemd unit only.
migate-install --repair-service --yes

# Install or repair runtime cores only. This does not install MiGate itself.
migate-install --install-xray --yes
migate-install --install-singbox --yes

# Uninstall service and binaries while keeping config/data.
migate-install --uninstall --yes
```

During first installation, you will be prompted for:

- Panel port, default `9999`
- Username, default `admin`
- Password, leave empty for auto-generated random password
- Web path, default `/panel`
- Whether to install/repair Xray and sing-box

After installation, access:

```text
http://SERVER_IP:9999/panel
```

## Common Commands

Check status:

```bash
systemctl status migate
```

Restart panel:

```bash
systemctl restart migate
```

View logs:

```bash
journalctl -u migate -f
```

Config file:

```text
/etc/migate/panel.json
```

Database:

```text
/usr/local/migate/migate.db
```

Xray config:

```text
/usr/local/migate/xray.json
```

More details: `docs/install.md`.

## Development

The WebUI source lives in `web/` and builds into `internal/web/static/dist`, which is embedded into the Go binary.

```bash
make web-install   # install frontend dependencies
make web-dev       # start Vite dev server
make web-build     # build embedded frontend dist
make go-build      # build final migate binary
make test          # run frontend and Go tests
```

Node/npm are only build-time tools for contributors and release automation. The one-click installer and VPS runtime continue to use the Go single binary only.

See `docs/frontend-refactor.md` for the split architecture and route compatibility details.

## Note

MiGate currently focuses on single-machine VPS scenarios and is still under rapid iteration. It is recommended to test on a test VPS before using it for long-term services.
