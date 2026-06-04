package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthIsDisabledByDefault(t *testing.T) {
	router := NewRouter()
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 with no auth, got %d", response.Code)
	}
}

func TestAuthRejectsUnauthenticatedAccess(t *testing.T) {
	router := NewRouter(WithAuth("admin", "secret"))
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session cookie, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAuthAPIEndpointsRequireSession(t *testing.T) {
	router := NewRouter(WithAuth("admin", "secret"))
	for _, path := range []string{"/api/inbounds", "/api/clients", "/api/xray/config", "/api/xray/apply", "/api/xray/status"} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for %s without auth, got %d", path, response.Code)
		}
	}
}

func TestAuthLoginPagesArePublic(t *testing.T) {
	router := NewRouter(WithAuth("admin", "secret"))
	for _, path := range []string{"/login", "/api/health"} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200 for public path %s, got %d", path, response.Code)
		}
	}
}

func TestAuthLoginRejectsWrongCredentials(t *testing.T) {
	router := NewRouter(WithAuth("admin", "secret"))

	body := `{"username":"admin","password":"wrong"}`
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAuthLoginSucceedsWithValidCredentials(t *testing.T) {
	router := NewRouter(WithAuth("admin", "secret"))

	body := `{"username":"admin","password":"secret"}`
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid login, got %d: %s", response.Code, response.Body.String())
	}

	// Response should set a session cookie
	cookies := response.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "migate_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie 'migate_session' in response")
	}
	if sessionCookie.HttpOnly == false {
		t.Error("session cookie should be HttpOnly")
	}
	if sessionCookie.Value == "" {
		t.Error("session cookie value should not be empty")
	}

	// Use the session cookie to access a protected route
	protected := httptest.NewRecorder()
	protectedReq := httptest.NewRequest(http.MethodGet, "/", nil)
	protectedReq.AddCookie(sessionCookie)
	router.ServeHTTP(protected, protectedReq)
	if protected.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid session cookie, got %d: %s", protected.Code, protected.Body.String())
	}
}

func TestAuthLoginPageContainsLoginForm(t *testing.T) {
	router := NewRouter(WithAuth("admin", "secret"))
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	router.ServeHTTP(response, req)
	body := response.Body.String()
	for _, want := range []string{"login", "password", "submit"} {
		if !strings.Contains(strings.ToLower(body), want) {
			t.Fatalf("login page missing %q: %s", want, body)
		}
	}
}

func TestAuthLogoutClearsSession(t *testing.T) {
	router := NewRouter(WithAuth("admin", "secret"))

	// First login
	loginBody := `{"username":"admin","password":"secret"}`
	loginResp := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader([]byte(loginBody)))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginResp, loginReq)

	cookies := loginResp.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "migate_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("login should set session cookie")
	}

	// Then logout
	logoutResp := httptest.NewRecorder()
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	router.ServeHTTP(logoutResp, logoutReq)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("expected 200 on logout, got %d", logoutResp.Code)
	}

	// Verify cookie is cleared (max-age = 0 or empty value)
	logoutCookies := logoutResp.Result().Cookies()
	var cleared bool
	for _, c := range logoutCookies {
		if c.Name == "migate_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout should clear migate_session cookie")
	}
}

func TestAuthHealthEndpointDoesNotRequireAuthEvenWhenAuthEnabled(t *testing.T) {
	// This test is already in TestAuthLoginPagesArePublic, but let's be explicit
	router := NewRouter(WithAuth("admin", "secret"))
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("health should be public, got %d", response.Code)
	}
}

// registerWithAuthTestImports ensures unused import doesn't cause issues
var _ = context.Background
var _ = json.Marshal
