package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"
	_ "modernc.org/sqlite"
)

const (
	autoInboundPortMin = 20000
	autoInboundPortMax = 60999
)

var supportedOutboundProtocols = map[string]bool{
	"freedom":     true,
	"blackhole":   true,
	"dns":         true,
	"socks":       true,
	"http":        true,
	"https":       true,
	"vless":       true,
	"trojan":      true,
	"shadowsocks": true,
	"hysteria2":   true,
	"tuic":        true,
	"shadowtls":   true,
}

var tableIdentifierForMigration = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const sqliteVariableChunkSize = 900

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

type RoutingRule struct {
	ID          int64  `json:"id"`
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

type Store struct {
	db                        *sql.DB
	trafficCleanupMu          sync.Mutex
	nextTrafficSamplesCleanup time.Time
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

func Open(ctx context.Context, path string) (*Store, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: database}
	if err := store.migrate(ctx); err != nil {
		database.Close()
		return nil, err
	}
	// Enable WAL mode for better concurrent read/write performance
	_, _ = database.ExecContext(ctx, `PRAGMA journal_mode=WAL`)
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

type BlacklistedSession struct {
	ID        int64  `json:"id"`
	TokenHash string `json:"token_hash"`
	CreatedAt string `json:"created_at"`
	LastUsed  string `json:"last_used"`
	ExpiresAt string `json:"expires_at"`
	Revoked   bool   `json:"revoked"`
}

var sessionMaxAge = 7 * 24 * time.Hour // 168 hours

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS inbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uuid TEXT NOT NULL UNIQUE,
  remark TEXT NOT NULL,
  protocol TEXT NOT NULL,
  core TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL,
  network TEXT NOT NULL,
  security TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS clients (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  inbound_id INTEGER NOT NULL REFERENCES inbounds(id) ON DELETE CASCADE,
  uuid TEXT NOT NULL UNIQUE,
  credential_id TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  subscription_token TEXT NOT NULL DEFAULT '',
  stats_key TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_clients_inbound_id ON clients(inbound_id);
CREATE TABLE IF NOT EXISTS outbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tag TEXT NOT NULL UNIQUE,
  remark TEXT NOT NULL,
  protocol TEXT NOT NULL,
  address TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 0,
  username TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS routing_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  inbound_tag TEXT NOT NULL DEFAULT '',
  client_id INTEGER NOT NULL DEFAULT 0,
  client_email TEXT NOT NULL DEFAULT '',
  outbound_id INTEGER NOT NULL DEFAULT 0,
  outbound_tag TEXT NOT NULL,
  domain TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  rule_set TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS token_blacklist (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  last_used TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS traffic_states (
  engine TEXT NOT NULL,
  scope_type TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  total_up INTEGER NOT NULL DEFAULT 0,
  total_down INTEGER NOT NULL DEFAULT 0,
  last_raw_up INTEGER NOT NULL DEFAULT 0,
  last_raw_down INTEGER NOT NULL DEFAULT 0,
  rate_up REAL NOT NULL DEFAULT 0,
  rate_down REAL NOT NULL DEFAULT 0,
  last_seen_at TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'waiting',
  message TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (engine, scope_type, scope_key)
);
CREATE TABLE IF NOT EXISTS traffic_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sampled_at TEXT NOT NULL,
  engine TEXT NOT NULL,
  scope_type TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  total_up INTEGER NOT NULL DEFAULT 0,
  total_down INTEGER NOT NULL DEFAULT 0,
  rate_up REAL NOT NULL DEFAULT 0,
  rate_down REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'waiting'
);
CREATE INDEX IF NOT EXISTS idx_outbounds_sort_id ON outbounds(sort, id);
CREATE INDEX IF NOT EXISTS idx_routing_rules_sort_id ON routing_rules(sort, id);
CREATE INDEX IF NOT EXISTS idx_clients_email ON clients(email);
CREATE INDEX IF NOT EXISTS idx_clients_inbound_email ON clients(inbound_id, email);
CREATE INDEX IF NOT EXISTS idx_token_blacklist_expires_at ON token_blacklist(expires_at);
CREATE INDEX IF NOT EXISTS idx_token_blacklist_active ON token_blacklist(revoked, expires_at, id);
CREATE INDEX IF NOT EXISTS idx_traffic_states_scope ON traffic_states(scope_type, scope_key);
CREATE INDEX IF NOT EXISTS idx_traffic_samples_lookup ON traffic_samples(scope_type, scope_key, sampled_at);
CREATE INDEX IF NOT EXISTS idx_traffic_samples_sampled_at ON traffic_samples(sampled_at);
`)
	if err != nil {
		return err
	}
	if err := s.seedDefaultOutbounds(ctx); err != nil {
		return err
	}
	if err := s.ensureUniqueInboundPortIndex(ctx); err != nil {
		return err
	}
	// Migration: add traffic/expiry columns (ignore errors if already exist)
	for _, col := range []struct{ name, typ string }{
		{"credential_id", "TEXT NOT NULL DEFAULT ''"},
		{"password", "TEXT NOT NULL DEFAULT ''"},
		{"subscription_token", "TEXT NOT NULL DEFAULT ''"},
		{"stats_key", "TEXT NOT NULL DEFAULT ''"},
		{"up", "INTEGER NOT NULL DEFAULT 0"},
		{"down", "INTEGER NOT NULL DEFAULT 0"},
		{"traffic_limit", "INTEGER NOT NULL DEFAULT 0"},
		{"expiry_at", "INTEGER NOT NULL DEFAULT 0"},
	} {
		_, _ = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE clients ADD COLUMN %s %s", col.name, col.typ))
	}
	if err := s.ensureClientSubscriptionTokens(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE clients SET credential_id = uuid WHERE credential_id = '' OR credential_id IS NULL`); err != nil {
		return err
	}
	if err := s.ensureUniqueCredentialIDIndex(ctx); err != nil {
		return err
	}
	if err := s.ensureClientStatsKeys(ctx); err != nil {
		return err
	}
	// Migration: add transport columns to inbounds (ignore errors if already exist)
	for _, col := range []struct{ name, typ, def string }{
		{"core", "TEXT", "DEFAULT ''"},
		{"ws_path", "TEXT", "DEFAULT ''"},
		{"ws_host", "TEXT", "DEFAULT ''"},
		{"grpc_service_name", "TEXT", "DEFAULT ''"},
		{"reality_dest", "TEXT", "DEFAULT ''"},
		{"reality_server_names", "TEXT", "DEFAULT ''"},
		{"reality_short_id", "TEXT", "DEFAULT ''"},
		{"reality_private_key", "TEXT", "DEFAULT ''"},
		{"reality_public_key", "TEXT", "DEFAULT ''"},
		{"ss_method", "TEXT", "DEFAULT '2022-blake3-aes-128-gcm'"},
		{"tls_cert_file", "TEXT", "DEFAULT ''"},
		{"tls_key_file", "TEXT", "DEFAULT ''"},
		{"xhttp_path", "TEXT", "DEFAULT ''"},
		{"xhttp_mode", "TEXT", "DEFAULT ''"},
		{"hy2_up_mbps", "INTEGER", "DEFAULT 0"},
		{"hy2_down_mbps", "INTEGER", "DEFAULT 0"},
		{"hy2_obfs", "TEXT", "DEFAULT ''"},
		{"hy2_obfs_password", "TEXT", "DEFAULT ''"},
		{"hy2_mport", "TEXT", "DEFAULT ''"},
		{"tls_sni", "TEXT", "DEFAULT ''"},
		{"tls_fingerprint", "TEXT", "DEFAULT ''"},
		{"tls_alpn", "TEXT", "DEFAULT ''"},
		{"tuic_congestion_control", "TEXT", "DEFAULT 'bbr'"},
		{"tuic_zero_rtt", "INTEGER", "DEFAULT 0"},
		{"wg_private_key", "TEXT", "DEFAULT ''"},
		{"wg_address", "TEXT", "DEFAULT ''"},
		{"wg_peer_public_key", "TEXT", "DEFAULT ''"},
		{"wg_allowed_ips", "TEXT", "DEFAULT '0.0.0.0/0, ::/0'"},
		{"wg_endpoint", "TEXT", "DEFAULT ''"},
		{"wg_preshared_key", "TEXT", "DEFAULT ''"},
		{"wg_mtu", "INTEGER", "DEFAULT 0"},
		{"shadowtls_version", "INTEGER", "DEFAULT 3"},
		{"shadowtls_password", "TEXT", "DEFAULT ''"},
	} {
		_, _ = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE inbounds ADD COLUMN %s %s %s", col.name, col.typ, col.def))
	}
	for _, col := range []struct{ name, typ string }{
		{"ip", "TEXT NOT NULL DEFAULT ''"},
		{"rule_set", "TEXT NOT NULL DEFAULT ''"},
		{"client_id", "INTEGER NOT NULL DEFAULT 0"},
		{"client_email", "TEXT NOT NULL DEFAULT ''"},
		{"outbound_id", "INTEGER NOT NULL DEFAULT 0"},
	} {
		_, _ = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE routing_rules ADD COLUMN %s %s", col.name, col.typ))
	}
	if err := s.backfillCoreFields(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureUniqueInboundPortIndex(ctx context.Context) error {
	return s.ensureUniqueIndex(ctx, "inbounds", "idx_inbounds_port", `CREATE UNIQUE INDEX IF NOT EXISTS idx_inbounds_port ON inbounds(port)`)
}

func (s *Store) ensureUniqueCredentialIDIndex(ctx context.Context) error {
	return s.ensureUniqueIndex(ctx, "clients", "idx_clients_credential_id", `CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_credential_id ON clients(credential_id) WHERE credential_id <> ''`)
}

func (s *Store) ensureUniqueIndex(ctx context.Context, table, indexName, createSQL string) error {
	if !tableIdentifierForMigration.MatchString(table) || !tableIdentifierForMigration.MatchString(indexName) {
		return fmt.Errorf("invalid index migration target: %s.%s", table, indexName)
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_list(%s)`, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return err
		}
		if name == indexName && unique == 0 {
			if err := rows.Close(); err != nil {
				return err
			}
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`DROP INDEX %s`, indexName)); err != nil {
				return err
			}
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, createSQL)
	return err
}

func (s *Store) backfillCoreFields(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, protocol FROM inbounds WHERE core = '' OR core IS NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var protocol string
		if err := rows.Scan(&id, &protocol); err != nil {
			rows.Close()
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE inbounds SET core=? WHERE id=?`, InferInboundCore(protocol), id); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
UPDATE routing_rules
SET outbound_id = COALESCE((SELECT id FROM outbounds WHERE outbounds.tag = routing_rules.outbound_tag), 0)
WHERE outbound_id = 0 AND outbound_tag <> ''
`)
	return err
}

func (s *Store) ensureClientSubscriptionTokens(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_subscription_token ON clients(subscription_token) WHERE subscription_token <> ''`); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM clients WHERE subscription_token = '' OR subscription_token IS NULL ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		token, err := s.newSubscriptionToken(ctx)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE clients SET subscription_token=? WHERE id=?`, token, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureClientStatsKeys(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_stats_key ON clients(stats_key) WHERE stats_key <> ''`); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM clients WHERE stats_key = '' OR stats_key IS NULL ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		key, err := s.newStatsKey(ctx)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE clients SET stats_key=? WHERE id=?`, key, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedDefaultOutbounds(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	defaults := []Outbound{
		{Tag: "direct", Remark: "直接连接", Protocol: "freedom", Enabled: true, Sort: 0},
		{Tag: "blocked", Remark: "阻断", Protocol: "blackhole", Enabled: true, Sort: 1},
		{Tag: "dns", Remark: "DNS", Protocol: "dns", Enabled: true, Sort: 2},
	}
	for _, outbound := range defaults {
		_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO outbounds (tag, remark, protocol, address, port, username, password, enabled, sort, created_at) VALUES (?, ?, ?, '', 0, '', '', 1, ?, ?)`,
			outbound.Tag, outbound.Remark, outbound.Protocol, outbound.Sort, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListOutbounds(ctx context.Context) ([]Outbound, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	outbounds := []Outbound{}
	for rows.Next() {
		var outbound Outbound
		var enabled int
		if err := rows.Scan(&outbound.ID, &outbound.Tag, &outbound.Remark, &outbound.Protocol, &outbound.Address, &outbound.Port, &outbound.Username, &outbound.Password, &enabled, &outbound.Sort); err != nil {
			return nil, err
		}
		outbound.SupportedCores = OutboundProtocolSupportedCores(outbound.Protocol)
		outbound.Enabled = enabled != 0
		outbounds = append(outbounds, outbound)
	}
	return outbounds, rows.Err()
}

func (s *Store) CreateOutbound(ctx context.Context, params CreateOutboundParams) (Outbound, error) {
	protocol := NormalizeOutboundProtocol(params.Protocol)
	if !supportedOutboundProtocols[protocol] {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	tag := strings.TrimSpace(params.Tag)
	if tag == "" {
		return Outbound{}, fmt.Errorf("tag cannot be empty")
	}
	remark := strings.TrimSpace(params.Remark)
	if remark == "" {
		remark = tag
	}
	address := strings.TrimSpace(params.Address)
	if outboundProtocolNeedsAddress(protocol) && address == "" {
		return Outbound{}, fmt.Errorf("address cannot be empty")
	}
	if outboundProtocolNeedsAddress(protocol) && (params.Port <= 0 || params.Port > 65535) {
		return Outbound{}, fmt.Errorf("invalid port: %d", params.Port)
	}
	supportedCores := OutboundProtocolSupportedCores(protocol)
	if len(supportedCores) == 0 {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	if err := ValidateOutboundProfile(Outbound{Protocol: protocol, Address: address, Port: params.Port, Username: params.Username, Password: params.Password}); err != nil {
		return Outbound{}, err
	}
	var sort int
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort)+1, 0) FROM outbounds`).Scan(&sort)
	result, err := s.db.ExecContext(ctx, `INSERT INTO outbounds (tag, remark, protocol, address, port, username, password, enabled, sort, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		tag, remark, protocol, address, params.Port, strings.TrimSpace(params.Username), params.Password, sort, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return Outbound{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Outbound{}, err
	}
	return Outbound{ID: id, Tag: tag, Remark: remark, Protocol: protocol, Address: address, Port: params.Port, Username: strings.TrimSpace(params.Username), Password: params.Password, SupportedCores: supportedCores, Enabled: true, Sort: sort}, nil
}

func (s *Store) UpdateOutbound(ctx context.Context, id int64, params UpdateOutboundParams) (Outbound, error) {
	protocol := NormalizeOutboundProtocol(params.Protocol)
	if !supportedOutboundProtocols[protocol] {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	tag := strings.TrimSpace(params.Tag)
	if tag == "" {
		return Outbound{}, fmt.Errorf("tag cannot be empty")
	}
	remark := strings.TrimSpace(params.Remark)
	if remark == "" {
		remark = tag
	}
	address := strings.TrimSpace(params.Address)
	if outboundProtocolNeedsAddress(protocol) && address == "" {
		return Outbound{}, fmt.Errorf("address cannot be empty")
	}
	if outboundProtocolNeedsAddress(protocol) && (params.Port <= 0 || params.Port > 65535) {
		return Outbound{}, fmt.Errorf("invalid port: %d", params.Port)
	}
	supportedCores := OutboundProtocolSupportedCores(protocol)
	if len(supportedCores) == 0 {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	if err := ValidateOutboundProfile(Outbound{Protocol: protocol, Address: address, Port: params.Port, Username: params.Username, Password: params.Password}); err != nil {
		return Outbound{}, err
	}
	enabled := 0
	if params.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE outbounds SET tag=?, remark=?, protocol=?, address=?, port=?, username=?, password=?, enabled=? WHERE id=?`,
		tag, remark, protocol, address, params.Port, strings.TrimSpace(params.Username), params.Password, enabled, id)
	if err != nil {
		return Outbound{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Outbound{}, err
	}
	if n == 0 {
		return Outbound{}, fmt.Errorf("outbound not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds WHERE id=?`, id)
	var outbound Outbound
	var dbEnabled int
	if err := row.Scan(&outbound.ID, &outbound.Tag, &outbound.Remark, &outbound.Protocol, &outbound.Address, &outbound.Port, &outbound.Username, &outbound.Password, &dbEnabled, &outbound.Sort); err != nil {
		return Outbound{}, err
	}
	outbound.Enabled = dbEnabled != 0
	outbound.SupportedCores = OutboundProtocolSupportedCores(outbound.Protocol)
	return outbound, nil
}

func (s *Store) DeleteOutbound(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM outbounds WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("outbound not found: %d", id)
	}
	return nil
}

func (s *Store) ReorderOutbounds(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Collect IDs of editable (non-default) outbounds already in DB
	rows, err := tx.QueryContext(ctx, `SELECT id FROM outbounds WHERE protocol NOT IN ('freedom','blackhole','dns') ORDER BY sort ASC`)
	if err != nil {
		return err
	}
	var existing []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing = append(existing, id)
	}
	rows.Close()

	if len(ids) != len(existing) {
		return fmt.Errorf("expected %d IDs for reordering, got %d", len(existing), len(ids))
	}

	// Find defaults count
	var defaultCount int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbounds WHERE protocol IN ('freedom','blackhole','dns')`).Scan(&defaultCount); err != nil {
		return err
	}

	for i, id := range ids {
		_, err := tx.ExecContext(ctx, `UPDATE outbounds SET sort = ? WHERE id = ?`, int(defaultCount)+i, id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListRoutingRules(ctx context.Context) ([]RoutingRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, inbound_tag, client_id, client_email, outbound_id, outbound_tag, domain, ip, rule_set, protocol, enabled, sort FROM routing_rules ORDER BY sort ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules = make([]RoutingRule, 0)
	for rows.Next() {
		var r RoutingRule
		var dbEnabled int
		if err := rows.Scan(&r.ID, &r.InboundTag, &r.ClientID, &r.ClientEmail, &r.OutboundID, &r.OutboundTag, &r.Domain, &r.IP, &r.RuleSet, &r.Protocol, &dbEnabled, &r.Sort); err != nil {
			return nil, err
		}
		r.Enabled = dbEnabled != 0
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func isVirtualOutboundTag(tag string) bool {
	return false
}

func outboundProtocolNeedsAddress(protocol string) bool {
	switch protocol {
	case "socks", "http", "https", "vless", "trojan", "shadowsocks", "hysteria2", "tuic", "shadowtls":
		return true
	default:
		return false
	}
}

func (s *Store) CreateRoutingRule(ctx context.Context, params CreateRoutingRuleParams) (RoutingRule, error) {
	clientID, clientEmail, inboundTag, err := s.resolveRoutingRuleClient(ctx, params.ClientID, params.ClientEmail, params.InboundTag)
	if err != nil {
		return RoutingRule{}, err
	}
	outbound, err := s.resolveRoutingOutbound(ctx, params.OutboundID, params.OutboundTag)
	if err != nil {
		return RoutingRule{}, err
	}
	if err := s.validateRoutingCoreCompatibility(ctx, inboundTag, clientID, outbound); err != nil {
		return RoutingRule{}, err
	}
	var sort int
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort)+1, 0) FROM routing_rules`).Scan(&sort)
	enabled := 0
	if params.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO routing_rules (inbound_tag, client_id, client_email, outbound_id, outbound_tag, domain, ip, rule_set, protocol, enabled, sort) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inboundTag, clientID, clientEmail, outbound.ID, outbound.Tag, strings.TrimSpace(params.Domain), strings.TrimSpace(params.IP), strings.TrimSpace(params.RuleSet), strings.TrimSpace(params.Protocol), enabled, sort)
	if err != nil {
		return RoutingRule{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return RoutingRule{}, err
	}
	return RoutingRule{ID: id, InboundTag: inboundTag, ClientID: clientID, ClientEmail: clientEmail, OutboundID: outbound.ID, OutboundTag: outbound.Tag, Domain: strings.TrimSpace(params.Domain), IP: strings.TrimSpace(params.IP), RuleSet: strings.TrimSpace(params.RuleSet), Protocol: strings.TrimSpace(params.Protocol), Enabled: params.Enabled, Sort: sort}, nil
}

func (s *Store) UpdateRoutingRule(ctx context.Context, id int64, params UpdateRoutingRuleParams) (RoutingRule, error) {
	clientID, clientEmail, inboundTag, err := s.resolveRoutingRuleClient(ctx, params.ClientID, params.ClientEmail, params.InboundTag)
	if err != nil {
		return RoutingRule{}, err
	}
	outbound, err := s.resolveRoutingOutbound(ctx, params.OutboundID, params.OutboundTag)
	if err != nil {
		return RoutingRule{}, err
	}
	if err := s.validateRoutingCoreCompatibility(ctx, inboundTag, clientID, outbound); err != nil {
		return RoutingRule{}, err
	}
	enabled := 0
	if params.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE routing_rules SET inbound_tag=?, client_id=?, client_email=?, outbound_id=?, outbound_tag=?, domain=?, ip=?, rule_set=?, protocol=?, enabled=? WHERE id=?`,
		inboundTag, clientID, clientEmail, outbound.ID, outbound.Tag, strings.TrimSpace(params.Domain), strings.TrimSpace(params.IP), strings.TrimSpace(params.RuleSet), strings.TrimSpace(params.Protocol), enabled, id)
	if err != nil {
		return RoutingRule{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return RoutingRule{}, err
	}
	if n == 0 {
		return RoutingRule{}, fmt.Errorf("routing rule not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, inbound_tag, client_id, client_email, outbound_id, outbound_tag, domain, ip, rule_set, protocol, enabled, sort FROM routing_rules WHERE id=?`, id)
	var r RoutingRule
	var dbEnabled int
	if err := row.Scan(&r.ID, &r.InboundTag, &r.ClientID, &r.ClientEmail, &r.OutboundID, &r.OutboundTag, &r.Domain, &r.IP, &r.RuleSet, &r.Protocol, &dbEnabled, &r.Sort); err != nil {
		return RoutingRule{}, err
	}
	r.Enabled = dbEnabled != 0
	return r, nil
}

