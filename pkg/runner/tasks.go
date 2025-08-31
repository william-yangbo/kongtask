package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"plugin"
	"reflect"
	"strings"

	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TaskDiscovery handles task loading and management (v0.4.0 alignment)
type TaskDiscovery struct {
	taskList map[string]worker.TaskHandler
	// watchers for future watch mode implementation
}

// NewTaskDiscovery creates a new task discovery instance
func NewTaskDiscovery() *TaskDiscovery {
	return &TaskDiscovery{
		taskList: make(map[string]worker.TaskHandler),
	}
}

// LoadFromDirectory loads tasks from a directory (v0.4.0 alignment)
func (td *TaskDiscovery) LoadFromDirectory(directory string) error {
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return fmt.Errorf("task directory does not exist: %s", directory)
	}

	return filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process .go files or .so plugin files
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".so" {
			return nil
		}

		// For .so files, try to load as plugin
		if ext == ".so" {
			return td.loadPluginFile(path)
		}

		// For .go files, we would need to parse the Go code
		// This is complex and typically done at build time
		// For now, we'll document this as a future enhancement
		return nil
	})
}

// LoadFromTaskList loads tasks from a provided task list (v0.4.0 alignment)
func (td *TaskDiscovery) LoadFromTaskList(taskList map[string]worker.TaskHandler) error {
	if taskList == nil {
		return fmt.Errorf("task list cannot be nil")
	}

	for name, handler := range taskList {
		if handler == nil {
			return fmt.Errorf("task handler for '%s' cannot be nil", name)
		}
		td.taskList[name] = handler
	}

	return nil
}

// loadPluginFile loads a task from a Go plugin file
func (td *TaskDiscovery) loadPluginFile(path string) error {
	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	// Try common naming patterns
	basename := strings.TrimSuffix(filepath.Base(path), ".so")
	taskName := strings.ToLower(basename)

	// Look for Handler symbol
	handlerSymbol, err := p.Lookup("Handler")
	if err == nil {
		if handler, ok := handlerSymbol.(worker.TaskHandler); ok {
			td.taskList[taskName] = handler
			return nil
		}
	}

	// Look for TaskHandler symbol
	taskHandlerSymbol, err := p.Lookup("TaskHandler")
	if err == nil {
		if handler, ok := taskHandlerSymbol.(worker.TaskHandler); ok {
			td.taskList[taskName] = handler
			return nil
		}
	}

	// Look for function with specific signature
	taskFuncSymbol, err := p.Lookup("Task")
	if err == nil {
		handlerFunc := reflect.ValueOf(taskFuncSymbol)
		if td.isValidTaskHandler(handlerFunc) {
			// Convert to TaskHandler
			handler := func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
				args := []reflect.Value{
					reflect.ValueOf(ctx),
					reflect.ValueOf(payload),
					reflect.ValueOf(helpers),
				}
				results := handlerFunc.Call(args)
				if len(results) > 0 && !results[0].IsNil() {
					return results[0].Interface().(error)
				}
				return nil
			}
			td.taskList[taskName] = handler
			return nil
		}
	}

	return fmt.Errorf("no valid task handler found in plugin %s", path)
}

// isValidTaskHandler checks if a function has the correct TaskHandler signature
func (td *TaskDiscovery) isValidTaskHandler(fn reflect.Value) bool {
	if fn.Kind() != reflect.Func {
		return false
	}

	fnType := fn.Type()

	// Check number of parameters and return values
	if fnType.NumIn() != 3 || fnType.NumOut() != 1 {
		return false
	}

	// Check parameter types (loosely - we can't import context here easily)
	// This is a simplified check
	if fnType.Out(0).String() != "error" {
		return false
	}

	return true
}

// GetTaskList returns the loaded task list
func (td *TaskDiscovery) GetTaskList() map[string]worker.TaskHandler {
	return td.taskList
}

// GetTaskCount returns the number of loaded tasks
func (td *TaskDiscovery) GetTaskCount() int {
	return len(td.taskList)
}

// HasTask checks if a task exists
func (td *TaskDiscovery) HasTask(name string) bool {
	_, exists := td.taskList[name]
	return exists
}

// assertTaskList creates and validates task list (v0.4.0 alignment)
func assertTaskList(options RunnerOptions, releasers *Releasers) (map[string]worker.TaskHandler, error) {
	td := NewTaskDiscovery()

	if options.TaskList != nil {
		if err := td.LoadFromTaskList(options.TaskList); err != nil {
			return nil, fmt.Errorf("failed to load task list: %w", err)
		}
	} else if options.TaskDirectory != "" {
		if err := td.LoadFromDirectory(options.TaskDirectory); err != nil {
			return nil, fmt.Errorf("failed to load tasks from directory: %w", err)
		}

		// Add cleanup for watchers (future implementation)
		releasers.Add(func() error {
			// TODO: Stop file watchers when implemented
			return nil
		})
	} else {
		return nil, fmt.Errorf("either TaskList or TaskDirectory must be provided")
	}

	taskList := td.GetTaskList()
	if len(taskList) == 0 {
		return nil, fmt.Errorf("no tasks found")
	}

	return taskList, nil
}
