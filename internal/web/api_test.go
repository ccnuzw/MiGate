package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