func (s *Store) resolveRoutingRuleClient(ctx context.Context, clientID int64, clientEmail, inboundTag string) (int64, string, string, error) {
	clientEmail = strings.TrimSpace(clientEmail)
	inboundTag = strings.TrimSpace(inboundTag)
	if clientID <= 0 {
		if clientEmail != "" {
			return 0, "", "", fmt.Errorf("client_id is required when client_email is set")
		}
		return 0, "", inboundTag, nil
	}
	var inboundID int64
	var protocol string
	var remark string
	var email string
	if err := s.db.QueryRowContext(ctx, `
SELECT c.inbound_id, c.email, i.protocol, i.remark
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.id = ?
`, clientID).Scan(&inboundID, &email, &protocol, &remark); err != nil {
		if err == sql.ErrNoRows {
			return 0, "", "", fmt.Errorf("client not found: %d", clientID)
		}
		return 0, "", "", err
	}
	actualTag := fmt.Sprintf("inbound-%d-%s", inboundID, strings.ToLower(strings.TrimSpace(protocol)))
	remark = strings.TrimSpace(remark)
	if inboundTag != "" && inboundTag != actualTag && inboundTag != remark {
		return 0, "", "", fmt.Errorf("client %d does not belong to inbound_tag %q", clientID, inboundTag)
	}
	return clientID, strings.TrimSpace(email), actualTag, nil
}

func (s *Store) resolveRoutingOutbound(ctx context.Context, outboundID int64, outboundTag string) (Outbound, error) {
	outboundTag = strings.TrimSpace(outboundTag)
	var row *sql.Row
	if outboundID > 0 {
		row = s.db.QueryRowContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds WHERE id=?`, outboundID)
	} else {
		if outboundTag == "" {
			return Outbound{}, fmt.Errorf("outbound_id or outbound_tag cannot be empty")
		}
		if isVirtualOutboundTag(outboundTag) {
			return Outbound{}, fmt.Errorf("virtual outbound tags are not supported")
		}
		row = s.db.QueryRowContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds WHERE tag=?`, outboundTag)
	}
	var outbound Outbound
	var enabled int
	if err := row.Scan(&outbound.ID, &outbound.Tag, &outbound.Remark, &outbound.Protocol, &outbound.Address, &outbound.Port, &outbound.Username, &outbound.Password, &enabled, &outbound.Sort); err != nil {
		if err == sql.ErrNoRows {
			return Outbound{}, fmt.Errorf("outbound not found")
		}
		return Outbound{}, err
	}
	outbound.Enabled = enabled != 0
	outbound.SupportedCores = OutboundProtocolSupportedCores(outbound.Protocol)
	return outbound, nil
}

func (s *Store) validateRoutingCoreCompatibility(ctx context.Context, inboundTag string, clientID int64, outbound Outbound) error {
	cores, err := s.routingRuleCores(ctx, inboundTag, clientID)
	if err != nil {
		return err
	}
	for _, core := range cores {
		if !OutboundSupportsCore(outbound, core) {
			return fmt.Errorf("outbound %q does not support %s", outbound.Tag, core)
		}
	}
	return nil
}

