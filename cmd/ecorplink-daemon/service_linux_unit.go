package main

import (
	"fmt"
	"path/filepath"
	"strconv"
)

func linuxSystemdServiceUnit(exe, configPath, pidFile, workDir string) string {
	home := filepath.Dir(workDir)
	return fmt.Sprintf(`[Unit]
Description=ECorpLink daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=%s
Environment=%s
WorkingDirectory=%s
ExecStart=%s -c %s --pid-file %s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
`, systemdQuote("ECORPLINK_DAEMON=1"), systemdQuote("HOME="+home), systemdQuote(workDir), systemdQuote(exe), systemdQuote(configPath), systemdQuote(pidFile))
}

func systemdQuote(s string) string {
	return strconv.Quote(s)
}
