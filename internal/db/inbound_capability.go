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
	Networks           []string            `json:"networks"`
	Securities         []string            `json:"securities"`
	DefaultNetwork     string              `json:"default_network"`
	DefaultSecurity    string              `json:"default_security"`
	SecurityByNetwork  map[string][]string `json:"security_by_network"`
	AdvancedFields     []string            `json:"advanced_fields"`
	CredentialType     string              `json:"credential_type"`
	Subscription       string              `json:"subscription"`
	LocalProxyInbound  bool                `json:"local_proxy_inbound,omitempty"`
	UnsupportedReasons []string            `json:"unsupported_reasons,omitempty"`
}

var inboundCapabilities = map[string]InboundCapability{
	"vless": {
		Protocol: "vless", Core: CoreXray,
		Networks:       []string{"tcp", "ws", "grpc", "h2", "xhttp"},
		Securities:     []string{"none", "tls", "reality"},
		DefaultNetwork: "tcp", DefaultSecurity: "reality",
		SecurityByNetwork: map[string][]string{
			"default": {"none", "tls"},
			"tcp":     {"none", "tls", "reality"},
			"grpc":    {"none", "tls", "reality"},
			"xhttp":   {"none", "tls", "reality"},
		},
		AdvancedFields: []string{"ws_path", "ws_host", "grpc_service_name", "reality_dest", "reality_server_names", "reality_short_id", "reality_private_key", "reality_public_key", "tls_cert_file", "tls_key_file", "tls_sni", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		CredentialType: CredentialUUID, Subscription: SubscriptionFull,
	},
	"vmess": {
		Protocol: "vmess", Core: CoreXray,
		Networks:       []string{"tcp", "ws", "grpc", "h2", "xhttp"},
		Securities:     []string{"none", "tls"},
		DefaultNetwork: "tcp", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{
			"default": {"none", "tls"},
		},
		AdvancedFields: []string{"ws_path", "ws_host", "grpc_service_name", "tls_cert_file", "tls_key_file", "tls_sni", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		CredentialType: CredentialUUID, Subscription: SubscriptionFull,
	},
	"trojan": {
		Protocol: "trojan", Core: CoreXray,
		Networks:       []string{"tcp", "ws", "grpc", "h2", "xhttp"},
		Securities:     []string{"none", "tls", "reality"},
		DefaultNetwork: "tcp", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{
			"default": {"none", "tls"},
			"tcp":     {"none", "tls", "reality"},
			"grpc":    {"none", "tls", "reality"},
			"xhttp":   {"none", "tls", "reality"},
		},
		AdvancedFields: []string{"ws_path", "ws_host", "grpc_service_name", "reality_dest", "reality_server_names", "reality_short_id", "reality_private_key", "reality_public_key", "tls_cert_file", "tls_key_file", "tls_sni", "tls_fingerprint", "tls_alpn", "xhttp_path", "xhttp_mode"},
		CredentialType: CredentialPassword, Subscription: SubscriptionFull,
	},
	"shadowsocks": {
		Protocol: "shadowsocks", Core: CoreXray,
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		AdvancedFields:    []string{"ss_method"},
		CredentialType:    CredentialNone, Subscription: SubscriptionFull,
	},
	"socks": {
		Protocol: "socks", Core: CoreXray,
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		CredentialType:    CredentialUsernamePassword, Subscription: SubscriptionNone,
		LocalProxyInbound: true,
	},
	"http": {
		Protocol: "http", Core: CoreXray,
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		CredentialType:    CredentialUsernamePassword, Subscription: SubscriptionNone,
		LocalProxyInbound: true,
	},
	"hysteria2": {
		Protocol: "hysteria2", Core: CoreSingbox,
		Networks:       []string{"udp"},
		Securities:     []string{"tls"},
		DefaultNetwork: "udp", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{"default": {"tls"}},
		AdvancedFields:    []string{"tls_cert_file", "tls_key_file", "tls_sni", "hy2_up_mbps", "hy2_down_mbps", "hy2_obfs", "hy2_obfs_password"},
		CredentialType:    CredentialPassword, Subscription: SubscriptionFull,
	},
	"tuic": {
		Protocol: "tuic", Core: CoreSingbox,
		Networks:       []string{"udp"},
		Securities:     []string{"tls"},
		DefaultNetwork: "udp", DefaultSecurity: "tls",
		SecurityByNetwork: map[string][]string{"default": {"tls"}},
		AdvancedFields:    []string{"tls_cert_file", "tls_key_file", "tls_sni", "tuic_congestion_control", "tuic_zero_rtt"},
		CredentialType:    CredentialIDPassword, Subscription: SubscriptionFull,
	},
	"shadowtls": {
		Protocol: "shadowtls", Core: CoreSingbox,
		Networks:       []string{"tcp"},
		Securities:     []string{"none"},
		DefaultNetwork: "tcp", DefaultSecurity: "none",
		SecurityByNetwork: map[string][]string{"default": {"none"}},
		AdvancedFields:    []string{"tls_sni", "shadowtls_version"},
		CredentialType:    CredentialPassword, Subscription: SubscriptionNone,
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
	capability, ok := GetInboundCapability(protocol)
	return ok && capability.Subscription == SubscriptionFull
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
