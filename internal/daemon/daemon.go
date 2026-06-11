package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ecorplink/internal/daemonipc"
)

// WritePidFile writes the PID to a file.
func WritePidFile(path string, pid int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0644); err != nil {
		return err
	}
	daemonipc.ChownToDirOwner(path) //nolint:errcheck
	return nil
}

// ReadPidFile reads the PID from a file.
func ReadPidFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// RemovePidFile removes the PID file.
func RemovePidFile(path string) error {
	return os.Remove(path)
}

// PidFilePath returns the default PID file path.
func PidFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ecorplink.pid"
	}
	return filepath.Join(home, ".ecorplink", "ecorplink.pid")
}
