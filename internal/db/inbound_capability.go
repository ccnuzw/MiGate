package db

import (
	"fmt"
	"strings"
)

const (
	CredentialNone             = "none"
	CredentialUUID             = "uuid"
	CredentialPassword         = "password"
	CredentialIDPassword       = "credential_id_password"
	CredentialUsernamePassword = "username_password"

	SubscriptionNone = "none"
	SubscriptionFull = "full"
)

type InboundCapability struct {
	Protocol           string              `json:"protocol"`
	Core               string              `json:"core"`
	TemplateID         string              `json:"template_id,omitempty"`
	TemplateLabel      string              `json:"template_label,omitempty"`
	TemplateSummary    string              `json:"template_summary,omitempty"`
	Networks           []string            `json:"networks"`
	Securities         []string            `json:"securities"`
	DefaultNetwork     string              `json:"default_network"`
	DefaultSecurity    string              `json:"default_security"`
	SecurityByNetwork  map[string][]string `json:"security_by_network"`
	VisibleFields      []string            `json:"visible_fields,omitempty"`
	AutoGenerateFields []string            `json:"auto_generate_fields,omitempty"`
	ExpertFields       []string            `json:"expert_fields,omitempty"`
	AdvancedFields     []string            `json:"advanced_fields"`
	CredentialType     string              `json:"credential_type"`
	Subscription       string              `json:"subscription"`
	ShareLink          bool                `json:"share_link"`
	LocalProxyInbound  bool                `json:"local_proxy_inbound,omitempty"`
	UnsupportedReasons []string            `json:"unsupported_reasons,omitempty"`
}

