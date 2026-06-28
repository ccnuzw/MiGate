package web

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

type OutboundSubscriptionRefresher struct {
	Store     Store
	Fetcher   SubscriptionFetcher
	Options   []Option
	mu        sync.Mutex
	applyJobs *coreApplyJobManager
}

func (r *OutboundSubscriptionRefresher) RefreshOutboundSubscription(ctx context.Context, id int64) error {
	if r == nil {
		return nil
	}
	cfg := routerConfig{
		xrayController:   defaultXrayController{},
		singboxRuntime:   defaultSingboxRuntime{},
		singboxApplier:   tryApplySingboxWithRuntime,
		autoCoreApply:    true,
		coreApplyTimeout: 2 * time.Minute,
	}
	r.mu.Lock()
	if r.applyJobs == nil {
		r.applyJobs = newCoreApplyJobManager()
	}
	cfg.applyJobs = r.applyJobs
	r.mu.Unlock()
	for _, option := range r.Options {
		option(&cfg)
	}
	cfg.store = r.Store
	includeXray, includeSingbox, err := xrayAndSingboxForAllOutbounds(ctx, r.Store)
	if err != nil {
		return err
	}
	before := captureCoreGeneratedHashes(ctx, &cfg, includeXray, includeSingbox)
	result, includeXray, includeSingbox, err := RefreshOutboundSubscription(ctx, r.Store, r.Fetcher, id)
	if err != nil {
		return err
	}
	after := captureCoreGeneratedHashes(ctx, &cfg, includeXray, includeSingbox)
	hashChange := changedCoresFromGeneratedHashes(before, after)
	for core, hashErr := range hashChange.Errors {
		if hashErr.Error != "" {
			if err := markCorePending(ctx, &cfg, core, hashErr.Error); err != nil {
				log.Printf("outbound subscription refresh: failed to mark %s pending after hash error for id=%d: %v", core, id, err)
			}
			log.Printf("outbound subscription refresh: skip auto apply for %s id=%d: %s %s", core, id, hashErr.Error, hashErr.Detail)
		}
	}
	for _, core := range hashChange.Changed {
		if cfg.autoCoreApply {
			if _, errCode, detail := enqueueCoreAutoApply(context.WithoutCancel(ctx), &cfg, core); errCode != "" {
				if err := markCorePending(ctx, &cfg, core, errCode); err != nil {
					log.Printf("outbound subscription refresh: failed to mark %s pending apply for id=%d: %v", core, id, err)
				}
				log.Printf("outbound subscription refresh: failed to enqueue %s apply for id=%d: %s %s", core, id, errCode, detail)
			}
			continue
		}
		if err := markCorePending(ctx, &cfg, core, "outbound_subscription_refreshed"); err != nil {
			log.Printf("outbound subscription refresh: failed to mark %s pending apply for id=%d: %v", db.NormalizeCore(core), id, err)
		}
	}
	if result != nil {
		log.Printf("outbound subscription refresh: id=%d count=%d skipped=%d", result.SubscriptionID, result.Count, result.SkippedCount)
	}
	return nil
}
