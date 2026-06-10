package daemonipc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestServerSocketIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not used on Windows")
	}
	path := filepath.Join(t.TempDir(), "daemon.sock")
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
