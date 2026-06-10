package router

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// ErrRouteExists means the OS already has an equal destination route. The
// manager can keep adding more-specific routes without taking ownership of it.
var ErrRouteExists = errors.New("route exists")

// Router is the cross-platform interface for adding/removing CIDR routes.
type Router interface {
	AddRoute(cidr string) error
	DelRoute(cidr string) error
	RouteInterface(cidr string) (string, bool)
	SetTunName(name string)
	TunName() string
}

// RouteManager manages the three routes needed for full-traffic capture.
type RouteManager struct {
	router  Router
	managed []managedRoute
	mu      sync.Mutex
	state   string
}

type persistedState struct {
	Version int            `json:"version"`
	Routes  []managedRoute `json:"routes"`
}

type managedRoute struct {
	CIDR  string `json:"cidr"`
	Iface string `json:"iface"`
}

func NewRouteManager(r Router) *RouteManager {
	home, _ := os.UserHomeDir()
	stateDir := filepath.Join(home, ".ecorplink")
	os.MkdirAll(stateDir, 0755)
	rm := &RouteManager{
		router: r,
		state:  filepath.Join(stateDir, "state.json"),
	}
	rm.load()
	return rm
}

// AddRoutes adds the fake IP pool route, broad default capture routes, and
// extra refined capture routes derived from the pre-existing route table.
// Call AFTER taking the routetable snapshot.
func (rm *RouteManager) AddRoutes(fakeIPPool string, extraCIDRs []string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for _, route := range rm.managed {
		if rm.shouldSkipDelete(route) {
			continue
		}
		rm.router.DelRoute(route.CIDR) //nolint:errcheck
	}
	rm.managed = nil
	for _, cidr := range captureCIDRs(fakeIPPool, extraCIDRs) {
		if err := rm.router.AddRoute(cidr); err != nil {
			if errors.Is(err, ErrRouteExists) {
				continue
			}
			return err
		}
		iface, ok := rm.router.RouteInterface(cidr)
		if !ok {
			continue
		}
		if tunName := rm.router.TunName(); tunName != "" && iface != tunName {
			continue
		}
		rm.managed = append(rm.managed, managedRoute{CIDR: cidr, Iface: iface})
	}
	return rm.save()
}

// Cleanup removes all managed routes.
func (rm *RouteManager) Cleanup() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for _, route := range rm.managed {
		if rm.shouldSkipDelete(route) {
			continue
		}
		rm.router.DelRoute(route.CIDR) //nolint:errcheck
	}
	rm.managed = nil
	rm.save() //nolint:errcheck
}

func (rm *RouteManager) save() error {
	data, _ := json.Marshal(persistedState{Version: 2, Routes: rm.managed})
	return os.WriteFile(rm.state, data, 0644)
}

func (rm *RouteManager) load() {
	data, err := os.ReadFile(rm.state)
	if err != nil {
		return
	}
	var current persistedState
	if err := json.Unmarshal(data, &current); err == nil && current.Version >= 2 {
		rm.managed = current.Routes
	}
}

func (rm *RouteManager) shouldSkipDelete(route managedRoute) bool {
	if route.Iface == "" {
		tunName := rm.router.TunName()
		if tunName == "" {
			return true
		}
		iface, ok := rm.router.RouteInterface(route.CIDR)
		return !ok || iface != tunName
	}
	iface, ok := rm.router.RouteInterface(route.CIDR)
	return !ok || iface != route.Iface
}

func captureCIDRs(fakeIPPool string, extraCIDRs []string) []string {
	base := []string{fakeIPPool, "0.0.0.0/1", "128.0.0.0/1"}
	seen := make(map[string]bool, len(base)+len(extraCIDRs))
	cidrs := make([]string, 0, len(base)+len(extraCIDRs))
	for _, cidr := range append(base, extraCIDRs...) {
		if cidr == "" || seen[cidr] {
			continue
		}
		seen[cidr] = true
		cidrs = append(cidrs, cidr)
	}
	return cidrs
}
