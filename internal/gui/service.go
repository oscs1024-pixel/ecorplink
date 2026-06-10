package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"ecorplink/internal/config"
	"ecorplink/internal/daemonipc"
)

type Service struct {
	daemonPath string
	configPath string
	logPath    string
	pidPath    string
	workDir    string
	appVersion string
	runner     Runner
}

func NewService(opts Options) *Service {
	if opts.DaemonPath == "" {
		opts.DaemonPath = filepath.Join(defaultEcorplinkDir(), "bin", executableName("ecorplink-daemon"))
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = filepath.Join(defaultEcorplinkDir(), "config.json")
	}
	if opts.LogPath == "" {
		opts.LogPath = filepath.Join(defaultEcorplinkDir(), "ecorplink.log")
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{Timeout: 15 * time.Second}
	}
	return &Service{
		daemonPath: opts.DaemonPath,
		configPath: opts.ConfigPath,
		logPath:    opts.LogPath,
		pidPath:    filepath.Join(defaultEcorplinkDir(), "ecorplink.pid"),
		workDir:    opts.WorkDir,
		appVersion: opts.AppVersion,
		runner:     opts.Runner,
	}
}

// GetVersion returns the application version injected at build time.
// The actual value is set via the AppVersion field on Options.
func (s *Service) GetVersion() string {
	return s.appVersion
}

// GetAppState returns current binary/config/log paths and daemon status.
func (s *Service) GetAppState() AppState {
	status := s.GetDaemonStatus()
	return AppState{
		DaemonPath: s.daemonPath,
		ConfigPath: s.configPath,
		LogPath:    s.logPath,
		PidPath:    s.pidPath,
		Status:     status,
	}
}

func (s *Service) LoadConfig(path string) ConfigDocument {
	path = s.configPathOr(path)
	cfg, err := config.LoadConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg = config.DefaultConfig()
			if res := s.SaveConfig(path, cfg); !res.OK {
				return ConfigDocument{OK: false, Path: path, Error: strings.Join(res.Errors, "\n")}
			}
			return ConfigDocument{OK: true, Path: path, Config: cfg}
		}
		return ConfigDocument{OK: false, Path: path, Error: err.Error()}
	}
	return ConfigDocument{OK: true, Path: path, Config: cfg}
}

func (s *Service) SaveConfig(path string, cfg *config.Config) ValidationResult {
	if cfg == nil {
		return ValidationResult{OK: false, Errors: []string{"config is nil"}}
	}
	if res := s.ValidateConfig(cfg); !res.OK {
		return res
	}
	path = s.configPathOr(path)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return ValidationResult{OK: false, Errors: []string{err.Error()}}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return ValidationResult{OK: false, Errors: []string{err.Error()}}
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return ValidationResult{OK: false, Errors: []string{err.Error()}}
	}
	return ValidationResult{OK: true}
}

func (s *Service) ValidateConfig(cfg *config.Config) ValidationResult {
	if cfg == nil {
		return ValidationResult{OK: false, Errors: []string{"config is nil"}}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return ValidationResult{OK: false, Errors: []string{err.Error()}}
	}
	if _, err := config.LoadConfigFromBytes(data); err != nil {
		return ValidationResult{OK: false, Errors: []string{err.Error()}}
	}
	return ValidationResult{OK: true}
}

// ApplyRecommendedRules replaces the rules in the config at path with the
// built-in recommended rule set, then saves the file.
func (s *Service) ApplyRecommendedRules(path string) CommandResult {
	path = s.configPathOr(path)
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return CommandResult{OK: false, Summary: "加载配置失败: " + err.Error()}
	}
	rules, err := config.RecommendedRules()
	if err != nil {
		return CommandResult{OK: false, Summary: "读取推荐规则失败: " + err.Error()}
	}
	cfg.Rules = rules
	res := s.SaveConfig(path, cfg)
	if !res.OK {
		return CommandResult{OK: false, Summary: "保存配置失败: " + strings.Join(res.Errors, "; ")}
	}
	return CommandResult{OK: true, Summary: "已应用推荐规则"}
}

