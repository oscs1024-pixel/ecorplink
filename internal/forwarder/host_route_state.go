package forwarder

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
)

var hostRouteStateMu sync.Mutex

func hostRouteStatePath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ecorplink")
	os.MkdirAll(dir, 0755) //nolint:errcheck
	return filepath.Join(dir, "host-routes.json")
}

func loadPersistedHostRoutes() map[string]struct{} {
	hostRouteStateMu.Lock()
	defer hostRouteStateMu.Unlock()
	return loadPersistedHostRoutesLocked()
}

func savePersistedHostRoutes(routes map[string]struct{}) {
	hostRouteStateMu.Lock()
	defer hostRouteStateMu.Unlock()
	savePersistedHostRoutesLocked(routes)
}

func rememberHostRoute(ip net.IP) {
	if ip == nil || ip.To4() == nil {
		return
	}
	hostRouteStateMu.Lock()
	defer hostRouteStateMu.Unlock()
	routes := loadPersistedHostRoutesLocked()
	routes[ip.String()] = struct{}{}
	savePersistedHostRoutesLocked(routes)
}

func forgetHostRoute(ip string) {
	hostRouteStateMu.Lock()
	defer hostRouteStateMu.Unlock()
	routes := loadPersistedHostRoutesLocked()
	delete(routes, ip)
	savePersistedHostRoutesLocked(routes)
}

func CleanupPersistedHostRoutes() {
	hostRouteStateMu.Lock()
	defer hostRouteStateMu.Unlock()
	for ip := range loadPersistedHostRoutesLocked() {
		deleteHostRoute(ip) //nolint:errcheck
	}
	savePersistedHostRoutesLocked(map[string]struct{}{})
}

func loadPersistedHostRoutesLocked() map[string]struct{} {
	routes := make(map[string]struct{})
	data, err := os.ReadFile(hostRouteStatePath())
	if err != nil {
		return routes
	}
	var list []string
	if json.Unmarshal(data, &list) != nil {
		return routes
	}
	for _, ip := range list {
		if net.ParseIP(ip).To4() != nil {
			routes[ip] = struct{}{}
		}
	}
	return routes
}

func savePersistedHostRoutesLocked(routes map[string]struct{}) {
	list := make([]string, 0, len(routes))
	for ip := range routes {
		list = append(list, ip)
	}
	data, _ := json.Marshal(list)
	os.WriteFile(hostRouteStatePath(), data, 0644) //nolint:errcheck
}
