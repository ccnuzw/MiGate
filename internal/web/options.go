package web

import (
	"strings"

	"github.com/imzyb/MiGate/internal/xray"
)

type Option func(*routerConfig)

func WithStore(store Store) Option {
	return func(cfg *routerConfig) {
		cfg.store = store
	}
}

func WithVersion(version string) Option {
	return func(cfg *routerConfig) {
		cfg.version = version
	}
}

func WithXrayController(controller XrayController) Option {
	return func(cfg *routerConfig) {
		cfg.xrayController = controller
	}
}

func WithConfigDir(dir string) Option {
	return func(cfg *routerConfig) {
		cfg.configDir = dir
	}
}

func WithBasePath(basePath string) Option {
	return func(cfg *routerConfig) {
		cfg.basePath = normalizeBasePath(basePath)
	}
}

func WithSocks5PoolURL(poolURL string) Option {
	return func(cfg *routerConfig) {
		cfg.socks5PoolURL = strings.TrimSpace(poolURL)
	}
}

func WithUpdateCheckURL(checkURL string) Option {
	return func(cfg *routerConfig) {
		cfg.updateCheckURL = strings.TrimSpace(checkURL)
	}
}

// WithStatsClient sets the stats client for traffic statistics.
func WithStatsClient(client xray.StatsClient) Option {
	return func(cfg *routerConfig) {
		cfg.statsClient = client
	}
}
