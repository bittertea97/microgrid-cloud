# 项目结构说明

## 📁 目录结构

```
microgrid-cloud/
├── backend/                    # 后端服务（Go）
│   ├── internal/              # 内部模块（DDD 领域模块）
│   │   ├── alarms/           # 告警模块
│   │   │   ├── domain/       # 领域模型
│   │   │   ├── application/  # 应用服务
│   │   │   ├── infrastructure/ # 基础设施（数据库、外部服务）
│   │   │   └── interfaces/   # 接口层（HTTP、事件）
│   │   ├── analytics/        # 统计分析模块
│   │   ├── api/              # API 查询接口
│   │   ├── audit/            # 审计日志
│   │   ├── auth/             # 认证授权
│   │   ├── commands/         # 指令下发模块
│   │   ├── eventing/         # 事件驱动基础设施
│   │   ├── masterdata/       # 主数据（站点、设备）
│   │   ├── observability/    # 可观测性（Metrics）
│   │   ├── provisioning/     # 站点配置
│   │   ├── settlement/       # 结算计算模块
│   │   ├── shadowrun/        # 策略回测模块
│   │   ├── strategy/         # 策略调度模块
│   │   ├── tbadapter/        # ThingsBoard 适配器
│   │   └── telemetry/        # 遥测数据模块
│   ├── migrations/           # 数据库迁移文件
│   │   ├── 001_init.sql
│   │   ├── 002_settlement.sql
│   │   └── ...
│   ├── tools/                # 工具代码
│   │   └── fake_tb_server/  # 模拟 TB 服务器
│   ├── main.go              # 主入口
│   ├── go.mod               # Go 依赖管理
│   ├── go.sum
│   ├── Dockerfile           # Docker 镜像构建
│   ├── Makefile             # 构建脚本
│   └── .dockerignore
│
├── frontend/                 # 前端应用（React + Vite）
│   ├── src/                 # 源代码
│   │   ├── App.jsx         # 主应用组件
│   │   ├── main.jsx        # 入口文件
│   │   └── styles.css      # 样式文件
│   ├── public/              # 静态资源
│   ├── index.html           # HTML 模板
│   ├── package.json         # NPM 依赖
│   ├── vite.config.js       # Vite 配置
│   ├── .env                 # 环境变量
│   └── README.md            # 前端文档
│
├── deploy/                   # 部署配置
│   ├── docker/              # Docker Compose 配置
│   │   ├── docker-compose.yml           # 生产环境
│   │   ├── docker-compose.dev.yml       # 开发环境
│   │   ├── docker-compose.test.yml      # 测试环境
│   │   └── docker-compose.override.yml.example  # 本地覆盖示例
│   ├── k8s/                 # Kubernetes 配置（待完善）
│   ├── prometheus/          # Prometheus 配置
│   │   └── prometheus.yml
│   └── grafana/             # Grafana 配置
│       └── provisioning/
│
├── docs/                     # 文档
│   ├── README.md            # 文档索引
│   ├── QUICKSTART.md        # 快速开始
│   ├── SYSTEM_STATUS_REPORT.md  # 系统状态报告
│   ├── TB_INTEGRATION_GUIDE.md  # TB 集成指南
│   ├── TB_DATA_FORWARDING.md    # TB 数据转发
│   ├── FRONTEND_INTEGRATION.md  # 前端集成
│   ├── INTEGRATION_SUMMARY.md   # 集成总结
│   ├── OUTBOX_TROUBLESHOOTING.md # Outbox 故障排查
│   ├── PERF.md              # 性能优化
│   ├── M2_RUNBOOK.md        # 运维手册
│   └── PROJECT_STRUCTURE.md # 本文档
│
├── scripts/                  # 脚本工具
│   ├── start_dev.sh         # 启动开发环境
│   ├── test_integration.sh  # 集成测试
│   ├── configure_tb_integration.sh  # 配置 TB 集成
│   ├── start_cloud_platform.sh      # 启动云平台
│   └── lib_auth.sh          # 认证工具库
│
├── alerts/                   # 告警配置
├── dashboards/               # Grafana Dashboard 配置
│
├── .github/                  # GitHub 配置
│   └── workflows/           # CI/CD 工作流
│
├── README.md                 # 项目主文档
├── .gitignore               # Git 忽略文件
├── .gitattributes           # Git 属性配置
├── .env.example             # 环境变量示例
└── LICENSE                  # 许可证
```

## 🏛️ 架构分层

### 后端架构（DDD + CQRS）

每个领域模块遵循以下分层：

```
module/
├── domain/              # 领域层
│   ├── entity.go       # 实体
│   ├── aggregate.go    # 聚合根
│   ├── value_object.go # 值对象
│   ├── repository.go   # 仓储接口
│   └── service.go      # 领域服务
├── application/         # 应用层
│   ├── service.go      # 应用服务
│   ├── command.go      # 命令
│   ├── query.go        # 查询
│   └── events/         # 事件处理器
├── infrastructure/      # 基础设施层
│   └── postgres/       # PostgreSQL 实现
│       ├── repository.go
│       └── query.go
└── interfaces/          # 接口层
    ├── http/           # HTTP 接口
    │   └── handler.go
    └── events/         # 事件订阅
        └── consumer.go
```

