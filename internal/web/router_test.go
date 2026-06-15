package web_test

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/imzyb/MiGate/internal/web"
	"github.com/imzyb/MiGate/internal/web/static"
)

func join(parts ...string) string { return strings.Join(parts, "") }

func webPackageSource(t *testing.T) string {
	t.Helper()
	var body strings.Builder
	err := fs.WalkDir(os.DirFS("."), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == "static" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body.Write(source)
		body.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("read web package source: %v", err)
	}
	return body.String()
}

func TestRouterBackendSecurityContracts(t *testing.T) {
	body := webPackageSource(t)
	if strings.Contains(body, `exec.Command("bash", "-c"`) || strings.Contains(body, `exec.Command("sh", "-c"`) {
		t.Fatalf("web package must not execute shell strings via bash/sh -c")
	}
	if regexp.MustCompile(`tail",\s*"-n",\s*lines`).FindString(body) != "" && !strings.Contains(body, "maxXrayLogLines") {
		t.Fatalf("xray log line count must be clamped before passing to journalctl/tail")
	}
}

func TestSystemResourcesAPIReportsServerUsage(t *testing.T) {
	router := web.NewRouter()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/system/resources", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected system resources 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]float64
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode resources response: %v", err)
	}
	for _, key := range []string{"cpu_percent", "memory_total", "memory_used", "memory_percent", "disk_total", "disk_used", "disk_percent", "uptime_seconds"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("resources response missing %s: %#v", key, body)
		}
	}
	if body["disk_total"] <= 0 {
		t.Fatalf("resources response should contain positive disk total: %#v", body)
	}
	if body["cpu_percent"] < 0 || body["cpu_percent"] > 100 || body["memory_percent"] < 0 || body["memory_percent"] > 100 || body["disk_percent"] < 0 || body["disk_percent"] > 100 {
		t.Fatalf("resource percentages should be clamped to 0..100: %#v", body)
	}

	post := httptest.NewRecorder()
	router.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/system/resources", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected POST to be rejected, got %d", post.Code)
	}
}

func TestRouterServesEmbeddedSPAAndHealthAPI(t *testing.T) {
	router := web.NewRouter()

	page := httptest.NewRecorder()
	router.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("expected 200 for panel, got %d: %s", page.Code, page.Body.String())
	}
	body := page.Body.String()
	for _, want := range []string{"MiGate", `id="root"`, `./assets/`} {
		if !strings.Contains(body, want) {
			t.Fatalf("SPA index missing %q: %s", want, body)
		}
	}

	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `id="root"`) {
		t.Fatalf("expected /login to serve SPA, got %d: %s", login.Code, login.Body.String())
	}

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", health.Code, health.Body.String())
	}
	if !strings.Contains(health.Body.String(), `"status":"ok"`) || !strings.Contains(health.Body.String(), `"mode":"single-binary"`) {
		t.Fatalf("unexpected health body: %s", health.Body.String())
	}
}

func TestRouterSetsSecurityHeaders(t *testing.T) {
	router := web.NewRouter()
	for _, tc := range []struct {
		path  string
		https bool
	}{
		{path: "/"},
		{path: "/api/health", https: true},
	} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.https {
			req.Header.Set("X-Forwarded-Proto", "https")
		}
		router.ServeHTTP(resp, req)
		for _, header := range []string{"X-Content-Type-Options", "Referrer-Policy", "Content-Security-Policy"} {
			if resp.Header().Get(header) == "" {
				t.Fatalf("%s response missing %s", tc.path, header)
			}
		}
		if resp.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s response should deny framing, got %q", tc.path, resp.Header().Get("X-Frame-Options"))
		}
		csp := resp.Header().Get("Content-Security-Policy")
		if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
			t.Fatalf("%s response should not allow inline scripts in CSP: %s", tc.path, csp)
		}
		if tc.path == "/" {
			nonce := firstCSPNonce(csp)
			if nonce == "" {
				t.Fatalf("%s response CSP should include a script nonce: %s", tc.path, csp)
			}
			if !strings.Contains(resp.Body.String(), `script nonce="`+nonce+`"`) {
				t.Fatalf("%s response should apply CSP nonce to inline scripts", tc.path)
			}
		}
		if tc.https && resp.Header().Get("Strict-Transport-Security") != "" {
			t.Fatalf("%s response should not trust X-Forwarded-Proto by default", tc.path)
		}
	}

	trusted := web.NewRouter(web.WithTrustedProxyHeaders(true))
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	trusted.ServeHTTP(resp, req)
	if resp.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("trusted proxy HTTPS response missing HSTS")
	}

	directTLS := web.NewRouter()
	tlsResp := httptest.NewRecorder()
	tlsReq := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	tlsReq.TLS = &tls.ConnectionState{}
	directTLS.ServeHTTP(tlsResp, tlsReq)
	if tlsResp.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("direct TLS response missing HSTS")
	}
}

