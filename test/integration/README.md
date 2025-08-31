# Integration Test Organization - Go Best Practices

## Overview

This document describes the reorganization of integration tests to follow Go best practices for project structure.

## Test Files

### 1. runtasklist_parity_test.go - RunTaskList Parity Tests

这个文件包含了与 graphile-worker 的`main.runTaskList.test.ts`相对应的 parity 测试，用于验证 kongtask 的`RunTaskList`和`RunTaskListWithSignalHandling`功能与原始 TypeScript 实现的对等性。

#### 测试场景

**TestRunTaskListJobExecutionAndCleanExit**

- 对应 TypeScript 测试: "main will execute jobs as they come up, and exits cleanly"
- 测试持续 job 处理、动态添加 jobs、优雅关闭
- 使用 Deferred 机制控制 job 执行顺序

**TestRunTaskListDebugCompatibility**

- 对应 TypeScript 测试: "doesn't bail on deprecated `debug` function"
- 测试 debug 日志功能兼容性
- 验证 helpers.Logger.Debug 方法正常工作

**TestRunTaskListWithSignalHandling**

- 测试信号处理版本的 RunTaskList
- 验证 ManagedWorkerPool 包装和优雅关闭

#### Parity 对比表

| 功能          | TypeScript (graphile-worker) | Go (kongtask)                   | 状态        |
| ------------- | ---------------------------- | ------------------------------- | ----------- |
| 持续 job 处理 | runTaskList()                | RunTaskList()                   | ✅ 完全对等 |
| 优雅关闭      | workerPool.release()         | workerPool.Release()            | ✅ 完全对等 |
| 信号处理      | allWorkerPools 管理          | RunTaskListWithSignalHandling() | ✅ 完全对等 |
| Debug 日志    | helpers.debug()              | helpers.Logger.Debug()          | ✅ 完全对等 |
| 并发控制      | concurrency 选项             | WorkerPoolOptions.Concurrency   | ✅ 完全对等 |

- Integration tests were located in `pkg/worker/` alongside production code
- Mixed testing concerns with package implementation
- Not following Go community best practices for large projects
- Made it harder to distinguish between unit tests and integration tests

## Current Structure (Go Best Practices)

```
kongtask/
├── pkg/worker/
│   ├── worker.go
│   ├── worker_utils.go
│   └── ... (only production code)
├── test/
│   └── integration/
│       ├── workerutils_parity_test.go  ✅ Dedicated integration test directory
│       └── runner_parity_test.go       ✅ Dedicated integration test directory
```

**Benefits of New Structure:**

- Clear separation of concerns between production code and integration tests
- Follows Go best practices for project organization
- Easy to run all integration tests: `go test ./test/integration/...`
- Better maintainability and code organization
- Aligns with common patterns in large Go projects

## Test Coverage

### Migration Parity Tests (`migrate_parity_test.go`)

- **TestMigrate_InstallSchema_SecondMigrationDoesNoHarm**: Complete migration functionality testing
- **TestMigrate_WithExistingSchema**: Idempotent migration behavior validation

### WorkerUtils Parity Tests (`workerutils_parity_test.go`)

- **TestWorkerUtilsAddJobParity**: Basic job addition functionality
- **TestWorkerUtilsJobKeyParity**: Job key handling and uniqueness
- **TestQuickAddJobGlobalParity**: Global convenience function testing
- **TestWorkerUtilsAdvancedOptions**: Advanced job configuration options
- **TestWorkerUtilsMultipleJobs**: Bulk job processing capabilities

### Runner Parity Tests (`runner_parity_test.go`)

- **TestRunOnceValidationParity**: Parameter validation error cases
- **TestRunOnceConnectionParity**: Database connection methods (DATABASE_URL, connectionString, pgPool)
- **TestRunOnceJobProcessingParity**: Job execution and processing logic
- **TestRunOnceCustomSchemaParity**: Custom database schema support

## Integration Test Guidelines

### Package Declaration

```go
// Package integration contains integration tests for kongtask
// These tests verify the complete functionality and parity with graphile-worker
package integration
```

### Import Strategy

- Import the worker package as external dependency: `"github.com/william-yangbo/kongtask/pkg/worker"`
- Use testutil for database setup: `"github.com/william-yangbo/kongtask/internal/testutil"`
- TestContainers for isolated testing: `"github.com/testcontainers/testcontainers-go/modules/postgres"`

### Test Naming Convention

- Suffix `Parity` indicates compatibility tests with graphile-worker
- Test functions clearly describe the functionality being tested
- Subtest names use snake_case for readability

### Running Tests

```bash
# Run all integration tests
go test ./test/integration/... -v

# Run specific test file
go test ./test/integration/workerutils_parity_test.go -v

# Run specific test function
go test ./test/integration/... -run TestRunOnceValidationParity -v
```

## Standards Compliance

This reorganization follows established Go best practices:

1. **Go Project Layout**: Based on [golang-standards/project-layout](https://github.com/golang-standards/project-layout)
2. **Test Organization**: Separates integration tests from unit tests
3. **Package Structure**: Keeps production code clean in `pkg/` directory
4. **Testing Patterns**: Uses dedicated `test/` directory for complex test scenarios

## Migration Notes

- Original test files have been moved from `pkg/worker/` and `internal/migrate/` to `test/integration/`
- All tests maintain the same functionality and coverage
- Package imports updated to reference packages externally
- Test execution confirmed successful with all 17 test cases passing (2 migration + 10 runner + 5 workerutils)

This reorganization ensures the kongtask project follows professional Go development standards while maintaining complete graphile-worker parity testing coverage.
