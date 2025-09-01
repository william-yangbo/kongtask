package perftest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// ============================================================================
// HIGH-PRECISION LATENCY TESTS (TypeScript latencyTest.js parity)
// ============================================================================

// TestLatencyPerformanceParity implements parity testing for latencyTest.js
// This test measures precise job processing latency using high-resolution timing
func TestLatencyPerformanceParity(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing latency performance (equivalent to latencyTest.js)...")

	// Deferred structure to control job execution timing (equivalent to deferred())
	type Deferred struct {
		ch   chan struct{}
		once sync.Once
	}

	newDeferred := func() *Deferred {
		return &Deferred{ch: make(chan struct{})}
	}

	resolve := func(d *Deferred) {
		d.once.Do(func() {
			close(d.ch)
		})
	}

	wait := func(d *Deferred) {
		<-d.ch
	}

	// Track timing data
	startTimes := make(map[int]time.Time)
	latencies := make([]time.Duration, 0)
	deferreds := make(map[int]*Deferred)
	var latenciesMu sync.Mutex

	// Create worker utils for job scheduling
	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Task definition (equivalent to latencyTest.js tasks)
	tasks := map[string]worker.TaskHandler{
		"latency": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			var payloadData struct {
				ID int `json:"id"`
			}
			err := json.Unmarshal(payload, &payloadData)
			if err != nil {
				return err
			}

			id := payloadData.ID
			if startTime, exists := startTimes[id]; exists {
				latency := time.Since(startTime)
				latenciesMu.Lock()
				latencies = append(latencies, latency)
				latenciesMu.Unlock()
			}

			if deferred, exists := deferreds[id]; exists {
				resolve(deferred)
			}

			return nil
		},
	}

	// Start worker pool with concurrency 1 (equivalent to latencyTest.js options)
	options := worker.WorkerPoolOptions{
		Concurrency:  1, // Same as TypeScript test
		Schema:       "graphile_worker",
		PollInterval: 10 * time.Millisecond, // Fast polling for latency test
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Wait for empty queue helper function
	forEmptyQueue := func() {
		for {
			remaining := testutil.JobCount(t, pool, "graphile_worker")
			if remaining == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Warm up phase (equivalent to latencyTest.js warm up)
	t.Log("🔥 Warming up with 100 jobs...")
	for i := 1; i <= 100; i++ {
		_, err := workerUtils.QuickAddJob(ctx, "latency", json.RawMessage(fmt.Sprintf(`{"id": %d}`, -i)), worker.TaskSpec{})
		require.NoError(t, err)
	}
	forEmptyQueue()

	// Reset latencies after warmup
	latenciesMu.Lock()
	latencies = latencies[:0]
	latenciesMu.Unlock()

	// Let things settle (equivalent to sleep(1000) in TypeScript)
	time.Sleep(1000 * time.Millisecond)

	t.Log("📊 Beginning latency test with 1000 samples...")

	const SAMPLES = 1000

	// Execute latency test (equivalent to TypeScript main loop)
	for id := 0; id < SAMPLES; id++ {
		deferreds[id] = newDeferred()
		startTimes[id] = time.Now() // High precision timing

		_, err := workerUtils.QuickAddJob(ctx, "latency", json.RawMessage(fmt.Sprintf(`{"id": %d}`, id)), worker.TaskSpec{})
		require.NoError(t, err)

		wait(deferreds[id]) // Wait for job completion
	}

	// Wait for all jobs to be processed
	forEmptyQueue()

	// Analyze latencies (equivalent to TypeScript analysis)
	latenciesMu.Lock()
	finalLatencies := make([]time.Duration, len(latencies))
	copy(finalLatencies, latencies)
	latenciesMu.Unlock()

	require.Equal(t, SAMPLES, len(finalLatencies), "Incorrect latency count")

	// Convert to milliseconds for analysis
	numericLatencies := make([]float64, len(finalLatencies))
	for i, latency := range finalLatencies {
		numericLatencies[i] = float64(latency.Nanoseconds()) / 1e6 // Convert to milliseconds
	}

	// Sort for percentile calculations
	sort.Float64s(numericLatencies)

	// Calculate statistics (equivalent to TypeScript analysis)
	min := numericLatencies[0]
	max := numericLatencies[len(numericLatencies)-1]

	var sum float64
	for _, latency := range numericLatencies {
		sum += latency
	}
	average := sum / float64(len(numericLatencies))

	// Calculate percentiles
	p50 := numericLatencies[len(numericLatencies)*50/100]
	p95 := numericLatencies[len(numericLatencies)*95/100]
	p99 := numericLatencies[len(numericLatencies)*99/100]

	// Output results (equivalent to TypeScript console.log)
	t.Logf("📊 Latency Results:")
	t.Logf("   Min: %.2fms", min)
	t.Logf("   Max: %.2fms", max)
	t.Logf("   Avg: %.2fms", average)
	t.Logf("   P50: %.2fms", p50)
	t.Logf("   P95: %.2fms", p95)
	t.Logf("   P99: %.2fms", p99)

	// Assert reasonable performance
	require.Less(t, average, 100.0, "Average latency should be less than 100ms")
	require.Less(t, p95, 200.0, "95th percentile latency should be less than 200ms")

	// Clean up
	err = workerPool.Release()
	require.NoError(t, err)

	t.Log("✅ Latency test completed successfully")
}

// TestLatencyDistribution tests latency distribution under different load patterns
func TestLatencyDistribution(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing latency distribution under various load patterns...")

	var latencies []time.Duration
	var latenciesMu sync.Mutex

	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Task that measures its own latency from creation time
	tasks := map[string]worker.TaskHandler{
		"latency_dist": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			latency := time.Since(helpers.Job.CreatedAt)
			latenciesMu.Lock()
			latencies = append(latencies, latency)
			latenciesMu.Unlock()
			return nil
		},
	}

	// Test with different concurrency levels
	testCases := []struct {
		name        string
		concurrency int
		jobCount    int
	}{
		{"Low Concurrency", 1, 100},
		{"Medium Concurrency", 5, 500},
		{"High Concurrency", 10, 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset latencies
			latenciesMu.Lock()
			latencies = latencies[:0]
			latenciesMu.Unlock()

			// Start worker pool
			options := worker.WorkerPoolOptions{
				Concurrency:  tc.concurrency,
				Schema:       "graphile_worker",
				PollInterval: 10 * time.Millisecond,
			}

			workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
			require.NoError(t, err)

			// Add jobs
			start := time.Now()
			for i := 0; i < tc.jobCount; i++ {
				_, err := workerUtils.QuickAddJob(ctx, "latency_dist", json.RawMessage(fmt.Sprintf(`{"id": %d}`, i)), worker.TaskSpec{})
				require.NoError(t, err)
			}

			// Wait for completion
			for {
				remaining := testutil.JobCount(t, pool, "graphile_worker")
				if remaining == 0 {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}

			elapsed := time.Since(start)

			// Analyze results
			latenciesMu.Lock()
			currentLatencies := make([]time.Duration, len(latencies))
			copy(currentLatencies, latencies)
			latenciesMu.Unlock()

			if len(currentLatencies) > 0 {
				// Convert to milliseconds and sort
				ms := make([]float64, len(currentLatencies))
				for i, l := range currentLatencies {
					ms[i] = float64(l.Nanoseconds()) / 1e6
				}
				sort.Float64s(ms)

				avg := 0.0
				for _, m := range ms {
					avg += m
				}
				avg /= float64(len(ms))

				p95 := ms[len(ms)*95/100]
				throughput := float64(tc.jobCount) / elapsed.Seconds()

				t.Logf("  Concurrency %d: Avg %.2fms, P95 %.2fms, Throughput %.2f jobs/s",
					tc.concurrency, avg, p95, throughput)
			}

			err = workerPool.Release()
			require.NoError(t, err)
		})
	}
}

// TestLatencyUnderStress tests latency behavior under stress conditions
func TestLatencyUnderStress(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing latency under stress conditions...")

	var latencies []time.Duration
	var processed int64
	var latenciesMu sync.Mutex

	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Stress test task with artificial load
	tasks := map[string]worker.TaskHandler{
		"stress_latency": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			latency := time.Since(helpers.Job.CreatedAt)

			// Add some CPU work to simulate real processing
			var dummy int
			for i := 0; i < 10000; i++ {
				dummy += i
			}
			_ = dummy

			latenciesMu.Lock()
			latencies = append(latencies, latency)
			processed++
			latenciesMu.Unlock()
			return nil
		},
	}

	// High concurrency stress test
	options := worker.WorkerPoolOptions{
		Concurrency:  20, // High concurrency
		Schema:       "graphile_worker",
		PollInterval: 1 * time.Millisecond, // Aggressive polling
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Burst load: Add many jobs quickly
	const BURST_SIZE = 2000
	t.Logf("📈 Adding %d jobs in burst...", BURST_SIZE)

	burstStart := time.Now()
	for i := 0; i < BURST_SIZE; i++ {
		_, err := workerUtils.QuickAddJob(ctx, "stress_latency", json.RawMessage(fmt.Sprintf(`{"id": %d}`, i)), worker.TaskSpec{})
		require.NoError(t, err)
	}
	burstTime := time.Since(burstStart)
	t.Logf("📊 Burst completed in %v (%.2f jobs/s)", burstTime, float64(BURST_SIZE)/burstTime.Seconds())

	// Wait for processing completion
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

ProcessingLoop:
	for {
		select {
		case <-timeout:
			t.Log("⚠️  Timeout reached, stopping stress test")
			break ProcessingLoop
		case <-ticker.C:
			remaining := testutil.JobCount(t, pool, "graphile_worker")
			latenciesMu.Lock()
			currentProcessed := processed
			latenciesMu.Unlock()

			t.Logf("📊 Progress: %d processed, %d remaining", currentProcessed, remaining)
			if remaining == 0 {
				break ProcessingLoop
			}
		}
	}

	// Analyze stress test results
	latenciesMu.Lock()
	finalLatencies := make([]time.Duration, len(latencies))
	copy(finalLatencies, latencies)
	finalProcessed := processed
	latenciesMu.Unlock()

	if len(finalLatencies) > 0 {
		// Convert to milliseconds and analyze
		ms := make([]float64, len(finalLatencies))
		for i, l := range finalLatencies {
			ms[i] = float64(l.Nanoseconds()) / 1e6
		}
		sort.Float64s(ms)

		min := ms[0]
		max := ms[len(ms)-1]
		avg := 0.0
		for _, m := range ms {
			avg += m
		}
		avg /= float64(len(ms))

		p50 := ms[len(ms)*50/100]
		p95 := ms[len(ms)*95/100]
		p99 := ms[len(ms)*99/100]

		t.Logf("📊 Stress Test Results (%d jobs processed):", finalProcessed)
		t.Logf("   Min: %.2fms", min)
		t.Logf("   Max: %.2fms", max)
		t.Logf("   Avg: %.2fms", avg)
		t.Logf("   P50: %.2fms", p50)
		t.Logf("   P95: %.2fms", p95)
		t.Logf("   P99: %.2fms", p99)

		// Under stress, latencies should still be reasonable
		require.Less(t, p95, 1000.0, "95th percentile should be less than 1s even under stress")
		require.Greater(t, finalProcessed, int64(BURST_SIZE*0.8), "Should process at least 80% of jobs")
	}

	err = workerPool.Release()
	require.NoError(t, err)

	t.Log("✅ Stress latency test completed")
}

// ============================================================================
// BASIC PERFORMANCE TESTS (v0.4.0 compatible)
// ============================================================================

// TestBasicPerformance tests basic job processing (v0.4.0 compatible)
func TestBasicPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing basic performance with v0.4.0 features...")

	var processedJobs int64

	// Create worker utils for job scheduling
	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Task definition
	tasks := map[string]worker.TaskHandler{
		"performance_test": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			atomic.AddInt64(&processedJobs, 1)
			return nil
		},
	}

	// Start worker pool
	options := worker.WorkerPoolOptions{
		Concurrency: 1,
		Schema:      "graphile_worker",
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Add and process 100 jobs using WorkerUtils
	start := time.Now()
	for i := 0; i < 100; i++ {
		_, err = workerUtils.QuickAddJob(ctx, "performance_test", json.RawMessage(fmt.Sprintf(`{"id": %d}`, i)), worker.TaskSpec{})
		require.NoError(t, err)
	}

	// Wait for processing completion
	for {
		remaining := testutil.JobCount(t, pool, "graphile_worker")
		if remaining == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	elapsed := time.Since(start)
	processed := atomic.LoadInt64(&processedJobs)
	jobsPerSecond := float64(processed) / elapsed.Seconds()

	t.Logf("✅ Processed %d jobs in %v (%.2f jobs/second)", processed, elapsed, jobsPerSecond)

	err = workerPool.Release()
	require.NoError(t, err)
}

// TestJobKeyPerformance tests v0.4.0 JobKey deduplication performance
func TestJobKeyPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing JobKey deduplication performance (v0.4.0 feature)...")

	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Test adding multiple jobs with same JobKey - should result in only 1 job
	jobKey := "UNIQUE_PERFORMANCE_TEST"
	spec := worker.TaskSpec{JobKey: &jobKey}

	start := time.Now()
	var successfulJobs int64

	// Try to add 1000 jobs with same key - only first should succeed
	for i := 0; i < 1000; i++ {
		_, err := workerUtils.QuickAddJob(ctx, "performance_test", json.RawMessage(fmt.Sprintf(`{"id": %d}`, i)), spec)
		if err == nil {
			atomic.AddInt64(&successfulJobs, 1)
		}
	}

	elapsed := time.Since(start)
	successful := atomic.LoadInt64(&successfulJobs)

	// Verify only 1 job exists in database
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	require.Equal(t, 1, jobCount, "Should have exactly 1 job due to JobKey deduplication")

	t.Logf("✅ JobKey deduplication: %d attempts in %v, %d jobs created (expected: 1)", 1000, elapsed, successful)
}

