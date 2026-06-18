package db

import (
	"context"
	"database/sql"
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
	for _, name := range []string{"idx_clients_inbound_email", "idx_clients_credential_id", "idx_inbounds_port", "idx_traffic_samples_lookup", "idx_traffic_samples_scope_time", "idx_traffic_samples_sampled_at"} {
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

func TestStoreInitializesStrictRoutingRuleSchema(t *testing.T) {
	store, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if !indexIsUnique(t, store, "inbounds", "idx_inbounds_port") {
		t.Fatal("expected idx_inbounds_port to be unique in new schema")
	}
	if !indexIsUnique(t, store, "clients", "idx_clients_credential_id") {
		t.Fatal("expected idx_clients_credential_id to be unique in new schema")
	}
	columns := routingRuleColumnNullability(t, store)
	for _, name := range []string{"inbound_id", "client_id"} {
		if columns[name] {
			t.Fatalf("expected routing_rules.%s to be nullable, got NOT NULL", name)
		}
	}
	if !columns["outbound_id"] {
		t.Fatal("expected routing_rules.outbound_id to be NOT NULL")
	}
	foreignKeys := routingRuleForeignKeys(t, store)
	for _, want := range []string{"outbound_id->outbounds.id", "client_id->clients.id", "inbound_id->inbounds.id"} {
		if !foreignKeys[want] {
			t.Fatalf("expected routing_rules foreign key %s, got %#v", want, foreignKeys)
		}
	}
}

func routingRuleColumnNullability(t *testing.T, store *Store) map[string]bool {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), `PRAGMA table_info(routing_rules)`)
	if err != nil {
		t.Fatalf("table info: %v", err)
	}
	defer rows.Close()
	notNullByName := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info: %v", err)
		}
		notNullByName[name] = notNull != 0
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table info rows: %v", err)
	}
	return notNullByName
}

func routingRuleForeignKeys(t *testing.T, store *Store) map[string]bool {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), `PRAGMA foreign_key_list(routing_rules)`)
	if err != nil {
		t.Fatalf("foreign key list: %v", err)
	}
	defer rows.Close()
	keys := map[string]bool{}
	for rows.Next() {
		var id int
		var seq int
		var table string
		var from string
		var to string
		var onUpdate string
		var onDelete string
		var match string
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan foreign key: %v", err)
		}
		keys[from+"->"+table+"."+to] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign key rows: %v", err)
	}
	return keys
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
