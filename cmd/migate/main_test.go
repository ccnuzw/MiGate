package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouterFromPanelConfigOpensConfiguredDatabaseStore(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "panel.json")
	databasePath := filepath.Join(tmp, "migate.db")
	config := `{"panel_port":9999,"web_base_path":"/","database_path":"` + databasePath + `"}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	router, cleanup, err := routerFromConfig(configPath)
	if err != nil {
		t.Fatalf("router from config: %v", err)
	}
	defer cleanup()

	payload := []byte(`{"remark":"真机入口","protocol":"vless","port":8443,"network":"tcp","security":"reality"}`)
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/inbounds", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected configured store to create inbound, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"remark":"真机入口"`) {
		t.Fatalf("create response missing inbound: %s", response.Body.String())
	}
}

func TestRouterFromPanelConfigEnablesAuthWhenCredentialsPresent(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "panel.json")
	config := `{"panel_port":9999,"panel_username":"admin","panel_password":"secret","database_path":"` + filepath.Join(tmp, "migate.db") + `"}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	router, cleanup, err := routerFromConfig(configPath)
	if err != nil {
		t.Fatalf("router from config: %v", err)
	}
	defer cleanup()

	// Without cookie -> 401
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", response.Code)
	}

	// Login -> 200 with cookie
	loginResp := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader([]byte(`{"username":"admin","password":"secret"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected 200 login, got %d", loginResp.Code)
	}

	cookies := loginResp.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "migate_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// With cookie -> 200
	authResp := httptest.NewRecorder()
	authReq := httptest.NewRequest(http.MethodGet, "/", nil)
	authReq.AddCookie(sessionCookie)
	router.ServeHTTP(authResp, authReq)
	if authResp.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid cookie, got %d", authResp.Code)
	}
}

func TestRouterFromPanelConfigMountsConfiguredWebBasePath(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "panel_base_path.json")
	config := `{"panel_port":9999,"panel_username":"admin","panel_password":"secret","web_base_path":"/migate","database_path":"` + filepath.Join(tmp, "migate.db") + `"}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	router, cleanup, err := routerFromConfig(configPath)
	if err != nil {
		t.Fatalf("router from config: %v", err)
	}
	defer cleanup()

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/migate/login", want: http.StatusOK},
		{path: "/migate/api/health", want: http.StatusOK},
		{path: "/migate", want: http.StatusUnauthorized},
		{path: "/migate/", want: http.StatusUnauthorized},
	} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		router.ServeHTTP(resp, req)
		if resp.Code != tc.want {
			t.Fatalf("%s: expected %d, got %d: %s", tc.path, tc.want, resp.Code, resp.Body.String())
		}
	}
}

func TestRouterFromPanelConfigSkipsAuthWhenNoCredentials(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "panel_noauth.json")
	config := `{"panel_port":9999,"database_path":"` + filepath.Join(tmp, "migate.db") + `"}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	router, cleanup, err := routerFromConfig(configPath)
	if err != nil {
		t.Fatalf("router from config: %v", err)
	}
	defer cleanup()

	// Without cookie -> 200 (auth is off)
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 (no auth) when credentials absent, got %d", response.Code)
	}
}
