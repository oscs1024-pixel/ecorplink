//go:build !windows

package router

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouterCompiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_ = NewRouteManager(NewPlatformRouter("test"))
}

type fakeRouter struct {
	added      []string
	deleted    []string
	exists     map[string]bool
	routeIface map[string]string
	tunName    string
}

func (r *fakeRouter) AddRoute(cidr string) error {
	r.added = append(r.added, cidr)
	if r.exists[cidr] {
		return ErrRouteExists
	}
	if r.routeIface == nil {
		r.routeIface = make(map[string]string)
	}
	if _, ok := r.routeIface[cidr]; !ok {
		r.routeIface[cidr] = r.TunName()
	}
	return nil
}

func (r *fakeRouter) DelRoute(cidr string) error {
	r.deleted = append(r.deleted, cidr)
	return nil
}

func (r *fakeRouter) RouteInterface(cidr string) (string, bool) {
	if r.routeIface == nil {
		return "", false
	}
	iface, ok := r.routeIface[cidr]
	return iface, ok
}

func (r *fakeRouter) TunName() string {
	if r.tunName == "" {
		return "utun-test"
	}
	return r.tunName
}

func TestRouteManagerSkipsExistingRoutesAndContinues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	r := &fakeRouter{exists: map[string]bool{"0.0.0.0/1": true}}
	rm := NewRouteManager(r)
	err := rm.AddRoutes("198.19.0.0/16", []string{"0.0.0.0/2", "64.0.0.0/2"})
	if err != nil {
		t.Fatal(err)
	}

	wantAdded := []string{"198.19.0.0/16", "0.0.0.0/1", "128.0.0.0/1", "0.0.0.0/2", "64.0.0.0/2"}
	if len(r.added) != len(wantAdded) {
		t.Fatalf("added %v, want %v", r.added, wantAdded)
	}
	for i := range wantAdded {
		if r.added[i] != wantAdded[i] {
			t.Fatalf("added %v, want %v", r.added, wantAdded)
		}
	}
	if errors.Is(err, ErrRouteExists) {
		t.Fatal("ErrRouteExists should be skipped")
	}
}

func (r *fakeRouter) SetTunName(name string) { r.tunName = name }

func TestRouteManagerCleansStaleStateBeforeAddingRoutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, ".ecorplink")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(`{"version":2,"routes":[{"cidr":"198.18.0.0/15","iface":"utun-test"},{"cidr":"0.0.0.0/1","iface":"utun4"},{"cidr":"128.0.0.0/1","iface":"utun4"}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRouter{routeIface: map[string]string{
		"198.18.0.0/15": "utun-test",
		"64.0.0.0/3":    "utun-test",
	}}
	rm := NewRouteManager(r)
	if err := rm.AddRoutes("198.19.0.0/16", []string{"64.0.0.0/3", "96.0.0.0/3", "64.0.0.0/3"}); err != nil {
		t.Fatal(err)
	}

	wantDeleted := []string{"198.18.0.0/15"}
	if len(r.deleted) != len(wantDeleted) {
		t.Fatalf("deleted %v, want %v", r.deleted, wantDeleted)
	}
	for i := range wantDeleted {
		if r.deleted[i] != wantDeleted[i] {
			t.Fatalf("deleted %v, want %v", r.deleted, wantDeleted)
		}
	}

	wantAdded := []string{"198.19.0.0/16", "0.0.0.0/1", "128.0.0.0/1", "64.0.0.0/3", "96.0.0.0/3"}
	if len(r.added) != len(wantAdded) {
		t.Fatalf("added %v, want %v", r.added, wantAdded)
	}
	for i := range wantAdded {
		if r.added[i] != wantAdded[i] {
			t.Fatalf("added %v, want %v", r.added, wantAdded)
		}
	}
}

func TestRouteManagerIgnoresStringArrayState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, ".ecorplink")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldState := `["198.18.0.0/15","0.0.0.0/1","128.0.0.0/1","64.0.0.0/3"]`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(oldState), 0644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRouter{}
	rm := NewRouteManager(r)
	rm.Cleanup()

	if len(r.deleted) != 0 {
		t.Fatalf("deleted %v, want none", r.deleted)
	}
}

func TestRouteManagerIgnoresVersionOneState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, ".ecorplink")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldState := `{"version":1,"routes":["198.18.0.0/15","64.0.0.0/3"]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(oldState), 0644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRouter{routeIface: map[string]string{
		"198.18.0.0/15": "utun-test",
		"64.0.0.0/3":    "utun4",
	}}
	rm := NewRouteManager(r)
	rm.Cleanup()

	if len(r.deleted) != 0 {
		t.Fatalf("deleted %v, want none", r.deleted)
	}
}

func TestRouteManagerCurrentStateDeletesOwnedBroadRoutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, ".ecorplink")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	current := `{"version":2,"routes":[{"cidr":"198.18.0.0/15","iface":"utun-test"},{"cidr":"0.0.0.0/1","iface":"utun-test"},{"cidr":"128.0.0.0/1","iface":"utun-test"}]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(current), 0644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRouter{routeIface: map[string]string{
		"198.18.0.0/15": "utun-test",
		"0.0.0.0/1":     "utun-test",
		"128.0.0.0/1":   "utun-test",
	}}
	rm := NewRouteManager(r)
	rm.Cleanup()

	wantDeleted := []string{"198.18.0.0/15", "0.0.0.0/1", "128.0.0.0/1"}
	if len(r.deleted) != len(wantDeleted) {
		t.Fatalf("deleted %v, want %v", r.deleted, wantDeleted)
	}
	for i := range wantDeleted {
		if r.deleted[i] != wantDeleted[i] {
			t.Fatalf("deleted %v, want %v", r.deleted, wantDeleted)
		}
	}
}

func TestRouteManagerCurrentStateSkipsRoutesOwnedByAnotherInterface(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, ".ecorplink")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	current := `{"version":2,"routes":[{"cidr":"0.0.0.0/1","iface":"utun-test"},{"cidr":"128.0.0.0/1","iface":"utun-test"}]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(current), 0644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRouter{routeIface: map[string]string{
		"0.0.0.0/1":   "utun4",
		"128.0.0.0/1": "utun4",
	}}
	rm := NewRouteManager(r)
	rm.Cleanup()

	if len(r.deleted) != 0 {
		t.Fatalf("deleted %v, want none", r.deleted)
	}
}

func TestRouteManagerDoesNotPersistRoutesOwnedByAnotherInterface(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	r := &fakeRouter{routeIface: map[string]string{
		"198.19.0.0/16": "utun-test",
		"0.0.0.0/1":     "utun4",
		"128.0.0.0/1":   "utun-test",
	}}
	rm := NewRouteManager(r)
	if err := rm.AddRoutes("198.19.0.0/16", nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".ecorplink", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, `"cidr":"0.0.0.0/1"`) {
		t.Fatalf("state persisted route owned by another interface: %s", got)
	}
	if !strings.Contains(got, `"cidr":"198.19.0.0/16"`) || !strings.Contains(got, `"cidr":"128.0.0.0/1"`) {
		t.Fatalf("state missing owned routes: %s", got)
	}
}
