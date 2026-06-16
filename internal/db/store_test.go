package db_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

func join(parts ...string) string { return strings.Join(parts, "") }

func TestStoreCreatesAndListsOutboundsWithDefaults(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	outbounds, err := store.ListOutbounds(context.Background())
	if err != nil {
		t.Fatalf("list default outbounds: %v", err)
	}
	if len(outbounds) != 2 {
		t.Fatalf("expected direct and blocked defaults, got %+v", outbounds)
	}
	if outbounds[0].Tag != "direct" || outbounds[0].Protocol != "freedom" || outbounds[0].Sort != 0 {
		t.Fatalf("unexpected first default outbound: %+v", outbounds[0])
	}
	if outbounds[1].Tag != "blocked" || outbounds[1].Protocol != "blackhole" || outbounds[1].Sort != 1 {
		t.Fatalf("unexpected second default outbound: %+v", outbounds[1])
	}

	created, err := store.CreateOutbound(context.Background(), db.CreateOutboundParams{
		Tag:      "proxy-socks",
		Protocol: "socks",
		Address:  "127.0.0.1",
		Port:     1080,
		Username: "sam",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	if created.ID == 0 || created.Tag != "proxy-socks" || created.Protocol != "socks" || created.Address != "127.0.0.1" || created.Port != 1080 || !created.Enabled {
		t.Fatalf("unexpected created outbound: %+v", created)
	}

	outbounds, err = store.ListOutbounds(context.Background())
	if err != nil {
		t.Fatalf("list after create: %v", err)
	}
	if len(outbounds) != 3 || outbounds[2].Tag != "proxy-socks" || outbounds[2].Sort != 2 {
		t.Fatalf("created outbound not appended after defaults: %+v", outbounds)
	}
}

func TestStoreUpdatesOutboundFields(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ob, err := store.CreateOutbound(context.Background(), db.CreateOutboundParams{
		Tag: "proxy-http", Protocol: "http", Address: "10.0.0.1", Port: 8080,
	})
	if err != nil {
		t.Fatalf("create outbound: %v", err)
	}

	updated, err := store.UpdateOutbound(context.Background(), ob.ID, db.UpdateOutboundParams{
		Tag: "proxy-http-v2", Remark: "HTTP代理v2", Protocol: "socks",
		Address: "10.0.0.2", Port: 1080, Username: "newuser", Password: "newpass", Enabled: false,
	})
	if err != nil {
		t.Fatalf("update outbound: %v", err)
	}
	if updated.Tag != "proxy-http-v2" || updated.Remark != "HTTP代理v2" || updated.Protocol != "socks" ||
		updated.Address != "10.0.0.2" || updated.Port != 1080 || updated.Username != "newuser" ||
		updated.Password != "newpass" || updated.Enabled != false || updated.ID != ob.ID {
		t.Fatalf("unexpected updated outbound: %+v", updated)
	}

	loaded, err := store.ListOutbounds(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, o := range loaded {
		if o.ID == ob.ID {
			if o.Tag != "proxy-http-v2" || o.Enabled != false {
				t.Fatalf("updated values not persisted: %+v", o)
			}
		}
	}
}

func TestStoreUpdateOutboundCascadesRoutingRuleTag(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ob, err := store.CreateOutbound(context.Background(), db.CreateOutboundParams{
		Tag: "proxy-old", Protocol: "http", Address: "10.0.0.1", Port: 8080,
	})
	if err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	rule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		OutboundTag: "proxy-old",
		Domain:      "geosite:netflix",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create routing rule: %v", err)
	}

	if _, err := store.UpdateOutbound(context.Background(), ob.ID, db.UpdateOutboundParams{
		Tag: "proxy-new", Protocol: "http", Address: "10.0.0.2", Port: 8081, Enabled: true,
	}); err != nil {
		t.Fatalf("update outbound: %v", err)
	}
	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID || rules[0].OutboundTag != "proxy-new" {
		t.Fatalf("routing rule outbound tag was not cascaded: %+v", rules)
	}
}

func TestStoreUpdateOutboundRejectsUnknownID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.UpdateOutbound(context.Background(), 99999, db.UpdateOutboundParams{Tag: "x", Remark: "x", Protocol: "socks", Address: "1.1.1.1", Port: 80})
	if err == nil {
		t.Fatal("expected error for unknown outbound")
	}
}

func TestStoreDeleteOutboundDeletesOutbound(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ob, err := store.CreateOutbound(context.Background(), db.CreateOutboundParams{
		Tag: "temp-proxy", Protocol: "socks", Address: "10.0.0.1", Port: 1080,
	})
	if err != nil {
		t.Fatalf("create outbound: %v", err)
	}

	if err := store.DeleteOutbound(context.Background(), ob.ID); err != nil {
		t.Fatalf("delete outbound: %v", err)
	}

	outbounds, err := store.ListOutbounds(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, o := range outbounds {
		if o.ID == ob.ID {
			t.Fatalf("outbound %d still present after deletion", ob.ID)
		}
	}
}

func TestStoreDeleteOutboundRejectsUnknownID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.DeleteOutbound(context.Background(), 99999); err == nil {
		t.Fatal("expected error for unknown outbound")
	}
}

func TestStoreReorderOutboundsUpdatesSortOrder(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	// After seeding: direct=1, blocked=2
	o1, _ := store.CreateOutbound(context.Background(), db.CreateOutboundParams{Tag: "p1", Protocol: "socks", Address: "10.0.0.1", Port: 1080})
	o2, _ := store.CreateOutbound(context.Background(), db.CreateOutboundParams{Tag: "p2", Protocol: "http", Address: "10.0.0.2", Port: 3128})
	// Current order: direct(1), blocked(2), p1(3), p2(4)
	// Swap: p2, p1
	err = store.ReorderOutbounds(context.Background(), []int64{o2.ID, o1.ID})
	if err != nil {
		t.Fatalf("reorder outbounds: %v", err)
	}
	list, err := store.ListOutbounds(context.Background())
	if err != nil {
		t.Fatalf("list after reorder: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("expected 4 outbounds, got %d", len(list))
	}
	// Defaults stay first (sort 0-1), then reordered custom outbounds (sort 2-3)
	if list[0].ID != 1 || list[1].ID != 2 || list[2].ID != o2.ID || list[3].ID != o1.ID {
		t.Fatalf("expected defaults then reordered custom: got %d,%d,%d,%d", list[0].ID, list[1].ID, list[2].ID, list[3].ID)
	}
	if list[0].Sort != 0 || list[1].Sort != 1 || list[2].Sort != 2 || list[3].Sort != 3 {
		t.Fatalf("expected sequential sort values: got %d,%d,%d,%d", list[0].Sort, list[1].Sort, list[2].Sort, list[3].Sort)
	}
}

func TestStoreCreatesAndListsRoutingRules(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// No routing rules initially
	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 default routing rules, got %d", len(rules))
	}

	rule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "",
		OutboundTag: "blocked",
		Domain:      "geosite:malware",
		IP:          "geoip:private",
		RuleSet:     "geosite-category-ads-all",
		Protocol:    "bittorrent",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create routing rule: %v", err)
	}
	if rule.OutboundTag != "blocked" || rule.Domain != "geosite:malware" || rule.IP != "geoip:private" || rule.RuleSet != "geosite-category-ads-all" || rule.Protocol != "bittorrent" || !rule.Enabled {
		t.Fatalf("unexpected rule: %+v", rule)
	}

	rules, err = store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID {
		t.Fatalf("expected 1 routing rule, got %+v", rules)
	}
}