func (s *Service) StartDaemon(path string) CommandResult {
	path = s.configPathOr(path)
	if status := s.GetDaemonStatus(); status.State == "running" {
		return CommandResult{OK: true, Summary: status.Summary}
	}
	res := s.privilegedCommand("-c", path, "--pid-file", s.pidPath)
	res = normalizeDaemonCommandResult(res)
	if !res.OK {
		return res
	}
	if status := s.waitForDaemonRunning(5 * time.Second); status.State == "running" {
		return CommandResult{OK: true, Summary: status.Summary, Details: res.Details}
	}
	res.OK = false
	res.Summary = "daemon did not become running"
	res.Details = strings.TrimSpace(res.Details + "\n\n" + s.GetDaemonStatus().Summary)
	return res
}

func (s *Service) StopDaemon() CommandResult {
	return s.privilegedCommand("stop", "--pid-file", s.pidPath)
}

func (s *Service) RestartDaemon(path string) CommandResult {
	stop := s.StopDaemon()
	if !stop.OK && !strings.Contains(strings.ToLower(stop.Summary+stop.Details), "daemon not running") {
		return stop
	}
	return s.StartDaemon(path)
}

func (s *Service) GetDaemonStatus() DaemonStatus {
	if pid, err := readPidFile(s.pidPath); err == nil {
		if isProcessRunning(pid) {
			return DaemonStatus{
				State:   "running",
				PID:     pid,
				Summary: fmt.Sprintf("daemon running (pid %d)", pid),
			}
		}
		return DaemonStatus{State: "stopped", Summary: fmt.Sprintf("stale pid file: process %d is not running", pid)}
	}

	return DaemonStatus{State: "stopped", Summary: "daemon not running"}
}

func (s *Service) command(args ...string) CommandResult {
	return s.commandPath(s.daemonPath, args...)
}

func (s *Service) privilegedCommand(args ...string) CommandResult {
	if runtime.GOOS != "darwin" {
		return s.command(args...)
	}
	command := shellQuote(s.daemonPath)
	for _, arg := range args {
		command += " " + shellQuote(arg)
	}
	if s.workDir != "" {
		command = "cd " + shellQuote(s.workDir) + " && " + command
	}
	osa := fmt.Sprintf("do shell script %s with administrator privileges", appleScriptQuote(command))
	res := s.runner.Run(nilContext(), RunRequest{
		Path: "osascript",
		Args: []string{"-e", osa},
		Dir:  s.workDir,
	})
	return commandResult(res)
}

func (s *Service) commandPath(path string, args ...string) CommandResult {
	res := s.runner.Run(context.Background(), RunRequest{
		Path: path,
		Args: args,
		Dir:  s.workDir,
	})
	return commandResult(res)
}

func commandResult(res RunResult) CommandResult {
	output := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
	details := commandDetails(res)
	if res.OK {
		return CommandResult{OK: true, Summary: output, Details: details}
	}
	summary := firstOutputLine(res.Stderr)
	if summary == "" {
		summary = firstOutputLine(res.Stdout)
	}
	if res.TimedOut {
		summary = "command timed out"
	}
	if summary == "" {
		summary = res.Error
	}
	if summary == "" && res.ExitCode != 0 {
		summary = fmt.Sprintf("command failed with exit code %d", res.ExitCode)
	}
	return CommandResult{OK: false, Summary: summary, Details: details, ExitCode: res.ExitCode}
}

func commandDetails(res RunResult) string {
	var parts []string
	if stdout := strings.TrimSpace(res.Stdout); stdout != "" {
		parts = append(parts, "stdout:\n"+stdout)
	}
	if stderr := strings.TrimSpace(res.Stderr); stderr != "" {
		parts = append(parts, "stderr:\n"+stderr)
	}
	if res.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit code: %d", res.ExitCode))
	}
	if res.TimedOut {
		parts = append(parts, "timeout: true")
	}
	if errText := strings.TrimSpace(res.Error); errText != "" {
		parts = append(parts, "error: "+errText)
	}
	return strings.Join(parts, "\n\n")
}

