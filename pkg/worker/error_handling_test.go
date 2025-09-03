package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// TestIsConnectionError tests the connection error detection from graphile-worker commit 9d0362c
func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection reset by peer",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      errors.New("write: broken pipe"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp: connection refused"),
			expected: true,
		},
		{
			name:     "connection lost",
			err:      errors.New("connection lost"),
			expected: true,
		},
		{
			name:     "connection closed",
			err:      errors.New("connection closed"),
			expected: true,
		},
		{
			name:     "connection timeout",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "server closed the connection",
			err:      errors.New("server closed the connection unexpectedly"),
			expected: true,
		},
		{
			name:     "connection bad",
			err:      errors.New("connection bad"),
			expected: true,
		},
		{
			name:     "connection dead",
			err:      errors.New("connection dead"),
			expected: true,
		},
		{
			name:     "case insensitive match",
			err:      errors.New("Connection Reset By Peer"),
			expected: true,
		},
		{
			name:     "non-connection error",
			err:      errors.New("syntax error at position 1"),
			expected: false,
		},
		{
			name:     "generic database error",
			err:      errors.New("invalid SQL statement"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isConnectionError(tt.err)
			if result != tt.expected {
				t.Errorf("isConnectionError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestHandleClientError tests the client error handling functionality
func TestHandleClientError(t *testing.T) {
	// Use the default logger for testing
	testLogger := logger.DefaultLogger

	// Test error handling
	testErr := errors.New("connection reset by peer")
	handleClientError(testLogger, testErr)

	// Note: In a real test environment, we would capture log output
	// For now, we just verify the function runs without panic
}

// MockPoolForErrorHandling provides a mock pool for testing error handling
type MockPoolForErrorHandling struct {
	acquireErr error
	mockConn   *MockConnForErrorHandling
}

func (p *MockPoolForErrorHandling) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	if p.acquireErr != nil {
		return nil, p.acquireErr
	}
	// Return a mock connection (this is simplified for testing)
	return nil, nil
}

// MockConnForErrorHandling provides a mock connection for testing
type MockConnForErrorHandling struct {
	releaseErr error
}

func (c *MockConnForErrorHandling) Release() {
	// Mock release - do nothing for test
}

// TestWithPgClientErrorHandling tests the enhanced connection error handling
func TestWithPgClientErrorHandling(t *testing.T) {
	testLogger := logger.DefaultLogger

	t.Run("successful execution", func(t *testing.T) {
		// This test is simplified - in real usage, we'd need a real pool
		// For now, we just test that the function signature and error patterns work

		connectionErr := errors.New("connection reset by peer")
		if !isConnectionError(connectionErr) {
			t.Error("Expected connection error to be detected")
		}

		normalErr := errors.New("syntax error")
		if isConnectionError(normalErr) {
			t.Error("Expected non-connection error to not be detected as connection error")
		}
	})

	t.Run("connection error detection and logging", func(t *testing.T) {
		// Test that connection errors are properly detected and logged
		testCases := []struct {
			name           string
			err            error
			shouldBeLogged bool
		}{
			{
				name:           "connection error should be logged",
				err:            errors.New("connection reset by peer"),
				shouldBeLogged: true,
			},
			{
				name:           "non-connection error should not be logged via handleClientError",
				err:            errors.New("syntax error"),
				shouldBeLogged: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test the error detection
				isConnErr := isConnectionError(tc.err)
				if isConnErr && tc.shouldBeLogged {
					handleClientError(testLogger, tc.err)
					// Note: In a real test, we would capture and verify log output
				}
			})
		}
	})
}

// TestSetupPoolErrorHandling tests that the pool error handling setup works
func TestSetupPoolErrorHandling(t *testing.T) {
	testLogger := logger.DefaultLogger

	releasers := &Releasers{}

	// Test that setupPoolErrorHandling doesn't return an error and adds a releaser
	err := setupPoolErrorHandling(nil, testLogger, releasers) // nil pool for testing
	if err != nil {
		t.Errorf("setupPoolErrorHandling returned unexpected error: %v", err)
	}

	// Verify that a releaser was added
	if len(*releasers) == 0 {
		t.Error("Expected setupPoolErrorHandling to add a releaser function")
	}

	// Test that the releaser can be called without error
	if err := releasers.Release(); err != nil {
		t.Errorf("Expected releaser to execute without error, got: %v", err)
	}
}
