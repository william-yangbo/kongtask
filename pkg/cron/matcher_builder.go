package cron

import (
	"fmt"
	"time"
)

// CronMatcherBuilder provides utilities to create CronMatcher functions
type CronMatcherBuilder struct{}

// NewCronMatcherBuilder creates a new CronMatcherBuilder
func NewCronMatcherBuilder() *CronMatcherBuilder {
	return &CronMatcherBuilder{}
}

// CreatePatternMatcher creates a CronMatcher from a cron pattern string
func (b *CronMatcherBuilder) CreatePatternMatcher(pattern string) (CronMatcher, error) {
	parser := NewParser()

	// Create a temporary CronItem to validate and parse the pattern
	tempItem := CronItem{
		Task:  "temp",
		Match: pattern,
	}

	parsedItem, err := parser.ParseCronItem(tempItem)
	if err != nil {
		return nil, fmt.Errorf("failed to create pattern matcher: %w", err)
	}

	return parsedItem.Match, nil
}

// CreateCustomMatcher creates a CronMatcher from a custom function
func (b *CronMatcherBuilder) CreateCustomMatcher(fn func(digest TimestampDigest) bool) CronMatcher {
	return CronMatcher(fn)
}

// CreateEveryMinuteMatcher creates a matcher that fires every minute
func (b *CronMatcherBuilder) CreateEveryMinuteMatcher() CronMatcher {
	return func(digest TimestampDigest) bool {
		return true // Always match
	}
}

// CreateHourlyMatcher creates a matcher that fires at the specified minute of every hour
func (b *CronMatcherBuilder) CreateHourlyMatcher(minute int) CronMatcher {
	if minute < 0 || minute > 59 {
		return func(digest TimestampDigest) bool { return false } // Invalid minute, never match
	}

	return func(digest TimestampDigest) bool {
		return digest.Minute == minute
	}
}

// CreateDailyMatcher creates a matcher that fires once per day at the specified time
func (b *CronMatcherBuilder) CreateDailyMatcher(hour, minute int) CronMatcher {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return func(digest TimestampDigest) bool { return false } // Invalid time, never match
	}

	return func(digest TimestampDigest) bool {
		return digest.Hour == hour && digest.Minute == minute
	}
}

// CreateWeeklyMatcher creates a matcher that fires once per week on the specified day and time
func (b *CronMatcherBuilder) CreateWeeklyMatcher(weekday time.Weekday, hour, minute int) CronMatcher {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return func(digest TimestampDigest) bool { return false } // Invalid time, never match
	}

	dow := int(weekday) // time.Weekday: Sunday = 0

	return func(digest TimestampDigest) bool {
		return digest.DOW == dow && digest.Hour == hour && digest.Minute == minute
	}
}

// CreateMonthlyMatcher creates a matcher that fires once per month on the specified day and time
func (b *CronMatcherBuilder) CreateMonthlyMatcher(day, hour, minute int) CronMatcher {
	if day < 1 || day > 31 || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return func(digest TimestampDigest) bool { return false } // Invalid values, never match
	}

	return func(digest TimestampDigest) bool {
		return digest.Date == day && digest.Hour == hour && digest.Minute == minute
	}
}

// CreateBusinessHoursMatcher creates a matcher that fires only during business hours (Mon-Fri, 9-17)
func (b *CronMatcherBuilder) CreateBusinessHoursMatcher(minute int) CronMatcher {
	if minute < 0 || minute > 59 {
		return func(digest TimestampDigest) bool { return false } // Invalid minute, never match
	}

	return func(digest TimestampDigest) bool {
		// Monday = 1, Friday = 5 (Sunday = 0)
		isWeekday := digest.DOW >= 1 && digest.DOW <= 5
		isBusinessHour := digest.Hour >= 9 && digest.Hour <= 17
		isCorrectMinute := digest.Minute == minute

		return isWeekday && isBusinessHour && isCorrectMinute
	}
}

// CreateConditionalMatcher creates a matcher with custom conditions
// This allows for complex business logic like "only on the last Friday of the month"
func (b *CronMatcherBuilder) CreateConditionalMatcher(condition func(t time.Time, digest TimestampDigest) bool) CronMatcher {
	return func(digest TimestampDigest) bool {
		// Reconstruct time from digest for more complex checks
		// Note: This is an approximation since we don't have the exact timestamp
		now := time.Now()
		t := time.Date(now.Year(), time.Month(digest.Month), digest.Date,
			digest.Hour, digest.Minute, 0, 0, time.UTC)

		return condition(t, digest)
	}
}
