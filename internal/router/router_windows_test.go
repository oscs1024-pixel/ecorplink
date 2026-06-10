//go:build windows

package router

import "testing"

func TestRouterCompiles(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	_ = NewRouteManager(NewPlatformRouter("test"))
}
