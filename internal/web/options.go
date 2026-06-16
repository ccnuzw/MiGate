package web

import (
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/singbox"
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

func WithSingboxRuntime(runtime SingboxRuntime) Option {
	return func(cfg *routerConfig) {
		if runtime != nil {
			cfg.singboxRuntime = runtime
		}
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

func WithHTTPPoolURL(poolURL string) Option {
	return func(cfg *routerConfig) {
		cfg.httpPoolURL = strings.TrimSpace(poolURL)
	}
}

func WithHTTPSPoolURL(poolURL string) Option {
	return func(cfg *routerConfig) {
		cfg.httpsPoolURL = strings.TrimSpace(poolURL)
	}
}

func WithUpdateCheckURL(checkURL string) Option {
	return func(cfg *routerConfig) {
		cfg.updateCheckURL = strings.TrimSpace(checkURL)
	}
}

func WithPublicHost(publicHost string) Option {
	return func(cfg *routerConfig) {
		cfg.publicHost = strings.TrimSpace(publicHost)
	}
}

func WithTrustedProxyHeaders(enabled bool) Option {
	return func(cfg *routerConfig) {
		cfg.trustProxy = enabled
	}
}

func WithLoginRateLimit(failureLimit int, cooldown time.Duration) Option {
	return func(cfg *routerConfig) {
		cfg.loginLimiter = newLoginLimiter(failureLimit, cooldown)
	}
}

// WithStatsClient sets the stats client for traffic statistics.
func WithStatsClient(client xray.StatsClient) Option {
	return func(cfg *routerConfig) {
		cfg.statsClient = client
	}
}

func WithSingboxStatsClient(client singbox.StatsClient) Option {
	return func(cfg *routerConfig) {
		cfg.singboxStatsClient = client
	}
}

// WithCoreScriptRunner injects the executor used by core install/uninstall
// endpoints. Tests use this to avoid running privileged system changes.
func WithCoreScriptRunner(runner func(script string) ([]byte, error)) Option {
	return func(cfg *routerConfig) {
		cfg.coreScriptRunner = runner
	}
}
