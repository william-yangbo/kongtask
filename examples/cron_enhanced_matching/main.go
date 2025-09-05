package main

import (
	"fmt"
	"log"
	"time"

	"github.com/william-yangbo/kongtask/pkg/cron"
)

func main() {
	fmt.Println("🚀 Kongtask Cron Enhanced Matcher Demo")
	fmt.Println("=====================================")
	fmt.Println()

	// Initialize builder
	builder := cron.NewCronMatcherBuilder()
	parser := cron.NewParser()

	// Demo 1: Traditional cron pattern (backward compatible)
	fmt.Println("📅 Demo 1: Traditional Cron Pattern")
	traditionalItem := cron.CronItem{
		Task:  "daily_report",
		Match: "0 9 * * *", // Every day at 9:00 AM
	}

	parsed1, err := parser.ParseCronItem(traditionalItem)
	if err != nil {
		log.Fatalf("Failed to parse traditional item: %v", err)
	}

	fmt.Printf("Task: %s, Pattern: 0 9 * * *\n", parsed1.Task)
	testTime1 := time.Date(2025, 9, 5, 9, 0, 0, 0, time.UTC)
	fmt.Printf("Matches %s? %v\n", testTime1.Format("2006-01-02 15:04:05"),
		parsed1.Match(cron.TimestampDigest{
			Minute: testTime1.Minute(),
			Hour:   testTime1.Hour(),
			Date:   testTime1.Day(),
			Month:  int(testTime1.Month()),
			DOW:    int(testTime1.Weekday()),
		}))
	fmt.Println()

	// Demo 2: Custom business hours matcher
	fmt.Println("💼 Demo 2: Custom Business Hours Matcher")
	businessMatcher := builder.CreateBusinessHoursMatcher(0) // At the top of each hour

	businessItem := cron.CronItem{
		Task:  "business_check",
		Match: businessMatcher,
	}

	parsed2, err := parser.ParseCronItem(businessItem)
	if err != nil {
		log.Fatalf("Failed to parse business item: %v", err)
	}

	fmt.Printf("Task: %s, Custom Matcher: Business Hours\n", parsed2.Task)

	// Test different times
	testTimes := []time.Time{
		time.Date(2025, 9, 5, 10, 0, 0, 0, time.UTC), // Friday 10:00 AM - should match
		time.Date(2025, 9, 5, 18, 0, 0, 0, time.UTC), // Friday 6:00 PM - should not match
		time.Date(2025, 9, 6, 10, 0, 0, 0, time.UTC), // Saturday 10:00 AM - should not match
	}

	for _, testTime := range testTimes {
		digest := cron.TimestampDigest{
			Minute: testTime.Minute(),
			Hour:   testTime.Hour(),
			Date:   testTime.Day(),
			Month:  int(testTime.Month()),
			DOW:    int(testTime.Weekday()),
		}
		fmt.Printf("  %s (%s): %v\n",
			testTime.Format("Mon 15:04"),
			testTime.Format("2006-01-02"),
			parsed2.Match(digest))
	}
	fmt.Println()

	// Demo 3: Complex conditional matcher
	fmt.Println("🎯 Demo 3: Complex Conditional Matcher (Last Friday of Month)")

	lastFridayMatcher := builder.CreateConditionalMatcher(
		func(t time.Time, digest cron.TimestampDigest) bool {
			// Only fire at 4 PM on Fridays
			if digest.DOW != 5 || digest.Hour != 16 || digest.Minute != 0 {
				return false
			}

			// Check if this is the last Friday of the month
			year, month, day := t.Date()

			// Find the last day of the month
			nextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
			lastDayOfMonth := nextMonth.AddDate(0, 0, -1)

			// Find the last Friday
			lastDay := lastDayOfMonth.Day()
			lastWeekday := int(lastDayOfMonth.Weekday())

			// Calculate days to subtract to get to Friday (5)
			daysToSubtract := (lastWeekday - 5 + 7) % 7
			lastFriday := lastDay - daysToSubtract

			return day == lastFriday
		},
	)

	complexItem := cron.CronItem{
		Task:  "monthly_report",
		Match: lastFridayMatcher,
	}

	parsed3, err := parser.ParseCronItem(complexItem)
	if err != nil {
		log.Fatalf("Failed to parse complex item: %v", err)
	}

	fmt.Printf("Task: %s, Custom Matcher: Last Friday at 4 PM\n", parsed3.Task)

	// Test last Friday of September 2025 (should be Sept 26)
	lastFridayTest := time.Date(2025, 9, 26, 16, 0, 0, 0, time.UTC)
	regularFridayTest := time.Date(2025, 9, 19, 16, 0, 0, 0, time.UTC)

	testCases := []time.Time{lastFridayTest, regularFridayTest}

	for _, testTime := range testCases {
		digest := cron.TimestampDigest{
			Minute: testTime.Minute(),
			Hour:   testTime.Hour(),
			Date:   testTime.Day(),
			Month:  int(testTime.Month()),
			DOW:    int(testTime.Weekday()),
		}
		fmt.Printf("  %s: %v\n",
			testTime.Format("2006-01-02 (Mon) 15:04"),
			parsed3.Match(digest))
	}
	fmt.Println()

	// Demo 4: Simple convenience matchers
	fmt.Println("⚡ Demo 4: Convenience Matchers")

	// Hourly at minute 30
	hourlyMatcher := builder.CreateHourlyMatcher(30)
	fmt.Printf("Hourly at :30 - 10:30: %v, 10:45: %v\n",
		hourlyMatcher(cron.TimestampDigest{Minute: 30, Hour: 10}),
		hourlyMatcher(cron.TimestampDigest{Minute: 45, Hour: 10}))

	// Daily at 6 AM
	dailyMatcher := builder.CreateDailyMatcher(6, 0)
	fmt.Printf("Daily at 6:00 AM - 6:00: %v, 6:01: %v\n",
		dailyMatcher(cron.TimestampDigest{Minute: 0, Hour: 6}),
		dailyMatcher(cron.TimestampDigest{Minute: 1, Hour: 6}))

	// Weekly on Monday at 9 AM
	weeklyMatcher := builder.CreateWeeklyMatcher(time.Monday, 9, 0)
	fmt.Printf("Weekly Monday 9:00 AM - Mon 9:00: %v, Tue 9:00: %v\n",
		weeklyMatcher(cron.TimestampDigest{Minute: 0, Hour: 9, DOW: 1}), // Monday
		weeklyMatcher(cron.TimestampDigest{Minute: 0, Hour: 9, DOW: 2})) // Tuesday

	fmt.Println()
	fmt.Println("✨ Demo completed! The new CronMatcher system provides:")
	fmt.Println("   • Backward compatibility with string patterns")
	fmt.Println("   • Custom matcher functions for complex logic")
	fmt.Println("   • Performance optimized function-based matching")
	fmt.Println("   • Rich set of convenience builders")
}
