package scheduler

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

type mockVPNGateStore struct {
	mu        sync.Mutex
	outbounds []db.Outbound
	disabled  map[int64]bool
}

func (m *mockVPNGateStore) ListOutbounds(ctx context.Context) ([]db.Outbound, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]db.Outbound, len(m.outbounds))
	copy(out, m.outbounds)
	return out, nil
}

func (m *mockVPNGateStore) SetOutboundEnabled(ctx context.Context, id int64, enabled bool) (db.Outbound, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled == nil {
		m.disabled = make(map[int64]bool)
	}
	m.disabled[id] = enabled
	for i, ob := range m.outbounds {
		if ob.ID == id {
			m.outbounds[i].Enabled = enabled
			return m.outbounds[i], nil
		}
	}
	return db.Outbound{}, nil
}

type mockApplyer struct {
	mu     sync.Mutex
	called int
}

func (m *mockApplyer) Apply(ctx context.Context) error {
	m.mu.Lock()
	m.called++
	m.mu.Unlock()
	return nil
}

func TestVPNGateHealthSchedulerSkipsNonVPNGateOutbounds(t *testing.T) {
	store := &mockVPNGateStore{
		outbounds: []db.Outbound{
			{ID: 1, Tag: "direct", Protocol: "freedom", Enabled: true},
			{ID: 2, Tag: "block", Protocol: "blackhole", Enabled: true},
			{ID: 3, Tag: "vpngate-test", Protocol: "socks", Address: "127.0.0.1", Port: 1080, Enabled: true},
		},
	}
	applyer := &mockApplyer{}
	s := NewVPNGateHealthScheduler(store, applyer, 1*time.Hour, 5)
	s.check()
	res, disabled := s.LastResult()
	if len(res) != 1 {
		t.Fatalf("expected 1 vpngate result, got %d", len(res))
	}
	if disabled != 0 {
		t.Fatalf("expected 0 disabled, got %d", disabled)
	}
	if res[0].Tag != "vpngate-test" {
		t.Fatalf("expected tag vpngate-test, got %s", res[0].Tag)
	}
}

func TestVPNGateHealthSchedulerDisablesAfterThreshold(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	store := &mockVPNGateStore{
		outbounds: []db.Outbound{
			{ID: 42, Tag: "vpngate-ok", Protocol: "socks", Address: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Enabled: true},
			{ID: 99, Tag: "vpngate-bad", Protocol: "socks", Address: "127.0.0.1", Port: 1, Enabled: true},
		},
	}
	applyer := &mockApplyer{}

	// threshold=2, so after 2 failures it should disable
	s := NewVPNGateHealthScheduler(store, applyer, 1*time.Hour, 2)

	// Cycle 1: id=99 should fail once, id=42 should succeed
	s.check()
	if v, ok := store.disabled[99]; ok && !v {
		t.Fatal("should not disable after 1 failure")
	}
	if applyer.called != 0 {
		t.Fatal("should not call apply after 0 disables")
	}

	// Cycle 2: id=99 fails again -> threshold 2 reached -> disabled
	s.check()
	if v, ok := store.disabled[99]; !ok || v != false {
		t.Fatal("expected outbound 99 to be disabled after 2 failures")
	}
	if applyer.called != 1 {
		t.Fatalf("expected apply to be called once after disable, got %d", applyer.called)
	}

	// Verify LastResult reflects the disable
	_, disabled := s.LastResult()
	if disabled != 1 {
		t.Fatalf("expected 1 disabled in last result, got %d", disabled)
	}

	// Check that id=42 (reachable node) wasn't disabled
	if disabled, ok := store.disabled[42]; ok && !disabled {
		t.Fatal("should not disable a reachable node")
	}
}

func TestVPNGateHealthSchedulerResetsFailuresOnSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	store := &mockVPNGateStore{
		outbounds: []db.Outbound{
			{ID: 55, Tag: "vpngate-flakey", Protocol: "socks", Address: "127.0.0.1", Port: 1, Enabled: true},
		},
	}
	applyer := &mockApplyer{}
	s := NewVPNGateHealthScheduler(store, applyer, 1*time.Hour, 3)

	// Two failures (not enough to disable)
	s.check()
	s.check()
	if v, ok := store.disabled[55]; ok && !v {
		t.Fatal("should not disable after 2 failures with threshold=3")
	}

	// Fix the outbound by making it reachable
	store.mu.Lock()
	store.outbounds[0].Port = ln.Addr().(*net.TCPAddr).Port
	store.mu.Unlock()

	// Success resets the counter
	s.check()
	if v, ok := store.disabled[55]; ok && !v {
		t.Fatal("should not disable after success resets counter")
	}

	// Now two more failures should NOT trigger disable (counter reset)
	store.mu.Lock()
	store.outbounds[0].Port = 1
	store.mu.Unlock()
	s.check()
	s.check()
	if v, ok := store.disabled[55]; ok && !v {
		t.Fatal("should not disable after 2 failures following reset with threshold=3")
	}

	// Third failure after reset should finally trigger
	s.check()
	if v, ok := store.disabled[55]; !ok || v != false {
		t.Fatal("expected disable after 3 consecutive failures following reset")
	}
}

func TestVPNGateHealthSchedulerDoesNotCallApplyWhenNoChanges(t *testing.T) {
	store := &mockVPNGateStore{
		outbounds: []db.Outbound{
			{ID: 1, Tag: "vpngate-test", Protocol: "socks", Address: "127.0.0.1", Port: 1, Enabled: true},
		},
	}
	applyer := &mockApplyer{}
	s := NewVPNGateHealthScheduler(store, applyer, 1*time.Hour, 10)

	// Outbound fails but threshold is higher, so no disable, no apply
	for i := 0; i < 3; i++ {
		s.check()
	}

	if applyer.called != 0 {
		t.Fatalf("expected 0 apply calls, got %d", applyer.called)
	}
}