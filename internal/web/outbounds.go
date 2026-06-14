package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

const defaultSocks5PoolURL = "https://github.cmliussss.net/raw.githubusercontent.com/EDT-Pages/Proxy-List/main/data/socks5.json"

func outboundsHandler(store Store, ctrl XrayController) http.HandlerFunc {
	if ctrl == nil {
		ctrl = defaultXrayController{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			outbounds, err := store.ListOutbounds(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_outbounds_failed")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(outbounds)
		case http.MethodPost:
			var params db.CreateOutboundParams
			if err := decodeJSONBody(r, &params); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			outbound, err := store.CreateOutbound(r.Context(), params)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "create_outbound_failed")
				return
			}
			applyResult := ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"outbound": outbound, "xray": applyResult})
		default:
			methodNotAllowed(w)
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
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
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
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	var req struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	address := strings.TrimSpace(req.Address)
	if address == "" || req.Port <= 0 || req.Port > 65535 {
		writeJSONError(w, http.StatusBadRequest, "invalid_proxy")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pingOutbound(address, req.Port))
}

func socks5PoolImportHandler(store Store, ctrl XrayController, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return
	}
	if ctrl == nil {
		ctrl = defaultXrayController{}
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
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	address := strings.TrimSpace(req.Address)
	if address == "" || req.Port <= 0 || req.Port > 65535 {
		writeJSONError(w, http.StatusBadRequest, "invalid_proxy")
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
		writeJSONError(w, http.StatusBadRequest, "create_outbound_failed")
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
	if ctrl == nil {
		ctrl = defaultXrayController{}
	}
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
				writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
				return
			}
			var req struct {
				IDs []int64 `json:"ids"`
			}
			if err := decodeJSONBody(r, &req); err != nil || len(req.IDs) == 0 {
				writeJSONError(w, http.StatusBadRequest, "invalid_payload")
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
				writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
				return
			}
			obs, err := store.ListOutbounds(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "load_failed")
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
				writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
				return
			}
			idStr := strings.TrimSuffix(path, "/ping")
			obID, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_id")
				return
			}
			outbounds, err := store.ListOutbounds(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "list_failed")
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
			writeJSONError(w, http.StatusBadRequest, "invalid_id")
			return
		}
		switch r.Method {
		case http.MethodPut:
			var params db.UpdateOutboundParams
			if err := decodeJSONBody(r, &params); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			outbound, err := store.UpdateOutbound(r.Context(), id, params)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSONError(w, http.StatusNotFound, "not_found")
				} else {
					writeJSONError(w, http.StatusBadRequest, "update_failed")
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
					writeJSONError(w, http.StatusNotFound, "not_found")
				} else {
					writeJSONError(w, http.StatusInternalServerError, "delete_failed")
				}
				return
			}
			_ = ctrl.Apply(r.Context())
			_ = tryApplySingbox(r.Context(), store)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			methodNotAllowed(w)
		}
	}
}
