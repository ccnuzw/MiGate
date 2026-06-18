package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/xray"
)

func subscriptionHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if cfg == nil {
			http.NotFound(w, r)
			return
		}
		store := cfg.store
		if store == nil {
			http.NotFound(w, r)
			return
		}
		token := strings.Trim(strings.TrimPrefix(r.URL.Path, "/sub/"), "/")
		if token == "" {
			http.NotFound(w, r)
			return
		}
		inbound, client, found, err := store.GetSubscriptionByToken(r.Context(), token)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "get_subscription_failed")
			return
		}
		if !found || !inbound.Enabled {
			writeInactiveSubscription(w)
			return
		}
		inbounds := []db.Inbound{inbound}
		deriveRealityPublicKeys(inbounds)
		inbound = inbounds[0]
		now := time.Now().Unix()
		if !client.Enabled {
			writeInactiveSubscription(w)
			return
		}
		if client.ExpiryAt > 0 && client.ExpiryAt <= now {
			writeInactiveSubscription(w)
			return
		}
		if client.TrafficLimit > 0 {
			usage, found, err := store.GetClientTrafficUsageForClient(r.Context(), client.ID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "get_traffic_usage_failed")
				return
			}
			used := client.Up + client.Down
			if found {
				used = usage.TotalUp + usage.TotalDown
			}
			if used >= client.TrafficLimit {
				writeInactiveSubscription(w)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		link, err := shareLink(subscriptionRequestHost(cfg, r), inbound, client)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		_, _ = w.Write([]byte(link))
	}
}

func writeInactiveSubscription(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("// Subscription unavailable"))
}

func subscriptionRequestHost(cfg *routerConfig, r *http.Request) string {
	if cfg != nil && strings.TrimSpace(cfg.publicHost) != "" {
		return cfg.publicHost
	}
	return r.Host
}

type shareLinkGenerator func(host string, inbound db.Inbound, client db.Client) string

var shareLinkGenerators = map[string]shareLinkGenerator{
	"vless":       universalShareLink,
	"vmess":       vmessShareLink,
	"trojan":      universalShareLink,
	"shadowsocks": ssShareLink,
	"hysteria2":   hysteria2ShareLink,
	"tuic":        tuicShareLink,
}

func shareLink(host string, inbound db.Inbound, client db.Client) (string, error) {
	protocol := db.NormalizeInboundProtocol(inbound.Protocol)
	if !db.InboundSupportsShareLink(protocol) {
		return "", fmt.Errorf("%s inbound does not support share links", protocol)
	}
	generator, ok := shareLinkGenerators[protocol]
	if !ok {
		return "", fmt.Errorf("unsupported share link protocol: %s", protocol)
	}
	inbound.Protocol = protocol
	return generator(subscriptionHost(host), inbound, client), nil
}

func universalShareLink(host string, inbound db.Inbound, client db.Client) string {
	var params []string
	addParam := func(k, v string) {
		if v != "" {
			params = append(params, k+"="+url.QueryEscape(v))
		}
	}
	addParam("type", inbound.Network)
	addParam("security", inbound.Security)
	if inbound.Security == "reality" {
		if inbound.Network != "xhttp" {
			params = append(params, "flow=xtls-rprx-vision")
		}
		addParam("sni", firstCSV(inbound.RealityServerNames))
		addParam("fp", firstNonEmpty(inbound.TLSFingerprint, "chrome"))
		addParam("pbk", inbound.RealityPublicKey)
		addParam("sid", inbound.RealityShortID)
	} else if inbound.Security == "tls" {
		addParam("sni", inbound.TLSSNI)
		params = append(params, "allowInsecure=1")
	}
	switch inbound.Network {
	case "ws":
		addParam("path", inbound.WsPath)
		addParam("host", inbound.WsHost)
	case "h2":
		addParam("path", inbound.WsPath)
		addParam("host", inbound.WsHost)
	case "grpc":
		addParam("serviceName", inbound.GrpcServiceName)
	case "xhttp":
		addParam("path", inbound.XHTTPPath)
		addParam("mode", inbound.XHTTPMode)
	}
	query := strings.Join(params, "&")
	credential := client.CredentialIDValue()
	if inbound.Protocol == "trojan" {
		credential = client.PasswordValue()
	}
	return inbound.Protocol + "://" + url.User(credential).String() + "@" + host + ":" + strconv.Itoa(inbound.Port) + "?" + query + "#" + url.QueryEscape(client.Email)
}

