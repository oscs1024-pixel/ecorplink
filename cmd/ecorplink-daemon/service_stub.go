//go:build !windows && !linux

package main

import "fmt"

func isWindowsService() bool {
	return false
}

func runService() error {
	return fmt.Errorf("service mode is only supported on Windows")
}

func installService(configPath, pidFile string) error {
	return fmt.Errorf("service install is only supported on Windows")
}

func uninstallService() error {
	return fmt.Errorf("service uninstall is only supported on Windows")
}
