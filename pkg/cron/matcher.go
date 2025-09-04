package cron

import (
	"time"
)

// DefaultMatcher implements the Matcher interface
type DefaultMatcher struct{}

// NewMatcher creates a new DefaultMatcher
func NewMatcher() Matcher {
	return &DefaultMatcher{}
}

// Matches checks if a cron item should fire at the given time
func (m *DefaultMatcher) Matches(item ParsedCronItem, t time.Time) bool {
	digest := m.DigestTimestamp(t)

	// Check minute, hour, and month - these must all match
	if !m.containsInt(item.Minutes, digest.Minute) ||
		!m.containsInt(item.Hours, digest.Hour) ||
		!m.containsInt(item.Months, digest.Month) {
		return false
	}

	// Cron has special behavior for date and day-of-week:
	// If both are exclusionary (not "*"), then matching either one passes
	dateIsExclusionary := len(item.Dates) != 31 // Not all days 1-31
	dowIsExclusionary := len(item.DOWs) != 7    // Not all days 0-6

	if dateIsExclusionary && dowIsExclusionary {
		// Both date and DOW are specified, so match either one
		return m.containsInt(item.Dates, digest.Date) || m.containsInt(item.DOWs, digest.DOW)
	} else if dateIsExclusionary {
		// Only date is specified
		return m.containsInt(item.Dates, digest.Date)
	} else if dowIsExclusionary {
		// Only DOW is specified
		return m.containsInt(item.DOWs, digest.DOW)
	} else {
		// Both are "*", so always match
		return true
	}
}

// DigestTimestamp extracts time components from a timestamp
func (m *DefaultMatcher) DigestTimestamp(t time.Time) TimestampDigest {
	return TimestampDigest{
		Minute: t.Minute(),
		Hour:   t.Hour(),
		Date:   t.Day(),
		Month:  int(t.Month()),
		DOW:    int(t.Weekday()),
	}
}

// GetScheduleTimesInRange finds all matching times in a range
func (m *DefaultMatcher) GetScheduleTimesInRange(item ParsedCronItem, start, end time.Time) []time.Time {
	var times []time.Time

	// Round start time down to the nearest minute
	startTime := start.Truncate(time.Minute)

	// Iterate through each minute in the range
	for current := startTime; current.Before(end); current = current.Add(time.Minute) {
		if m.Matches(item, current) {
			times = append(times, current)
		}
	}

	return times
}

// containsInt checks if a slice contains a specific integer
func (m *DefaultMatcher) containsInt(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