func hysteria2ShareLink(host string, inbound db.Inbound, client db.Client) string {
	var params []string
	addParam := func(k, v string) {
		if v != "" {
			params = append(params, k+"="+url.QueryEscape(v))
		}
	}
	if inbound.Hy2UpMbps > 0 {
		params = append(params, "up_mbps="+strconv.Itoa(inbound.Hy2UpMbps))
	}
	if inbound.Hy2DownMbps > 0 {
		params = append(params, "down_mbps="+strconv.Itoa(inbound.Hy2DownMbps))
	}
	addParam("obfs", inbound.Hy2Obfs)
	addParam("obfs-password", inbound.Hy2ObfsPassword)
	params = append(params, "security=tls")
	addParam("sni", inbound.TLSSNI)
	params = append(params, "insecure=1")
	query := strings.Join(params, "&")
	suffix := ""
	if query != "" {
		suffix = "?" + query
	}
	return "hysteria2://" + url.User(client.PasswordValue()).String() + "@" + host + ":" + strconv.Itoa(inbound.Port) + suffix + "#" + url.QueryEscape(client.Email)
}

func tuicShareLink(host string, inbound db.Inbound, client db.Client) string {
	var params []string
	addParam := func(k, v string) {
		if v != "" {
			params = append(params, k+"="+url.QueryEscape(v))
		}
	}
	addParam("sni", inbound.TLSSNI)
	addParam("congestion_control", firstNonEmpty(inbound.TuicCongestionControl, "bbr"))
	if inbound.TuicZeroRTT {
		params = append(params, "zero_rtt_handshake=1")
	}
	params = append(params, "insecure=1")
	query := strings.Join(params, "&")
	credential := url.UserPassword(client.CredentialIDValue(), client.PasswordValue()).String()
	return "tuic://" + credential + "@" + host + ":" + strconv.Itoa(inbound.Port) + "?" + query + "#" + url.QueryEscape(client.Email)
}

func vmessShareLink(host string, inbound db.Inbound, client db.Client) string {
	inboundPort := inbound.Port
	portStr := strconv.Itoa(inboundPort)
	tls := ""
	if inbound.Security == "tls" || inbound.Security == "reality" {
		tls = "tls"
	}

	// Transport-specific host and path
	vHost, vPath := "", ""
	sni := ""
	switch inbound.Network {
	case "ws":
		vHost = inbound.WsHost
		vPath = inbound.WsPath
	case "grpc":
		vPath = inbound.GrpcServiceName
	case "xhttp":
		vPath = inbound.XHTTPPath
	case "h2":
		vHost = inbound.WsHost
		vPath = inbound.WsPath
	}
	if inbound.Security == "tls" || inbound.Security == "reality" {
		if inbound.Security == "reality" {
			sni = firstCSV(inbound.RealityServerNames)
		} else {
			sni = inbound.TLSSNI
		}
	}
	vmessData := map[string]interface{}{
		"v":    "2",
		"ps":   client.Email,
		"add":  host,
		"port": portStr,
		"id":   client.CredentialIDValue(),
		"aid":  "0",
		"scy":  "auto",
		"net":  inbound.Network,
		"type": "none",
		"host": vHost,
		"path": vPath,
		"tls":  tls,
		"sni":  sni,
	}
	b, _ := json.Marshal(vmessData)
	encoded := base64.StdEncoding.EncodeToString(b)
	return "vmess://" + encoded
}

func ssShareLink(host string, inbound db.Inbound, client db.Client) string {
	method := inbound.SSMethod
	if method == "" {
		method = "2022-blake3-aes-128-gcm"
	}
	userPass := method + ":" + xray.SSInboundPassword(method, inbound.UUID)
	encoded := base64.StdEncoding.EncodeToString([]byte(userPass))
	return "ss://" + encoded + "@" + host + ":" + strconv.Itoa(inbound.Port) + "#" + url.QueryEscape(client.Email)
}

func firstCSV(value string) string {
	for _, part := range strings.Split(value, ",") {
		if strings.TrimSpace(part) != "" {
			return strings.TrimSpace(part)
		}
	}
	return ""
}

func subscriptionHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "SERVER_IP"
	}
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return "SERVER_IP"
		}
		host = parsed.Hostname()
		if host == "" {
			return "SERVER_IP"
		}
	}
	name, _, err := net.SplitHostPort(host)
	if err == nil && name != "" {
		host = name
	}
	host = strings.Trim(host, "[]")
	if validSubscriptionHost(host) {
		if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
			return "[" + host + "]"
		}
		return host
	}
	return "SERVER_IP"
}

func validSubscriptionHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, "/\\@?#") {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, ch := range label {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return false
		}
	}
	return true
}
