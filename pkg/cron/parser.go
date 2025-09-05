package cron

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Regular expressions for crontab parsing
var (
	// Matches a complete crontab line: minute hour date month dow task ?options {payload}
	crontabLineRegex = regexp.MustCompile(`^([0-9*/,-]+)\s+([0-9*/,-]+)\s+([0-9*/,-]+)\s+([0-9*/,-]+)\s+([0-9*/,-]+)\s+([_a-zA-Z][_a-zA-Z0-9:_-]*)(?:\s+\?([^\s]+))?(?:\s+(\{.*\}))?$`)

	// Matches just the time expression
	crontabTimeRegex = regexp.MustCompile(`^([0-9*/,-]+)\s+([0-9*/,-]+)\s+([0-9*/,-]+)\s+([0-9*/,-]+)\s+([0-9*/,-]+)$`)

	// Range parsing patterns
	numberRegex   = regexp.MustCompile(`^([0-9]+)$`)
	rangeRegex    = regexp.MustCompile(`^([0-9]+)-([0-9]+)$`)
	wildcardRegex = regexp.MustCompile(`^\*(?:\/([0-9]+))?$`)

	// Time phrase patterns (for backfill durations)
	timePhraseRegex = regexp.MustCompile(`^([0-9]+)([smhdw])`)
)

// Time period durations for parsing time phrases
var periodDurations = map[string]time.Duration{
	"s": time.Second,
	"m": time.Minute,
	"h": time.Hour,
	"d": 24 * time.Hour,
	"w": 7 * 24 * time.Hour,
}

// DefaultParser implements the Parser interface
type DefaultParser struct{}

// NewParser creates a new crontab parser
func NewParser() Parser {
	return &DefaultParser{}
}

// ParseCrontab parses a crontab string into ParsedCronItem slice
func (p *DefaultParser) ParseCrontab(crontab string) ([]ParsedCronItem, error) {
	lines := strings.Split(crontab, "\n")
	var items []ParsedCronItem

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		item, err := p.parseCrontabLine(line, lineNum+1)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
		}

		items = append(items, item)
	}

	// Validate unique identifiers
	if err := p.validateUniqueIdentifiers(items); err != nil {
		return nil, err
	}

	return items, nil
}

