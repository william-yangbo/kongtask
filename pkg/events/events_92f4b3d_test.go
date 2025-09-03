package events

import (
	"testing"
)

// TestNewEvents_92f4b3d tests the new event constants added in commit 92f4b3d
func TestNewEvents_92f4b3d(t *testing.T) {
	// Test WorkerGetJobStart event constant
	if string(WorkerGetJobStart) != "worker:getJob:start" {
		t.Errorf("WorkerGetJobStart should be 'worker:getJob:start', got %s", string(WorkerGetJobStart))
	}

	// Test JobComplete event constant
	if string(JobComplete) != "job:complete" {
		t.Errorf("JobComplete should be 'job:complete', got %s", string(JobComplete))
	}

	// Verify the constants are different from existing ones
	if WorkerGetJobStart == WorkerGetJobError {
		t.Error("WorkerGetJobStart should be different from WorkerGetJobError")
	}

	if JobComplete == JobSuccess {
		t.Error("JobComplete should be different from JobSuccess")
	}

	if JobComplete == JobFailed {
		t.Error("JobComplete should be different from JobFailed")
	}
}
