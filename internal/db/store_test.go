package db_test

import (
	"context"
	"testing"

	"github.com/imzyb/MiGate/internal/db"
)

func TestStoreMigratesAndCreatesInboundWithClients(t *testing.T) {
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
	if inbound.ID == 0 || inbound.UUID == "" || inbound.Enabled != true {
		t.Fatalf("unexpected inbound: %+v", inbound)
	}

	client, err := store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID,
		Email:     "sam@example.com",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if client.ID == 0 || client.UUID == "" || client.Enabled != true {
		t.Fatalf("unexpected client: %+v", client)
	}

	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(inbounds) != 1 || len(inbounds[0].Clients) != 1 {
		t.Fatalf("expected inbound with one client, got %+v", inbounds)
	}
	if inbounds[0].Clients[0].Email != "sam@example.com" {
		t.Fatalf("unexpected client email: %+v", inbounds[0].Clients[0])
	}
}

func TestStoreRejectsUnsupportedProtocol(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.CreateInbound(context.Background(), db.CreateInboundParams{
		Protocol: "http",
		Port:     8080,
	})
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
}

func TestStoreDeletesClient(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "test", Protocol: "vless", Port: 443, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID, Email: "del@test.com",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := store.DeleteClient(context.Background(), client.ID); err != nil {
		t.Fatalf("delete client: %v", err)
	}

	// Verify client is gone
	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	for _, ib := range inbounds {
		for _, c := range ib.Clients {
			if c.ID == client.ID {
				t.Fatalf("client %d still present after deletion", client.ID)
			}
		}
	}
}

func TestStoreDeletesInboundAndCascadesClients(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "to-delete", Protocol: "vmess", Port: 8443, Network: "ws", Security: "none",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	_, err = store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID, Email: "orphan@test.com",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := store.DeleteInbound(context.Background(), inbound.ID); err != nil {
		t.Fatalf("delete inbound: %v", err)
	}

	// Verify inbound and its clients are gone
	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	for _, ib := range inbounds {
		if ib.ID == inbound.ID {
			t.Fatalf("inbound %d still present after deletion", inbound.ID)
		}
	}
}

func TestStoreDeleteInboundRejectsUnknownID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.DeleteInbound(context.Background(), 99999); err == nil {
		t.Fatal("expected error when deleting non-existent inbound")
	}
}

func TestStoreDeleteClientRejectsUnknownID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.DeleteClient(context.Background(), 99999); err == nil {
		t.Fatal("expected error when deleting non-existent client")
	}
}
