package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
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

// authMiddleware wraps an http.Handler and checks the session cookie for all
// non-public routes when auth is enabled.
func authMiddleware(next http.Handler, cfg *routerConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.authEnabled {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		// Public paths that do not need auth
		if path == "/login" || path == "/api/health" || path == "/api/login" {
			next.ServeHTTP(w, r)
			return
		}
		// Check session cookie
		cookie, err := r.Cookie("migate_session")
		if err != nil || !validateSessionToken(cookie.Value, cfg.sessionSecret) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func createSessionToken(username string, secret []byte) string {
	expiry := time.Now().Add(24 * time.Hour).Unix()
	payload := fmt.Sprintf("%s:%d", username, expiry)
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
	idx := strings.LastIndex(payload, ":")
	if idx < 0 {
		return false
	}
	expiry, err := strconv.ParseInt(payload[idx+1:], 10, 64)
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request"})
			return
		}
		if req.Username != cfg.authUsername || req.Password != cfg.authPassword {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_credentials"})
			return
		}
		token := createSessionToken(req.Username, cfg.sessionSecret)
		http.SetCookie(w, &http.Cookie{
			Name:     "migate_session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// logoutHandler handles POST /api/logout by clearing the session cookie.
func logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "migate_session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
	}
}

// loginPageHTML is a self-contained login form served at /login.
var loginPageHTML = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>MiGate - Login</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f1117;color:#e4e4e7;display:flex;align-items:center;justify-content:center;min-height:100vh}
.login-card{background:#1a1b23;border:1px solid #27272a;border-radius:12px;padding:40px;width:360px;box-shadow:0 4px 24px rgba(0,0,0,.4)}
.login-card h1{font-size:22px;font-weight:600;margin-bottom:8px;text-align:center}
.login-card p{color:#71717a;font-size:14px;text-align:center;margin-bottom:28px}
.form-group{margin-bottom:20px}
.form-group label{display:block;font-size:13px;font-weight:500;color:#a1a1aa;margin-bottom:6px}
.form-group input{width:100%;padding:10px 14px;background:#0f1117;border:1px solid #27272a;border-radius:8px;color:#e4e4e7;font-size:14px;outline:none;transition:border-color .2s}
.form-group input:focus{border-color:#6366f1}
button{width:100%;padding:10px;background:#6366f1;color:#fff;border:none;border-radius:8px;font-size:14px;font-weight:500;cursor:pointer;transition:background .2s}
button:hover{background:#4f46e5}
.error{color:#ef4444;font-size:13px;text-align:center;margin-top:12px;display:none}
</style>
</head>
<body>
<div class="login-card">
<h1>MiGate</h1>
<p>面板登录</p>
<form id="loginForm">
<div class="form-group"><label for="username">用户名</label><input type="text" id="username" name="username" placeholder="admin" autocomplete="username" required></div>
<div class="form-group"><label for="password">密码</label><input type="password" id="password" name="password" placeholder="••••••••" autocomplete="current-password" required></div>
<button type="submit">登录</button>
<div class="error" id="errorMsg"></div>
</form>
</div>
<script>
document.getElementById('loginForm').addEventListener('submit',async function(e){e.preventDefault();const u=document.getElementById('username').value;const p=document.getElementById('password').value;try{const r=await fetch('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:u,password:p})});if(r.ok){window.location.href='/'}else{const d=await r.json();const err=document.getElementById('errorMsg');err.textContent=d.error||'登录失败';err.style.display='block'}}catch{const err=document.getElementById('errorMsg');err.textContent='网络错误';err.style.display='block'}})
</script>
</body>
</html>`)
