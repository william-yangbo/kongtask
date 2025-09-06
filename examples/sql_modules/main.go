package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/worker/sql"
)

func main() {
	// Example usage of the SQL modules
	fmt.Println("SQL Modules Example")

	// This is a demonstration of how to use the new SQL modules
	// Note: This won't actually connect to a database in this example

	// Create compiled shared options
	options := sql.CompiledSharedOptions{
		Schema:               "graphile_worker",
		EscapedWorkerSchema:  "graphile_worker",
		WorkerSchema:         "graphile_worker",
		NoPreparedStatements: false,
	}

	fmt.Printf("SQL modules configured for schema: %s\n", options.Schema)
	fmt.Printf("No prepared statements: %v\n", options.NoPreparedStatements)

	// In a real application, you would:
	// 1. Create a database connection pool
	// 2. Use the SQL modules like this:

	exampleUsage(options)

	// Show mock usage example as well
	fmt.Println("\n" + strings.Repeat("-", 50))
	fmt.Println("Mock Usage Example:")
	mockUsage()
}

func exampleUsage(options sql.CompiledSharedOptions) {
	fmt.Println("\nExample function signatures:")

	fmt.Println("GetJob:")
	fmt.Println("  sql.GetJob(ctx, options, pool, workerId, taskNames, useNodeTime, flagsToSkip, timeProvider)")

	fmt.Println("CompleteJob:")
	fmt.Println("  sql.CompleteJob(ctx, options, pool, workerId, jobId)")

	fmt.Println("FailJob:")
	fmt.Println("  sql.FailJob(ctx, options, pool, workerId, jobId, message)")

	fmt.Println("\nThese modules provide the same functionality as the TypeScript version:")
	fmt.Println("- Modular SQL operations")
	fmt.Println("- Prepared statement support")
	fmt.Println("- Time provider integration")
	fmt.Println("- Proper error handling")
}

// mockUsage shows how these would be used in a real application
func mockUsage() {
	ctx := context.Background()

	// This would be a real connection pool in practice
	// var pool *pgxpool.Pool = createPool()...
	var pool *pgxpool.Pool

	options := sql.CompiledSharedOptions{
		Schema:               "graphile_worker",
		EscapedWorkerSchema:  "graphile_worker",
		WorkerSchema:         "graphile_worker",
		NoPreparedStatements: false,
	}

	workerId := "worker-12345"
	supportedTasks := []string{"send_email", "process_image"}
	useNodeTime := true
	flagsToSkip := []string{"maintenance"}
	timeProvider := func() time.Time { return time.Now() }

	// For demonstration purposes, we'll skip the actual database calls
	_ = pool // Avoid unused variable warning

	// In a real application with an actual pool:
	if false { // This would be: if pool != nil {
		// Get a job
		job, err := sql.GetJob(ctx, options, pool, workerId, supportedTasks, useNodeTime, flagsToSkip, timeProvider)
		if err != nil {
			log.Printf("Error getting job: %v", err)
			return
		}

		if job != nil {
			fmt.Printf("Got job: %s\n", job.ID)

			// Complete the job
			err = sql.CompleteJob(ctx, options, pool, workerId, job.ID)
			if err != nil {
				log.Printf("Error completing job: %v", err)
				return
			}

			fmt.Println("Job completed successfully")
		} else {
			fmt.Println("No jobs available")
		}
	}
}
