# CI 优化方案 - 降低 GitHub Actions 费用

## 🎯 优化目标

将原本耗时的 CI 流程优化，从 **6 个并行矩阵任务** 减少到 **快速反馈机制**，预计可节省 **70-80%** 的 CI 费用。

## 📊 原始 CI 问题分析

### 成本问题

1. **矩阵过度使用**: 2 Go 版本 × 3 PostgreSQL 版本 = 6 个并行 job
2. **重复工具**: staticcheck + golint + golangci-lint 都在运行
3. **每次都运行性能测试**: 即使是文档修改也运行
4. **安全扫描重复**: CodeQL + Gosec 每次都运行
5. **无条件运行**: 所有检查对所有变更都运行

### 时间成本

- 原始配置：~15-20 分钟 × 6 个 job = 90-120 分钟总耗时
- 优化后配置：~5-8 分钟总耗时

## ✅ 优化方案

### 1. 主要 CI 流程 (`ci.yml`)

**理念**: 快速反馈 + 智能触发

```yaml
Jobs:
├── quick-checks (2-3分钟)     # 快速检查，立即反馈
│   ├── go mod verify
│   ├── go build
│   ├── go vet
│   └── golangci-lint --fast
├── test (3-5分钟)            # 核心测试，单一版本
│   ├── Go 1.23 + PostgreSQL 16
│   ├── 单元测试 + 集成测试
│   └── 代码覆盖率上传
├── performance (条件运行)      # 仅在相关变更时运行
│   └── 仅当修改 perftest/ 或 pkg/worker/ 时
└── security (仅main分支)      # 轻量安全扫描
    └── 仅Gosec，无SARIF上传
```

**优化效果**:

- **减少 90%运行时间**: 从 6 个 job 并行 → 顺序执行 3-4 个 job
- **条件执行**: 性能测试仅在相关文件变更时运行
- **工具整合**: 一个 golangci-lint 替代多个 linting 工具

### 2. 周度完整测试 (`weekly-full-test.yml`)

**理念**: 定期保证质量

```yaml
触发条件:
├── 每周日凌晨2点自动运行
└── 手动触发（workflow_dispatch）

Jobs:
├── full-matrix-test          # 完整兼容性矩阵
│   └── 2 Go版本 × 3 PostgreSQL版本
├── comprehensive-security    # 完整安全扫描
│   ├── golangci-lint (完整模式)
│   ├── CodeQL Analysis
│   └── Gosec + SARIF上传
└── performance-benchmark     # 性能基准测试
    ├── 性能测试套件
    └── Go benchmarks
```

**优化效果**:

- **减少 95%日常运行**: 大部分繁重检查移到周度运行
- **保持质量**: 定期全面检查确保没有回归
- **成本控制**: 只在真正需要时运行完整测试套件

### 3. 可选的兼容性测试

```yaml
# 仅在以下情况运行:
- main分支推送
- 修改了.go文件
- 手动触发

# 测试范围:
- Go 1.22 (向后兼容)
- PostgreSQL 14, 15 (旧版本兼容)
```

## 💰 成本节省计算

### 原始配置 (每次运行)

```
矩阵任务: 6 jobs × 15分钟 = 90分钟
性能测试: 1 job × 10分钟 = 10分钟
Lint任务: 1 job × 8分钟 = 8分钟
安全扫描: 2 jobs × 12分钟 = 24分钟
总计: 132分钟/次
```

### 优化配置 (每次运行)

```
快速检查: 1 job × 3分钟 = 3分钟
核心测试: 1 job × 5分钟 = 5分钟
性能测试: 0.2次 × 8分钟 = 1.6分钟 (条件运行)
安全扫描: 0.5次 × 2分钟 = 1分钟 (仅main分支)
总计: 10.6分钟/次
```

### 节省率

- **日常节省**: (132 - 10.6) / 132 = **92%**
- **考虑周度测试**: 假设每周 30 次提交
  - 原始: 30 × 132 = 3960 分钟/周
  - 优化: 30 × 10.6 + 132 = 450 分钟/周
  - **总体节省**: **89%**

## 🚀 使用方法

### 1. 替换现有 CI

```bash
# 备份原始配置
mv .github/workflows/ci.yml .github/workflows/ci-original.yml.bak

# 使用优化配置
# ci.yml 已经更新为优化版本
```

### 2. 添加周度测试

```bash
# weekly-full-test.yml 已创建
# 将在每周日自动运行
```

### 3. 监控效果

- 观察 PR 检查时间: 应该从 15-20 分钟降到 5-8 分钟
- 查看 Actions 使用量: 应该减少 80-90%
- 确保测试覆盖率保持: Codecov 报告应该一致

## 🔧 进一步优化建议

### 1. 缓存优化

```yaml
# 已启用Go模块缓存
cache: true

# 可考虑添加其他缓存
- name: Cache golangci-lint
  uses: actions/cache@v4
  with:
    path: ~/.cache/golangci-lint
    key: golangci-lint-${{ runner.os }}-${{ hashFiles('**/*.go') }}
```

### 2. 条件优化

```yaml
# 根据文件变更类型决定运行内容
- if: contains(github.event.head_commit.modified, '.md')
  # 文档变更时跳过测试

- if: contains(github.event.head_commit.modified, 'go.mod')
  # 依赖变更时运行安全扫描
```

### 3. 并行优化

```yaml
# 可以将某些独立检查并行运行
jobs:
  quick-checks:
    # lint检查
  quick-build:
    # 构建检查
  # 两个job可以并行运行
```

## 📈 监控指标

定期检查以下指标确保优化效果:

1. **CI 运行时间**: 目标 < 8 分钟
2. **Actions 计费时间**: 减少 80%+
3. **失败检测时间**: 快速检查应在 3 分钟内检测到问题
4. **测试覆盖率**: 保持现有水平
5. **安全检测**: 每周全面扫描无遗漏

这个优化方案在保持代码质量的同时，大幅降低了 CI 成本，特别适合频繁提交的开发团队。
