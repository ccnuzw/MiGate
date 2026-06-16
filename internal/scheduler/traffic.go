package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/singbox"
	"github.com/imzyb/MiGate/internal/xray"
)

// Store is the subset of db.Store methods needed by the scheduler.
type Store interface {
	UpdateClientTraffic(ctx context.Context, email string, uplink, downlink int64) error
	ApplyTrafficRawStats(ctx context.Context, stats []db.TrafficRawStat, observedAt time.Time) error
	MarkTrafficUnavailable(ctx context.Context, engine, status, message string, observedAt time.Time) error
}

type batchTrafficStore interface {
	UpdateClientTrafficBatch(ctx context.Context, stats map[string]db.ClientTrafficUpdate) error
}

// TrafficSyncScheduler periodically syncs traffic statistics from Xray to the database.
type TrafficSyncScheduler struct {
	store              Store
	statsClient        xray.StatsClient
	singboxStatsClient singbox.StatsClient
	interval           time.Duration
	ctx                context.Context
	cancel             context.CancelFunc
	stopped            bool
	mu                 sync.Mutex
}

// NewTrafficSyncScheduler creates a new scheduler.
// interval: how often to sync (e.g., 1 * time.Minute)
func NewTrafficSyncScheduler(store Store, statsClient xray.StatsClient, interval time.Duration) *TrafficSyncScheduler {
	return &TrafficSyncScheduler{
		store:       store,
		statsClient: statsClient,
		interval:    interval,
	}
}

func NewTrafficSyncSchedulerWithSingbox(store Store, statsClient xray.StatsClient, singboxStatsClient singbox.StatsClient, interval time.Duration) *TrafficSyncScheduler {
	scheduler := NewTrafficSyncScheduler(store, statsClient, interval)
	scheduler.singboxStatsClient = singboxStatsClient
	return scheduler
}

// Start begins the periodic sync loop.
// This is a blocking call - run it in a separate goroutine.
func (s *TrafficSyncScheduler) Start() {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	if s.stopped {
		s.cancel()
	}
	s.mu.Unlock()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run once immediately on start
	s.sync()

	for {
		select {
		case <-s.ctx.Done():
			log.Println("traffic sync scheduler stopped")
			return
		case <-ticker.C:
			s.sync()
		}
	}
}

// Stop stops the scheduler.
func (s *TrafficSyncScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.cancel != nil {
		s.cancel()
	}
}

// sync performs a single sync cycle: query Xray stats and update DB.
func (s *TrafficSyncScheduler) sync() {
	ctx, timeout := context.WithTimeout(context.Background(), 10*time.Second)
	defer timeout()
	observedAt := time.Now().UTC()

	rawStats := []db.TrafficRawStat{}
	if s.statsClient != nil {
		stats, err := s.statsClient.QueryTrafficStats(ctx)
		if err != nil {
			log.Printf("traffic sync: failed to query xray stats: %v", err)
			_ = s.store.MarkTrafficUnavailable(ctx, "xray", "unavailable", err.Error(), observedAt)
		} else {
			rawStats = append(rawStats, convertRawStats(stats)...)
		}
	}
	if s.singboxStatsClient != nil {
		stats, err := s.singboxStatsClient.QueryTrafficStats(ctx)
		if err != nil {
			log.Printf("traffic sync: failed to query sing-box stats: %v", err)
			_ = s.store.MarkTrafficUnavailable(ctx, "singbox", "unavailable", err.Error(), observedAt)
		} else {
			rawStats = append(rawStats, convertRawStats(stats)...)
		}
	}
	if len(rawStats) > 0 {
		if err := s.store.ApplyTrafficRawStats(ctx, rawStats, observedAt); err != nil {
			log.Printf("traffic sync: failed to apply traffic states: %v", err)
			return
		}
		log.Printf("traffic sync: applied %d traffic counters", len(rawStats))
		return
	}

	if s.statsClient == nil {
		return
	}
	stats, err := s.statsClient.QueryAllStats(ctx)
	if err != nil {
		log.Printf("traffic sync: failed to query legacy stats: %v", err)
		return
	}
	if batchStore, ok := s.store.(batchTrafficStore); ok {
		batch := make(map[string]db.ClientTrafficUpdate, len(stats))
		for email, clientStats := range stats {
			batch[email] = db.ClientTrafficUpdate{Up: clientStats.Uplink, Down: clientStats.Downlink}
		}
		if err := batchStore.UpdateClientTrafficBatch(ctx, batch); err != nil {
			log.Printf("traffic sync: failed to batch update clients: %v", err)
		}
	}
}

func convertRawStats(stats []xray.TrafficStat) []db.TrafficRawStat {
	raw := make([]db.TrafficRawStat, 0, len(stats))
	for _, stat := range stats {
		raw = append(raw, db.TrafficRawStat{
			Engine: stat.Engine, ScopeType: stat.ScopeType, ScopeKey: stat.ScopeKey,
			RawUp: stat.Uplink, RawDown: stat.Downlink, Status: "ok",
		})
	}
	return raw
}
