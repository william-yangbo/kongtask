# Creating Task Handlers

Task handlers are the core of KongTask - they define the work that gets executed when jobs are processed. This guide covers best practices for creating robust, efficient, and maintainable task handlers in Go.

## Basic Task Handler Structure

A task handler is a function that implements the `TaskHandler` interface:

```go
type TaskHandler func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error
```

### Simple Example

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    "github.com/william-yangbo/kongtask/pkg/worker"
)

func emailHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    // Parse the payload
    var emailData struct {
        To      string `json:"to"`
        Subject string `json:"subject"`
        Body    string `json:"body"`
    }

    if err := json.Unmarshal(payload, &emailData); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    // Log the operation
    helpers.Logger.Info("Sending email", map[string]interface{}{
        "to":      emailData.To,
        "subject": emailData.Subject,
    })

    // Perform the actual work
    if err := sendEmail(emailData.To, emailData.Subject, emailData.Body); err != nil {
        return fmt.Errorf("failed to send email: %w", err)
    }

    helpers.Logger.Info("Email sent successfully")
    return nil
}
```

## Task Registration

Register your tasks when creating the worker:

```go
func main() {
    tasks := map[string]worker.TaskHandler{
        "send_email":       emailHandler,
        "process_payment":  paymentHandler,
        "generate_report":  reportHandler,
        "cleanup_files":    cleanupHandler,
    }

    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Concurrency: 4,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer workerPool.Release()
}
```

## Best Practices

### 1. Payload Validation

Always validate and parse your payload safely:

```go
func paymentHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var payment struct {
        UserID    int     `json:"user_id"`
        Amount    float64 `json:"amount"`
        Currency  string  `json:"currency"`
        PaymentID string  `json:"payment_id"`
    }

    // Validate JSON structure
    if err := json.Unmarshal(payload, &payment); err != nil {
        return fmt.Errorf("invalid payment payload: %w", err)
    }

    // Validate business logic
    if payment.UserID <= 0 {
        return fmt.Errorf("invalid user_id: %d", payment.UserID)
    }

    if payment.Amount <= 0 {
        return fmt.Errorf("invalid amount: %f", payment.Amount)
    }

    if payment.Currency == "" {
        return fmt.Errorf("currency is required")
    }

    // Process the payment...
    return processPayment(ctx, payment)
}
```

### 2. Context Handling

Always respect the context for cancellation and timeouts:

```go
func reportHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var reportReq struct {
        ReportID string `json:"report_id"`
        UserID   int    `json:"user_id"`
    }

    if err := json.Unmarshal(payload, &reportReq); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    // Use context in database operations
    data, err := fetchReportData(ctx, reportReq.UserID)
    if err != nil {
        return fmt.Errorf("failed to fetch data: %w", err)
    }

    // Check for cancellation before expensive operations
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // Generate report with context
    report, err := generateReport(ctx, data)
    if err != nil {
        return fmt.Errorf("failed to generate report: %w", err)
    }

    // Save report with context
    if err := saveReport(ctx, reportReq.ReportID, report); err != nil {
        return fmt.Errorf("failed to save report: %w", err)
    }

    return nil
}
```

### 3. Error Handling

Use clear, actionable error messages and distinguish between retryable and permanent failures:

```go
func apiCallHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var apiReq struct {
        URL     string            `json:"url"`
        Method  string            `json:"method"`
        Headers map[string]string `json:"headers"`
        Body    string            `json:"body"`
    }

    if err := json.Unmarshal(payload, &apiReq); err != nil {
        // Permanent failure - don't retry
        return fmt.Errorf("PERMANENT: invalid payload: %w", err)
    }

    resp, err := makeAPICall(ctx, apiReq)
    if err != nil {
        // Check if this is a retryable error
        if isRetryableError(err) {
            return fmt.Errorf("RETRYABLE: API call failed: %w", err)
        }
        // Permanent failure
        return fmt.Errorf("PERMANENT: API call failed permanently: %w", err)
    }

    if resp.StatusCode >= 500 {
        // Server errors are usually retryable
        return fmt.Errorf("RETRYABLE: server error %d: %s", resp.StatusCode, resp.Status)
    }

    if resp.StatusCode >= 400 {
        // Client errors are usually permanent
        return fmt.Errorf("PERMANENT: client error %d: %s", resp.StatusCode, resp.Status)
    }

    return nil
}
```

### 4. Logging

Use structured logging with the provided logger:

```go
func fileProcessorHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var fileJob struct {
        FilePath   string `json:"file_path"`
        Operation  string `json:"operation"`
        Parameters map[string]interface{} `json:"parameters"`
    }

    if err := json.Unmarshal(payload, &fileJob); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    // Log start of processing
    helpers.Logger.Info("Starting file processing", map[string]interface{}{
        "file_path": fileJob.FilePath,
        "operation": fileJob.Operation,
    })

    startTime := time.Now()

    // Process file
    result, err := processFile(ctx, fileJob.FilePath, fileJob.Operation, fileJob.Parameters)
    if err != nil {
        // Log error with context
        helpers.Logger.Error("File processing failed", map[string]interface{}{
            "file_path": fileJob.FilePath,
            "operation": fileJob.Operation,
            "error":     err.Error(),
            "duration":  time.Since(startTime),
        })
        return err
    }

    // Log successful completion
    helpers.Logger.Info("File processing completed", map[string]interface{}{
        "file_path":    fileJob.FilePath,
        "operation":    fileJob.Operation,
        "result_size":  len(result),
        "duration":     time.Since(startTime),
    })

    return nil
}
```

### 5. Resource Management

Properly manage resources like database connections, files, and HTTP clients:

```go
func databaseTaskHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var dbTask struct {
        Query      string      `json:"query"`
        Parameters interface{} `json:"parameters"`
    }

    if err := json.Unmarshal(payload, &dbTask); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    // Get database connection from pool
    conn, err := helpers.Pool.Acquire(ctx)
    if err != nil {
        return fmt.Errorf("failed to acquire connection: %w", err)
    }
    defer conn.Release() // Always release the connection

    // Use transaction for consistency
    tx, err := conn.Begin(ctx)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback(ctx) // Rollback if not committed

    // Execute the database operation
    _, err = tx.Exec(ctx, dbTask.Query, dbTask.Parameters)
    if err != nil {
        return fmt.Errorf("failed to execute query: %w", err)
    }

    // Commit the transaction
    if err := tx.Commit(ctx); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}
