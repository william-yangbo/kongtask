# Kongtask Cron Implementation

## 概述

本实现成功将 graphile-worker 的 cron 功能同步到 kongtask，遵循 Go 语言的最佳实践。此实现基于 graphile-worker 的 commit cb369ad，提供了完整的分布式 cron 调度功能。

## 核心特性

### 1. 分布式 Cron 调度

- **多实例协调**：通过 PostgreSQL `known_crontabs` 表实现多个 kongtask 实例间的协调
- **防重复执行**：使用时间戳摘要确保同一时间点的任务不会被重复调度
- **故障转移**：任何实例都可以接管 cron 调度，保证高可用性

### 2. 回填 (Backfill) 支持

- **启动时回填**：worker 启动时可以回填过去错过的任务
- **可配置时间窗口**：可设置回填的时间范围
- **避免重复**：智能检测已执行的任务，避免重复回填

### 3. 完整的 Cron 语法支持

- **标准 5 字段格式**：`分钟 小时 日 月 星期`
- **特殊字符支持**：`*`, `/`, `,`, `-` 等
- **时间短语**：支持 `@hourly`, `@daily`, `@weekly` 等
- **选项配置**：支持队列、优先级、最大重试等选项

## 实现架构

### 包结构

```
pkg/cron/
├── types.go       # 核心类型定义
├── parser.go      # Cron 表达式解析器
├── matcher.go     # 时间匹配引擎
├── scheduler.go   # 主调度器实现
└── example/       # 使用示例
```

### 核心接口

#### Scheduler 接口

```go
type Scheduler interface {
    Start(ctx context.Context) error
    Stop() error
    ReloadCronItems() error
    GetCronItems() []ParsedCronItem
}
```

#### Parser 接口

```go
type Parser interface {
    ParseCrontab(crontab string) ([]ParsedCronItem, error)
    ParseCronItems(items []CronItem) ([]ParsedCronItem, error)
    ParseCronItem(item CronItem) (ParsedCronItem, error)
    ParseOptions(optionsJSON string) (CronItemOptions, error)
    ValidatePattern(pattern string) error
}
```

#### Matcher 接口

```go
type Matcher interface {
    Matches(item ParsedCronItem, t time.Time) bool
    DigestTimestamp(t time.Time) TimestampDigest
    GetScheduleTimesInRange(item ParsedCronItem, start, end time.Time) []time.Time
}
```

## 数据库集成

### Migration 000009

```sql
-- 创建 known_crontabs 表
CREATE TABLE known_crontabs (
    identifier text PRIMARY KEY,
    known_since timestamptz NOT NULL DEFAULT now(),
    last_execution text[]
);

-- 性能优化索引
CREATE INDEX known_crontabs_known_since_idx ON known_crontabs(known_since);
CREATE INDEX known_crontabs_last_execution_idx ON known_crontabs USING gin(last_execution);
```

### 与现有表的关系

- 通过 `graphile_worker.jobs` 表创建实际的任务
- 使用 `known_crontabs` 表跟踪 cron 调度状态
- 兼容现有的队列、优先级、重试等机制

## 事件系统集成

### 支持的事件类型

- `cron:starting` - 调度器启动
- `cron:started` - 调度器已启动
- `cron:stopped` - 调度器已停止
- `cron:schedule` - 开始调度任务
- `cron:scheduled` - 任务已调度
- `cron:backfill` - 开始回填
- `cron:overdueTimer` - 检测到延迟执行
- `cron:prematureTimer` - 检测到提前执行

### 事件数据示例

```go
{
    "taskIdentifier": "daily-report",
    "scheduledAt": "2024-01-01T04:00:00Z",
    "digest": "00:04:01:01:01"
}
```

## 使用示例

### 基本配置

```go
config := cron.SchedulerConfig{
    PgPool:             pgPool,
    Schema:             "graphile_worker",
    WorkerUtils:        workerUtils,
    Events:             eventBus,
    Logger:             logger,
    BackfillOnStart:    true,
    ClockSkewTolerance: 10 * time.Second,
}

scheduler := cron.NewScheduler(config)
```

### 启动调度器

