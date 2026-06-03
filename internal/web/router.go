package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/imzyb/MiGate/internal/db"
)

type Store interface {
	ListInbounds(ctx context.Context) ([]db.Inbound, error)
	CreateInbound(ctx context.Context, params db.CreateInboundParams) (db.Inbound, error)
}

type routerConfig struct {
	store Store
}

type Option func(*routerConfig)

func WithStore(store Store) Option {
	return func(cfg *routerConfig) {
		cfg.store = store
	}
}

func NewRouter(options ...Option) http.Handler {
	cfg := routerConfig{}
	for _, option := range options {
		option(&cfg)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", panelHandler)
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/inbounds", inboundsHandler(cfg.store))
	return mux
}

func panelHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(panelHTML))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","mode":"go-lite"}`))
}

func inboundsHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listInbounds(w, r, store)
		case http.MethodPost:
			createInbound(w, r, store)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func listInbounds(w http.ResponseWriter, r *http.Request, store Store) {
	inbounds := []db.Inbound{}
	if store != nil {
		loaded, err := store.ListInbounds(r.Context())
		if err != nil {
			http.Error(w, `{"error":"list_inbounds_failed"}`, http.StatusInternalServerError)
			return
		}
		inbounds = loaded
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"inbounds": inbounds})
}

func createInbound(w http.ResponseWriter, r *http.Request, store Store) {
	if store == nil {
		http.Error(w, `{"error":"store_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	var payload db.CreateInboundParams
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	created, err := store.CreateInbound(r.Context(), payload)
	if err != nil {
		http.Error(w, `{"error":"unsupported_protocol"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

const panelHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>MiGate Go Lite</title>
  <style>
    :root { color-scheme: dark; --bg:#070b14; --card:#101827; --muted:#94a3b8; --text:#e5eefb; --line:#223047; --accent:#4f8cff; --accent2:#22c55e; --danger:#ef4444; }
    * { box-sizing: border-box; }
    body { margin:0; min-height:100vh; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: radial-gradient(circle at 20% 10%, rgba(79,140,255,.24), transparent 36%), radial-gradient(circle at 80% 0%, rgba(34,197,94,.14), transparent 30%), var(--bg); color:var(--text); }
    .shell { display:grid; grid-template-columns: 240px 1fr; min-height:100vh; }
    aside { border-right:1px solid var(--line); padding:24px 18px; background:rgba(7,11,20,.74); backdrop-filter: blur(18px); }
    .brand { font-size:24px; font-weight:800; letter-spacing:.4px; margin-bottom:4px; }
    .brand span { color:var(--accent); }
    .subtitle { color:var(--muted); font-size:13px; margin-bottom:28px; }
    nav a { display:block; color:var(--text); text-decoration:none; padding:11px 12px; border-radius:12px; margin:6px 0; border:1px solid transparent; }
    nav a.active, nav a:hover { background:rgba(79,140,255,.13); border-color:rgba(79,140,255,.25); }
    main { padding:28px; }
    .hero { display:flex; justify-content:space-between; gap:20px; align-items:flex-start; margin-bottom:22px; }
    h1 { margin:0 0 8px; font-size:32px; }
    p { color:var(--muted); line-height:1.6; }
    .badge { display:inline-flex; align-items:center; gap:8px; padding:8px 12px; border-radius:999px; background:rgba(34,197,94,.12); color:#bbf7d0; border:1px solid rgba(34,197,94,.24); font-size:13px; }
    .grid { display:grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap:16px; margin-bottom:18px; }
    .card { background:linear-gradient(180deg, rgba(16,24,39,.92), rgba(12,18,30,.92)); border:1px solid var(--line); border-radius:18px; padding:18px; box-shadow:0 18px 60px rgba(0,0,0,.22); }
    .metric { font-size:26px; font-weight:800; margin-top:10px; }
    .section-title { font-size:18px; font-weight:750; margin:0 0 12px; }
    .protocols { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:12px; }
    .protocol { padding:14px; border-radius:16px; background:rgba(148,163,184,.08); border:1px solid rgba(148,163,184,.14); }
    .protocol strong { display:block; margin-bottom:6px; }
    .actions { display:flex; gap:10px; flex-wrap:wrap; margin-top:14px; }
    button { background:linear-gradient(135deg,var(--accent),#7c3aed); border:none; color:white; padding:10px 14px; border-radius:12px; font-weight:700; cursor:pointer; }
    button.secondary { background:rgba(148,163,184,.12); border:1px solid var(--line); }
    @media (max-width: 900px) { .shell { grid-template-columns:1fr; } aside { border-right:0; border-bottom:1px solid var(--line); } .grid,.protocols { grid-template-columns:1fr 1fr; } }
    @media (max-width: 560px) { .grid,.protocols { grid-template-columns:1fr; } main { padding:18px; } }
  </style>
</head>
<body>
  <div class="shell">
    <aside>
      <div class="brand">MiGate <span>Go Lite</span></div>
      <div class="subtitle">轻量面板 风格单二进制面板</div>
      <nav>
        <a class="active" href="/">概览</a>
        <a href="/#inbounds">入站</a>
        <a href="/#clients">客户端</a>
        <a href="/#subscriptions">订阅</a>
        <a href="/#xray">Xray</a>
      </nav>
    </aside>
    <main>
      <section class="hero">
        <div>
          <h1>MiGate Go Lite</h1>
          <p>从零重写为轻量 Go 单二进制：SQLite、本地 Xray 配置、订阅链接与核心面板能力。</p>
        </div>
        <div class="badge">● 服务在线</div>
      </section>
      <section class="grid" aria-label="概览指标">
        <div class="card"><div>入站</div><div class="metric">0</div><p>VLESS / VMess / Trojan / Shadowsocks</p></div>
        <div class="card"><div>客户端</div><div class="metric">0</div><p>按 inbound 管理账号</p></div>
        <div class="card"><div>订阅</div><div class="metric">Ready</div><p>Clash / 通用链接规划中</p></div>
        <div class="card"><div>Xray</div><div class="metric">Direct</div><p>默认 freedom 出站</p></div>
      </section>
      <section id="inbounds" class="card">
        <h2 class="section-title">核心协议</h2>
        <div class="protocols">
          <div class="protocol"><strong>VLESS</strong><span>Reality / TLS 入站</span></div>
          <div class="protocol"><strong>VMess</strong><span>WebSocket / TLS 兼容</span></div>
          <div class="protocol"><strong>Trojan</strong><span>TLS 节点支持</span></div>
          <div class="protocol"><strong>Shadowsocks</strong><span>轻量转发协议</span></div>
        </div>
        <div class="actions">
          <button>新增入站</button>
          <button class="secondary">生成 Xray 配置</button>
          <button class="secondary">查看订阅</button>
        </div>
      </section>
    </main>
  </div>
</body>
</html>`