func TestStoreUpdateRoutingRule(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	rule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "",
		OutboundTag: "blocked",
		Domain:      "geosite:malware",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := store.UpdateRoutingRule(context.Background(), rule.ID, db.UpdateRoutingRuleParams{
		InboundTag:  "socks-in",
		OutboundTag: "direct",
		Domain:      "geosite:netflix",
		IP:          "8.8.8.8",
		RuleSet:     "geoip-cn",
		Protocol:    "dns",
		Enabled:     false,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.InboundTag != "socks-in" || updated.OutboundTag != "direct" || updated.Domain != "geosite:netflix" || updated.IP != "8.8.8.8" || updated.RuleSet != "geoip-cn" || updated.Protocol != "dns" || updated.Enabled {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 1 || rules[0].Domain != "geosite:netflix" || rules[0].IP != "8.8.8.8" || rules[0].RuleSet != "geoip-cn" || rules[0].Protocol != "dns" {
		t.Fatalf("update not persisted: %+v", rules)
	}
}

func TestStoreRoutingRuleClientFields(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "edge", Protocol: "vless", Port: 443, Network: "tcp", Security: "reality",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(context.Background(), db.CreateClientParams{InboundID: inbound.ID, Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	rule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "edge",
		ClientID:    client.ID,
		OutboundTag: "blocked",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create client routing rule: %v", err)
	}
	wantInboundTag := fmt.Sprintf("inbound-%d-vless", inbound.ID)
	if rule.ClientID != client.ID || rule.ClientEmail != "alice@example.com" || rule.InboundTag != wantInboundTag {
		t.Fatalf("unexpected client routing rule: %+v", rule)
	}

	updated, err := store.UpdateRoutingRule(context.Background(), rule.ID, db.UpdateRoutingRuleParams{
		ClientID:    client.ID,
		OutboundTag: "direct",
		Domain:      "example.com",
		Enabled:     false,
	})
	if err != nil {
		t.Fatalf("update client routing rule: %v", err)
	}
	if updated.ClientID != client.ID || updated.ClientEmail != "alice@example.com" || updated.OutboundTag != "direct" || updated.Enabled {
		t.Fatalf("unexpected updated client routing rule: %+v", updated)
	}

	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	if len(rules) != 1 || rules[0].ClientID != client.ID || rules[0].ClientEmail != "alice@example.com" {
		t.Fatalf("client fields not persisted: %+v", rules)
	}
}

func TestStoreRejectsRoutingRuleClientInboundMismatch(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "edge", Protocol: "vless", Port: 443, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(context.Background(), db.CreateClientParams{InboundID: inbound.ID, Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "other-inbound",
		ClientID:    client.ID,
		OutboundTag: "direct",
		Enabled:     true,
	})
	if err == nil {
		t.Fatal("expected client inbound mismatch to be rejected")
	}
}

func TestStoreRejectsRoutingRuleClientEmailWithoutClientID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "inbound-1-vless",
		ClientEmail: "alice@example.com",
		OutboundTag: "direct",
		Enabled:     true,
	})
	if err == nil {
		t.Fatal("expected client_email without client_id to be rejected")
	}
}

func TestStoreDeleteRoutingRule(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	rule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		OutboundTag: "blocked", Domain: "geosite:malware",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := store.DeleteRoutingRule(context.Background(), rule.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("rule not deleted: %+v", rules)
	}

	if err := store.DeleteRoutingRule(context.Background(), 99999); err == nil {
		t.Fatal("expected error for unknown routing rule")
	}
}

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
	disabled := false
	disabledClient, err := store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID,
		Email:     "disabled@example.com",
		Enabled:   &disabled,
	})
	if err != nil {
		t.Fatalf("create disabled client: %v", err)
	}
	if disabledClient.Enabled != false {
		t.Fatalf("expected disabled client, got %+v", disabledClient)
	}

	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(inbounds) != 1 || len(inbounds[0].Clients) != 2 {
		t.Fatalf("expected inbound with two clients, got %+v", inbounds)
	}
	if inbounds[0].Clients[0].Email != "sam@example.com" {
		t.Fatalf("unexpected client email: %+v", inbounds[0].Clients[0])
	}
	if inbounds[0].Clients[1].Email != "disabled@example.com" || inbounds[0].Clients[1].Enabled {
		t.Fatalf("disabled client not persisted: %+v", inbounds[0].Clients[1])
	}
}

func TestStoreRejectsDuplicateClientEmailAndUUID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "dupe", Protocol: "vless", Port: 443, Network: "tcp", Security: "reality",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	_, err = store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID, Email: "sam@example.com", UUID: "11111111-1111-4111-8111-111111111111",
	})
	if err != nil {
		t.Fatalf("create first client: %v", err)
	}
	if _, err := store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID, Email: "sam@example.com", UUID: "22222222-2222-4222-8222-222222222222",
	}); err == nil || !strings.Contains(err.Error(), "duplicate client email") {
		t.Fatalf("expected duplicate client email error, got %v", err)
	}
	if _, err := store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID, Email: "other@example.com", UUID: "11111111-1111-4111-8111-111111111111",
	}); err == nil || !strings.Contains(err.Error(), "duplicate client uuid") {
		t.Fatalf("expected duplicate client uuid error, got %v", err)
	}
	second, err := store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID, Email: "other@example.com", UUID: "33333333-3333-4333-8333-333333333333",
	})
	if err != nil {
		t.Fatalf("create second client: %v", err)
	}
	if _, err := store.UpdateClient(context.Background(), second.ID, db.UpdateClientParams{
		Email: "sam@example.com", Enabled: true,
	}); err == nil || !strings.Contains(err.Error(), "duplicate client email") {
		t.Fatalf("expected duplicate client email on update, got %v", err)
	}
	if _, err := store.UpdateClient(context.Background(), second.ID, db.UpdateClientParams{
		Email: "other@example.com", UUID: "11111111-1111-4111-8111-111111111111", Enabled: true,
	}); err == nil || !strings.Contains(err.Error(), "duplicate client uuid") {
		t.Fatalf("expected duplicate client uuid on update, got %v", err)
	}
}

func TestStoreRejectsUnsupportedProtocol(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	for _, protocol := range []string{"http", "wireguard"} {
		_, err = store.CreateInbound(context.Background(), db.CreateInboundParams{
			Protocol: protocol,
			Port:     8080,
		})
		if err == nil {
			t.Fatalf("expected error for unsupported protocol %q", protocol)
		}
	}
}

func TestStoreAutoAssignsInboundPort(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	first, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "auto-a", Protocol: "vless", Port: 0, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create first auto-port inbound: %v", err)
	}
	if first.Port < 20000 || first.Port > 60999 {
		t.Fatalf("expected auto-assigned high port, got %+v", first)
	}

	second, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "auto-b", Protocol: "vmess", Port: 0, Network: "ws", Security: "tls",
	})
	if err != nil {
		t.Fatalf("create second auto-port inbound: %v", err)
	}
	if second.Port == first.Port {
		t.Fatalf("auto-assigned duplicate port: first=%+v second=%+v", first, second)
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

func TestStoreUpdateInboundUpdatesFields(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "old", Protocol: "vless", Port: 443, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	updated, err := store.UpdateInbound(context.Background(), inbound.ID, db.UpdateInboundParams{
		Remark:   "new",
		Port:     8443,
		Network:  "ws",
		Security: "tls",
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("update inbound: %v", err)
	}
	if updated.Remark != "new" || updated.Port != 8443 || updated.Network != "ws" || updated.Security != "tls" || updated.Enabled != false {
		t.Fatalf("unexpected updated inbound: %+v", updated)
	}
	if updated.ID != inbound.ID || updated.UUID != inbound.UUID {
		t.Fatalf("id/uuid changed after update: old=%+v new=%+v", inbound, updated)
	}

	loaded, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Remark != "new" || loaded[0].Enabled != false {
		t.Fatalf("updated values not persisted: %+v", loaded[0])
	}
}

func TestStoreUpdateInboundCascadesRoutingRuleInboundTag(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "old-edge", Protocol: "vless", Port: 443, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	remarkRule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "old-edge",
		OutboundTag: "blocked",
		Domain:      "geosite:netflix",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create remark routing rule: %v", err)
	}
	generatedRule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  fmt.Sprintf("inbound-%d-vless", inbound.ID),
		OutboundTag: "direct",
		Domain:      "example.com",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create generated routing rule: %v", err)
	}

	if _, err := store.UpdateInbound(context.Background(), inbound.ID, db.UpdateInboundParams{
		Remark: "new-edge", Protocol: "vmess", Port: 8443, Network: "ws", Security: "tls", Enabled: true,
	}); err != nil {
		t.Fatalf("update inbound: %v", err)
	}
	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	got := map[int64]string{}
	for _, rule := range rules {
		got[rule.ID] = rule.InboundTag
	}
	if got[remarkRule.ID] != "new-edge" {
		t.Fatalf("remark inbound tag was not cascaded: %+v", rules)
	}
	if got[generatedRule.ID] != fmt.Sprintf("inbound-%d-vmess", inbound.ID) {
		t.Fatalf("generated inbound tag was not cascaded: %+v", rules)
	}
}

