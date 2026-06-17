export interface Session {
  auth_enabled: boolean;
  authenticated: boolean;
  username: string;
  revoked: boolean;
  default_password: boolean;
}

export interface Client {
  id: number;
  inbound_id: number;
  email: string;
  uuid: string;
  credential_id?: string;
  password?: string;
  subscription_token?: string;
  enabled: boolean;
  up?: number;
  down?: number;
  traffic_limit?: number;
  expiry_at?: number;
  xray_up?: number;
  xray_down?: number;
  rate_up?: number;
  rate_down?: number;
  traffic_status?: string;
  traffic_message?: string;
  traffic_stats_source?: string;
  realtime_stats_source?: string;
  traffic_stats_note?: string;
}

export interface Inbound {
  id: number;
  remark: string;
  protocol: string;
  core?: 'xray' | 'sing-box' | string;
  port: number;
  network: string;
  security: string;
  enabled: boolean;
  uuid?: string;
  clients?: Client[];
  traffic_up?: number;
  traffic_down?: number;
  traffic_total?: number;
  rate_up?: number;
  rate_down?: number;
  traffic_status?: string;
  traffic_message?: string;
  traffic_stats_source?: string;
  realtime_stats_source?: string;
  client_traffic?: Record<string, { up: number; down: number; rate_up?: number; rate_down?: number; xray_up?: number; xray_down?: number; status?: string; message?: string; source?: string; realtime_source?: string; note?: string }>;
  [key: string]: unknown;
}

export interface InboundCapability {
  protocol: string;
  core: 'xray' | 'sing-box' | string;
  template_id?: string;
  template_label?: string;
  template_summary?: string;
  networks: string[];
  securities: string[];
  default_network: string;
  default_security: string;
  security_by_network: Record<string, string[]>;
  visible_fields?: string[];
  auto_generate_fields?: string[];
  expert_fields?: string[];
  advanced_fields?: string[];
  credential_type: 'none' | 'uuid' | 'password' | 'credential_id_password' | 'username_password' | string;
  subscription: 'none' | 'full' | string;
  share_link?: boolean;
  local_proxy_inbound?: boolean;
  unsupported_reasons?: string[];
}

export interface Outbound {
  id: number;
  tag: string;
  remark?: string;
  protocol: string;
  address?: string;
  port?: number;
  username?: string;
  password?: string;
  supported_cores?: Array<'xray' | 'sing-box' | string>;
  enabled: boolean;
  sort?: number;
  [key: string]: unknown;
}

export interface RoutingRule {
  id: number;
  remark?: string;
  inbound_tag?: string;
  client_id?: number;
  client_email?: string;
  client_label?: string;
  domain?: string;
  ip?: string;
  rule_set?: string;
  protocol?: string;
  outbound_id?: number;
  outbound_tag: string;
  enabled: boolean;
  sort_order?: number;
  [key: string]: unknown;
}

export interface Resources {
  cpu_percent: number;
  memory_total: number;
  memory_used: number;
  memory_percent: number;
  disk_total: number;
  disk_used: number;
  disk_percent: number;
  uptime_seconds: number;
}

export interface CoreStatus {
  service: string;
  status: string;
  managed?: boolean;
  installed?: boolean;
  version?: string;
  memory_rss_bytes?: number;
  uptime?: string;
  active_connections?: number;
  config_path?: string;
  commands_executed?: string[];
}

export interface CoreActionResponse {
  core?: string;
  status?: string;
  output?: string;
  commands_executed?: string[];
  xray?: {
    status?: string;
    service?: string;
    commands_executed?: string[];
    error_output?: string;
  };
  singbox?: {
    applied?: boolean;
    reason?: string;
    error?: string;
    output?: string;
    commands_executed?: string[];
    inbounds?: number;
  };
  applied?: boolean;
  reason?: string;
  error?: string;
  inbounds?: number;
}

export interface ConfigValidation {
  target: 'xray' | 'singbox';
  valid: boolean;
  error?: string;
  warnings?: string[];
  inbounds?: number;
  outbounds?: number;
  rules?: number;
}

