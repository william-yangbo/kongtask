//go:build !windows
// +build !windows

package worker

import (
	"os"
	"syscall"
)

// SIGNALS defines the signals to handle for graceful shutdown on Unix systems
// (mirrors TypeScript signals.ts, excluding SIGPIPE per commit e714bd0)
var SIGNALS = []os.Signal{
	syscall.SIGUSR2,
	syscall.SIGINT,
	syscall.SIGTERM,
	/*
	 * SIGPIPE is excluded (aligned with graphile-worker e714bd0).
	 * Go handles SIGPIPE by converting it to EPIPE errors in syscalls,
	 * which is the appropriate behavior. We don't want the process to
	 * exit on SIGPIPE, so we exclude it from signal handling.
	 */
	// syscall.SIGPIPE,
	syscall.SIGHUP,
	syscall.SIGABRT,
}