func TestStoreUpdateInboundDoesNotCascadeDuplicateRemarkRules(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	first, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "shared-edge", Protocol: "vless", Port: 443, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create first inbound: %v", err)
	}
	second, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "shared-edge", Protocol: "vmess", Port: 8443, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create second inbound: %v", err)
	}
	remarkRule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "shared-edge",
		OutboundTag: "blocked",
		Domain:      "geosite:netflix",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create remark routing rule: %v", err)
	}
	generatedRule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  fmt.Sprintf("inbound-%d-vless", first.ID),
		OutboundTag: "direct",
		Domain:      "example.com",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create generated routing rule: %v", err)
	}

	if _, err := store.UpdateInbound(context.Background(), first.ID, db.UpdateInboundParams{
		Remark: "renamed-edge", Protocol: "trojan", Port: 9443, Network: "tcp", Security: "tls", Enabled: true,
	}); err != nil {
		t.Fatalf("update inbound: %v", err)
	}
	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	got := map[int64]string{}
	for _, rule := range rules {
		got[rule.ID] = rule.InboundTag
	}
	if got[remarkRule.ID] != "shared-edge" {
		t.Fatalf("duplicate remark rule should not be cascaded: %+v", rules)
	}
	if got[generatedRule.ID] != fmt.Sprintf("inbound-%d-trojan", first.ID) {
		t.Fatalf("generated inbound tag should still be cascaded: %+v", rules)
	}
	if second.ID == first.ID {
		t.Fatal("test setup produced duplicate inbound IDs")
	}
}

func TestStoreUpdateInboundRejectsUnknownID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.UpdateInbound(context.Background(), 99999, db.UpdateInboundParams{Remark: "x", Port: 80})
	if err == nil {
		t.Fatal("expected error for unknown inbound")
	}
}

func TestStoreUpdateInboundRejectsUnsupportedProtocol(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "test", Protocol: "vless", Port: 8443, Network: "tcp",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	for _, protocol := range []string{join("vpn", "gate", "_soft", "ether"), "wireguard"} {
		_, err = store.UpdateInbound(context.Background(), inbound.ID, db.UpdateInboundParams{
			Remark: "test", Protocol: protocol, Port: 8443, Network: "tcp", Enabled: true,
		})
		if err == nil {
			t.Fatalf("expected unsupported protocol error for %q", protocol)
		}
	}
}

