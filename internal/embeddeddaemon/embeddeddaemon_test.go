package embeddeddaemon

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestEnsureWritesEmbeddedDaemonAndSkipsMatchingFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "ecorplink-daemon")
	src := Source{
		FS:     fstest.MapFS{"ecorplink-daemon": {Data: []byte("daemon-v1")}},
		Path:   "ecorplink-daemon",
		SHA256: "d4375538e8bc2b9b78c2c780d9028d761f7ec9527dd0040153a107d348033f5e",
	}
	path, err := Ensure(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if path != dst {
		t.Fatalf("path = %q, want %q", path, dst)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatalf("installed daemon is not executable: %v", info.Mode())
	}
	path, err = Ensure(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if path != dst {
		t.Fatalf("path = %q, want %q", path, dst)
	}
}

func TestEnsureRejectsSHA256Mismatch(t *testing.T) {
	_, err := Ensure(Source{
		FS:     fstest.MapFS{"ecorplink-daemon": {Data: []byte("daemon-v1")}},
		Path:   "ecorplink-daemon",
		SHA256: "bad",
	}, filepath.Join(t.TempDir(), "ecorplink-daemon"))
	if err == nil {
		t.Fatal("expected sha mismatch error")
	}
}
