# syntax=docker/dockerfile:1.7

FROM node:25-bookworm AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/internal/web/static/dist ./internal/web/static/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migate ./cmd/migate

FROM debian:bookworm-slim

ARG TARGETARCH
ARG XRAY_VERSION=26.3.27
ARG SINGBOX_VERSION=1.13.13

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    iproute2 \
    procps \
    tar \
    unzip \
  && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
  case "${TARGETARCH}" in \
    amd64) xray_arch="64"; singbox_arch="amd64" ;; \
    arm64) xray_arch="arm64-v8a"; singbox_arch="arm64" ;; \
    *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
  esac; \
  tmp="$(mktemp -d)"; \
  curl -fsSL "https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VERSION}/Xray-linux-${xray_arch}.zip" -o "$tmp/xray.zip"; \
  unzip -q "$tmp/xray.zip" -d "$tmp/xray"; \
  install -m 0755 "$tmp/xray/xray" /usr/local/bin/xray; \
  mkdir -p /usr/local/share/xray; \
  [ -f "$tmp/xray/geoip.dat" ] && install -m 0644 "$tmp/xray/geoip.dat" /usr/local/share/xray/geoip.dat || true; \
  [ -f "$tmp/xray/geosite.dat" ] && install -m 0644 "$tmp/xray/geosite.dat" /usr/local/share/xray/geosite.dat || true; \
  curl -fsSL "https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-${singbox_arch}.tar.gz" -o "$tmp/sing-box.tar.gz"; \
  tar -xzf "$tmp/sing-box.tar.gz" -C "$tmp"; \
  install -m 0755 "$tmp/sing-box-${SINGBOX_VERSION}-linux-${singbox_arch}/sing-box" /usr/local/bin/sing-box; \
  rm -rf "$tmp"

COPY --from=go-build /out/migate /usr/local/bin/migate
COPY packaging/docker-entrypoint.sh /usr/local/bin/migate-docker-entrypoint
COPY packaging/docker-systemctl.sh /usr/local/bin/systemctl
RUN ln -sf /usr/local/bin/migate /usr/local/bin/mg \
  && chmod +x /usr/local/bin/migate-docker-entrypoint /usr/local/bin/systemctl \
  && mkdir -p /etc/migate/cores /etc/migate/certs /var/lib/migate /var/log/migate /run/migate

EXPOSE 9999 20000-20050 21000-21050
VOLUME ["/etc/migate", "/var/lib/migate", "/var/log/migate", "/run/migate"]

ENTRYPOINT ["/usr/local/bin/migate-docker-entrypoint"]
CMD ["migate", "serve", "--host", "0.0.0.0", "--config", "/etc/migate/panel.json"]
