# 可靠外部 HTTP 通知投递服务

> **HTTP Notification Delivery Service** — 企业内部可靠通知投递中间件（MVP 版本）

---

## 🤖 AI Skills 使用说明

本项目完全由 AI 辅助完成，使用了以下三个 **Codebuddy Skills**：

### 1. `senior-pm-prd-generator` — 高级产品经理 PRD 生成器

**使用步骤**：
1. 向 AI 提供原始需求描述（PDF/文本），包括业务场景和目标说明
2. Skill 自动分析需求，拆分核心功能模块，提出初步架构方案与技术选型建议
3. 输出结构化的 PRD 文档，包含模块划分、数据存储规划、核心技术决策建议，并等待 Tech Lead 确认与裁决

**输出目录**：[`docs/ai_prd.md`](./docs/ai_prd.md)

文档包含：需求分析、初步模块方案（接入层、任务持久化层、投递引擎、重试调度器、死信处理）、数据存储规划、核心技术决策建议（投递语义、重试策略、队列选型）。

---

### 2. `implementation-design-generator` — 内部实现设计文档生成器

**使用步骤**：
1. 向 AI 提供产品需求描述（PRD），包括系统目标、业务场景和约束条件
2. Skill 自动分析功能模块，提出初步架构方案（含技术选型建议）
3. **Tech Lead 审查并裁决**：明确拒绝过度设计（如 Redis 队列、独立日志表），确定 MVP 边界
4. AI 根据裁决结果生成完整的《内部实现设计文档》

**输出目录**：[`docs/design.md`](./docs/design.md)

文档包含：Mermaid 架构图、任务状态机、核心流程时序图、重试流程图、表结构设计、Go 包结构、关键代码骨架、架构演进路线。

---

### 3. `go-unit-test` — Go 单元测试生成器

**使用步骤**：
1. 在 AI 完成核心代码实现后，调用此 Skill 对项目进行单测生成
2. Skill 自动分析各模块的依赖关系，提取 `ITaskRepo` 接口以支持 Mock
3. 使用 `go.uber.org/mock/mockgen` 生成 Mock 代码
4. 为每个核心包生成 Table-Driven 单元测试，覆盖正常路径、错误路径和边界条件
5. 自动运行测试并修复失败用例，直至全部通过

**输出目录**：各测试文件与被测文件同包，具体路径如下：

| 测试文件 | 覆盖率 | 测试重点 |
|---------|--------|---------|
| [`internal/dispatcher/dispatcher_test.go`](./internal/dispatcher/dispatcher_test.go) | 87.3% | 4xx/5xx 状态隔离、指数退避、乐观锁抢占 |
| [`internal/api/handler_test.go`](./internal/api/handler_test.go) | 88.1% | 参数校验、DB 错误、正常响应 |
| [`internal/api/router_test.go`](./internal/api/router_test.go) | 88.1% | 路由注册验证 |
| [`internal/scheduler/scheduler_test.go`](./internal/scheduler/scheduler_test.go) | 100% | 定时调度、僵尸任务恢复 |
| [`internal/repository/task_repo_test.go`](./internal/repository/task_repo_test.go) | 90.5% | 所有 SQL 操作（含乐观锁 UPDATE） |
| [`internal/mock/task_repo_mock.go`](./internal/mock/task_repo_mock.go) | — | 自动生成的 Mock 实现 |

**整体覆盖率：82.1%**

---

## 📁 项目结构

```
rc_xqz/
├── cmd/server/main.go              # 程序入口，初始化 DB、启动服务
├── docs/
│   ├── ai_prd.md                   # PRD 文档（由 senior-pm-prd-generator Skill 生成）
│   └── design.md                   # 内部实现设计文档（由 implementation-design-generator Skill 生成）
├── internal/
│   ├── api/
│   │   ├── handler.go              # HTTP Handler：提交通知、手动重投、查询
│   │   ├── handler_test.go
│   │   ├── router.go               # 路由注册
│   │   └── router_test.go
│   ├── config/config.go            # 配置加载
│   ├── dispatcher/
│   │   ├── dispatcher.go           # 投递引擎：HTTP 请求、4xx/5xx 判断、指数退避
│   │   └── dispatcher_test.go
│   ├── mock/task_repo_mock.go      # 自动生成的 ITaskRepo Mock
│   ├── model/task.go               # NotificationTask 结构体、状态常量
│   ├── repository/
│   │   ├── task_repo.go            # DB 操作封装（含 ITaskRepo 接口）
│   │   └── task_repo_test.go
│   └── scheduler/
│       ├── scheduler.go            # 定时调度器：扫表、分发、僵尸任务恢复
│       └── scheduler_test.go
├── data.db                         # SQLite 数据库（自动创建）
├── go.mod
└── README.md
```

---

## 🚀 快速启动

```bash
# 安装依赖
go mod tidy

# 启动服务（自动创建 data.db 并建表）
go run cmd/server/main.go

# 运行所有单元测试
go test -v ./...

# 查看覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

服务默认监听 `:8080`，提供以下接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/notifications` | 提交通知任务，返回 202 |
| `GET` | `/api/v1/notifications` | 查询任务列表（支持按 status 过滤） |
| `GET` | `/api/v1/notifications/:id` | 查询单个任务详情 |
| `POST` | `/api/v1/notifications/:id/retry` | 手动重投 FAILED 任务 |

---

## 📐 设计问答

### 1. 系统边界

#### ✅ 系统内解决的问题

