package cron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test constants matching the TypeScript version
var (
	// 0...59
	ALL_MINUTES = func() []int {
		var minutes []int
		for i := 0; i < 60; i++ {
			minutes = append(minutes, i)
		}
		return minutes
	}()

	// 0...23
	ALL_HOURS = func() []int {
		var hours []int
		for i := 0; i < 24; i++ {
			hours = append(hours, i)
		}
		return hours
	}()

	// 1...31
	ALL_DATES = func() []int {
		var dates []int
		for i := 1; i <= 31; i++ {
			dates = append(dates, i)
		}
		return dates
	}()

	// 1...12
	ALL_MONTHS = func() []int {
		var months []int
		for i := 1; i <= 12; i++ {
			months = append(months, i)
		}
		return months
	}()

	// 0...6
	ALL_DOWS = []int{0, 1, 2, 3, 4, 5, 6}
)

// Time constants
const (
	MINUTE = time.Minute
	HOUR   = time.Hour
	DAY    = 24 * time.Hour
	WEEK   = 7 * DAY
)

// TestParsesCrontabFileCorrectly corresponds to "parses crontab file correctly" test in crontab.test.ts
func TestParsesCrontabFileCorrectly(t *testing.T) {
	exampleCrontab := `# ┌───────────── UTC minute (0 - 59)
# │ ┌───────────── UTC hour (0 - 23)
# │ │ ┌───────────── UTC day of the month (1 - 31)
# │ │ │ ┌───────────── UTC month (1 - 12)
# │ │ │ │ ┌───────────── UTC day of the week (0 - 6) (Sunday to Saturday)
# │ │ │ │ │ ┌───────────── task (identifier) to schedule
# │ │ │ │ │ │    ┌────────── optional scheduling options
# │ │ │ │ │ │    │     ┌────── optional payload to merge
# │ │ │ │ │ │    │     │
# │ │ │ │ │ │    │     │
# * * * * * task ?opts {payload}

* * * * * simple
0 4 * * * every_day_at_4_am
0 4 * * 0 every_sunday_at_4_am
0 4 * * 7 every_sunday_at_4_am ?id=sunday_7




0 4 * * 2 every_tuesday_at_4_am {"isTuesday": true}
*/10,7,56-59 1 1 1 1 one ?id=stuff&fill=4w3d2h1m&max=3&queue=my_queue&priority=3 {"myExtraPayload":{"stuff":"here with # hash char"}}
    *     *      *       *       *      lots_of_spaces     
`

	parser := NewParser()
	parsed, err := parser.ParseCrontab(exampleCrontab)
	require.NoError(t, err)

	// Test parsed[0]: simple
	require.Equal(t, "simple", parsed[0].Job.Task)
	require.Equal(t, "simple", parsed[0].Identifier)
	require.Equal(t, ALL_MINUTES, parsed[0].Minutes)
	require.Equal(t, ALL_HOURS, parsed[0].Hours)
	require.Equal(t, ALL_DATES, parsed[0].Dates)
	require.Equal(t, ALL_MONTHS, parsed[0].Months)
	require.Equal(t, ALL_DOWS, parsed[0].DOWs)
	require.Equal(t, time.Duration(0), parsed[0].Options.BackfillPeriod)
	require.Nil(t, parsed[0].Job.Payload)

	// Test parsed[1]: every_day_at_4_am
	require.Equal(t, "every_day_at_4_am", parsed[1].Job.Task)
	require.Equal(t, "every_day_at_4_am", parsed[1].Identifier)
	require.Equal(t, []int{0}, parsed[1].Minutes)
	require.Equal(t, []int{4}, parsed[1].Hours)
	require.Equal(t, ALL_DATES, parsed[1].Dates)
	require.Equal(t, ALL_MONTHS, parsed[1].Months)
	require.Equal(t, ALL_DOWS, parsed[1].DOWs)
	require.Equal(t, time.Duration(0), parsed[1].Options.BackfillPeriod)
	require.Nil(t, parsed[1].Job.Payload)

	// Test parsed[2]: every_sunday_at_4_am
	require.Equal(t, "every_sunday_at_4_am", parsed[2].Job.Task)
	require.Equal(t, "every_sunday_at_4_am", parsed[2].Identifier)
	require.Equal(t, []int{0}, parsed[2].Minutes)
	require.Equal(t, []int{4}, parsed[2].Hours)
	require.Equal(t, ALL_DATES, parsed[2].Dates)
	require.Equal(t, ALL_MONTHS, parsed[2].Months)
	require.Equal(t, []int{0}, parsed[2].DOWs) // Sunday = 0
	require.Equal(t, time.Duration(0), parsed[2].Options.BackfillPeriod)
	require.Nil(t, parsed[2].Job.Payload)

	// Test parsed[3]: every_sunday_at_4_am with custom id
	require.Equal(t, "every_sunday_at_4_am", parsed[3].Job.Task)
	require.Equal(t, "sunday_7", parsed[3].Identifier)
	require.Equal(t, []int{0}, parsed[3].Minutes)
	require.Equal(t, []int{4}, parsed[3].Hours)
	require.Equal(t, ALL_DATES, parsed[3].Dates)
	require.Equal(t, ALL_MONTHS, parsed[3].Months)
	require.Equal(t, []int{0}, parsed[3].DOWs) // day 7 wraps to 0 (Sunday)
	require.Equal(t, time.Duration(0), parsed[3].Options.BackfillPeriod)
	require.Nil(t, parsed[3].Job.Payload)

	// Test parsed[4]: every_tuesday_at_4_am with payload
	require.Equal(t, "every_tuesday_at_4_am", parsed[4].Job.Task)
	require.Equal(t, "every_tuesday_at_4_am", parsed[4].Identifier)
	require.Equal(t, []int{0}, parsed[4].Minutes)
	require.Equal(t, []int{4}, parsed[4].Hours)
	require.Equal(t, ALL_DATES, parsed[4].Dates)
	require.Equal(t, ALL_MONTHS, parsed[4].Months)
	require.Equal(t, []int{2}, parsed[4].DOWs) // Tuesday = 2
	require.Equal(t, time.Duration(0), parsed[4].Options.BackfillPeriod)
	expectedPayload := map[string]interface{}{"isTuesday": true}
	require.Equal(t, expectedPayload, parsed[4].Job.Payload)

	// Test parsed[5]: complex cron expression with all options
	// */10,7,56-59 1 1 1 1 one ?id=stuff&fill=4w3d2h1m&max=3&queue=my_queue&priority=3 {myExtraPayload:{stuff:"here with # hash char"}}
	require.Equal(t, "one", parsed[5].Job.Task)
	require.Equal(t, "stuff", parsed[5].Identifier)
	// */10 should give: 0, 10, 20, 30, 40, 50
	// ,7 adds: 7
	// ,56-59 adds: 56, 57, 58, 59
	// Expected: [0, 7, 10, 20, 30, 40, 50, 56, 57, 58, 59]
	require.Equal(t, []int{0, 7, 10, 20, 30, 40, 50, 56, 57, 58, 59}, parsed[5].Minutes)
	require.Equal(t, []int{1}, parsed[5].Hours)
	require.Equal(t, []int{1}, parsed[5].Dates)
	require.Equal(t, []int{1}, parsed[5].Months)
	require.Equal(t, []int{1}, parsed[5].DOWs) // Monday = 1

	// Check options: fill=4w3d2h1m&max=3&queue=my_queue&priority=3
	expectedBackfill := 4*WEEK + 3*DAY + 2*HOUR + 1*MINUTE
	require.Equal(t, expectedBackfill, parsed[5].Options.BackfillPeriod)
	require.Equal(t, 3, *parsed[5].Job.MaxAttempts)
	require.Equal(t, 3, *parsed[5].Job.Priority)
	require.Equal(t, "my_queue", *parsed[5].Job.QueueName)

	expectedComplexPayload := map[string]interface{}{
		"myExtraPayload": map[string]interface{}{
			"stuff": "here with # hash char",
		},
	}
	require.Equal(t, expectedComplexPayload, parsed[5].Job.Payload)

	// Test parsed[6]: lots_of_spaces
	require.Equal(t, "lots_of_spaces", parsed[6].Job.Task)
	require.Equal(t, "lots_of_spaces", parsed[6].Identifier)
	require.Equal(t, ALL_MINUTES, parsed[6].Minutes)
	require.Equal(t, ALL_HOURS, parsed[6].Hours)
	require.Equal(t, ALL_DATES, parsed[6].Dates)
	require.Equal(t, ALL_MONTHS, parsed[6].Months)
	require.Equal(t, ALL_DOWS, parsed[6].DOWs)
	require.Equal(t, time.Duration(0), parsed[6].Options.BackfillPeriod)
	require.Nil(t, parsed[6].Job.Payload)

	// Verify total number of parsed items
	require.Len(t, parsed, 7)
}

