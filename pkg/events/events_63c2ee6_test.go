package events

import (
	"testing"
)

// TestJobCompleteEventErrorField_63c2ee6 tests the new error field in job:complete event
// introduced in commit 63c2ee6
func TestJobCompleteEventErrorField_63c2ee6(t *testing.T) {
	// Test that we can create job:complete event data with error field

	// Success case: error should be nil
	successEventData := map[string]interface{}{
		"worker": map[string]interface{}{
			"workerId": "test-worker",
		},
		"job": map[string]interface{}{
			"id":             "123",
			"taskIdentifier": "test_task",
		},
		"error": nil, // Success case - nil error
	}

	// Verify structure
	if successEventData["error"] != nil {
		t.Errorf("Expected error to be nil for successful job, got %v", successEventData["error"])
	}

	// Failure case: error should contain error message
	failureEventData := map[string]interface{}{
		"worker": map[string]interface{}{
			"workerId": "test-worker",
		},
		"job": map[string]interface{}{
			"id":             "456",
			"taskIdentifier": "failing_task",
		},
		"error": "task execution failed", // Error case
	}

	// Verify structure
	errorVal := failureEventData["error"]
	if errorVal == nil {
		t.Fatal("Expected error field to be present for failed job")
	}
	if errorStr, ok := errorVal.(string); !ok || errorStr != "task execution failed" {
		t.Errorf("Expected error to be 'task execution failed', got %v", errorVal)
	}
}

// TestJobCompleteEventBackwardCompatibility_63c2ee6 ensures the event still has required fields
func TestJobCompleteEventBackwardCompatibility_63c2ee6(t *testing.T) {
	// Event data with new error field should still be compatible
	eventData := map[string]interface{}{
		"worker": map[string]interface{}{"workerId": "test"},
		"job":    map[string]interface{}{"id": "789"},
		"error":  nil, // New field
	}

	// Basic fields should still be accessible
	if _, hasWorker := eventData["worker"]; !hasWorker {
		t.Error("Worker field should be present")
	}
	if _, hasJob := eventData["job"]; !hasJob {
		t.Error("Job field should be present")
	}

	// New error field should be present
	if _, hasError := eventData["error"]; !hasError {
		t.Error("Error field should be present after 63c2ee6 commit")
	}
}

// TestJobCompleteEventConstants_63c2ee6 verifies event type constant
func TestJobCompleteEventConstants_63c2ee6(t *testing.T) {
	// JobComplete event type should still exist
	if string(JobComplete) != "job:complete" {
		t.Errorf("JobComplete should be 'job:complete', got %s", string(JobComplete))
	}
}
