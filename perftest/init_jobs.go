package perftest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// InitJobs is a Go implementation of graphile-worker's perfTest/init.js
// This matches the functionality added in commit 7abf0af
func InitJobs(ctx context.Context, pool *pgxpool.Pool, jobCount int, taskIdentifier string) error {
	// Validate task identifier (matching init.js security check)
	validTaskRegex := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	if !validTaskRegex.MatchString(taskIdentifier) {
		return fmt.Errorf("disallowed task identifier - must match ^[a-zA-Z0-9_]+$")
	}

	// Create worker utils
	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	if err != nil {
		return fmt.Errorf("failed to create WorkerUtils: %w", err)
	}

	fmt.Printf("📊 Scheduling %d jobs with task identifier '%s'...\n", jobCount, taskIdentifier)

	if taskIdentifier == "stuck" {
		// Handle "stuck" task scenario (matching init.js logic)
		fmt.Printf("⚠️  Creating '%s' jobs - these simulate stuck/locked jobs\n", taskIdentifier)

		// Add the jobs first
		for i := 1; i <= jobCount; i++ {
			payload := json.RawMessage(fmt.Sprintf(`{"id": %d}`, i))

			// For stuck jobs, we could use a specific queue name
			spec := worker.TaskSpec{
				QueueName: &taskIdentifier, // Use task identifier as queue name
			}

			_, err := workerUtils.QuickAddJob(ctx, taskIdentifier, payload, spec)
			if err != nil {
				log.Printf("WARNING: Failed to add job %d: %v", i, err)
			}

			if i%1000 == 0 {
				fmt.Printf("  ✓ Scheduled %d/%d jobs\n", i, jobCount)
			}
		}

		// Note: The original init.js had commented-out code to manually lock the queue:
		// update graphile_worker.job_queues
		// set locked_at = now(), locked_by = 'fakelock'
		// where queue_name = 'stuck';

		// We could implement this if needed for testing stuck job scenarios
		fmt.Printf("💡 Note: To simulate truly stuck jobs, manually run:\n")
		fmt.Printf("   UPDATE graphile_worker.job_queues SET locked_at = NOW(), locked_by = 'test_lock' WHERE queue_name = '%s';\n", taskIdentifier)

	} else {
		// Regular job scheduling (matching default init.js behavior)
		for i := 1; i <= jobCount; i++ {
			payload := json.RawMessage(fmt.Sprintf(`{"id": %d}`, i))

			_, err := workerUtils.QuickAddJob(ctx, taskIdentifier, payload, worker.TaskSpec{})
			if err != nil {
				log.Printf("WARNING: Failed to add job %d: %v", i, err)
			}

			if i%1000 == 0 {
				fmt.Printf("  ✓ Scheduled %d/%d jobs\n", i, jobCount)
			}
		}
	}

	fmt.Printf("✅ Successfully scheduled %d '%s' jobs\n", jobCount, taskIdentifier)
	return nil
}