```

## Advanced Patterns

### 1. Task Composition

Break large tasks into smaller, composable units:

```go
func orderProcessingHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var order struct {
        OrderID   string `json:"order_id"`
        CustomerID string `json:"customer_id"`
        Items     []Item `json:"items"`
    }

    if err := json.Unmarshal(payload, &order); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    // Step 1: Validate inventory
    if err := validateInventory(ctx, order.Items); err != nil {
        return fmt.Errorf("inventory validation failed: %w", err)
    }

    // Step 2: Process payment
    if err := processOrderPayment(ctx, order.OrderID); err != nil {
        return fmt.Errorf("payment processing failed: %w", err)
    }

    // Step 3: Update inventory
    if err := updateInventory(ctx, order.Items); err != nil {
        return fmt.Errorf("inventory update failed: %w", err)
    }

    // Step 4: Schedule shipping (as a separate job)
    shippingPayload := map[string]interface{}{
        "order_id":    order.OrderID,
        "customer_id": order.CustomerID,
        "items":       order.Items,
    }

    if err := helpers.AddJob(ctx, "schedule_shipping", shippingPayload); err != nil {
        return fmt.Errorf("failed to schedule shipping: %w", err)
    }

    return nil
}
```

### 2. Retry Strategy

Implement custom retry logic for specific scenarios:

```go
func externalAPIHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var apiCall struct {
        URL        string `json:"url"`
        RetryCount int    `json:"retry_count"`
    }

    if err := json.Unmarshal(payload, &apiCall); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    // Implement exponential backoff within the task
    var lastErr error
    for attempt := 0; attempt < 3; attempt++ {
        if attempt > 0 {
            // Wait before retry
            backoff := time.Duration(attempt*attempt) * time.Second
            select {
            case <-time.After(backoff):
            case <-ctx.Done():
                return ctx.Err()
            }
        }

        err := callExternalAPI(ctx, apiCall.URL)
        if err == nil {
            return nil // Success
        }

        lastErr = err

        // Check if error is retryable
        if !isRetryableAPIError(err) {
            return fmt.Errorf("PERMANENT: %w", err)
        }

        helpers.Logger.Warn("API call failed, retrying", map[string]interface{}{
            "attempt": attempt + 1,
            "error":   err.Error(),
        })
    }

    return fmt.Errorf("API call failed after retries: %w", lastErr)
}
```

### 3. Progress Tracking

For long-running tasks, update progress:

```go
func batchProcessHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var batch struct {
        BatchID string   `json:"batch_id"`
        Items   []string `json:"items"`
    }

    if err := json.Unmarshal(payload, &batch); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    total := len(batch.Items)
    processed := 0

    for i, item := range batch.Items {
        // Check for cancellation
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        // Process item
        if err := processItem(ctx, item); err != nil {
            return fmt.Errorf("failed to process item %d: %w", i, err)
        }

        processed++

        // Log progress periodically
        if processed%10 == 0 || processed == total {
            helpers.Logger.Info("Batch progress", map[string]interface{}{
                "batch_id":  batch.BatchID,
                "processed": processed,
                "total":     total,
                "progress":  fmt.Sprintf("%.1f%%", float64(processed)/float64(total)*100),
            })
        }
    }

    helpers.Logger.Info("Batch completed", map[string]interface{}{
        "batch_id": batch.BatchID,
        "total":    total,
    })

    return nil
}
```

## Testing Task Handlers

### Unit Testing

```go
func TestEmailHandler(t *testing.T) {
    ctx := context.Background()

    // Mock helpers
    helpers := &worker.Helpers{
        Logger: testLogger,
        Pool:   mockPool,
    }

    tests := []struct {
        name        string
        payload     interface{}
        expectError bool
    }{
        {
            name: "valid email",
            payload: map[string]string{
                "to":      "test@example.com",
                "subject": "Test",
                "body":    "Hello",
            },
            expectError: false,
        },
        {
            name:        "invalid payload",
            payload:     "invalid json",
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            payloadBytes, _ := json.Marshal(tt.payload)
            err := emailHandler(ctx, payloadBytes, helpers)

            if tt.expectError && err == nil {
                t.Error("expected error but got none")
            }
            if !tt.expectError && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
        })
    }
}
```

### Integration Testing

```go
func TestTaskIntegration(t *testing.T) {
    // Set up test database
    pool := setupTestDB(t)
    defer pool.Close()

    // Register tasks
    tasks := map[string]worker.TaskHandler{
        "test_task": testTaskHandler,
    }

    // Add a job
    w := worker.NewWorker(pool, "graphile_worker")
    err := w.AddJob(context.Background(), "test_task", map[string]string{
        "data": "test",
    })
    require.NoError(t, err)

    // Run once to process the job
    err = worker.RunTaskListOnce(context.Background(), tasks, pool, worker.WorkerPoolOptions{})
    require.NoError(t, err)

    // Verify results
    // ... check database state, files created, etc.
}
```

## Performance Considerations

### 1. Efficient Payload Handling

```go
// Good: Use json.RawMessage for optional processing
func flexibleHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    // Only parse what you need
    var minimal struct {
        Action string `json:"action"`
    }

    if err := json.Unmarshal(payload, &minimal); err != nil {
        return err
    }

    // Parse additional fields only if needed
    switch minimal.Action {
    case "send_email":
        var emailData struct {
            Action  string `json:"action"`
            To      string `json:"to"`
            Subject string `json:"subject"`
        }
        if err := json.Unmarshal(payload, &emailData); err != nil {
            return err
        }
        return sendEmail(emailData.To, emailData.Subject)

    case "process_file":
        var fileData struct {
            Action   string `json:"action"`
            FilePath string `json:"file_path"`
        }
        if err := json.Unmarshal(payload, &fileData); err != nil {
            return err
        }
        return processFile(fileData.FilePath)
    }

    return fmt.Errorf("unknown action: %s", minimal.Action)
}
```

### 2. Connection Pooling

```go
// Good: Reuse connections from the pool
func databaseHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    conn, err := helpers.Pool.Acquire(ctx)
    if err != nil {
        return err
    }
    defer conn.Release()

    // Use the connection...
    return nil
}

