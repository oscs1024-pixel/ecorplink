package gui

import (
	"net/http"

	"ecorplink/internal/config"
	"ecorplink/internal/daemonipc"
)

type Options struct {
	DaemonPath string
	ConfigPath string
	LogPath    string
	WorkDir    string
	AppVersion string
	Runner     Runner
}

type AppState struct {
	DaemonPath string       `json:"daemon_path"`
	ConfigPath string       `json:"config_path"`
	LogPath    string       `json:"log_path"`
	PidPath    string       `json:"pid_path"`
	Status     DaemonStatus `json:"status"`
}

type ConfigDocument struct {
	OK     bool           `json:"ok"`
	Path   string         `json:"path"`
	Config *config.Config `json:"config,omitempty"`
	Error  string         `json:"error,omitempty"`
}

type ValidationResult struct {
	OK     bool     `json:"ok"`
	Errors []string `json:"errors,omitempty"`
}

type CommandResult struct {
	OK       bool   `json:"ok"`
	Summary  string `json:"summary,omitempty"`
	Details  string `json:"details,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

type DaemonStatus struct {
	State   string `json:"state"`
	PID     int    `json:"pid,omitempty"`
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

type TestRequest struct {
	TimeoutMillis int          `json:"timeout_millis,omitempty"`
	Items         []TestTarget `json:"items,omitempty"`
	HTTPClient    *http.Client `json:"-"`
}

type TestTarget struct {
	Name           string `json:"name"`
	URL            string `json:"url,omitempty"`
	Domain         string `json:"domain,omitempty"`
	ExpectedPolicy string `json:"expected_policy,omitempty"`
	Kind           string `json:"kind,omitempty"`
}

type TestReport struct {
	OK      bool         `json:"ok"`
	Results []TestResult `json:"results"`
}

type TestResult struct {
	Name           string `json:"name"`
	URL            string `json:"url,omitempty"`
	Domain         string `json:"domain,omitempty"`
	ExpectedPolicy string `json:"expected_policy,omitempty"`
	Reachable      bool   `json:"reachable"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	RemoteIP       string `json:"remote_ip,omitempty"`
	DurationMillis int64  `json:"duration_millis"`
	Error          string `json:"error,omitempty"`
}

type LogRequest struct {
	Lines int    `json:"lines,omitempty"`
	Query string `json:"query,omitempty"`
}

type LogChunk struct {
	OK    bool   `json:"ok"`
	Path  string `json:"path"`
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

type ServiceStatus struct {
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	NeedsUpdate bool   `json:"needs_update"` // installed but plist has stale binary path
	Details     string `json:"details,omitempty"`
	Error       string `json:"error,omitempty"`
}

type WindowState struct {
	Width               int    `json:"width"`
	Height              int    `json:"height"`
	SelectedNodeID      int    `json:"selected_node_id,omitempty"`
	SelectedNodeName    string `json:"selected_node_name,omitempty"`
	SelectedNodeLatency int    `json:"selected_node_latency,omitempty"`
	FollowSplitRoutes   *bool  `json:"follow_split_routes,omitempty"`
}

type LaunchServiceRequest struct {
	Label      string `json:"label"`
	BinaryPath string `json:"binary_path"`
	ConfigPath string `json:"config_path"`
	WorkDir    string `json:"work_dir"`
}

// LoginMethodsResult holds available login method names and verify types.
type LoginMethodsResult struct {
	OK          bool     `json:"ok"`
	Methods     []string `json:"methods,omitempty"`
	VerifyTypes []string `json:"verify_types,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// QRCodeResult holds QR login URL and token.
type QRCodeResult struct {
	OK       bool   `json:"ok"`
	LoginURL string `json:"login_url,omitempty"`
	Token    string `json:"token,omitempty"`
	Error    string `json:"error,omitempty"`
}

// VPNNodesResult holds a list of VPN nodes.
type VPNNodesResult struct {
	OK    bool                   `json:"ok"`
	Nodes []daemonipc.VPNNodeDTO `json:"nodes,omitempty"`
	Error string                 `json:"error,omitempty"`
}

// VPNStatusResult holds VPN connection status.
type VPNStatusResult struct {
	OK           bool   `json:"ok"`
	Connected    bool   `json:"connected"`
	Reconnecting bool   `json:"reconnecting,omitempty"`
	NodeName     string `json:"node_name,omitempty"`
	VpnIP        string `json:"vpn_ip,omitempty"`
	DNS          string `json:"dns,omitempty"`
	Protocol     string `json:"protocol,omitempty"`
	ConnectedAt  int64  `json:"connected_at,omitempty"`
	Error        string `json:"error,omitempty"`
}

// VPNStatsResult holds cumulative WireGuard byte counters.
type VPNStatsResult struct {
	OK      bool  `json:"ok"`
	TxBytes int64 `json:"tx_bytes"`
	RxBytes int64 `json:"rx_bytes"`
}
