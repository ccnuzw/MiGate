package db

import "strings"

type RoutingRule struct {
	ID          int64  `json:"id"`
	InboundID   int64  `json:"inbound_id,omitempty"`
	InboundTag  string `json:"inbound_tag"`
	ClientID    int64  `json:"client_id,omitempty"`
	ClientEmail string `json:"client_email,omitempty"`
	OutboundID  int64  `json:"outbound_id,omitempty"`
	OutboundTag string `json:"outbound_tag"`
	Domain      string `json:"domain"`
	IP          string `json:"ip"`
	RuleSet     string `json:"rule_set"`
	Protocol    string `json:"protocol"`
	Enabled     bool   `json:"enabled"`
	Sort        int    `json:"sort"`
}

type CreateRoutingRuleParams struct {
	InboundID   int64  `json:"inbound_id,omitempty"`
	InboundTag  string `json:"inbound_tag"`
	ClientID    int64  `json:"client_id,omitempty"`
	ClientEmail string `json:"client_email,omitempty"`
	OutboundID  int64  `json:"outbound_id,omitempty"`
	OutboundTag string `json:"outbound_tag"`
	Domain      string `json:"domain"`
	IP          string `json:"ip"`
	RuleSet     string `json:"rule_set"`
	Protocol    string `json:"protocol"`
	Enabled     bool   `json:"enabled"`
}

type UpdateRoutingRuleParams struct {
	InboundID   int64  `json:"inbound_id,omitempty"`
	InboundTag  string `json:"inbound_tag"`
	ClientID    int64  `json:"client_id,omitempty"`
	ClientEmail string `json:"client_email,omitempty"`
	OutboundID  int64  `json:"outbound_id,omitempty"`
	OutboundTag string `json:"outbound_tag"`
	Domain      string `json:"domain"`
	IP          string `json:"ip"`
	RuleSet     string `json:"rule_set"`
	Protocol    string `json:"protocol"`
	Enabled     bool   `json:"enabled"`
}

type Inbound struct {
	ID                    int64    `json:"id"`
	UUID                  string   `json:"uuid"`
	Remark                string   `json:"remark"`
	Protocol              string   `json:"protocol"`
	Core                  string   `json:"core"`
	Port                  int      `json:"port"`
	Network               string   `json:"network"`
	Security              string   `json:"security"`
	Enabled               bool     `json:"enabled"`
	WsPath                string   `json:"ws_path"`
	WsHost                string   `json:"ws_host"`
	GrpcServiceName       string   `json:"grpc_service_name"`
	RealityDest           string   `json:"reality_dest"`
	RealityServerNames    string   `json:"reality_server_names"`
	RealityShortID        string   `json:"reality_short_id"`
	RealityPrivateKey     string   `json:"reality_private_key"`
	RealityPublicKey      string   `json:"reality_public_key"`
	SSMethod              string   `json:"ss_method"`
	TLSCertFile           string   `json:"tls_cert_file"`
	TLSKeyFile            string   `json:"tls_key_file"`
	TLSSNI                string   `json:"tls_sni"`
	TLSFingerprint        string   `json:"tls_fingerprint"`
	TLSALPN               string   `json:"tls_alpn"`
	XHTTPPath             string   `json:"xhttp_path"`
	XHTTPMode             string   `json:"xhttp_mode"`
	Hy2UpMbps             int      `json:"hy2_up_mbps"`
	Hy2DownMbps           int      `json:"hy2_down_mbps"`
	Hy2Obfs               string   `json:"hy2_obfs"`
	Hy2ObfsPassword       string   `json:"hy2_obfs_password"`
	Hy2MPort              string   `json:"hy2_mport"`
	TuicCongestionControl string   `json:"tuic_congestion_control"`
	TuicZeroRTT           bool     `json:"tuic_zero_rtt"`
	WgPrivateKey          string   `json:"wg_private_key"`
	WgAddress             string   `json:"wg_address"`
	WgPeerPublicKey       string   `json:"wg_peer_public_key"`
	WgAllowedIPs          string   `json:"wg_allowed_ips"`
	WgEndpoint            string   `json:"wg_endpoint"`
	WgPresharedKey        string   `json:"wg_preshared_key"`
	WgMTU                 int      `json:"wg_mtu"`
	ShadowTLSVersion      int      `json:"shadowtls_version"`
	ShadowTLSPassword     string   `json:"shadowtls_password"`
	Clients               []Client `json:"clients"`
}