func TestStoreUpdateClientUpdatesFields(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "test", Protocol: "trojan", Port: 443, Network: "tcp", Security: "tls",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(context.Background(), db.CreateClientParams{
		InboundID: inbound.ID, Email: "old@test.com",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	updated, err := store.UpdateClient(context.Background(), client.ID, db.UpdateClientParams{
		Email:   "new@test.com",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("update client: %v", err)
	}
	if updated.Email != "new@test.com" || updated.Enabled != false {
		t.Fatalf("unexpected updated client: %+v", updated)
	}
	if updated.ID != client.ID || updated.UUID != client.UUID {
		t.Fatalf("id/uuid changed: old=%+v new=%+v", client, updated)
	}

	updated, err = store.UpdateClient(context.Background(), client.ID, db.UpdateClientParams{
		Email:   "new@test.com",
		UUID:    "22222222-2222-4222-8222-222222222222",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("update client uuid: %v", err)
	}
	if updated.UUID != "22222222-2222-4222-8222-222222222222" || !updated.Enabled {
		t.Fatalf("uuid/enabled not updated: %+v", updated)
	}

	loaded, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(loaded) != 1 || len(loaded[0].Clients) != 1 || loaded[0].Clients[0].Email != "new@test.com" || loaded[0].Clients[0].UUID != "22222222-2222-4222-8222-222222222222" || loaded[0].Clients[0].Enabled != true {
		t.Fatalf("updated client not persisted: %+v", loaded[0].Clients[0])
	}
}

func TestStoreUpdateClientCascadesRoutingRuleClientEmail(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "edge", Protocol: "vless", Port: 443, Network: "tcp", Security: "reality",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(context.Background(), db.CreateClientParams{InboundID: inbound.ID, Email: "old@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	rule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		InboundTag:  "edge",
		ClientID:    client.ID,
		OutboundTag: "blocked",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create client routing rule: %v", err)
	}

	if _, err := store.UpdateClient(context.Background(), client.ID, db.UpdateClientParams{
		Email: "new@example.com", UUID: client.UUID, Enabled: true,
	}); err != nil {
		t.Fatalf("update client: %v", err)
	}
	rules, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID || rules[0].ClientEmail != "new@example.com" {
		t.Fatalf("client email was not cascaded to routing rule: %+v", rules)
	}
}

func TestStoreUpdateClientRejectsUnknownID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.UpdateClient(context.Background(), 99999, db.UpdateClientParams{Email: "x"})
	if err == nil {
		t.Fatal("expected error for unknown client")
	}
}

func TestStoreCreateInboundWithTransportFields(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Create inbound with WS + Reality + SS fields
	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark:             "transport-test",
		Protocol:           "vless",
		Port:               20000,
		Network:            "ws",
		Security:           "tls",
		WsPath:             "/migate",
		WsHost:             "test.example.com",
		GrpcServiceName:    "migate",
		RealityDest:        "www.google.com:443",
		RealityServerNames: "www.google.com",
		RealityShortID:     "6ba85179e30d4fc2",
		SSMethod:           "2022-blake3-aes-256-gcm",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	// Verify returned fields
	tests := []struct {
		name string
		got  string
	}{
		{"ws_path", inbound.WsPath},
		{"ws_host", inbound.WsHost},
		{"grpc_service_name", inbound.GrpcServiceName},
		{"reality_dest", inbound.RealityDest},
		{"reality_server_names", inbound.RealityServerNames},
		{"reality_short_id", inbound.RealityShortID},
		{"ss_method", inbound.SSMethod},
	}
	for _, tc := range tests {
		if tc.got == "" {
			t.Errorf("expected non-empty %s", tc.name)
		}
	}

	if inbound.WsPath != "/migate" {
		t.Fatalf("ws_path: got %q, want /migate", inbound.WsPath)
	}
	if inbound.WsHost != "test.example.com" {
		t.Fatalf("ws_host: got %q, want test.example.com", inbound.WsHost)
	}

	// Verify via list
	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	var found bool
	for _, ib := range inbounds {
		if ib.ID == inbound.ID {
			found = true
			if ib.WsPath != "/migate" {
				t.Fatalf("list ws_path: got %q, want /migate", ib.WsPath)
			}
			if ib.RealityDest != "www.google.com:443" {
				t.Fatalf("list reality_dest: got %q, want www.google.com:443", ib.RealityDest)
			}
			if ib.SSMethod != "2022-blake3-aes-256-gcm" {
				t.Fatalf("list ss_method: got %q, want 2022-blake3-aes-256-gcm", ib.SSMethod)
			}
			break
		}
	}
	if !found {
		t.Fatal("inbound not found in list")
	}

	// Test UpdateInbound preserves transport fields
	updated, err := store.UpdateInbound(context.Background(), inbound.ID, db.UpdateInboundParams{
		Remark:             "transport-updated",
		Protocol:           "vmess",
		Port:               20000,
		Network:            "ws",
		Security:           "tls",
		Enabled:            true,
		WsPath:             "/updated-path",
		WsHost:             "updated.example.com",
		GrpcServiceName:    "updated-grpc",
		RealityDest:        "updated.com:443",
		RealityServerNames: "updated.com",
		RealityShortID:     "deadbeef",
		SSMethod:           "2022-blake3-aes-128-gcm",
	})
	if err != nil {
		t.Fatalf("update inbound: %v", err)
	}
	if updated.WsPath != "/updated-path" {
		t.Fatalf("update ws_path: got %q, want /updated-path", updated.WsPath)
	}
	if updated.RealityDest != "updated.com:443" {
		t.Fatalf("update reality_dest: got %q, want updated.com:443", updated.RealityDest)
	}
	if updated.Remark != "transport-updated" {
		t.Fatalf("update remark: got %q, want transport-updated", updated.Remark)
	}
}

func TestStoreCreateInboundWithTLSFields(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Create inbound with TLS fields
	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark:      "tls-test",
		Protocol:    "vless",
		Port:        30010,
		Network:     "tcp",
		Security:    "tls",
		TLSCertFile: "/etc/letsencrypt/live/example.com/fullchain.pem",
		TLSKeyFile:  "/etc/letsencrypt/live/example.com/privkey.pem",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	if inbound.TLSCertFile != "/etc/letsencrypt/live/example.com/fullchain.pem" {
		t.Fatalf("tls_cert_file: got %q, want /etc/letsencrypt/live/example.com/fullchain.pem", inbound.TLSCertFile)
	}
	if inbound.TLSKeyFile != "/etc/letsencrypt/live/example.com/privkey.pem" {
		t.Fatalf("tls_key_file: got %q, want /etc/letsencrypt/live/example.com/privkey.pem", inbound.TLSKeyFile)
	}

	// Verify via list
	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	var found bool
	for _, ib := range inbounds {
		if ib.ID == inbound.ID {
			found = true
			if ib.TLSCertFile != "/etc/letsencrypt/live/example.com/fullchain.pem" {
				t.Fatalf("list tls_cert_file: got %q", ib.TLSCertFile)
			}
			if ib.TLSKeyFile != "/etc/letsencrypt/live/example.com/privkey.pem" {
				t.Fatalf("list tls_key_file: got %q", ib.TLSKeyFile)
			}
			break
		}
	}
	if !found {
		t.Fatal("inbound not found in list")
	}

	// Update and verify TLS fields preserved
	updated, err := store.UpdateInbound(context.Background(), inbound.ID, db.UpdateInboundParams{
		Remark:      "tls-updated",
		Protocol:    "vless",
		Port:        30011,
		Network:     "tcp",
		Security:    "tls",
		Enabled:     true,
		TLSCertFile: "/new/path/cert.pem",
		TLSKeyFile:  "/new/path/key.pem",
	})
	if err != nil {
		t.Fatalf("update inbound: %v", err)
	}
	if updated.TLSCertFile != "/new/path/cert.pem" {
		t.Fatalf("update tls_cert_file: got %q", updated.TLSCertFile)
	}
	if updated.TLSKeyFile != "/new/path/key.pem" {
		t.Fatalf("update tls_key_file: got %q", updated.TLSKeyFile)
	}
}

func TestStoreCreateInboundWithXHTTPFields(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark:             "xhttp-test",
		Protocol:           "vless",
		Port:               30040,
		Network:            "xhttp",
		Security:           "reality",
		RealityDest:        "www.cloudflare.com:443",
		RealityServerNames: "www.cloudflare.com",
		XHTTPPath:          "/migate-xhttp",
		XHTTPMode:          "stream-one",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if inbound.XHTTPPath != "/migate-xhttp" {
		t.Fatalf("xhttp_path: got %q, want /migate-xhttp", inbound.XHTTPPath)
	}
	if inbound.XHTTPMode != "stream-one" {
		t.Fatalf("xhttp_mode: got %q, want stream-one", inbound.XHTTPMode)
	}

	loaded, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(loaded) != 1 || loaded[0].XHTTPPath != "/migate-xhttp" || loaded[0].XHTTPMode != "stream-one" {
		t.Fatalf("xhttp fields not persisted via list: %+v", loaded)
	}

	updated, err := store.UpdateInbound(context.Background(), inbound.ID, db.UpdateInboundParams{
		Remark:             "xhttp-updated",
		Protocol:           "vless",
		Port:               30041,
		Network:            "xhttp",
		Security:           "reality",
		Enabled:            true,
		RealityDest:        "www.microsoft.com:443",
		RealityServerNames: "www.microsoft.com",
		XHTTPPath:          "/updated-xhttp",
		XHTTPMode:          "packet-up",
	})
	if err != nil {
		t.Fatalf("update inbound: %v", err)
	}
	if updated.XHTTPPath != "/updated-xhttp" || updated.XHTTPMode != "packet-up" {
		t.Fatalf("xhttp fields not updated: %+v", updated)
	}
}

func TestStoreCreateInboundWithInitialClient(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Create inbound with an initial client in one call
	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark:   "init-client-test",
		Protocol: "vless",
		Port:     8443,
		Network:  "tcp",
		Security: "none",
		InitialClient: &db.CreateClientParams{
			Email:        "init@test.com",
			UUID:         "11111111-2222-4333-8444-555555555555",
			TrafficLimit: 100_000_000_000,
		},
	})
	if err != nil {
		t.Fatalf("create inbound with initial client: %v", err)
	}
	if inbound.ID == 0 {
		t.Fatalf("expected non-zero inbound ID")
	}
	if len(inbound.Clients) != 1 {
		t.Fatalf("expected 1 client attached to inbound, got %d: %+v", len(inbound.Clients), inbound.Clients)
	}
	if inbound.Clients[0].Email != "init@test.com" {
		t.Fatalf("unexpected client email: %s", inbound.Clients[0].Email)
	}
	if inbound.Clients[0].UUID != "11111111-2222-4333-8444-555555555555" {
		t.Fatalf("expected custom initial client uuid to be preserved, got %s", inbound.Clients[0].UUID)
	}
	if inbound.Clients[0].TrafficLimit != 100_000_000_000 {
		t.Fatalf("unexpected traffic limit: %d", inbound.Clients[0].TrafficLimit)
	}

	// Verify via ListInbounds
	inbounds, err := store.ListInbounds(context.Background())
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(inbounds) != 1 || len(inbounds[0].Clients) != 1 {
		t.Fatalf("expected 1 inbound with 1 client, got %+v", inbounds)
	}
}

func TestStoreCreateInboundWithoutInitialClient(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Creating inbound without initial client should work as before
	inbound, err := store.CreateInbound(context.Background(), db.CreateInboundParams{
		Remark: "no-init-client", Protocol: "vless", Port: 9443, Network: "tcp", Security: "none",
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if inbound.ID == 0 {
		t.Fatalf("expected non-zero inbound ID")
	}
	if len(inbound.Clients) != 0 {
		t.Fatalf("expected 0 clients, got %d", len(inbound.Clients))
	}
}

func TestStoreRejectsRemovedLegacyOutbound(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.CreateOutbound(context.Background(), db.CreateOutboundParams{
		Tag:      "removed-vpn-outbound",
		Remark:   "removed VPN feature",
		Protocol: join("vpn", "gate", "_soft", "ether"),
		Address:  "10.77.1.2",
		Port:     21080,
	})
	if err == nil {
		t.Fatal("expected removed outbound protocol to be rejected after removal")
	}
}

func TestStoreRejectsRemovedLegacyPoolVirtualOutbound(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{OutboundTag: join("vpn", "gate", "-pool"), Domain: "geosite:google", Enabled: true})
	if err == nil {
		t.Fatal("expected removed virtual outbound to be rejected")
	}
}

func TestStoreCreateRoutingRuleRejectsNonexistentOutboundTag(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Default outbounds are "direct" and "blocked" — "nonexistent" should be rejected
	_, err = store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		OutboundTag: "nonexistent",
		Domain:      "example.com",
		Enabled:     true,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent outbound_tag, got nil")
	}
}

func TestStoreUpdateRoutingRuleRejectsNonexistentOutboundTag(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	rule, err := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		OutboundTag: "blocked",
		Domain:      "geosite:malware",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	_, err = store.UpdateRoutingRule(context.Background(), rule.ID, db.UpdateRoutingRuleParams{
		OutboundTag: "nonexistent",
		Domain:      "geosite:netflix",
		Enabled:     false,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent outbound_tag on update, got nil")
	}
}

func TestStoreReorderRoutingRulesUpdatesSortOrder(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	r1, _ := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		OutboundTag: "blocked", Domain: "geosite:malware", Enabled: true,
	})
	r2, _ := store.CreateRoutingRule(context.Background(), db.CreateRoutingRuleParams{
		OutboundTag: "blocked", Domain: "geosite:netflix", Enabled: true,
	})

	err = store.ReorderRoutingRules(context.Background(), []int64{r2.ID, r1.ID})
	if err != nil {
		t.Fatalf("reorder routing rules: %v", err)
	}

	list, err := store.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("list after reorder: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 routing rules, got %d", len(list))
	}
	if list[0].ID != r2.ID || list[1].ID != r1.ID {
		t.Fatalf("expected rules in order [%d, %d], got [%d, %d]", r2.ID, r1.ID, list[0].ID, list[1].ID)
	}
}

