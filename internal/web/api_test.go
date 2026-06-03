package web_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/web"
)

func TestInboundsAPIListsStoredInboundsWithClients(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark:   "主入口",
		Protocol: "vless",
		Port:     443,
		Network:  "tcp",
		Security: "reality",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	_, err = store.CreateClient(context.Background(), db.CreateClientParams{InboundID: inbound.ID, Email: "sam@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	router := web.NewRouter(web.WithStore(store))
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/inbounds", nil)
	router.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`"remark":"主入口"`, `"protocol":"vless"`, `"port":443`, `"email":"sam@example.com"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "panel_password") || strings.Contains(body, "super-secret-password") {
		t.Fatalf("inbounds api leaked panel secrets: %s", body)
	}
}

func TestCreateInboundAPIStoresInbound(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	router := web.NewRouter(web.WithStore(store))
	payload := []byte(`{"remark":"新入口","protocol":"trojan","port":9443,"network":"tcp","security":"tls"}`)
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/inbounds", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`"remark":"新入口"`, `"protocol":"trojan"`, `"port":9443`, `"enabled":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("create response missing %q: %s", want, body)
		}
	}

	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(inbounds) != 1 || inbounds[0].Remark != "新入口" || inbounds[0].Protocol != "trojan" {
		t.Fatalf("inbound was not persisted: %+v", inbounds)
	}
}

func TestCreateInboundAPIRejectsUnsupportedProtocol(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	router := web.NewRouter(web.WithStore(store))
	payload := []byte(`{"remark":"legacy","protocol":"openvpn","port":1194,"network":"udp"}`)
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/inbounds", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "unsupported_protocol") {
		t.Fatalf("expected unsupported_protocol body, got: %s", response.Body.String())
	}
	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(inbounds) != 0 {
		t.Fatalf("unsupported inbound should not persist: %+v", inbounds)
	}
}

func TestCreateClientAPIStoresClientUnderInbound(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{Remark: "vless", Protocol: "vless", Port: 443, Network: "tcp", Security: "reality"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	router := web.NewRouter(web.WithStore(store))
	payload := []byte(`{"email":"client@example.com"}`)
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/inbounds/"+strconv.FormatInt(inbound.ID, 10)+"/clients", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`"email":"client@example.com"`, `"enabled":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("create client response missing %q: %s", want, body)
		}
	}
	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(inbounds) != 1 || len(inbounds[0].Clients) != 1 || inbounds[0].Clients[0].Email != "client@example.com" {
		t.Fatalf("client was not persisted under inbound: %+v", inbounds)
	}
}

func TestCreateClientAPIRejectsUnknownInbound(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	router := web.NewRouter(web.WithStore(store))
	payload := []byte(`{"email":"ghost@example.com"}`)
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/inbounds/999/clients", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", response.Code, response.Body.String())
	}
}
