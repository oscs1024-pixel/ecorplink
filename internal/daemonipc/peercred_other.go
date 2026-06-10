//go:build !darwin && !linux

package daemonipc

import (
	"fmt"
	"net"
	"runtime"
)

func validatePeer(conn net.Conn, socketDir string) error {
	return nil
}

func setSocketOwnerToDirOwner(socketPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return fmt.Errorf("peer credentials are not implemented on %s", runtime.GOOS)
}