// ParseCronItems converts CronItem slice to ParsedCronItem slice
func (p *DefaultParser) ParseCronItems(items []CronItem) ([]ParsedCronItem, error) {
	var parsed []ParsedCronItem

	for i, item := range items {
		parsedItem, err := p.parseCronItem(item, i)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		parsed = append(parsed, parsedItem)
	}

	// Validate unique identifiers
	if err := p.validateUniqueIdentifiers(parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}

// ParseCronItem converts a single CronItem to ParsedCronItem (interface method)
func (p *DefaultParser) ParseCronItem(item CronItem) (ParsedCronItem, error) {
	return p.parseCronItem(item, 0)
}

// ValidatePattern validates a cron pattern
func (p *DefaultParser) ValidatePattern(pattern string) error {
	if !crontabTimeRegex.MatchString(pattern) {
		return fmt.Errorf("invalid cron pattern format: %s", pattern)
	}

	parts := strings.Fields(pattern)
	if len(parts) != 5 {
		return fmt.Errorf("cron pattern must have exactly 5 fields: %s", pattern)
	}

	// Validate each field
	ranges := []struct {
		field string
		min   int
		max   int
		wrap  bool
	}{
		{"minute", 0, 59, false},
		{"hour", 0, 23, false},
		{"date", 1, 31, false},
		{"month", 1, 12, false},
		{"dow", 0, 6, true}, // Allow 7 as Sunday (wraps to 0)
	}

	for i, r := range ranges {
		_, err := p.parseCrontabRange(fmt.Sprintf("%s field", r.field), parts[i], r.min, r.max, r.wrap)
		if err != nil {
			return err
		}
	}

	return nil
}

// parseCrontabLine parses a single crontab line
func (p *DefaultParser) parseCrontabLine(line string, lineNum int) (ParsedCronItem, error) {
	matches := crontabLineRegex.FindStringSubmatch(line)
	if matches == nil {
		return ParsedCronItem{}, fmt.Errorf("invalid crontab line format")
	}

	// Parse time fields
	minutes, err := p.parseCrontabRange("minute", matches[1], 0, 59, false)
	if err != nil {
		return ParsedCronItem{}, err
	}

	hours, err := p.parseCrontabRange("hour", matches[2], 0, 23, false)
	if err != nil {
		return ParsedCronItem{}, err
	}

	dates, err := p.parseCrontabRange("date", matches[3], 1, 31, false)
	if err != nil {
		return ParsedCronItem{}, err
	}

	months, err := p.parseCrontabRange("month", matches[4], 1, 12, false)
	if err != nil {
		return ParsedCronItem{}, err
	}

	dows, err := p.parseCrontabRange("dow", matches[5], 0, 6, true)
	if err != nil {
		return ParsedCronItem{}, err
	}

	// Parse task and options
	task := matches[6]
	optionsStr := matches[7]
	payloadStr := matches[8]

	options, identifier, err := p.parseCrontabOptions(lineNum, optionsStr)
	if err != nil {
		return ParsedCronItem{}, err
	}

	if identifier == "" {
		identifier = task
	}

	payload, err := p.parseCrontabPayload(lineNum, payloadStr)
	if err != nil {
		return ParsedCronItem{}, err
	}

	// Create CronJob from parsed data
	job := CronJob{
		Task:        task,
		Payload:     payload,
		MaxAttempts: options.MaxAttempts,
		Priority:    options.Priority,
	}

	// Handle optional pointer fields
	if options.QueueName != "" {
		job.QueueName = &options.QueueName
	}
	if options.JobKey != "" {
		job.JobKey = &options.JobKey
	}

	// Create matcher function from parsed arrays
	matcher := p.createArrayMatcher(minutes, hours, dates, months, dows)

	return ParsedCronItem{
		_isParsed:  IsParsed, // Mark as properly parsed
		Match:      matcher,
		Minutes:    minutes, // Keep for backward compatibility
		Hours:      hours,
		Dates:      dates,
		Months:     months,
		DOWs:       dows,
		Task:       task,
		Identifier: identifier,
		Options:    options,
		Payload:    payload,
		Job:        job,
	}, nil
}

// parseCronItem converts a CronItem to ParsedCronItem
func (p *DefaultParser) parseCronItem(item CronItem, index int) (ParsedCronItem, error) {
	// Validate that either Match or Pattern is provided
	if item.Match == nil && item.Pattern == "" {
		return ParsedCronItem{}, fmt.Errorf("either Match or Pattern must be provided")
	}

	// Handle backward compatibility: if Pattern is provided but Match is not
	if item.Match == nil && item.Pattern != "" {
		item.Match = item.Pattern
	}

	var matcher CronMatcher
	var minutes, hours, dates, months, dows []int

	switch m := item.Match.(type) {
	case string:
		// Traditional cron pattern string
		if err := p.ValidatePattern(m); err != nil {
			return ParsedCronItem{}, err
		}

		// Parse the pattern and create time arrays (for backward compatibility)
		parts := strings.Fields(m)
		var err error

		minutes, err = p.parseCrontabRange("minute", parts[0], 0, 59, false)
		if err != nil {
			return ParsedCronItem{}, err
		}

		hours, err = p.parseCrontabRange("hour", parts[1], 0, 23, false)
		if err != nil {
			return ParsedCronItem{}, err
		}

		dates, err = p.parseCrontabRange("date", parts[2], 1, 31, false)
		if err != nil {
			return ParsedCronItem{}, err
		}

		months, err = p.parseCrontabRange("month", parts[3], 1, 12, false)
		if err != nil {
			return ParsedCronItem{}, err
		}

		dows, err = p.parseCrontabRange("dow", parts[4], 0, 6, true)
		if err != nil {
			return ParsedCronItem{}, err
		}

		// Create matcher function from parsed arrays
		matcher = p.createArrayMatcher(minutes, hours, dates, months, dows)

	case CronMatcher:
		// Custom matcher function
		matcher = m

	case func(TimestampDigest) bool:
		// Function with correct signature but not explicitly typed as CronMatcher
		matcher = CronMatcher(m)

	default:
		return ParsedCronItem{}, fmt.Errorf("match must be either a string (cron pattern) or CronMatcher function")
	}

	identifier := item.Identifier
	if identifier == "" {
		identifier = item.Task
	}

	payload := item.Payload
	// Don't create empty map, preserve nil if no payload

	// Create CronJob from parsed data
	job := CronJob{
		Task:        item.Task,
		Payload:     payload,
		MaxAttempts: item.Options.MaxAttempts,
		Priority:    item.Options.Priority,
	}

	// Handle optional pointer fields
	if item.Options.QueueName != "" {
		job.QueueName = &item.Options.QueueName
	}
	if item.Options.JobKey != "" {
		job.JobKey = &item.Options.JobKey
	}

	return ParsedCronItem{
		_isParsed:  IsParsed, // Mark as properly parsed
		Match:      matcher,
		Minutes:    minutes, // Keep for backward compatibility
		Hours:      hours,
		Dates:      dates,
		Months:     months,
		DOWs:       dows,
		Task:       item.Task,
		Identifier: identifier,
		Options:    item.Options,
		Payload:    payload,
		Job:        job,
	}, nil
}

// createArrayMatcher creates a CronMatcher function from time component arrays
func (p *DefaultParser) createArrayMatcher(minutes, hours, dates, months, dows []int) CronMatcher {
	return func(digest TimestampDigest) bool {
		// Check if current minute matches
		if !contains(minutes, digest.Minute) {
			return false
		}

		// Check if current hour matches
		if !contains(hours, digest.Hour) {
			return false
		}

		// Check if current month matches
		if !contains(months, digest.Month) {
			return false
		}

		// Cron has special behavior for date and day-of-week:
		// If both are exclusionary (not "*"), then matching either one passes
		dateIsExclusionary := len(dates) != 31 // Not all days 1-31
		dowIsExclusionary := len(dows) != 7    // Not all days 0-6

		if dateIsExclusionary && dowIsExclusionary {
			// Both date and DOW are specified, so match either one
			return contains(dates, digest.Date) || contains(dows, digest.DOW)
		} else if dateIsExclusionary {
			// Only date is specified
			return contains(dates, digest.Date)
		} else if dowIsExclusionary {
			// Only DOW is specified
			return contains(dows, digest.DOW)
		} else {
			// Both are "*", so always match
			return true
		}
	}
}

// contains checks if a slice contains a specific integer
func contains(slice []int, value int) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// parseCrontabRange parses a cron time field (e.g., "*/5", "1-3", "1,2,3")
func (p *DefaultParser) parseCrontabRange(fieldName, rangeStr string, min, max int, wrap bool) ([]int, error) {
	parts := strings.Split(rangeStr, ",")
	var numbers []int

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Handle exact number
		if matches := numberRegex.FindStringSubmatch(part); matches != nil {
			num, err := strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("invalid number in %s: %s", fieldName, part)
			}

			// Handle wrap (7 -> 0 for Sunday)
			if wrap && num == max+1 {
				num = min
			}

			if num < min || num > max {
				return nil, fmt.Errorf("%s value %d out of range [%d-%d]", fieldName, num, min, max)
			}

			numbers = append(numbers, num)
			continue
		}

		// Handle range (e.g., "1-5")
		if matches := rangeRegex.FindStringSubmatch(part); matches != nil {
			start, err := strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range start in %s: %s", fieldName, part)
			}

			end, err := strconv.Atoi(matches[2])
			if err != nil {
				return nil, fmt.Errorf("invalid range end in %s: %s", fieldName, part)
			}

			if end <= start {
				return nil, fmt.Errorf("invalid range in %s: end must be greater than start", fieldName)
			}

			for i := start; i <= end; i++ {
				num := i
				if wrap && num == max+1 {
					num = min
				}

				if num < min || num > max {
					continue // Skip invalid values in range
				}

				numbers = append(numbers, num)
			}
			continue
		}

		// Handle wildcard (e.g., "*", "*/5")
		if matches := wildcardRegex.FindStringSubmatch(part); matches != nil {
			step := 1
			if matches[1] != "" {
				var err error
				step, err = strconv.Atoi(matches[1])
				if err != nil || step < 1 {
					return nil, fmt.Errorf("invalid wildcard step in %s: %s", fieldName, part)
				}
			}

			for i := min; i <= max; i += step {
				numbers = append(numbers, i)
			}
			continue
		}

		return nil, fmt.Errorf("unsupported syntax in %s: %s", fieldName, part)
	}

	// Sort and deduplicate
	sort.Ints(numbers)
	unique := make([]int, 0, len(numbers))
	for i, num := range numbers {
		if i == 0 || numbers[i-1] != num {
			unique = append(unique, num)
		}
	}

	return unique, nil
}