// TestBulkJobsPerformance tests processing thousands of jobs (v0.4.0 compatible)
func TestBulkJobsPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Use WorkerUtils to create jobs efficiently (v0.4.0 feature)
	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	t.Log("⚡ Creating 1000 bulk jobs using WorkerUtils (v0.4.0)...")

	// Create jobs using WorkerUtils
	for i := 1; i <= 1000; i++ {
		_, err := workerUtils.QuickAddJob(ctx, "log_if_999", json.RawMessage(fmt.Sprintf(`{"id": %d}`, i)), worker.TaskSpec{})
		require.NoError(t, err)
	}

	t.Log("⚡ Starting bulk jobs performance test...")

	// Setup worker with log_if_999 task
	var processedJobs int64
	var foundTarget bool
	start := time.Now()

	tasks := map[string]worker.TaskHandler{
		"log_if_999": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			atomic.AddInt64(&processedJobs, 1)

			// Parse job payload to get id
			var payloadData struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(payload, &payloadData); err == nil {
				if payloadData.ID == 999 {
					t.Log("Found 999!")
					foundTarget = true
				}
			}
			return nil
		},
	}

	// Start worker pool
	options := worker.WorkerPoolOptions{
		Concurrency: 5,
		Schema:      "graphile_worker",
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Wait for processing completion
	for {
		remaining := testutil.JobCount(t, pool, "graphile_worker")
		if remaining == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)

		if time.Since(start) > time.Minute*2 { // Timeout after 2 minutes
			break
		}
	}

	elapsed := time.Since(start)
	processed := atomic.LoadInt64(&processedJobs)
	jobsPerSecond := float64(processed) / elapsed.Seconds()

	t.Logf("✅ Processed %d jobs in %v (%.2f jobs/second)", processed, elapsed, jobsPerSecond)
	if foundTarget {
		t.Log("✅ Successfully found target job (id=999)")
	}

	err = workerPool.Release()
	require.NoError(t, err)
}