func TestStoreBlacklistAddAndCheck(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	expires := time.Now().Add(24 * time.Hour)

	// Add as active session (revoked=false)
	if err := store.AddToBlacklist(ctx, hash, expires, false); err != nil {
		t.Fatalf("AddToBlacklist: %v", err)
	}

	// Should not be blacklisted
	revoked, err := store.IsBlacklisted(ctx, hash)
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if revoked {
		t.Fatal("expected session to NOT be blacklisted yet")
	}

	// Revoke it
	if err := store.AddToBlacklist(ctx, hash, expires, true); err != nil {
		t.Fatalf("AddToBlacklist (revoke): %v", err)
	}

	// Should now be blacklisted
	revoked, err = store.IsBlacklisted(ctx, hash)
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if !revoked {
		t.Fatal("expected session to be blacklisted after revoke")
	}
}

func TestStoreBlacklistExpiredEntry(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	hash := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	// Add with already-expired timestamp
	expires := time.Now().Add(-1 * time.Hour)
	if err := store.AddToBlacklist(ctx, hash, expires, true); err != nil {
		t.Fatalf("AddToBlacklist: %v", err)
	}

	// Should be auto-cleaned and not blacklisted
	revoked, err := store.IsBlacklisted(ctx, hash)
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if revoked {
		t.Fatal("expected expired entry to be auto-cleaned")
	}
}

func TestStoreListActiveSessions(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	expires := time.Now().Add(24 * time.Hour)

	hash1 := "1111111111111111111111111111111111111111111111111111111111111111"
	hash2 := "2222222222222222222222222222222222222222222222222222222222222222"
	hash3 := "3333333333333333333333333333333333333333333333333333333333333333"

	// Add two active sessions
	if err := store.AddToBlacklist(ctx, hash1, expires, false); err != nil {
		t.Fatalf("AddToBlacklist hash1: %v", err)
	}
	if err := store.AddToBlacklist(ctx, hash2, expires, false); err != nil {
		t.Fatalf("AddToBlacklist hash2: %v", err)
	}
	// Add one revoked session
	if err := store.AddToBlacklist(ctx, hash3, expires, true); err != nil {
		t.Fatalf("AddToBlacklist hash3: %v", err)
	}

	sessions, err := store.ListActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 active sessions, got %d", len(sessions))
	}
}

func TestStoreRevokeSessionByID(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	expires := time.Now().Add(24 * time.Hour)

	if err := store.AddToBlacklist(ctx, hash, expires, false); err != nil {
		t.Fatalf("AddToBlacklist: %v", err)
	}

	// List to get the ID
	sessions, err := store.ListActiveSessions(ctx)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("expected 1 active session, got %d: %v", len(sessions), err)
	}

	// Revoke by ID
	if err := store.RevokeSession(ctx, sessions[0].ID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	// Should be blacklisted now
	revoked, err := store.IsBlacklisted(ctx, hash)
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if !revoked {
		t.Fatal("expected session to be revoked")
	}

	// Revoking again should fail (already revoked)
	if err := store.RevokeSession(ctx, sessions[0].ID); err == nil {
		t.Fatal("expected error when revoking already-revoked session")
	}
}

func TestStorePruneActiveSessions(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	expires := time.Now().Add(24 * time.Hour)
	for i := 0; i < 4; i++ {
		hash := fmt.Sprintf("%064d", i)
		if err := store.AddToBlacklist(ctx, hash, expires, false); err != nil {
			t.Fatalf("AddToBlacklist %d: %v", i, err)
		}
	}

	if err := store.PruneActiveSessions(ctx, 2); err != nil {
		t.Fatalf("PruneActiveSessions: %v", err)
	}
	sessions, err := store.ListActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 active sessions, got %d", len(sessions))
	}
	if sessions[0].TokenHash != fmt.Sprintf("%064d", 3) || sessions[1].TokenHash != fmt.Sprintf("%064d", 2) {
		t.Fatalf("expected newest sessions to remain, got %+v", sessions)
	}
}

func TestStoreRevokeOtherSessions(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	expires := time.Now().Add(24 * time.Hour)
	current := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	other := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	revokedHash := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if err := store.AddToBlacklist(ctx, current, expires, false); err != nil {
		t.Fatalf("AddToBlacklist current: %v", err)
	}
	if err := store.AddToBlacklist(ctx, other, expires, false); err != nil {
		t.Fatalf("AddToBlacklist other: %v", err)
	}
	if err := store.AddToBlacklist(ctx, revokedHash, expires, true); err != nil {
		t.Fatalf("AddToBlacklist revoked: %v", err)
	}

	n, err := store.RevokeOtherSessions(ctx, current)
	if err != nil {
		t.Fatalf("RevokeOtherSessions: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 revoked session, got %d", n)
	}
	sessions, err := store.ListActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].TokenHash != current {
		t.Fatalf("expected only current session to remain, got %+v", sessions)
	}
}

func TestStoreUpdateClientTrafficBatch(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "batch", Protocol: "vless", Port: 28080, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	clientA, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "a@example.com"})
	if err != nil {
		t.Fatalf("create client a: %v", err)
	}
	clientB, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "b@example.com"})
	if err != nil {
		t.Fatalf("create client b: %v", err)
	}

	err = store.UpdateClientTrafficBatch(ctx, map[string]db.ClientTrafficUpdate{
		clientA.StatsKey:      {Up: 11, Down: 22},
		clientB.StatsKey:      {Up: 33, Down: 44},
		"missing@example.com": {Up: 55, Down: 66},
	})
	if err != nil {
		t.Fatalf("batch update traffic: %v", err)
	}
	err = store.UpdateClientTrafficBatch(ctx, map[string]db.ClientTrafficUpdate{
		clientA.StatsKey: {Up: 21, Down: 42},
		clientB.StatsKey: {Up: 63, Down: 84},
	})
	if err != nil {
		t.Fatalf("second batch update traffic: %v", err)
	}

	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	traffic := map[string][2]int64{}
	for _, client := range inbounds[0].Clients {
		traffic[client.Email] = [2]int64{client.Up, client.Down}
	}
	if traffic["a@example.com"] != [2]int64{10, 20} || traffic["b@example.com"] != [2]int64{30, 40} {
		t.Fatalf("unexpected traffic after batch update: %+v", traffic)
	}
}

