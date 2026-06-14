package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/singbox"
	"github.com/imzyb/MiGate/internal/web/static"
	"github.com/imzyb/MiGate/internal/xray"
)

var validDomain = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
var validEmail = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

const maxXrayLogLines = 200
const defaultSocks5PoolURL = "https://github.cmliussss.net/raw.githubusercontent.com/EDT-Pages/Proxy-List/main/data/socks5.json"

type XrayStatusStore interface {
	ListInbounds(ctx context.Context) ([]db.Inbound, error)
}

type clientTrafficSummary struct {
	Up             int64  `json:"up"`
	Down           int64  `json:"down"`
	XrayUp         int64  `json:"xray_up,omitempty"`
	XrayDown       int64  `json:"xray_down,omitempty"`
	Source         string `json:"source"`
	RealtimeSource string `json:"realtime_source,omitempty"`
	Note           string `json:"note,omitempty"`
}

type inboundTrafficSummary struct {
	Up             int64  `json:"up"`
	Down           int64  `json:"down"`
	Total          int64  `json:"total"`
	Source         string `json:"source"`
	RealtimeSource string `json:"realtime_source,omitempty"`
}

type inboundView struct {
	db.Inbound
	TrafficUp      int64                          `json:"traffic_up"`
	TrafficDown    int64                          `json:"traffic_down"`
	TrafficTotal   int64                          `json:"traffic_total"`
	TrafficSource  string                         `json:"traffic_stats_source"`
	RealtimeSource string                         `json:"realtime_stats_source,omitempty"`
	ClientTraffic  map[int64]clientTrafficSummary `json:"client_traffic,omitempty"`
}

type Store interface {
	ListInbounds(ctx context.Context) ([]db.Inbound, error)
	CreateInbound(ctx context.Context, params db.CreateInboundParams) (db.Inbound, error)
	ListOutbounds(ctx context.Context) ([]db.Outbound, error)
	CreateOutbound(ctx context.Context, params db.CreateOutboundParams) (db.Outbound, error)
	UpdateOutbound(ctx context.Context, id int64, params db.UpdateOutboundParams) (db.Outbound, error)
	DeleteOutbound(ctx context.Context, id int64) error
	ReorderOutbounds(ctx context.Context, ids []int64) error
	ListRoutingRules(ctx context.Context) ([]db.RoutingRule, error)
	CreateRoutingRule(ctx context.Context, params db.CreateRoutingRuleParams) (db.RoutingRule, error)
	UpdateRoutingRule(ctx context.Context, id int64, params db.UpdateRoutingRuleParams) (db.RoutingRule, error)
	DeleteRoutingRule(ctx context.Context, id int64) error
	ReorderRoutingRules(ctx context.Context, ids []int64) error
	CreateClient(ctx context.Context, params db.CreateClientParams) (db.Client, error)
	DeleteInbound(ctx context.Context, id int64) error
	DeleteClient(ctx context.Context, id int64) error
	UpdateInbound(ctx context.Context, id int64, params db.UpdateInboundParams) (db.Inbound, error)
	UpdateClient(ctx context.Context, id int64, params db.UpdateClientParams) (db.Client, error)
	SetInboundEnabled(ctx context.Context, id int64, enabled bool) (db.Inbound, error)
	SetOutboundEnabled(ctx context.Context, id int64, enabled bool) (db.Outbound, error)
	SetClientEnabled(ctx context.Context, inboundID int64, id int64, enabled bool) (db.Client, error)
	ResetClientTraffic(ctx context.Context, id int64) (db.Client, error)
	AddToBlacklist(ctx context.Context, tokenHash string, expiresAt time.Time, revoked bool) error
	IsBlacklisted(ctx context.Context, tokenHash string) (bool, error)
	RecordSessionTouch(ctx context.Context, tokenHash string) error
	ListActiveSessions(ctx context.Context) ([]db.BlacklistedSession, error)
	RevokeSession(ctx context.Context, id int64) error
}

type XrayController interface {
	Status(ctx context.Context) XrayStatus
	Apply(ctx context.Context) XrayApplyResult
	Version(ctx context.Context) string
}

type XrayStatus struct {
	Service           string   `json:"service"`
	Status            string   `json:"status"`
	Managed           bool     `json:"managed"`
	Installed         bool     `json:"installed"`
	Version           string   `json:"version"`
	MemoryRSSBytes    int64    `json:"memory_rss_bytes"`
	Uptime            string   `json:"uptime"`
	ActiveConnections int      `json:"active_connections"`
	ConfigPath        string   `json:"config_path"`
	CommandsExecuted  []string `json:"commands_executed"`
}

type XrayApplyResult struct {
	Status           string   `json:"status"`
	Service          string   `json:"service"`
	CommandsExecuted []string `json:"commands_executed"`
	ErrorOutput      string   `json:"error_output,omitempty"`
}

type defaultXrayController struct{}

func (defaultXrayController) Status(ctx context.Context) XrayStatus {
	return XrayStatus{Service: "xray", Status: "unknown", Managed: false, CommandsExecuted: []string{}}
}

func (defaultXrayController) Apply(ctx context.Context) XrayApplyResult {
	return XrayApplyResult{Status: "not_managed"}
}

func (defaultXrayController) Version(ctx context.Context) string { return "" }

type routerConfig struct {
	store          Store
	xrayController XrayController
	authEnabled    bool
	authUsername   string
	authPassword   string
	sessionSecret  []byte
	configDir      string
	version        string
	basePath       string
	statsClient    xray.StatsClient
	socks5PoolURL  string
	updateCheckURL string
}

const defaultUpdateCheckURL = "https://api.github.com/repos/imzyb/MiGate/releases/latest"

type updateRuntimeStatus struct {
	Status         string    `json:"status"`
	CurrentVersion string    `json:"current_version,omitempty"`
	TargetVersion  string    `json:"target_version,omitempty"`
	Message        string    `json:"message,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type updateRuntimeState struct {
	mu      sync.Mutex
	running bool
	status  updateRuntimeStatus
}

var globalUpdateState = &updateRuntimeState{status: updateRuntimeStatus{Status: "idle", Message: "idle"}}

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

func NewRouter(options ...Option) http.Handler {
	cfg := routerConfig{
		xrayController: defaultXrayController{},
		socks5PoolURL:  defaultSocks5PoolURL,
		updateCheckURL: defaultUpdateCheckURL,
	}
	for _, option := range options {
		option(&cfg)
	}
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static.Assets()))))
	mux.HandleFunc("/login", loginPageHandler(&cfg))
	mux.HandleFunc("/api/login", loginHandler(&cfg))
	mux.HandleFunc("/api/logout", logoutHandler(&cfg))
	mux.HandleFunc("/api/session", sessionHandler(&cfg))
	mux.HandleFunc("/api/sessions", sessionsListHandler(&cfg))
	mux.HandleFunc("/api/sessions/", sessionRevokeHandler(&cfg))
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/inbounds", inboundsHandler(cfg.store, cfg.xrayController, cfg.statsClient))
	mux.HandleFunc("/api/inbounds/", inboundChildrenHandler(cfg.store, cfg.xrayController))
	mux.HandleFunc("/api/outbounds", outboundsHandler(cfg.store, cfg.xrayController))
	mux.HandleFunc("/api/outbounds/", outboundChildrenHandler(&cfg))
	mux.HandleFunc("/api/routing-rules", routingRulesHandler(cfg.store, cfg.xrayController))
	mux.HandleFunc("/api/routing-rules/", routingRuleChildrenHandler(cfg.store, cfg.xrayController))
	mux.HandleFunc("/api/stats", statsHandler(cfg.store, cfg.statsClient))
	mux.HandleFunc("/api/system/resources", systemResourcesHandler())
	mux.HandleFunc("/api/xray/config", xrayConfigHandler(cfg.store))
	mux.HandleFunc("/api/xray/validate", xrayValidateHandler(cfg.store))
	mux.HandleFunc("/api/xray/status", xrayStatusHandler(cfg.xrayController))
	mux.HandleFunc("/api/xray/apply", xrayApplyHandler(cfg.xrayController, cfg.store))
	mux.HandleFunc("/api/xray/install", coreInstallHandler("xray"))
	mux.HandleFunc("/api/xray/uninstall", coreUninstallHandler("xray"))
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
	mux.HandleFunc("/api/update", updateHandler(cfg.version))
	mux.HandleFunc("/api/singbox/status", singboxStatusHandler())
	mux.HandleFunc("/api/singbox/apply", singboxApplyHandler(cfg.store))
	mux.HandleFunc("/api/singbox/validate", singboxValidateHandler(cfg.store))
	mux.HandleFunc("/api/singbox/install", coreInstallHandler("singbox"))
	mux.HandleFunc("/api/singbox/uninstall", coreUninstallHandler("singbox"))
	mux.HandleFunc("/api/singbox/config", singboxConfigHandler())
	mux.HandleFunc("/api/singbox/version", singboxVersionHandler())
	mux.HandleFunc("/api/singbox/logs", singboxLogsHandler())
	mux.HandleFunc("/sub/", subscriptionHandler(cfg.store))
	mux.HandleFunc("/", spaHandler(cfg.basePath))
	handler := authMiddleware(mux, &cfg)
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

func loginPageHandler(cfg *routerConfig) http.HandlerFunc {
	spa := spaHandler(cfg.basePath)
	login := loginHandler(cfg)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			login(w, r)
			return
		}
		spa(w, r)
	}
}

func spaHandler(basePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/sub/") || strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		index, err := static.ReadIndex()
		if err != nil {
			http.Error(w, "web assets are not built", http.StatusInternalServerError)
			return
		}
		baseJSON, _ := json.Marshal(basePath)
		injected := strings.Replace(string(index), "</head>", `<script>window.__MIGATE_BASE_PATH__=`+string(baseJSON)+`;</script></head>`, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(injected))
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","mode":"single-binary"}`))
}

