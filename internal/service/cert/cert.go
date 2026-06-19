package cert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	panelcfg "github.com/imzyb/MiGate/internal/config"
	"github.com/imzyb/MiGate/internal/paths"
	runtimecmd "github.com/imzyb/MiGate/internal/runtime/command"
)

var validDomain = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
var validEmail = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type CertSaver interface {
	SaveCert(domain, email string) error
}

type Service struct {
	ConfigDir  string
	CertDir    string
	Runner     runtimecmd.CommandRunner
	LookPath   func(string) (string, error)
	SaveConfig func(domain, email string) error
}

type IssueRequest struct {
	Domain string
	Email  string
}

type StatusResponse struct {
	Domain   string `json:"domain"`
	Email    string `json:"email"`
	Issued   bool   `json:"issued"`
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}

type IssueResponse struct {
	Status   string `json:"status"`
	Domain   string `json:"domain"`
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}

type Error struct {
	Code   string
	Detail string
}

func (e Error) Error() string {
	if e.Detail != "" {
		return e.Code + ": " + e.Detail
	}
	return e.Code
}

func (e Error) ServiceCode() string {
	return e.Code
}

func (e Error) ServiceDetail() string {
	return e.Detail
}

func ValidateIssueRequest(domain, email string) error {
	if domain == "" || email == "" {
		return Error{Code: "domain_and_email_required"}
	}
	if !validDomain.MatchString(domain) {
		return Error{Code: "invalid_domain"}
	}
	if !validEmail.MatchString(email) {
		return Error{Code: "invalid_email"}
	}
	return nil
}

func (s Service) Status() StatusResponse {
	response := StatusResponse{}
	if s.ConfigDir == "" {
		return response
	}
	configPath := filepath.Join(s.ConfigDir, "panel.json")
	domain, email, err := panelcfg.LoadCertFields(configPath)
	if err == nil {
		response.Domain = domain
		response.Email = email
	}
	if response.Domain == "" {
		return response
	}
	certDir := s.certDir()
	response.CertPath = filepath.Join(certDir, response.Domain+".pem")
	response.KeyPath = filepath.Join(certDir, response.Domain+".key")
	response.Issued = filesExist(response.CertPath, response.KeyPath)
	if response.Issued {
		return response
	}
	fallbackCertDir := filepath.Join(s.ConfigDir, "certs", response.Domain)
	response.CertPath = filepath.Join(fallbackCertDir, "fullchain.pem")
	response.KeyPath = filepath.Join(fallbackCertDir, "privkey.pem")
	response.Issued = filesExist(response.CertPath, response.KeyPath)
	return response
}

func (s Service) Issue(ctx context.Context, req IssueRequest) (IssueResponse, error) {
	if err := ValidateIssueRequest(req.Domain, req.Email); err != nil {
		return IssueResponse{}, err
	}
	if s.ConfigDir == "" {
		return IssueResponse{}, Error{Code: "cert_not_available"}
	}
	runner := s.commandRunner()
	lookPath := s.lookPath()
	certDir := s.certDir()
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return IssueResponse{}, Error{Code: "mkdir_cert_dir_failed", Detail: err.Error()}
	}
	certFile := filepath.Join(certDir, req.Domain+".pem")
	keyFile := filepath.Join(certDir, req.Domain+".key")
	if _, err := lookPath("acme.sh"); err != nil {
		installOut, installErr := installACMESh(req.Email)
		if installErr != nil {
			return IssueResponse{}, Error{Code: "install_acme_failed", Detail: installOut}
		}
	}
	out, err := runner.RunOutput(ctx, "acme.sh",
		"--issue", "--standalone", "-d", req.Domain,
		"--keylength", "ec-256",
		"--fullchain-file", certFile,
		"--key-file", keyFile,
		"--reloadcmd", "systemctl restart "+paths.XrayService+" || true",
	)
	if err != nil {
		return IssueResponse{}, Error{Code: "issue_cert_failed", Detail: string(out)}
	}
	_ = runner.Run(context.Background(), "chown", "root:migate", certFile, keyFile)
	_ = runner.Run(context.Background(), "chmod", "640", certFile)
	_ = runner.Run(context.Background(), "chmod", "600", keyFile)
	if err := s.saveCert(req.Domain, req.Email); err != nil {
		return IssueResponse{}, Error{Code: "write_panel_config_failed", Detail: err.Error()}
	}
	return IssueResponse{Status: "issued", Domain: req.Domain, CertPath: certFile, KeyPath: keyFile}, nil
}

func installACMESh(email string) (string, error) {
	return "acme.sh is not installed. Install acme.sh from a pinned release and verify its checksum or signature before retrying.", fmt.Errorf("refusing to download and execute unverified acme.sh installer for %s", email)
}

func (s Service) commandRunner() runtimecmd.CommandRunner {
	if s.Runner != nil {
		return s.Runner
	}
	return runtimecmd.NewRealCommandRunner(2 * time.Minute)
}

func (s Service) certDir() string {
	if s.CertDir != "" {
		return s.CertDir
	}
	return "/etc/xray/certs"
}

func (s Service) lookPath() func(string) (string, error) {
	if s.LookPath != nil {
		return s.LookPath
	}
	return runtimecmd.LookPath
}

func (s Service) saveCert(domain, email string) error {
	if s.SaveConfig != nil {
		return s.SaveConfig(domain, email)
	}
	return Error{Code: "write_panel_config_failed", Detail: "cert config saver is not configured"}
}

func filesExist(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}