func (s *Store) routingRuleCores(ctx context.Context, inboundTag string, clientID int64) ([]string, error) {
	if clientID > 0 {
		var protocol string
		var core string
		if err := s.db.QueryRowContext(ctx, `
SELECT i.protocol, i.core
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.id = ?
`, clientID).Scan(&protocol, &core); err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("client not found: %d", clientID)
			}
			return nil, err
		}
		if strings.TrimSpace(core) == "" {
			core = InferInboundCore(protocol)
		}
		return []string{NormalizeCore(core)}, nil
	}
	inboundTag = strings.TrimSpace(inboundTag)
	if inboundTag != "" {
		var protocol string
		var core string
		err := s.db.QueryRowContext(ctx, `
SELECT protocol, core FROM inbounds
WHERE ? = ('inbound-' || id || '-' || lower(protocol)) OR TRIM(remark) = ?
ORDER BY id ASC
LIMIT 1
`, inboundTag, inboundTag).Scan(&protocol, &core)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("inbound_tag %q does not match any existing inbound", inboundTag)
			}
			return nil, err
		}
		if strings.TrimSpace(core) == "" {
			core = InferInboundCore(protocol)
		}
		return []string{NormalizeCore(core)}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT protocol, core FROM inbounds ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	cores := []string{}
	for rows.Next() {
		var protocol string
		var core string
		if err := rows.Scan(&protocol, &core); err != nil {
			return nil, err
		}
		if strings.TrimSpace(core) == "" {
			core = InferInboundCore(protocol)
		}
		core = NormalizeCore(core)
		if !seen[core] {
			seen[core] = true
			cores = append(cores, core)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cores) == 0 {
		return []string{CoreXray}, nil
	}
	return cores, nil
}

func (s *Store) DeleteRoutingRule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM routing_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("routing rule not found: %d", id)
	}
	return nil
}

