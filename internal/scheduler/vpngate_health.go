package scheduler

import (
	"context"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

// VPNGateStore is the subset of db.Store needed by the VPN Gate health scheduler.
type VPNGateStore interface {
	ListOutbounds(ctx context.Context) ([]db.Outbound, error)
	SetOutboundEnabled(ctx context.Context, id int64, enabled bool) (db.Outbound, error)
}

// XrayApplyer triggers Xray config reload. Returns nil on success.
type XrayApplyer interface {
	Apply(ctx context.Context) error
}

// VPNGateHealthCheckResult records one check cycle result.
type VPNGateHealthCheckResult struct {
	OutboundID int64  `json:"outbound_id"`
	Tag        string `json:"tag"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
	OK         bool   `json:"ok"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
	Disabled   bool   `json:"disabled"`
}

// VPNGateHealthScheduler periodically checks all vpngate-* SOCKS outbounds
// and auto-disables nodes that exceed the consecutive failure threshold.
type VPNGateHealthScheduler struct {
	store      VPNGateStore
	applyer    XrayApplyer
	interval   time.Duration
	threshold  int // consecutive failures before disabling
	failures   map[int64]int
	lastResult []VPNGateHealthCheckResult
	disabled   int // total disabled in last cycle
	ctx        context.Context
	cancel     context.CancelFunc
	stopped    bool
	mu         sync.Mutex
}

// NewVPNGateHealthScheduler creates a new scheduler.
// interval: how often to check (default 5min), threshold: consecutive failures before disable (default 3).
func NewVPNGateHealthScheduler(store VPNGateStore, applyer XrayApplyer, interval time.Duration, threshold int) *VPNGateHealthScheduler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if threshold <= 0 {
		threshold = 3
	}
	return &VPNGateHealthScheduler{
		store:     store,
		applyer:   applyer,
		interval:  interval,
		threshold: threshold,
		failures:  make(map[int64]int),
	}
}

// Start begins the periodic health check loop. Blocking — run in goroutine.
func (s *VPNGateHealthScheduler) Start() {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	if s.stopped {
		s.cancel()
	}
	s.mu.Unlock()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.check()

	for {
		select {
		case <-s.ctx.Done():
			log.Println("VPN Gate health scheduler stopped")
			return
		case <-ticker.C:
			s.check()
		}
	}
}

// Stop stops the scheduler.
func (s *VPNGateHealthScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.cancel != nil {
		s.cancel()
	}
}

// LastResult returns the result of the last check cycle and count of disabled nodes.
func (s *VPNGateHealthScheduler) LastResult() ([]VPNGateHealthCheckResult, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]VPNGateHealthCheckResult, len(s.lastResult))
	copy(result, s.lastResult)
	return result, s.disabled
}

// check performs one health check cycle.
func (s *VPNGateHealthScheduler) check() {
	outbounds, err := s.store.ListOutbounds(context.Background())
	if err != nil {
		log.Printf("VPN Gate health: list outbounds failed: %v", err)
		return
	}

	var results []VPNGateHealthCheckResult
	needsApply := false
	disabledCount := 0

	for _, ob := range outbounds {
		if !ob.Enabled || ob.Protocol != "socks" || !strings.HasPrefix(ob.Tag, "vpngate-") || ob.Address == "" {
			continue
		}

		res := VPNGateHealthCheckResult{
			OutboundID: ob.ID,
			Tag:        ob.Tag,
			Address:    ob.Address,
			Port:       ob.Port,
		}

		port := ob.Port
		if port == 0 {
			port = 1080
		}
		start := time.Now()
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ob.Address, strconv.Itoa(port)), 1200*time.Millisecond)
		res.LatencyMS = time.Since(start).Milliseconds()

		if err != nil {
			res.Error = err.Error()
			s.mu.Lock()
			s.failures[ob.ID]++
			failCount := s.failures[ob.ID]
			s.mu.Unlock()

			if failCount >= s.threshold {
				_, updateErr := s.store.SetOutboundEnabled(context.Background(), ob.ID, false)
				if updateErr != nil {
					log.Printf("VPN Gate health: failed to disable outbound %s (id=%d): %v", ob.Tag, ob.ID, updateErr)
				} else {
					log.Printf("VPN Gate health: disabled outbound %s (id=%d) after %d consecutive failures", ob.Tag, ob.ID, failCount)
					res.Disabled = true
					disabledCount++
					needsApply = true
					s.mu.Lock()
					delete(s.failures, ob.ID)
					s.mu.Unlock()
				}
			}
		} else {
			_ = conn.Close()
			res.OK = true
			s.mu.Lock()
			delete(s.failures, ob.ID)
			s.mu.Unlock()
		}
		results = append(results, res)
	}

	s.mu.Lock()
	s.lastResult = results
	s.disabled = disabledCount
	s.mu.Unlock()

	if needsApply && s.applyer != nil {
		if err := s.applyer.Apply(context.Background()); err != nil {
			log.Printf("VPN Gate health: Xray Apply failed after disabling nodes: %v", err)
		} else {
			log.Printf("VPN Gate health: Xray config updated after disabling %d nodes", disabledCount)
		}
	}
}
