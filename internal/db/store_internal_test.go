package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStoreCreatesTrafficLookupIndexes(t *testing.T) {
	store, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	indexes := map[string]bool{}
	uniqueIndexes := map[string]bool{}
	rows, err := store.db.QueryContext(context.Background(), `SELECT name, sql FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var ddl sql.NullString
		if err := rows.Scan(&name, &ddl); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		indexes[name] = true
		if ddl.Valid && strings.Contains(strings.ToUpper(ddl.String), "CREATE UNIQUE INDEX") {
			uniqueIndexes[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("index rows: %v", err)
	}
	for _, name := range []string{"idx_clients_inbound_email", "idx_clients_credential_id", "idx_inbounds_port", "idx_traffic_samples_sampled_at"} {
		if !indexes[name] {
			t.Fatalf("expected index %s to exist, got %#v", name, indexes)
		}
	}
	for _, name := range []string{"idx_clients_credential_id", "idx_inbounds_port"} {
		if !uniqueIndexes[name] {
			t.Fatalf("expected index %s to be unique, got %#v", name, uniqueIndexes)
		}
	}
}

func TestTrafficSamplesCleanupIsThrottled(t *testing.T) {
	store, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, CreateInboundParams{Remark: "cleanup", Protocol: "vless", Port: 28096, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, CreateClientParams{InboundID: inbound.ID, Email: "cleanup@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	oldAt := time.Unix(100, 0)
	if err := store.ApplyTrafficRawStats(ctx, []TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 100, RawDown: 100, Status: "ok"}}, oldAt); err != nil {
		t.Fatalf("old baseline: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 110, RawDown: 110, Status: "ok"}}, oldAt.Add(10*time.Second)); err != nil {
		t.Fatalf("old increment: %v", err)
	}
	newAt := oldAt.Add(8 * 24 * time.Hour)
	if err := store.ApplyTrafficRawStats(ctx, []TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 120, RawDown: 120, Status: "ok"}}, newAt); err != nil {
		t.Fatalf("new cleanup trigger: %v", err)
	}
	samples, err := store.ListTrafficSamples(ctx, "client", time.Unix(0, 0), 100)
	if err != nil {
		t.Fatalf("list samples after cleanup trigger: %v", err)
	}
	if len(samples) != 1 || samples[0].SampledAt != newAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("expected first new sample to prune old samples, got %+v", samples)
	}
	staleAt := newAt.Add(-8 * 24 * time.Hour)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO traffic_samples (sampled_at, engine, scope_type, scope_key, total_up, total_down, rate_up, rate_down, status) VALUES (?, 'xray', 'client', ?, 1, 1, 0, 0, 'ok')`, staleAt.UTC().Format(time.RFC3339Nano), client.StatsKey); err != nil {
		t.Fatalf("insert manual stale sample: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 130, RawDown: 130, Status: "ok"}}, newAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("within throttle sample: %v", err)
	}
	samples, err = store.ListTrafficSamples(ctx, "client", time.Unix(0, 0), 100)
	if err != nil {
		t.Fatalf("list samples after throttled write: %v", err)
	}
	foundManualStale := false
	for _, sample := range samples {
		if sample.SampledAt == staleAt.UTC().Format(time.RFC3339Nano) {
			foundManualStale = true
		}
	}
	if !foundManualStale {
		t.Fatalf("expected stale manual sample to remain until cleanup throttle expires, got %+v", samples)
	}
	if err := store.ApplyTrafficRawStats(ctx, []TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 140, RawDown: 140, Status: "ok"}}, newAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("post-throttle sample: %v", err)
	}
	samples, err = store.ListTrafficSamples(ctx, "client", time.Unix(0, 0), 100)
	if err != nil {
		t.Fatalf("list samples after cleanup expiry: %v", err)
	}
	for _, sample := range samples {
		if sample.SampledAt == staleAt.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("expected stale manual sample to be pruned after throttle expiry, got %+v", samples)
		}
	}
}

