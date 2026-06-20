package subscription

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

const (
	MaxBodyBytes = 2 << 20
	UserAgent    = "MiGate outbound-subscription/1.0"
)

type ParsedLink struct {
	Identity     string
	Remark       string
	Protocol     string
	Address      string
	Port         int
	Username     string
	Password     string
	RawLink      string
	SettingsJSON string
}

type SkippedLink struct {
	Raw      string `json:"raw"`
	Reason   string `json:"reason"`
	Protocol string `json:"protocol,omitempty"`
}

type ParseResult struct {
	Nodes   []ParsedLink  `json:"nodes"`
	Skipped []SkippedLink `json:"skipped"`
}

type Settings struct {
	Security         string   `json:"security,omitempty"`
	Network          string   `json:"network,omitempty"`
	TLS              bool     `json:"tls,omitempty"`
	Reality          bool     `json:"reality,omitempty"`
	SNI              string   `json:"sni,omitempty"`
	Host             string   `json:"host,omitempty"`
	Path             string   `json:"path,omitempty"`
	Flow             string   `json:"flow,omitempty"`
	Fingerprint      string   `json:"fp,omitempty"`
	PublicKey        string   `json:"pbk,omitempty"`
	ShortID          string   `json:"sid,omitempty"`
	ALPN             []string `json:"alpn,omitempty"`
	Method           string   `json:"method,omitempty"`
	ServiceName      string   `json:"service_name,omitempty"`
	AllowInsecure    bool     `json:"allow_insecure,omitempty"`
	VmessUnsupported bool     `json:"vmess_unsupported,omitempty"`
}

type HTTPFetcher struct {
	Client *http.Client
}

func (f HTTPFetcher) Fetch(ctx context.Context, rawURL string, allowPrivate bool) ([]byte, error) {
	parsed, err := validateFetchURL(ctx, rawURL, allowPrivate)
	if err != nil {
		return nil, err
	}
	client := safeHTTPClient(allowPrivate)
	if f.Client != nil {
		if f.Client.Timeout > 0 {
			client.Timeout = f.Client.Timeout
		}
		client.Jar = f.Client.Jar
	}
	baseCheckRedirect := client.CheckRedirect
	if f.Client != nil && f.Client.CheckRedirect != nil {
		baseCheckRedirect = f.Client.CheckRedirect
	}
	copyClient := *client
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if baseCheckRedirect != nil {
			if err := baseCheckRedirect(req, via); err != nil {
				return err
			}
		}
		if _, err := validateFetchURL(req.Context(), req.URL.String(), allowPrivate); err != nil {
			return err
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := copyClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription upstream returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBodyBytes {
		return nil, fmt.Errorf("subscription body exceeds %d bytes", MaxBodyBytes)
	}
	return body, nil
}

func safeHTTPClient(allowPrivate bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		target, err := resolveDialTarget(ctx, host, allowPrivate)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(target.String(), port))
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}
}

func DecodeBody(body []byte) []string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return nil
	}
	if decoded, ok := tryDecodeBase64(text); ok {
		text = strings.TrimSpace(decoded)
	}
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	links := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "://") {
			links = append(links, line)
		}
	}
	return links
}

func ParseBody(body []byte) ([]ParsedLink, error) {
	result, err := ParseLinks(DecodeBody(body))
	if err != nil {
		return nil, err
	}
	return result.Nodes, nil
}

func ParseLinks(links []string) (ParseResult, error) {
	result := ParseResult{Nodes: make([]ParsedLink, 0, len(links))}
	parsed := make([]ParsedLink, 0, len(links))
	for _, raw := range links {
		item, err := ParseLink(raw)
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedLink{Raw: raw, Reason: err.Error(), Protocol: linkProtocol(raw)})
			continue
		}
		parsed = append(parsed, item)
	}
	if len(parsed) == 0 && len(links) > 0 {
		return result, fmt.Errorf("no supported subscription links found")
	}
	result.Nodes = parsed
	return result, nil
}

func ParseLink(raw string) (ParsedLink, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedLink{}, errors.New("empty link")
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "vless://"):
		return parseURLLink(raw, "vless")
	case strings.HasPrefix(lower, "trojan://"):
		return parseURLLink(raw, "trojan")
	case strings.HasPrefix(lower, "ss://"):
		return parseSSLink(raw)
	case strings.HasPrefix(lower, "vmess://"):
		return ParsedLink{}, errors.New("vmess links are not supported yet")
	default:
		return ParsedLink{}, fmt.Errorf("unsupported link")
	}
}

func linkProtocol(raw string) string {
	raw = strings.TrimSpace(raw)
	idx := strings.Index(raw, "://")
	if idx <= 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(raw[:idx]))
}