// Bad: Creating new connections
func badDatabaseHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    // Don't do this - creates unnecessary overhead
    newConn, err := pgx.Connect(ctx, connectionString)
    if err != nil {
        return err
    }
    defer newConn.Close(ctx)

    // Use the connection...
    return nil
}
```

### 3. Memory Management

```go
func fileProcessingHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    var fileJob struct {
        FilePath string `json:"file_path"`
    }

    if err := json.Unmarshal(payload, &fileJob); err != nil {
        return err
    }

    // Process large files in chunks to avoid memory issues
    file, err := os.Open(fileJob.FilePath)
    if err != nil {
        return err
    }
    defer file.Close()

    buffer := make([]byte, 8192) // 8KB buffer
    for {
        n, err := file.Read(buffer)
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }

        // Process chunk
        if err := processChunk(buffer[:n]); err != nil {
            return err
        }
    }

    return nil
}
```

## Common Pitfalls

### 1. Not Handling Context Cancellation

```go
// Bad: Ignoring context
func badHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    for i := 0; i < 1000000; i++ {
        doWork(i) // This could run forever even if context is cancelled
    }
    return nil
}

// Good: Respecting context
func goodHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    for i := 0; i < 1000000; i++ {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            doWork(i)
        }
    }
    return nil
}
```

### 2. Inconsistent Error Handling

```go
// Bad: Inconsistent error messages
func badErrorHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    if err := step1(); err != nil {
        return err // Raw error
    }

    if err := step2(); err != nil {
        return fmt.Errorf("Something went wrong") // No context
    }

    return nil
}

// Good: Consistent, informative errors
func goodErrorHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    if err := step1(); err != nil {
        return fmt.Errorf("step1 failed: %w", err)
    }

    if err := step2(); err != nil {
        return fmt.Errorf("step2 failed: %w", err)
    }

    return nil
}
```

### 3. Resource Leaks

```go
// Bad: Not cleaning up resources
func badResourceHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    file, err := os.Open("somefile.txt")
    if err != nil {
        return err
    }
    // Missing defer file.Close() - resource leak!

    conn, err := helpers.Pool.Acquire(ctx)
    if err != nil {
        return err
    }
    // Missing defer conn.Release() - connection leak!

    return processFile(file)
}

// Good: Proper resource cleanup
func goodResourceHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    file, err := os.Open("somefile.txt")
    if err != nil {
        return err
    }
    defer file.Close() // Always clean up

    conn, err := helpers.Pool.Acquire(ctx)
    if err != nil {
        return err
    }
    defer conn.Release() // Always release

    return processFile(file)
}
```

By following these patterns and best practices, you'll create robust, maintainable task handlers that integrate well with KongTask's job processing system.
