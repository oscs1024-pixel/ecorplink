export type DaemonState = 'running' | 'stopped' | 'starting' | 'stopping' | 'error' | 'unknown'

export interface RuleConfig {
  id?: string
  enabled: boolean
  type: 'DOMAIN' | 'DOMAIN-SUFFIX' | 'DOMAIN-KEYWORD' | 'IP-CIDR' | 'GEOIP'
  value: string
  policy: 'DIRECT' | 'VPN'
  group?: string
  comment?: string
}

export interface ConfigDocument {
  tun: { name: string; ip: string; mask: number; mtu: number }
  fakeip: { pool: string }
  dns: { upstream: string[]; hijack: string[]; system_hijack: boolean }
  socks5: { enabled: boolean; bind_host: string; port: number }
  geoip: { file: string }
  direct_outbound: { interface: string }
  corplink: { company_name: string; insecure_skip_verify: boolean; debug_http_body: boolean }
  log: { level: string; file: string; max_size: string; max_age: number }
  rules: RuleConfig[]
}

export interface DaemonStatus {
  state: DaemonState
  pid?: number
  summary?: string
  error?: string
}

export interface AppState {
  daemon_path: string
  config_path: string
  log_path: string
  pid_path?: string
  status: DaemonStatus
}

export interface ConfigResponse {
  ok: boolean
  path: string
  config?: ConfigDocument
  error?: string
}

export interface CommandResult {
  ok: boolean
  summary?: string
  details?: string
  exit_code?: number
}

export interface WindowState {
  width: number
  height: number
  selected_node_id?: number
  selected_node_name?: string
  selected_node_latency?: number
  follow_split_routes?: boolean
}

export interface ValidationResult {
  ok: boolean
  errors?: string[]
}

export interface TestTarget {
  name: string
  url?: string
  domain?: string
  expected_policy?: string
  kind?: string
}

export interface TestResult {
  name: string
  url?: string
  domain?: string
  expected_policy?: string
  reachable: boolean
  http_status?: number
  remote_ip?: string
  duration_millis: number
  error?: string
}

export interface TestReport {
  ok: boolean
  results: TestResult[]
}

export interface LogChunk {
  ok: boolean
  path: string
  text: string
  error?: string
}

export interface LaunchServiceRequest {
  label: string
  binary_path: string
  config_path: string
  work_dir: string
}

export interface ServiceStatus {
  installed: boolean
  running: boolean
  needs_update?: boolean
  details?: string
  error?: string
}

// New types for corplink integration
export interface LoginMethodsResult { ok: boolean; methods?: string[]; error?: string }
export interface QRCodeResult { ok: boolean; login_url?: string; token?: string; error?: string }
export interface VPNNode { id: number; name: string; latency_ms: number; protocol_mode: number }
export interface VPNNodesResult { ok: boolean; nodes?: VPNNode[]; error?: string }
export interface VPNStatusResult { ok: boolean; connected: boolean; reconnecting?: boolean; node_name?: string; vpn_ip?: string; dns?: string; protocol?: string; connected_at?: number; error?: string }
export interface VPNStatsResult { ok: boolean; tx_bytes: number; rx_bytes: number }
