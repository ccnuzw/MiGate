package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"
)

// WithAuth enables panel login authentication. Requests to most routes must
// carry a valid session cookie obtained via POST /api/login.
func WithAuth(username, password string) Option {
	return func(cfg *routerConfig) {
		cfg.authEnabled = true
		cfg.authUsername = username
		cfg.authPassword = password
		secret := make([]byte, 32)
		_, _ = rand.Read(secret)
		cfg.sessionSecret = secret
	}
}

// hashToken returns the BLAKE2b-256 hex-encoded hash of the session token.
func hashToken(token string) string {
	h := blake2b.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// authMiddleware wraps an http.Handler and checks the session cookie for all
// non-public routes when auth is enabled. It also verifies the session has
// not been revoked via the token blacklist.
func authMiddleware(next http.Handler, cfg *routerConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.authEnabled {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		// Public paths that do not need auth
		if path == "/login" || path == "/api/health" || path == "/api/login" || path == "/api/session" || path == "/api/singbox/status" || path == "/api/singbox/version" || strings.HasPrefix(path, "/sub/") || strings.HasPrefix(path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		// Check session cookie
		cookie, err := r.Cookie("migate_session")
		if err != nil || !validateSessionToken(cookie.Value, cfg.sessionSecret) {
			if path == "/" && r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(loginPageHTML)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		// Check if session token has been revoked
		if cfg.store != nil {
			revoked, err := cfg.store.IsBlacklisted(r.Context(), hashToken(cookie.Value))
			if err == nil && revoked {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "session_revoked"})
				return
			}
			// Update last_used timestamp
			if err == nil {
				_ = cfg.store.RecordSessionTouch(r.Context(), hashToken(cookie.Value))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func createSessionToken(username string, secret []byte) string {
	// Generate a random nonce to ensure unique tokens per login
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	expiry := time.Now().Add(7 * 24 * time.Hour).Unix()
	payload := fmt.Sprintf("%s:%d:%s", username, expiry, hex.EncodeToString(nonce))
	sig := signMessage(payload, secret)
	return hex.EncodeToString([]byte(payload)) + "." + sig
}

func validateSessionToken(token string, secret []byte) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payloadBytes, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	payload := string(payloadBytes)
	// Payload format: username:expiry or username:expiry:nonce
	// Find the expiry from the second colon-delimited field
	fields := strings.SplitN(payload, ":", 3)
	if len(fields) < 2 {
		return false
	}
	expiry, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	expectedSig := signMessage(payload, secret)
	return hmac.Equal([]byte(parts[1]), []byte(expectedSig))
}

func signMessage(msg string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// loginHandler serves the login page HTML and handles POST /api/login.
func loginHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(loginPageHTML)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request"})
			return
		}
		if !constantTimeStringEqual(req.Username, cfg.authUsername) || !constantTimeStringEqual(req.Password, cfg.authPassword) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_credentials"})
			return
		}
		token := createSessionToken(req.Username, cfg.sessionSecret)
		cookiePath := cfg.basePath
		if cookiePath == "" {
			cookiePath = "/"
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "migate_session",
			Value:    token,
			Path:     cookiePath,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400 * 7,
		})

		// Track the new session in the token_blacklist table (revoked=0 = active)
		if cfg.store != nil {
			_ = cfg.store.AddToBlacklist(r.Context(), hashToken(token), time.Now().Add(7*24*time.Hour), false)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func constantTimeStringEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// logoutHandler handles POST /api/logout by clearing the session cookie and
// adding the session token hash to the blacklist.
func logoutHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cookiePath := cfg.basePath
		if cookiePath == "" {
			cookiePath = "/"
		}

		// Add the session token to the blacklist before clearing
		if cookie, err := r.Cookie("migate_session"); err == nil && cookie.Value != "" {
			if cfg.store != nil {
				_ = cfg.store.AddToBlacklist(r.Context(), hashToken(cookie.Value), time.Now().Add(7*24*time.Hour), true)
			}
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "migate_session",
			Value:    "",
			Path:     cookiePath,
			HttpOnly: true,
			MaxAge:   -1,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
	}
}

// sessionHandler reports the authentication status. It is public (no auth
// middleware check) but still validates the cookie directly.
func sessionHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := map[string]interface{}{
			"auth_enabled":     cfg.authEnabled,
			"authenticated":    false,
			"username":         "",
			"revoked":          false,
			"default_password": false,
		}
		if !cfg.authEnabled {
			resp["username"] = "未启用认证"
		} else if cookie, err := r.Cookie("migate_session"); err == nil && validateSessionToken(cookie.Value, cfg.sessionSecret) {
			// Check blacklist
			revoked := false
			if cfg.store != nil {
				r, err := cfg.store.IsBlacklisted(r.Context(), hashToken(cookie.Value))
				if err == nil {
					revoked = r
				}
			}
			if !revoked {
				resp["authenticated"] = true
				resp["username"] = cfg.authUsername
				resp["default_password"] = cfg.authPassword == "admin"
			}
			resp["revoked"] = revoked
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// sessionsListHandler returns a list of active sessions (GET /api/sessions).
func sessionsListHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if cfg.store == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		sessions, err := cfg.store.ListActiveSessions(r.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "list_sessions_failed"})
			return
		}
		// Return a safe representation: only the hash prefix, created_at, last_used
		type sessionResponse struct {
			ID        int64  `json:"id"`
			IDPrefix  string `json:"id_prefix"`
			CreatedAt string `json:"created_at"`
			LastUsed  string `json:"last_used"`
			ExpiresAt string `json:"expires_at"`
		}
		result := make([]sessionResponse, 0, len(sessions))
		for _, s := range sessions {
			prefix := s.TokenHash
			if len(prefix) > 8 {
				prefix = prefix[:8]
			}
			result = append(result, sessionResponse{
				ID:        s.ID,
				IDPrefix:  prefix,
				CreatedAt: s.CreatedAt,
				LastUsed:  s.LastUsed,
				ExpiresAt: s.ExpiresAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

// sessionRevokeHandler handles DELETE /api/sessions/{id} to revoke a specific session.
func sessionRevokeHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		path = strings.TrimSuffix(path, "/")
		id, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_session_id"})
			return
		}
		if cfg.store == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
			return
		}
		if err := cfg.store.RevokeSession(r.Context(), id); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "session_not_found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
	}
}

