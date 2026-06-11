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
	return ChownToDirOwner(socketPath)
}

// ChownToDirOwner is a no-op on platforms that do not support Unix ownership.
func ChownToDirOwner(path string) error {
	return nil
}
