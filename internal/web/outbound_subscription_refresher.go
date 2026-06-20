package web

import (
	"context"
	"log"
)

type OutboundSubscriptionRefresher struct {
	Store   Store
	Fetcher SubscriptionFetcher
	Options []Option
}

func (r OutboundSubscriptionRefresher) RefreshOutboundSubscription(ctx context.Context, id int64) error {
	result, includeXray, includeSingbox, err := RefreshOutboundSubscription(ctx, r.Store, r.Fetcher, id)
	if err != nil {
		return err
	}
	cfg := routerConfig{
		xrayController: defaultXrayController{},
		singboxRuntime: defaultSingboxRuntime{},
		singboxApplier: tryApplySingboxWithRuntime,
	}
	for _, option := range r.Options {
		option(&cfg)
	}
	cfg.store = r.Store
	payload := map[string]interface{}{}
	attachXrayAndMaybeSingboxResult(ctx, &cfg, r.Store, payload, includeXray, includeSingbox)
	if result != nil {
		log.Printf("outbound subscription refresh: id=%d count=%d skipped=%d", result.SubscriptionID, result.Count, result.SkippedCount)
	}
	return nil
}
