package daemonipc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServerSocketIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not used on Windows")
	}
	dir, err := os.MkdirTemp("/tmp", "ecipc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "daemon.sock")
	srv := NewServer(path, func(cmd Cmd) Response { return Response{OK: true} })
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("socket mode = %#o, want 0600", got)
	}
}

func TestServerStartRefusesActiveSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not used on Windows")
	}
	dir, err := os.MkdirTemp("/tmp", "ecipc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "daemon.sock")
	first := NewServer(path, func(cmd Cmd) Response { return Response{OK: true} })
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	defer first.Stop()

	second := NewServer(path, func(cmd Cmd) Response { return Response{OK: true} })
	err = second.Start()
	if err == nil {
		second.Stop()
		t.Fatal("second server should not replace an active daemon socket")
	}
	if !strings.Contains(err.Error(), "already active") {
		t.Fatalf("Start error = %v, want already active", err)
	}
}
