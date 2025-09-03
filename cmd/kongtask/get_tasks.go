package main

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"unicode"

	"github.com/william-yangbo/kongtask/pkg/worker"
)

// titleCase converts the first character to uppercase (replacement for deprecated strings.Title)
func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// WatchedTaskList provides task list with a release function for cleanup (sync from getTasks.ts)
type WatchedTaskList struct {
	Tasks   map[string]worker.TaskHandler
	Release func()
}

// getTasks loads tasks from a directory (simplified version of graphile-worker getTasks.ts)
// TODO: Implement full watch mode functionality
func getTasks(taskDirectory string, watch bool) (*WatchedTaskList, error) {
	if taskDirectory == "" {
		taskDirectory = "./tasks"
	}

	// Check if directory exists
	if _, err := os.Stat(taskDirectory); os.IsNotExist(err) {
		// Return empty task list if directory doesn't exist
		return &WatchedTaskList{
			Tasks:   make(map[string]worker.TaskHandler),
			Release: func() {}, // No-op release function
		}, nil
	}

	tasks := make(map[string]worker.TaskHandler)

	// For now, implement basic file loading without watch mode
	// TODO: Implement proper plugin loading and watch functionality
	if watch {
		return nil, fmt.Errorf("watch mode not yet implemented")
	}

	// Walk through task directory
	err := filepath.Walk(taskDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-Go files
		if info.IsDir() || (!strings.HasSuffix(path, ".so") && !strings.HasSuffix(path, ".go")) {
			return nil
		}

		// For .so files, try to load as plugin
		if strings.HasSuffix(path, ".so") {
			taskName := strings.TrimSuffix(filepath.Base(path), ".so")
			if err := loadTaskFromPlugin(tasks, path, taskName); err != nil {
				// Log warning but continue
				fmt.Printf("Warning: Failed to load task plugin %s: %v\n", path, err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan task directory: %w", err)
	}

	return &WatchedTaskList{
		Tasks:   tasks,
		Release: func() {}, // No-op release function for now
	}, nil
}

// loadTaskFromPlugin loads a task handler from a Go plugin file
func loadTaskFromPlugin(tasks map[string]worker.TaskHandler, pluginPath, taskName string) error {
	// Load the plugin
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to open plugin: %w", err)
	}

	// Look for a symbol named "Handler" or the task name
	var symbol plugin.Symbol
	for _, symbolName := range []string{"Handler", taskName, titleCase(taskName)} {
		symbol, err = p.Lookup(symbolName)
		if err == nil {
			break
		}
	}

	if err != nil {
		return fmt.Errorf("failed to find handler symbol in plugin: %w", err)
	}

	// Assert that the symbol is a task handler
	handler, ok := symbol.(worker.TaskHandler)
	if !ok {
		return fmt.Errorf("symbol is not a TaskHandler")
	}

	tasks[taskName] = handler
	return nil
}
