package cert

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeRunner struct {
	calls [][]string
	out   []byte
	err   error
}

func (r *fakeRunner) Run(ctx context.Context, name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	return nil
}

func (r *fakeRunner) RunOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.out, r.err
}

func TestValidateIssueRequest(t *testing.T) {
	tests := []struct {
		name string
		req  IssueRequest
		want string
	}{
		{name: "missing domain", req: IssueRequest{Email: "admin@example.com"}, want: "domain_and_email_required"},
		{name: "invalid domain", req: IssueRequest{Domain: "-bad.example.com", Email: "admin@example.com"}, want: "invalid_domain"},
		{name: "invalid email", req: IssueRequest{Domain: "example.com", Email: "admin"}, want: "invalid_email"},
		{name: "valid", req: IssueRequest{Domain: "example.com", Email: "admin@example.com"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateIssueRequest(tc.req.Domain, tc.req.Email)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("ValidateIssueRequest returned error: %v", err)
				}
				return
			}
			serviceErr, ok := err.(Error)
			if !ok || serviceErr.Code != tc.want {
				t.Fatalf("error = %#v, want code %s", err, tc.want)
			}
		})
	}
}

func TestStatusReadsConfigAndFallbackCertFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "panel.json"), []byte(`{"cert_domain":"example.com","cert_email":"admin@example.com"}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	certDir := filepath.Join(dir, "certs", "example.com")
	if err := os.MkdirAll(certDir, 0755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("cert"), 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("key"), 0644); err != nil {
		t.Fatalf("write key: %v", err)
	}

	status := Service{ConfigDir: dir}.Status()
	if status.Domain != "example.com" || status.Email != "admin@example.com" || !status.Issued {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.CertPath != filepath.Join(certDir, "fullchain.pem") {
		t.Fatalf("cert path = %q, want fallback path", status.CertPath)
	}
}

func TestIssueRunsACMEAndSavesConfig(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	var savedDomain, savedEmail string
	service := Service{
		ConfigDir: dir,
		CertDir:   filepath.Join(dir, "system-certs"),
		Runner:    runner,
		LookPath: func(name string) (string, error) {
			if name != "acme.sh" {
				t.Fatalf("unexpected lookup %q", name)
			}
			return "/usr/bin/acme.sh", nil
		},
		SaveConfig: func(domain, email string) error {
			savedDomain, savedEmail = domain, email
			return nil
		},
	}

	response, err := service.Issue(context.Background(), IssueRequest{Domain: "example.com", Email: "admin@example.com"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if response.Status != "issued" || response.Domain != "example.com" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if savedDomain != "example.com" || savedEmail != "admin@example.com" {
		t.Fatalf("saved cert = %q/%q", savedDomain, savedEmail)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("runner calls = %#v, want acme + chmod/chown calls", runner.calls)
	}
	if runner.calls[0][0] != "acme.sh" {
		t.Fatalf("first command = %#v", runner.calls[0])
	}
}

func TestIssueReturnsACMEOutputOnFailure(t *testing.T) {
	service := Service{
		ConfigDir: t.TempDir(),
		CertDir:   filepath.Join(t.TempDir(), "system-certs"),
		Runner:    &fakeRunner{out: []byte("issue failed"), err: errors.New("exit 1")},
		LookPath:  func(string) (string, error) { return "/usr/bin/acme.sh", nil },
	}
	_, err := service.Issue(context.Background(), IssueRequest{Domain: "example.com", Email: "admin@example.com"})
	serviceErr, ok := err.(Error)
	if !ok || serviceErr.Code != "issue_cert_failed" || serviceErr.Detail != "issue failed" {
		t.Fatalf("unexpected error: %#v", err)
	}
}