func firstCSPNonce(csp string) string {
	match := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func TestRouterServesViteAssets(t *testing.T) {
	entries, err := fs.ReadDir(static.Dist(), "assets")
	if err != nil {
		t.Fatalf("read embedded assets: %v", err)
	}
	var asset string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".js") {
			asset = entry.Name()
			break
		}
	}
	if asset == "" {
		t.Fatal("expected embedded Vite JS asset")
	}
	router := web.NewRouter()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/assets/"+asset, nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected asset 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if cache := resp.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected asset cache header: %q", cache)
	}
}

func TestRouterSPAFallbackAndAPISubIsolation(t *testing.T) {
	router := web.NewRouter()
	for _, path := range []string{"/inbounds", "/settings", "/login"} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `id="root"`) {
			t.Fatalf("expected %s to fallback to SPA, got %d: %s", path, resp.Code, resp.Body.String())
		}
	}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/not-found"},
		{http.MethodPost, "/api/not-found"},
		{http.MethodGet, "/sub/not-found"},
		{http.MethodGet, "/assets/not-found.js"},
	} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(tc.method, tc.path, nil))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s %s should not fallback to SPA, got %d", tc.method, tc.path, resp.Code)
		}
	}
}

func TestRouterBasePathServesSPAAssetsAndAPI(t *testing.T) {
	router := web.NewRouter(web.WithBasePath("/panel"))
	root := httptest.NewRecorder()
	router.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/panel", nil))
	if root.Code != http.StatusPermanentRedirect || root.Header().Get("Location") != "/panel/" {
		t.Fatalf("expected /panel to redirect to /panel/, got %d location=%q", root.Code, root.Header().Get("Location"))
	}
	for _, path := range []string{"/panel/", "/panel/login", "/panel/inbounds"} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `id="root"`) {
			t.Fatalf("expected %s to serve SPA, got %d: %s", path, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), `window.__MIGATE_BASE_PATH__="/panel"`) {
			t.Fatalf("expected %s to inject SPA base path, got: %s", path, resp.Body.String())
		}
	}
	apiResp := httptest.NewRecorder()
	router.ServeHTTP(apiResp, httptest.NewRequest(http.MethodGet, "/panel/api/session", nil))
	if apiResp.Code != http.StatusOK {
		t.Fatalf("expected base-path API 200, got %d: %s", apiResp.Code, apiResp.Body.String())
	}
	outside := httptest.NewRecorder()
	router.ServeHTTP(outside, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if outside.Code != http.StatusNotFound {
		t.Fatalf("expected outside base path 404, got %d", outside.Code)
	}
}

func TestRouterBasePathLoginPathAcceptsPostForCompatibility(t *testing.T) {
	router := web.NewRouter(web.WithAuth("admin", "secret"), web.WithBasePath("/panel"))
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/panel/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1")
	req.Host = "127.0.0.1"
	req.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected POST /panel/login to login, got %d: %s", resp.Code, resp.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == "migate_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Path != "/panel" {
		t.Fatalf("expected /panel session cookie, got %+v", sessionCookie)
	}
}

func TestUpdateAPIStartsInstallerUpdateWithoutBlockingResponse(t *testing.T) {
	router := web.NewRouter()

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/update", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected GET /api/update 405, got %d", get.Code)
	}

	missingConfirm := httptest.NewRecorder()
	reqMissing := httptest.NewRequest(http.MethodPost, "/api/update", strings.NewReader(`{}`))
	reqMissing.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(missingConfirm, reqMissing)
	if missingConfirm.Code != http.StatusForbidden {
		t.Fatalf("expected POST /api/update without confirmation 403, got %d", missingConfirm.Code)
	}

	post := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/update", strings.NewReader(`{"confirm":true,"allow_system_changes":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(post, req)
	if post.Code != http.StatusOK {
		t.Fatalf("expected POST /api/update 200, got %d: %s", post.Code, post.Body.String())
	}
	for _, want := range []string{`"status":"updating"`, `"command":"/usr/local/bin/migate-install --update"`} {
		if !strings.Contains(post.Body.String(), want) {
			t.Fatalf("update response missing %q: %s", want, post.Body.String())
		}
	}
}