func (s *Store) ReorderRoutingRules(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		_, err := tx.ExecContext(ctx, `UPDATE routing_rules SET sort = ? WHERE id = ?`, i, id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CreateInbound(ctx context.Context, params CreateInboundParams) (Inbound, error) {
	protocol := NormalizeInboundProtocol(params.Protocol)
	if !SupportedInboundProtocol(protocol) {
		return Inbound{}, fmt.Errorf("unsupported protocol: %s", params.Protocol)
	}
	core := InferInboundCore(protocol)
	port := params.Port
	if port == 0 {
		allocated, err := s.allocateInboundPort(ctx, 0)
		if err != nil {
			return Inbound{}, err
		}
		port = allocated
	}
	if port <= 0 || port > 65535 {
		return Inbound{}, fmt.Errorf("invalid port: %d", params.Port)
	}
	network := NormalizeInboundNetwork(protocol, params.Network)
	security := NormalizeInboundSecurity(protocol, params.Security)
	if err := prepareCreateInboundRealityMaterial(security, &params); err != nil {
		return Inbound{}, err
	}
	remark := strings.TrimSpace(params.Remark)
	if remark == "" {
		remark = protocol
	}
	candidate := Inbound{Remark: remark, Protocol: protocol, Core: core, Port: port, Network: network, Security: security,
		RealityDest: params.RealityDest, RealityServerNames: params.RealityServerNames, RealityPrivateKey: params.RealityPrivateKey, RealityPublicKey: params.RealityPublicKey,
		ShadowTLSVersion: params.ShadowTLSVersion, TLSSNI: params.TLSSNI}
	if err := ValidateInboundCombination(candidate); err != nil {
		return Inbound{}, err
	}
	var preparedClient *Client
	if params.InitialClient != nil {
		initialClient := *params.InitialClient
		initialClient.InboundID = 0
		client, err := s.prepareClientForCreate(ctx, Inbound{Protocol: protocol}, initialClient)
		if err != nil {
			return Inbound{}, err
		}
		preparedClient = &client
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Inbound{}, err
	}
	defer tx.Rollback()
	id, uuid, err := s.insertInboundTx(ctx, tx, params.UUID, remark, protocol, core, port, network, security,
		params.WsPath, params.WsHost, params.GrpcServiceName,
		params.RealityDest, params.RealityServerNames, params.RealityShortID, params.RealityPrivateKey, params.RealityPublicKey,
		params.SSMethod, params.TLSCertFile, params.TLSKeyFile, params.TLSSNI, params.TLSFingerprint, params.TLSALPN, params.XHTTPPath, params.XHTTPMode,
		params.Hy2UpMbps, params.Hy2DownMbps, params.Hy2Obfs, params.Hy2ObfsPassword, params.Hy2MPort,
		params.TuicCongestionControl, params.TuicZeroRTT,
		params.WgPrivateKey, params.WgAddress, params.WgPeerPublicKey, params.WgAllowedIPs, params.WgEndpoint, params.WgPresharedKey, params.WgMTU,
		params.ShadowTLSVersion, params.ShadowTLSPassword)
	if err != nil {
		return Inbound{}, err
	}
	var clients []Client
	if preparedClient != nil {
		preparedClient.InboundID = id
		createdClient, err := s.insertClientTx(ctx, tx, *preparedClient)
		if err != nil {
			return Inbound{}, err
		}
		clients = []Client{createdClient}
	}
	if err := tx.Commit(); err != nil {
		return Inbound{}, err
	}
	return Inbound{ID: id, UUID: uuid, Remark: remark, Protocol: protocol, Core: core, Port: port, Network: network, Security: security, Enabled: true,
		WsPath: params.WsPath, WsHost: params.WsHost, GrpcServiceName: params.GrpcServiceName,
		RealityDest: params.RealityDest, RealityServerNames: params.RealityServerNames, RealityShortID: params.RealityShortID,
		RealityPrivateKey: params.RealityPrivateKey,
		RealityPublicKey:  params.RealityPublicKey,
		SSMethod:          params.SSMethod,
		TLSCertFile:       params.TLSCertFile, TLSKeyFile: params.TLSKeyFile,
		TLSSNI: params.TLSSNI, TLSFingerprint: params.TLSFingerprint, TLSALPN: params.TLSALPN,
		XHTTPPath: params.XHTTPPath, XHTTPMode: params.XHTTPMode,
		Hy2UpMbps: params.Hy2UpMbps, Hy2DownMbps: params.Hy2DownMbps,
		Hy2Obfs: params.Hy2Obfs, Hy2ObfsPassword: params.Hy2ObfsPassword, Hy2MPort: params.Hy2MPort,
		TuicCongestionControl: params.TuicCongestionControl,
		TuicZeroRTT:           params.TuicZeroRTT,
		WgPrivateKey:          params.WgPrivateKey,
		WgAddress:             params.WgAddress,
		WgPeerPublicKey:       params.WgPeerPublicKey,
		WgAllowedIPs:          params.WgAllowedIPs,
		WgEndpoint:            params.WgEndpoint,
		WgPresharedKey:        params.WgPresharedKey,
		WgMTU:                 params.WgMTU,
		ShadowTLSVersion:      params.ShadowTLSVersion,
		ShadowTLSPassword:     params.ShadowTLSPassword,
		Clients:               clients}, nil
}

func (s *Store) insertInbound(ctx context.Context, inboundUUID, remark, protocol, core string, port int, network, security string,
	wsPath, wsHost, grpcServiceName, realityDest, realityServerNames, realityShortID, realityPrivateKey, realityPublicKey, ssMethod, tlsCertFile, tlsKeyFile, tlsSNI, tlsFingerprint, tlsALPN, xhttpPath, xhttpMode string,
	hy2UpMbps, hy2DownMbps int, hy2Obfs, hy2ObfsPassword, hy2MPort string,
	tuicCongestionControl string, tuicZeroRTT bool,
	wgPrivateKey, wgAddress, wgPeerPublicKey, wgAllowedIPs, wgEndpoint, wgPresharedKey string, wgMTU int,
	shadowTLSVersion int, shadowTLSPassword string) (int64, string, error) {
	return s.insertInboundTx(ctx, s.db, inboundUUID, remark, protocol, core, port, network, security,
		wsPath, wsHost, grpcServiceName, realityDest, realityServerNames, realityShortID, realityPrivateKey, realityPublicKey, ssMethod, tlsCertFile, tlsKeyFile, tlsSNI, tlsFingerprint, tlsALPN, xhttpPath, xhttpMode,
		hy2UpMbps, hy2DownMbps, hy2Obfs, hy2ObfsPassword, hy2MPort,
		tuicCongestionControl, tuicZeroRTT,
		wgPrivateKey, wgAddress, wgPeerPublicKey, wgAllowedIPs, wgEndpoint, wgPresharedKey, wgMTU,
		shadowTLSVersion, shadowTLSPassword)
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func (s *Store) insertInboundTx(ctx context.Context, execer sqlExecer, inboundUUID, remark, protocol, core string, port int, network, security string,
	wsPath, wsHost, grpcServiceName, realityDest, realityServerNames, realityShortID, realityPrivateKey, realityPublicKey, ssMethod, tlsCertFile, tlsKeyFile, tlsSNI, tlsFingerprint, tlsALPN, xhttpPath, xhttpMode string,
	hy2UpMbps, hy2DownMbps int, hy2Obfs, hy2ObfsPassword, hy2MPort string,
	tuicCongestionControl string, tuicZeroRTT bool,
	wgPrivateKey, wgAddress, wgPeerPublicKey, wgAllowedIPs, wgEndpoint, wgPresharedKey string, wgMTU int,
	shadowTLSVersion int, shadowTLSPassword string) (int64, string, error) {
	uuid := strings.TrimSpace(inboundUUID)
	if uuid == "" {
		uuid = newUUID()
	}
	tuicZeroRTTInt := 0
	if tuicZeroRTT {
		tuicZeroRTTInt = 1
	}
	result, err := execer.ExecContext(ctx, `
INSERT INTO inbounds (uuid, remark, protocol, core, port, network, security, enabled, created_at,
  ws_path, ws_host, grpc_service_name, reality_dest, reality_server_names, reality_short_id, reality_private_key, reality_public_key, ss_method, tls_cert_file, tls_key_file, tls_sni, tls_fingerprint, tls_alpn, xhttp_path, xhttp_mode,
  hy2_up_mbps, hy2_down_mbps, hy2_obfs, hy2_obfs_password, hy2_mport,
  tuic_congestion_control, tuic_zero_rtt,
  wg_private_key, wg_address, wg_peer_public_key, wg_allowed_ips, wg_endpoint, wg_preshared_key, wg_mtu,
  shadowtls_version, shadowtls_password)
VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?,
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
  ?, ?, ?, ?, ?,
  ?, ?,
  ?, ?, ?, ?, ?, ?, ?,
  ?, ?)`,
		uuid, remark, protocol, core, port, network, security, time.Now().UTC().Format(time.RFC3339),
		wsPath, wsHost, grpcServiceName, realityDest, realityServerNames, realityShortID, realityPrivateKey, realityPublicKey, ssMethod, tlsCertFile, tlsKeyFile, tlsSNI, tlsFingerprint, tlsALPN, xhttpPath, xhttpMode,
		hy2UpMbps, hy2DownMbps, hy2Obfs, hy2ObfsPassword, hy2MPort,
		tuicCongestionControl, tuicZeroRTTInt,
		wgPrivateKey, wgAddress, wgPeerPublicKey, wgAllowedIPs, wgEndpoint, wgPresharedKey, wgMTU,
		shadowTLSVersion, shadowTLSPassword)
	if err != nil {
		return 0, "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	return id, uuid, nil
}

func (s *Store) CreateClient(ctx context.Context, params CreateClientParams) (Client, error) {
	if params.InboundID <= 0 {
		return Client{}, fmt.Errorf("invalid inbound id: %d", params.InboundID)
	}
	inbound, err := s.getInboundBasic(ctx, params.InboundID)
	if err != nil {
		return Client{}, err
	}
	client, err := s.prepareClientForCreate(ctx, inbound, params)
	if err != nil {
		return Client{}, err
	}
	return s.insertClientTx(ctx, s.db, client)
}

func (s *Store) prepareClientForCreate(ctx context.Context, inbound Inbound, params CreateClientParams) (Client, error) {
	email := strings.TrimSpace(params.Email)
	if email == "" {
		email = "client"
	}
	uuid, credentialID, password := normalizeClientCredentials(inbound.Protocol, params.UUID, params.CredentialID, params.Password)
	clientForValidation := Client{UUID: uuid, CredentialID: credentialID, Password: password}
	if err := ValidateClientCredential(inbound.Protocol, clientForValidation); err != nil {
		return Client{}, err
	}
	var existingID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE inbound_id = ? AND email = ? LIMIT 1`, params.InboundID, email).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client email: %s", email)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE uuid = ? LIMIT 1`, uuid).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client uuid: %s", uuid)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if credentialID != "" {
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE credential_id = ? LIMIT 1`, credentialID).Scan(&existingID); err == nil {
			return Client{}, fmt.Errorf("duplicate client credential_id: %s", credentialID)
		} else if err != sql.ErrNoRows {
			return Client{}, err
		}
	}
	subscriptionToken, err := s.newSubscriptionToken(ctx)
	if err != nil {
		return Client{}, err
	}
	statsKey, err := s.newStatsKey(ctx)
	if err != nil {
		return Client{}, err
	}
	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}
	return Client{InboundID: params.InboundID, UUID: uuid, CredentialID: credentialID, Password: password, SubscriptionToken: subscriptionToken, StatsKey: statsKey, Email: email, Enabled: enabled, TrafficLimit: params.TrafficLimit, ExpiryAt: params.ExpiryAt}, nil
}

func (s *Store) insertClientTx(ctx context.Context, execer sqlExecer, client Client) (Client, error) {
	dbEnabled := 0
	if client.Enabled {
		dbEnabled = 1
	}
	result, err := execer.ExecContext(ctx, `
INSERT INTO clients (inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, created_at, traffic_limit, expiry_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, client.InboundID, client.UUID, client.CredentialID, client.Password, client.SubscriptionToken, client.StatsKey, client.Email, dbEnabled, time.Now().UTC().Format(time.RFC3339), client.TrafficLimit, client.ExpiryAt)
	if err != nil {
		return Client{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Client{}, err
	}
	client.ID = id
	return client, nil
}

func (s *Store) DeleteClient(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("client not found: %d", id)
	}
	return nil
}

func (s *Store) DeleteInbound(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM inbounds WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("inbound not found: %d", id)
	}
	return nil
}

func (s *Store) getInboundBasic(ctx context.Context, id int64) (Inbound, error) {
	var inbound Inbound
	if err := s.db.QueryRowContext(ctx, `SELECT id, uuid, remark, protocol, core, port, network, security, enabled FROM inbounds WHERE id=?`, id).Scan(
		&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, new(int),
	); err != nil {
		if err == sql.ErrNoRows {
			return Inbound{}, fmt.Errorf("inbound not found: %d", id)
		}
		return Inbound{}, err
	}
	if inbound.Core == "" {
		inbound.Core = InferInboundCore(inbound.Protocol)
	}
	return inbound, nil
}

func normalizeClientCredentials(protocol, uuid, credentialID, password string) (string, string, string) {
	protocol = NormalizeInboundProtocol(protocol)
	uuid = strings.TrimSpace(uuid)
	credentialID = strings.TrimSpace(credentialID)
	password = strings.TrimSpace(password)
	if credentialID == "" {
		credentialID = uuid
	}
	capability, _ := GetInboundCapability(protocol)
	switch capability.CredentialType {
	case CredentialUUID:
		if credentialID == "" {
			credentialID = newUUID()
		}
		return credentialID, credentialID, ""
	case CredentialPassword:
		if password == "" {
			password = uuid
		}
		if password == "" {
			password = newSecret(24)
		}
		if credentialID == "" {
			credentialID = newUUID()
		}
		return password, credentialID, password
	case CredentialIDPassword:
		if credentialID == "" {
			credentialID = newUUID()
		}
		if password == "" {
			password = newSecret(24)
		}
		return credentialID, credentialID, password
	case CredentialUsernamePassword:
		if credentialID == "" {
			credentialID = "user-" + newSecret(8)
		}
		if password == "" {
			password = newSecret(24)
		}
		return credentialID, credentialID, password
	case CredentialNone:
		if uuid == "" {
			uuid = newSecret(24)
		}
		return uuid, uuid, ""
	default:
		if uuid == "" {
			uuid = newUUID()
		}
		return uuid, uuid, password
	}
}

func prepareCreateInboundRealityMaterial(security string, params *CreateInboundParams) error {
	if strings.ToLower(strings.TrimSpace(security)) != "reality" {
		return nil
	}
	if strings.TrimSpace(params.RealityDest) == "" {
		params.RealityDest = "www.cloudflare.com:443"
	}
	if strings.TrimSpace(params.RealityServerNames) == "" {
		params.RealityServerNames = "www.cloudflare.com"
	}
	if strings.TrimSpace(params.RealityPrivateKey) == "" {
		privateKey, publicKey, err := generateRealityKeyPair()
		if err != nil {
			return err
		}
		params.RealityPrivateKey = privateKey
		params.RealityPublicKey = publicKey
	} else if strings.TrimSpace(params.RealityPublicKey) == "" {
		publicKey, err := deriveRealityPublicKey(params.RealityPrivateKey)
		if err == nil {
			params.RealityPublicKey = publicKey
		} else {
			privateKey, publicKey, err := generateRealityKeyPair()
			if err != nil {
				return err
			}
			params.RealityPrivateKey = privateKey
			params.RealityPublicKey = publicKey
		}
	}
	if strings.TrimSpace(params.RealityShortID) == "" {
		params.RealityShortID = newSecret(4)
	}
	return nil
}

func (s *Store) prepareUpdateInboundRealityMaterial(ctx context.Context, id int64, security string, params *UpdateInboundParams) error {
	if strings.ToLower(strings.TrimSpace(security)) != "reality" {
		return nil
	}
	if strings.TrimSpace(params.RealityDest) == "" {
		params.RealityDest = "www.cloudflare.com:443"
	}
	if strings.TrimSpace(params.RealityServerNames) == "" {
		params.RealityServerNames = "www.cloudflare.com"
	}
	var existingPrivateKey string
	var existingPublicKey string
	var existingShortID string
	_ = s.db.QueryRowContext(ctx, `SELECT reality_private_key, reality_public_key, reality_short_id FROM inbounds WHERE id=?`, id).Scan(&existingPrivateKey, &existingPublicKey, &existingShortID)
	if strings.TrimSpace(params.RealityPrivateKey) == "" {
		params.RealityPrivateKey = existingPrivateKey
	}
	if strings.TrimSpace(params.RealityPublicKey) == "" {
		params.RealityPublicKey = existingPublicKey
	}
	if strings.TrimSpace(params.RealityShortID) == "" {
		params.RealityShortID = existingShortID
	}
	if strings.TrimSpace(params.RealityPrivateKey) == "" {
		privateKey, publicKey, err := generateRealityKeyPair()
		if err != nil {
			return err
		}
		params.RealityPrivateKey = privateKey
		params.RealityPublicKey = publicKey
	} else if strings.TrimSpace(params.RealityPublicKey) == "" {
		publicKey, err := deriveRealityPublicKey(params.RealityPrivateKey)
		if err == nil {
			params.RealityPublicKey = publicKey
		} else {
			privateKey, publicKey, err := generateRealityKeyPair()
			if err != nil {
				return err
			}
			params.RealityPrivateKey = privateKey
			params.RealityPublicKey = publicKey
		}
	}
	if strings.TrimSpace(params.RealityShortID) == "" {
		params.RealityShortID = newSecret(4)
	}
	return nil
}

func generateRealityKeyPair() (string, string, error) {
	privateBytes := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateBytes); err != nil {
		return "", "", err
	}
	publicBytes, err := curve25519.X25519(privateBytes, curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(privateBytes), base64.RawURLEncoding.EncodeToString(publicBytes), nil
}

func deriveRealityPublicKey(privateKey string) (string, error) {
	privateBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(privateKey))
	if err != nil {
		return "", err
	}
	if len(privateBytes) != curve25519.ScalarSize {
		return "", fmt.Errorf("invalid reality private key length: %d", len(privateBytes))
	}
	publicBytes, err := curve25519.X25519(privateBytes, curve25519.Basepoint)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(publicBytes), nil
}

func (s *Store) UpdateInbound(ctx context.Context, id int64, params UpdateInboundParams) (Inbound, error) {
	remark := strings.TrimSpace(params.Remark)
	if remark == "" {
		return Inbound{}, fmt.Errorf("remark cannot be empty")
	}
	port := params.Port
	if port == 0 {
		allocated, err := s.allocateInboundPort(ctx, id)
		if err != nil {
			return Inbound{}, err
		}
		port = allocated
	}
	if port <= 0 || port > 65535 {
		return Inbound{}, fmt.Errorf("invalid port: %d", params.Port)
	}
	protocol := NormalizeInboundProtocol(params.Protocol)
	if protocol == "" {
		protocol = "vless"
	}
	if !SupportedInboundProtocol(protocol) {
		return Inbound{}, fmt.Errorf("unsupported protocol: %s", params.Protocol)
	}
	core := InferInboundCore(protocol)
	network := NormalizeInboundNetwork(protocol, params.Network)
	security := NormalizeInboundSecurity(protocol, params.Security)
	if err := s.prepareUpdateInboundRealityMaterial(ctx, id, security, &params); err != nil {
		return Inbound{}, err
	}
	candidate := Inbound{ID: id, UUID: params.UUID, Remark: remark, Protocol: protocol, Core: core, Port: port, Network: network, Security: security,
		RealityDest: params.RealityDest, RealityServerNames: params.RealityServerNames, RealityPrivateKey: params.RealityPrivateKey, RealityPublicKey: params.RealityPublicKey,
		ShadowTLSVersion: params.ShadowTLSVersion, TLSSNI: params.TLSSNI}
	if err := ValidateInboundCombination(candidate); err != nil {
		return Inbound{}, err
	}
	// Preserve existing UUID if not provided in update
	uuid := params.UUID
	if uuid == "" {
		var existingUUID string
		err := s.db.QueryRowContext(ctx, `SELECT uuid FROM inbounds WHERE id=?`, id).Scan(&existingUUID)
		if err == nil {
			uuid = existingUUID
		}
	}
	enabled := 0
	if params.Enabled {
		enabled = 1
	}
	tuicZeroRTTInt := 0
	if params.TuicZeroRTT {
		tuicZeroRTTInt = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Inbound{}, err
	}
	defer tx.Rollback()
	var oldRemark string
	var oldProtocol string
	if err := tx.QueryRowContext(ctx, `SELECT remark, protocol FROM inbounds WHERE id=?`, id).Scan(&oldRemark, &oldProtocol); err != nil {
		if err == sql.ErrNoRows {
			return Inbound{}, fmt.Errorf("inbound not found: %d", id)
		}
		return Inbound{}, err
	}
	if NormalizeInboundProtocol(oldProtocol) != protocol {
		if err := validateExistingClientsForProtocolChange(ctx, tx, id, protocol); err != nil {
			return Inbound{}, err
		}
	}
	oldRemark = strings.TrimSpace(oldRemark)
	oldRemarkMatches := 0
	if oldRemark != "" {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM inbounds WHERE TRIM(remark)=?`, oldRemark).Scan(&oldRemarkMatches); err != nil {
			return Inbound{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE inbounds SET uuid=?, remark=?, protocol=?, core=?, port=?, network=?, security=?, enabled=?,
		ws_path=?, ws_host=?, grpc_service_name=?, reality_dest=?, reality_server_names=?, reality_short_id=?, reality_private_key=?, reality_public_key=?, ss_method=?,
		tls_cert_file=?, tls_key_file=?, tls_sni=?, tls_fingerprint=?, tls_alpn=?, xhttp_path=?, xhttp_mode=?,
		hy2_up_mbps=?, hy2_down_mbps=?, hy2_obfs=?, hy2_obfs_password=?, hy2_mport=?,
		tuic_congestion_control=?, tuic_zero_rtt=?,
		wg_private_key=?, wg_address=?, wg_peer_public_key=?, wg_allowed_ips=?, wg_endpoint=?, wg_preshared_key=?, wg_mtu=?,
		shadowtls_version=?, shadowtls_password=? WHERE id=?`,
		uuid, remark, protocol, core, port, network, security, enabled,
		params.WsPath, params.WsHost, params.GrpcServiceName, params.RealityDest, params.RealityServerNames, params.RealityShortID, params.RealityPrivateKey, params.RealityPublicKey, params.SSMethod,
		params.TLSCertFile, params.TLSKeyFile, params.TLSSNI, params.TLSFingerprint, params.TLSALPN, params.XHTTPPath, params.XHTTPMode,
		params.Hy2UpMbps, params.Hy2DownMbps, params.Hy2Obfs, params.Hy2ObfsPassword, params.Hy2MPort,
		params.TuicCongestionControl, tuicZeroRTTInt,
		params.WgPrivateKey, params.WgAddress, params.WgPeerPublicKey, params.WgAllowedIPs, params.WgEndpoint, params.WgPresharedKey, params.WgMTU,
		params.ShadowTLSVersion, params.ShadowTLSPassword, id)
	if err != nil {
		return Inbound{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Inbound{}, err
	}
	if n == 0 {
		return Inbound{}, fmt.Errorf("inbound not found: %d", id)
	}
	oldGeneratedTag := fmt.Sprintf("inbound-%d-%s", id, strings.ToLower(strings.TrimSpace(oldProtocol)))
	newGeneratedTag := fmt.Sprintf("inbound-%d-%s", id, protocol)
	if oldGeneratedTag != newGeneratedTag {
		if _, err := tx.ExecContext(ctx, `UPDATE routing_rules SET inbound_tag=? WHERE inbound_tag=?`, newGeneratedTag, oldGeneratedTag); err != nil {
			return Inbound{}, err
		}
	}
	if oldRemark != "" && oldRemark != remark && oldRemarkMatches == 1 {
		if _, err := tx.ExecContext(ctx, `UPDATE routing_rules SET inbound_tag=? WHERE inbound_tag=?`, remark, oldRemark); err != nil {
			return Inbound{}, err
		}
	}
	// Reload to get the full row
	row := tx.QueryRowContext(ctx, `SELECT id, uuid, remark, protocol, core, port, network, security, enabled,
		ws_path, ws_host, grpc_service_name, reality_dest, reality_server_names, reality_short_id, reality_private_key, reality_public_key, ss_method,
		tls_cert_file, tls_key_file, tls_sni, tls_fingerprint, tls_alpn, xhttp_path, xhttp_mode,
		hy2_up_mbps, hy2_down_mbps, hy2_obfs, hy2_obfs_password, hy2_mport,
		tuic_congestion_control, tuic_zero_rtt,
		wg_private_key, wg_address, wg_peer_public_key, wg_allowed_ips, wg_endpoint, wg_preshared_key, wg_mtu,
		shadowtls_version, shadowtls_password FROM inbounds WHERE id=?`, id)
	var inbound Inbound
	var dbEnabled int
	if err := row.Scan(&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, &dbEnabled,
		&inbound.WsPath, &inbound.WsHost, &inbound.GrpcServiceName, &inbound.RealityDest, &inbound.RealityServerNames, &inbound.RealityShortID, &inbound.RealityPrivateKey, &inbound.RealityPublicKey, &inbound.SSMethod,
		&inbound.TLSCertFile, &inbound.TLSKeyFile, &inbound.TLSSNI, &inbound.TLSFingerprint, &inbound.TLSALPN, &inbound.XHTTPPath, &inbound.XHTTPMode,
		&inbound.Hy2UpMbps, &inbound.Hy2DownMbps, &inbound.Hy2Obfs, &inbound.Hy2ObfsPassword, &inbound.Hy2MPort,
		&inbound.TuicCongestionControl, &inbound.TuicZeroRTT,
		&inbound.WgPrivateKey, &inbound.WgAddress, &inbound.WgPeerPublicKey, &inbound.WgAllowedIPs, &inbound.WgEndpoint, &inbound.WgPresharedKey, &inbound.WgMTU,
		&inbound.ShadowTLSVersion, &inbound.ShadowTLSPassword); err != nil {
		return Inbound{}, err
	}
	if err := tx.Commit(); err != nil {
		return Inbound{}, err
	}
	inbound.Enabled = dbEnabled != 0
	if inbound.Core == "" {
		inbound.Core = InferInboundCore(inbound.Protocol)
	}
	inbound.Clients = []Client{}
	return inbound, nil
}

type sqlQuerier interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

func validateExistingClientsForProtocolChange(ctx context.Context, querier sqlQuerier, inboundID int64, targetProtocol string) error {
	rows, err := querier.QueryContext(ctx, `SELECT id, uuid, credential_id, password, email FROM clients WHERE inbound_id=? ORDER BY id ASC`, inboundID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var client Client
		if err := rows.Scan(&client.ID, &client.UUID, &client.CredentialID, &client.Password, &client.Email); err != nil {
			return err
		}
		if err := ValidateClientCredential(targetProtocol, client); err != nil {
			label := strings.TrimSpace(client.Email)
			if label == "" {
				label = fmt.Sprintf("client-%d", client.ID)
			}
			return fmt.Errorf("cannot change inbound protocol to %s: client %s has incompatible credentials: %w", targetProtocol, label, err)
		}
	}
	return rows.Err()
}

func (s *Store) UpdateClient(ctx context.Context, id int64, params UpdateClientParams) (Client, error) {
	email := strings.TrimSpace(params.Email)
	if email == "" {
		email = "client"
	}
	enabled := 0
	if params.Enabled {
		enabled = 1
	}
	var inboundID int64
	var existingUUID string
	var existingCredentialID string
	var existingPassword string
	var oldEmail string
	if err := s.db.QueryRowContext(ctx, `SELECT inbound_id, uuid, credential_id, password, email FROM clients WHERE id = ?`, id).Scan(&inboundID, &existingUUID, &existingCredentialID, &existingPassword, &oldEmail); err != nil {
		return Client{}, err
	}
	inbound, err := s.getInboundBasic(ctx, inboundID)
	if err != nil {
		return Client{}, err
	}
	rawUUID := firstNonEmpty(params.UUID, existingUUID)
	rawCredentialID := firstNonEmpty(params.CredentialID, existingCredentialID, rawUUID)
	rawPassword := firstNonEmpty(params.Password, existingPassword)
	if capability, ok := GetInboundCapability(inbound.Protocol); ok {
		switch capability.CredentialType {
		case CredentialUUID:
			rawCredentialID = rawUUID
		case CredentialPassword:
			if strings.TrimSpace(params.Password) == "" && strings.TrimSpace(params.UUID) != "" {
				rawPassword = params.UUID
			}
		case CredentialIDPassword, CredentialUsernamePassword:
			if strings.TrimSpace(params.CredentialID) == "" && strings.TrimSpace(params.UUID) != "" {
				rawCredentialID = params.UUID
			}
		}
	}
	uuid, credentialID, password := normalizeClientCredentials(inbound.Protocol, rawUUID, rawCredentialID, rawPassword)
	if err := ValidateClientCredential(inbound.Protocol, Client{UUID: uuid, CredentialID: credentialID, Password: password}); err != nil {
		return Client{}, err
	}
	var existingID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE inbound_id = ? AND email = ? AND id <> ? LIMIT 1`, inboundID, email, id).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client email: %s", email)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE uuid = ? AND id <> ? LIMIT 1`, uuid, id).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client uuid: %s", uuid)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if credentialID != "" {
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE credential_id = ? AND id <> ? LIMIT 1`, credentialID, id).Scan(&existingID); err == nil {
			return Client{}, fmt.Errorf("duplicate client credential_id: %s", credentialID)
		} else if err != sql.ErrNoRows {
			return Client{}, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE clients SET uuid=?, credential_id=?, password=?, email=?, enabled=?, traffic_limit=?, expiry_at=? WHERE id=?`,
		uuid, credentialID, password, email, enabled, params.TrafficLimit, params.ExpiryAt, id)
	if err != nil {
		return Client{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Client{}, err
	}
	if n == 0 {
		return Client{}, fmt.Errorf("client not found: %d", id)
	}
	if strings.TrimSpace(oldEmail) != email {
		if _, err := tx.ExecContext(ctx, `UPDATE routing_rules SET client_email=? WHERE client_id=?`, email, id); err != nil {
			return Client{}, err
		}
	}
	row := tx.QueryRowContext(ctx, `SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at FROM clients WHERE id=?`, id)
	var client Client
	var dbEnabled int
	if err := row.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &dbEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	client.Enabled = dbEnabled != 0
	return client, nil
}

func (s *Store) SetInboundEnabled(ctx context.Context, id int64, enabled bool) (Inbound, error) {
	dbEnabled := 0
	if enabled {
		dbEnabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE inbounds SET enabled=? WHERE id=?`, dbEnabled, id)
	if err != nil {
		return Inbound{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Inbound{}, err
	}
	if n == 0 {
		return Inbound{}, fmt.Errorf("inbound not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, uuid, remark, protocol, core, port, network, security, enabled,
		ws_path, ws_host, grpc_service_name, reality_dest, reality_server_names, reality_short_id, reality_private_key, reality_public_key, ss_method,
		tls_cert_file, tls_key_file, tls_sni, tls_fingerprint, tls_alpn, xhttp_path, xhttp_mode,
		hy2_up_mbps, hy2_down_mbps, hy2_obfs, hy2_obfs_password, hy2_mport,
		tuic_congestion_control, tuic_zero_rtt,
		wg_private_key, wg_address, wg_peer_public_key, wg_allowed_ips, wg_endpoint, wg_preshared_key, wg_mtu,
		shadowtls_version, shadowtls_password FROM inbounds WHERE id=?`, id)
	var inbound Inbound
	if err := row.Scan(&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, &dbEnabled,
		&inbound.WsPath, &inbound.WsHost, &inbound.GrpcServiceName, &inbound.RealityDest, &inbound.RealityServerNames, &inbound.RealityShortID, &inbound.RealityPrivateKey, &inbound.RealityPublicKey, &inbound.SSMethod,
		&inbound.TLSCertFile, &inbound.TLSKeyFile, &inbound.TLSSNI, &inbound.TLSFingerprint, &inbound.TLSALPN, &inbound.XHTTPPath, &inbound.XHTTPMode,
		&inbound.Hy2UpMbps, &inbound.Hy2DownMbps, &inbound.Hy2Obfs, &inbound.Hy2ObfsPassword, &inbound.Hy2MPort,
		&inbound.TuicCongestionControl, &inbound.TuicZeroRTT,
		&inbound.WgPrivateKey, &inbound.WgAddress, &inbound.WgPeerPublicKey, &inbound.WgAllowedIPs, &inbound.WgEndpoint, &inbound.WgPresharedKey, &inbound.WgMTU,
		&inbound.ShadowTLSVersion, &inbound.ShadowTLSPassword); err != nil {
		return Inbound{}, err
	}
	inbound.Enabled = dbEnabled != 0
	if inbound.Core == "" {
		inbound.Core = InferInboundCore(inbound.Protocol)
	}
	inbound.Clients = []Client{}
	return inbound, nil
}

func (s *Store) SetOutboundEnabled(ctx context.Context, id int64, enabled bool) (Outbound, error) {
	dbEnabled := 0
	if enabled {
		dbEnabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE outbounds SET enabled=? WHERE id=?`, dbEnabled, id)
	if err != nil {
		return Outbound{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Outbound{}, err
	}
	if n == 0 {
		return Outbound{}, fmt.Errorf("outbound not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds WHERE id=?`, id)
	var outbound Outbound
	var dbEnabledInt int
	if err := row.Scan(&outbound.ID, &outbound.Tag, &outbound.Remark, &outbound.Protocol, &outbound.Address, &outbound.Port, &outbound.Username, &outbound.Password, &dbEnabledInt, &outbound.Sort); err != nil {
		return Outbound{}, err
	}
	outbound.Enabled = dbEnabledInt != 0
	outbound.SupportedCores = OutboundProtocolSupportedCores(outbound.Protocol)
	return outbound, nil
}

func (s *Store) SetClientEnabled(ctx context.Context, inboundID int64, id int64, enabled bool) (Client, error) {
	dbEnabled := 0
	if enabled {
		dbEnabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE clients SET enabled=? WHERE inbound_id=? AND id=?`, dbEnabled, inboundID, id)
	if err != nil {
		return Client{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Client{}, err
	}
	if n == 0 {
		return Client{}, fmt.Errorf("client not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at FROM clients WHERE inbound_id=? AND id=?`, inboundID, id)
	var client Client
	if err := row.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &dbEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		return Client{}, err
	}
	client.Enabled = dbEnabled != 0
	return client, nil
}

