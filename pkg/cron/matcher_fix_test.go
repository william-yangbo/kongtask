package cron

import (
	"testing"
	"time"
)

// TestDOWAndDateExclusionary tests the cron standard behavior where
// if both date and day-of-week are specified (not *), then matching
// either one should pass.
func TestDOWAndDateExclusionary(t *testing.T) {
	matcher := NewMatcher()

	// Create a test item: 0 15 3-4 7 0-2
	// This should match:
	// - Minute 0, Hour 15, Month 7 (July)
	// - AND (Date 3-4 OR DOW 0-2)
	testItem := ParsedCronItem{
		Minutes: []int{0},
		Hours:   []int{15},
		Dates:   []int{3, 4},    // 3rd and 4th of month
		Months:  []int{7},       // July
		DOWs:    []int{0, 1, 2}, // Sunday, Monday, Tuesday
	}

	// Test cases from graphile-worker test
	testCases := []struct {
		name     string
		time     time.Time
		expected bool
	}{
		{
			name:     "Monday July 1st at 15:00 - matches DOW but not date",
			time:     time.Date(2024, 7, 1, 15, 0, 0, 0, time.UTC), // Monday (1), 1st
			expected: true,                                         // Should match because Monday (1) is in DOWs
		},
		{
			name:     "Tuesday July 2nd at 15:00 - matches DOW but not date",
			time:     time.Date(2024, 7, 2, 15, 0, 0, 0, time.UTC), // Tuesday (2), 2nd
			expected: true,                                         // Should match because Tuesday (2) is in DOWs
		},
		{
			name:     "Thursday July 4th at 15:00 - matches date but not DOW",
			time:     time.Date(2024, 7, 4, 15, 0, 0, 0, time.UTC), // Thursday (4), 4th
			expected: true,                                         // Should match because 4th is in dates (even though Thursday is not in DOWs)
		},
		{
			name:     "Friday July 5th at 15:00 - matches neither",
			time:     time.Date(2024, 7, 5, 15, 0, 0, 0, time.UTC), // Friday (5), 5th
			expected: false,                                        // Should not match (Friday not in DOWs, 5th not in dates)
		},
		{
			name:     "Sunday July 7th at 15:00 - matches DOW but not date",
			time:     time.Date(2024, 7, 7, 15, 0, 0, 0, time.UTC), // Sunday (0), 7th
			expected: true,                                         // Should match because Sunday (0) is in DOWs
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := matcher.Matches(testItem, tc.time)
			if actual != tc.expected {
				t.Errorf("Expected %v, got %v for time %s (DOW: %d, Date: %d)",
					tc.expected, actual, tc.time.Format("2006-01-02 15:04:05"),
					tc.time.Weekday(), tc.time.Day())
			}
		})
	}
}