### 前端架构（React）

```
frontend/
├── src/
│   ├── components/     # 可复用组件（待拆分）
│   ├── pages/          # 页面组件（待拆分）
│   ├── services/       # API 服务（待拆分）
│   ├── utils/          # 工具函数（待拆分）
│   ├── App.jsx         # 主应用
│   ├── main.jsx        # 入口
│   └── styles.css      # 样式
└── public/             # 静态资源
```

## 📦 模块说明

### 后端核心模块

| 模块 | 职责 | 主要功能 |
|------|------|----------|
| **telemetry** | 遥测数据 | 接收、存储、查询遥测数据 |
| **analytics** | 统计分析 | 小时/日/月统计聚合 |
| **settlement** | 结算计算 | 日结算、账单生成 |
| **alarms** | 告警判定 | 规则引擎、告警通知 |
| **commands** | 指令下发 | 指令创建、TB RPC 调用 |
| **strategy** | 策略调度 | 策略执行、定时调度 |
| **eventing** | 事件驱动 | Outbox、事件总线、幂等处理 |
| **masterdata** | 主数据 | 站点、设备、点位映射 |
| **auth** | 认证授权 | JWT、租户隔离 |
| **observability** | 可观测性 | Metrics、健康检查 |

### 基础设施模块

| 模块 | 职责 |
|------|------|
| **eventing** | 事件驱动基础设施（Outbox、DLQ） |
| **audit** | 审计日志 |
| **observability** | 监控指标、健康检查 |
| **tbadapter** | ThingsBoard 适配器（ACL） |

## 🔄 数据流

### 遥测数据流

```
TB 设备 → TB → HTTP Integration → /ingest/thingsboard/telemetry
  → telemetry 模块 → telemetry_points 表
  → TelemetryReceived 事件 → Outbox
  → alarms 模块（实时告警）
  → 窗口关闭触发 → analytics 模块
  → 小时统计 → StatisticCalculated(HOUR) 事件
  → 日汇总 → StatisticCalculated(DAY) 事件
  → settlement 模块 → 日结算
  → SettlementCalculated 事件
```

### 控制数据流

```
策略配置 → strategy 模块 → 定时调度
  → 策略执行 → commands 模块
  → CommandIssued 事件 → tbadapter
  → TB RPC 调用 → 设备执行
  → 回执 → CommandAcked/CommandFailed 事件
```

## 🗄️ 数据库设计

### 核心表

| 表名 | 说明 | 分区 |
|------|------|------|
| `telemetry_points` | 遥测数据（时序） | 按日分区 |
| `analytics_statistics` | 统计结果 | 无 |
| `settlements_day` | 日结算 | 无 |
| `stations` | 站点主数据 | 无 |
| `devices` | 设备主数据 | 无 |
| `point_mappings` | 点位映射 | 无 |
| `alarms` | 告警记录 | 无 |
| `alarm_rules` | 告警规则 | 无 |
| `commands` | 指令记录 | 无 |
| `strategies` | 策略配置 | 无 |
| `outbox_events` | Outbox 事件 | 无 |
| `processed_events` | 已处理事件（幂等） | 无 |
| `dlq_events` | 死信队列 | 无 |

## 🚀 部署架构

### 开发环境

```
Docker Compose:
  - postgres (5432)
  - nats (4222)
  - minio (9002)
  - thingsboard (fake, 18080)
  - app (8081)
  - frontend (5173)
```

### 生产环境（规划）

```
Kubernetes:
  - Backend Deployment (多副本)
  - Frontend Deployment (Nginx)
  - PostgreSQL StatefulSet
  - NATS Cluster
  - Prometheus + Grafana
```

## 📝 命名规范

### Go 代码

- 包名：小写，单数，简短（如 `alarm`, `analytics`）
- 文件名：小写，下划线分隔（如 `alarm_service.go`）
- 接口：名词或形容词（如 `Repository`, `Notifier`）
- 实现：接口名 + 实现方式（如 `PostgresRepository`）

### 数据库

- 表名：小写，下划线分隔，复数（如 `alarm_rules`）
- 列名：小写，下划线分隔（如 `station_id`）
- 索引：`idx_表名_列名`（如 `idx_alarms_station`）

### 前端

- 组件：PascalCase（如 `StationList.jsx`）
- 文件：camelCase（如 `apiService.js`）
- CSS 类：kebab-case（如 `.station-card`）

## 🔗 相关文档

- [快速开始](QUICKSTART.md)
- [系统状态报告](SYSTEM_STATUS_REPORT.md)
- [API 文档](API.md)（待完善）
- [部署指南](DEPLOYMENT.md)（待完善）

---

**最后更新**: 2026-01-28