func (s *Store) ListInbounds(ctx context.Context) ([]Inbound, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, uuid, remark, protocol, core, port, network, security, enabled,
  ws_path, ws_host, grpc_service_name, reality_dest, reality_server_names, reality_short_id, reality_private_key, reality_public_key, ss_method,
  tls_cert_file, tls_key_file, tls_sni, tls_fingerprint, tls_alpn, xhttp_path, xhttp_mode,
  hy2_up_mbps, hy2_down_mbps, hy2_obfs, hy2_obfs_password, hy2_mport,
  tuic_congestion_control, tuic_zero_rtt,
  wg_private_key, wg_address, wg_peer_public_key, wg_allowed_ips, wg_endpoint, wg_preshared_key, wg_mtu,
  shadowtls_version, shadowtls_password
FROM inbounds
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inbounds []Inbound
	byID := make(map[int64]int)
	for rows.Next() {
		var inbound Inbound
		var enabled int
		if err := rows.Scan(&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, &enabled,
			&inbound.WsPath, &inbound.WsHost, &inbound.GrpcServiceName, &inbound.RealityDest, &inbound.RealityServerNames, &inbound.RealityShortID, &inbound.RealityPrivateKey, &inbound.RealityPublicKey, &inbound.SSMethod,
			&inbound.TLSCertFile, &inbound.TLSKeyFile, &inbound.TLSSNI, &inbound.TLSFingerprint, &inbound.TLSALPN, &inbound.XHTTPPath, &inbound.XHTTPMode,
			&inbound.Hy2UpMbps, &inbound.Hy2DownMbps, &inbound.Hy2Obfs, &inbound.Hy2ObfsPassword, &inbound.Hy2MPort,
			&inbound.TuicCongestionControl, &inbound.TuicZeroRTT,
			&inbound.WgPrivateKey, &inbound.WgAddress, &inbound.WgPeerPublicKey, &inbound.WgAllowedIPs, &inbound.WgEndpoint, &inbound.WgPresharedKey, &inbound.WgMTU,
			&inbound.ShadowTLSVersion, &inbound.ShadowTLSPassword); err != nil {
			return nil, err
		}
		inbound.Enabled = enabled != 0
		if inbound.Core == "" {
			inbound.Core = InferInboundCore(inbound.Protocol)
		}
		inbound.Clients = []Client{}
		byID[inbound.ID] = len(inbounds)
		inbounds = append(inbounds, inbound)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	clientRows, err := s.db.QueryContext(ctx, `
SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at
FROM clients
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer clientRows.Close()
	for clientRows.Next() {
		var client Client
		var enabled int
		if err := clientRows.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &enabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
			return nil, err
		}
		client.Enabled = enabled != 0
		if idx, ok := byID[client.InboundID]; ok {
			inbounds[idx].Clients = append(inbounds[idx].Clients, client)
		}
	}
	if err := clientRows.Err(); err != nil {
		return nil, err
	}
	return inbounds, nil
}

func (s *Store) ListInboundTraffic(ctx context.Context) ([]Inbound, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, uuid, remark, protocol, core, port, network, security, enabled
FROM inbounds
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inbounds []Inbound
	byID := make(map[int64]int)
	for rows.Next() {
		var inbound Inbound
		var enabled int
		if err := rows.Scan(&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, &enabled); err != nil {
			return nil, err
		}
		inbound.Enabled = enabled != 0
		if inbound.Core == "" {
			inbound.Core = InferInboundCore(inbound.Protocol)
		}
		inbound.Clients = []Client{}
		byID[inbound.ID] = len(inbounds)
		inbounds = append(inbounds, inbound)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	clientRows, err := s.db.QueryContext(ctx, `
SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at
FROM clients
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer clientRows.Close()
	for clientRows.Next() {
		var client Client
		var enabled int
		if err := clientRows.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &enabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
			return nil, err
		}
		client.Enabled = enabled != 0
		if idx, ok := byID[client.InboundID]; ok {
			inbounds[idx].Clients = append(inbounds[idx].Clients, client)
		}
	}
	if err := clientRows.Err(); err != nil {
		return nil, err
	}
	return inbounds, nil
}

func (s *Store) InboundExists(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	var found int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM inbounds WHERE id=? LIMIT 1`, id).Scan(&found); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) FindInboundByPort(ctx context.Context, port int, excludeID int64) (Inbound, bool, error) {
	if port <= 0 {
		return Inbound{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, uuid, remark, protocol, core, port, network, security, enabled
FROM inbounds
WHERE port=? AND id<>?
ORDER BY id ASC
LIMIT 1
`, port, excludeID)
	var inbound Inbound
	var enabled int
	if err := row.Scan(&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, &enabled); err != nil {
		if err == sql.ErrNoRows {
			return Inbound{}, false, nil
		}
		return Inbound{}, false, err
	}
	inbound.Enabled = enabled != 0
	if inbound.Core == "" {
		inbound.Core = InferInboundCore(inbound.Protocol)
	}
	inbound.Clients = []Client{}
	return inbound, true, nil
}

