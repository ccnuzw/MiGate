package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"
)

const sessionTouchMinAge = time.Minute
const sessionTouchRetention = 7 * 24 * time.Hour
const sessionTouchGCInterval = time.Hour
const maxActiveSessions = 10

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
		if path == "/login" || path == "/" || path == "/api/health" || path == "/api/login" || path == "/api/session" || path == "/api/singbox/status" || path == "/api/singbox/version" || strings.HasPrefix(path, "/sub/") || strings.HasPrefix(path, "/assets/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet && !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/sub/") {
			next.ServeHTTP(w, r)
			return
		}
		// Check session cookie
		cookie, err := r.Cookie("migate_session")
		if err != nil || !validateSessionToken(cookie.Value, cfg.sessionSecret) {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		// Check if session token has been revoked
		if cfg.store != nil {
			tokenHash := hashToken(cookie.Value)
			revoked, err := cfg.store.IsBlacklisted(r.Context(), tokenHash)
			if err == nil && revoked {
				writeJSONError(w, http.StatusUnauthorized, "session_revoked")
				return
			}
			if err == nil {
				if reservedAt, ok := reserveSessionTouch(cfg, tokenHash, sessionTouchMinAge); ok {
					var touchErr error
					if throttled, ok := cfg.store.(sessionTouchThrottler); ok {
						touchErr = throttled.RecordSessionTouchAfter(r.Context(), tokenHash, sessionTouchMinAge)
					} else {
						touchErr = cfg.store.RecordSessionTouch(r.Context(), tokenHash)
					}
					if touchErr != nil {
						rollbackSessionTouch(cfg, tokenHash, reservedAt)
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func reserveSessionTouch(cfg *routerConfig, tokenHash string, minAge time.Duration) (time.Time, bool) {
	now := time.Now()
	cfg.sessionTouchMu.Lock()
	defer cfg.sessionTouchMu.Unlock()
	if cfg.sessionTouches == nil {
		cfg.sessionTouches = make(map[string]time.Time)
	}
	cleanupSessionTouches(cfg, now)
	if last, ok := cfg.sessionTouches[tokenHash]; ok && now.Sub(last) < minAge {
		return time.Time{}, false
	}
	cfg.sessionTouches[tokenHash] = now
	return now, true
}

func rollbackSessionTouch(cfg *routerConfig, tokenHash string, reservedAt time.Time) {
	cfg.sessionTouchMu.Lock()
	defer cfg.sessionTouchMu.Unlock()
	if cfg.sessionTouches != nil {
		if current, ok := cfg.sessionTouches[tokenHash]; ok && current.Equal(reservedAt) {
			delete(cfg.sessionTouches, tokenHash)
		}
	}
}

func cleanupSessionTouches(cfg *routerConfig, now time.Time) {
	if !cfg.sessionTouchGC.IsZero() && now.Sub(cfg.sessionTouchGC) < sessionTouchGCInterval {
		return
	}
	cfg.sessionTouchGC = now
	cutoff := now.Add(-sessionTouchRetention)
	for tokenHash, touchedAt := range cfg.sessionTouches {
		if touchedAt.Before(cutoff) {
			delete(cfg.sessionTouches, tokenHash)
		}
	}
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

// loginHandler handles POST /api/login.
func loginHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		keys := loginRateLimitKeys(r, req.Username, cfg.trustProxy)
		if cfg.loginLimiter != nil && !cfg.loginLimiter.allow(keys...) {
			writeJSONError(w, http.StatusTooManyRequests, "login_rate_limited")
			return
		}
		if !constantTimeStringEqual(req.Username, cfg.authUsername) || !constantTimeStringEqual(req.Password, cfg.authPassword) {
			if cfg.loginLimiter != nil {
				cfg.loginLimiter.recordFailure(keys...)
			}
			writeJSONError(w, http.StatusUnauthorized, "invalid_credentials")
			return
		}
		if cfg.loginLimiter != nil {
			cfg.loginLimiter.reset(keys...)
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
			Secure:   requestIsHTTPS(r, cfg),
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400 * 7,
		})

		// Track the new session in the token_blacklist table (revoked=0 = active)
		if cfg.store != nil {
			if err := cfg.store.AddToBlacklist(r.Context(), hashToken(token), time.Now().Add(7*24*time.Hour), false); err == nil {
				_ = cfg.store.PruneActiveSessions(r.Context(), maxActiveSessions)
			}
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
			methodNotAllowed(w)
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
			Secure:   requestIsHTTPS(r, cfg),
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	}
}

// sessionHandler reports the authentication status. It is public (no auth
// middleware check) but still validates the cookie directly.
func sessionHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
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
		writeJSON(w, http.StatusOK, resp)
	}
}

// sessionsListHandler returns a list of active sessions (GET /api/sessions).
func sessionsListHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if cfg.store == nil {
			writeJSON(w, http.StatusOK, []interface{}{})
			return
		}
		sessions, err := cfg.store.ListActiveSessions(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_sessions_failed")
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
		writeJSON(w, http.StatusOK, result)
	}
}

// sessionRevokeHandler handles DELETE /api/sessions/{id} to revoke a specific session.
func sessionRevokeHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		path = strings.TrimSuffix(path, "/")
		if path == "others" {
			revokeOtherSessions(w, r, cfg)
			return
		}
		id, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_session_id")
			return
		}
		if cfg.store == nil {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		if err := cfg.store.RevokeSession(r.Context(), id); err != nil {
			writeJSONError(w, http.StatusNotFound, "session_not_found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	}
}

func revokeOtherSessions(w http.ResponseWriter, r *http.Request, cfg *routerConfig) {
	if cfg.store == nil {
		writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}
	cookie, err := r.Cookie("migate_session")
	if err != nil || !validateSessionToken(cookie.Value, cfg.sessionSecret) {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	revoked, err := cfg.store.RevokeOtherSessions(r.Context(), hashToken(cookie.Value))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "revoke_sessions_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "revoked", "revoked": revoked})
}
