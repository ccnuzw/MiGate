package db

import (
	"context"
	"path/filepath"
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

func TestStoreMigratesRoutingRuleClientColumns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE routing_rules`); err != nil {
		t.Fatalf("drop routing_rules: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
CREATE TABLE routing_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  inbound_tag TEXT NOT NULL DEFAULT '',
  outbound_tag TEXT NOT NULL,
  domain TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort INTEGER NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatalf("create legacy routing_rules: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_rules (inbound_tag, outbound_tag, domain, protocol, enabled, sort) VALUES ('edge', 'direct', 'example.com', 'dns', 1, 0)`); err != nil {
		t.Fatalf("insert legacy routing rule: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer migrated.Close()

	rules, err := migrated.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("list migrated routing rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected legacy rule to survive migration, got %+v", rules)
	}
	if rules[0].ClientID != 0 || rules[0].ClientEmail != "" || rules[0].IP != "" || rules[0].RuleSet != "" {
		t.Fatalf("expected migrated client fields to default empty: %+v", rules[0])
	}
}

func TestStoreMigratesLegacyClientsBeforeStatsKeyIndex(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-clients.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE clients`); err != nil {
		t.Fatalf("drop clients: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO inbounds (id, uuid, remark, protocol, port, network, security, enabled, created_at) VALUES (1, 'legacy-inbound-uuid', 'legacy', 'vless', 443, 'tcp', 'none', 1, 'now')`); err != nil {
		t.Fatalf("insert legacy inbound: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
CREATE TABLE clients (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  inbound_id INTEGER NOT NULL,
  uuid TEXT NOT NULL UNIQUE,
  subscription_token TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  up INTEGER NOT NULL DEFAULT 0,
  down INTEGER NOT NULL DEFAULT 0,
  traffic_limit INTEGER NOT NULL DEFAULT 0,
  expiry_at INTEGER NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatalf("create legacy clients: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO clients (inbound_id, uuid, subscription_token, email, enabled, created_at, up, down, traffic_limit) VALUES (1, 'legacy-uuid', 'token', 'legacy@example.com', 1, 'now', 12, 34, 40)`); err != nil {
		t.Fatalf("insert legacy client: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer migrated.Close()
	rows, err := migrated.db.QueryContext(ctx, `SELECT id, stats_key, up, down, traffic_limit FROM clients`)
	if err != nil {
		t.Fatalf("query migrated clients: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected migrated client row")
	}
	var id int64
	var statsKey string
	var up int64
	var down int64
	var trafficLimit int64
	if err := rows.Scan(&id, &statsKey, &up, &down, &trafficLimit); err != nil {
		t.Fatalf("scan migrated client: %v", err)
	}
	if statsKey == "" || up != 12 || down != 34 || trafficLimit != 40 {
		t.Fatalf("unexpected migrated client stats_key=%q up=%d down=%d limit=%d", statsKey, up, down, trafficLimit)
	}
	usage, found, err := migrated.GetClientTrafficUsageForClient(ctx, id)
	if err != nil {
		t.Fatalf("legacy usage after migration: %v", err)
	}
	if !found || usage.TotalUp+usage.TotalDown < trafficLimit || usage.Engine != "migate" || usage.Status != "cumulative_only" {
		t.Fatalf("expected migrated legacy usage to exceed limit, found=%v usage=%+v limit=%d", found, usage, trafficLimit)
	}
}
