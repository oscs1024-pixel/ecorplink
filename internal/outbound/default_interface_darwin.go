//go:build darwin

package outbound

import (
	"net"
	"os/exec"
	"strings"
)

func defaultRouteInterface() (*net.Interface, error) {
	out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
	if err != nil {
		return nil, err
	}
	name := parseRouteInterface(out)
	if name == "" {
		return nil, nil
	}
	return net.InterfaceByName(name)
}

func parseRouteInterface(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	return ""
}
