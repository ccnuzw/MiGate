package db

import (
	"context"
	"testing"
)

func TestStoreCreatesTrafficLookupIndexes(t *testing.T) {
	store, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	indexes := map[string]bool{}
	rows, err := store.db.QueryContext(context.Background(), `SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		indexes[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("index rows: %v", err)
	}
	for _, name := range []string{"idx_clients_inbound_email", "idx_inbounds_port"} {
		if !indexes[name] {
			t.Fatalf("expected index %s to exist, got %#v", name, indexes)
		}
	}
}

func TestStoreLightweightInboundQueries(t *testing.T) {
	store, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), CreateInboundParams{
		Remark: "edge", Protocol: "vless", Port: 443, Network: "tcp", Security: "reality",
		RealityPrivateKey: "private-key", TLSCertFile: "/etc/cert.pem",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if _, err := store.CreateClient(context.Background(), CreateClientParams{InboundID: inbound.ID, Email: "sam@example.com"}); err != nil {
		t.Fatalf("create client: %v", err)
	}

	exists, err := store.InboundExists(context.Background(), inbound.ID)
	if err != nil || !exists {
		t.Fatalf("expected inbound to exist, exists=%v err=%v", exists, err)
	}
	conflict, ok, err := store.FindInboundByPort(context.Background(), 443, 0)
	if err != nil || !ok || conflict.ID != inbound.ID {
		t.Fatalf("expected port conflict, conflict=%+v ok=%v err=%v", conflict, ok, err)
	}
	if _, ok, err := store.FindInboundByPort(context.Background(), 443, inbound.ID); err != nil || ok {
		t.Fatalf("expected excluded port conflict to be ignored, ok=%v err=%v", ok, err)
	}

	light, err := store.ListInboundTraffic(context.Background())
	if err != nil {
		t.Fatalf("list inbound traffic: %v", err)
	}
	if len(light) != 1 || len(light[0].Clients) != 1 {
		t.Fatalf("unexpected traffic snapshot: %+v", light)
	}
	if light[0].RealityPrivateKey != "" || light[0].TLSCertFile != "" {
		t.Fatalf("traffic snapshot should omit full config fields: %+v", light[0])
	}
}