func firstOutputLine(text string) string {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func normalizeDaemonCommandResult(res CommandResult) CommandResult {
	if res.OK {
		return res
	}
	text := strings.ToLower(res.Summary + "\n" + res.Details)
	if strings.Contains(text, "daemon already running") {
		res.OK = true
		res.ExitCode = 0
	}
	return res
}

func (s *Service) waitForDaemonRunning(timeout time.Duration) DaemonStatus {
	deadline := time.Now().Add(timeout)
	for {
		status := s.GetDaemonStatus()
		if status.State == "running" {
			return status
		}
		if time.Now().After(deadline) {
			return status
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Service) configPathOr(path string) string {
	if path != "" {
		return path
	}
	return s.configPath
}

func readPidFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return 0, err
	}
	return pid, nil
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func defaultEcorplinkDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".ecorplink")
	}
	return filepath.Join(home, ".ecorplink")
}

func executableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

var _ = fmt.Sprintf

func (s *Service) socketPath() string {
	return filepath.Join(defaultEcorplinkDir(), "daemon.sock")
}

func (s *Service) sendCmd(cmd daemonipc.Cmd) (*daemonipc.Response, error) {
	cl := daemonipc.NewClient(s.socketPath())
	return cl.Send(cmd)
}

func socketResult(resp *daemonipc.Response, err error) CommandResult {
	if err != nil {
		return CommandResult{OK: false, Summary: "daemon unreachable: " + err.Error()}
	}
	if !resp.OK {
		return CommandResult{OK: false, Summary: resp.Error}
	}
	return CommandResult{OK: true}
}

func remarshal(src, dst any) {
	b, _ := json.Marshal(src)
	json.Unmarshal(b, dst) //nolint:errcheck
}

// DiscoverCompany calls the daemon to discover company server URL.
func (s *Service) DiscoverCompany(company string) CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionDiscover, Company: company})
	return socketResult(resp, err)
}

// GetLoginMethods returns available login method names and verify types from the daemon.
func (s *Service) GetLoginMethods() LoginMethodsResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionLoginMethods})
	if err != nil {
		return LoginMethodsResult{OK: false, Error: err.Error()}
	}
	if !resp.OK {
		return LoginMethodsResult{OK: false, Error: resp.Error}
	}
	var info struct {
		Methods     []string `json:"methods"`
		VerifyTypes []string `json:"verify_types"`
	}
	remarshal(resp.Data, &info)
	return LoginMethodsResult{OK: true, Methods: info.Methods, VerifyTypes: info.VerifyTypes}
}

// LoginWithPassword performs password-based login via the daemon.
func (s *Service) LoginWithPassword(account, password string) CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionLoginPassword, Account: account, Password: password})
	return socketResult(resp, err)
}

// SendVerifyCode sends a verification code via the daemon.
func (s *Service) SendVerifyCode(codeType, account string) CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionSendCode, CodeType: codeType, Account: account})
	return socketResult(resp, err)
}

// VerifyCode submits a verification code via the daemon.
func (s *Service) VerifyCode(codeType, account, code string) CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionVerifyCode, CodeType: codeType, Account: account, Code: code})
	return socketResult(resp, err)
}

// GetQRCode fetches QR login URL and token from the daemon.
func (s *Service) GetQRCode() QRCodeResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionGetQRCode})
	if err != nil {
		return QRCodeResult{OK: false, Error: err.Error()}
	}
	if !resp.OK {
		return QRCodeResult{OK: false, Error: resp.Error}
	}
	var dto daemonipc.QRCodeDTO
	remarshal(resp.Data, &dto)
	return QRCodeResult{OK: true, LoginURL: dto.LoginURL, Token: dto.Token}
}

// PollQRStatus polls the daemon for QR login completion.
func (s *Service) PollQRStatus(token string) CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionPollQR, Token: token})
	return socketResult(resp, err)
}

// Logout logs out via the daemon.
func (s *Service) Logout() CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionLogout})
	return socketResult(resp, err)
}

// CleanupRoutes removes stale capture routes and resets system DNS.
// Safe to call at any time, including when VPN is not connected.
func (s *Service) CleanupRoutes() CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionCleanupRoutes})
	return socketResult(resp, err)
}