export interface DashboardSummary {
  generated_at: string;
  counts: {
    inbounds: number;
    inbounds_enabled: number;
    clients: number;
    clients_active: number;
    clients_expired: number;
    clients_limited: number;
    outbounds: number;
    outbounds_enabled: number;
    routing_rules: number;
    routing_enabled: number;
  };
  traffic: {
    up: number;
    down: number;
    total: number;
    xray_up: number;
    xray_down: number;
    xray_realtime: number;
  };
  traffic_rates?: {
    rate_up: number;
    rate_down: number;
    rate_total: number;
  };
  traffic_status?: {
    overall: string;
    engines?: Record<string, string>;
  };
  protocols: Record<string, number>;
  traffic_series: Array<{
    name: string;
    time?: string;
    up: number;
    down: number;
    rate_up?: number;
    rate_down?: number;
  }>;
  validation: {
    xray: ConfigValidation;
    singbox: ConfigValidation;
  };
}

export interface TrafficCoverage {
  overall: string;
  ok?: number;
  partial?: number;
  unsupported?: number;
  not_configured?: number;
  unavailable?: number;
  waiting?: number;
  engines?: Record<string, string>;
}

export interface TrafficSummary {
  total_up: number;
  total_down: number;
  total: number;
  rate_up: number;
  rate_down: number;
  rate_total: number;
  status: TrafficCoverage;
  last_updated_at: string;
}

export interface TrafficInbound {
  id: number;
  remark: string;
  protocol: string;
  port: number;
  total_up: number;
  total_down: number;
  total: number;
  rate_up: number;
  rate_down: number;
  status: string;
  message?: string;
  engine?: string;
  last_updated_at: string;
}

export interface TrafficClient {
  id: number;
  inbound_id: number;
  email: string;
  protocol: string;
  total_up: number;
  total_down: number;
  total: number;
  rate_up: number;
  rate_down: number;
  traffic_limit: number;
  expiry_at: number;
  status: string;
  message?: string;
  engine?: string;
  last_updated_at: string;
}

export interface TrafficSeriesPoint {
  name: string;
  time?: string;
  up: number;
  down: number;
  rate_up?: number;
  rate_down?: number;
}

export interface VersionResponse {
  version: string;
}

export interface Settings {
  panel_port?: number;
  panel_username?: string;
  panel_password?: string;
  web_base_path?: string;
  database_path?: string;
  xray_config_path?: string;
  cert_domain?: string;
  cert_email?: string;
  has_password?: boolean;
  [key: string]: unknown;
}

export interface UpdateCheck {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  release_url: string;
  release_name?: string;
  status: string;
  message?: string;
}

export interface UpdateStatus {
  status: string;
  current_version?: string;
  target_version?: string;
  message?: string;
  started_at?: string;
  updated_at?: string;
}

export interface SessionInfo {
  id: number;
  id_prefix: string;
  created_at: string;
  last_used: string;
  expires_at: string;
}

export interface CertStatus {
  domain: string;
  email: string;
  issued: boolean;
  cert_path: string;
  key_path: string;
}

export interface ProxyPoolRegion {
  code: string;
  name: string;
  count: number;
}

export interface ProxyPoolProxy {
  protocol?: string;
  address: string;
  port: number;
  username?: string;
  password?: string;
  country_code: string;
  country: string;
  city: string;
  asn: string;
  organization: string;
  latitude?: number;
  longitude?: number;
  latency?: number;
}

export interface ProxyPoolResponse {
  regions: ProxyPoolRegion[];
  proxies: ProxyPoolProxy[];
  cache_status: string;
  cache_updated_at: string;
  next_refresh_at: string;
}

export type Socks5PoolRegion = ProxyPoolRegion;
export type Socks5PoolProxy = ProxyPoolProxy;
export type Socks5PoolResponse = ProxyPoolResponse;

export interface PingResult {
  latency: number;
  method?: string;
  error?: string;
}
