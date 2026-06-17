package web

import (
	"net/http"
	"strings"
)

func NewRouter(options ...Option) http.Handler {
	cfg := routerConfig{
		xrayController:   defaultXrayController{},
		singboxRuntime:   defaultSingboxRuntime{},
		socks5PoolURL:    defaultSocks5PoolURL,
		httpPoolURL:      defaultHTTPPoolURL,
		httpsPoolURL:     defaultHTTPSPoolURL,
		updateCheckURL:   defaultUpdateCheckURL,
		loginLimiter:     newLoginLimiter(defaultLoginFailureLimit, defaultLoginCooldown),
		coreScriptRunner: runCoreScript,
	}
	for _, option := range options {
		option(&cfg)
	}
	mux := http.NewServeMux()
	mux.Handle("/assets/", staticAssetsHandler())
	mux.HandleFunc("/login", loginPageHandler(&cfg))
	mux.HandleFunc("/api/login", loginHandler(&cfg))
	mux.HandleFunc("/api/logout", logoutHandler(&cfg))
	mux.HandleFunc("/api/session", sessionHandler(&cfg))
	mux.HandleFunc("/api/sessions", sessionsListHandler(&cfg))
	mux.HandleFunc("/api/sessions/", sessionRevokeHandler(&cfg))
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/inbound-capabilities", inboundCapabilitiesHandler)
	mux.HandleFunc("/api/reality/keypair", realityKeypairHandler)
	mux.HandleFunc("/api/inbounds", inboundsHandler(cfg.store, cfg.xrayController, cfg.statsClient))
	mux.HandleFunc("/api/inbounds/", inboundChildrenHandler(cfg.store, cfg.xrayController, cfg.statsClient, cfg.singboxStatsClient))
	mux.HandleFunc("/api/outbounds", outboundsHandler(cfg.store, cfg.xrayController))
	mux.HandleFunc("/api/outbounds/", outboundChildrenHandler(&cfg))
	mux.HandleFunc("/api/routing-rules", routingRulesHandler(cfg.store, cfg.xrayController))
	mux.HandleFunc("/api/routing-rules/", routingRuleChildrenHandler(cfg.store, cfg.xrayController))
	mux.HandleFunc("/api/stats", statsHandler(cfg.store, cfg.statsClient))
	mux.HandleFunc("/api/traffic/summary", trafficSummaryHandler(cfg.store))
	mux.HandleFunc("/api/traffic/inbounds", trafficInboundsHandler(cfg.store))
	mux.HandleFunc("/api/traffic/clients", trafficClientsHandler(cfg.store))
	mux.HandleFunc("/api/traffic/series", trafficSeriesHandler(cfg.store))
	mux.HandleFunc("/api/dashboard/summary", dashboardSummaryHandler(&cfg))
	mux.HandleFunc("/api/system/resources", systemResourcesHandler())
	mux.HandleFunc("/api/xray/config", xrayConfigHandler(cfg.store))
	mux.HandleFunc("/api/xray/validate", xrayValidateHandler(cfg.store))
	mux.HandleFunc("/api/xray/status", xrayStatusHandler(cfg.xrayController))
	mux.HandleFunc("/api/xray/apply", xrayApplyHandler(&cfg))
	mux.HandleFunc("/api/xray/install", coreInstallHandler("xray", cfg.coreScriptRunner))
	mux.HandleFunc("/api/xray/uninstall", coreUninstallHandler("xray", cfg.coreScriptRunner))
	mux.HandleFunc("/api/xray/logs", xrayLogsHandler())
	mux.HandleFunc("/api/xray/version", xrayVersionHandler(cfg.xrayController))
	mux.HandleFunc("/api/cert/status", certStatusHandler(&cfg))
	mux.HandleFunc("/api/cert/issue", certIssueHandler(&cfg))
	mux.HandleFunc("/api/settings", settingsHandler(&cfg))
	mux.HandleFunc("/api/restart", restartHandler())
	mux.HandleFunc("/api/service/status", serviceStatusHandler())
	mux.HandleFunc("/api/version", versionHandler(cfg.version))
	mux.HandleFunc("/api/update/check", updateCheckHandler(&cfg))
	mux.HandleFunc("/api/update/status", updateStatusHandler())
	mux.HandleFunc("/api/update/logs", updateLogsHandler())
	mux.HandleFunc("/api/update", updateHandler(cfg.version))
	mux.HandleFunc("/api/singbox/status", singboxStatusHandler())
	mux.HandleFunc("/api/singbox/apply", singboxApplyHandler(&cfg))
	mux.HandleFunc("/api/singbox/validate", singboxValidateHandler(&cfg))
	mux.HandleFunc("/api/singbox/install", coreInstallHandler("singbox", cfg.coreScriptRunner))
	mux.HandleFunc("/api/singbox/uninstall", coreUninstallHandler("singbox", cfg.coreScriptRunner))
	mux.HandleFunc("/api/singbox/config", singboxConfigHandler(&cfg))
	mux.HandleFunc("/api/singbox/version", singboxVersionHandler())
	mux.HandleFunc("/api/singbox/logs", singboxLogsHandler())
	mux.HandleFunc("/sub/", subscriptionHandler(&cfg))
	mux.HandleFunc("/", spaHandler(cfg.basePath))
	handler := csrfMiddleware(mux, &cfg)
	handler = authMiddleware(handler, &cfg)
	handler = securityHeadersMiddleware(handler, &cfg)
	if cfg.basePath != "" {
		return basePathMiddleware(handler, cfg.basePath)
	}
	return handler
}

func normalizeBasePath(basePath string) string {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" || basePath == "/" {
		return ""
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return strings.TrimRight(basePath, "/")
}

func basePathMiddleware(next http.Handler, basePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == basePath {
			target := basePath + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
			return
		}
		if r.URL.Path != basePath && !strings.HasPrefix(r.URL.Path, basePath+"/") {
			http.NotFound(w, r)
			return
		}
		cloned := r.Clone(r.Context())
		cloned.URL.Path = strings.TrimPrefix(r.URL.Path, basePath)
		if cloned.URL.Path == "" {
			cloned.URL.Path = "/"
		}
		cloned.URL.RawPath = ""
		next.ServeHTTP(w, cloned)
	})
}