// Falls back to listing nodes when daemon doesn't support the action yet.
func (s *Service) IsAuthenticated() bool {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionIsAuthenticated})
	if err != nil {
		return false
	}
	// Old daemon returns ok=false with "unknown action" — fall back to node list check.
	if !resp.OK {
		fallback, ferr := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionListNodes})
		return ferr == nil && fallback.OK
	}
	v, _ := resp.Data.(bool)
	return v
}

// ListVPNNodes returns available VPN nodes from the daemon.
func (s *Service) ListVPNNodes() VPNNodesResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionListNodes})
	if err != nil {
		return VPNNodesResult{OK: false, Error: err.Error()}
	}
	if !resp.OK {
		return VPNNodesResult{OK: false, Error: resp.Error}
	}
	var nodes []daemonipc.VPNNodeDTO
	remarshal(resp.Data, &nodes)
	return VPNNodesResult{OK: true, Nodes: nodes}
}

// PingNodes returns VPN nodes with latency measurements.
func (s *Service) PingNodes() VPNNodesResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionPingNodes})
	if err != nil {
		return VPNNodesResult{OK: false, Error: err.Error()}
	}
	if !resp.OK {
		return VPNNodesResult{OK: false, Error: resp.Error}
	}
	var nodes []daemonipc.VPNNodeDTO
	remarshal(resp.Data, &nodes)
	return VPNNodesResult{OK: true, Nodes: nodes}
}

// PingSingleNode pings one VPN node by ID and returns its latency.
func (s *Service) PingSingleNode(nodeID int) VPNNodesResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionPingSingle, NodeID: nodeID})
	if err != nil {
		return VPNNodesResult{OK: false, Error: "daemon unreachable: " + err.Error()}
	}
	if !resp.OK {
		return VPNNodesResult{OK: false, Error: resp.Error}
	}
	var node daemonipc.VPNNodeDTO
	remarshal(resp.Data, &node)
	return VPNNodesResult{OK: true, Nodes: []daemonipc.VPNNodeDTO{node}}
}

// SetFollowSplitRoutes switches between split-tunnel and full-tunnel mode at
// runtime. Takes effect immediately without reconnecting.
func (s *Service) SetFollowSplitRoutes(v bool) CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionSetFollowSplitRoutes, FollowSplitRoutes: v})
	return socketResult(resp, err)
}

// ReloadConfig asks the daemon to reload the saved config and apply supported
// runtime changes.
func (s *Service) ReloadConfig() CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionReloadConfig})
	return socketResult(resp, err)
}

// ConnectVPN connects to the specified VPN node via the daemon.
func (s *Service) ConnectVPN(nodeID int, followSplitRoutes bool) CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionConnect, NodeID: nodeID, FollowSplitRoutes: followSplitRoutes})
	return socketResult(resp, err)
}

// DisconnectVPN disconnects the active VPN connection via the daemon.
func (s *Service) DisconnectVPN() CommandResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionDisconnect})
	return socketResult(resp, err)
}

// GetVPNStatus returns the current VPN connection status from the daemon.
func (s *Service) GetVPNStatus() VPNStatusResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionStatus})
	if err != nil {
		return VPNStatusResult{OK: false, Error: "daemon unreachable: " + err.Error()}
	}
	if !resp.OK {
		return VPNStatusResult{OK: false, Error: resp.Error}
	}
	var st daemonipc.VPNStatusDTO
	remarshal(resp.Data, &st)
	return VPNStatusResult{OK: true, Connected: st.Connected, Reconnecting: st.Reconnecting, NodeName: st.NodeName, VpnIP: st.VpnIP, DNS: st.DNS, Protocol: st.Protocol, ConnectedAt: st.ConnectedAt}
}

// GetVPNStats returns cumulative WireGuard byte counters from the daemon.
func (s *Service) GetVPNStats() VPNStatsResult {
	resp, err := s.sendCmd(daemonipc.Cmd{Action: daemonipc.ActionGetStats})
	if err != nil {
		return VPNStatsResult{OK: false}
	}
	if !resp.OK {
		return VPNStatsResult{OK: false}
	}
	var st daemonipc.VPNStatsDTO
	remarshal(resp.Data, &st)
	return VPNStatsResult{OK: true, TxBytes: st.TxBytes, RxBytes: st.RxBytes}
}
