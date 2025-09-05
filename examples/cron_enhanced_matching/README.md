# Cron Enhanced Matching Example

This example demonstrates the new enhanced cron matching capabilities synchronized from graphile-worker commit c1b384b.

## Key Features

### 🔄 Backward Compatibility

- Existing cron patterns (strings) continue to work exactly as before
- No breaking changes to existing code
- Legacy array-based matching is preserved for compatibility

### ⚡ Enhanced Performance

- Function-based matching is more efficient than array lookups
- Reduced memory usage by eliminating large time component arrays
- Optimized execution path for matching logic

### 🎯 Custom Matchers

- Support for arbitrary functions as cron matchers
- Complex business logic can be implemented directly
- Rich set of convenience builders for common patterns

## Usage Examples

### Traditional Pattern (Unchanged)

```go
item := cron.CronItem{
    Task:  "daily_backup",
    Match: "0 2 * * *", // Every day at 2 AM
}
```

### Custom Business Logic

```go
builder := cron.NewCronMatcherBuilder()

// Only during business hours (9-17, Mon-Fri)
businessMatcher := builder.CreateBusinessHoursMatcher(0)

item := cron.CronItem{
    Task:  "hourly_check",
    Match: businessMatcher,
}
```

### Complex Conditional Logic

```go
// Last Friday of every month at 4 PM
lastFridayMatcher := builder.CreateConditionalMatcher(
    func(t time.Time, digest cron.TimestampDigest) bool {
        // Custom logic to determine last Friday
        return isLastFridayOfMonth(t) &&
               digest.Hour == 16 &&
               digest.Minute == 0
    },
)
```

## Running the Example

```bash
cd examples/cron_enhanced_matching
go run main.go
```

## Architecture Changes

### Before (Array-based)

```go
type ParsedCronItem struct {
    Minutes []int  // 0-59
    Hours   []int  // 0-23
    Dates   []int  // 1-31
    Months  []int  // 1-12
    DOWs    []int  // 0-6
    // ... other fields
}
```

### After (Function-based)

```go
type CronMatcher func(digest TimestampDigest) bool

type ParsedCronItem struct {
    Match   CronMatcher  // Optimized function

    // Legacy arrays kept for compatibility
    Minutes []int `json:"minutes,omitempty"` // deprecated
    // ... other legacy fields
}
```

## Benefits

1. **Performance**: Function calls are faster than array searches
2. **Flexibility**: Custom business logic can be implemented
3. **Memory**: No need to store large arrays of time values
4. **Maintainability**: Complex logic is easier to read and test
5. **Compatibility**: Existing code continues to work

## Migration Guide

### No Changes Required

Existing code using string patterns continues to work:

```go
// This still works exactly as before
item := cron.CronItem{
    Task: "backup",
    Pattern: "0 2 * * *",  // Deprecated but supported
}

// Or use the new Match field
item := cron.CronItem{
    Task: "backup",
    Match: "0 2 * * *",   // Recommended
}
```

### New Custom Matchers

For advanced use cases, implement custom matchers:

```go
customMatcher := func(digest cron.TimestampDigest) bool {
    // Your custom logic here
    return yourBusinessLogic(digest)
}

item := cron.CronItem{
    Task: "custom_task",
    Match: customMatcher,
}
```

This enhancement brings kongtask's cron system in line with modern graphile-worker capabilities while maintaining full backward compatibility.