func TestUpdateCheckAPIReportsLatestRelease(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET update check, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"https://github.com/imzyb/MiGate/releases/tag/v1.2.0","name":"v1.2.0"}`))
	}))
	defer upstream.Close()

	router := web.NewRouter(web.WithVersion("v1.1.0"), web.WithUpdateCheckURL(upstream.URL))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/update/check", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected update check 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode update check: %v", err)
	}
	for _, want := range []string{`"current_version":"v1.1.0"`, `"latest_version":"v1.2.0"`, `"release_url":"https://github.com/imzyb/MiGate/releases/tag/v1.2.0"`, `"status":"ok"`} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("update check response missing %q: %s", want, resp.Body.String())
		}
	}
	if body["update_available"] != true {
		t.Fatalf("expected update_available true, got %#v", body["update_available"])
	}
}

func TestUpdateStatusAPIReportsState(t *testing.T) {
	router := web.NewRouter()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/update/status", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected update status 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"status"`) {
		t.Fatalf("update status response missing status: %s", resp.Body.String())
	}
}

func TestUpdateAPIRunsInstallerOutsideMiGateServiceCgroup(t *testing.T) {
	body := webPackageSource(t)
	for _, want := range []string{
		`exec.Command("systemd-run", "--wait", "--unit=migate-update", "--replace", "--collect", "--same-dir", "--property=Type=oneshot", "--property=User=root", "--property=TimeoutSec=180", "--property=StandardOutput=append:/var/log/migate-update.log", "--property=StandardError=append:/var/log/migate-update.log", "/usr/local/bin/migate-install", "--update")`,
		`/var/log/migate-update.log`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("update handler missing detached updater contract %q", want)
		}
	}
	if strings.Contains(body, `exec.Command("/usr/local/bin/migate-install", "--update").Run()`) {
		t.Fatalf("update handler must not run updater inside the migate service cgroup")
	}
}

func TestSocks5PoolEndpointReportsCacheMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"proxy":"socks5://user:pass@127.0.0.1:65000","country":"US","country_en":"United States"}]`))
	}))
	defer upstream.Close()
	router := web.NewRouter(web.WithSocks5PoolURL(upstream.URL))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/outbounds/socks5-pool", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected socks5 pool 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["cache_status"] == nil || body["cache_updated_at"] == nil {
		t.Fatalf("SOCKS5 pool response must expose cache metadata: %s", resp.Body.String())
	}
}

func TestSessionAPIReportsAuthUser(t *testing.T) {
	router := web.NewRouter(web.WithAuth("sam", "secret"))

	unauth := httptest.NewRecorder()
	router.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if unauth.Code != http.StatusOK {
		t.Fatalf("expected public session endpoint 200, got %d: %s", unauth.Code, unauth.Body.String())
	}
	if !strings.Contains(unauth.Body.String(), `"authenticated":false`) || !strings.Contains(unauth.Body.String(), `"auth_enabled":true`) {
		t.Fatalf("unexpected unauthenticated session body: %s", unauth.Body.String())
	}

	login := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"sam","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1")
	req.Host = "127.0.0.1"
	req.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(login, req)
	if login.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", login.Code, login.Body.String())
	}

	sess := httptest.NewRecorder()
	sessReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	for _, c := range login.Result().Cookies() {
		sessReq.AddCookie(c)
	}
	router.ServeHTTP(sess, sessReq)
	if sess.Code != http.StatusOK {
		t.Fatalf("expected authenticated session 200, got %d: %s", sess.Code, sess.Body.String())
	}
	for _, want := range []string{`"authenticated":true`, `"auth_enabled":true`, `"username":"sam"`} {
		if !strings.Contains(sess.Body.String(), want) {
			t.Fatalf("session response missing %q: %s", want, sess.Body.String())
		}
	}
}

func TestRestartEndpoint(t *testing.T) {
	t.Run("POST returns restarting status", func(t *testing.T) {
		router := web.NewRouter()
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/restart", strings.NewReader(`{"confirm":true,"allow_system_changes":true}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "restarting") {
			t.Fatalf("expected restarting status, got %s", response.Body.String())
		}
	})
	t.Run("GET returns 405", func(t *testing.T) {
		router := web.NewRouter()
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/restart", nil)
		router.ServeHTTP(response, req)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", response.Code)
		}
	})
}

func TestAPIErrorResponsesUseJSONContentType(t *testing.T) {
	router := web.NewRouter()
	for _, tc := range []struct {
		method string
		path   string
		body   string
		status int
		error  string
	}{
		{http.MethodPost, "/api/xray/apply", `{"confirm":true}`, http.StatusForbidden, "confirmation_required"},
		{http.MethodPost, "/api/xray/apply", `{bad`, http.StatusBadRequest, "invalid_json"},
		{http.MethodGet, "/api/restart", "", http.StatusMethodNotAllowed, "method_not_allowed"},
	} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Fatalf("%s %s expected %d, got %d: %s", tc.method, tc.path, tc.status, response.Code, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
			t.Fatalf("%s %s expected JSON content type, got %q", tc.method, tc.path, contentType)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s %s should return JSON body: %v body=%s", tc.method, tc.path, err, response.Body.String())
		}
		if payload["error"] != tc.error {
			t.Fatalf("%s %s expected error %q, got %#v", tc.method, tc.path, tc.error, payload)
		}
	}
}

