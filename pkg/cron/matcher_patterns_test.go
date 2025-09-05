package cron

import (
	"testing"
	"time"
)

// TestCronPatternMatching tests various cron pattern matching scenarios
// This corresponds to the "matches datetime" test group in graphile-worker
func TestCronPatternMatching(t *testing.T) {
	matcher := NewMatcher()

	// Test data representing different datetime scenarios
	// Format: {min, hour, date, month, dow}
	testTimes := map[string]time.Time{
		"0_0_1_1_0":  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),  // Sunday
		"0_0_1_1_5":  time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),  // Friday
		"0_0_1_1_4":  time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),  // Thursday
		"0_15_1_7_0": time.Date(2024, 7, 7, 15, 0, 0, 0, time.UTC), // Sunday, July 7th at 15:00
		"0_15_4_7_2": time.Date(2024, 7, 2, 15, 0, 0, 0, time.UTC), // Tuesday, July 2nd at 15:00
		"0_15_4_7_5": time.Date(2025, 7, 4, 15, 0, 0, 0, time.UTC), // Friday, July 4th at 15:00 (2025)
		"6_15_4_7_5": time.Date(2025, 7, 4, 15, 6, 0, 0, time.UTC), // Friday, July 4th at 15:06 (2025)
	}

	testCases := []struct {
		name     string
		pattern  string
		expected map[string]bool
	}{
		{
			name:    "every minute",
			pattern: "* * * * *",
			expected: map[string]bool{
				"0_0_1_1_0":  true,
				"0_0_1_1_5":  true,
				"0_0_1_1_4":  true,
				"0_15_1_7_0": true,
				"0_15_4_7_2": true,
				"0_15_4_7_5": true,
				"6_15_4_7_5": true,
			},
		},
		{
			name:    "dow range (Friday-Saturday)",
			pattern: "* * * * 5-6",
			expected: map[string]bool{
				"0_0_1_1_0":  false, // Sunday
				"0_0_1_1_5":  true,  // Friday
				"0_0_1_1_4":  false, // Thursday
				"0_15_1_7_0": false, // Sunday
				"0_15_4_7_2": false, // Tuesday
				"0_15_4_7_5": true,  // Friday
				"6_15_4_7_5": true,  // Friday
			},
		},
		{
			name:    "dow and date range (minutes 0-5, hour 15, dates 3-4, month 7, dow 0-2)",
			pattern: "0-5 15 3-4 7 0-2",
			expected: map[string]bool{
				"0_0_1_1_0":  false, // Wrong hour, month
				"0_0_1_1_5":  false, // Wrong hour, month
				"0_0_1_1_4":  false, // Wrong hour, month
				"0_15_1_7_0": true,  // July 7th is Sunday (DOW=0), matches dow range 0-2 via OR logic
				"0_15_4_7_2": true,  // July 2nd is Tuesday (DOW=2), matches dow range 0-2 via OR logic
				"0_15_4_7_5": true,  // July 4th matches date range 3-4 (DOW doesn't matter since date matches)
				"6_15_4_7_5": false, // Minute 6 is not in range 0-5
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the cron pattern using the existing parser
			parser := NewParser()
			cronItems, err := parser.ParseCrontab(tc.pattern + " test_task")
			if err != nil {
				t.Fatalf("Failed to parse cron pattern %s: %v", tc.pattern, err)
			}
			if len(cronItems) == 0 {
				t.Fatalf("No cron items parsed from pattern: %s", tc.pattern)
			}
			cronItem := cronItems[0]

			for timeKey, expectedMatch := range tc.expected {
				testTime := testTimes[timeKey]
				actualMatch := matcher.Matches(cronItem, testTime)

				if actualMatch != expectedMatch {
					t.Errorf("Pattern: %s, Time: %s (%s), Expected: %v, Got: %v",
						tc.pattern, timeKey, testTime.Format("2006-01-02 15:04:05 Mon"), expectedMatch, actualMatch)
				}
			}
		})
	}
}