func (s *Store) allocateInboundPort(ctx context.Context, excludeID int64) (int, error) {
	for port := autoInboundPortMin; port <= autoInboundPortMax; port++ {
		if _, ok, err := s.FindInboundByPort(ctx, port, excludeID); err != nil {
			return 0, err
		} else if ok {
			continue
		}
		if !inboundPortAvailable(port) {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("no available inbound port in range %d-%d", autoInboundPortMin, autoInboundPortMax)
}

func inboundPortAvailable(port int) bool {
	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	defer tcpListener.Close()
	udpConn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = udpConn.Close()
	return true
}

func (s *Store) GetSubscriptionByClientUUID(ctx context.Context, uuid string) (Inbound, Client, bool, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return Inbound{}, Client{}, false, nil
	}
	return s.getSubscriptionByClientRow(s.db.QueryRowContext(ctx, subscriptionLookupSQLByUUID, uuid))
}

func (s *Store) GetSubscriptionByToken(ctx context.Context, token string) (Inbound, Client, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Inbound{}, Client{}, false, nil
	}
	return s.getSubscriptionByClientRow(s.db.QueryRowContext(ctx, subscriptionLookupSQLByToken, token))
}

const subscriptionLookupSelect = `
SELECT i.id, i.uuid, i.remark, i.protocol, i.core, i.port, i.network, i.security, i.enabled,
  i.ws_path, i.ws_host, i.grpc_service_name, i.reality_dest, i.reality_server_names, i.reality_short_id, i.reality_private_key, i.reality_public_key, i.ss_method,
  i.tls_cert_file, i.tls_key_file, i.tls_sni, i.tls_fingerprint, i.tls_alpn, i.xhttp_path, i.xhttp_mode,
  i.hy2_up_mbps, i.hy2_down_mbps, i.hy2_obfs, i.hy2_obfs_password, i.hy2_mport,
  i.tuic_congestion_control, i.tuic_zero_rtt,
  i.wg_private_key, i.wg_address, i.wg_peer_public_key, i.wg_allowed_ips, i.wg_endpoint, i.wg_preshared_key, i.wg_mtu,
  i.shadowtls_version, i.shadowtls_password,
  c.id, c.inbound_id, c.uuid, c.credential_id, c.password, c.subscription_token, c.stats_key, c.email, c.enabled, c.up, c.down, c.traffic_limit, c.expiry_at
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
`

const subscriptionLookupSQLByUUID = subscriptionLookupSelect + `
WHERE c.uuid = ?
LIMIT 1
`

const subscriptionLookupSQLByToken = subscriptionLookupSelect + `
WHERE c.subscription_token = ?
LIMIT 1
`

func (s *Store) getSubscriptionByClientRow(row *sql.Row) (Inbound, Client, bool, error) {
	var inbound Inbound
	var client Client
	var inboundEnabled int
	var clientEnabled int
	if err := row.Scan(&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, &inboundEnabled,
		&inbound.WsPath, &inbound.WsHost, &inbound.GrpcServiceName, &inbound.RealityDest, &inbound.RealityServerNames, &inbound.RealityShortID, &inbound.RealityPrivateKey, &inbound.RealityPublicKey, &inbound.SSMethod,
		&inbound.TLSCertFile, &inbound.TLSKeyFile, &inbound.TLSSNI, &inbound.TLSFingerprint, &inbound.TLSALPN, &inbound.XHTTPPath, &inbound.XHTTPMode,
		&inbound.Hy2UpMbps, &inbound.Hy2DownMbps, &inbound.Hy2Obfs, &inbound.Hy2ObfsPassword, &inbound.Hy2MPort,
		&inbound.TuicCongestionControl, &inbound.TuicZeroRTT,
		&inbound.WgPrivateKey, &inbound.WgAddress, &inbound.WgPeerPublicKey, &inbound.WgAllowedIPs, &inbound.WgEndpoint, &inbound.WgPresharedKey, &inbound.WgMTU,
		&inbound.ShadowTLSVersion, &inbound.ShadowTLSPassword,
		&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &clientEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		if err == sql.ErrNoRows {
			return Inbound{}, Client{}, false, nil
		}
		return Inbound{}, Client{}, false, err
	}
	inbound.Enabled = inboundEnabled != 0
	if inbound.Core == "" {
		inbound.Core = InferInboundCore(inbound.Protocol)
	}
	client.Enabled = clientEnabled != 0
	inbound.Clients = []Client{client}
	return inbound, client, true, nil
}

func (s *Store) ResetClientTraffic(ctx context.Context, id int64) (Client, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE clients SET up=0, down=0 WHERE id=?`, id)
	if err != nil {
		return Client{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Client{}, err
	}
	if n == 0 {
		return Client{}, fmt.Errorf("client not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at FROM clients WHERE id=?`, id)
	var client Client
	var dbEnabled int
	if err := row.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &dbEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		return Client{}, err
	}
	client.Enabled = dbEnabled != 0
	return client, nil
}

func (s *Store) UpdateClientTraffic(ctx context.Context, email string, uplink, downlink int64) error {
	key, ok, err := s.resolveLegacyTrafficKey(ctx, email)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return s.ApplyTrafficRawStats(ctx, []TrafficRawStat{{Engine: "xray", ScopeType: "client", ScopeKey: key, RawUp: uplink, RawDown: downlink, Status: "ok"}}, time.Now().UTC())
}

func (s *Store) UpdateClientTrafficBatch(ctx context.Context, stats map[string]ClientTrafficUpdate) error {
	if len(stats) == 0 {
		return nil
	}
	raw := make([]TrafficRawStat, 0, len(stats))
	for key, traffic := range stats {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !strings.HasPrefix(key, "c_") {
			resolved, ok, err := s.resolveLegacyTrafficKey(ctx, key)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			key = resolved
		}
		raw = append(raw, TrafficRawStat{Engine: "xray", ScopeType: "client", ScopeKey: key, RawUp: traffic.Up, RawDown: traffic.Down, Status: "ok"})
	}
	return s.ApplyTrafficRawStats(ctx, raw, time.Now().UTC())
}

