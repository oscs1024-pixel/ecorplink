//go:build linux

package daemonipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func validatePeer(conn net.Conn, socketDir string) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("unexpected connection type %T", conn)
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var peerUID uint32
	var peerErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			peerErr = err
			return
		}
		peerUID = uint32(cred.Uid)
	}); err != nil {
		return err
	}
	if peerErr != nil {
		return peerErr
	}
	return authorizePeerUID(peerUID, socketDir)
}

func authorizePeerUID(uid uint32, socketDir string) error {
	if uid == uint32(os.Getuid()) {
		return nil
	}
	dirUID, ok := dirOwnerUID(socketDir)
	if ok && uid == dirUID {
		return nil
	}
	return fmt.Errorf("unauthorized uid %d", uid)
}

func dirOwnerUID(path string) (uint32, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}

func setSocketOwnerToDirOwner(socketPath string) error {
	if os.Getuid() != 0 {
		return nil
	}
	info, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(socketPath, int(st.Uid), int(st.Gid))
}