func outboundsHandler(store Store, ctrl XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			outbounds, err := store.ListOutbounds(r.Context())
			if err != nil {
				http.Error(w, `{"error":"list_outbounds_failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(outbounds)
		case http.MethodPost:
			var params db.CreateOutboundParams
			if err := decodeJSONBody(r, &params); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			outbound, err := store.CreateOutbound(r.Context(), params)
			if err != nil {
				http.Error(w, `{"error":"create_outbound_failed"}`, http.StatusBadRequest)
				return
			}
			applyResult := ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"outbound": outbound, "xray": applyResult})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func pingOutbound(address string, port int) map[string]interface{} {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(address, strconv.Itoa(port)), 3*time.Second)
	if err != nil {
		return map[string]interface{}{"latency": -1, "method": "tcping", "error": err.Error()}
	}
	// tcping semantics: measure TCP connect latency only. Do not perform a SOCKS5 handshake.
	latency := time.Since(start).Milliseconds()
	_ = conn.Close()
	return map[string]interface{}{"latency": latency, "method": "tcping"}
}

type socks5PoolProxy struct {
	Address      string  `json:"address"`
	Port         int     `json:"port"`
	Username     string  `json:"username,omitempty"`
	Password     string  `json:"password,omitempty"`
	CountryCode  string  `json:"country_code"`
	Country      string  `json:"country"`
	City         string  `json:"city"`
	ASN          string  `json:"asn"`
	Organization string  `json:"organization"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Latency      int64   `json:"latency"`
}

type socks5PoolRegion struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type socks5PoolCache struct {
	mu        sync.Mutex
	proxies   []socks5PoolProxy
	updatedAt time.Time
	err       string
}

var globalSocks5PoolCache = &socks5PoolCache{}

func nextSocks5PoolRefresh(now time.Time) time.Time {
	loc := now.Location()
	next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, loc)
	if !now.Before(next) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// StartSocks5PoolCacheScheduler refreshes the public SOCKS5 pool once a day at
// 06:00 local time (upstream updates around 05:30) and keeps an in-memory cache
// so opening the dialog does not block on the remote pool.
func StartSocks5PoolCacheScheduler(poolURL string) func() {
	cfg := &routerConfig{socks5PoolURL: poolURL}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_, _, _, _ = cachedSocks5Pool(ctx, cfg)
		for {
			delay := time.Until(nextSocks5PoolRefresh(time.Now()))
			if delay < time.Second {
				delay = time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				_, _, _, _ = cachedSocks5Pool(ctx, cfg)
			}
		}
	}()
	return cancel
}

func cachedSocks5Pool(ctx context.Context, cfg *routerConfig) ([]socks5PoolProxy, time.Time, string, error) {
	globalSocks5PoolCache.mu.Lock()
	cached := append([]socks5PoolProxy(nil), globalSocks5PoolCache.proxies...)
	updatedAt := globalSocks5PoolCache.updatedAt
	lastErr := globalSocks5PoolCache.err
	fresh := len(cached) > 0 && time.Now().Before(nextSocks5PoolRefresh(updatedAt))
	globalSocks5PoolCache.mu.Unlock()
	if fresh {
		return cached, updatedAt, "hit", nil
	}
	proxies, err := fetchSocks5Pool(ctx, cfg.socks5PoolURL)
	globalSocks5PoolCache.mu.Lock()
	defer globalSocks5PoolCache.mu.Unlock()
	if err != nil {
		globalSocks5PoolCache.err = err.Error()
		if len(globalSocks5PoolCache.proxies) > 0 {
			return append([]socks5PoolProxy(nil), globalSocks5PoolCache.proxies...), globalSocks5PoolCache.updatedAt, "stale", nil
		}
		return nil, time.Time{}, "miss", err
	}
	globalSocks5PoolCache.proxies = append([]socks5PoolProxy(nil), proxies...)
	globalSocks5PoolCache.updatedAt = time.Now()
	globalSocks5PoolCache.err = ""
	_ = lastErr
	return append([]socks5PoolProxy(nil), proxies...), globalSocks5PoolCache.updatedAt, "refresh", nil
}

func fetchSocks5Pool(ctx context.Context, poolURL string) ([]socks5PoolProxy, error) {
	if poolURL == "" {
		poolURL = defaultSocks5PoolURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, poolURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MiGate/1.0 socks5-pool")
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pool upstream returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseSocks5Pool(body)
}

func parseSocks5Pool(body []byte) ([]socks5PoolProxy, error) {
	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	items := flattenSocks5PoolItems(raw)
	proxies := make([]socks5PoolProxy, 0, len(items))
	for _, item := range items {
		proxyURL := firstString(item, "proxy", "url", "uri")
		parsedAddress, parsedPort, parsedUser, parsedPass := parseSocks5ProxyURL(proxyURL)
		proxy := socks5PoolProxy{
			Address:      firstNonEmpty(firstString(item, "address", "addr", "ip", "host", "server"), parsedAddress),
			Port:         firstInt(item, "port"),
			Username:     parsedUser,
			Password:     parsedPass,
			CountryCode:  strings.ToUpper(firstString(item, "country_code", "countryCode", "cc", "code")),
			Country:      firstString(item, "country_cn", "country_en", "country_name", "countryName", "name", "country"),
			City:         firstString(item, "city", "region", "location"),
			ASN:          normalizeASN(firstString(item, "asn", "as", "AS")),
			Organization: firstString(item, "organization", "asOrganization", "org", "isp", "operator"),
			Latitude:     firstFloat(item, "latitude", "lat"),
			Longitude:    firstFloat(item, "longitude", "lon", "lng"),
			Latency:      -1,
		}
		if proxy.Port <= 0 && parsedPort > 0 {
			proxy.Port = parsedPort
		}
		if proxy.Address == "" || proxy.Port <= 0 || proxy.Port > 65535 {
			continue
		}
		if proxy.CountryCode == "" {
			country := firstString(item, "country")
			if len(country) == 2 {
				proxy.CountryCode = strings.ToUpper(country)
			}
		}
		proxies = append(proxies, proxy)
	}
	return proxies, nil
}

func flattenSocks5PoolItems(raw interface{}) []map[string]interface{} {
	switch v := raw.(type) {
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(v))
		for _, entry := range v {
			if m, ok := entry.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
		return items
	case map[string]interface{}:
		for _, key := range []string{"proxies", "data", "items", "servers", "socks5"} {
			if nested, ok := v[key]; ok {
				return flattenSocks5PoolItems(nested)
			}
		}
		return []map[string]interface{}{v}
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseSocks5ProxyURL(raw string) (string, int, string, string) {
	if raw == "" {
		return "", 0, "", ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", 0, "", ""
	}
	host := parsed.Hostname()
	port, _ := strconv.Atoi(parsed.Port())
	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	return host, port, username, password
}

func firstString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := item[key]; ok {
			switch x := v.(type) {
			case string:
				return strings.TrimSpace(x)
			case float64:
				return strconv.FormatInt(int64(x), 10)
			}
		}
	}
	return ""
}

func firstInt(item map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := item[key]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case string:
				i, _ := strconv.Atoi(strings.TrimSpace(x))
				return i
			}
		}
	}
	return 0
}

func firstFloat(item map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := item[key]; ok {
			switch x := v.(type) {
			case float64:
				return x
			case string:
				f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
				return f
			}
		}
	}
	return 0
}

func normalizeASN(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(strings.ToUpper(value), "AS") {
		return value
	}
	return "AS" + value
}

func socks5PoolRegions(proxies []socks5PoolProxy) []socks5PoolRegion {
	counts := map[string]*socks5PoolRegion{}
	for _, proxy := range proxies {
		code := proxy.CountryCode
		if code == "" {
			code = "UNKNOWN"
		}
		if counts[code] == nil {
			name := proxy.Country
			if name == "" {
				name = code
			}
			counts[code] = &socks5PoolRegion{Code: code, Name: name}
		}
		counts[code].Count++
	}
	regions := make([]socks5PoolRegion, 0, len(counts))
	for _, region := range counts {
		regions = append(regions, *region)
	}
	return regions
}