// parseCrontabOptions parses cron options string (e.g., "fill=1d&max=3&queue=my_queue")
func (p *DefaultParser) parseCrontabOptions(lineNum int, optionsStr string) (CronItemOptions, string, error) {
	options := CronItemOptions{}
	var identifier string

	if optionsStr == "" {
		return options, identifier, nil
	}

	values, err := url.ParseQuery(optionsStr)
	if err != nil {
		return options, identifier, fmt.Errorf("invalid options format on line %d: %w", lineNum, err)
	}

	for key, vals := range values {
		if len(vals) != 1 {
			return options, identifier, fmt.Errorf("option '%s' specified multiple times on line %d", key, lineNum)
		}
		value := vals[0]

		switch key {
		case "id":
			if !regexp.MustCompile(`^[_a-zA-Z][-_a-zA-Z0-9]*$`).MatchString(value) {
				return options, identifier, fmt.Errorf("invalid id format on line %d: %s", lineNum, value)
			}
			identifier = value

		case "fill":
			duration, err := p.parseTimePhrase(value)
			if err != nil {
				return options, identifier, fmt.Errorf("invalid fill duration on line %d: %w", lineNum, err)
			}
			options.BackfillPeriod = duration
			options.Backfill = true // Enable backfill when fill period is specified

		case "max":
			max, err := strconv.Atoi(value)
			if err != nil || max < 1 {
				return options, identifier, fmt.Errorf("invalid max attempts on line %d: %s", lineNum, value)
			}
			options.MaxAttempts = &max

		case "queue":
			if !regexp.MustCompile(`^[-a-zA-Z0-9_:]+$`).MatchString(value) {
				return options, identifier, fmt.Errorf("invalid queue name on line %d: %s", lineNum, value)
			}
			options.QueueName = value

		case "priority":
			priority, err := strconv.Atoi(value)
			if err != nil {
				return options, identifier, fmt.Errorf("invalid priority on line %d: %s", lineNum, value)
			}
			options.Priority = &priority

		case "jobKey":
			options.JobKey = value

		default:
			return options, identifier, fmt.Errorf("unsupported option '%s' on line %d", key, lineNum)
		}
	}

	return options, identifier, nil
}