func StableIdentity(raw string, parsed ParsedLink) string {
	base := strings.ToLower(strings.TrimSpace(parsed.Protocol)) + "|" + strings.TrimSpace(parsed.Address) + "|" + strconv.Itoa(parsed.Port) + "|" + strings.TrimSpace(parsed.Username) + "|" + strings.TrimSpace(parsed.Password)
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}

func Materialize(subscriptionID int64, links []ParsedLink, existing []db.Outbound, tagPrefix string) ([]db.MaterializedSubscriptionOutbound, []string) {
	byIdentity := map[string]db.Outbound{}
	subscriptionOutbounds := []db.Outbound{}
	usedTags := map[string]bool{}
	for _, outbound := range existing {
		if strings.TrimSpace(outbound.Tag) != "" {
			usedTags[outbound.Tag] = true
		}
		if outbound.SubscriptionID == subscriptionID && outbound.Source == db.OutboundSourceSubscription {
			subscriptionOutbounds = append(subscriptionOutbounds, outbound)
			if strings.TrimSpace(outbound.SubscriptionIdentity) != "" {
				byIdentity[outbound.SubscriptionIdentity] = outbound
			}
		}
	}
	sort.Slice(subscriptionOutbounds, func(i, j int) bool {
		if subscriptionOutbounds[i].Sort == subscriptionOutbounds[j].Sort {
			return subscriptionOutbounds[i].ID < subscriptionOutbounds[j].ID
		}
		return subscriptionOutbounds[i].Sort < subscriptionOutbounds[j].Sort
	})
	nodes := make([]db.MaterializedSubscriptionOutbound, 0, len(links))
	identities := make([]string, 0, len(links))
	for i, link := range links {
		identity := strings.TrimSpace(link.Identity)
		if identity == "" {
			identity = StableIdentity(link.RawLink, link)
		}
		identities = append(identities, identity)
		var existingID int64
		tag := ""
		if old, ok := byIdentity[identity]; ok {
			existingID = old.ID
			tag = old.Tag
		} else if i < len(subscriptionOutbounds) {
			old := subscriptionOutbounds[i]
			existingID = old.ID
			tag = old.Tag
		}
		if strings.TrimSpace(tag) == "" {
			tag = uniqueTag(tagPrefix+slug(link.Remark), usedTags)
		}
		usedTags[tag] = true
		nodes = append(nodes, db.MaterializedSubscriptionOutbound{
			ID: existingID, Tag: tag, Remark: firstNonEmpty(link.Remark, link.Address), Protocol: link.Protocol, Address: link.Address, Port: link.Port,
			Username: link.Username, Password: link.Password, SubscriptionIdentity: identity, RawLink: link.RawLink, SettingsJSON: link.SettingsJSON, Position: i,
		})
	}
	return nodes, identities
}

func parseURLLink(raw string, protocol string) (ParsedLink, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedLink{}, err
	}
	address := strings.TrimSpace(u.Hostname())
	port, _ := strconv.Atoi(u.Port())
	if address == "" || port <= 0 || port > 65535 {
		return ParsedLink{}, fmt.Errorf("invalid endpoint")
	}
	settings := settingsFromQuery(u.Query(), address)
	username := ""
	password := ""
	switch protocol {
	case "vless":
		username = strings.TrimSpace(u.User.Username())
	case "trojan":
		password, _ = u.User.Password()
		if password == "" {
			password = strings.TrimSpace(u.User.Username())
		}
	}
	settingsJSON, _ := json.Marshal(settings)
	remark, _ := url.QueryUnescape(strings.TrimPrefix(u.Fragment, "#"))
	item := ParsedLink{
		Remark: strings.TrimSpace(remark), Protocol: protocol, Address: address, Port: port, Username: username, Password: password,
		RawLink: raw, SettingsJSON: string(settingsJSON),
	}
	item.Identity = StableIdentity(raw, item)
	return item, nil
}