func TestStoreGetSubscriptionByClientUUIDLoadsOnlyMatchedClient(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "sub", Protocol: "vless", Port: 443, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	target, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "target@example.com"})
	if err != nil {
		t.Fatalf("create target client: %v", err)
	}
	if _, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "other@example.com"}); err != nil {
		t.Fatalf("create other client: %v", err)
	}

	loadedInbound, loadedClient, found, err := store.GetSubscriptionByClientUUID(ctx, target.UUID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !found {
		t.Fatal("expected subscription client to be found")
	}
	if loadedInbound.ID != inbound.ID || loadedClient.ID != target.ID || loadedClient.Email != "target@example.com" {
		t.Fatalf("unexpected subscription row: inbound=%+v client=%+v", loadedInbound, loadedClient)
	}
	if len(loadedInbound.Clients) != 1 || loadedInbound.Clients[0].ID != target.ID {
		t.Fatalf("subscription query should attach only the matched client, got %+v", loadedInbound.Clients)
	}

	_, _, found, err = store.GetSubscriptionByClientUUID(ctx, "missing")
	if err != nil {
		t.Fatalf("get missing subscription: %v", err)
	}
	if found {
		t.Fatal("expected missing subscription to return found=false")
	}
}

func TestStoreCreatesAndLooksUpIndependentSubscriptionToken(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "sub-token", Protocol: "vless", Port: 9443, Network: "tcp", Security: "reality"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "token@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if client.SubscriptionToken == "" || client.SubscriptionToken == client.UUID {
		t.Fatalf("expected independent subscription token, got client=%+v", client)
	}

	loadedInbound, loadedClient, found, err := store.GetSubscriptionByToken(ctx, client.SubscriptionToken)
	if err != nil {
		t.Fatalf("lookup by token: %v", err)
	}
	if !found || loadedInbound.ID != inbound.ID || loadedClient.ID != client.ID {
		t.Fatalf("unexpected token lookup result: found=%v inbound=%+v client=%+v", found, loadedInbound, loadedClient)
	}
}

func TestStoreUpdateClientTrafficByStatsKeyDoesNotPolluteDuplicateEmailsAcrossInbounds(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	first, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "first", Protocol: "vless", Port: 28081, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create first inbound: %v", err)
	}
	second, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "second", Protocol: "vless", Port: 28082, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create second inbound: %v", err)
	}
	firstClient, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: first.ID, Email: "shared@example.com"})
	if err != nil {
		t.Fatalf("create first shared client: %v", err)
	}
	secondClient, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: second.ID, Email: "shared@example.com"})
	if err != nil {
		t.Fatalf("create second shared client: %v", err)
	}

	if firstClient.StatsKey == "" || secondClient.StatsKey == "" || firstClient.StatsKey == secondClient.StatsKey {
		t.Fatalf("expected unique stats keys, got first=%q second=%q", firstClient.StatsKey, secondClient.StatsKey)
	}
	if err := store.UpdateClientTraffic(ctx, firstClient.StatsKey, 7, 9); err != nil {
		t.Fatalf("update duplicate email traffic: %v", err)
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	var matched int
	for _, inbound := range inbounds {
		for _, client := range inbound.Clients {
			if client.Email == "shared@example.com" {
				matched++
				if client.ID == firstClient.ID && (client.Up != 0 || client.Down != 0) {
					t.Fatalf("first sample should establish baseline without usage delta: %+v", client)
				}
				if client.ID == secondClient.ID && (client.Up != 0 || client.Down != 0) {
					t.Fatalf("duplicate email client should not be updated by another stats key: %+v", client)
				}
			}
		}
	}
	if matched != 2 {
		t.Fatalf("expected two duplicate-email clients, got %d", matched)
	}
}