func socks5PoolListHandler(cfg *routerConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	proxies, updatedAt, cacheStatus, err := cachedSocks5Pool(r.Context(), cfg)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "pool_fetch_failed", map[string]interface{}{"cache_status": cacheStatus, "detail": err.Error()})
		return
	}
	country := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("country")))
	filtered := make([]socks5PoolProxy, 0, len(proxies))
	for _, proxy := range proxies {
		if country == "" || proxy.CountryCode == country {
			filtered = append(filtered, proxy)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"regions": socks5PoolRegions(proxies), "proxies": filtered,
		"cache_status": cacheStatus, "cache_updated_at": updatedAt.Format(time.RFC3339),
		"next_refresh_at": nextSocks5PoolRefresh(time.Now()).Format(time.RFC3339),
	})
}

func socks5PoolPingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	address := strings.TrimSpace(req.Address)
	if address == "" || req.Port <= 0 || req.Port > 65535 {
		http.Error(w, `{"error":"invalid_proxy"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pingOutbound(address, req.Port))
}

func socks5PoolImportHandler(store Store, ctrl XrayController, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Address      string `json:"address"`
		Port         int    `json:"port"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		City         string `json:"city"`
		ASN          string `json:"asn"`
		Organization string `json:"organization"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	address := strings.TrimSpace(req.Address)
	if address == "" || req.Port <= 0 || req.Port > 65535 {
		http.Error(w, `{"error":"invalid_proxy"}`, http.StatusBadRequest)
		return
	}
	remarkParts := []string{}
	for _, part := range []string{req.City, normalizeASN(req.ASN), req.Organization} {
		part = strings.TrimSpace(part)
		if part != "" {
			remarkParts = append(remarkParts, part)
		}
	}
	remark := strings.Join(remarkParts, " ")
	if remark == "" {
		remark = address
	}
	outbound, err := store.CreateOutbound(r.Context(), db.CreateOutboundParams{
		Tag:      fmt.Sprintf("pool-socks-%s-%d", strings.NewReplacer(".", "-", ":", "-").Replace(address), req.Port),
		Remark:   remark,
		Protocol: "socks",
		Address:  address,
		Port:     req.Port,
		Username: strings.TrimSpace(req.Username),
		Password: req.Password,
	})
	if err != nil {
		http.Error(w, `{"error":"create_outbound_failed"}`, http.StatusBadRequest)
		return
	}
	applyResult := ctrl.Apply(r.Context())
	_ = tryApplySingbox(r.Context(), store)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"outbound": outbound, "xray": applyResult})
}

func outboundChildrenHandler(cfg *routerConfig) http.HandlerFunc {
	store := cfg.store
	ctrl := cfg.xrayController
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/outbounds/")
		if path == "socks5-pool" {
			socks5PoolListHandler(cfg, w, r)
			return
		}
		if path == "socks5-pool/ping" {
			socks5PoolPingHandler(w, r)
			return
		}
		if path == "socks5-pool/import" {
			socks5PoolImportHandler(store, ctrl, w, r)
			return
		}
		// Handle /api/outbounds/reorder
		if path == "reorder" {
			// ...existing reorder handler...
			if r.Method != http.MethodPost {
				http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				IDs []int64 `json:"ids"`
			}
			if err := decodeJSONBody(r, &req); err != nil || len(req.IDs) == 0 {
				http.Error(w, `{"error":"invalid_payload"}`, http.StatusBadRequest)
				return
			}
			if err := store.ReorderOutbounds(r.Context(), req.IDs); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "reorder_failed")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"reordered"}`))
			return
		}
		// Handle /api/outbounds/speedtest-all
		if path == "speedtest-all" {
			if r.Method != http.MethodPost {
				http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
				return
			}
			obs, err := store.ListOutbounds(r.Context())
			if err != nil {
				http.Error(w, `{"error":"load_failed"}`, http.StatusInternalServerError)
				return
			}
			results := make(map[int64]map[string]interface{})
			var mu sync.Mutex
			var wg sync.WaitGroup
			for _, ob := range obs {
				if ob.Protocol == "freedom" || ob.Protocol == "blackhole" || ob.Address == "" {
					continue
				}
				wg.Add(1)
				go func(o db.Outbound) {
					defer wg.Done()
					result := pingOutbound(o.Address, o.Port)
					mu.Lock()
					results[o.ID] = result
					mu.Unlock()
				}(ob)
			}
			wg.Wait()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(results)
			return
		}
		if strings.HasSuffix(path, "/ping") {
			if r.Method != http.MethodGet {
				http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
				return
			}
			idStr := strings.TrimSuffix(path, "/ping")
			obID, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
				return
			}
			outbounds, err := store.ListOutbounds(r.Context())
			if err != nil {
				http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
				return
			}
			var target *db.Outbound
			for i := range outbounds {
				if outbounds[i].ID == obID {
					target = &outbounds[i]
					break
				}
			}
			if target == nil || !target.Enabled || target.Protocol == "freedom" || target.Protocol == "blackhole" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"latency": -1, "error": "not_pingable"})
				return
			}
			result := pingOutbound(target.Address, target.Port)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		idStr := strings.TrimSuffix(path, "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			var params db.UpdateOutboundParams
			if err := decodeJSONBody(r, &params); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			outbound, err := store.UpdateOutbound(r.Context(), id, params)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
				} else {
					http.Error(w, `{"error":"update_failed"}`, http.StatusBadRequest)
				}
				return
			}
			applyResult := ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"outbound": outbound, "xray": applyResult})
		case http.MethodDelete:
			err := store.DeleteOutbound(r.Context(), id)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
				} else {
					http.Error(w, `{"error":"delete_failed"}`, http.StatusInternalServerError)
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func routingRulesHandler(store Store, ctrl XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rules, err := store.ListRoutingRules(r.Context())
			if err != nil {
				http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rules)
		case http.MethodPost:
			var params db.CreateRoutingRuleParams
			if err := decodeJSONBody(r, &params); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			rule, err := store.CreateRoutingRule(r.Context(), params)
			if err != nil {
				http.Error(w, `{"error":"create_failed"}`, http.StatusBadRequest)
				return
			}
			applyResult := ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"rule": rule, "xray": applyResult})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func routingRuleChildrenHandler(store Store, ctrl XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/routing-rules/")
		if path == "reorder" {
			if r.Method != http.MethodPost {
				http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				IDs []int64 `json:"ids"`
			}
			if err := decodeJSONBody(r, &req); err != nil || len(req.IDs) == 0 {
				http.Error(w, `{"error":"invalid_payload"}`, http.StatusBadRequest)
				return
			}
			if err := store.ReorderRoutingRules(r.Context(), req.IDs); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "reorder_failed")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"reordered"}`))
			return
		}
		idStr := strings.TrimSuffix(path, "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			var params db.UpdateRoutingRuleParams
			if err := decodeJSONBody(r, &params); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			rule, err := store.UpdateRoutingRule(r.Context(), id, params)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
				} else {
					http.Error(w, `{"error":"update_failed"}`, http.StatusBadRequest)
				}
				return
			}
			applyResult := ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"rule": rule, "xray": applyResult})
		case http.MethodDelete:
			err := store.DeleteRoutingRule(r.Context(), id)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
				} else {
					http.Error(w, `{"error":"delete_failed"}`, http.StatusInternalServerError)
				}
				return
			}
			applyResult := ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "xray": applyResult})
		case http.MethodGet:
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func isSingBoxProtocol(protocol string) bool {
	switch protocol {
	case "hysteria2", "tuic", "shadowtls":
		return true
	default:
		return false
	}
}

func summarizeTraffic(ctx context.Context, inbounds []db.Inbound, statsClient xray.StatsClient) (map[int64]inboundTrafficSummary, map[int64]clientTrafficSummary) {
	liveStats := map[string]*xray.ClientStats{}
	if statsClient != nil {
		if stats, err := statsClient.QueryAllStats(ctx); err == nil {
			liveStats = stats
		}
	}
	byInbound := map[int64]inboundTrafficSummary{}
	byClient := map[int64]clientTrafficSummary{}
	for _, inbound := range inbounds {
		inboundSummary := inboundTrafficSummary{Source: "db"}
		if isSingBoxProtocol(inbound.Protocol) {
			inboundSummary.Source = "unavailable"
		}
		for _, client := range inbound.Clients {
			clientSummary := clientTrafficSummary{Up: client.Up, Down: client.Down, Source: "db"}
			if isSingBoxProtocol(inbound.Protocol) {
				clientSummary.Source = "unavailable"
				clientSummary.Note = "sing-box realtime traffic stats are not yet wired"
			} else if stats, ok := liveStats[client.Email]; ok {
				clientSummary.XrayUp = stats.Uplink
				clientSummary.XrayDown = stats.Downlink
				clientSummary.RealtimeSource = "xray"
				inboundSummary.RealtimeSource = "xray"
			}
			byClient[client.ID] = clientSummary
			inboundSummary.Up += clientSummary.Up
			inboundSummary.Down += clientSummary.Down
		}
		inboundSummary.Total = inboundSummary.Up + inboundSummary.Down
		byInbound[inbound.ID] = inboundSummary
	}
	return byInbound, byClient
}

func statsHandler(store Store, statsClient xray.StatsClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		inb, _ := store.ListInbounds(ctx)
		obs, _ := store.ListOutbounds(ctx)
		rules, _ := store.ListRoutingRules(ctx)
		var clientCount int
		for _, in := range inb {
			clientCount += len(in.Clients)
		}
		totalObs := len(obs)
		enabledObs := 0
		for _, ob := range obs {
			if ob.Enabled {
				enabledObs++
			}
		}
		totalRules := len(rules)
		enabledRules := 0
		for _, r := range rules {
			if r.Enabled {
				enabledRules++
			}
		}

		trafficByInbound, trafficByClient := summarizeTraffic(ctx, inb, statsClient)
		var clientList []map[string]interface{}
		var totalUp int64
		var totalDown int64
		for _, in := range inb {
			for _, c := range in.Clients {
				clientTraffic := trafficByClient[c.ID]
				info := map[string]interface{}{
					"id":                   c.ID,
					"inbound_id":           c.InboundID,
					"protocol":             in.Protocol,
					"email":                c.Email,
					"enabled":              c.Enabled,
					"up":                   clientTraffic.Up,
					"down":                 clientTraffic.Down,
					"xray_up":              clientTraffic.XrayUp,
					"xray_down":            clientTraffic.XrayDown,
					"traffic_limit":        c.TrafficLimit,
					"expiry_at":            c.ExpiryAt,
					"traffic_stats_source": clientTraffic.Source,
				}
				if clientTraffic.RealtimeSource != "" {
					info["realtime_stats_source"] = clientTraffic.RealtimeSource
				}
				if clientTraffic.Note != "" {
					info["traffic_stats_note"] = clientTraffic.Note
				}
				clientList = append(clientList, info)
			}
		}
		for _, traffic := range trafficByInbound {
			totalUp += traffic.Up
			totalDown += traffic.Down
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"inbounds":              len(inb),
			"clients":               clientCount,
			"client_details":        clientList,
			"traffic_up":            totalUp,
			"traffic_down":          totalDown,
			"traffic_total":         totalUp + totalDown,
			"outbounds":             totalObs,
			"outbounds_enabled":     enabledObs,
			"routing_rules":         totalRules,
			"routing_rules_enabled": enabledRules,
		})
	}
}

type cpuSample struct {
	Idle  uint64
	Total uint64
}

func systemResourcesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memTotal, memUsed, memPercent := readMemoryUsage()
		diskTotal, diskUsed, diskPercent := readDiskUsage("/")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"cpu_percent":    sampleCPUPercent(),
			"memory_total":   memTotal,
			"memory_used":    memUsed,
			"memory_percent": memPercent,
			"disk_total":     diskTotal,
			"disk_used":      diskUsed,
			"disk_percent":   diskPercent,
			"uptime_seconds": readUptimeSeconds(),
		})
	}
}

func sampleCPUPercent() float64 {
	first, err := readCPUSample()
	if err != nil {
		return 0
	}
	time.Sleep(100 * time.Millisecond)
	second, err := readCPUSample()
	if err != nil || second.Total <= first.Total {
		return 0
	}
	totalDelta := second.Total - first.Total
	idleDelta := second.Idle - first.Idle
	if totalDelta == 0 || idleDelta > totalDelta {
		return 0
	}
	return clampPercent(round1(float64(totalDelta-idleDelta) * 100 / float64(totalDelta)))
}

func readCPUSample() (cpuSample, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSample{}, fmt.Errorf("invalid cpu stat")
	}
	var total uint64
	var idle uint64
	for i, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuSample{}, err
		}
		total += value
		if i == 3 || i == 4 {
			idle += value
		}
	}
	return cpuSample{Idle: idle, Total: total}, nil
}

func readMemoryUsage() (totalBytes, usedBytes int64, percent float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	values := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSuffix(fields[0], ":")] = value * 1024
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total <= 0 || available < 0 {
		return 0, 0, 0
	}
	used := total - available
	return total, used, clampPercent(round1(float64(used) * 100 / float64(total)))
}

func readDiskUsage(path string) (totalBytes, usedBytes int64, percent float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize)
	if total <= 0 || free < 0 {
		return 0, 0, 0
	}
	used := total - free
	return total, used, clampPercent(round1(float64(used) * 100 / float64(total)))
}

func readUptimeSeconds() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(seconds)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func inboundsHandler(store Store, ctrl XrayController, statsClient xray.StatsClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listInbounds(w, r, store, statsClient)
		case http.MethodPost:
			createInbound(w, r, store)
			applyXrayAsync(ctrl)
			applySingboxAsync(store)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func applyXrayAsync(ctrl XrayController) {
	go func() {
		result := ctrl.Apply(context.Background())
		if strings.HasPrefix(result.Status, "failed") {
			log.Printf("xray apply failed: status=%s service=%s commands=%v error=%s", result.Status, result.Service, result.CommandsExecuted, result.ErrorOutput)
		}
	}()
}

func applySingboxAsync(store Store) {
	go func() {
		if err := tryApplySingbox(context.Background(), store); err != nil {
			log.Printf("sing-box auto apply: %v", err)
		}
	}()
}

func writeJSONError(w http.ResponseWriter, status int, code string, fields ...map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]interface{}{"error": code}
	for _, extra := range fields {
		for k, v := range extra {
			payload[k] = v
		}
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// decodeJSONBody wraps r.Body with a 512KB MaxBytesReader and decodes JSON
// into v. Returns an error if the body is too large or invalid JSON.
func decodeJSONBody(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<19) // 512KB
	return json.NewDecoder(r.Body).Decode(v)
}

func deriveRealityPublicKeys(inbounds []db.Inbound) {
	for i := range inbounds {
		if inbounds[i].Security == "reality" && inbounds[i].RealityPublicKey == "" && inbounds[i].RealityPrivateKey != "" {
			if pubKey, err := xray.DeriveRealityPublicKey(inbounds[i].RealityPrivateKey); err == nil {
				inbounds[i].RealityPublicKey = pubKey
			}
		}
	}
}

func listInbounds(w http.ResponseWriter, r *http.Request, store Store, statsClient xray.StatsClient) {
	inbounds := []db.Inbound{}
	if store != nil {
		loaded, err := store.ListInbounds(r.Context())
		if err != nil {
			http.Error(w, `{"error":"list_inbounds_failed"}`, http.StatusInternalServerError)
			return
		}
		deriveRealityPublicKeys(loaded)
		inbounds = loaded
	}
	trafficByInbound, trafficByClient := summarizeTraffic(r.Context(), inbounds, statsClient)
	views := make([]inboundView, 0, len(inbounds))
	for _, inbound := range inbounds {
		summary := trafficByInbound[inbound.ID]
		view := inboundView{
			Inbound:        inbound,
			TrafficUp:      summary.Up,
			TrafficDown:    summary.Down,
			TrafficTotal:   summary.Total,
			TrafficSource:  summary.Source,
			RealtimeSource: summary.RealtimeSource,
			ClientTraffic:  map[int64]clientTrafficSummary{},
		}
		for _, client := range inbound.Clients {
			if clientTraffic, ok := trafficByClient[client.ID]; ok {
				view.ClientTraffic[client.ID] = clientTraffic
			}
		}
		views = append(views, view)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"inbounds": views})
}

func createInbound(w http.ResponseWriter, r *http.Request, store Store) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	var payload db.CreateInboundParams
	if err := decodeJSONBody(r, &payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	// Auto-generate REALITY private key if missing
	if payload.Security == "reality" && payload.RealityPrivateKey == "" {
		if privKey, pubKey, err := xray.GenerateRealityKey(); err == nil {
			payload.RealityPrivateKey = privKey
			payload.RealityPublicKey = pubKey
		}
	}
	// Port conflict check
	if payload.Port > 0 {
		existing, _ := store.ListInbounds(r.Context())
		for _, ib := range existing {
			if ib.Port == payload.Port {
				writeJSONError(w, http.StatusConflict, "port_conflict", map[string]interface{}{
					"message": "端口 " + strconv.FormatInt(int64(ib.Port), 10) + " 已被入站 " + strconv.FormatInt(ib.ID, 10) + " 使用",
				})
				return
			}
		}
	}
	created, err := store.CreateInbound(r.Context(), payload)
	if err != nil {
		http.Error(w, `{"error":"unsupported_protocol"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func inboundChildrenHandler(store Store, ctrl XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/inbounds/")
		parts := strings.Split(strings.Trim(path, "/"), "/")

		switch r.Method {
		case http.MethodPost:
			if len(parts) == 4 && parts[1] == "clients" && parts[3] == "reset-traffic" {
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				resetClientTraffic(w, r, store, inboundID, clientID)
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			} else if len(parts) != 2 || parts[1] != "clients" {
				http.NotFound(w, r)
				return
			} else {
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				createClient(w, r, store, inboundID)
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			}
		case http.MethodPatch:
			if len(parts) == 2 && parts[1] == "enabled" {
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				patchInboundEnabled(w, r, store, inboundID)
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			} else if len(parts) == 4 && parts[1] == "clients" && parts[3] == "enabled" {
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				patchClientEnabled(w, r, store, inboundID, clientID)
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			} else {
				http.NotFound(w, r)
			}
		case http.MethodPut:
			if len(parts) == 1 {
				// PUT /api/inbounds/{id}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				updateInbound(w, r, store, inboundID)
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			} else if len(parts) == 3 && parts[1] == "clients" {
				// PUT /api/inbounds/{id}/clients/{clientId}
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				updateClient(w, r, store, clientID)
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			} else {
				http.NotFound(w, r)
			}
		case http.MethodDelete:
			if len(parts) == 1 {
				// DELETE /api/inbounds/{id}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				if store == nil {
					http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
					return
				}
				if err := store.DeleteInbound(r.Context(), inboundID); err != nil {
					http.Error(w, `{"error":"inbound_not_found"}`, http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			} else if len(parts) == 3 && parts[1] == "clients" {
				// DELETE /api/inbounds/{id}/clients/{clientId}
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				if store == nil {
					http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
					return
				}
				if err := store.DeleteClient(r.Context(), clientID); err != nil {
					http.Error(w, `{"error":"client_not_found"}`, http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
				applyXrayAsync(ctrl)
				applySingboxAsync(store)
			} else {
				http.NotFound(w, r)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func createClient(w http.ResponseWriter, r *http.Request, store Store, inboundID int64) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	if !inboundExists(r.Context(), store, inboundID) {
		http.Error(w, `{"error":"inbound_not_found"}`, http.StatusNotFound)
		return
	}
	var payload struct {
		Email        string `json:"email"`
		UUID         string `json:"uuid"`
		TrafficLimit int64  `json:"traffic_limit"`
		ExpiryAt     int64  `json:"expiry_at"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	created, err := store.CreateClient(r.Context(), db.CreateClientParams{InboundID: inboundID, Email: payload.Email, UUID: payload.UUID, TrafficLimit: payload.TrafficLimit, ExpiryAt: payload.ExpiryAt})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate client") {
			writeJSONError(w, http.StatusConflict, "duplicate_client", map[string]interface{}{
				"message": "同一入站下客户端邮箱或凭据已存在，请更换后重试",
			})
			return
		}
		http.Error(w, `{"error":"create_client_failed"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func patchInboundEnabled(w http.ResponseWriter, r *http.Request, store Store, inboundID int64) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	updated, err := store.SetInboundEnabled(r.Context(), inboundID, payload.Enabled)
	if err != nil {
		http.Error(w, `{"error":"inbound_not_found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func patchClientEnabled(w http.ResponseWriter, r *http.Request, store Store, inboundID int64, clientID int64) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	updated, err := store.SetClientEnabled(r.Context(), inboundID, clientID, payload.Enabled)
	if err != nil {
		http.Error(w, `{"error":"client_not_found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func inboundExists(ctx context.Context, store Store, inboundID int64) bool {
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return false
	}
	for _, inbound := range inbounds {
		if inbound.ID == inboundID {
			return true
		}
	}
	return false
}

func updateInbound(w http.ResponseWriter, r *http.Request, store Store, inboundID int64) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	var payload db.UpdateInboundParams
	if err := decodeJSONBody(r, &payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	// Auto-generate REALITY private key if switching to reality without one
	if payload.Security == "reality" && payload.RealityPrivateKey == "" {
		if key, _, err := xray.GenerateRealityKey(); err == nil {
			payload.RealityPrivateKey = key
		}
	}
	// Port conflict check (excluding current inbound)
	if payload.Port > 0 {
		existing, _ := store.ListInbounds(r.Context())
		for _, ib := range existing {
			if ib.ID != inboundID && ib.Port == payload.Port {
				writeJSONError(w, http.StatusConflict, "port_conflict", map[string]interface{}{
					"message": "端口 " + strconv.FormatInt(int64(ib.Port), 10) + " 已被入站 " + strconv.FormatInt(ib.ID, 10) + " 使用",
				})
				return
			}
		}
	}
	updated, err := store.UpdateInbound(r.Context(), inboundID, payload)
	if err != nil {
		http.Error(w, `{"error":"update_inbound_failed"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func resetClientTraffic(w http.ResponseWriter, r *http.Request, store Store, inboundID, clientID int64) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	updated, err := store.ResetClientTraffic(r.Context(), clientID)
	if err != nil {
		http.Error(w, `{"error":"reset_traffic_failed"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func updateClient(w http.ResponseWriter, r *http.Request, store Store, clientID int64) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	var payload db.UpdateClientParams
	if err := decodeJSONBody(r, &payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	updated, err := store.UpdateClient(r.Context(), clientID, payload)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate client") {
			writeJSONError(w, http.StatusConflict, "duplicate_client", map[string]interface{}{
				"message": "同一入站下客户端邮箱已存在，请更换后重试",
			})
			return
		}
		http.Error(w, `{"error":"update_client_failed"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func xrayConfigHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		inbounds := []db.Inbound{}
		outbounds := []db.Outbound{}
		rules := []db.RoutingRule{}
		if store != nil {
			if loaded, err := store.ListInbounds(r.Context()); err == nil {
				inbounds = loaded
			}
			if loaded, err := store.ListOutbounds(r.Context()); err == nil {
				outbounds = loaded
			}
			if loaded, err := store.ListRoutingRules(r.Context()); err == nil {
				rules = loaded
			}
		}
		config, err := xray.BuildConfigWithOutbounds(inbounds, outbounds, rules)
		if err != nil {
			http.Error(w, `{"error":"build_xray_config_failed"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}
}

func xrayStatusHandler(controller XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if controller == nil {
			controller = defaultXrayController{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(controller.Status(r.Context()))
	}
}

type configValidationResult struct {
	Target    string   `json:"target"`
	Valid     bool     `json:"valid"`
	Error     string   `json:"error,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
	Inbounds  int      `json:"inbounds"`
	Outbounds int      `json:"outbounds,omitempty"`
	Rules     int      `json:"rules,omitempty"`
}

func xrayValidateHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		result := validateXrayConfig(r.Context(), store)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func validateXrayConfig(ctx context.Context, store Store) configValidationResult {
	result := configValidationResult{Target: "xray", Valid: true, Warnings: []string{}}
	if store == nil {
		result.Valid = false
		result.Error = "store_unavailable"
		return result
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_inbounds_failed"
		return result
	}
	outbounds, err := store.ListOutbounds(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_outbounds_failed"
		return result
	}
	rules, err := store.ListRoutingRules(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_routing_rules_failed"
		return result
	}
	cfg, err := xray.BuildConfigWithOutbounds(inbounds, outbounds, rules)
	if err != nil {
		result.Valid = false
		result.Error = err.Error()
		return result
	}
	result.Inbounds = len(cfg.Inbounds)
	result.Outbounds = len(cfg.Outbounds)
	if cfg.Routing != nil {
		result.Rules = len(cfg.Routing.Rules)
	}
	for _, inbound := range inbounds {
		if inbound.Enabled && isSingBoxProtocol(inbound.Protocol) {
			result.Warnings = append(result.Warnings, inbound.Protocol+" is handled by sing-box")
		}
	}
	for _, rule := range rules {
		if strings.TrimSpace(rule.RuleSet) != "" {
			result.Warnings = append(result.Warnings, "rule_set is stored for future use but not emitted in Xray config")
			break
		}
	}
	return result
}

func xrayApplyHandler(controller XrayController, store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Confirm            bool `json:"confirm"`
			AllowSystemChanges bool `json:"allow_system_changes"`
		}
		if err := decodeJSONBody(r, &payload); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		if !payload.Confirm || !payload.AllowSystemChanges {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "confirmation_required", "commands_executed": []string{}})
			return
		}
		if controller == nil {
			controller = defaultXrayController{}
		}

		// 1. Apply Xray config
		xrayResult := controller.Apply(r.Context())

		// 2. Apply sing-box config if sing-box supported inbounds exist
		singboxResult := map[string]interface{}{
			"applied": false,
			"reason":  "not_needed",
		}
		if store != nil && singbox.IsInstalled() {
			inbounds, err := store.ListInbounds(r.Context())
			if err == nil {
				hasSingboxInbound := false
				for _, ib := range inbounds {
					if ib.Enabled {
						switch ib.Protocol {
						case "hysteria2", "tuic", "wireguard", "shadowtls":
							hasSingboxInbound = true
							break
						}
					}
				}
				if hasSingboxInbound {
					cfg := singbox.BuildConfig(inbounds)
					if _, err := os.Stat(singbox.CertFile); os.IsNotExist(err) {
						_ = singbox.GenerateSelfSignedCert()
					}
					raw, mErr := json.MarshalIndent(cfg, "", "  ")
					if mErr == nil {
						_ = os.WriteFile(singbox.DefaultConfigPath, raw, 0644)
					}
					applyErr := singbox.Apply()
					if applyErr != nil {
						singboxResult = map[string]interface{}{
							"applied": false,
							"error":   applyErr.Error(),
						}
					} else {
						singboxResult = map[string]interface{}{
							"applied":  true,
							"inbounds": len(cfg.Inbounds),
						}
					}
				}
			}
		} else if store == nil {
			singboxResult["reason"] = "no_store"
		} else {
			singboxResult["reason"] = "singbox_not_installed"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"xray":    xrayResult,
			"singbox": singboxResult,
		})
	}
}

type coreActionPayload struct {
	Confirm            bool `json:"confirm"`
	AllowSystemChanges bool `json:"allow_system_changes"`
}

func decodeCoreActionPayload(w http.ResponseWriter, r *http.Request) (coreActionPayload, bool) {
	var payload coreActionPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return payload, false
	}
	if !payload.Confirm || !payload.AllowSystemChanges {
		writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
		return payload, false
	}
	return payload, true
}

func coreInstallHandler(core string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		var script string
		var commands []string
		switch core {
		case "xray":
			commands = []string{"download Xray-install script", "run installed script", "mkdir -p /usr/local/etc/xray", "ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/xray.json", "systemctl enable --now xray"}
			script = `set -euo pipefail
if ! command -v curl >/dev/null 2>&1; then echo 'curl is required' >&2; exit 1; fi
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fL "https://github.com/XTLS/Xray-install/raw/main/install-release.sh" -o "$tmp/install-release.sh"
bash "$tmp/install-release.sh"
mkdir -p /usr/local/etc/xray
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/xray.json
ln -sf /usr/local/migate/xray.json /usr/local/etc/xray/config.json
systemctl enable xray
systemctl restart xray || true
xray --version | head -1`
		case "singbox":
			commands = []string{"download sing-box release", "install /usr/local/bin/sing-box", "write /etc/systemd/system/migate-singbox.service", "systemctl enable --now migate-singbox"}
			script = `set -euo pipefail
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch=amd64 ;;
  aarch64|arm64) asset_arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
version="${SINGBOX_VERSION:-1.13.13}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
url="https://github.com/SagerNet/sing-box/releases/download/v${version}/sing-box-${version}-linux-${asset_arch}.tar.gz"
checksums_url="https://github.com/SagerNet/sing-box/releases/download/v${version}/sing-box-${version}-checksums.txt"
curl -fL "$url" -o "$tmp/sing-box.tar.gz"
curl -fL "$checksums_url" -o "$tmp/checksums.txt"
grep "sing-box-${version}-linux-${asset_arch}.tar.gz" "$tmp/checksums.txt" > "$tmp/sing-box.tar.gz.sha256"
(cd "$tmp" && sha256sum -c "sing-box.tar.gz.sha256")
tar -xzf "$tmp/sing-box.tar.gz" -C "$tmp"
cp "$tmp"/sing-box-*/sing-box /usr/local/bin/sing-box
chmod +x /usr/local/bin/sing-box
mkdir -p /etc/sing-box
if [ ! -f /etc/sing-box/config.json ]; then
  printf '%s\n' '{"log":{"level":"warn"},"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}]}' > /etc/sing-box/config.json
fi
cat > /etc/systemd/system/migate-singbox.service <<'UNIT'
[Unit]
Description=MiGate managed sing-box service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sing-box run -c /etc/sing-box/config.json
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable migate-singbox
systemctl restart migate-singbox || true
sing-box version | head -1`
		default:
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		out, err := runCoreScript(script)
		status := "installed"
		if err != nil {
			status = "failed"
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"core": core, "status": status, "output": string(out), "commands_executed": commands})
	}
}

func runCoreScript(script string) ([]byte, error) {
	cmd := exec.Command("bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	return cmd.CombinedOutput()
}

func coreUninstallHandler(core string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		var script string
		var commands []string
		switch core {
		case "xray":
			commands = []string{"systemctl disable --now xray", "bash Xray-install remove", "remove MiGate xray symlinks"}
			script = `set -euo pipefail
systemctl disable --now xray 2>/dev/null || true
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" -- remove --purge 2>&1 || true
rm -f /usr/local/etc/xray/xray.json /usr/local/etc/xray/config.json
printf 'Xray removed or disabled\n'`
		case "singbox":
			commands = []string{"systemctl disable --now migate-singbox", "remove sing-box binary and service"}
			script = `set -euo pipefail
systemctl disable --now migate-singbox 2>/dev/null || true
rm -f /etc/systemd/system/migate-singbox.service /usr/local/bin/sing-box
systemctl daemon-reload 2>/dev/null || true
printf 'sing-box removed\n'`
		default:
			writeJSONError(w, http.StatusBadRequest, "unknown_core")
			return
		}
		out, err := runCoreScript(script)
		status := "uninstalled"
		if err != nil {
			status = "failed"
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"core": core, "status": status, "output": string(out), "commands_executed": commands})
	}
}

func xrayLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		lines := r.URL.Query().Get("lines")
		if lines == "" {
			lines = "50"
		}
		if n, err := strconv.Atoi(lines); err != nil || n < 1 {
			lines = "50"
		} else if n > maxXrayLogLines {
			lines = strconv.Itoa(maxXrayLogLines)
		}
		out, err := exec.Command("journalctl", "-u", "xray", "-n", lines, "--no-pager", "-o", "short-iso").CombinedOutput()
		if err != nil {
			// Fallback: try reading from syslog
			out, err = exec.Command("tail", "-n", lines, "/var/log/syslog").CombinedOutput()
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"logs": "无法读取 Xray 日志：journalctl 和 syslog 均不可用。"})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"logs": string(out)})
	}
}

func xrayVersionHandler(controller XrayController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		ver := controller.Version(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": ver})
	}
}

func certStatusHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		domain := ""
		email := ""
		certPath := ""
		keyPath := ""
		issued := false

		if cfg.configDir != "" {
			configPath := cfg.configDir + "/panel.json"
			data, err := os.ReadFile(configPath)
			if err == nil {
				var raw map[string]interface{}
				if err := json.Unmarshal(data, &raw); err == nil {
					if d, ok := raw["cert_domain"].(string); ok {
						domain = d
					}
					if e, ok := raw["cert_email"].(string); ok {
						email = e
					}
				}
			}
			if domain != "" {
				// Check /etc/xray/certs/{domain}.pem and .key first
				certPath = "/etc/xray/certs/" + domain + ".pem"
				keyPath = "/etc/xray/certs/" + domain + ".key"
				if _, err := os.Stat(certPath); err == nil {
					if _, err := os.Stat(keyPath); err == nil {
						issued = true
					}
				}
				// Fallback to config dir for tests
				if !issued && cfg.configDir != "" {
					certDir := cfg.configDir + "/certs/" + domain
					certPath = certDir + "/fullchain.pem"
					keyPath = certDir + "/privkey.pem"
					if _, err := os.Stat(certPath); err == nil {
						if _, err := os.Stat(keyPath); err == nil {
							issued = true
						}
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"domain":    domain,
			"email":     email,
			"issued":    issued,
			"cert_path": certPath,
			"key_path":  keyPath,
		})
	}
}

func installACMESh(email string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "acmesh-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	scriptPath := tmpDir + "/acme.sh"
	resp, err := http.Get("https://get.acme.sh")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download acme.sh installer failed: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(scriptPath, body, 0755); err != nil {
		return "", fmt.Errorf("write acme.sh: %w", err)
	}
	cmd := exec.Command(scriptPath, "--email", email)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func certIssueHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Domain string `json:"domain"`
			Email  string `json:"email"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_json"})
			return
		}
		if req.Domain == "" || req.Email == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "domain_and_email_required"})
			return
		}
		if !validDomain.MatchString(req.Domain) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_domain"})
			return
		}
		if !validEmail.MatchString(req.Email) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_email"})
			return
		}
		if cfg.configDir == "" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "cert_not_available"})
			return
		}

		// Issue cert via acme.sh directly to /etc/xray/certs/
		certDir := "/etc/xray/certs"
		if err := os.MkdirAll(certDir, 0755); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "mkdir_cert_dir_failed"})
			return
		}

		certFile := certDir + "/" + req.Domain + ".pem"
		keyFile := certDir + "/" + req.Domain + ".key"

		// Check if acme.sh is installed; if not, install it without interpolating
		// request data into a shell command string.
		if _, err := exec.LookPath("acme.sh"); err != nil {
			installOut, err := installACMESh(req.Email)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":  "install_acme_failed",
					"detail": installOut,
				})
				return
			}
		}

		// Run acme.sh --issue --standalone
		out, err := exec.Command("acme.sh",
			"--issue", "--standalone", "-d", req.Domain,
			"--keylength", "ec-256",
			"--fullchain-file", certFile,
			"--key-file", keyFile,
			"--reloadcmd", "systemctl restart xray || true",
		).CombinedOutput()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":  "issue_cert_failed",
				"detail": string(out),
			})
			return
		}

		// Set permissions for xray user
		exec.Command("chmod", "644", certFile, keyFile).Run()

		// Update panel.json with cert domain/email
		configPath := cfg.configDir + "/panel.json"
		existing, err := os.ReadFile(configPath)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "read_panel_config_failed"})
			return
		}
		var raw map[string]interface{}
		if err := json.Unmarshal(existing, &raw); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "parse_panel_config_failed"})
			return
		}
		raw["cert_domain"] = req.Domain
		raw["cert_email"] = req.Email
		updated, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "serialize_failed"})
			return
		}
		if err := os.WriteFile(configPath, updated, 0o600); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "write_panel_config_failed"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "issued",
			"domain":    req.Domain,
			"cert_path": certFile,
			"key_path":  keyFile,
		})
	}
}

func versionHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if version == "" {
			version = "dev"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
	}
}

func updateCheckHandler(cfg *routerConfig) http.HandlerFunc {
	type releaseResponse struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Name    string `json:"name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		current := strings.TrimSpace(cfg.version)
		if current == "" {
			current = "dev"
		}
		result := map[string]interface{}{
			"current_version":  current,
			"latest_version":   "",
			"update_available": false,
			"release_url":      "",
			"status":           "unknown",
		}
		if current == "dev" {
			result["status"] = "dev"
			result["message"] = "dev builds cannot be checked against releases"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		checkURL := strings.TrimSpace(cfg.updateCheckURL)
		if checkURL == "" {
			checkURL = defaultUpdateCheckURL
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "update_check_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "MiGate-update-check")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "update_check_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeJSONError(w, http.StatusBadGateway, "update_check_failed", map[string]interface{}{"detail": resp.Status})
			return
		}
		var release releaseResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
			writeJSONError(w, http.StatusBadGateway, "update_check_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		latest := strings.TrimSpace(release.TagName)
		result["latest_version"] = latest
		result["release_url"] = strings.TrimSpace(release.HTMLURL)
		result["release_name"] = strings.TrimSpace(release.Name)
		result["status"] = "ok"
		result["update_available"] = latest != "" && normalizeMiGateVersion(latest) != normalizeMiGateVersion(current)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func updateStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		status := globalUpdateState.snapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func updateHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		current := strings.TrimSpace(version)
		if current == "" {
			current = "dev"
		}
		status, started := globalUpdateState.start(current)
		if !started {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(status)
			return
		}
		command := "/usr/local/bin/migate-install --update"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "updating", "command": command, "message": status.Message})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if runningUnderGoTest() {
			globalUpdateState.finish("started", "update command accepted in test mode")
			return
		}
		go func() {
			time.Sleep(500 * time.Millisecond)
			err := exec.Command("systemd-run", "--wait", "--unit=migate-update", "--replace", "--collect", "--same-dir", "--property=Type=oneshot", "--property=User=root", "--property=TimeoutSec=180", "--property=StandardOutput=append:/var/log/migate-update.log", "--property=StandardError=append:/var/log/migate-update.log", "/usr/local/bin/migate-install", "--update").Run()
			if err != nil {
				globalUpdateState.finish("failed", err.Error())
				return
			}
			globalUpdateState.finish("restarting", "update command started, MiGate will restart shortly")
		}()
	}
}

func normalizeMiGateVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "MiGate version:")
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	return version
}

func (s *updateRuntimeState) start(current string) (updateRuntimeStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return s.status, false
	}
	now := time.Now().UTC()
	s.running = true
	s.status = updateRuntimeStatus{
		Status:         "updating",
		CurrentVersion: current,
		Message:        "update command accepted",
		StartedAt:      now,
		UpdatedAt:      now,
	}
	return s.status, true
}

func (s *updateRuntimeState) finish(status, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.status.Status = status
	s.status.Message = message
	s.status.UpdatedAt = time.Now().UTC()
}

func (s *updateRuntimeState) snapshot() updateRuntimeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func settingsHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.configDir == "" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"settings_not_available"}`))
			return
		}
		configPath := cfg.configDir + "/panel.json"
		switch r.Method {
		case http.MethodGet:
			data, err := os.ReadFile(configPath)
			if err != nil {
				http.Error(w, `{"error":"read_config_failed"}`, http.StatusInternalServerError)
				return
			}
			// Mask password for GET
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				http.Error(w, `{"error":"parse_config_failed"}`, http.StatusInternalServerError)
				return
			}
			if _, exists := raw["panel_password"]; exists {
				raw["has_password"] = true
				delete(raw, "panel_password")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(raw)
		case http.MethodPut:
			var updated map[string]interface{}
			if err := decodeJSONBody(r, &updated); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			// Read existing to preserve password if not provided
			existing, err := os.ReadFile(configPath)
			if err == nil {
				var existingMap map[string]interface{}
				if err := json.Unmarshal(existing, &existingMap); err == nil {
					if pw, has := updated["panel_password"]; !has || pw == "" {
						if oldPW, ok := existingMap["panel_password"]; ok {
							updated["panel_password"] = oldPW
						}
					}
					// Preserve database_path if not in update
					if _, has := updated["database_path"]; !has {
						if oldDP, ok := existingMap["database_path"]; ok {
							updated["database_path"] = oldDP
						}
					}
				}
			}
			data, err := json.MarshalIndent(updated, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"serialize_failed"}`, http.StatusInternalServerError)
				return
			}
			if err := os.WriteFile(configPath, data, 0o600); err != nil {
				http.Error(w, `{"error":"write_config_failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func runningUnderGoTest() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func restartHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"restarting"}`))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Fork a child that restarts after a brief delay so the response is sent first
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = exec.Command("systemctl", "restart", "migate").Run()
		}()
		if !runningUnderGoTest() {
			go func() {
				time.Sleep(2 * time.Second)
				os.Exit(0)
			}()
		}
	}
}

func serviceStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		status, detail := "unknown", ""
		out, err := exec.Command("systemctl", "is-active", "migate").Output()
		if err == nil {
			status = strings.TrimSpace(string(out))
		}
		if status == "active" {
			out2, _ := exec.Command("systemctl", "show", "migate", "--property=ActiveEnterTimestamp", "--value").Output()
			if len(out2) > 0 {
				detail = "启动于 " + strings.TrimSpace(string(out2))
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "migate",
			"status":  status,
			"detail":  detail,
		})
	}
}

func subscriptionHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.NotFound(w, r)
			return
		}
		token := strings.Trim(strings.TrimPrefix(r.URL.Path, "/sub/"), "/")
		if token == "" {
			http.NotFound(w, r)
			return
		}
		inbounds, err := store.ListInbounds(r.Context())
		if err != nil {
			http.Error(w, `{"error":"list_inbounds_failed"}`, http.StatusInternalServerError)
			return
		}
		deriveRealityPublicKeys(inbounds)
		for _, inbound := range inbounds {
			if !inbound.Enabled {
				continue
			}
			now := time.Now().Unix()
			for _, client := range inbound.Clients {
				if client.UUID != token {
					continue
				}
				if !client.Enabled {
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					_, _ = w.Write([]byte("// Subscription disabled"))
					return
				}
				// Check expired or over-limit
				if client.ExpiryAt > 0 && client.ExpiryAt <= now {
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					_, _ = w.Write([]byte("// Subscription expired"))
					return
				}
				if client.TrafficLimit > 0 && (client.Up+client.Down) >= client.TrafficLimit {
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					_, _ = w.Write([]byte("// Traffic limit exceeded"))
					return
				}
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(shareLink(r.Host, inbound, client)))
				return
			}
		}
		http.NotFound(w, r)
	}
}