| 问题 | 处理方式 |
|------|---------|
| 网络级别故障（超时、连接重置） | 捕获错误，触发指数退避重试 |
| 外部服务临时故障（HTTP 5xx） | 视为可重试错误，最多重试 3 次 |
| 服务进程崩溃后的任务恢复 | 任务持久化在 DB，重启后继续扫表投递；`DELIVERING` 状态超时自动恢复为 `RETRYING` |
| 投递结果可查询 | 主表记录 `last_error`、`retry_count`、`status` |
| 人工补偿入口 | 提供 API 查询 `FAILED` 任务并手动触发重投 |

#### ❌ 系统明确不解决的问题

| 问题 | 不解决的原因 |
|------|------------|
| **上游业务系统的鉴权** | 默认由内网 API 网关统一处理，本服务不重复建设 |
| **外部系统的业务错误（HTTP 4xx）** | 属于确定性错误（参数错误/权限问题），重试无意义，直接标记 `FAILED` 且不增加 `retry_count` |
| **Exactly-Once 精确一次投递** | 需要外部供应商配合实现幂等，本服务仅通过 `trace_id` 字段传递去重键，不强制保证 |
| **外部 API 的返回值处理** | 业务系统不关心返回值，本服务仅记录用于排查 |
| **消息顺序保证** | MVP 阶段不保证同一目标的消息有序投递 |

---

### 2. 可靠性与失败处理

#### 投递语义：At-Least-Once（至少一次）

- **保证**：只要任务未达到 `SUCCESS` 或 `FAILED` 终态，系统会持续尝试投递
- **代价**：极端情况下（投递成功但更新 DB 前进程崩溃），可能重复投递
- **应对**：通过 `trace_id` 字段传递去重键，要求外部供应商 API 实现幂等

#### 外部系统失败或长期不可用的处理策略

采用**指数退避重试**，最大重试 **3 次**：

| 重试次数 | 等待时间（base=10s） | 计算公式 |
|---------|-------------------|---------|
| 第 1 次 | 20s | `2¹ × 10s` |
| 第 2 次 | 40s | `2² × 10s` |
| 第 3 次 | 80s | `2³ × 10s` |
| 超过 3 次 | 标记 `FAILED` | 停止主动投递 |

**长期不可用的最终处理**：3 次重试耗尽后，任务状态扭转为 `FAILED`，系统**停止主动投递**，仅提供 API 供人工查阅或手动触发补偿（`POST /api/v1/notifications/:id/retry`）。

**并发安全保障**：调度器通过乐观锁（`UPDATE status='DELIVERING' WHERE id=? AND status IN ('PENDING','RETRYING')`）防止多实例重复投递，`RowsAffected=0` 时直接跳过。

---

### 3. 取舍与演进

#### 被拒绝的"过度设计"

| 被拒绝的设计 | 拒绝原因 |
|------------|---------|
| **引入 Redis 作为队列** | MVP 阶段同时维护 DB + Redis 带来双写一致性问题和额外运维成本，收益不足以覆盖成本 |
| **独立的 `notification_logs` 日志表** | 增加关联写入操作，MVP 阶段直接在主表增加 `last_error`、`retry_count` 字段即可满足排查需求 |
| **消息队列（Kafka/RocketMQ）** | 重型中间件引入门槛高，MVP 阶段扫表方案完全够用，待流量增长后再演进 |
| **独立的死信队列（DLQ）服务** | 过度设计，`FAILED` 状态 + 人工 API 查询已足够，无需独立 DLQ 服务 |

**判断依据**：MVP 阶段的核心目标是**以最低运维成本验证核心链路的可行性**。每引入一个新组件，都意味着额外的部署、监控、故障排查成本。在日通知量未达到瓶颈之前，单一 DB 依赖是最优解。

#### 架构演进路线

```
第一版 MVP                    成长期                      规模期
─────────────────────         ─────────────────────       ─────────────────────
单体 Go 服务                  引入 Redis ZSET             引入 Kafka/RocketMQ
+ SQLite/MySQL 扫表重试   →   延迟队列                →   接入层与投递层
+ 后台 goroutine 调度         减少 DB 扫表压力            彻底解耦为独立微服务
                                                          + DLQ 死信队列
                                                          + 告警平台
```

| 阶段 | 触发条件 | 演进动作 |
|------|---------|---------|
| **MVP** | 日通知量 < 10 万 | 单体服务 + DB 扫表，运维成本最低 |
| **成长期** | 扫表延迟 > 1s 或 DB CPU > 70% | 引入 Redis ZSET 延迟队列，减少 DB 扫表压力 |
| **规模期** | 日通知量 > 1000 万 | 引入 Kafka/RocketMQ，接入层与投递层彻底解耦为独立微服务 |

#### 关于开源中间件的使用

**本项目 MVP 阶段未引入任何消息队列中间件**，理由如下：

- **选择不使用的原因**：消息队列（RabbitMQ/Kafka）引入了额外的部署依赖、网络分区风险和运维复杂度。对于 MVP 阶段，"DB 即队列"的发件箱模式（Outbox Pattern）已能满足可靠性需求，且故障排查更直观。
- **替代方案**：使用关系型数据库（SQLite/MySQL）的 `notification_tasks` 表作为持久化队列，通过定时扫表实现任务调度。
- **未来引入时机**：当 DB 扫表成为性能瓶颈（如 TPS > 1000/s）时，优先考虑引入 **Redis ZSET** 作为轻量延迟队列；当需要多消费者水平扩展时，再迁移至 **Kafka/RocketMQ**。