// loginPageHTML is a self-contained login form served at /login.
// Vercel-style design with Geist font, CSS variables, light/dark support, and mobile responsive.
var loginPageHTML = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>MiGate - Login</title>
<link href="https://fonts.googleapis.com/css2?family=Geist:wght@300;400;500;600&family=Geist+Mono:wght@400;500&display=swap" rel="stylesheet">
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{--bg:#ffffff;--fg:#171717;--surface:#ffffff;--surface-subtle:#fafafa;--muted:#666666;--line:rgba(0,0,0,.08);--line-strong:#ebebeb;--accent:#171717;--danger:#dc2626;--focus:hsla(212,100%,48%,1);--shadow-sm:0 0 0 1px rgba(0,0,0,.08);--shadow-md:0 0 0 1px rgba(0,0,0,.08),0 2px 2px rgba(0,0,0,.04),0 8px 8px -8px rgba(0,0,0,.04);--radius-sm:6px;--radius-lg:12px;--space-4:16px;--space-5:20px;--space-6:24px;--text-sm:13px;--text-md:14px;--control-height:40px}
:root[data-theme="dark"]{--bg:#0a0a0a;--fg:#ededed;--surface:#111111;--surface-subtle:#18181b;--muted:#a1a1aa;--line:rgba(255,255,255,.10);--line-strong:rgba(255,255,255,.14);--accent:#ededed;--danger:#ef4444;--focus:rgba(99,102,241,.36);--shadow-sm:0 0 0 1px rgba(255,255,255,.10);--shadow-md:0 0 0 1px rgba(255,255,255,.10),0 12px 28px rgba(0,0,0,0)}
body{font-family:'Geist',system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--fg);display:flex;align-items:center;justify-content:center;min-height:100vh;padding:var(--space-4)}
.login-card{background:var(--surface);border:none;border-radius:var(--radius-lg);padding:var(--space-6);width:360px;max-width:100%;box-shadow:var(--shadow-md)}
.login-card h1{font-size:22px;font-weight:600;margin-bottom:4px;text-align:center;color:var(--fg)}
.login-card p{color:var(--muted);font-size:var(--text-sm);text-align:center;margin-bottom:var(--space-6);line-height:1.5}
.form-group{display:grid;gap:6px;margin-bottom:var(--space-4)}
.form-group label{font-size:var(--text-sm);font-weight:500;color:var(--fg)}
.form-group input{width:100%;min-height:var(--control-height);padding:0 12px;background:var(--bg);border:none;border-radius:var(--radius-sm);color:var(--fg);font-size:var(--text-md);font-family:inherit;outline:none;transition:box-shadow .15s;box-shadow:var(--shadow-sm)}
.form-group input:focus{box-shadow:var(--shadow-sm),0 0 0 2px var(--focus)}
button{width:100%;min-height:var(--control-height);padding:0 16px;background:var(--accent);color:var(--bg);border:none;border-radius:var(--radius-sm);font-size:var(--text-md);font-weight:500;font-family:inherit;cursor:pointer;transition:opacity .15s}
button:hover{opacity:.85}
.error{color:var(--danger);font-size:var(--text-sm);text-align:center;margin-top:var(--space-4);display:none;line-height:1.5}
@media (max-width: 480px){.login-card{padding:var(--space-5)}}
</style>
<script>
const i18n={zh:{overview:"概览",inbounds:"入站",outbound:"出站",routing:"路由",settings:"设置",currentUser:"当前用户",loading:"加载中...",logout:"登出",darkMode:"深色模式",lightMode:"浅色模式",langToggle:"English",serverResources:"服务器资源",cpu:"CPU",memory:"内存",disk:"硬盘",uptime:"开机时长",businessStatus:"业务状态",clients:"客户端",totalTraffic:"总流量",routingRules:"路由规则",runningStatus:"运行状态",protocolDistribution:"协议分布",coreProtocols:"核心协议",newInbound:"新增入站",searchInbound:"搜索入站...",defaultSort:"默认排序",byPort:"按端口",byProtocol:"按协议",byClients:"按客户端数",loadingInbounds:"正在加载入站...",outboundManagement:"出站管理",outboundDesc:"配置链式代理转发（SOCKS5 / HTTP），实现流量经外部代理链路中转。",newOutbound:"新增出站",loadingOutbounds:"正在加载出站...",routingManagement:"路由管理",routingDesc:"配置域名/IP 路由规则，决定匹配流量的出站选择。",newRoute:"新增路由",loadingRoutes:"正在加载路由...",xrayConfig:"Xray 配置",xrayDesc:"Xray 运行状态、生成的配置预览与应用操作。",preview:"预览",apply:"应用",validate:"验证",restart:"重启",reloadConfig:"重载配置",singboxConfig:"Sing-box 配置",singboxDesc:"Sing-box 运行状态、生成的配置预览与应用操作。",panelSettings:"面板设置",panelSettingsDesc:"WebUI 端口、路径、凭据等面板运行参数。",refresh:"刷新",saveSettings:"保存设置",confirmRestart:"确认重启 MiGate 服务？",cancel:"取消",confirm:"确认",name:"名称",protocol:"协议类型",port:"端口",enabled:"启用",actions:"操作",edit:"编辑",delete:"删除",copy:"复制",active:"活跃",total:"总计",usedTotal:"已用 / 总量",systemUptime:"系统运行时间",checking:"检查中...",runningOverview:"运行概况",activeClients:"活跃客户端",noInbounds:"暂无入站",noOutbounds:"暂无出站",noRoutes:"暂无路由",panelLogin:"面板登录",username:"用户名",password:"密码",login:"登录",loginFailed:"登录失败",networkError:"网络错误"},en:{overview:"Overview",inbounds:"Inbounds",outbound:"Outbound",routing:"Routing",settings:"Settings",currentUser:"Current User",loading:"Loading...",logout:"Logout",darkMode:"Dark Mode",lightMode:"Light Mode",langToggle:"中文",serverResources:"Server Resources",cpu:"CPU",memory:"Memory",disk:"Disk",uptime:"Uptime",businessStatus:"Business Status",clients:"Clients",totalTraffic:"Total Traffic",routingRules:"Routing Rules",runningStatus:"Running Status",protocolDistribution:"Protocol Distribution",coreProtocols:"Core Protocols",newInbound:"New Inbound",searchInbound:"Search inbound...",defaultSort:"Default Sort",byPort:"By Port",byProtocol:"By Protocol",byClients:"By Clients",loadingInbounds:"Loading inbounds...",outboundManagement:"Outbound Management",outboundDesc:"Configure chained proxy forwarding (SOCKS5 / HTTP) to relay traffic through external proxy chains.",newOutbound:"New Outbound",loadingOutbounds:"Loading outbounds...",routingManagement:"Routing Management",routingDesc:"Configure domain/IP routing rules to determine outbound selection for matched traffic.",newRoute:"New Route",loadingRoutes:"Loading routes...",xrayConfig:"Xray Config",xrayDesc:"Xray running status, generated config preview and apply operations.",preview:"Preview",apply:"Apply",validate:"Validate",restart:"Restart",reloadConfig:"Reload Config",singboxConfig:"Sing-box Config",singboxDesc:"Sing-box running status, generated config preview and apply operations.",panelSettings:"Panel Settings",panelSettingsDesc:"WebUI port, path, credentials and other panel runtime parameters.",refresh:"Refresh",saveSettings:"Save Settings",confirmRestart:"Confirm restart MiGate service?",cancel:"Cancel",confirm:"Confirm",name:"Name",protocol:"Protocol",port:"Port",enabled:"Enabled",actions:"Actions",edit:"Edit",delete:"Delete",copy:"Copy",active:"Active",total:"Total",usedTotal:"Used / Total",systemUptime:"System Uptime",checking:"Checking...",runningOverview:"Running Overview",activeClients:"Active Clients",noInbounds:"No inbounds",noOutbounds:"No outbounds",noRoutes:"No routes",panelLogin:"Panel Login",username:"Username",password:"Password",login:"Login",loginFailed:"Login failed",networkError:"Network error"}};
let currentLang=((document.cookie.match(/migate_lang=([^;]+)/)||[])[1]||'zh');
function t(k){return i18n[currentLang][k]||k}
</script>
<script>
(function(){try{var t=localStorage.getItem('migate-theme')||(window.matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light');document.documentElement.dataset.theme=t}catch(e)})()
</script>
</head>
<body>
<div class="login-card">
<h1>MiGate</h1>
<p><script>document.write(t('panelLogin'))</script></p>
<form id="loginForm">
<div class="form-group"><label for="username"><script>document.write(t('username'))</script></label><input type="text" id="username" name="username" placeholder="admin" autocomplete="username" required></div>
<div class="form-group"><label for="password"><script>document.write(t('password'))</script></label><input type="password" id="password" name="password" placeholder="........" autocomplete="current-password" required></div>
<button type="submit"><script>document.write(t('login'))</script></button>
<div class="error" id="errorMsg"></div>
</form>
</div>
<script>
document.getElementById('loginForm').addEventListener('submit',async function(e){e.preventDefault();const u=document.getElementById('username').value;const p=document.getElementById('password').value;const base=(()=>{let path=window.location.pathname||'/';if(path.endsWith('/login'))path=path.slice(0,-6);if(path.endsWith('/')&&path!=='/')path=path.slice(0,-1);return path==='/'?'':path})();try{const r=await fetch(base+'/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:u,password:p})});if(r.ok){window.location.href=(base||'')+'/'}else{let msg=t('loginFailed');try{const d=await r.json();msg=d.error||msg}catch{}const err=document.getElementById('errorMsg');err.textContent=msg;err.style.display='block'}}catch{const err=document.getElementById('errorMsg');err.textContent=t('networkError');err.style.display='block'}})
</script>
</body>
</html>`)
