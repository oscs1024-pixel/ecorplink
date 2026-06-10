//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ensureAdmin() error {
	if os.Getuid() == 0 {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("permission denied: ecorplink requires root privileges")
	}

	// Build a safe argument string
	var parts []string
	for i, a := range os.Args {
		if i == 0 {
			continue
		}
		parts = append(parts, strings.ReplaceAll(a, `"`, `\"`))
	}
	argStr := strings.Join(parts, " ")

	script := fmt.Sprintf(`do shell script "%s %s" with administrator privileges with prompt "ecorplink needs root access to create TUN device"`, exe, argStr)
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err == nil {
		os.Exit(0)
	}

	return fmt.Errorf("permission denied: ecorplink requires root privileges (try: sudo %s)", os.Args[0])
}