func parseSSLink(raw string) (ParsedLink, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedLink{}, err
	}
	remark, _ := url.QueryUnescape(strings.TrimPrefix(u.Fragment, "#"))
	address := strings.TrimSpace(u.Hostname())
	port, _ := strconv.Atoi(u.Port())
	userInfo := ""
	if u.User != nil {
		userInfo = userInfoString(u.User)
	}
	if address == "" || port == 0 {
		payload := strings.TrimPrefix(raw, "ss://")
		if idx := strings.Index(payload, "#"); idx >= 0 {
			payload = payload[:idx]
		}
		if idx := strings.Index(payload, "?"); idx >= 0 {
			payload = payload[:idx]
		}
		if decoded, ok := tryDecodeBase64(payload); ok {
			if parsed, err := url.Parse("ss://" + decoded); err == nil {
				address = parsed.Hostname()
				port, _ = strconv.Atoi(parsed.Port())
				if parsed.User != nil {
					userInfo = userInfoString(parsed.User)
				}
			}
		}
	}
	if address == "" || port <= 0 || port > 65535 || userInfo == "" {
		return ParsedLink{}, fmt.Errorf("invalid ss link")
	}
	if decoded, ok := tryDecodeBase64(userInfo); ok {
		userInfo = decoded
	}
	parts := strings.SplitN(userInfo, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return ParsedLink{}, fmt.Errorf("invalid ss credentials")
	}
	settings := settingsFromQuery(u.Query(), address)
	settings.Method = strings.TrimSpace(parts[0])
	settingsJSON, _ := json.Marshal(settings)
	item := ParsedLink{
		Remark: strings.TrimSpace(remark), Protocol: "shadowsocks", Address: address, Port: port, Username: strings.TrimSpace(parts[0]), Password: parts[1],
		RawLink: raw, SettingsJSON: string(settingsJSON),
	}
	item.Identity = StableIdentity(raw, item)
	return item, nil
}

func userInfoString(user *url.Userinfo) string {
	if user == nil {
		return ""
	}
	username := user.Username()
	password, hasPassword := user.Password()
	if hasPassword {
		return username + ":" + password
	}
	return username
}

func settingsFromQuery(values url.Values, address string) Settings {
	security := strings.ToLower(firstQuery(values, "security", "tls"))
	network := strings.ToLower(firstQuery(values, "type", "network"))
	settings := Settings{
		Security:      security,
		Network:       network,
		SNI:           firstQuery(values, "sni", "servername", "serverName", "peer"),
		Host:          firstQuery(values, "host"),
		Path:          firstQuery(values, "path"),
		Flow:          firstQuery(values, "flow"),
		Fingerprint:   firstQuery(values, "fp", "fingerprint"),
		PublicKey:     firstQuery(values, "pbk", "publicKey"),
		ShortID:       firstQuery(values, "sid", "shortId"),
		ServiceName:   firstQuery(values, "serviceName", "service_name"),
		AllowInsecure: truthy(firstQuery(values, "allowInsecure", "allow_insecure", "skip-cert-verify")),
	}
	if settings.SNI == "" && security != "" && security != "none" {
		settings.SNI = address
	}
	if security == "tls" {
		settings.TLS = true
	}
	if security == "reality" {
		settings.Reality = true
	}
	if alpn := firstQuery(values, "alpn"); alpn != "" {
		settings.ALPN = splitList(alpn)
	}
	return settings
}

func validateFetchURL(ctx context.Context, rawURL string, allowPrivate bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("subscription url must be http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("subscription url host cannot be empty")
	}
	if _, err := resolveDialTarget(ctx, host, allowPrivate); err != nil {
		return nil, err
	}
	return parsed, nil
}

func resolveDialTarget(ctx context.Context, host string, allowPrivate bool) (netip.Addr, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return netip.Addr{}, fmt.Errorf("subscription url host cannot be empty")
	}
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !allowPrivate && !isPublicAddr(addr) {
			return netip.Addr{}, fmt.Errorf("subscription url resolves to private or local address")
		}
		return addr, nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(ips) == 0 {
		return netip.Addr{}, fmt.Errorf("subscription url host has no addresses")
	}
	for _, ipAddr := range ips {
		addr, ok := netip.AddrFromSlice(ipAddr.IP)
		if !ok {
			continue
		}
		if allowPrivate || isPublicAddr(addr) {
			return addr, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("subscription url resolves to private or local address")
}

func isPublicAddr(addr netip.Addr) bool {
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast() && !addr.IsLinkLocalMulticast() && !addr.IsMulticast() && !addr.IsUnspecified()
}

func tryDecodeBase64(s string) (string, bool) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, strings.TrimSpace(s))
	if clean == "" {
		return "", false
	}
	if b, err := base64.StdEncoding.DecodeString(padBase64(clean)); err == nil {
		return string(b), true
	}
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(clean, "=")); err == nil {
		return string(b), true
	}
	if b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "=")); err == nil {
		return string(b), true
	}
	return "", false
}

func padBase64(s string) string {
	if rem := len(s) % 4; rem != 0 {
		s += strings.Repeat("=", 4-rem)
	}
	return s
}

func firstQuery(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			if decoded, err := url.QueryUnescape(value); err == nil {
				return strings.TrimSpace(decoded)
			}
			return value
		}
	}
	return ""
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '|' || r == ' ' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func slug(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "node"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r > 127:
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "node"
	}
	return out
}

func uniqueTag(base string, used map[string]bool) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "sub-node"
	}
	tag := base
	for i := 2; used[tag]; i++ {
		tag = fmt.Sprintf("%s-%d", base, i)
	}
	return tag
}
