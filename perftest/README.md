# KongTask Performance Testing Suite

这个性能测试套件使用 TestContainers 来提供真实的 PostgreSQL 环境，对 KongTask 进行全面的性能测试。它完全对应了原始 graphile-worker 的 perfTest。

## 测试概述

### 🎯 测试对应关系

| 原始 perfTest (v0.4.0) | KongTask 测试                   | 描述                       |
| ---------------------- | ------------------------------- | -------------------------- |
| `init.js`              | `TestParallelWorkerPerformance` | 动态调度 20,000 个作业     |
| `run.js`               | `TestParallelWorkerPerformance` | 4 个并行 worker，10 个并发 |
| `latencyTest.js`       | `TestLatencyPerformance`        | 测试作业延迟性能           |
| `recreateDb.js`        | TestContainers 自动管理         | 每个测试自动重建数据库     |
| `tasks/log_if_999.js`  | `log_if_999` 任务处理器         | 当 ID 为 999 时记录日志    |

### 🧪 测试套件

1. **BasicPerformance** - 基础性能测试

   - 使用 WorkerUtils (v0.4.0) 调度作业
   - 验证基本处理能力
   - 测量基础吞吐量

2. **JobKeyPerformance** - JobKey 性能测试 (v0.4.0 新功能)

   - 测试 JobKey 去重功能
   - 验证 1000 次重复调度只创建 1 个作业
   - 确保 JobKey 特性正常工作

3. **StringIDPerformance** - 字符串 ID 性能测试 (v0.4.0 变更)

   - 验证 Job.ID 从 int64 改为 string
   - 确保 ID 唯一性和性能
   - 测试字符串 ID 处理效率

4. **BulkJobsPerformance** - 批量作业性能测试

   - 调度 1,000 个作业 (简化版)
   - 单个工作器处理
   - 测量调度和处理时间

5. **ParallelWorkerPerformance** - 并行工作器性能测试 (v0.4.0 对齐)

   - **精确对应 v0.4.0 run.js**
   - 20,000 个作业 (JOB_COUNT)
   - 4 个并行工作器 (PARALLELISM)
   - 10 个并发度 (CONCURRENCY)
   - 查找目标作业 (id=999)

6. **LatencyPerformance** - 延迟性能测试

   - 测量 50 个作业的端到端延迟
   - 单个工作器精确测量
   - 提供平均延迟统计

7. **StartupShutdownPerformance** - 启动/关闭性能测试

   - 测量工作器启动时间
   - 测量工作器关闭时间
   - 多次迭代平均值

8. **MemoryPerformance** - 内存性能测试
   - 测试内存使用效率
   - 处理大量作业时的内存稳定性

## 🔧 技术实现

### TestContainers 集成

- **PostgreSQL 15 Alpine**: 轻量级容器化数据库
- **自动迁移**: 每个测试自动设置 kongtask 架构
- **隔离环境**: 每个测试套件独立的 PostgreSQL 实例
- **自动清理**: 测试完成后自动清理容器

### 关键特性

- **真实数据库**: 使用真实 PostgreSQL 而非模拟
- **并发安全**: 安全的并发测试和数据收集
- **精确测量**: 高精度时间测量
- **全面断言**: 性能期望验证

## 🚀 运行测试

### 前置条件

```bash
# 确保Docker正在运行
docker --version

# 安装依赖
go mod tidy
```

### 运行所有性能测试

```bash
cd perftest
go test -v
```

### 运行特定测试

```bash
# 批量作业性能测试
go test -v -run TestBulkJobsPerformance

# 延迟性能测试
go test -v -run TestLatencyPerformance

# 启动/关闭性能测试
go test -v -run TestStartupShutdownPerformance

# 并发性能测试
go test -v -run TestConcurrencyPerformance
```

### 运行长时间测试

```bash
# 增加超时时间进行压力测试
go test -v -timeout 30m
```

## 📊 性能基准

### 预期性能指标

| 指标     | 目标值         | 描述                    |
| -------- | -------------- | ----------------------- |
| 平均延迟 | <100ms         | 作业端到端处理延迟      |
| P95 延迟 | <200ms         | 95%作业在此时间内完成   |
| 启动时间 | <1s            | 工作器启动时间          |
| 关闭时间 | <5s            | 工作器优雅关闭时间      |
| 处理速率 | >1000 jobs/sec | 10 个工作器的总处理速率 |

### 示例输出

```
=== RUN   TestBulkJobsPerformance
    performance_test.go:125: Starting bulk jobs performance test...
    performance_test.go:132: Job scheduling completed in: 2.345s
    performance_test.go:133: Total jobs scheduled: 20000
    performance_test.go:159: Job processing completed in: 15.678s
    performance_test.go:160: Average processing rate: 1275.32 jobs/second
    performance_test.go:162: Per-worker rate: 127.53 jobs/second
--- PASS: TestBulkJobsPerformance (18.45s)

=== RUN   TestLatencyPerformance
    performance_test.go:169: Starting latency performance test...
    performance_test.go:196: Warming up...
    performance_test.go:207: Beginning latency measurement...
    performance_test.go:230: Latency Results (1000 samples):
    performance_test.go:231:   Min: 15.23ms
    performance_test.go:232:   Max: 89.45ms
    performance_test.go:233:   Avg: 32.67ms
    performance_test.go:234:   P50: 28.34ms
    performance_test.go:235:   P95: 56.78ms
    performance_test.go:236:   P99: 78.90ms
--- PASS: TestLatencyPerformance (45.67s)
```

## 🔍 测试分析

### 与原始 graphile-worker 对比

- **语言优势**: Go 的原生并发性能优于 Node.js
- **内存效率**: 更低的内存占用和 GC 压力
- **类型安全**: 编译时错误检查
- **容器化**: TestContainers 提供更可靠的测试环境

### 性能优化建议

1. **连接池**: 调整 pgxpool 配置优化数据库连接
2. **并发级别**: 根据 CPU 核心数调整工作器数量
3. **批处理**: 考虑批量作业插入优化
4. **索引**: 确保适当的数据库索引

## 🐛 故障排除

### 常见问题

1. **Docker 未运行**

   ```
   Error: Cannot connect to the Docker daemon
   ```

   **解决**: 启动 Docker Desktop 或 Docker 服务

2. **端口冲突**

   ```
   Error: port already allocated
   ```

   **解决**: TestContainers 会自动分配端口，重试即可

3. **测试超时**

   ```
   Error: context deadline exceeded
   ```

   **解决**: 增加测试超时时间或检查系统性能

4. **内存不足**
   ```
   Error: cannot allocate memory
   ```
   **解决**: 减少并发级别或增加系统内存

## 📝 扩展测试

### 添加自定义测试

```go
func TestCustomPerformance(t *testing.T) {
    suite := SetupPerfTestSuite(t)
    defer suite.Cleanup(t)

    // 自定义性能测试逻辑
    w := suite.createWorker()
    w.RegisterTask("custom_task", func(ctx context.Context, job *worker.Job) error {
        // 自定义任务处理逻辑
        return nil
    })

    // 测试实现...
}
```

### 集成 CI/CD

```yaml
# .github/workflows/performance.yml
name: Performance Tests
on: [push, pull_request]
jobs:
  performance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.22'
      - name: Run Performance Tests
        run: |
          cd perftest
          go test -v -timeout 30m
```

这个性能测试套件提供了全面的性能验证，确保 KongTask 在各种负载条件下都能保持优秀的性能表现！