func TestTrafficRawIncrementRollbackAndResetBaseline(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "traffic", Protocol: "vless", Port: 28083, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "meter@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if client.StatsKey == "" || client.StatsKey == client.Email {
		t.Fatalf("expected opaque stats key, got %+v", client)
	}
	t0 := time.Unix(100, 0)
	raw := func(up, down int64) []db.TrafficRawStat {
		return []db.TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: up, RawDown: down, Status: "ok"}}
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(100, 200), t0); err != nil {
		t.Fatalf("baseline sample: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(160, 260), t0.Add(10*time.Second)); err != nil {
		t.Fatalf("increment sample: %v", err)
	}
	states, err := store.ListTrafficStates(ctx)
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	state := findTrafficState(states, "xray", "client", client.StatsKey)
	if state == nil || state.TotalUp != 60 || state.TotalDown != 60 || state.RateUp != 6 || state.RateDown != 6 {
		t.Fatalf("unexpected increment state: %+v", state)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(10, 20), t0.Add(20*time.Second)); err != nil {
		t.Fatalf("rollback sample: %v", err)
	}
	states, _ = store.ListTrafficStates(ctx)
	state = findTrafficState(states, "xray", "client", client.StatsKey)
	if state == nil || state.TotalUp != 60 || state.TotalDown != 60 {
		t.Fatalf("raw rollback should not reduce totals: %+v", state)
	}
	if _, err := store.ResetClientTrafficBaseline(ctx, client.ID, raw(10, 20)); err != nil {
		t.Fatalf("reset baseline: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(10, 20), t0.Add(30*time.Second)); err != nil {
		t.Fatalf("same raw after reset: %v", err)
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	got := inbounds[0].Clients[0]
	if got.Up != 0 || got.Down != 0 {
		t.Fatalf("reset should not rebound on same raw baseline: %+v", got)
	}
}

func TestSingboxResetBaselineDoesNotReboundOldTraffic(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "hy2", Protocol: "hysteria2", Port: 28088, Network: "udp", Security: "tls"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "hy2@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	t0 := time.Unix(1000, 0)
	raw := func(up, down int64) []db.TrafficRawStat {
		return []db.TrafficRawStat{{Engine: "singbox", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: up, RawDown: down, Status: "ok"}}
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(1000, 2000), t0); err != nil {
		t.Fatalf("baseline sample: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(1500, 2600), t0.Add(10*time.Second)); err != nil {
		t.Fatalf("increment sample: %v", err)
	}
	if _, err := store.ResetClientTrafficBaseline(ctx, client.ID, raw(1500, 2600)); err != nil {
		t.Fatalf("reset baseline: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(1700, 2900), t0.Add(20*time.Second)); err != nil {
		t.Fatalf("post-reset sample: %v", err)
	}
	usage, found, err := store.GetClientTrafficUsageForClient(ctx, client.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if !found || usage.Engine != "singbox" || usage.TotalUp != 200 || usage.TotalDown != 300 {
		t.Fatalf("expected only post-reset singbox delta, got found=%v usage=%+v", found, usage)
	}
}

func TestTrafficScopeStatusDoesNotPolluteRawBaseline(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "hy2-status", Protocol: "hysteria2", Port: 28095, Network: "udp", Security: "tls"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "hy2-status@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	inboundKey := "hy2-inbound-28095"
	raw := func(up, down int64) []db.TrafficRawStat {
		return []db.TrafficRawStat{
			{Engine: "singbox", ScopeType: "inbound", ScopeKey: inboundKey, RawUp: up, RawDown: down, Status: "ok"},
			{Engine: "singbox", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: up, RawDown: down, Status: "ok"},
		}
	}
	t0 := time.Unix(1000, 0)
	if err := store.ApplyTrafficRawStats(ctx, raw(1000, 2000), t0); err != nil {
		t.Fatalf("baseline sample: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(1100, 2300), t0.Add(10*time.Second)); err != nil {
		t.Fatalf("increment sample: %v", err)
	}
	samples, err := store.ListTrafficSamples(ctx, "client", time.Unix(0, 0), 100)
	if err != nil {
		t.Fatalf("list samples before marker: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected two client samples before marker, got %+v", samples)
	}
	markerAt := t0.Add(20 * time.Second)
	if err := store.MarkTrafficScopeStatus(ctx, []db.TrafficStatusMarker{
		{Engine: "singbox", ScopeType: "inbound", ScopeKey: inboundKey, Status: "unsupported", Message: "stats unsupported"},
		{Engine: "singbox", ScopeType: "client", ScopeKey: client.StatsKey, Status: "unsupported", Message: "stats unsupported"},
	}, markerAt); err != nil {
		t.Fatalf("mark scope status: %v", err)
	}
	states, err := store.ListTrafficStates(ctx)
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	for _, scope := range []struct {
		scopeType string
		scopeKey  string
	}{
		{scopeType: "inbound", scopeKey: inboundKey},
		{scopeType: "client", scopeKey: client.StatsKey},
	} {
		state := findTrafficState(states, "singbox", scope.scopeType, scope.scopeKey)
		if state == nil {
			t.Fatalf("missing traffic state for %s/%s", scope.scopeType, scope.scopeKey)
		}
		if state.TotalUp != 100 || state.TotalDown != 300 || state.LastRawUp != 1100 || state.LastRawDown != 2300 {
			t.Fatalf("status marker polluted totals/raw for %s/%s: %+v", scope.scopeType, scope.scopeKey, state)
		}
		if state.Status != "unsupported" || state.Message != "stats unsupported" || state.LastSeenAt != markerAt.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("status marker did not update status fields for %s/%s: %+v", scope.scopeType, scope.scopeKey, state)
		}
		if state.RateUp != 0 || state.RateDown != 0 {
			t.Fatalf("status marker should clear rates for %s/%s: %+v", scope.scopeType, scope.scopeKey, state)
		}
	}
	samples, err = store.ListTrafficSamples(ctx, "client", time.Unix(0, 0), 100)
	if err != nil {
		t.Fatalf("list samples after marker: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("status marker must not write traffic sample, got %+v", samples)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(1200, 2600), t0.Add(30*time.Second)); err != nil {
		t.Fatalf("recovered sample: %v", err)
	}
	states, err = store.ListTrafficStates(ctx)
	if err != nil {
		t.Fatalf("list states after recovery: %v", err)
	}
	state := findTrafficState(states, "singbox", "client", client.StatsKey)
	if state == nil || state.TotalUp != 200 || state.TotalDown != 600 || state.LastRawUp != 1200 || state.LastRawDown != 2600 {
		t.Fatalf("recovered raw should add only incremental delta, got %+v", state)
	}
	samples, err = store.ListTrafficSamples(ctx, "client", time.Unix(0, 0), 100)
	if err != nil {
		t.Fatalf("list samples after recovery: %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("expected one recovered client sample after marker, got %+v", samples)
	}
}

func TestResetWithoutRawBaselineClearsExistingEngineAndUsesNextRawAsBaseline(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "hy2", Protocol: "hysteria2", Port: 28089, Network: "udp", Security: "tls"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "hy2-wait@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	raw := func(up, down int64) []db.TrafficRawStat {
		return []db.TrafficRawStat{{Engine: "singbox", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: up, RawDown: down, Status: "ok"}}
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(500, 700), time.Unix(100, 0)); err != nil {
		t.Fatalf("baseline sample: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(900, 1200), time.Unix(110, 0)); err != nil {
		t.Fatalf("increment sample: %v", err)
	}
	if _, err := store.ResetClientTrafficBaseline(ctx, client.ID, nil); err != nil {
		t.Fatalf("reset without baseline: %v", err)
	}
	states, err := store.ListTrafficStates(ctx)
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	state := findTrafficState(states, "singbox", "client", client.StatsKey)
	if state == nil || state.TotalUp != 0 || state.TotalDown != 0 || state.Status != "unavailable" {
		t.Fatalf("expected cleared unavailable singbox state, got %+v", state)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(950, 1300), time.Unix(120, 0)); err != nil {
		t.Fatalf("first raw after missing baseline reset: %v", err)
	}
	usage, found, err := store.GetClientTrafficUsageForClient(ctx, client.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if !found || usage.TotalUp != 0 || usage.TotalDown != 0 || usage.Status != "ok" {
		t.Fatalf("first raw after missing baseline should not rebound totals, got found=%v usage=%+v", found, usage)
	}
	if err := store.ApplyTrafficRawStats(ctx, raw(1000, 1400), time.Unix(130, 0)); err != nil {
		t.Fatalf("second raw after reset: %v", err)
	}
	usage, found, err = store.GetClientTrafficUsageForClient(ctx, client.ID)
	if err != nil {
		t.Fatalf("usage after second raw: %v", err)
	}
	if !found || usage.TotalUp != 50 || usage.TotalDown != 100 {
		t.Fatalf("expected only post-baseline delta, got found=%v usage=%+v", found, usage)
	}
}

func TestFirstTrafficSamplePreservesLegacyClientTotals(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "legacy", Protocol: "vless", Port: 28084, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "legacy@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := store.UpdateClientTraffic(ctx, client.StatsKey, 100, 100); err != nil {
		t.Fatalf("baseline key sample: %v", err)
	}
	if err := store.UpdateClientTraffic(ctx, client.StatsKey, 150, 170); err != nil {
		t.Fatalf("increment key sample: %v", err)
	}
	loaded, err := store.ListInbounds(ctx)
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if loaded[0].Clients[0].Up != 50 || loaded[0].Clients[0].Down != 70 {
		t.Fatalf("expected prepared legacy totals, got %+v", loaded[0].Clients[0])
	}
	legacyClient := loaded[0].Clients[0]
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{{Engine: "singbox", ScopeType: "client", ScopeKey: legacyClient.StatsKey, RawUp: 5, RawDown: 6, Status: "ok"}}, time.Unix(200, 0)); err != nil {
		t.Fatalf("first new engine sample: %v", err)
	}
	states, err := store.ListTrafficStates(ctx)
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	state := findTrafficState(states, "singbox", "client", legacyClient.StatsKey)
	if state == nil || state.TotalUp != 50 || state.TotalDown != 70 {
		t.Fatalf("first new state should preserve legacy totals: %+v", state)
	}
}

func TestLegacyEmailTrafficFallbackOnlyForUniqueEmail(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	firstInbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "first", Protocol: "vless", Port: 28085, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create first inbound: %v", err)
	}
	secondInbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "second", Protocol: "vless", Port: 28086, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create second inbound: %v", err)
	}
	unique, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: firstInbound.ID, Email: "unique@example.com"})
	if err != nil {
		t.Fatalf("create unique client: %v", err)
	}
	if _, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: firstInbound.ID, Email: "shared@example.com"}); err != nil {
		t.Fatalf("create first shared client: %v", err)
	}
	if _, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: secondInbound.ID, Email: "shared@example.com"}); err != nil {
		t.Fatalf("create second shared client: %v", err)
	}
	if err := store.UpdateClientTraffic(ctx, "unique@example.com", 10, 20); err != nil {
		t.Fatalf("legacy unique baseline: %v", err)
	}
	if err := store.UpdateClientTraffic(ctx, "unique@example.com", 15, 30); err != nil {
		t.Fatalf("legacy unique increment: %v", err)
	}
	if err := store.UpdateClientTraffic(ctx, "shared@example.com", 100, 200); err != nil {
		t.Fatalf("legacy duplicate should be ignored without error: %v", err)
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	for _, inbound := range inbounds {
		for _, client := range inbound.Clients {
			if client.ID == unique.ID && (client.Up != 5 || client.Down != 10) {
				t.Fatalf("unique legacy email should map to stats_key: %+v", client)
			}
			if client.Email == "shared@example.com" && (client.Up != 0 || client.Down != 0) {
				t.Fatalf("duplicate legacy email should not be polluted: %+v", client)
			}
		}
	}
}

