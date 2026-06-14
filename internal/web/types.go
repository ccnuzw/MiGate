package web

import (
	"context"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/xray"
)

type XrayStatusStore interface {
	ListInbounds(ctx context.Context) ([]db.Inbound, error)
}

type clientTrafficSummary struct {
	Up             int64  `json:"up"`
	Down           int64  `json:"down"`
	XrayUp         int64  `json:"xray_up,omitempty"`
	XrayDown       int64  `json:"xray_down,omitempty"`
	Source         string `json:"source"`
	RealtimeSource string `json:"realtime_source,omitempty"`
	Note           string `json:"note,omitempty"`
}

type inboundTrafficSummary struct {
	Up             int64  `json:"up"`
	Down           int64  `json:"down"`
	Total          int64  `json:"total"`
	Source         string `json:"source"`
	RealtimeSource string `json:"realtime_source,omitempty"`
}

type trafficSeriesPoint struct {
	Name string `json:"name"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

type inboundView struct {
	db.Inbound
	TrafficUp      int64                          `json:"traffic_up"`
	TrafficDown    int64                          `json:"traffic_down"`
	TrafficTotal   int64                          `json:"traffic_total"`
	TrafficSource  string                         `json:"traffic_stats_source"`
	RealtimeSource string                         `json:"realtime_stats_source,omitempty"`
	ClientTraffic  map[int64]clientTrafficSummary `json:"client_traffic,omitempty"`
}

type Store interface {
	ListInbounds(ctx context.Context) ([]db.Inbound, error)
	GetSubscriptionByClientUUID(ctx context.Context, uuid string) (db.Inbound, db.Client, bool, error)
	CreateInbound(ctx context.Context, params db.CreateInboundParams) (db.Inbound, error)
	ListOutbounds(ctx context.Context) ([]db.Outbound, error)
	CreateOutbound(ctx context.Context, params db.CreateOutboundParams) (db.Outbound, error)
	UpdateOutbound(ctx context.Context, id int64, params db.UpdateOutboundParams) (db.Outbound, error)
	DeleteOutbound(ctx context.Context, id int64) error
	ReorderOutbounds(ctx context.Context, ids []int64) error
	ListRoutingRules(ctx context.Context) ([]db.RoutingRule, error)
	CreateRoutingRule(ctx context.Context, params db.CreateRoutingRuleParams) (db.RoutingRule, error)
	UpdateRoutingRule(ctx context.Context, id int64, params db.UpdateRoutingRuleParams) (db.RoutingRule, error)
	DeleteRoutingRule(ctx context.Context, id int64) error
	ReorderRoutingRules(ctx context.Context, ids []int64) error
	CreateClient(ctx context.Context, params db.CreateClientParams) (db.Client, error)
	DeleteInbound(ctx context.Context, id int64) error
	DeleteClient(ctx context.Context, id int64) error
	UpdateInbound(ctx context.Context, id int64, params db.UpdateInboundParams) (db.Inbound, error)
	UpdateClient(ctx context.Context, id int64, params db.UpdateClientParams) (db.Client, error)
	SetInboundEnabled(ctx context.Context, id int64, enabled bool) (db.Inbound, error)
	SetOutboundEnabled(ctx context.Context, id int64, enabled bool) (db.Outbound, error)
	SetClientEnabled(ctx context.Context, inboundID int64, id int64, enabled bool) (db.Client, error)
	ResetClientTraffic(ctx context.Context, id int64) (db.Client, error)
	AddToBlacklist(ctx context.Context, tokenHash string, expiresAt time.Time, revoked bool) error
	IsBlacklisted(ctx context.Context, tokenHash string) (bool, error)
	RecordSessionTouch(ctx context.Context, tokenHash string) error
	ListActiveSessions(ctx context.Context) ([]db.BlacklistedSession, error)
	RevokeSession(ctx context.Context, id int64) error
}

type sessionTouchThrottler interface {
	RecordSessionTouchAfter(ctx context.Context, tokenHash string, minAge time.Duration) error
}

type XrayController interface {
	Status(ctx context.Context) XrayStatus
	Apply(ctx context.Context) XrayApplyResult
	Version(ctx context.Context) string
}

type XrayStatus struct {
	Service           string   `json:"service"`
	Status            string   `json:"status"`
	Managed           bool     `json:"managed"`
	Installed         bool     `json:"installed"`
	Version           string   `json:"version"`
	MemoryRSSBytes    int64    `json:"memory_rss_bytes"`
	Uptime            string   `json:"uptime"`
	ActiveConnections int      `json:"active_connections"`
	ConfigPath        string   `json:"config_path"`
	CommandsExecuted  []string `json:"commands_executed"`
}

type XrayApplyResult struct {
	Status           string   `json:"status"`
	Service          string   `json:"service"`
	CommandsExecuted []string `json:"commands_executed"`
	ErrorOutput      string   `json:"error_output,omitempty"`
}

type defaultXrayController struct{}

func (defaultXrayController) Status(ctx context.Context) XrayStatus {
	return XrayStatus{Service: "xray", Status: "unknown", Managed: false, CommandsExecuted: []string{}}
}

func (defaultXrayController) Apply(ctx context.Context) XrayApplyResult {
	return XrayApplyResult{Status: "not_managed"}
}

func (defaultXrayController) Version(ctx context.Context) string { return "" }

type routerConfig struct {
	store          Store
	xrayController XrayController
	authEnabled    bool
	authUsername   string
	authPassword   string
	sessionSecret  []byte
	configDir      string
	version        string
	basePath       string
	statsClient    xray.StatsClient
	socks5PoolURL  string
	updateCheckURL string
	sessionTouches map[string]time.Time
	sessionTouchGC time.Time
	sessionTouchMu sync.Mutex
}