```go
ctx := context.Background()
if err := scheduler.Start(ctx); err != nil {
    log.Fatalf("Failed to start cron scheduler: %v", err)
}
defer scheduler.Stop()
```

### Cron 项配置示例

```go
cronItem := cron.CronItem{
    Task:    "daily-report",
    Pattern: "0 4 * * *", // 每天凌晨 4 点
    Options: cron.CronItemOptions{
        QueueName:   "reports",
        Priority:    intPtr(10),
        MaxAttempts: intPtr(3),
        Backfill:    true,
    },
    Payload: map[string]interface{}{
        "type": "daily",
        "format": "pdf",
    },
}
```

## Go 最佳实践

### 1. 接口设计

- 使用小而聚焦的接口
- 支持依赖注入和测试
- 清晰的职责分离

### 2. 错误处理

- 使用 `fmt.Errorf` 包装错误上下文
- 适当的错误级别（Error vs Warn）
- 结构化日志记录

### 3. 并发安全

- 使用 `sync.RWMutex` 保护共享状态
- Context 支持优雅关闭
- 无锁的只读操作

### 4. 内存管理

- 最小化内存分配
- 及时释放资源
- 使用对象池模式（如需要）

### 5. 可测试性

- 接口驱动设计
- 依赖注入
- 时间和随机性的抽象

## 性能考虑

### 1. 数据库查询优化

- 索引优化的查询
- 批量操作减少往返
- 连接池复用

### 2. 内存使用

- 增量加载 cron 项
- 懒加载非关键数据
- 及时清理过期数据

### 3. CPU 使用

- 高效的时间匹配算法
- 最小化字符串操作
- 缓存计算结果

## 与 Graphile-Worker 的对比

| 功能     | Graphile-Worker   | Kongtask     |
| -------- | ----------------- | ------------ |
| 语言     | TypeScript        | Go           |
| 并发模型 | 事件循环          | Goroutines   |
| 类型系统 | 动态 + TypeScript | 静态类型     |
| 错误处理 | Promise/async     | Error values |
| 配置     | JSON/JS 对象      | Go 结构体    |
| 扩展性   | 插件系统          | 接口组合     |

## 部署建议

### 1. 单实例部署

```go
// 简单的单实例配置
scheduler := cron.NewScheduler(cron.SchedulerConfig{
    PgPool:          pgPool,
    WorkerUtils:     workerUtils,
    BackfillOnStart: true,
})
```

### 2. 多实例部署

```go
// 多实例配置，避免竞争
scheduler := cron.NewScheduler(cron.SchedulerConfig{
    PgPool:             pgPool,
    WorkerUtils:        workerUtils,
    ClockSkewTolerance: 30 * time.Second, // 更宽松的时间容差
    BackfillOnStart:    false, // 只有一个实例启用回填
})
```

### 3. 监控配置

```go
// 启用详细事件监控
eventBus.Subscribe(events.EventType("cron:scheduled"), func(ctx context.Context, event events.Event) {
    log.Printf("Cron task scheduled: %+v", event.Data)
})
```

## 下一步工作

### 1. 测试完善

- [ ] 单元测试覆盖
- [ ] 集成测试
- [ ] 性能基准测试
- [ ] 故障恢复测试

### 2. 功能扩展

- [ ] 动态 cron 项管理 API
- [ ] 时区支持
- [ ] 条件执行规则
- [ ] 执行历史查询

### 3. 运维工具

- [ ] 健康检查端点
- [ ] 指标收集
- [ ] 配置热重载
- [ ] 调试工具

### 4. 文档完善

- [ ] API 文档
- [ ] 部署指南
- [ ] 故障排除手册
- [ ] 最佳实践指南

## 结论

这个实现成功地将 graphile-worker 的强大 cron 功能引入到 kongtask 中，同时充分利用了 Go 语言的优势：

1. **类型安全**：编译时错误检查，减少运行时问题
2. **性能优化**：高效的并发模型和内存管理
3. **可维护性**：清晰的接口设计和模块化架构
4. **可扩展性**：基于接口的设计支持未来扩展

该实现为 kongtask 提供了企业级的 cron 调度能力，支持分布式部署、故障恢复和高可用性需求。
