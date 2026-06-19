package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	certsvc "github.com/imzyb/MiGate/internal/service/cert"
)

type CertRenewService interface {
	RenewDue(ctx context.Context, days int) (certsvc.RenewResult, error)
}

type CertificateApplyFunc func(context.Context, []db.Certificate) map[string]interface{}

type CertificateRenewScheduler struct {
	service      CertRenewService
	days         int
	interval     time.Duration
	startDelay   time.Duration
	timeout      time.Duration
	applyRenewed CertificateApplyFunc
	ctx          context.Context
	cancel       context.CancelFunc
	stopped      bool
	mu           sync.Mutex
}

type CertificateRenewSchedulerOptions struct {
	Days         int
	Interval     time.Duration
	StartDelay   time.Duration
	Timeout      time.Duration
	ApplyRenewed CertificateApplyFunc
}

func NewCertificateRenewScheduler(service CertRenewService, options CertificateRenewSchedulerOptions) *CertificateRenewScheduler {
	if options.Days <= 0 {
		options.Days = 30
	}
	if options.Interval <= 0 {
		options.Interval = 24 * time.Hour
	}
	if options.Timeout <= 0 {
		options.Timeout = 10 * time.Minute
	}
	if options.StartDelay < 0 {
		options.StartDelay = 0
	}
	return &CertificateRenewScheduler{
		service:      service,
		days:         options.Days,
		interval:     options.Interval,
		startDelay:   options.StartDelay,
		timeout:      options.Timeout,
		applyRenewed: options.ApplyRenewed,
	}
}

func (s *CertificateRenewScheduler) Start() {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	if s.stopped {
		s.cancel()
	}
	ctx := s.ctx
	s.mu.Unlock()

	if s.startDelay > 0 {
		timer := time.NewTimer(s.startDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Println("certificate renew scheduler stopped")
			return
		case <-timer.C:
		}
	}
	s.RunOnce(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("certificate renew scheduler stopped")
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

func (s *CertificateRenewScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *CertificateRenewScheduler) RunOnce(parent context.Context) certsvc.RenewResult {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	if s.service == nil {
		return certsvc.RenewResult{}
	}
	result, err := s.service.RenewDue(ctx, s.days)
	if err != nil {
		log.Printf("certificate renew: check failed: %v", err)
		return result
	}
	if len(result.Renewed) > 0 && s.applyRenewed != nil {
		applyResult := s.applyRenewed(ctx, result.Renewed)
		log.Printf("certificate renew: renewed %d certificate(s), apply result: %+v", len(result.Renewed), applyResult)
		return result
	}
	if len(result.Checked) > 0 || len(result.Failed) > 0 {
		log.Printf("certificate renew: checked=%d renewed=%d failed=%d", len(result.Checked), len(result.Renewed), len(result.Failed))
	}
	return result
}