func TestRouterDoesNotServeRemovedHeavyRoutes(t *testing.T) {
	router := web.NewRouter()
	for _, path := range []string{"/api/remote/readiness", "/api/leak-check", "/api/egress/status", "/api/" + join("open", "vpn") + "/status", "/api/proxy/status"} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("removed heavy route %s should be 404, got %d", path, response.Code)
		}
	}
}

func TestCoreInstallUninstallAPIsRequireExplicitSystemChangeConfirmation(t *testing.T) {
	router := web.NewRouter()
	for _, tc := range []struct {
		path string
	}{
		{"/api/xray/install"},
		{"/api/xray/uninstall"},
		{"/api/singbox/install"},
		{"/api/singbox/uninstall"},
	} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{"confirm":true}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s without allow_system_changes = %d, want 403", tc.path, response.Code)
		}
		if !strings.Contains(response.Body.String(), "confirmation_required") {
			t.Fatalf("%s response missing confirmation_required: %s", tc.path, response.Body.String())
		}
	}
}

func TestCoreInstallFailureReturnsStructuredActionResult(t *testing.T) {
	router := web.NewRouter(web.WithCoreScriptRunner(func(script string) ([]byte, error) {
		if !strings.Contains(script, "Xray-linux-${asset_arch}.zip") {
			t.Fatalf("runner received unexpected script: %s", script)
		}
		return []byte("download Xray release failed"), errors.New("download failed")
	}))
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/xray/install", strings.NewReader(`{"confirm":true,"allow_system_changes":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("expected structured install failure response to be 200, got %d: %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"status":"failed"`, `"error":"install_failed"`, "download Xray release"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("install failure response missing %q: %s", want, response.Body.String())
		}
	}
}

func TestCoreXrayInstallScriptVerifiesChecksumBeforeExtracting(t *testing.T) {
	script := webPackageSource(t)
	for _, want := range []string{
		`asset_name="Xray-linux-${asset_arch}.zip"`,
		`url="https://github.com/XTLS/Xray-core/releases/download/v${version}/${asset_name}"`,
		`dgst_url="${url}.dgst"`,
		`curl -fL "$url" -o "$tmp/$asset_name"`,
		`curl -fL "$dgst_url" -o "$tmp/$asset_name.dgst"`,
		`awk -F'= ' -v asset="$asset_name" '/^SHA2-256=/{print $2 "  " asset}' "$tmp/$asset_name.dgst" > "$tmp/$asset_name.sha256"`,
		`sha256sum -c "$asset_name.sha256"`,
		`unzip -q "$tmp/$asset_name" -d "$tmp/xray"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("Xray WebUI install script missing checksum contract %q", want)
		}
	}
	if strings.Index(script, `sha256sum -c "$asset_name.sha256"`) > strings.Index(script, `unzip -q "$tmp/$asset_name" -d "$tmp/xray"`) {
		t.Fatalf("Xray WebUI install script must verify checksum before extracting archive")
	}
}

func TestCoreSingboxInstallScriptVerifiesChecksumBeforeExtracting(t *testing.T) {
	script := webPackageSource(t)
	for _, want := range []string{
		`asset_name="sing-box-${version}-linux-${asset_arch}.tar.gz"`,
		`url="https://github.com/SagerNet/sing-box/releases/download/v${version}/${asset_name}"`,
		`checksums_url="https://github.com/SagerNet/sing-box/releases/download/v${version}/sing-box-${version}-checksums.txt"`,
		`curl -fL "$url" -o "$tmp/$asset_name"`,
		`curl -fL "$checksums_url" -o "$tmp/checksums.txt"`,
		`grep "$asset_name" "$tmp/checksums.txt" > "$tmp/sing-box.tar.gz.sha256"`,
		`sha256sum -c "sing-box.tar.gz.sha256"`,
		`tar -xzf "$tmp/$asset_name" -C "$tmp"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("sing-box WebUI install script missing checksum contract %q", want)
		}
	}
	if strings.Index(script, `sha256sum -c "sing-box.tar.gz.sha256"`) > strings.Index(script, `tar -xzf "$tmp/$asset_name"`) {
		t.Fatalf("sing-box WebUI install script must verify checksum before extracting archive")
	}
}

func TestCoreInstallersDoNotExecuteUnverifiedRemoteScripts(t *testing.T) {
	script := webPackageSource(t)
	for _, forbidden := range []string{
		"get.acme.sh",
		"Xray-install/raw/main/install-release.sh",
		`bash "$tmp/install-release.sh"`,
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("web package must not download and execute unverified remote installer %q", forbidden)
		}
	}
	for _, want := range []string{
		"refusing to download and execute unverified acme.sh installer",
		"download Xray release",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("web package missing safe installer marker %q", want)
		}
	}
}