// TestGivesErrorOnSyntaxError corresponds to "gives error on syntax error" describe block
func TestGivesErrorOnSyntaxError(t *testing.T) {
	parser := NewParser()

	t.Run("too few parameters", func(t *testing.T) {
		_, err := parser.ParseCrontab("* * * * too_few_parameters")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid crontab line format")
	})

	t.Run("invalid command (two parts)", func(t *testing.T) {
		_, err := parser.ParseCrontab("* * * * * two tasks")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid crontab line format")
	})

	t.Run("range exceeded", func(t *testing.T) {
		_, err := parser.ParseCrontab("1,60 * * * * out_of_range")
		require.Error(t, err)
		require.Contains(t, err.Error(), "value 60 out of range")
	})

	t.Run("invalid wildcard divisor", func(t *testing.T) {
		_, err := parser.ParseCrontab("*/0 * * * * division_by_zero")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid wildcard step")
	})

	t.Run("unknown option", func(t *testing.T) {
		_, err := parser.ParseCrontab("* * * * * invalid_options ?unknown=3")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported option 'unknown'")
	})

	t.Run("invalid JSON syntax", func(t *testing.T) {
		_, err := parser.ParseCrontab("* * * * * json_syntax_error {\"invalidJson\"=true}")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid JSON payload")
	})
}