var inboundCapabilities = map[string]InboundCapability{
	"vless": {
		Protocol: "vless", Core: CoreXray,
		TemplateID:      "recommended",
		TemplateLabel:   "推荐节点",
		TemplateSummary: "VLESS + TCP + REALITY",
		Networks:       []string{"tcp", "ws", "grpc", "h2", "xhttp"},
		Securities:     []string{"none", "tls", "reality"},
		DefaultNetwork: "tcp", DefaultSecurity: "reality",
		SecurityByNetwork: map[string][]string{
			"default": {"none", "tls"},
			"tcp":     {"none", "tls", "reality"},
			"grpc":    {"none", "tls", "reality"},
			"xhttp":   {"none", "tls", "reality"},
		},
		VisibleFields:      []string{"remark", "port", "public_host", "reality_dest", "reality_server_names", "tls_certificate"},
		AutoGenerateFields: []string{"uuid", "client_uuid", "reality_private_key", "reality_public_key", "reality_short_id"},
		ExpertFields:       []string{"uuid", "ws_path", "ws_host", "grpc_service_name", "reality_short_id", "reality_private_key", "reality_public_key", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		AdvancedFields:     []string{"ws_path", "ws_host", "grpc_service_name", "reality_dest", "reality_server_names", "reality_short_id", "reality_private_key", "reality_public_key", "tls_cert_file", "tls_key_file", "tls_sni", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		CredentialType:     CredentialUUID, Subscription: SubscriptionFull, ShareLink: true,
	},
	"vmess": {
		Protocol: "vmess", Core: CoreXray,
		TemplateID:      "compatible",
		TemplateLabel:   "兼容节点",
		TemplateSummary: "VMess + WS + TLS",
		Networks:       []string{"tcp", "ws", "grpc", "h2", "xhttp"},
		Securities:     []string{"none", "tls"},
		DefaultNetwork: "ws", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{
			"default": {"none", "tls"},
		},
		VisibleFields:      []string{"remark", "port", "public_host", "tls_sni", "tls_certificate"},
		AutoGenerateFields: []string{"uuid", "client_uuid"},
		ExpertFields:       []string{"uuid", "ws_path", "ws_host", "grpc_service_name", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		AdvancedFields:     []string{"ws_path", "ws_host", "grpc_service_name", "tls_cert_file", "tls_key_file", "tls_sni", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		CredentialType:     CredentialUUID, Subscription: SubscriptionFull, ShareLink: true,
	},
	"trojan": {
		Protocol: "trojan", Core: CoreXray,
		TemplateID:      "password",
		TemplateLabel:   "密码节点",
		TemplateSummary: "Trojan + TLS",
		Networks:       []string{"tcp", "ws", "grpc", "h2", "xhttp"},
		Securities:     []string{"none", "tls", "reality"},
		DefaultNetwork: "tcp", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{
			"default": {"none", "tls"},
			"tcp":     {"none", "tls", "reality"},
			"grpc":    {"none", "tls", "reality"},
			"xhttp":   {"none", "tls", "reality"},
		},
		VisibleFields:      []string{"remark", "port", "public_host", "tls_sni", "tls_certificate"},
		AutoGenerateFields: []string{"uuid", "client_password", "reality_private_key", "reality_public_key", "reality_short_id"},
		ExpertFields:       []string{"uuid", "ws_path", "ws_host", "grpc_service_name", "reality_dest", "reality_server_names", "reality_short_id", "reality_private_key", "reality_public_key", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		AdvancedFields:     []string{"ws_path", "ws_host", "grpc_service_name", "reality_dest", "reality_server_names", "reality_short_id", "reality_private_key", "reality_public_key", "tls_cert_file", "tls_key_file", "tls_sni", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		CredentialType:     CredentialPassword, Subscription: SubscriptionFull, ShareLink: true,
	},
	"shadowsocks": {
		Protocol: "shadowsocks", Core: CoreXray,
		TemplateID:      "light",
		TemplateLabel:   "轻量节点",
		TemplateSummary: "Shadowsocks 2022",
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		VisibleFields:      []string{"remark", "port", "public_host"},
		AutoGenerateFields: []string{"uuid", "shadowsocks_password"},
		ExpertFields:       []string{"uuid", "ss_method"},
		AdvancedFields:     []string{"ss_method"},
		CredentialType:     CredentialNone, Subscription: SubscriptionFull, ShareLink: true,
	},
	"socks": {
		Protocol: "socks", Core: CoreXray,
		TemplateID:      "local-socks",
		TemplateLabel:   "本地代理",
		TemplateSummary: "SOCKS",
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		VisibleFields:      []string{"remark", "port"},
		AutoGenerateFields: []string{"uuid", "username", "password"},
		ExpertFields:       []string{"uuid"},
		CredentialType:     CredentialUsernamePassword, Subscription: SubscriptionFull, ShareLink: true,
		LocalProxyInbound:  true,
	},
	"http": {
		Protocol: "http", Core: CoreXray,
		TemplateID:      "local-http",
		TemplateLabel:   "本地代理",
		TemplateSummary: "HTTP",
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		VisibleFields:      []string{"remark", "port"},
		AutoGenerateFields: []string{"uuid", "username", "password"},
		ExpertFields:       []string{"uuid"},
		CredentialType:     CredentialUsernamePassword, Subscription: SubscriptionFull, ShareLink: true,
		LocalProxyInbound:  true,
	},
	"hysteria2": {
		Protocol: "hysteria2", Core: CoreSingbox,
		TemplateID:      "udp-fast",
		TemplateLabel:   "高速 UDP",
		TemplateSummary: "Hysteria2",
		Networks:       []string{"udp"},
		Securities:     []string{"tls"},
		DefaultNetwork: "udp", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{"default": {"tls"}},
		VisibleFields:      []string{"remark", "port", "public_host", "tls_sni", "tls_certificate"},
		AutoGenerateFields: []string{"uuid", "client_password", "hy2_obfs_password"},
		ExpertFields:       []string{"uuid", "hy2_up_mbps", "hy2_down_mbps", "hy2_obfs", "hy2_obfs_password"},
		AdvancedFields:     []string{"tls_cert_file", "tls_key_file", "tls_sni", "hy2_up_mbps", "hy2_down_mbps", "hy2_obfs", "hy2_obfs_password"},
		CredentialType:     CredentialPassword, Subscription: SubscriptionFull, ShareLink: true,
	},
	"tuic": {
		Protocol: "tuic", Core: CoreSingbox,
		TemplateID:      "low-latency",
		TemplateLabel:   "高速低延迟",
		TemplateSummary: "TUIC",
		Networks:       []string{"udp"},
		Securities:     []string{"tls"},
		DefaultNetwork: "udp", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{"default": {"tls"}},
		VisibleFields:      []string{"remark", "port", "public_host", "tls_sni", "tls_certificate"},
		AutoGenerateFields: []string{"uuid", "tuic_uuid", "tuic_password"},
		ExpertFields:       []string{"uuid", "tuic_congestion_control", "tuic_zero_rtt"},
		AdvancedFields:     []string{"tls_cert_file", "tls_key_file", "tls_sni", "tuic_congestion_control", "tuic_zero_rtt"},
		CredentialType:     CredentialIDPassword, Subscription: SubscriptionFull, ShareLink: true,
	},
	"shadowtls": {
		Protocol: "shadowtls", Core: CoreSingbox,
		TemplateID:      "handshake-mask",
		TemplateLabel:   "伪装握手",
		TemplateSummary: "ShadowTLS",
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		VisibleFields:      []string{"remark", "port", "tls_sni"},
		AutoGenerateFields: []string{"uuid", "client_password"},
		ExpertFields:       []string{"uuid", "shadowtls_version"},
		AdvancedFields:     []string{"tls_sni", "shadowtls_version"},
		CredentialType:     CredentialPassword, Subscription: SubscriptionNone, ShareLink: false,
		UnsupportedReasons: []string{
			"shadowtls share URI is not stable across clients; use manual configuration",
		},
	},
}

func InboundCapabilities() []InboundCapability {
	order := []string{"vless", "vmess", "trojan", "shadowsocks", "socks", "http", "hysteria2", "tuic", "shadowtls"}
	result := make([]InboundCapability, 0, len(order))
	for _, protocol := range order {
		result = append(result, inboundCapabilities[protocol])
	}
	return result
}

func GetInboundCapability(protocol string) (InboundCapability, bool) {
	capability, ok := inboundCapabilities[strings.ToLower(strings.TrimSpace(protocol))]
	return capability, ok
}

func SupportedInboundProtocol(protocol string) bool {
	_, ok := GetInboundCapability(protocol)
	return ok
}

func NormalizeInboundProtocol(protocol string) string {
	return strings.ToLower(strings.TrimSpace(protocol))
}

func NormalizeInboundNetwork(protocol, network string) string {
	capability, ok := GetInboundCapability(protocol)
	if !ok {
		return strings.ToLower(strings.TrimSpace(network))
	}
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" {
		return capability.DefaultNetwork
	}
	return network
}

func NormalizeInboundSecurity(protocol, security string) string {
	capability, ok := GetInboundCapability(protocol)
	if !ok {
		return strings.ToLower(strings.TrimSpace(security))
	}
	security = strings.ToLower(strings.TrimSpace(security))
	if security == "" {
		return capability.DefaultSecurity
	}
	return security
}

func ValidateInboundCombination(inbound Inbound) error {
	protocol := NormalizeInboundProtocol(inbound.Protocol)
	capability, ok := GetInboundCapability(protocol)
	if !ok {
		return fmt.Errorf("unsupported inbound protocol: %s", inbound.Protocol)
	}
	if NormalizeCore(inbound.Core) != capability.Core {
		return fmt.Errorf("%s inbound must use %s core", protocol, capability.Core)
	}
	network := NormalizeInboundNetwork(protocol, inbound.Network)
	if !containsString(capability.Networks, network) {
		return fmt.Errorf("%s inbound does not support network %q", protocol, inbound.Network)
	}
	security := NormalizeInboundSecurity(protocol, inbound.Security)
	if !containsString(InboundSecuritiesForNetwork(capability, network), security) {
		return fmt.Errorf("%s inbound does not support security %q on network %q", protocol, inbound.Security, network)
	}
	if security == "reality" {
		if strings.TrimSpace(inbound.RealityPrivateKey) == "" || strings.TrimSpace(inbound.RealityPublicKey) == "" {
			return fmt.Errorf("reality inbound requires persistent private and public keys")
		}
		if strings.TrimSpace(inbound.RealityDest) == "" {
			return fmt.Errorf("reality inbound requires reality_dest")
		}
		if strings.TrimSpace(inbound.RealityServerNames) == "" {
			return fmt.Errorf("reality inbound requires reality_server_names")
		}
	}
	if protocol == "shadowtls" {
		if inbound.ShadowTLSVersion != 0 && inbound.ShadowTLSVersion != 3 {
			return fmt.Errorf("shadowtls inbound currently supports only v3")
		}
		if strings.TrimSpace(inbound.TLSSNI) == "" {
			return fmt.Errorf("shadowtls inbound requires tls_sni as handshake server")
		}
	}
	return nil
}

func InboundSecuritiesForNetwork(capability InboundCapability, network string) []string {
	if values := capability.SecurityByNetwork[strings.ToLower(strings.TrimSpace(network))]; len(values) > 0 {
		return values
	}
	return capability.SecurityByNetwork["default"]
}

func InboundSupportsSubscription(protocol string) bool {
	return InboundSupportsShareLink(protocol)
}

func InboundSupportsShareLink(protocol string) bool {
	capability, ok := GetInboundCapability(protocol)
	return ok && capability.ShareLink
}

func ValidateClientCredential(protocol string, client Client) error {
	capability, ok := GetInboundCapability(protocol)
	if !ok {
		return fmt.Errorf("unsupported inbound protocol: %s", protocol)
	}
	id := strings.TrimSpace(client.CredentialIDValue())
	password := strings.TrimSpace(client.Password)
	switch capability.CredentialType {
	case CredentialUUID:
		if !IsUUID(id) {
			return fmt.Errorf("%s client requires valid uuid", capability.Protocol)
		}
	case CredentialPassword:
		if password == "" {
			return fmt.Errorf("%s client requires password", capability.Protocol)
		}
	case CredentialIDPassword:
		if !IsUUID(id) {
			return fmt.Errorf("%s client requires valid credential_id uuid", capability.Protocol)
		}
		if password == "" {
			return fmt.Errorf("%s client requires password", capability.Protocol)
		}
	case CredentialUsernamePassword:
		if id == "" {
			return fmt.Errorf("%s client requires username", capability.Protocol)
		}
		if password == "" {
			return fmt.Errorf("%s client requires password", capability.Protocol)
		}
	case CredentialNone:
		return nil
	}
	return nil
}

func containsString(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == want {
			return true
		}
	}
	return false
}