func shareLink(host string, inbound db.Inbound, client db.Client) string {
	host = subscriptionHost(host)
	switch inbound.Protocol {
	case "vmess":
		return vmessShareLink(host, inbound, client)
	case "shadowsocks":
		return ssShareLink(host, inbound, client)
	case "hysteria2":
		// hysteria2://password@host:port/?params#name
		var params []string
		addParam := func(k, v string) {
			if v != "" {
				params = append(params, k+"="+url.QueryEscape(v))
			}
		}
		if inbound.Hy2UpMbps > 0 {
			params = append(params, "up_mbps="+strconv.Itoa(inbound.Hy2UpMbps))
		}
		if inbound.Hy2DownMbps > 0 {
			params = append(params, "down_mbps="+strconv.Itoa(inbound.Hy2DownMbps))
		}
		addParam("obfs", inbound.Hy2Obfs)
		addParam("obfs-password", inbound.Hy2ObfsPassword)
		// sing-box v1.13 requires TLS for Hysteria2 server inbounds.
		// MiGate uses generated self-signed certs by default, so share links must
		// include TLS + insecure even when the UI stores security=none.
		params = append(params, "security=tls")
		addParam("sni", inbound.RealityServerNames)
		params = append(params, "insecure=1")
		query := strings.Join(params, "&")
		suffix := ""
		if query != "" {
			suffix = "?" + query
		}
		return "hysteria2://" + client.UUID + "@" + host + ":" + strconv.Itoa(inbound.Port) + suffix + "#" + url.QueryEscape(client.Email)
	default:
		// vless, trojan, etc. use universal link format
		var params []string
		addParam := func(k, v string) {
			if v != "" {
				params = append(params, k+"="+url.QueryEscape(v))
			}
		}
		addParam("type", inbound.Network)
		addParam("security", inbound.Security)
		if inbound.Security == "reality" {
			if inbound.Network != "xhttp" {
				params = append(params, "flow=xtls-rprx-vision")
			}
			addParam("sni", inbound.RealityServerNames)
			params = append(params, "fp=chrome")
			addParam("pbk", inbound.RealityPublicKey)
			addParam("sid", inbound.RealityShortID)
		} else if inbound.Security == "tls" {
			addParam("sni", inbound.RealityServerNames)
			params = append(params, "allowInsecure=1")
		}
		// Transport-specific params
		switch inbound.Network {
		case "ws":
			addParam("path", inbound.WsPath)
			addParam("host", inbound.WsHost)
		case "h2":
			addParam("path", inbound.WsPath)
			addParam("host", inbound.WsHost)
		case "grpc":
			addParam("serviceName", inbound.GrpcServiceName)
		case "xhttp":
			addParam("path", inbound.XHTTPPath)
			addParam("mode", inbound.XHTTPMode)
		case "kcp":
		case "quic":
		}
		query := strings.Join(params, "&")
		return inbound.Protocol + "://" + client.UUID + "@" + host + ":" + strconv.Itoa(inbound.Port) + "?" + query + "#" + url.QueryEscape(client.Email)
	}
}

