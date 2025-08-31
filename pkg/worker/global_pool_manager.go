package worker

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

var (
	// Global registry of worker pools for signal handling (mirrors TypeScript allWorkerPools)
	allWorkerPools     []*WorkerPool
	allWorkerPoolsLock sync.RWMutex

	// Global signal handling state (mirrors TypeScript _registeredSignalHandlers, _shuttingDown)
	registeredSignalHandlers bool
	shuttingDown             bool
	signalHandlerLock        sync.Mutex
)

// SIGNALS defines the signals to handle for graceful shutdown (mirrors TypeScript signals.ts)
var SIGNALS = []os.Signal{
	syscall.SIGUSR2,
	syscall.SIGINT,
	syscall.SIGTERM,
	syscall.SIGPIPE,
	syscall.SIGHUP,
	syscall.SIGABRT,
}

// GetAllWorkerPools returns all active worker pools (exported for testing, mirrors _allWorkerPools)
func GetAllWorkerPools() []*WorkerPool {
	allWorkerPoolsLock.RLock()
	defer allWorkerPoolsLock.RUnlock()

	pools := make([]*WorkerPool, len(allWorkerPools))
	copy(pools, allWorkerPools)
	return pools
}

// registerSignalHandlers sets up global signal handlers (mirrors TypeScript registerSignalHandlers)
func registerSignalHandlers(logger *logger.Logger) error {
	signalHandlerLock.Lock()
	defer signalHandlerLock.Unlock()

	if shuttingDown {
		return fmt.Errorf("system has already gone into shutdown, should not be spawning new workers now")
	}

	if registeredSignalHandlers {
		return nil
	}

	registeredSignalHandlers = true

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, SIGNALS...)

	go func() {
		for sig := range sigChan {
			logger.Error(fmt.Sprintf("Received '%v'; attempting graceful shutdown...", sig))

			// Set shutdown flag
			signalHandlerLock.Lock()
			if shuttingDown {
				signalHandlerLock.Unlock()
				continue
			}
			shuttingDown = true
			signalHandlerLock.Unlock()

			// Start shutdown timer (5 seconds like TypeScript)
			shutdownTimer := time.NewTimer(5 * time.Second)
			defer shutdownTimer.Stop()

			// Gracefully shutdown all worker pools
			allWorkerPoolsLock.RLock()
			pools := make([]*WorkerPool, len(allWorkerPools))
			copy(pools, allWorkerPools)
			allWorkerPoolsLock.RUnlock()

			shutdownComplete := make(chan struct{})
			go func() {
				var wg sync.WaitGroup
				for _, pool := range pools {
					wg.Add(1)
					go func(p *WorkerPool) {
						defer wg.Done()
						if err := p.GracefulShutdown(fmt.Sprintf("Forced worker shutdown due to %v", sig)); err != nil {
							logger.Error(fmt.Sprintf("Error during graceful shutdown: %v", err))
						}
					}(pool)
				}
				wg.Wait()
				close(shutdownComplete)
			}()

			// Wait for shutdown completion or timeout
			select {
			case <-shutdownComplete:
				logger.Error("Graceful shutdown completed")
			case <-shutdownTimer.C:
				logger.Error("Graceful shutdown timeout reached")
			}

			// Unregister signal handlers
			signal.Stop(sigChan)
			close(sigChan)

			logger.Error(fmt.Sprintf("Graceful shutdown attempted; killing self via %v", sig))

			// Kill self with the same signal
			process, err := os.FindProcess(os.Getpid())
			if err == nil {
				process.Signal(sig)
			}
			os.Exit(1)
		}
	}()

	// Register debug logging for each signal
	for _, sig := range SIGNALS {
		logger.Debug(fmt.Sprintf("Registering signal handler for %v", sig))
	}

	return nil
}

// addWorkerPool adds a worker pool to the global registry (mirrors TypeScript allWorkerPools.push)
func addWorkerPool(pool *WorkerPool) {
	allWorkerPoolsLock.Lock()
	defer allWorkerPoolsLock.Unlock()
	allWorkerPools = append(allWorkerPools, pool)
}

// removeWorkerPool removes a worker pool from the global registry (mirrors TypeScript allWorkerPools.splice)
func removeWorkerPool(pool *WorkerPool) {
	allWorkerPoolsLock.Lock()
	defer allWorkerPoolsLock.Unlock()

	for i, p := range allWorkerPools {
		if p == pool {
			allWorkerPools = append(allWorkerPools[:i], allWorkerPools[i+1:]...)
			break
		}
	}
}

// ManagedWorkerPool wraps WorkerPool with enhanced global management capabilities
type ManagedWorkerPool struct {
	*WorkerPool
}

// Release implements enhanced release with global registry cleanup
func (mwp *ManagedWorkerPool) Release() error {
	err := mwp.WorkerPool.Release()
	removeWorkerPool(mwp.WorkerPool)
	return err
}

// GracefulShutdown implements enhanced graceful shutdown with global registry cleanup
func (mwp *ManagedWorkerPool) GracefulShutdown(message string) error {
	err := mwp.WorkerPool.GracefulShutdown(message)
	removeWorkerPool(mwp.WorkerPool)
	return err
}

// RunTaskListWithSignalHandling creates and starts a worker pool with global signal handling
// This is the main entry point that mirrors TypeScript runTaskList exactly
func RunTaskListWithSignalHandling(ctx context.Context, tasks map[string]TaskHandler, pool *pgxpool.Pool, options WorkerPoolOptions) (*ManagedWorkerPool, error) {
	// Set up global signal handlers
	if err := registerSignalHandlers(options.Logger); err != nil {
		return nil, fmt.Errorf("failed to register signal handlers: %w", err)
	}

	// Create worker pool using existing RunTaskList
	wp, err := RunTaskList(ctx, tasks, pool, options)
	if err != nil {
		return nil, err
	}

	// Add to global registry for signal handling
	addWorkerPool(wp)

	// Wrap in managed worker pool
	managedWP := &ManagedWorkerPool{WorkerPool: wp}

	return managedWP, nil
}
