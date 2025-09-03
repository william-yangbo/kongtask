package runner

import (
	"testing"
	"time"

	"github.com/william-yangbo/kongtask/pkg/worker"
)

func TestRunnerWithCron(t *testing.T) {
	// Test options validation with cron settings
	options := RunnerOptions{
		TaskList:         map[string]worker.TaskHandler{},
		ConnectionString: "postgres://test",
		Crontab:          "0 4 * * * echo_task",
		Schema:           "test_schema",
		Concurrency:      1,
		PollInterval:     time.Second,
	}

	// Validate that options are parsed correctly
	if err := validateOptions(options); err != nil {
		t.Errorf("Expected valid options, got error: %v", err)
	}

	// Test cron items parsing (basic validation)
	releasers := &Releasers{}
	cronItems, err := getParsedCronItemsFromOptions(options, releasers)

	if err != nil {
		t.Errorf("Expected successful cron parsing, got error: %v", err)
	}

	if len(cronItems) != 1 {
		t.Errorf("Expected 1 cron item, got %d", len(cronItems))
	}

	// Cleanup
	releasers.ReleaseAll()
}

func TestRunnerOptionsWithCronFile(t *testing.T) {
	options := RunnerOptions{
		TaskList:         map[string]worker.TaskHandler{},
		ConnectionString: "postgres://test",
		CrontabFile:      "/path/to/crontab",
		Schema:           "test_schema",
	}

	// This should return an error since file parsing is not implemented
	releasers := &Releasers{}
	_, err := getParsedCronItemsFromOptions(options, releasers)

	if err == nil {
		t.Error("Expected error for unimplemented crontab file parsing")
	}

	expectedMsg := "crontab file parsing not yet implemented"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}

	releasers.ReleaseAll()
}

func TestRunnerOptionsNoCron(t *testing.T) {
	options := RunnerOptions{
		TaskList:         map[string]worker.TaskHandler{},
		ConnectionString: "postgres://test",
		Schema:           "test_schema",
	}

	// Should return empty cron items when no cron config is provided
	releasers := &Releasers{}
	cronItems, err := getParsedCronItemsFromOptions(options, releasers)

	if err != nil {
		t.Errorf("Expected no error for empty cron config, got: %v", err)
	}

	if len(cronItems) != 0 {
		t.Errorf("Expected 0 cron items, got %d", len(cronItems))
	}

	releasers.ReleaseAll()
}
