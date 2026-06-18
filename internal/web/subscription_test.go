package web

import (
	"testing"

	"github.com/imzyb/MiGate/internal/db"
)

func TestShareLinkGeneratorsMatchInboundCapabilities(t *testing.T) {
	for _, capability := range db.InboundCapabilities() {
		_, hasGenerator := shareLinkGenerators[capability.Protocol]
		if hasGenerator != capability.ShareLink {
			t.Fatalf("protocol %s share generator = %v, want share_link %v", capability.Protocol, hasGenerator, capability.ShareLink)
		}
	}
}

func TestShareLinkRejectsUnsupportedCapabilityProtocols(t *testing.T) {
	for _, capability := range db.InboundCapabilities() {
		if capability.ShareLink {
			continue
		}
		_, err := shareLink("panel.example.com", db.Inbound{Protocol: capability.Protocol, Port: 1080}, db.Client{})
		if err == nil {
			t.Fatalf("protocol %s should be rejected by subscription share link", capability.Protocol)
		}
	}
}
