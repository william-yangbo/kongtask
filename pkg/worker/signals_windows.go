//go:build windows
// +build windows

package worker

import (
	"os"
	"syscall"
)

// SIGNALS defines the signals to handle for graceful shutdown on Windows
// Windows doesn't support Unix signals like SIGUSR2, SIGPIPE, SIGHUP
var SIGNALS = []os.Signal{
	syscall.SIGINT,
	syscall.SIGTERM,
	syscall.SIGABRT,
}
