//go:build darwin || linux

package main

import (
	"os"
	"syscall"
)

// shutdownSigs are signals that trigger graceful shutdown.
var shutdownSigs = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// reloadSigs are signals that trigger config reload.
var reloadSigs = []os.Signal{syscall.SIGHUP}
