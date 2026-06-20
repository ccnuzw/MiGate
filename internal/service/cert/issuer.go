package cert

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/runtime/httpchallenge"
	"golang.org/x/crypto/acme"
)

type NativeHTTP01Issuer struct {
	DirectoryURL string
	AccountKey   string
	HTTPAddr     string
}

func (i NativeHTTP01Issuer) Issue(ctx context.Context, req IssueRequest, certPath, keyPath string) (IssueResult, error) {
	domains, err := normalizeDomains(req)
	if err != nil {
		return IssueResult{}, err
	}
	accountKey, err := i.loadOrCreateAccountKey(certPath)
	if err != nil {
		return IssueResult{}, err
	}
	client := &acme.Client{Key: accountKey, DirectoryURL: i.directoryURL(), UserAgent: "MiGate"}
	account := &acme.Account{Contact: []string{"mailto:" + strings.TrimSpace(req.Email)}}
	if strings.TrimSpace(req.Email) == "" {
		account.Contact = nil
	}
	if _, err := client.Register(ctx, account, acme.AcceptTOS); err != nil {
		return IssueResult{}, fmt.Errorf("register ACME account: %w", err)
	}
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return IssueResult{}, err
	}
	challengeServer := httpchallenge.New(i.httpAddr())
	if err := challengeServer.Start(); err != nil {
		return IssueResult{}, fmt.Errorf("%s: %w", CodeHTTP01PortUnavailable, err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = challengeServer.Shutdown(shutdownCtx)
	}()
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
	if err != nil {
		return IssueResult{}, fmt.Errorf("create ACME order: %w", err)
	}
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return IssueResult{}, fmt.Errorf("get authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		challenge := findChallenge(authz.Challenges, "http-01")
		if challenge == nil {
			return IssueResult{}, fmt.Errorf("http-01 challenge unavailable for %s", authz.Identifier.Value)
		}
		response, err := client.HTTP01ChallengeResponse(challenge.Token)
		if err != nil {
			return IssueResult{}, err
		}
		challengeServer.Set(challenge.Token, response)
		if _, err := client.Accept(ctx, challenge); err != nil {
			return IssueResult{}, fmt.Errorf("accept challenge: %w", err)
		}
		if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
			return IssueResult{}, fmt.Errorf("wait authorization: %w", err)
		}
	}
	template := &x509.CertificateRequest{Subject: pkix.Name{CommonName: domains[0]}, DNSNames: domains}
	csr, err := x509.CreateCertificateRequest(rand.Reader, template, certKey)
	if err != nil {
		return IssueResult{}, err
	}
	derChain, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return IssueResult{}, fmt.Errorf("finalize order: %w", err)
	}
	certPEM := []byte{}
	for _, der := range derChain {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyDER, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		return IssueResult{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return IssueResult{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

func (i NativeHTTP01Issuer) directoryURL() string {
	if strings.TrimSpace(i.DirectoryURL) != "" {
		return strings.TrimSpace(i.DirectoryURL)
	}
	return acme.LetsEncryptURL
}

func (i NativeHTTP01Issuer) ACMEDirectory() string {
	return i.directoryURL()
}

func (i NativeHTTP01Issuer) httpAddr() string {
	if strings.TrimSpace(i.HTTPAddr) != "" {
		return strings.TrimSpace(i.HTTPAddr)
	}
	return ":80"
}

func (i NativeHTTP01Issuer) accountKeyPath(certPath string) string {
	if strings.TrimSpace(i.AccountKey) != "" {
		return strings.TrimSpace(i.AccountKey)
	}
	return filepath.Join(filepath.Dir(filepath.Dir(certPath)), "acme-account.key")
}

func (i NativeHTTP01Issuer) loadOrCreateAccountKey(certPath string) (*ecdsa.PrivateKey, error) {
	path := i.accountKeyPath(certPath)
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("invalid ACME account key PEM")
		}
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return key, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func findChallenge(challenges []*acme.Challenge, typ string) *acme.Challenge {
	for _, challenge := range challenges {
		if challenge != nil && challenge.Type == typ {
			return challenge
		}
	}
	return nil
}

func selfSignedPair(domains []string, notAfter time.Time) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: domains[0]},
		DNSNames:     domains,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}
