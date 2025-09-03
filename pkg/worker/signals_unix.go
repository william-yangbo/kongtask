//go:build !windows
// +build !windows

package worker

import (
	"os"
	"syscall"
)

// SIGNALS defines the signals to handle for graceful shutdown on Unix systems
// (mirrors TypeScript signals.ts)
var SIGNALS = []os.Signal{
	syscall.SIGUSR2,
	syscall.SIGINT,
	syscall.SIGTERM,
	syscall.SIGPIPE,
	syscall.SIGHUP,
	syscall.SIGABRT,
}
