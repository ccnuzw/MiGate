package db_test

import (
	"testing"

	"github.com/imzyb/MiGate/internal/db"
)

func TestInboundCapabilitiesCoverSupportedProtocols(t *testing.T) {
	capabilities := db.InboundCapabilities()
	if len(capabilities) != 9 {
		t.Fatalf("expected 9 inbound capabilities, got %+v", capabilities)
	}
	seen := map[string]db.InboundCapability{}
	for _, capability := range capabilities {
		seen[capability.Protocol] = capability
		if capability.Core == "" || capability.DefaultNetwork == "" || capability.DefaultSecurity == "" || capability.CredentialType == "" || capability.Subscription == "" {
			t.Fatalf("capability missing required fields: %+v", capability)
		}
	}
	for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks", "socks", "http", "hysteria2", "tuic", "shadowtls"} {
		if !db.SupportedInboundProtocol(protocol) {
			t.Fatalf("protocol %s should be supported", protocol)
		}
		if _, ok := seen[protocol]; !ok {
			t.Fatalf("protocol %s missing from capabilities", protocol)
		}
	}
	if seen["hysteria2"].Core != db.CoreSingbox || seen["tuic"].Core != db.CoreSingbox || seen["shadowtls"].Core != db.CoreSingbox {
		t.Fatalf("sing-box protocol ownership mismatch: %+v", seen)
	}
	if seen["socks"].Subscription != db.SubscriptionNone || seen["http"].Subscription != db.SubscriptionNone {
		t.Fatalf("socks/http should not expose share links: %+v %+v", seen["socks"], seen["http"])
	}
}

func TestInboundShareLinkCapabilityMatchesExpectedProtocols(t *testing.T) {
	supported := map[string]bool{
		"vless":       true,
		"vmess":       true,
		"trojan":      true,
		"shadowsocks": true,
		"hysteria2":   true,
		"tuic":        true,
	}
	for _, capability := range db.InboundCapabilities() {
		want := supported[capability.Protocol]
		if capability.ShareLink != want {
			t.Fatalf("protocol %s share_link = %v, want %v", capability.Protocol, capability.ShareLink, want)
		}
		if db.InboundSupportsShareLink(capability.Protocol) != capability.ShareLink {
			t.Fatalf("protocol %s share_link helper drifted from capability", capability.Protocol)
		}
	}
}

func TestValidateInboundCombinationRejectsUnsupportedNetworkAndSecurity(t *testing.T) {
	if err := db.ValidateInboundCombination(db.Inbound{Protocol: "vless", Core: db.CoreXray, Network: "ws", Security: "reality", Port: 443}); err == nil {
		t.Fatal("expected ws+reality to be rejected")
	}
	if err := db.ValidateInboundCombination(db.Inbound{Protocol: "hysteria2", Core: db.CoreSingbox, Network: "tcp", Security: "tls", Port: 443}); err == nil {
		t.Fatal("expected hysteria2 tcp to be rejected")
	}
}
