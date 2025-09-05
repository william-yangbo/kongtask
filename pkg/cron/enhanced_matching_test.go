package cron

import (
	"testing"
	"time"
)

// TestEnhancedCronMatching tests the new CronMatcher functionality
func TestEnhancedCronMatching(t *testing.T) {
	parser := NewParser()
	matcher := NewMatcher()

	t.Run("BackwardCompatibility", func(t *testing.T) {
		// Test that traditional string patterns still work
		item := CronItem{
			Task:  "test_task",
			Match: "0 9 * * *", // Every day at 9 AM
		}

		parsed, err := parser.ParseCronItem(item)
		if err != nil {
			t.Fatalf("Failed to parse traditional pattern: %v", err)
		}

		// Ensure it's properly parsed
		if !parsed.IsParsed() {
			t.Error("ParsedCronItem should be marked as parsed")
		}

		// Test matching
		testTime := time.Date(2025, 9, 5, 9, 0, 0, 0, time.UTC)
		if !matcher.Matches(parsed, testTime) {
			t.Error("Should match 9:00 AM")
		}

		testTime = time.Date(2025, 9, 5, 10, 0, 0, 0, time.UTC)
		if matcher.Matches(parsed, testTime) {
			t.Error("Should not match 10:00 AM")
		}
	})

	t.Run("CustomMatcherFunction", func(t *testing.T) {
		// Test custom matcher function
		customMatcher := func(digest TimestampDigest) bool {
			// Match only business hours (9-17, Mon-Fri)
			return digest.DOW >= 1 && digest.DOW <= 5 && // Monday-Friday
				digest.Hour >= 9 && digest.Hour <= 17 && // 9 AM - 5 PM
				digest.Minute == 0 // Only at the top of the hour
		}

		item := CronItem{
			Task:  "business_task",
			Match: customMatcher,
		}

		parsed, err := parser.ParseCronItem(item)
		if err != nil {
			t.Fatalf("Failed to parse custom matcher: %v", err)
		}

		if !parsed.IsParsed() {
			t.Error("ParsedCronItem should be marked as parsed")
		}

		// Test weekday business hours (should match)
		testTime := time.Date(2025, 9, 8, 10, 0, 0, 0, time.UTC) // Monday 10:00 AM
		if !matcher.Matches(parsed, testTime) {
			t.Error("Should match Monday 10:00 AM")
		}

		// Test weekend (should not match)
		testTime = time.Date(2025, 9, 6, 10, 0, 0, 0, time.UTC) // Saturday 10:00 AM
		if matcher.Matches(parsed, testTime) {
			t.Error("Should not match Saturday 10:00 AM")
		}

		// Test after hours (should not match)
		testTime = time.Date(2025, 9, 8, 18, 0, 0, 0, time.UTC) // Monday 6:00 PM
		if matcher.Matches(parsed, testTime) {
			t.Error("Should not match Monday 6:00 PM")
		}

		// Test wrong minute (should not match)
		testTime = time.Date(2025, 9, 8, 10, 30, 0, 0, time.UTC) // Monday 10:30 AM
		if matcher.Matches(parsed, testTime) {
			t.Error("Should not match Monday 10:30 AM")
		}
	})

	t.Run("LegacyPatternFieldSupport", func(t *testing.T) {
		// Test that the deprecated Pattern field still works
		item := CronItem{
			Task:    "legacy_task",
			Pattern: "30 14 * * 1-5", // Weekdays at 2:30 PM (using deprecated field)
		}

		parsed, err := parser.ParseCronItem(item)
		if err != nil {
			t.Fatalf("Failed to parse legacy pattern field: %v", err)
		}

		if !parsed.IsParsed() {
			t.Error("ParsedCronItem should be marked as parsed")
		}

		// Test weekday at 2:30 PM (should match)
		testTime := time.Date(2025, 9, 8, 14, 30, 0, 0, time.UTC) // Monday 2:30 PM
		if !matcher.Matches(parsed, testTime) {
			t.Error("Should match Monday 2:30 PM")
		}

		// Test weekend (should not match)
		testTime = time.Date(2025, 9, 6, 14, 30, 0, 0, time.UTC) // Saturday 2:30 PM
		if matcher.Matches(parsed, testTime) {
			t.Error("Should not match Saturday 2:30 PM")
		}
	})

	t.Run("InvalidMatchType", func(t *testing.T) {
		// Test invalid match type
		item := CronItem{
			Task:  "invalid_task",
			Match: 12345, // Invalid type
		}

		_, err := parser.ParseCronItem(item)
		if err == nil {
			t.Error("Should fail with invalid match type")
		}
	})
}