func (s *Store) resolveLegacyTrafficKey(ctx context.Context, key string) (string, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, nil
	}
	var statsKey string
	if err := s.db.QueryRowContext(ctx, `SELECT stats_key FROM clients WHERE stats_key=? LIMIT 1`, key).Scan(&statsKey); err == nil {
		return statsKey, true, nil
	} else if err != sql.ErrNoRows {
		return "", false, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT stats_key FROM clients WHERE email=? ORDER BY id ASC LIMIT 2`, key)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var candidate string
		if err := rows.Scan(&candidate); err != nil {
			return "", false, err
		}
		keys = append(keys, candidate)
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if len(keys) != 1 || strings.TrimSpace(keys[0]) == "" {
		return "", false, nil
	}
	return keys[0], true, nil
}

func (s *Store) ApplyTrafficRawStats(ctx context.Context, stats []TrafficRawStat, observedAt time.Time) error {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	normalizedStats := normalizeTrafficRawStats(stats)
	if len(normalizedStats) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	currentStates, err := prefetchTrafficStates(ctx, tx, normalizedStats)
	if err != nil {
		return err
	}
	clientInfo, err := prefetchTrafficClientInfo(ctx, tx, normalizedStats)
	if err != nil {
		return err
	}
	seenClients := map[string]trafficClientInfo{}
	seenAt := observedAt.UTC().Format(time.RFC3339Nano)
	for _, raw := range normalizedStats {
		stateKey := trafficStateKey(raw.Engine, raw.ScopeType, raw.ScopeKey)
		current, hasCurrent := currentStates[stateKey]
		if !hasCurrent && raw.ScopeType == "client" {
			if info, ok := clientInfo[raw.ScopeKey]; ok {
				current.TotalUp = info.Up
				current.TotalDown = info.Down
			}
		}
		var elapsed float64
		if hasCurrent && current.LastSeenAt != "" {
			if previous, parseErr := time.Parse(time.RFC3339Nano, current.LastSeenAt); parseErr == nil && observedAt.After(previous) {
				elapsed = observedAt.Sub(previous).Seconds()
			}
		}
		deltaUp := int64(0)
		deltaDown := int64(0)
		if !hasCurrent || isResetWithoutRawBaseline(current) {
			deltaUp = 0
			deltaDown = 0
		} else {
			if raw.RawUp >= current.LastRawUp {
				deltaUp = raw.RawUp - current.LastRawUp
			}
			if raw.RawDown >= current.LastRawDown {
				deltaDown = raw.RawDown - current.LastRawDown
			}
		}
		totalUp := current.TotalUp + deltaUp
		totalDown := current.TotalDown + deltaDown
		rateUp := 0.0
		rateDown := 0.0
		if elapsed > 0 {
			rateUp = float64(deltaUp) / elapsed
			rateDown = float64(deltaDown) / elapsed
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_states (engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, rate_up, rate_down, last_seen_at, status, message)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(engine, scope_type, scope_key) DO UPDATE SET
  total_up=excluded.total_up,
  total_down=excluded.total_down,
  last_raw_up=excluded.last_raw_up,
  last_raw_down=excluded.last_raw_down,
  rate_up=excluded.rate_up,
  rate_down=excluded.rate_down,
  last_seen_at=excluded.last_seen_at,
  status=excluded.status,
  message=excluded.message
`, raw.Engine, raw.ScopeType, raw.ScopeKey, totalUp, totalDown, raw.RawUp, raw.RawDown, rateUp, rateDown, seenAt, raw.Status, strings.TrimSpace(raw.Message)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO traffic_samples (sampled_at, engine, scope_type, scope_key, total_up, total_down, rate_up, rate_down, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			seenAt, raw.Engine, raw.ScopeType, raw.ScopeKey, totalUp, totalDown, rateUp, rateDown, raw.Status); err != nil {
			return err
		}
		current.Engine = raw.Engine
		current.ScopeType = raw.ScopeType
		current.ScopeKey = raw.ScopeKey
		current.TotalUp = totalUp
		current.TotalDown = totalDown
		current.LastRawUp = raw.RawUp
		current.LastRawDown = raw.RawDown
		current.RateUp = rateUp
		current.RateDown = rateDown
		current.LastSeenAt = seenAt
		current.Status = raw.Status
		current.Message = strings.TrimSpace(raw.Message)
		currentStates[stateKey] = current
		if raw.ScopeType == "client" {
			if info, ok := clientInfo[raw.ScopeKey]; ok && info.ExpectedEngine == raw.Engine {
				info.Up = totalUp
				info.Down = totalDown
				seenClients[raw.ScopeKey] = info
			}
		}
	}
	for statsKey, info := range seenClients {
		if _, err := tx.ExecContext(ctx, `
UPDATE clients
SET up = ?, down = ?
WHERE stats_key = ?`, info.Up, info.Down, statsKey); err != nil {
			return err
		}
	}
	rollbackCleanup, err := s.cleanupTrafficSamples(ctx, tx, observedAt)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		rollbackCleanup()
		return err
	}
	return nil
}

func normalizeTrafficRawStats(stats []TrafficRawStat) []TrafficRawStat {
	normalized := make([]TrafficRawStat, 0, len(stats))
	for _, raw := range stats {
		raw.Engine = normalizeTrafficEngine(raw.Engine)
		raw.ScopeType = normalizeTrafficToken(raw.ScopeType)
		raw.ScopeKey = strings.TrimSpace(raw.ScopeKey)
		raw.Status = strings.TrimSpace(raw.Status)
		raw.Message = strings.TrimSpace(raw.Message)
		if raw.Status == "" {
			raw.Status = "ok"
		}
		if raw.Engine == "" || raw.ScopeType == "" || raw.ScopeKey == "" {
			continue
		}
		normalized = append(normalized, raw)
	}
	return normalized
}

func trafficStateKey(engine, scopeType, scopeKey string) string {
	return engine + "\x00" + scopeType + "\x00" + scopeKey
}