// TestSpecificCronPatterns tests specific cron patterns from graphile-worker
func TestSpecificCronPatterns(t *testing.T) {
	matcher := NewMatcher()

	testCases := []struct {
		name        string
		pattern     string
		time        time.Time
		shouldMatch bool
		description string
	}{
		{
			name:        "sunday_july_7_15_00_matches_dow_in_0-2",
			pattern:     "0-5 15 3-4 7 0-2",
			time:        time.Date(2024, 7, 7, 15, 0, 0, 0, time.UTC), // Sunday (0), July 7th at 15:00
			shouldMatch: true,
			description: "Should match because Sunday (0) is in DOW range 0-2, even though 7th is not in date range 3-4",
		},
		{
			name:        "tuesday_july_2_15_00_matches_dow_in_0-2",
			pattern:     "0-5 15 3-4 7 0-2",
			time:        time.Date(2024, 7, 2, 15, 0, 0, 0, time.UTC), // Tuesday (2), July 2nd at 15:00
			shouldMatch: true,
			description: "Should match because Tuesday (2) is in DOW range 0-2, even though 2nd is not in date range 3-4",
		},
		{
			name:        "friday_july_4_15_00_matches_date_in_3-4",
			pattern:     "0-5 15 3-4 7 0-2",
			time:        time.Date(2024, 7, 4, 15, 0, 0, 0, time.UTC), // Friday (5), July 4th at 15:00
			shouldMatch: true,
			description: "Should match because 4th is in date range 3-4, even though Friday (5) is not in DOW range 0-2",
		},
		{
			name:        "friday_july_5_15_06_no_match",
			pattern:     "0-5 15 3-4 7 0-2",
			time:        time.Date(2024, 7, 5, 15, 6, 0, 0, time.UTC), // Friday (5), July 5th at 15:06
			shouldMatch: false,
			description: "Should not match because minute 6 is not in range 0-5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the cron pattern using the existing parser
			parser := NewParser()
			cronItems, err := parser.ParseCrontab(tc.pattern + " test_task")
			if err != nil {
				t.Fatalf("Failed to parse cron pattern %s: %v", tc.pattern, err)
			}
			if len(cronItems) == 0 {
				t.Fatalf("No cron items parsed from pattern: %s", tc.pattern)
			}
			cronItem := cronItems[0]

			actualMatch := matcher.Matches(cronItem, tc.time)
			if actualMatch != tc.shouldMatch {
				t.Errorf("%s\nPattern: %s\nTime: %s (DOW: %d, Date: %d)\nExpected: %v, Got: %v",
					tc.description, tc.pattern, tc.time.Format("2006-01-02 15:04:05 Mon"),
					tc.time.Weekday(), tc.time.Day(), tc.shouldMatch, actualMatch)
			}
		})
	}
}

// TestWildcardPatterns tests various wildcard scenarios
func TestWildcardPatterns(t *testing.T) {
	matcher := NewMatcher()

	testCases := []struct {
		name    string
		pattern string
		time    time.Time
		match   bool
	}{
		{
			name:    "all_wildcards",
			pattern: "* * * * *",
			time:    time.Date(2024, 7, 15, 10, 30, 0, 0, time.UTC),
			match:   true,
		},
		{
			name:    "specific_minute_hour",
			pattern: "30 10 * * *",
			time:    time.Date(2024, 7, 15, 10, 30, 0, 0, time.UTC),
			match:   true,
		},
		{
			name:    "wrong_minute",
			pattern: "30 10 * * *",
			time:    time.Date(2024, 7, 15, 10, 25, 0, 0, time.UTC),
			match:   false,
		},
		{
			name:    "specific_dow_wildcard_date",
			pattern: "* * * * 1",                                   // Monday only
			time:    time.Date(2024, 7, 1, 10, 30, 0, 0, time.UTC), // Monday
			match:   true,
		},
		{
			name:    "specific_date_wildcard_dow",
			pattern: "* * 15 * *", // 15th of month
			time:    time.Date(2024, 7, 15, 10, 30, 0, 0, time.UTC),
			match:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the cron pattern using the existing parser
			parser := NewParser()
			cronItems, err := parser.ParseCrontab(tc.pattern + " test_task")
			if err != nil {
				t.Fatalf("Failed to parse cron pattern %s: %v", tc.pattern, err)
			}
			if len(cronItems) == 0 {
				t.Fatalf("No cron items parsed from pattern: %s", tc.pattern)
			}
			cronItem := cronItems[0]

			actualMatch := matcher.Matches(cronItem, tc.time)
			if actualMatch != tc.match {
				t.Errorf("Pattern: %s, Time: %s, Expected: %v, Got: %v",
					tc.pattern, tc.time.Format("2006-01-02 15:04:05 Mon"), tc.match, actualMatch)
			}
		})
	}
}
