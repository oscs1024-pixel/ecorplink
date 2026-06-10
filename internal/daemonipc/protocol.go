package daemonipc

// Cmd is a command sent from GUI to daemon over the Unix socket.
type Cmd struct {
	Action            string `json:"action"`
	Company           string `json:"company,omitempty"`
	CodeType          string `json:"code_type,omitempty"`
	Account           string `json:"account,omitempty"`
	Code              string `json:"code,omitempty"`
	Token             string `json:"token,omitempty"`
	NodeID            int    `json:"node_id,omitempty"`
	FollowSplitRoutes bool   `json:"follow_split_routes,omitempty"`
}

// Response is the daemon's reply.
type Response struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

// VPNNodeDTO is the wire format for a VPN node.
type VPNNodeDTO struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	LatencyMs    int    `json:"latency_ms"`
	ProtocolMode int    `json:"protocol_mode"`
}

// VPNStatusDTO describes current VPN state.
type VPNStatusDTO struct {
	Connected    bool   `json:"connected"`
	Reconnecting bool   `json:"reconnecting,omitempty"`
	NodeName     string `json:"node_name,omitempty"`
	VpnIP        string `json:"vpn_ip,omitempty"`
	DNS          string `json:"dns,omitempty"`
	Protocol     string `json:"protocol,omitempty"`     // "TCP" or "UDP"
	ConnectedAt  int64  `json:"connected_at,omitempty"` // unix timestamp
}

// QRCodeDTO carries the QR login URL and token.
type QRCodeDTO struct {
	LoginURL string `json:"login_url"`
	Token    string `json:"token"`
}

// VPNStatsDTO holds cumulative WireGuard byte counters.
type VPNStatsDTO struct {
	TxBytes int64 `json:"tx_bytes"`
	RxBytes int64 `json:"rx_bytes"`
}

// Action constants.
const (
	ActionDiscover             = "discover"
	ActionLoginMethods         = "login_methods"
	ActionSendCode             = "send_code"
	ActionVerifyCode           = "verify_code"
	ActionGetQRCode            = "get_qrcode"
	ActionPollQR               = "poll_qr"
	ActionLogout               = "logout"
	ActionListNodes            = "list_nodes"
	ActionPingNodes            = "ping_nodes"
	ActionConnect              = "connect"
	ActionDisconnect           = "disconnect"
	ActionStatus               = "status"
	ActionGetStats             = "get_stats"
	ActionPingSingle           = "ping_single"
	ActionIsAuthenticated      = "is_authenticated"
	ActionCleanupRoutes        = "cleanup_routes"
	ActionSetFollowSplitRoutes = "set_follow_split_routes"
	ActionShutdown             = "shutdown"
	ActionReloadConfig         = "reload_config"
)