func prefetchTrafficStates(ctx context.Context, tx *sql.Tx, stats []TrafficRawStat) (map[string]TrafficState, error) {
	keys := make([]string, 0, len(stats))
	seen := map[string]struct{}{}
	for _, raw := range stats {
		key := trafficStateKey(raw.Engine, raw.ScopeType, raw.ScopeKey)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	states := map[string]TrafficState{}
	for start := 0; start < len(keys); start += sqliteVariableChunkSize / 3 {
		end := start + sqliteVariableChunkSize/3
		if end > len(keys) {
			end = len(keys)
		}
		conditions := make([]string, 0, end-start)
		args := make([]interface{}, 0, (end-start)*3)
		for _, key := range keys[start:end] {
			parts := strings.SplitN(key, "\x00", 3)
			conditions = append(conditions, "(engine=? AND scope_type=? AND scope_key=?)")
			args = append(args, parts[0], parts[1], parts[2])
		}
		rows, err := tx.QueryContext(ctx, `
SELECT engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, rate_up, rate_down, last_seen_at, status, message
FROM traffic_states
WHERE `+strings.Join(conditions, " OR "), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var state TrafficState
			if err := rows.Scan(&state.Engine, &state.ScopeType, &state.ScopeKey, &state.TotalUp, &state.TotalDown, &state.LastRawUp, &state.LastRawDown, &state.RateUp, &state.RateDown, &state.LastSeenAt, &state.Status, &state.Message); err != nil {
				rows.Close()
				return nil, err
			}
			state.Engine = normalizeTrafficEngine(state.Engine)
			state.ScopeType = normalizeTrafficToken(state.ScopeType)
			state.ScopeKey = strings.TrimSpace(state.ScopeKey)
			states[trafficStateKey(state.Engine, state.ScopeType, state.ScopeKey)] = state
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return states, nil
}

type trafficClientInfo struct {
	ExpectedEngine string
	Up             int64
	Down           int64
}

func prefetchTrafficClientInfo(ctx context.Context, tx *sql.Tx, stats []TrafficRawStat) (map[string]trafficClientInfo, error) {
	keys := make([]string, 0, len(stats))
	seen := map[string]struct{}{}
	for _, raw := range stats {
		if raw.ScopeType != "client" {
			continue
		}
		if _, ok := seen[raw.ScopeKey]; ok {
			continue
		}
		seen[raw.ScopeKey] = struct{}{}
		keys = append(keys, raw.ScopeKey)
	}
	info := map[string]trafficClientInfo{}
	for start := 0; start < len(keys); start += sqliteVariableChunkSize {
		end := start + sqliteVariableChunkSize
		if end > len(keys) {
			end = len(keys)
		}
		placeholders := placeholders(len(keys[start:end]))
		args := make([]interface{}, 0, end-start)
		for _, key := range keys[start:end] {
			args = append(args, key)
		}
		rows, err := tx.QueryContext(ctx, `
SELECT c.stats_key, c.up, c.down, i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.stats_key IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var statsKey string
			var protocol string
			var item trafficClientInfo
			if err := rows.Scan(&statsKey, &item.Up, &item.Down, &protocol); err != nil {
				rows.Close()
				return nil, err
			}
			item.ExpectedEngine = expectedTrafficEngineForProtocol(protocol)
			info[statsKey] = item
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return info, nil
}

const trafficSamplesCleanupInterval = time.Hour

func (s *Store) cleanupTrafficSamples(ctx context.Context, tx *sql.Tx, observedAt time.Time) (func(), error) {
	s.trafficCleanupMu.Lock()
	if !s.nextTrafficSamplesCleanup.IsZero() && observedAt.Before(s.nextTrafficSamplesCleanup) {
		s.trafficCleanupMu.Unlock()
		return func() {}, nil
	}
	previousCleanup := s.nextTrafficSamplesCleanup
	nextCleanup := observedAt.Add(trafficSamplesCleanupInterval)
	s.nextTrafficSamplesCleanup = nextCleanup
	s.trafficCleanupMu.Unlock()
	cutoff := observedAt.Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `DELETE FROM traffic_samples WHERE sampled_at < ?`, cutoff); err != nil {
		s.trafficCleanupMu.Lock()
		if s.nextTrafficSamplesCleanup.Equal(nextCleanup) {
			s.nextTrafficSamplesCleanup = previousCleanup
		}
		s.trafficCleanupMu.Unlock()
		return func() {}, err
	}
	return func() {
		s.trafficCleanupMu.Lock()
		if s.nextTrafficSamplesCleanup.Equal(nextCleanup) {
			s.nextTrafficSamplesCleanup = previousCleanup
		}
		s.trafficCleanupMu.Unlock()
	}, nil
}

func isResetWithoutRawBaseline(state TrafficState) bool {
	return state.TotalUp == 0 && state.TotalDown == 0 &&
		state.LastRawUp == 0 && state.LastRawDown == 0 &&
		normalizeTrafficStatus(state.Status) == "unavailable" &&
		strings.Contains(strings.ToLower(state.Message), "baseline unavailable")
}

func (s *Store) MarkTrafficUnavailable(ctx context.Context, engine, status, message string, observedAt time.Time) error {
	engine = normalizeTrafficToken(engine)
	if engine == "" {
		return nil
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "unavailable"
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE traffic_states SET rate_up=0, rate_down=0, status=?, message=?, last_seen_at=? WHERE engine=?`,
		status, strings.TrimSpace(message), observedAt.UTC().Format(time.RFC3339Nano), engine)
	return err
}

func (s *Store) MarkTrafficScopeStatus(ctx context.Context, stats []TrafficStatusMarker, observedAt time.Time) error {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if len(stats) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seenAt := observedAt.UTC().Format(time.RFC3339Nano)
	for _, marker := range stats {
		engine := normalizeTrafficToken(marker.Engine)
		scopeType := normalizeTrafficToken(marker.ScopeType)
		scopeKey := strings.TrimSpace(marker.ScopeKey)
		if engine == "" || scopeType == "" || scopeKey == "" {
			continue
		}
		status := strings.TrimSpace(marker.Status)
		if status == "" {
			status = "unavailable"
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_states (engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, rate_up, rate_down, last_seen_at, status, message)
VALUES (?, ?, ?, 0, 0, 0, 0, 0, 0, ?, ?, ?)
ON CONFLICT(engine, scope_type, scope_key) DO UPDATE SET
  rate_up=0,
  rate_down=0,
  last_seen_at=excluded.last_seen_at,
  status=excluded.status,
  message=excluded.message
`, engine, scopeType, scopeKey, seenAt, status, strings.TrimSpace(marker.Message)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ResetClientTrafficBaseline(ctx context.Context, id int64, baselines []TrafficRawStat) (Client, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at FROM clients WHERE id=?`, id)
	var client Client
	var dbEnabled int
	if err := row.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &dbEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		return Client{}, err
	}
	client.Enabled = dbEnabled != 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	existingEngines, err := trafficStateEnginesForClient(ctx, tx, client.StatsKey)
	if err != nil {
		return Client{}, err
	}
	expectedEngine, ok, err := clientExpectedTrafficEngineByID(ctx, tx, client.ID)
	if err != nil {
		return Client{}, err
	}
	if ok && expectedEngine != "" {
		existingEngines[expectedEngine] = struct{}{}
	}
	baselineByEngine := map[string]TrafficRawStat{}
	for _, raw := range baselines {
		if normalizeTrafficToken(raw.ScopeType) != "client" || strings.TrimSpace(raw.ScopeKey) != client.StatsKey {
			continue
		}
		engine := normalizeTrafficToken(raw.Engine)
		if engine == "" {
			continue
		}
		baselineByEngine[engine] = raw
		existingEngines[engine] = struct{}{}
	}
	if len(existingEngines) == 0 {
		existingEngines["migate"] = struct{}{}
	}
	for engine := range existingEngines {
		raw, hasBaseline := baselineByEngine[engine]
		status := "waiting"
		message := "baseline reset"
		lastRawUp := int64(0)
		lastRawDown := int64(0)
		if hasBaseline {
			lastRawUp = raw.RawUp
			lastRawDown = raw.RawDown
		} else if engine != "migate" {
			status = "unavailable"
			message = "baseline unavailable during reset"
		} else {
			message = "waiting for first sample"
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_states (engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, rate_up, rate_down, last_seen_at, status, message)
VALUES (?, 'client', ?, 0, 0, ?, ?, 0, 0, ?, ?, ?)
ON CONFLICT(engine, scope_type, scope_key) DO UPDATE SET
  total_up=0,
  total_down=0,
  last_raw_up=excluded.last_raw_up,
  last_raw_down=excluded.last_raw_down,
  rate_up=0,
  rate_down=0,
  last_seen_at=excluded.last_seen_at,
  status=excluded.status,
  message=excluded.message
`, engine, client.StatsKey, lastRawUp, lastRawDown, now, status, message); err != nil {
			return Client{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE clients SET up=0, down=0 WHERE id=?`, id); err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	client.Up = 0
	client.Down = 0
	return client, nil
}

func (s *Store) ListTrafficStates(ctx context.Context) ([]TrafficState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, rate_up, rate_down, last_seen_at, status, message FROM traffic_states ORDER BY engine, scope_type, scope_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := []TrafficState{}
	for rows.Next() {
		var state TrafficState
		if err := rows.Scan(&state.Engine, &state.ScopeType, &state.ScopeKey, &state.TotalUp, &state.TotalDown, &state.LastRawUp, &state.LastRawDown, &state.RateUp, &state.RateDown, &state.LastSeenAt, &state.Status, &state.Message); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *Store) ListTrafficSamples(ctx context.Context, scopeType string, since time.Time, limit int) ([]TrafficSample, error) {
	scopeType = normalizeTrafficToken(scopeType)
	if scopeType == "" {
		scopeType = "core"
	}
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}
	sinceText := since.UTC().Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx, `
SELECT sampled_at, engine, scope_type, scope_key, total_up, total_down, rate_up, rate_down, status
FROM traffic_samples
WHERE scope_type = ? AND sampled_at >= ?
ORDER BY sampled_at ASC, engine ASC, scope_key ASC
LIMIT ?`, scopeType, sinceText, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	samples := []TrafficSample{}
	for rows.Next() {
		var sample TrafficSample
		if err := rows.Scan(&sample.SampledAt, &sample.Engine, &sample.ScopeType, &sample.ScopeKey, &sample.TotalUp, &sample.TotalDown, &sample.RateUp, &sample.RateDown, &sample.Status); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (s *Store) GetClientTrafficUsage(ctx context.Context, statsKey string) (ClientTrafficUsage, bool, error) {
	statsKey = strings.TrimSpace(statsKey)
	if statsKey == "" {
		return ClientTrafficUsage{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, `
SELECT c.id, c.stats_key, c.up, c.down, i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.stats_key = ?
LIMIT 1`, statsKey)
	return s.getClientTrafficUsageFromRow(ctx, row)
}

func (s *Store) GetClientTrafficUsageForClient(ctx context.Context, clientID int64) (ClientTrafficUsage, bool, error) {
	if clientID <= 0 {
		return ClientTrafficUsage{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, `
SELECT c.id, c.stats_key, c.up, c.down, i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.id = ?
LIMIT 1`, clientID)
	return s.getClientTrafficUsageFromRow(ctx, row)
}

func (s *Store) getClientTrafficUsageFromRow(ctx context.Context, row *sql.Row) (ClientTrafficUsage, bool, error) {
	var clientID int64
	var statsKey string
	var legacyUp int64
	var legacyDown int64
	var protocol string
	if err := row.Scan(&clientID, &statsKey, &legacyUp, &legacyDown, &protocol); err != nil {
		if err == sql.ErrNoRows {
			return ClientTrafficUsage{}, false, nil
		}
		return ClientTrafficUsage{}, false, err
	}
	states, err := s.trafficStatesForClient(ctx, statsKey)
	if err != nil {
		return ClientTrafficUsage{}, false, err
	}
	usage := chooseClientTrafficUsage(clientID, statsKey, expectedTrafficEngineForProtocol(protocol), states, legacyUp, legacyDown)
	return usage, true, nil
}

func (s *Store) trafficStatesForClient(ctx context.Context, statsKey string) ([]TrafficState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, rate_up, rate_down, last_seen_at, status, message
FROM traffic_states
WHERE scope_type='client' AND scope_key=?
ORDER BY engine ASC`, statsKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := []TrafficState{}
	for rows.Next() {
		var state TrafficState
		if err := rows.Scan(&state.Engine, &state.ScopeType, &state.ScopeKey, &state.TotalUp, &state.TotalDown, &state.LastRawUp, &state.LastRawDown, &state.RateUp, &state.RateDown, &state.LastSeenAt, &state.Status, &state.Message); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func chooseClientTrafficUsage(clientID int64, statsKey, expectedEngine string, states []TrafficState, legacyUp, legacyDown int64) ClientTrafficUsage {
	byEngine := map[string]TrafficState{}
	for _, state := range states {
		if normalizeTrafficToken(state.ScopeType) != "client" || strings.TrimSpace(state.ScopeKey) != statsKey {
			continue
		}
		engine := normalizeTrafficEngine(state.Engine)
		if engine == "" {
			continue
		}
		state.Engine = engine
		state.Status = normalizeTrafficStatus(state.Status)
		byEngine[engine] = state
	}
	if state, ok := byEngine[expectedEngine]; ok {
		return usageFromTrafficState(clientID, statsKey, state)
	}
	for _, engine := range fallbackTrafficEngines(expectedEngine) {
		if state, ok := byEngine[engine]; ok {
			return usageFromTrafficState(clientID, statsKey, state)
		}
	}
	if legacyUp > 0 || legacyDown > 0 {
		return ClientTrafficUsage{
			ClientID:  clientID,
			StatsKey:  statsKey,
			Engine:    "migate",
			TotalUp:   legacyUp,
			TotalDown: legacyDown,
			Status:    "cumulative_only",
		}
	}
	return ClientTrafficUsage{ClientID: clientID, StatsKey: statsKey, Engine: expectedEngine, Status: "waiting"}
}

func usageFromTrafficState(clientID int64, statsKey string, state TrafficState) ClientTrafficUsage {
	return ClientTrafficUsage{
		ClientID: clientID, StatsKey: statsKey, Engine: normalizeTrafficEngine(state.Engine),
		TotalUp: state.TotalUp, TotalDown: state.TotalDown, RateUp: state.RateUp, RateDown: state.RateDown,
		Status: normalizeTrafficStatus(state.Status), Message: state.Message, LastSeenAt: state.LastSeenAt,
	}
}

func fallbackTrafficEngines(expectedEngine string) []string {
	switch normalizeTrafficEngine(expectedEngine) {
	case "singbox":
		return []string{"xray", "migate"}
	case "xray":
		return []string{"singbox", "migate"}
	default:
		return []string{"xray", "singbox", "migate"}
	}
}

func trafficStateEnginesForClient(ctx context.Context, tx *sql.Tx, statsKey string) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `SELECT engine FROM traffic_states WHERE scope_type='client' AND scope_key=? ORDER BY engine ASC`, statsKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	engines := map[string]struct{}{}
	for rows.Next() {
		var engine string
		if err := rows.Scan(&engine); err != nil {
			return nil, err
		}
		if engine = normalizeTrafficEngine(engine); engine != "" {
			engines[engine] = struct{}{}
		}
	}
	return engines, rows.Err()
}

func clientExpectedTrafficEngineByID(ctx context.Context, tx *sql.Tx, clientID int64) (string, bool, error) {
	var protocol string
	if err := tx.QueryRowContext(ctx, `
SELECT i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.id = ?
LIMIT 1`, clientID).Scan(&protocol); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return expectedTrafficEngineForProtocol(protocol), true, nil
}

func lookupClientExpectedTrafficEngine(ctx context.Context, tx *sql.Tx, statsKey string) (string, bool, error) {
	var protocol string
	if err := tx.QueryRowContext(ctx, `
SELECT i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.stats_key = ?
LIMIT 1`, statsKey).Scan(&protocol); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return expectedTrafficEngineForProtocol(protocol), true, nil
}

func expectedTrafficEngineForProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "hysteria2", "tuic", "shadowtls":
		return "singbox"
	default:
		return "xray"
	}
}

func normalizeTrafficEngine(engine string) string {
	switch normalizeTrafficToken(engine) {
	case "sing-box":
		return "singbox"
	default:
		return normalizeTrafficToken(engine)
	}
}

func normalizeTrafficStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "waiting"
	}
	return status
}

func lookupClientLegacyTraffic(ctx context.Context, tx *sql.Tx, statsKey string) (int64, int64, bool, error) {
	var up int64
	var down int64
	if err := tx.QueryRowContext(ctx, `SELECT up, down FROM clients WHERE stats_key=? LIMIT 1`, statsKey).Scan(&up, &down); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	return up, down, true, nil
}

func normalizeTrafficToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]), hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]), hex.EncodeToString(b[10:16]))
}

func randomHexToken(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func newSecret(byteLen int) string {
	token, err := randomHexToken(byteLen)
	if err != nil {
		panic(err)
	}
	return token
}

func (s *Store) newSubscriptionToken(ctx context.Context) (string, error) {
	for i := 0; i < 8; i++ {
		token, err := randomHexToken(24)
		if err != nil {
			return "", err
		}
		var existingID int64
		err = s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE subscription_token = ? LIMIT 1`, token).Scan(&existingID)
		if err == sql.ErrNoRows {
			return token, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not generate unique subscription token")
}

func (s *Store) newStatsKey(ctx context.Context) (string, error) {
	for i := 0; i < 8; i++ {
		token, err := randomHexToken(16)
		if err != nil {
			return "", err
		}
		key := "c_" + token
		var existingID int64
		err = s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE stats_key = ? LIMIT 1`, key).Scan(&existingID)
		if err == sql.ErrNoRows {
			return key, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to generate unique stats key")
}

// AddToBlacklist inserts a token hash into the token_blacklist table or
// updates it if it already exists (e.g. marks as revoked on logout).
// Used both for initial session tracking (revoked=0) and for revocations (revoked=1).
func (s *Store) AddToBlacklist(ctx context.Context, tokenHash string, expiresAt time.Time, revoked bool) error {
	revokedInt := 0
	if revoked {
		revokedInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO token_blacklist (token_hash, created_at, last_used, expires_at, revoked) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(token_hash) DO UPDATE SET revoked=excluded.revoked, last_used=excluded.last_used`,
		tokenHash, now, now, expiresAt.UTC().Format(time.RFC3339), revokedInt)
	return err
}

// IsBlacklisted checks if a token hash exists in the blacklist and is marked as revoked.
func (s *Store) IsBlacklisted(ctx context.Context, tokenHash string) (bool, error) {
	var revoked int
	err := s.db.QueryRowContext(ctx, `SELECT revoked FROM token_blacklist WHERE token_hash=? AND expires_at > ?`,
		tokenHash, time.Now().UTC().Format(time.RFC3339)).Scan(&revoked)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return revoked != 0, nil
}

// RecordSessionTouch updates the last_used timestamp for a session token.
func (s *Store) RecordSessionTouch(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET last_used=? WHERE token_hash=?`,
		time.Now().UTC().Format(time.RFC3339), tokenHash)
	return err
}

// RecordSessionTouchAfter updates last_used only when it is older than minAge.
// This keeps revocation checks current without turning every API request into a
// write transaction.
func (s *Store) RecordSessionTouchAfter(ctx context.Context, tokenHash string, minAge time.Duration) error {
	cutoff := time.Now().UTC().Add(-minAge).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET last_used=? WHERE token_hash=? AND last_used < ?`,
		time.Now().UTC().Format(time.RFC3339), tokenHash, cutoff)
	return err
}

func (s *Store) CleanupExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM token_blacklist WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// PruneActiveSessions revokes older active sessions, keeping only the newest maxActive records.
func (s *Store) PruneActiveSessions(ctx context.Context, maxActive int) error {
	if err := s.CleanupExpiredSessions(ctx); err != nil {
		return err
	}
	if maxActive < 0 {
		maxActive = 0
	}
	_, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET revoked=1 WHERE id IN (
SELECT id FROM token_blacklist WHERE revoked=0 AND expires_at > ? ORDER BY id DESC LIMIT -1 OFFSET ?
)`, time.Now().UTC().Format(time.RFC3339), maxActive)
	return err
}

// RevokeOtherSessions revokes all active sessions except the supplied token hash.
func (s *Store) RevokeOtherSessions(ctx context.Context, currentTokenHash string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET revoked=1 WHERE revoked=0 AND token_hash<>? AND expires_at > ?`,
		currentTokenHash, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListActiveSessions returns non-revoked, non-expired sessions.
func (s *Store) ListActiveSessions(ctx context.Context) ([]BlacklistedSession, error) {
	if err := s.CleanupExpiredSessions(ctx); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, token_hash, created_at, last_used, expires_at, revoked FROM token_blacklist WHERE revoked=0 AND expires_at > ? ORDER BY id DESC`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []BlacklistedSession
	for rows.Next() {
		var s BlacklistedSession
		var revoked int
		if err := rows.Scan(&s.ID, &s.TokenHash, &s.CreatedAt, &s.LastUsed, &s.ExpiresAt, &revoked); err != nil {
			return nil, err
		}
		s.Revoked = revoked != 0
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// RevokeSession marks a session as revoked by its database ID.
func (s *Store) RevokeSession(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET revoked=1 WHERE id=? AND revoked=0`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("active session not found: %d", id)
	}
	return nil
}