// TestCrontabParserEdgeCases tests additional edge cases
func TestCrontabParserEdgeCases(t *testing.T) {
	parser := NewParser()

	t.Run("empty crontab", func(t *testing.T) {
		parsed, err := parser.ParseCrontab("")
		require.NoError(t, err)
		require.Len(t, parsed, 0)
	})

	t.Run("only comments", func(t *testing.T) {
		crontab := `# This is a comment
# Another comment
`
		parsed, err := parser.ParseCrontab(crontab)
		require.NoError(t, err)
		require.Len(t, parsed, 0)
	})

	t.Run("mixed empty lines and comments", func(t *testing.T) {
		crontab := `
# Comment

# Another comment

* * * * * test_task
`
		parsed, err := parser.ParseCrontab(crontab)
		require.NoError(t, err)
		require.Len(t, parsed, 1)
		require.Equal(t, "test_task", parsed[0].Job.Task)
	})

	t.Run("range validation", func(t *testing.T) {
		// Test various range boundaries
		testCases := []struct {
			name        string
			crontab     string
			shouldError bool
		}{
			{"valid minutes range", "0,59 * * * * test", false},
			{"invalid minutes range", "60 * * * * test", true},
			{"valid hours range", "* 0,23 * * * test", false},
			{"invalid hours range", "* 24 * * * test", true},
			{"valid dates range", "* * 1,31 * * test", false},
			{"invalid dates range", "* * 0 * * test", true},
			{"valid months range", "* * * 1,12 * test", false},
			{"invalid months range", "* * * 0 * test", true},
			{"valid DOW range", "* * * * 0,6 test", false},
			{"valid DOW range (7 wraps to 0)", "* * * * 7 test", false},
			{"invalid DOW range", "* * * * 8 test", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := parser.ParseCrontab(tc.crontab)
				if tc.shouldError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})
}

// TestComplexCronExpressions tests complex cron expressions
func TestComplexCronExpressions(t *testing.T) {
	parser := NewParser()

	testCases := []struct {
		name            string
		expression      string
		expectedMinutes []int
		expectedHours   []int
		expectedDates   []int
		expectedMonths  []int
		expectedDOWs    []int
	}{
		{
			name:            "wildcard expressions",
			expression:      "* * * * * test",
			expectedMinutes: ALL_MINUTES,
			expectedHours:   ALL_HOURS,
			expectedDates:   ALL_DATES,
			expectedMonths:  ALL_MONTHS,
			expectedDOWs:    ALL_DOWS,
		},
		{
			name:            "step expressions",
			expression:      "*/15 */6 * * * test",
			expectedMinutes: []int{0, 15, 30, 45},
			expectedHours:   []int{0, 6, 12, 18},
			expectedDates:   ALL_DATES,
			expectedMonths:  ALL_MONTHS,
			expectedDOWs:    ALL_DOWS,
		},
		{
			name:            "range expressions",
			expression:      "10-15 8-10 1-5 1-3 1-5 test",
			expectedMinutes: []int{10, 11, 12, 13, 14, 15},
			expectedHours:   []int{8, 9, 10},
			expectedDates:   []int{1, 2, 3, 4, 5},
			expectedMonths:  []int{1, 2, 3},
			expectedDOWs:    []int{1, 2, 3, 4, 5},
		},
		{
			name:            "list expressions",
			expression:      "0,15,30,45 9,17 1,15 1,6,12 1,3,5 test",
			expectedMinutes: []int{0, 15, 30, 45},
			expectedHours:   []int{9, 17},
			expectedDates:   []int{1, 15},
			expectedMonths:  []int{1, 6, 12},
			expectedDOWs:    []int{1, 3, 5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := parser.ParseCrontab(tc.expression)
			require.NoError(t, err)
			require.Len(t, parsed, 1)

			item := parsed[0]
			require.Equal(t, tc.expectedMinutes, item.Minutes)
			require.Equal(t, tc.expectedHours, item.Hours)
			require.Equal(t, tc.expectedDates, item.Dates)
			require.Equal(t, tc.expectedMonths, item.Months)
			require.Equal(t, tc.expectedDOWs, item.DOWs)
		})
	}
}
