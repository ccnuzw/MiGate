# MiGate Install Guide

MiGate ships as a Go single binary with the WebUI embedded. The installer is
intended for Linux VPS hosts and focuses on repeatable install, upgrade, repair,
core detection, and safe config handling.

## One-Click Install

```bash
bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh)
```

Install a specific release:

```bash
MIGATE_VERSION=v1.0.21 bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh)
```

The interactive flow prints these stages:

- `环境检测`: OS, architecture, root, systemd, and command dependencies.
- `已安装检测`: binary, CLI link, systemd service, config dir, `panel.json`, database, running process, and panel port.
- `配置确认`: first install creates `panel.json`; existing config is kept unless you choose fresh config.
- `核心检测`: Xray and sing-box binary paths, versions, and services.
- `安装 MiGate`: release asset download, checksum verification, binary replacement, config, and service unit.
- `服务启动`: daemon reload, enable, restart, and status check when systemd exists.
- `完成`: WebUI URL, username, password behavior, paths, service commands, and logs.

## Existing Installs

When MiGate is already detected, the installer shows:

```text
1) 升级并保留配置
2) 重装并保留配置
3) 重装并重新生成配置
4) 只修复 systemd 服务
5) 只安装/修复 Xray
6) 只安装/修复 sing-box
7) 卸载
8) 退出
```

By default, existing `/etc/migate/panel.json` is preserved. Choose fresh config
only when you explicitly want a new panel password, port, username, and paths.

## Non-Interactive Mode

```bash
migate-install --install --yes
migate-install --upgrade --yes
migate-install --reinstall --yes
migate-install --reinstall --fresh-config --yes
migate-install --repair-service --yes
migate-install --install-xray --yes
migate-install --install-singbox --yes
migate-install --uninstall --yes
```

`--install-xray` and `--install-singbox` by themselves are core-only repair
modes. They do not install or upgrade the MiGate panel unless they are combined
with `--install`, `--upgrade`, or `--reinstall`.

Preview without changing the system:

```bash
migate-install --install --yes --dry-run
migate-install --upgrade --yes --dry-run
migate-install --uninstall --yes --dry-run
migate-install --install-xray --yes --dry-run
migate-install --install-singbox --yes --dry-run
```

`--dry-run` prints the commands that would be run and does not install binaries,
write config, stop services, or remove files.

## Config Paths

Default paths:

```text
Binary:        /usr/local/bin/migate
CLI alias:     /usr/local/bin/mg
Installer:     /usr/local/bin/migate-install
Uninstaller:   /usr/local/bin/migate-uninstall
Config:        /etc/migate/panel.json
Database:      /usr/local/migate/migate.db
Xray config:   /usr/local/migate/xray.json
Web base path: /panel
```

First install writes a random password when the password prompt is left blank.
The generated password is printed once at the end of installation and is not
stored anywhere except `panel.json`.

## systemd

The installer writes or repairs:

```text
/etc/systemd/system/migate.service
```

The generated service binds the panel to `0.0.0.0` by default for VPS panel-style
access. For production use, set a strong password and prefer publishing through a
reverse proxy with HTTPS. Use
`public_host` in `/etc/migate/panel.json` to control the host embedded in
subscription share links. If the proxy terminates HTTPS, set `trust_proxy` to
`true` only when MiGate is reachable exclusively through that trusted proxy, so
the service can honor `X-Forwarded-Proto` for Secure cookies and HSTS. Passing
`migate serve --host 0.0.0.0` is still supported for explicit deployments that
accept that exposure.

Useful commands:

```bash
systemctl status migate
systemctl restart migate
journalctl -u migate -f
mg status
mg doctor
mg logs -f
mg restart
```

If systemd is unavailable, the installer skips service creation and prints a
manual command:

```bash
/usr/local/bin/migate serve --config /etc/migate/panel.json
```

## Xray and sing-box

The installer checks:

```text
/usr/local/bin/xray
/usr/bin/xray
/usr/local/bin/sing-box
/usr/bin/sing-box
xray.service
migate-singbox.service
```

Version commands:

```bash
xray version
sing-box version
```

Xray installation downloads the official XTLS install script and asks for
confirmation before executing it in interactive mode. In dry-run mode, the
download and execution commands are printed instead.

sing-box installation downloads the configured release archive, downloads
checksums, verifies the archive before extracting, installs
`/usr/local/bin/sing-box`, and writes `migate-singbox.service`.

MiGate and sing-box release archives are installed only after checksum
verification. Xray and acme.sh still rely on their official upstream installer
scripts, so treat those actions as privileged system changes and keep the WebUI
behind trusted administrator access.

Skipping core installation does not block the panel installation. Xray-backed
protocols or sing-box-backed protocols may not listen until the relevant core is
installed and configuration is applied.

## Uninstall

Default uninstall removes MiGate services and binaries while keeping config and
data:

```bash
mg uninstall --yes
# or
migate-install --uninstall --yes
```

Purge config/data only when you explicitly intend to delete them:

```bash
migate-uninstall --purge --yes
```

The uninstaller does not remove third-party Xray itself by default.

## Troubleshooting

Check local diagnostics:

```bash
mg doctor
```

Check service logs:

```bash
journalctl -u migate -n 80 --no-pager
journalctl -u migate -f
```

Common issues:

- Port already in use: choose a different `panel_port` or stop the conflicting service.
- systemd unavailable: run MiGate manually or configure your platform's service manager.
- Core skipped or missing: install Xray or sing-box and apply config from the WebUI Core page.
- Lost generated password: run `mg reset-password` on the server.
