package worker

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestMakeAddJobWithUseNodeTime tests that MakeAddJob correctly handles useNodeTime option
func TestMakeAddJobWithUseNodeTime(t *testing.T) {
	t.Run("useNodeTime false - should not set runAt automatically", func(t *testing.T) {
		var capturedArgs []interface{}

		mockWithPgClient := func(ctx context.Context, callback func(tx pgx.Tx) error) error {
			mockTx := &mockTransaction{
				execFunc: func(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
					capturedArgs = args
					return pgconn.CommandTag{}, nil
				},
			}
			return callback(mockTx)
		}

		options := &SharedOptions{
			UseNodeTime: &[]bool{false}[0],
		}

		addJob := MakeAddJobWithOptions(options, mockWithPgClient)
		err := addJob(context.Background(), "test_task", map[string]string{"test": "payload"}, TaskSpec{})

		if err != nil {
			t.Fatalf("AddJob failed: %v", err)
		}

		// runAt should be nil (args[3] - check for typed nil pointer

		if len(capturedArgs) < 4 {
			t.Fatalf("Expected at least 4 arguments, got %d", len(capturedArgs))
		}

		runAtArg := capturedArgs[3]
		t.Logf("runAtArg is nil: %v", runAtArg == nil)
		t.Logf("runAtArg is typed nil: %v", runAtArg == (*string)(nil))

		// Check for typed nil pointer
		if runAtPtr, ok := runAtArg.(*string); ok {
			if runAtPtr != nil {
				t.Errorf("Expected runAt to be nil when useNodeTime=false, but got %v", runAtArg)
			}
		} else if runAtArg != nil {
			t.Errorf("Expected runAt to be nil when useNodeTime=false, but got %v", runAtArg)
		}
	})

	t.Run("useNodeTime true - should set runAt to current time", func(t *testing.T) {
		var capturedArgs []interface{}

		mockWithPgClient := func(ctx context.Context, callback func(tx pgx.Tx) error) error {
			mockTx := &mockTransaction{
				execFunc: func(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
					capturedArgs = args
					return pgconn.CommandTag{}, nil
				},
			}
			return callback(mockTx)
		}

		options := &SharedOptions{
			UseNodeTime: &[]bool{true}[0],
		}

		addJob := MakeAddJobWithOptions(options, mockWithPgClient)
		err := addJob(context.Background(), "test_task", map[string]string{"test": "payload"}, TaskSpec{})

		if err != nil {
			t.Fatalf("AddJob failed: %v", err)
		}

		// runAt should be set (args[3])
		if capturedArgs[3] == nil {
			t.Errorf("Expected runAt to be set when useNodeTime=true, but got nil")
		}
	})
}

// mockTransaction implements pgx.Tx interface for testing
type mockTransaction struct {
	execFunc func(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error)
}

func (m *mockTransaction) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, nil
}

func (m *mockTransaction) Commit(ctx context.Context) error {
	return nil
}

func (m *mockTransaction) Rollback(ctx context.Context) error {
	return nil
}

func (m *mockTransaction) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (m *mockTransaction) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}

func (m *mockTransaction) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *mockTransaction) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (m *mockTransaction) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return m.execFunc(ctx, query, args...)
}

func (m *mockTransaction) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockTransaction) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return nil
}

func (m *mockTransaction) Conn() *pgx.Conn {
	return nil
}
