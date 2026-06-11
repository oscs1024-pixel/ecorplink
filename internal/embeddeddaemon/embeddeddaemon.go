package embeddeddaemon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"ecorplink/internal/daemonipc"
)

type Source struct {
	FS     fs.FS
	Path   string
	SHA256 string
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "ecorplink-daemon")
	}
	name := "ecorplink-daemon"
	if runtime.GOOS == "windows" {
		name = "ecorplink-daemon.exe"
	}
	return filepath.Join(home, ".ecorplink", "bin", name)
}

func Ensure(src Source, dst string) (string, error) {
	if dst == "" {
		dst = DefaultPath()
	}
	data, err := fs.ReadFile(src.FS, src.Path)
	if err != nil {
		return "", fmt.Errorf("read embedded daemon: %w", err)
	}
	sum := sha256Hex(data)
	if src.SHA256 != "" && sum != src.SHA256 {
		return "", fmt.Errorf("embedded daemon sha256 mismatch: got %s want %s", sum, src.SHA256)
	}
	current, err := os.ReadFile(dst)
	if err == nil && bytes.Equal(current, data) && sha256Hex(current) == sum {
		return dst, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", fmt.Errorf("create daemon dir: %w", err)
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0755); err != nil {
		return "", fmt.Errorf("write daemon: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return "", fmt.Errorf("install daemon: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dst, 0755); err != nil {
			return "", fmt.Errorf("chmod daemon: %w", err)
		}
	}
	daemonipc.ChownToDirOwner(dst) //nolint:errcheck
	return dst, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