func TestClientTrafficWritebackUsesCurrentEngineOnly(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "engine", Protocol: "vless", Port: 28087, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "engine@example.com"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	xrayRaw := func(up, down int64) []db.TrafficRawStat {
		return []db.TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: up, RawDown: down, Status: "ok"}}
	}
	singboxRaw := func(up, down int64) []db.TrafficRawStat {
		return []db.TrafficRawStat{{Engine: "singbox", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: up, RawDown: down, Status: "ok"}}
	}
	if err := store.ApplyTrafficRawStats(ctx, xrayRaw(100, 100), time.Unix(100, 0)); err != nil {
		t.Fatalf("xray baseline: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, xrayRaw(150, 160), time.Unix(110, 0)); err != nil {
		t.Fatalf("xray increment: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, singboxRaw(1, 2), time.Unix(120, 0)); err != nil {
		t.Fatalf("singbox baseline: %v", err)
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	got := inbounds[0].Clients[0]
	if got.Up != 50 || got.Down != 60 {
		t.Fatalf("singbox baseline must not overwrite xray client totals, got %+v", got)
	}
}

func TestGetClientTrafficUsageSelectsExpectedEngineWithoutDoubleCounting(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	xrayInbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "xray", Protocol: "vless", Port: 28090, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create xray inbound: %v", err)
	}
	singboxInbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "hy2", Protocol: "hysteria2", Port: 28091, Network: "udp", Security: "tls"})
	if err != nil {
		t.Fatalf("create singbox inbound: %v", err)
	}
	xrayClient, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: xrayInbound.ID, Email: "xray-usage"})
	if err != nil {
		t.Fatalf("create xray client: %v", err)
	}
	singboxClient, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: singboxInbound.ID, Email: "singbox-usage"})
	if err != nil {
		t.Fatalf("create singbox client: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{
		{Engine: "xray", ScopeType: "client", ScopeKey: xrayClient.StatsKey, RawUp: 10, RawDown: 10, Status: "ok"},
		{Engine: "singbox", ScopeType: "client", ScopeKey: xrayClient.StatsKey, RawUp: 100, RawDown: 100, Status: "ok"},
		{Engine: "xray", ScopeType: "client", ScopeKey: singboxClient.StatsKey, RawUp: 20, RawDown: 20, Status: "ok"},
		{Engine: "singbox", ScopeType: "client", ScopeKey: singboxClient.StatsKey, RawUp: 200, RawDown: 200, Status: "ok"},
	}, time.Unix(100, 0)); err != nil {
		t.Fatalf("baseline samples: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{
		{Engine: "xray", ScopeType: "client", ScopeKey: xrayClient.StatsKey, RawUp: 15, RawDown: 16, Status: "ok"},
		{Engine: "singbox", ScopeType: "client", ScopeKey: xrayClient.StatsKey, RawUp: 140, RawDown: 150, Status: "ok"},
		{Engine: "xray", ScopeType: "client", ScopeKey: singboxClient.StatsKey, RawUp: 25, RawDown: 26, Status: "ok"},
		{Engine: "singbox", ScopeType: "client", ScopeKey: singboxClient.StatsKey, RawUp: 230, RawDown: 240, Status: "ok"},
	}, time.Unix(110, 0)); err != nil {
		t.Fatalf("increment samples: %v", err)
	}
	xrayUsage, found, err := store.GetClientTrafficUsageForClient(ctx, xrayClient.ID)
	if err != nil {
		t.Fatalf("xray usage: %v", err)
	}
	if !found || xrayUsage.Engine != "xray" || xrayUsage.TotalUp != 5 || xrayUsage.TotalDown != 6 {
		t.Fatalf("expected xray usage only, got found=%v usage=%+v", found, xrayUsage)
	}
	singboxUsage, found, err := store.GetClientTrafficUsageForClient(ctx, singboxClient.ID)
	if err != nil {
		t.Fatalf("singbox usage: %v", err)
	}
	if !found || singboxUsage.Engine != "singbox" || singboxUsage.TotalUp != 30 || singboxUsage.TotalDown != 40 {
		t.Fatalf("expected singbox usage only, got found=%v usage=%+v", found, singboxUsage)
	}
}

func TestGetClientTrafficUsageUsesLegacyTotalsWhenNoTrafficState(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "legacy", Protocol: "vless", Port: 28092, Network: "tcp", Security: "none"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "legacy-usage"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 100, RawDown: 100, Status: "ok"}}, time.Unix(100, 0)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 112, RawDown: 134, Status: "ok"}}, time.Unix(110, 0)); err != nil {
		t.Fatalf("increment: %v", err)
	}
	if err := store.DeleteClientTrafficStatesForTest(ctx, client.StatsKey); err != nil {
		t.Fatalf("delete traffic states: %v", err)
	}
	usage, found, err := store.GetClientTrafficUsageForClient(ctx, client.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if !found || usage.Engine != "migate" || usage.Status != "cumulative_only" || usage.TotalUp != 12 || usage.TotalDown != 34 {
		t.Fatalf("expected legacy cumulative usage, got found=%v usage=%+v", found, usage)
	}
}

func TestGetClientTrafficUsageKeepsExpectedUnavailableOverFallbackOK(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "hy2", Protocol: "hysteria2", Port: 28093, Network: "udp", Security: "tls"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "hy2-unavailable"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{
		{Engine: "singbox", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 100, RawDown: 100, Status: "ok"},
		{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 1000, RawDown: 1000, Status: "ok"},
	}, time.Unix(100, 0)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{
		{Engine: "singbox", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 110, RawDown: 120, Status: "ok"},
		{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 1300, RawDown: 1400, Status: "ok"},
	}, time.Unix(110, 0)); err != nil {
		t.Fatalf("increment: %v", err)
	}
	if err := store.MarkTrafficUnavailable(ctx, "singbox", "unavailable", "stats offline", time.Unix(120, 0)); err != nil {
		t.Fatalf("mark unavailable: %v", err)
	}
	usage, found, err := store.GetClientTrafficUsageForClient(ctx, client.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if !found || usage.Engine != "singbox" || usage.Status != "unavailable" || usage.TotalUp != 10 || usage.TotalDown != 20 {
		t.Fatalf("expected unavailable singbox usage, got found=%v usage=%+v", found, usage)
	}
}

func TestGetClientTrafficUsageFallsBackWhenExpectedEngineMissing(t *testing.T) {
	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	inbound, err := store.CreateInbound(ctx, db.CreateInboundParams{Remark: "hy2", Protocol: "hysteria2", Port: 28094, Network: "udp", Security: "tls"})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client, err := store.CreateClient(ctx, db.CreateClientParams{InboundID: inbound.ID, Email: "hy2-fallback"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 100, RawDown: 100, Status: "ok"}}, time.Unix(100, 0)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := store.ApplyTrafficRawStats(ctx, []db.TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: client.StatsKey, RawUp: 130, RawDown: 140, Status: "ok"}}, time.Unix(110, 0)); err != nil {
		t.Fatalf("increment: %v", err)
	}
	usage, found, err := store.GetClientTrafficUsageForClient(ctx, client.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if !found || usage.Engine != "xray" || usage.Status != "ok" || usage.TotalUp != 30 || usage.TotalDown != 40 {
		t.Fatalf("expected xray fallback usage, got found=%v usage=%+v", found, usage)
	}
}

func findTrafficState(states []db.TrafficState, engine, scopeType, scopeKey string) *db.TrafficState {
	for i := range states {
		if states[i].Engine == engine && states[i].ScopeType == scopeType && states[i].ScopeKey == scopeKey {
			return &states[i]
		}
	}
	return nil
}