type Outbound struct {
	ID             int64    `json:"id"`
	Tag            string   `json:"tag"`
	Remark         string   `json:"remark"`
	Protocol       string   `json:"protocol"`
	Address        string   `json:"address"`
	Port           int      `json:"port"`
	Username       string   `json:"username"`
	Password       string   `json:"password"`
	SupportedCores []string `json:"supported_cores"`
	Enabled        bool     `json:"enabled"`
	Sort           int      `json:"sort"`
}

type CreateOutboundParams struct {
	Tag      string `json:"tag"`
	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type UpdateOutboundParams struct {
	Tag      string `json:"tag"`
	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  bool   `json:"enabled"`
}

type Client struct {
	ID                int64  `json:"id"`
	InboundID         int64  `json:"inbound_id"`
	UUID              string `json:"uuid"`
	CredentialID      string `json:"credential_id,omitempty"`
	Password          string `json:"password,omitempty"`
	SubscriptionToken string `json:"subscription_token,omitempty"`
	StatsKey          string `json:"stats_key,omitempty"`
	Email             string `json:"email"`
	Enabled           bool   `json:"enabled"`
	Up                int64  `json:"up"`
	Down              int64  `json:"down"`
	TrafficLimit      int64  `json:"traffic_limit"`
	ExpiryAt          int64  `json:"expiry_at"`
}

type ClientTrafficUpdate struct {
	Up   int64
	Down int64
}

func (c Client) CredentialIDValue() string {
	if strings.TrimSpace(c.CredentialID) != "" {
		return strings.TrimSpace(c.CredentialID)
	}
	return strings.TrimSpace(c.UUID)
}

func (c Client) PasswordValue() string {
	if strings.TrimSpace(c.Password) != "" {
		return c.Password
	}
	return c.UUID
}

type TrafficRawStat struct {
	Engine    string
	ScopeType string
	ScopeKey  string
	RawUp     int64
	RawDown   int64
	Status    string
	Message   string
}

type TrafficStatusMarker struct {
	Engine    string
	ScopeType string
	ScopeKey  string
	Status    string
	Message   string
}

type TrafficState struct {
	Engine      string  `json:"engine"`
	ScopeType   string  `json:"scope_type"`
	ScopeKey    string  `json:"scope_key"`
	TotalUp     int64   `json:"total_up"`
	TotalDown   int64   `json:"total_down"`
	LastRawUp   int64   `json:"last_raw_up"`
	LastRawDown int64   `json:"last_raw_down"`
	RateUp      float64 `json:"rate_up"`
	RateDown    float64 `json:"rate_down"`
	LastSeenAt  string  `json:"last_seen_at"`
	Status      string  `json:"status"`
	Message     string  `json:"message,omitempty"`
}

type TrafficSample struct {
	SampledAt string  `json:"sampled_at"`
	Engine    string  `json:"engine"`
	ScopeType string  `json:"scope_type"`
	ScopeKey  string  `json:"scope_key"`
	TotalUp   int64   `json:"total_up"`
	TotalDown int64   `json:"total_down"`
	RateUp    float64 `json:"rate_up"`
	RateDown  float64 `json:"rate_down"`
	Status    string  `json:"status"`
}

type ClientTrafficUsage struct {
	ClientID   int64   `json:"client_id"`
	StatsKey   string  `json:"stats_key"`
	Engine     string  `json:"engine"`
	TotalUp    int64   `json:"total_up"`
	TotalDown  int64   `json:"total_down"`
	RateUp     float64 `json:"rate_up"`
	RateDown   float64 `json:"rate_down"`
	Status     string  `json:"status"`
	Message    string  `json:"message,omitempty"`
	LastSeenAt string  `json:"last_seen_at,omitempty"`
}

type CreateInboundParams struct {
	UUID                  string              `json:"uuid,omitempty"`
	Remark                string              `json:"remark"`
	Protocol              string              `json:"protocol"`
	Port                  int                 `json:"port"`
	Network               string              `json:"network"`
	Security              string              `json:"security"`
	WsPath                string              `json:"ws_path"`
	WsHost                string              `json:"ws_host"`
	GrpcServiceName       string              `json:"grpc_service_name"`
	RealityDest           string              `json:"reality_dest"`
	RealityServerNames    string              `json:"reality_server_names"`
	RealityShortID        string              `json:"reality_short_id"`
	RealityPrivateKey     string              `json:"reality_private_key"`
	RealityPublicKey      string              `json:"reality_public_key"`
	SSMethod              string              `json:"ss_method"`
	TLSCertFile           string              `json:"tls_cert_file"`
	TLSKeyFile            string              `json:"tls_key_file"`
	TLSSNI                string              `json:"tls_sni"`
	TLSFingerprint        string              `json:"tls_fingerprint"`
	TLSALPN               string              `json:"tls_alpn"`
	XHTTPPath             string              `json:"xhttp_path"`
	XHTTPMode             string              `json:"xhttp_mode"`
	Hy2UpMbps             int                 `json:"hy2_up_mbps"`
	Hy2DownMbps           int                 `json:"hy2_down_mbps"`
	Hy2Obfs               string              `json:"hy2_obfs"`
	Hy2ObfsPassword       string              `json:"hy2_obfs_password"`
	Hy2MPort              string              `json:"hy2_mport"`
	TuicCongestionControl string              `json:"tuic_congestion_control"`
	TuicZeroRTT           bool                `json:"tuic_zero_rtt"`
	WgPrivateKey          string              `json:"wg_private_key"`
	WgAddress             string              `json:"wg_address"`
	WgPeerPublicKey       string              `json:"wg_peer_public_key"`
	WgAllowedIPs          string              `json:"wg_allowed_ips"`
	WgEndpoint            string              `json:"wg_endpoint"`
	WgPresharedKey        string              `json:"wg_preshared_key"`
	WgMTU                 int                 `json:"wg_mtu"`
	ShadowTLSVersion      int                 `json:"shadowtls_version"`
	ShadowTLSPassword     string              `json:"shadowtls_password"`
	InitialClient         *CreateClientParams `json:"initial_client,omitempty"`
}

type CreateClientParams struct {
	InboundID    int64  `json:"inbound_id,omitempty"`
	UUID         string `json:"uuid,omitempty"`
	CredentialID string `json:"credential_id,omitempty"`
	Password     string `json:"password,omitempty"`
	Email        string `json:"email"`
	Enabled      *bool  `json:"enabled,omitempty"`
	TrafficLimit int64  `json:"traffic_limit,omitempty"`
	ExpiryAt     int64  `json:"expiry_at,omitempty"`
}

type UpdateInboundParams struct {
	UUID                  string `json:"uuid"`
	Remark                string `json:"remark"`
	Protocol              string `json:"protocol"`
	Port                  int    `json:"port"`
	Network               string `json:"network"`
	Security              string `json:"security"`
	Enabled               bool   `json:"enabled"`
	WsPath                string `json:"ws_path"`
	WsHost                string `json:"ws_host"`
	GrpcServiceName       string `json:"grpc_service_name"`
	RealityDest           string `json:"reality_dest"`
	RealityServerNames    string `json:"reality_server_names"`
	RealityShortID        string `json:"reality_short_id"`
	RealityPrivateKey     string `json:"reality_private_key"`
	RealityPublicKey      string `json:"reality_public_key"`
	SSMethod              string `json:"ss_method"`
	TLSCertFile           string `json:"tls_cert_file"`
	TLSKeyFile            string `json:"tls_key_file"`
	TLSSNI                string `json:"tls_sni"`
	TLSFingerprint        string `json:"tls_fingerprint"`
	TLSALPN               string `json:"tls_alpn"`
	XHTTPPath             string `json:"xhttp_path"`
	XHTTPMode             string `json:"xhttp_mode"`
	Hy2UpMbps             int    `json:"hy2_up_mbps"`
	Hy2DownMbps           int    `json:"hy2_down_mbps"`
	Hy2Obfs               string `json:"hy2_obfs"`
	Hy2ObfsPassword       string `json:"hy2_obfs_password"`
	Hy2MPort              string `json:"hy2_mport"`
	TuicCongestionControl string `json:"tuic_congestion_control"`
	TuicZeroRTT           bool   `json:"tuic_zero_rtt"`
	WgPrivateKey          string `json:"wg_private_key"`
	WgAddress             string `json:"wg_address"`
	WgPeerPublicKey       string `json:"wg_peer_public_key"`
	WgAllowedIPs          string `json:"wg_allowed_ips"`
	WgEndpoint            string `json:"wg_endpoint"`
	WgPresharedKey        string `json:"wg_preshared_key"`
	WgMTU                 int    `json:"wg_mtu"`
	ShadowTLSVersion      int    `json:"shadowtls_version"`
	ShadowTLSPassword     string `json:"shadowtls_password"`
}

type UpdateClientParams struct {
	UUID         string `json:"uuid,omitempty"`
	CredentialID string `json:"credential_id,omitempty"`
	Password     string `json:"password,omitempty"`
	Email        string `json:"email"`
	Enabled      bool   `json:"enabled"`
	TrafficLimit int64  `json:"traffic_limit"`
	ExpiryAt     int64  `json:"expiry_at"`
}
