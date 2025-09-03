package main

import (
	"fmt"
	"time"

	"github.com/william-yangbo/kongtask/pkg/cron"
)

func main() {
	// This is a demonstration of how to use the cron package
	// Note: This example shows the API usage but won't run without proper database setup

	fmt.Println("=== Cron Package Demo ===")

	// 1. Create parser and test parsing
	parser := cron.NewParser()

	// Test cron pattern validation
	patterns := []string{
		"0 4 * * *",       // Daily at 4 AM
		"*/15 * * * *",    // Every 15 minutes
		"0 0 1 * *",       // First day of every month
		"invalid pattern", // Should fail
	}

	fmt.Println("\n--- Testing Cron Pattern Validation ---")
	for _, pattern := range patterns {
		if err := parser.ValidatePattern(pattern); err != nil {
			fmt.Printf("❌ Pattern '%s': %v\n", pattern, err)
		} else {
			fmt.Printf("✅ Pattern '%s': Valid\n", pattern)
		}
	}

	// 2. Test cron item parsing
	fmt.Println("\n--- Testing Cron Item Parsing ---")
	cronItems := []cron.CronItem{
		{
			Task:    "daily-report",
			Pattern: "0 4 * * *",
			Options: cron.CronItemOptions{
				QueueName: "reports",
				Priority:  intPtr(10),
				Backfill:  true,
			},
			Payload: map[string]interface{}{
				"type": "daily",
			},
		},
		{
			Task:    "cleanup",
			Pattern: "0 2 * * 0", // Sunday at 2 AM
			Options: cron.CronItemOptions{
				MaxAttempts: intPtr(3),
			},
		},
	}

	for _, item := range cronItems {
		parsedItem, err := parser.ParseCronItem(item)
		if err != nil {
			fmt.Printf("❌ Failed to parse item '%s': %v\n", item.Task, err)
		} else {
			fmt.Printf("✅ Parsed item '%s': %+v\n", item.Task, parsedItem.Task)
		}
	}

	// 3. Test time matching
	fmt.Println("\n--- Testing Time Matching ---")
	matcher := cron.NewMatcher()

	// Create a test parsed item for "every hour at minute 0"
	testItem := cron.ParsedCronItem{
		Minutes: []int{0},
		Hours:   []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
		Dates:   []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
		Months:  []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		DOWs:    []int{0, 1, 2, 3, 4, 5, 6},
		Task:    "hourly-task",
	}

	testTimes := []time.Time{
		time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC),  // Should match (hour start)
		time.Date(2024, 1, 1, 14, 30, 0, 0, time.UTC), // Should not match (half hour)
		time.Date(2024, 1, 1, 15, 0, 0, 0, time.UTC),  // Should match (next hour start)
	}

	for _, testTime := range testTimes {
		matches := matcher.Matches(testItem, testTime)
		fmt.Printf("Time %s matches: %t\n", testTime.Format("2006-01-02 15:04:05"), matches)
	}

	// 4. Demonstrate scheduler configuration (won't actually start without DB)
	fmt.Println("\n--- Scheduler Configuration Example ---")

	// This would be used with real database connections in production
	fmt.Println("Scheduler configuration would include:")
	fmt.Println("  - PostgreSQL connection pool")
	fmt.Println("  - WorkerUtils instance")
	fmt.Println("  - Event bus for monitoring")
	fmt.Println("  - Logger for debugging")
	fmt.Println("  - Backfill and timing tolerance settings")

	/*
		// Example configuration (commented out since we don't have real DB):
		config := cron.SchedulerConfig{
			PgPool:             pgPool,
			Schema:             "graphile_worker",
			WorkerUtils:        workerUtils,
			Events:             eventBus,
			Logger:             logger,
			BackfillOnStart:    true,
			ClockSkewTolerance: 10 * time.Second,
		}

		scheduler := cron.NewScheduler(config)

		if err := scheduler.Start(ctx); err != nil {
			log.Fatalf("Failed to start scheduler: %v", err)
		}
		defer scheduler.Stop()
	*/

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("The cron package is ready for integration with kongtask!")
}

func intPtr(i int) *int {
	return &i
}