// TestCronMatcherBuilder tests the convenience builder
func TestCronMatcherBuilder(t *testing.T) {
	builder := NewCronMatcherBuilder()
	parser := NewParser()
	matcher := NewMatcher()

	t.Run("CreatePatternMatcher", func(t *testing.T) {
		patternMatcher, err := builder.CreatePatternMatcher("0 12 * * *")
		if err != nil {
			t.Fatalf("Failed to create pattern matcher: %v", err)
		}

		item := CronItem{
			Task:  "pattern_test",
			Match: patternMatcher,
		}

		parsed, err := parser.ParseCronItem(item)
		if err != nil {
			t.Fatalf("Failed to parse pattern matcher item: %v", err)
		}

		testTime := time.Date(2025, 9, 5, 12, 0, 0, 0, time.UTC)
		if !matcher.Matches(parsed, testTime) {
			t.Error("Pattern matcher should match 12:00 PM")
		}
	})

	t.Run("CreateHourlyMatcher", func(t *testing.T) {
		hourlyMatcher := builder.CreateHourlyMatcher(15) // Every hour at :15

		digest := TimestampDigest{Minute: 15, Hour: 10}
		if !hourlyMatcher(digest) {
			t.Error("Should match at :15")
		}

		digest = TimestampDigest{Minute: 30, Hour: 10}
		if hourlyMatcher(digest) {
			t.Error("Should not match at :30")
		}
	})

	t.Run("CreateDailyMatcher", func(t *testing.T) {
		dailyMatcher := builder.CreateDailyMatcher(8, 30) // Every day at 8:30 AM

		digest := TimestampDigest{Minute: 30, Hour: 8}
		if !dailyMatcher(digest) {
			t.Error("Should match at 8:30 AM")
		}

		digest = TimestampDigest{Minute: 30, Hour: 9}
		if dailyMatcher(digest) {
			t.Error("Should not match at 9:30 AM")
		}
	})

	t.Run("CreateWeeklyMatcher", func(t *testing.T) {
		weeklyMatcher := builder.CreateWeeklyMatcher(time.Monday, 9, 0) // Monday 9 AM

		digest := TimestampDigest{Minute: 0, Hour: 9, DOW: 1} // Monday
		if !weeklyMatcher(digest) {
			t.Error("Should match Monday 9:00 AM")
		}

		digest = TimestampDigest{Minute: 0, Hour: 9, DOW: 2} // Tuesday
		if weeklyMatcher(digest) {
			t.Error("Should not match Tuesday 9:00 AM")
		}
	})

	t.Run("CreateBusinessHoursMatcher", func(t *testing.T) {
		businessMatcher := builder.CreateBusinessHoursMatcher(0) // Business hours at :00

		// Monday 10 AM (should match)
		digest := TimestampDigest{Minute: 0, Hour: 10, DOW: 1}
		if !businessMatcher(digest) {
			t.Error("Should match Monday 10:00 AM")
		}

		// Saturday 10 AM (should not match)
		digest = TimestampDigest{Minute: 0, Hour: 10, DOW: 6}
		if businessMatcher(digest) {
			t.Error("Should not match Saturday 10:00 AM")
		}

		// Monday 8 AM (should not match - before business hours)
		digest = TimestampDigest{Minute: 0, Hour: 8, DOW: 1}
		if businessMatcher(digest) {
			t.Error("Should not match Monday 8:00 AM")
		}

		// Monday 18 PM (should not match - after business hours)
		digest = TimestampDigest{Minute: 0, Hour: 18, DOW: 1}
		if businessMatcher(digest) {
			t.Error("Should not match Monday 6:00 PM")
		}
	})

	t.Run("InvalidValues", func(t *testing.T) {
		// Test invalid hour/minute values
		invalidHourly := builder.CreateHourlyMatcher(60) // Invalid minute
		digest := TimestampDigest{Minute: 60, Hour: 10}
		if invalidHourly(digest) {
			t.Error("Should not match invalid minute")
		}

		invalidDaily := builder.CreateDailyMatcher(24, 0) // Invalid hour
		digest = TimestampDigest{Minute: 0, Hour: 24}
		if invalidDaily(digest) {
			t.Error("Should not match invalid hour")
		}
	})
}

// TestParserValidation tests the new validation logic
func TestParserValidation(t *testing.T) {
	parser := NewParser()

	t.Run("RequiredMatchOrPattern", func(t *testing.T) {
		// Test that either Match or Pattern must be provided
		item := CronItem{
			Task: "test_task",
			// Neither Match nor Pattern provided
		}

		_, err := parser.ParseCronItem(item)
		if err == nil {
			t.Error("Should fail when neither Match nor Pattern is provided")
		}
	})

	t.Run("MatchTakesPrecedence", func(t *testing.T) {
		// Test that Match field takes precedence over Pattern
		customMatcher := func(digest TimestampDigest) bool {
			return digest.Hour == 15 // Only at 3 PM
		}

		item := CronItem{
			Task:    "precedence_test",
			Match:   customMatcher,
			Pattern: "0 9 * * *", // This should be ignored
		}

		parsed, err := parser.ParseCronItem(item)
		if err != nil {
			t.Fatalf("Failed to parse item with both Match and Pattern: %v", err)
		}

		matcher := NewMatcher()

		// Should match 3 PM (from Match field)
		testTime := time.Date(2025, 9, 5, 15, 0, 0, 0, time.UTC)
		if !matcher.Matches(parsed, testTime) {
			t.Error("Should match 3 PM from Match field")
		}

		// Should not match 9 AM (from Pattern field)
		testTime = time.Date(2025, 9, 5, 9, 0, 0, 0, time.UTC)
		if matcher.Matches(parsed, testTime) {
			t.Error("Should not match 9 AM from ignored Pattern field")
		}
	})
}
