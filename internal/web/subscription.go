package web

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

func subscriptionHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if store == nil {
			http.NotFound(w, r)
			return
		}
		token := strings.Trim(strings.TrimPrefix(r.URL.Path, "/sub/"), "/")
		if token == "" {
			http.NotFound(w, r)
			return
		}
		inbound, client, found, err := store.GetSubscriptionByClientUUID(r.Context(), token)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "get_subscription_failed")
			return
		}
		if !found || !inbound.Enabled {
			http.NotFound(w, r)
			return
		}
		inbounds := []db.Inbound{inbound}
		deriveRealityPublicKeys(inbounds)
		inbound = inbounds[0]
		now := time.Now().Unix()
		if !client.Enabled {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("// Subscription disabled"))
			return
		}
		if client.ExpiryAt > 0 && client.ExpiryAt <= now {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("// Subscription expired"))
			return
		}
		if client.TrafficLimit > 0 && (client.Up+client.Down) >= client.TrafficLimit {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("// Traffic limit exceeded"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(shareLink(r.Host, inbound, client)))
	}
}

func shareLink(host string, inbound db.Inbound, client db.Client) string {
	host = subscriptionHost(host)
	switch inbound.Protocol {
	case "vmess":
		return vmessShareLink(host, inbound, client)
	case "shadowsocks":
		return ssShareLink(host, inbound, client)
	case "hysteria2":
		// hysteria2://password@host:port/?params#name
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
		// sing-box v1.13 requires TLS for Hysteria2 server inbounds.
		// MiGate uses generated self-signed certs by default, so share links must
		// include TLS + insecure even when the UI stores security=none.
		params = append(params, "security=tls")
		addParam("sni", inbound.RealityServerNames)
		params = append(params, "insecure=1")
		query := strings.Join(params, "&")
		suffix := ""
		if query != "" {
			suffix = "?" + query
		}
		return "hysteria2://" + client.UUID + "@" + host + ":" + strconv.Itoa(inbound.Port) + suffix + "#" + url.QueryEscape(client.Email)
	default:
		// vless, trojan, etc. use universal link format
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
			addParam("sni", inbound.RealityServerNames)
			params = append(params, "fp=chrome")
			addParam("pbk", inbound.RealityPublicKey)
			addParam("sid", inbound.RealityShortID)
		} else if inbound.Security == "tls" {
			addParam("sni", inbound.RealityServerNames)
			params = append(params, "allowInsecure=1")
		}
		// Transport-specific params
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
		case "kcp":
		case "quic":
		}
		query := strings.Join(params, "&")
		return inbound.Protocol + "://" + client.UUID + "@" + host + ":" + strconv.Itoa(inbound.Port) + "?" + query + "#" + url.QueryEscape(client.Email)
	}
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
		sni = inbound.RealityServerNames
	}
	vmessData := map[string]interface{}{
		"v":    "2",
		"ps":   client.Email,
		"add":  host,
		"port": portStr,
		"id":   client.UUID,
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
	userPass := method + ":" + inbound.UUID
	encoded := base64.StdEncoding.EncodeToString([]byte(userPass))
	return "ss://" + encoded + "@" + host + ":" + strconv.Itoa(inbound.Port) + "#" + url.QueryEscape(client.Email)
}

func subscriptionHost(host string) string {
	if host == "" {
		return "SERVER_IP"
	}
	name, _, err := net.SplitHostPort(host)
	if err == nil && name != "" {
		return name
	}
	return strings.Trim(host, "[]")
}
