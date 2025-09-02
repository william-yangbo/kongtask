package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJobKeyModeConstants tests the job key mode constants
func TestJobKeyModeConstants(t *testing.T) {
	assert.Equal(t, "replace", JobKeyModeReplace)
	assert.Equal(t, "preserve_run_at", JobKeyModePreserveRunAt)
	assert.Equal(t, "unsafe_dedupe", JobKeyModeUnsafeDedupe)
}

// TestTaskSpecJobKeyMode tests the TaskSpec structure includes JobKeyMode
func TestTaskSpecJobKeyMode(t *testing.T) {
	// Test with replace mode
	replaceMode := JobKeyModeReplace
	spec := TaskSpec{
		JobKey:     stringPtr("test_key"),
		JobKeyMode: &replaceMode,
	}
	assert.Equal(t, "test_key", *spec.JobKey)
	assert.Equal(t, JobKeyModeReplace, *spec.JobKeyMode)

	// Test with preserve_run_at mode
	preserveMode := JobKeyModePreserveRunAt
	spec = TaskSpec{
		JobKey:     stringPtr("test_key_2"),
		JobKeyMode: &preserveMode,
	}
	assert.Equal(t, "test_key_2", *spec.JobKey)
	assert.Equal(t, JobKeyModePreserveRunAt, *spec.JobKeyMode)

	// Test with unsafe_dedupe mode
	dedupeMode := JobKeyModeUnsafeDedupe
	spec = TaskSpec{
		JobKey:     stringPtr("test_key_3"),
		JobKeyMode: &dedupeMode,
	}
	assert.Equal(t, "test_key_3", *spec.JobKey)
	assert.Equal(t, JobKeyModeUnsafeDedupe, *spec.JobKeyMode)
}