func vmessShareLink(host string, inbound db.Inbound, client db.Client) string {
	inboundPort := inbound.Port
	portStr := strconv.Itoa(inboundPort)
	tls := ""
	if inbound.Security == "tls" || inbound.Security == "reality" {
		tls = "tls"
	}

	// Transport-specific host and path
	vHost, vPath := "", ""
	sni := ""
	switch inbound.Network {
	case "ws":
		vHost = inbound.WsHost
		vPath = inbound.WsPath
	case "grpc":
		vPath = inbound.GrpcServiceName
	case "xhttp":
		vPath = inbound.XHTTPPath
	case "h2":
		vHost = inbound.WsHost
		vPath = inbound.WsPath
	}
	if inbound.Security == "tls" || inbound.Security == "reality" {
		sni = inbound.RealityServerNames
	}
	vmessData := map[string]interface{}{
		"v":    "2",
		"ps":   client.Email,
		"add":  host,
		"port": portStr,
		"id":   client.UUID,
		"aid":  "0",
		"scy":  "auto",
		"net":  inbound.Network,
		"type": "none",
		"host": vHost,
		"path": vPath,
		"tls":  tls,
		"sni":  sni,
	}
	b, _ := json.Marshal(vmessData)
	encoded := base64.StdEncoding.EncodeToString(b)
	return "vmess://" + encoded
}

