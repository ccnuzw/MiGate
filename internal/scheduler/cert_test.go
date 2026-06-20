package scheduler

import (
	"context"
	"testing"

	"github.com/imzyb/MiGate/internal/db"
	certsvc "github.com/imzyb/MiGate/internal/service/cert"
)

type fakeRenewService struct {
	result certsvc.RenewResult
	days   int
	calls  int
}

func (s *fakeRenewService) RenewDue(ctx context.Context, days int) (certsvc.RenewResult, error) {
	s.calls++
	s.days = days
	return s.result, nil
}

func TestCertificateRenewSchedulerRunOnceAppliesRenewedCertificates(t *testing.T) {
	renewed := db.Certificate{ID: 1, Source: db.CertSourceACME}
	service := &fakeRenewService{result: certsvc.RenewResult{
		Checked: []db.Certificate{renewed},
		Renewed: []db.Certificate{renewed},
	}}
	applyCalls := 0
	var applied []db.Certificate
	scheduler := NewCertificateRenewScheduler(service, CertificateRenewSchedulerOptions{
		Days: 30,
		ApplyRenewed: func(ctx context.Context, certs []db.Certificate) map[string]interface{} {
			applyCalls++
			applied = certs
			return map[string]interface{}{"xray": map[string]interface{}{"applied": true}}
		},
	})
	result := scheduler.RunOnce(context.Background())
	if service.calls != 1 || service.days != 30 {
		t.Fatalf("renew service not called correctly: calls=%d days=%d", service.calls, service.days)
	}
	if len(result.Renewed) != 1 || applyCalls != 1 || len(applied) != 1 || applied[0].ID != 1 {
		t.Fatalf("renewed certs not applied: result=%#v applyCalls=%d applied=%#v", result, applyCalls, applied)
	}
}

func TestCertificateRenewSchedulerRunOnceSkipsApplyWhenNothingRenewed(t *testing.T) {
	service := &fakeRenewService{result: certsvc.RenewResult{
		Checked: []db.Certificate{{ID: 1, Source: db.CertSourceACME}},
		Renewed: nil,
		Failed:  []db.Certificate{{ID: 2, Source: db.CertSourceImport}},
	}}
	applyCalls := 0
	scheduler := NewCertificateRenewScheduler(service, CertificateRenewSchedulerOptions{
		ApplyRenewed: func(ctx context.Context, certs []db.Certificate) map[string]interface{} {
			applyCalls++
			return nil
		},
	})
	result := scheduler.RunOnce(context.Background())
	if len(result.Renewed) != 0 || applyCalls != 0 {
		t.Fatalf("unexpected apply for non-renewed certs: result=%#v applyCalls=%d", result, applyCalls)
	}
}
