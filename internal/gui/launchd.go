package gui

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (s *Service) GetLaunchServiceStatus() ServiceStatus {
	switch runtime.GOOS {
	case "darwin":
		return s.getDarwinLaunchServiceStatus()
	case "linux":
		return s.getLinuxServiceStatus()
	case "windows":
		return s.getWindowsServiceStatus()
	default:
		return ServiceStatus{Installed: false, Running: false, Error: "service management is not supported on this platform"}
	}
}

func (s *Service) getDarwinLaunchServiceStatus() ServiceStatus {
	plistPath := "/Library/LaunchDaemons/com.ecorplink.daemon.plist"
	_, err := os.Stat(plistPath)
	installed := err == nil

	if !installed {
		return ServiceStatus{Installed: false, Running: false}
	}

	// Service is installed; check running state via launchctl print.
	res := s.runner.Run(nilContext(), RunRequest{
		Path: "launchctl",
		Args: []string{"print", "system/com.ecorplink.daemon"},
	})
	running := res.OK

	// NeedsUpdate: check if the installed plist references the current daemon helper.
	needsUpdate := false
	if s.daemonPath != "" {
		if data, err2 := os.ReadFile(plistPath); err2 == nil {
			needsUpdate = !bytes.Contains(data, []byte(s.daemonPath))
		}
	}

	return ServiceStatus{
		Installed:   true,
		Running:     running,
		NeedsUpdate: needsUpdate,
		Details:     res.Stdout + res.Stderr,
	}
}

func (s *Service) InstallLaunchService(req LaunchServiceRequest) CommandResult {
	if req.Label == "" {
		req.Label = "com.ecorplink.daemon"
	}
	if req.BinaryPath == "" {
		req.BinaryPath = s.daemonPath
	}
	if req.ConfigPath == "" {
		req.ConfigPath = s.configPath
	}
	if req.WorkDir == "" {
		req.WorkDir = defaultEcorplinkDir()
	}
	switch runtime.GOOS {
	case "darwin":
		return s.installDarwinLaunchService(req)
	case "linux":
		return s.privilegedCommand("install-service", "-c", req.ConfigPath, "--pid-file", s.pidPath)
	case "windows":
		return s.privilegedCommand("install-service", "-c", req.ConfigPath, "--pid-file", s.pidPath)
	default:
		return CommandResult{OK: false, Summary: "service install is not supported on this platform"}
	}
}

func (s *Service) installDarwinLaunchService(req LaunchServiceRequest) CommandResult {
	plist := LaunchDaemonPlist(req)
	plistPath := "/Library/LaunchDaemons/com.ecorplink.daemon.plist"
	if req.Label != "" {
		plistPath = "/Library/LaunchDaemons/" + req.Label + ".plist"
	}
	script := fmt.Sprintf(
		// Bootout first (ignore errors), then write plist, bootstrap and kickstart.
		"launchctl bootout system/%s 2>/dev/null; true && printf %%s %s > %s && chown root:wheel %s && chmod 644 %s && launchctl bootstrap system %s && launchctl enable system/%s && launchctl kickstart -k system/%s",
		shellQuote(req.Label),
		shellQuote(plist),
		shellQuote(plistPath),
		shellQuote(plistPath),
		shellQuote(plistPath),
		shellQuote(plistPath),
		shellQuote(req.Label),
		shellQuote(req.Label),
	)
	osa := fmt.Sprintf("do shell script %s with administrator privileges", appleScriptQuote(script))
	res := s.runner.Run(nilContext(), RunRequest{Path: "osascript", Args: []string{"-e", osa}})
	return commandResult(res)
}

func (s *Service) UninstallLaunchService() CommandResult {
	switch runtime.GOOS {
	case "darwin":
		return s.uninstallDarwinLaunchService("com.ecorplink.daemon")
	case "linux", "windows":
		return s.privilegedCommand("uninstall-service")
	default:
		return CommandResult{OK: false, Summary: "service uninstall is not supported on this platform"}
	}
}

func (s *Service) uninstallDarwinLaunchService(label string) CommandResult {
	if label == "" {
		label = "com.ecorplink.daemon"
	}
	plistPath := "/Library/LaunchDaemons/" + label + ".plist"
	script := fmt.Sprintf(
		"launchctl bootout system/%s 2>/dev/null; true && rm -f %s",
		label,
		plistPath,
	)
	osa := fmt.Sprintf("do shell script %s with administrator privileges", appleScriptQuote(script))
	res := s.runner.Run(nilContext(), RunRequest{Path: "osascript", Args: []string{"-e", osa}})
	return commandResult(res)
}

func (s *Service) getLinuxServiceStatus() ServiceStatus {
	unitPath := "/etc/systemd/system/com.ecorplink.daemon.service"
	_, err := os.Stat(unitPath)
	installed := err == nil
	if !installed {
		return ServiceStatus{Installed: false, Running: false}
	}
	res := s.runner.Run(nilContext(), RunRequest{Path: "systemctl", Args: []string{"is-active", "--quiet", "com.ecorplink.daemon.service"}})
	needsUpdate := false
	if s.daemonPath != "" {
		if data, err := os.ReadFile(unitPath); err == nil {
			needsUpdate = !bytes.Contains(data, []byte(s.daemonPath))
		}
	}
	return ServiceStatus{Installed: true, Running: res.OK, NeedsUpdate: needsUpdate, Details: res.Stdout + res.Stderr}
}

func (s *Service) getWindowsServiceStatus() ServiceStatus {
	res := s.runner.Run(nilContext(), RunRequest{Path: "sc", Args: []string{"query", "ecorplink"}})
	out := res.Stdout + res.Stderr
	if !res.OK {
		return ServiceStatus{Installed: false, Running: false, Details: out}
	}
	return ServiceStatus{Installed: true, Running: strings.Contains(out, "RUNNING"), Details: out}
}

func LaunchDaemonPlist(req LaunchServiceRequest) string {
	if req.Label == "" {
		req.Label = "com.ecorplink.daemon"
	}
	pidFile := req.WorkDir + "/ecorplink.pid"
	// HOME is derived from WorkDir parent so root-run daemon finds the right home.
	home := filepath.Dir(req.WorkDir)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
      <string>%s</string>
      <string>-c</string>
      <string>%s</string>
      <string>--pid-file</string>
      <string>%s</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
      <key>ECORPLINK_DAEMON</key>
      <string>1</string>
      <key>HOME</key>
      <string>%s</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>StandardOutPath</key>
    <string>/var/log/ecorplink.out.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/ecorplink.err.log</string>
  </dict>
</plist>
`, html.EscapeString(req.Label), html.EscapeString(req.BinaryPath), html.EscapeString(req.ConfigPath), html.EscapeString(pidFile), html.EscapeString(home), html.EscapeString(req.WorkDir))
}

func nilContext() context.Context {
	return context.Background()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func appleScriptQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}