func ssShareLink(host string, inbound db.Inbound, client db.Client) string {
	method := inbound.SSMethod
	if method == "" {
		method = "2022-blake3-aes-128-gcm"
	}
	userPass := method + ":" + inbound.UUID
	encoded := base64.StdEncoding.EncodeToString([]byte(userPass))
	return "ss://" + encoded + "@" + host + ":" + strconv.Itoa(inbound.Port) + "#" + url.QueryEscape(client.Email)
}

func subscriptionHost(host string) string {
	if host == "" {
		return "SERVER_IP"
	}
	name, _, err := net.SplitHostPort(host)
	if err == nil && name != "" {
		return name
	}
	return strings.Trim(host, "[]")
}

func singboxStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		installed := singbox.IsInstalled()
		if !installed {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"installed": false,
				"status":    "not_installed",
			})
			return
		}
		status := singbox.Status()
		ver, _ := singbox.Version()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"installed":          true,
			"status":             status,
			"version":            strings.TrimSpace(ver),
			"memory_rss_bytes":   singbox.MemoryRSS(),
			"uptime":             singbox.Uptime(),
			"active_connections": singbox.ActiveConnections(),
		})
	}
}

// singboxApplyHandler reads sing-box supported inbounds from the store, builds
// a sing-box config, generates a self-signed cert if missing, writes
// the config to disk and restarts the sing-box service.
func singboxApplyHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if !singbox.IsInstalled() {
			http.Error(w, `{"error":"singbox_not_installed"}`, http.StatusBadRequest)
			return
		}

		// Read sing-box inbounds
		inbounds, err := store.ListInbounds(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_failed", map[string]interface{}{"detail": err.Error()})
			return
		}

		// Build config
		cfg := singbox.BuildConfig(inbounds)

		// Ensure self-signed cert exists
		if _, err := os.Stat(singbox.CertFile); os.IsNotExist(err) {
			if err := singbox.GenerateSelfSignedCert(); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "cert_failed", map[string]interface{}{"detail": err.Error()})
				return
			}
		}

		// Encode and write config
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "marshal_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		if err := os.WriteFile(singbox.DefaultConfigPath, raw, 0644); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "write_failed", map[string]interface{}{"detail": err.Error()})
			return
		}

		// Restart sing-box
		applyErr := singbox.Apply()

		result := map[string]interface{}{
			"applied":     applyErr == nil,
			"config_path": singbox.DefaultConfigPath,
			"inbounds":    len(cfg.Inbounds),
		}
		if applyErr != nil {
			result["error"] = applyErr.Error()
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func singboxValidateHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		result := validateSingboxConfig(r.Context(), store)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func validateSingboxConfig(ctx context.Context, store Store) configValidationResult {
	result := configValidationResult{Target: "singbox", Valid: true, Warnings: []string{}}
	if store == nil {
		result.Valid = false
		result.Error = "store_unavailable"
		return result
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		result.Valid = false
		result.Error = "list_inbounds_failed"
		return result
	}
	cfg := singbox.BuildConfig(inbounds)
	result.Inbounds = len(cfg.Inbounds)
	if result.Inbounds == 0 {
		result.Warnings = append(result.Warnings, "no_enabled_singbox_inbounds")
	}
	if _, err := json.Marshal(cfg); err != nil {
		result.Valid = false
		result.Error = err.Error()
	}
	return result
}

// tryApplySingbox reads sing-box supported inbounds from the store, builds
// a sing-box config, writes it to disk and restarts sing-box. Errors are
// silently returned (not panicked) to avoid blocking the caller.
func tryApplySingbox(ctx context.Context, store Store) error {
	if !singbox.IsInstalled() {
		return nil // sing-box not available, skip silently
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return fmt.Errorf("list inbounds: %w", err)
	}
	cfg := singbox.BuildConfig(inbounds)
	if _, err := os.Stat(singbox.CertFile); os.IsNotExist(err) {
		if err := singbox.GenerateSelfSignedCert(); err != nil {
			return fmt.Errorf("generate cert: %w", err)
		}
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(singbox.DefaultConfigPath, raw, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return singbox.Apply()
}

// singboxConfigHandler returns the current sing-box config JSON.
func singboxConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data, err := os.ReadFile(singbox.DefaultConfigPath)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "read_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		// Parse and re-marshal so the client gets pretty-printed JSON
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			_, _ = w.Write(data)
			return
		}
		pretty, _ := json.MarshalIndent(parsed, "", "  ")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(pretty)
	}
}

// singboxVersionHandler returns the sing-box version.
func singboxVersionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !singbox.IsInstalled() {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": "not_installed"})
			return
		}
		ver, err := singbox.Version()
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": "unknown", "error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": strings.TrimSpace(ver)})
	}
}

// singboxLogsHandler returns recent sing-box service logs from journalctl.
func singboxLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		lines := r.URL.Query().Get("lines")
		if lines == "" {
			lines = "50"
		}
		if n, err := strconv.Atoi(lines); err != nil || n < 1 {
			lines = "50"
		} else if n > maxXrayLogLines {
			lines = strconv.Itoa(maxXrayLogLines)
		}
		out, err := exec.Command("journalctl", "-u", singbox.ServiceName(), "-n", lines, "--no-pager", "-o", "short-iso").CombinedOutput()
		if err != nil {
			out, err = exec.Command("tail", "-n", lines, "/var/log/syslog").CombinedOutput()
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"logs": "无法读取 Sing-box 日志：journalctl 和 syslog 均不可用。"})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"logs": string(out)})
	}
}