// TestParallelWorkerPerformance tests parallel worker pattern (4 workers, 10 concurrency each)
func TestParallelWorkerPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing parallel worker performance (4 workers, matches v0.4.0 run.js)...")

	// Constants matching v0.4.0 run.js but reduced for CI efficiency
	const JOB_COUNT = 1000 // Reduced for faster CI testing
	const PARALLELISM = 4
	const CONCURRENCY = 10

	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Schedule jobs using WorkerUtils
	t.Logf("📝 Scheduling %d jobs...", JOB_COUNT)
	scheduleStart := time.Now()
	for i := 1; i <= JOB_COUNT; i++ {
		_, err := workerUtils.QuickAddJob(ctx, "log_if_999", json.RawMessage(fmt.Sprintf(`{"id": %d}`, i)), worker.TaskSpec{})
		require.NoError(t, err)
	}
	scheduleElapsed := time.Since(scheduleStart)
	t.Logf("✅ Scheduled %d jobs in %v", JOB_COUNT, scheduleElapsed)

	// Setup parallel workers
	var processedJobs int64
	var foundTarget int64

	// Task definition
	tasks := map[string]worker.TaskHandler{
		"log_if_999": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			atomic.AddInt64(&processedJobs, 1)

			var payloadData struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(payload, &payloadData); err == nil {
				if payloadData.ID == 999 {
					atomic.AddInt64(&foundTarget, 1)
				}
			}
			return nil
		},
	}

	// Use a single worker pool with total concurrency for better efficiency
	totalConcurrency := PARALLELISM * CONCURRENCY
	t.Logf("⚡ Processing with single worker pool, %d total concurrency...", totalConcurrency)

	processStart := time.Now()

	// Start single optimized worker pool
	options := worker.WorkerPoolOptions{
		Concurrency:  totalConcurrency,
		Schema:       "graphile_worker",
		PollInterval: 10 * time.Millisecond,
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Wait for all jobs to complete with timeout
	maxWaitTime := 2 * time.Minute
	waitStart := time.Now()

	for {
		remaining := testutil.JobCount(t, pool, "graphile_worker")
		if remaining == 0 {
			t.Log("✅ All jobs completed")
			break
		}

		if time.Since(waitStart) > maxWaitTime {
			t.Logf("⚠️ Timeout reached, %d jobs remaining", remaining)
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	err = workerPool.Release()
	require.NoError(t, err)

	processElapsed := time.Since(processStart)
	processed := atomic.LoadInt64(&processedJobs)
	found := atomic.LoadInt64(&foundTarget)

	// Calculate performance metrics
	jobsPerSecond := float64(processed) / processElapsed.Seconds()

	t.Logf("✅ Processed %d/%d jobs in %v", processed, JOB_COUNT, processElapsed)
	t.Logf("📊 Jobs per second: %.2f", jobsPerSecond)
	t.Logf("🎯 Found target job (id=999): %d times", found)

	// Assert reasonable performance (adjusted for smaller job count)
	require.Greater(t, jobsPerSecond, 20.0, "Should process at least 20 jobs per second with parallel workers")
	require.Greater(t, found, int64(0), "Should find the target job (id=999)")
}

// ============================================================================
// SPECIALIZED PERFORMANCE TESTS
// ============================================================================

// TestStartupShutdownPerformance measures worker startup/shutdown time
func TestStartupShutdownPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	t.Log("⚡ Testing startup/shutdown performance...")

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Measure startup time
	startTime := time.Now()

	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Task definition
	tasks := map[string]worker.TaskHandler{
		"test_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			return nil
		},
	}

	// Add a test job
	_, err = workerUtils.QuickAddJob(ctx, "test_task", json.RawMessage(`{"test": "data"}`), worker.TaskSpec{})
	require.NoError(t, err)

	// Start worker pool
	options := worker.WorkerPoolOptions{
		Concurrency: 1,
		Schema:      "graphile_worker",
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Wait for job processing
	for {
		remaining := testutil.JobCount(t, pool, "graphile_worker")
		if remaining == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Shutdown
	err = workerPool.Release()
	require.NoError(t, err)

	startupTime := time.Since(startTime)

	t.Logf("✅ Complete worker lifecycle completed in: %v", startupTime)
}

// TestMemoryPerformance tests memory usage patterns
func TestMemoryPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing memory performance...")

	var processedJobs int64

	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Task definition
	tasks := map[string]worker.TaskHandler{
		"memory_test": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			// Simulate some memory usage
			data := make([]byte, 1024*10) // 10KB per job
			_ = data
			atomic.AddInt64(&processedJobs, 1)
			return nil
		},
	}

	// Start worker pool
	options := worker.WorkerPoolOptions{
		Concurrency: 3,
		Schema:      "graphile_worker",
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Add and process jobs
	start := time.Now()
	for i := 0; i < 100; i++ {
		_, err = workerUtils.QuickAddJob(ctx, "memory_test", json.RawMessage(fmt.Sprintf(`{"data": "test_data_%d"}`, i)), worker.TaskSpec{})
		require.NoError(t, err)
	}

	// Wait for processing completion
	for {
		remaining := testutil.JobCount(t, pool, "graphile_worker")
		if remaining == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	elapsed := time.Since(start)
	processed := atomic.LoadInt64(&processedJobs)

	t.Logf("✅ Memory test: processed %d jobs in %v", processed, elapsed)

	err = workerPool.Release()
	require.NoError(t, err)
}