func TestMigrateOldOutboundSchemaBeforeSeedingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	_, err = raw.Exec(`
CREATE TABLE outbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tag TEXT NOT NULL UNIQUE,
  remark TEXT NOT NULL,
  protocol TEXT NOT NULL,
  address TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 0,
  username TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
)`)
	if err != nil {
		t.Fatalf("create old outbounds table: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("migrate old schema: %v", err)
	}
	defer store.Close()
	outbounds, err := store.ListOutbounds(context.Background())
	if err != nil {
		t.Fatalf("list outbounds after migration: %v", err)
	}
	if len(outbounds) < 3 || outbounds[0].Tag != "direct" || len(outbounds[0].SupportedCores) == 0 {
		t.Fatalf("defaults were not seeded with supported cores: %+v", outbounds)
	}
}

func TestMigrateUpgradesInboundPortIndexToUnique(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old-port-index.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec(`
CREATE TABLE inbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uuid TEXT NOT NULL UNIQUE,
  remark TEXT NOT NULL,
  protocol TEXT NOT NULL,
  core TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL,
  network TEXT NOT NULL,
  security TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_inbounds_port ON inbounds(port);
INSERT INTO inbounds (uuid, remark, protocol, core, port, network, security, enabled, created_at)
VALUES ('legacy-uuid', 'legacy', 'vless', 'xray', 18450, 'tcp', 'none', 1, 'now');
`); err != nil {
		t.Fatalf("create old inbounds: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer store.Close()
	if !indexIsUnique(t, store, "inbounds", "idx_inbounds_port") {
		t.Fatal("expected legacy idx_inbounds_port to be upgraded to a unique index")
	}
}

func TestMigrateRejectsDuplicateInboundPortsWhenUpgradingIndex(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "duplicate-port.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec(`
CREATE TABLE inbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uuid TEXT NOT NULL UNIQUE,
  remark TEXT NOT NULL,
  protocol TEXT NOT NULL,
  core TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL,
  network TEXT NOT NULL,
  security TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_inbounds_port ON inbounds(port);
INSERT INTO inbounds (uuid, remark, protocol, core, port, network, security, enabled, created_at)
VALUES
  ('legacy-uuid-a', 'legacy-a', 'vless', 'xray', 18451, 'tcp', 'none', 1, 'now'),
  ('legacy-uuid-b', 'legacy-b', 'vmess', 'xray', 18451, 'ws', 'tls', 1, 'now');
`); err != nil {
		t.Fatalf("create duplicate-port db: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(ctx, path)
	if err == nil {
		_ = store.Close()
		t.Fatal("expected duplicate inbound ports to fail migration")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("expected unique constraint error, got %v", err)
	}
}

func TestMigrateUpgradesClientCredentialIDIndexToUnique(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old-credential-index.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec(`
CREATE TABLE clients (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  inbound_id INTEGER NOT NULL,
  uuid TEXT NOT NULL UNIQUE,
  credential_id TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  subscription_token TEXT NOT NULL DEFAULT '',
  stats_key TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  up INTEGER NOT NULL DEFAULT 0,
  down INTEGER NOT NULL DEFAULT 0,
  traffic_limit INTEGER NOT NULL DEFAULT 0,
  expiry_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_clients_credential_id ON clients(credential_id);
INSERT INTO clients (inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, created_at)
VALUES (1, 'client-uuid', 'client-credential', 'secret', 'sub-token', 'stats-key', 'client@example.com', 1, 'now');
`); err != nil {
		t.Fatalf("create old clients: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer store.Close()
	if !indexIsUnique(t, store, "clients", "idx_clients_credential_id") {
		t.Fatal("expected legacy idx_clients_credential_id to be upgraded to a unique index")
	}
}

func indexIsUnique(t *testing.T, store *Store, table string, indexName string) bool {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), `PRAGMA index_list(`+table+`)`)
	if err != nil {
		t.Fatalf("index list: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index list: %v", err)
		}
		if name == indexName {
			return unique != 0
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("index rows: %v", err)
	}
	return false
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