// parseCrontabPayload parses JSON payload string
func (p *DefaultParser) parseCrontabPayload(lineNum int, payloadStr string) (map[string]interface{}, error) {
	if payloadStr == "" {
		return nil, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON payload on line %d: %w", lineNum, err)
	}

	return payload, nil
}

// parseTimePhrase parses time phrase like "1d2h3m" into duration
func (p *DefaultParser) parseTimePhrase(phrase string) (time.Duration, error) {
	remaining := phrase
	var total time.Duration

	for remaining != "" {
		matches := timePhraseRegex.FindStringSubmatch(remaining)
		if matches == nil {
			return 0, fmt.Errorf("invalid time phrase: %s", phrase)
		}

		quantity, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, fmt.Errorf("invalid quantity in time phrase: %s", matches[1])
		}

		period := matches[2]
		duration, exists := periodDurations[period]
		if !exists {
			return 0, fmt.Errorf("unsupported time period: %s", period)
		}

		total += time.Duration(quantity) * duration
		remaining = remaining[len(matches[0]):]
	}

	return total, nil
}

// validateUniqueIdentifiers ensures all cron items have unique identifiers
func (p *DefaultParser) validateUniqueIdentifiers(items []ParsedCronItem) error {
	seen := make(map[string]bool)
	var duplicates []string

	for _, item := range items {
		if seen[item.Identifier] {
			duplicates = append(duplicates, item.Identifier)
		}
		seen[item.Identifier] = true
	}

	if len(duplicates) > 0 {
		return fmt.Errorf("duplicate cron identifiers found: %v", duplicates)
	}

	return nil
}

// ParseOptions parses JSON options string
func (p *DefaultParser) ParseOptions(optionsJSON string) (CronItemOptions, error) {
	var options CronItemOptions

	if optionsJSON == "" || optionsJSON == "{}" {
		return options, nil
	}

	if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
		return options, fmt.Errorf("failed to parse options JSON: %w", err)
	}

	return options, nil
}
