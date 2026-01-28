# 微电网云平台 (Microgrid Cloud Platform)

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

基于 **DDD + CQRS + 事件驱动** 架构的微电网物联网业务平台。

## 🎯 项目定位

将 ThingsBoard 的复杂业务逻辑迁移到独立的 Go 服务中，实现：
- ✅ 业务逻辑可测试、可版本化
- ✅ 模块化设计，高内聚低耦合
- ✅ 事件驱动，异步解耦
- ✅ 水平扩展，高可用

### 架构分层

```
┌─────────────────────────────────────┐
│     ThingsBoard (设备接入层)         │
│  - 设备认证与接入                    │
│  - 遥测数据收发                      │
│  - 指令下发通道                      │
└──────────────┬──────────────────────┘
               │ HTTP Integration
               ▼
┌─────────────────────────────────────┐
│      云平台 (业务逻辑层)             │
│  - 数据聚合与统计                    │
│  - 结算计算                          │
│  - 告警判定                          │
│  - 策略调度                          │
│  - 报表生成                          │
└─────────────────────────────────────┘
```

## 🚀 快速开始

### 前置要求

- Go 1.23+
- Docker & Docker Compose
- PostgreSQL 15+
- Node.js 20+ (前端开发)

### 启动服务

```bash
# 1. 克隆项目
git clone <repository-url>
cd microgrid-cloud

# 2. 启动所有服务
docker-compose -f deploy/docker/docker-compose.dev.yml up -d

# 3. 运行数据库迁移
docker run --rm -v $(pwd)/backend/migrations:/migrations --network host \
  migrate/migrate:v4.16.2 \
  -path=/migrations \
  -database "postgres://postgres:postgres@localhost:5432/microgrid_cloud?sslmode=disable" \
  up

# 4. 启动云平台服务
docker run -d --name microgrid-cloud-platform \
  --network host \
  -v $(pwd)/backend:/workspace \
  -w /workspace \
  -e DATABASE_URL="postgres://postgres:postgres@localhost:5432/microgrid_cloud?sslmode=disable" \
  -e HTTP_ADDR=":8081" \
  -e TB_BASE_URL="http://localhost:8080" \
  -e AUTH_JWT_SECRET="dev-secret-change-me" \
  -e INGEST_HMAC_SECRET="dev-ingest-secret" \
  golang:1.23-alpine \
  sh -c "go run main.go"

# 5. 启动前端
cd frontend
npm install
npm run dev
```

### 访问地址

- **前端界面**: http://localhost:5173
- **后端 API**: http://localhost:8081
- **ThingsBoard**: http://localhost:8080
- **健康检查**: http://localhost:8081/healthz
- **Metrics**: http://localhost:8081/metrics

## 📚 文档

- [快速开始](docs/QUICKSTART.md) - 5 分钟上手指南
- [系统状态报告](docs/SYSTEM_STATUS_REPORT.md) - 当前实现状态
- [架构设计](docs/ARCHITECTURE.md) - 详细架构说明
- [API 文档](docs/API.md) - API 接口文档
- [部署指南](docs/DEPLOYMENT.md) - 生产环境部署
- [开发指南](docs/DEVELOPMENT.md) - 开发环境配置
- [TB 集成指南](docs/TB_INTEGRATION_GUIDE.md) - ThingsBoard 集成
- [前端集成](docs/FRONTEND_INTEGRATION.md) - 前后端集成
- [运维手册](docs/M2_RUNBOOK.md) - 运维操作手册

## 🏗️ 项目结构

```
microgrid-cloud/
├── backend/              # 后端服务
│   ├── internal/        # 内部模块
│   │   ├── alarms/     # 告警模块
│   │   ├── analytics/  # 统计分析
│   │   ├── commands/   # 指令下发
│   │   ├── eventing/   # 事件驱动
│   │   ├── settlement/ # 结算计算
│   │   ├── strategy/   # 策略调度
│   │   ├── telemetry/  # 遥测数据
│   │   └── ...
│   ├── migrations/     # 数据库迁移
│   ├── tools/          # 工具代码
│   ├── main.go         # 主入口
│   ├── go.mod          # Go 依赖
│   └── Dockerfile      # Docker 镜像
├── frontend/           # 前端应用
│   ├── src/           # 源代码
│   ├── public/        # 静态资源
│   ├── package.json   # NPM 依赖
│   └── vite.config.js # Vite 配置
├── deploy/            # 部署配置
│   ├── docker/       # Docker Compose
│   ├── k8s/          # Kubernetes
│   ├── prometheus/   # Prometheus 配置
│   └── grafana/      # Grafana 配置
├── docs/             # 文档
├── scripts/          # 脚本工具
├── alerts/           # 告警配置
├── dashboards/       # Dashboard 配置
└── README.md         # 项目主文档
```

## ✨ 核心功能

### 已实现模块

- ✅ **Telemetry** - 遥测接收与存储
- ✅ **Analytics** - 统计聚合（小时/日/月）
- ✅ **Settlement** - 结算计算与账单
- ✅ **Eventing** - 事件驱动 + Outbox
- ✅ **Alarms** - 告警判定与通知
- ✅ **Commands** - 指令下发
- ✅ **Strategy** - 策略调度
- ✅ **Masterdata** - 站点/设备管理
- ✅ **Shadowrun** - 策略回测
- ✅ **Auth/IAM** - JWT 认证
- ✅ **Observability** - 监控指标
- ✅ **Frontend** - React 前端界面

### 核心闭环

**数据链路**: TB 设备 → 云平台 Ingest → 存储 → 统计 → 结算 → 前端展示
**控制链路**: 策略配置 → 定时调度 → 指令生成 → TB RPC → 设备执行
**告警链路**: 告警规则 → 遥测触发 → 判定 → Webhook/SSE 通知

## 🛠️ 技术栈

**后端**:
- Go 1.23
- PostgreSQL 15 (分区表)
- NATS (消息队列)
- Prometheus (监控)

**前端**:
- React 18
- Vite 5
- 原生 CSS

**架构模式**:
- DDD (领域驱动设计)
- CQRS (命令查询职责分离)
- Event Sourcing (事件溯源)
- Outbox Pattern (可靠消息投递)

## 🔧 开发

### 运行测试

```bash
# 单元测试
go test ./...

# 集成测试
go test -tags=integration ./...

# 测试覆盖率
go test -cover ./...
```

### 代码规范

```bash
# 格式化代码
go fmt ./...

# 静态检查
go vet ./...

# Lint
golangci-lint run
```

### 生成 JWT Token

```bash
source scripts/lib_auth.sh
jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600
```

## 📊 监控

- **Prometheus Metrics**: http://localhost:8081/metrics
- **健康检查**: http://localhost:8081/healthz
- **Grafana Dashboard**: http://localhost:3000 (需启动)

## 🤝 贡献

欢迎贡献代码、报告问题或提出建议！

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 📝 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

## 📞 联系方式

- 项目主页: [GitHub Repository]
- 问题反馈: [GitHub Issues]
- 文档: [docs/](docs/)

---

**当前版本**: v0.1.0 (MVP)
**最后更新**: 2026-01-28
