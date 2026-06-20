package subscription

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imzyb/MiGate/internal/db"
)

func TestDecodeBodyBase64AndPlain(t *testing.T) {
	plain := "vless://11111111-1111-4111-8111-111111111111@example.com:443#one\ntrojan://pw@example.net:443#two"
	encoded := base64.StdEncoding.EncodeToString([]byte(plain))
	for _, body := range [][]byte{[]byte(plain), []byte(encoded)} {
		links := DecodeBody(body)
		if len(links) != 2 || !strings.HasPrefix(links[0], "vless://") || !strings.HasPrefix(links[1], "trojan://") {
			t.Fatalf("unexpected links: %+v", links)
		}
	}
}

func TestParseVlessRealityTLSWS(t *testing.T) {
	link := "vless://11111111-1111-4111-8111-111111111111:unused@example.com:443?security=reality&type=ws&host=edge.example.com&path=%2Fws&flow=xtls-rprx-vision&sni=www.example.com&fp=chrome&pbk=PUB&sid=ab&alpn=h2,http/1.1#Reality"
	parsed, err := ParseLink(link)
	if err != nil {
		t.Fatalf("parse vless: %v", err)
	}
	if parsed.Protocol != "vless" || parsed.Address != "example.com" || parsed.Port != 443 || parsed.Username != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected parsed vless: %+v", parsed)
	}
	for _, want := range []string{`"reality":true`, `"network":"ws"`, `"path":"/ws"`, `"flow":"xtls-rprx-vision"`, `"pbk":"PUB"`, `"sid":"ab"`} {
		if !strings.Contains(parsed.SettingsJSON, want) {
			t.Fatalf("settings missing %s: %s", want, parsed.SettingsJSON)
		}
	}
}

func TestParseTrojanTLSAndSS(t *testing.T) {
	trojan, err := ParseLink("trojan://secret@example.com:443?security=tls&sni=tls.example.com&fp=firefox#TR")
	if err != nil {
		t.Fatalf("parse trojan: %v", err)
	}
	if trojan.Protocol != "trojan" || trojan.Password != "secret" || !strings.Contains(trojan.SettingsJSON, `"tls":true`) {
		t.Fatalf("unexpected trojan: %+v", trojan)
	}
	ss, err := ParseLink("ss://YWVzLTEyOC1nY206cGFzcw@example.net:8388#SS")
	if err != nil {
		t.Fatalf("parse ss: %v", err)
	}
	if ss.Protocol != "shadowsocks" || ss.Username != "aes-128-gcm" || ss.Password != "pass" {
		t.Fatalf("unexpected ss: %+v", ss)
	}
}

func TestParseLinksReturnsSkippedEntries(t *testing.T) {
	result, err := ParseLinks([]string{
		"trojan://secret@example.com:443#TR",
		"vmess://eyJhZGQiOiJleGFtcGxlLmNvbSJ9",
		"unknown://example.com:443",
	})
	if err != nil {
		t.Fatalf("parse mixed links: %v", err)
	}
	if len(result.Nodes) != 1 || len(result.Skipped) != 2 {
		t.Fatalf("unexpected parse result: %+v", result)
	}
	if result.Skipped[0].Protocol != "vmess" || !strings.Contains(result.Skipped[0].Reason, "not supported") {
		t.Fatalf("expected vmess skip reason, got %+v", result.Skipped[0])
	}
}

func TestMaterializeKeepsStableTags(t *testing.T) {
	first, _ := ParseLink("trojan://secret@example.com:443#one")
	second, _ := ParseLink("trojan://secret2@example.net:443#two")
	nodes, ids := Materialize(7, []ParsedLink{first, second}, nil, "sub1-")
	if len(nodes) != 2 || len(ids) != 2 {
		t.Fatalf("unexpected materialized nodes: %+v ids=%+v", nodes, ids)
	}
	existing := []db.Outbound{
		{ID: 10, Tag: "kept-identity", Source: db.OutboundSourceSubscription, SubscriptionID: 7, SubscriptionIdentity: ids[1], Sort: 50001},
		{ID: 11, Tag: "kept-position", Source: db.OutboundSourceSubscription, SubscriptionID: 7, SubscriptionIdentity: "old", Sort: 50000},
	}
	nextNodes, _ := Materialize(7, []ParsedLink{first, second}, existing, "sub1-")
	if nextNodes[0].Tag != "kept-position" || nextNodes[1].Tag != "kept-identity" {
		t.Fatalf("expected stable tags by position and identity, got %+v", nextNodes)
	}
}

func TestHTTPFetcherRejectsPrivateAndRedirect(t *testing.T) {
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("vless://x"))
	}))
	defer private.Close()
	if _, err := (HTTPFetcher{}).Fetch(context.Background(), private.URL, false); err == nil {
		t.Fatal("expected private test server URL to be rejected")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, private.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := (HTTPFetcher{}).Fetch(context.Background(), redirect.URL, true); err != nil {
		t.Fatalf("allow_private should allow local redirect target in test: %v", err)
	}
}

func TestResolveDialTargetRejectsPrivateAtDialStage(t *testing.T) {
	if _, err := resolveDialTarget(context.Background(), "127.0.0.1", false); err == nil {
		t.Fatal("expected loopback literal to be rejected at dial stage")
	}
	if _, err := resolveDialTarget(context.Background(), "localhost", false); err == nil {
		t.Fatal("expected localhost to be rejected at dial stage")
	}
	if addr, err := resolveDialTarget(context.Background(), "127.0.0.1", true); err != nil || !addr.IsLoopback() {
		t.Fatalf("allow_private should allow loopback literal, addr=%v err=%v", addr, err)
	}
}

func TestSafeHTTPClientDisablesEnvironmentProxy(t *testing.T) {
	client := safeHTTPClient(false)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("safe subscription fetch transport must not use environment proxy")
	}
}

func TestHTTPFetcherIgnoresInjectedUnsafeTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("trojan://secret@example.com:443#node"))
	}))
	defer server.Close()

	called := false
	unsafeTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unsafe transport should not be used")
	})
	if _, err := (HTTPFetcher{Client: &http.Client{Transport: unsafeTransport}}).Fetch(context.Background(), server.URL, true); err != nil {
		t.Fatalf("fetch with safe transport should reach controlled test server: %v", err)
	}
	if called {
		t.Fatal("injected client transport should not replace safe subscription transport")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
