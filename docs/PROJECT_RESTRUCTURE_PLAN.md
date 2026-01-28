# 项目结构整理方案

## 🔍 当前问题

1. **中文文件夹和 zip 文件**
   - `云平台前端/` - 应该删除（已有 frontend/）
   - `云平台前端.zip` - 应该删除

2. **多个 docker-compose 文件**（6 个）
   - `docker-compose.yml` - 主配置
   - `docker-compose.dev.yml` - 开发环境
   - `docker-compose.dev.v1.yml` - 旧版本？
   - `docker-compose.local.yml` - 本地环境
   - `docker-compose.override.yml` - 覆盖配置
   - `docker-compose.test.yml` - 测试环境

3. **根目录文档混乱**
   - `INTEGRATION_SUMMARY.md` - 应该移到 docs/
   - `QUICKSTART.md` - 应该移到 docs/

4. **缺少 README.md**
   - 项目根目录应该有主 README

## 📋 整理方案

### 1. 清理不需要的文件

```bash
# 删除中文文件夹和 zip
rm -rf 云平台前端/
rm -f 云平台前端.zip

# 删除旧版本 docker-compose
rm -f docker-compose.dev.v1.yml
```

### 2. 整理 docker-compose 文件

建议保留：
- `docker-compose.yml` - 生产环境基础配置
- `docker-compose.dev.yml` - 开发环境配置
- `docker-compose.override.yml` - 本地覆盖配置
- `docker-compose.test.yml` - 测试环境配置

删除：
- `docker-compose.local.yml` - 与 override 重复

或者创建 `deploy/` 目录统一管理：
```
deploy/
  ├── docker-compose.yml          # 生产环境
  ├── docker-compose.dev.yml      # 开发环境
  ├── docker-compose.test.yml     # 测试环境
  └── docker-compose.override.yml # 本地覆盖
```

### 3. 整理文档

```bash
# 移动根目录文档到 docs/
mv INTEGRATION_SUMMARY.md docs/
mv QUICKSTART.md docs/

# 创建主 README.md
```

### 4. 推荐的目录结构

```
microgrid-cloud/
├── README.md                    # 项目主文档
├── Makefile                     # 构建脚本
├── Dockerfile                   # Docker 镜像
├── go.mod                       # Go 依赖
├── go.sum
├── main.go                      # 主入口
│
├── .github/                     # GitHub 配置
│   └── workflows/               # CI/CD
│
├── cmd/                         # 命令行工具（可选）
│   └── migrate/                 # 迁移工具
│
├── internal/                    # 内部模块
│   ├── alarms/
│   ├── analytics/
│   ├── api/
│   ├── auth/
│   ├── commands/
│   ├── eventing/
│   ├── masterdata/
│   ├── observability/
│   ├── provisioning/
│   ├── settlement/
│   ├── shadowrun/
│   ├── strategy/
│   ├── tbadapter/
│   └── telemetry/
│
├── migrations/                  # 数据库迁移
│   ├── 001_init.sql
│   ├── 002_settlement.sql
│   └── ...
│
├── scripts/                     # 脚本工具
│   ├── start_dev.sh
│   ├── test_integration.sh
│   ├── configure_tb_integration.sh
│   └── lib_auth.sh
│
├── deploy/                      # 部署配置
│   ├── docker/
│   │   ├── docker-compose.yml
│   │   ├── docker-compose.dev.yml
│   │   ├── docker-compose.test.yml
│   │   └── docker-compose.override.yml.example
│   ├── k8s/                     # Kubernetes 配置
│   ├── prometheus/              # Prometheus 配置
│   └── grafana/                 # Grafana 配置
│
├── docs/                        # 文档
│   ├── README.md                # 文档索引
│   ├── ARCHITECTURE.md          # 架构设计
│   ├── API.md                   # API 文档
│   ├── DEPLOYMENT.md            # 部署指南
│   ├── DEVELOPMENT.md           # 开发指南
│   ├── QUICKSTART.md            # 快速开始
│   ├── INTEGRATION_SUMMARY.md   # 集成总结
│   ├── TB_INTEGRATION_GUIDE.md  # TB 集成指南
│   ├── TB_DATA_FORWARDING.md    # TB 数据转发
│   ├── FRONTEND_INTEGRATION.md  # 前端集成
│   ├── SYSTEM_STATUS_REPORT.md  # 系统状态报告
│   ├── OUTBOX_TROUBLESHOOTING.md # Outbox 故障排查
│   ├── PERF.md                  # 性能优化
│   └── M2_RUNBOOK.md            # 运维手册
│
├── frontend/                    # 前端应用
│   ├── src/
│   ├── public/
│   ├── package.json
│   ├── vite.config.js
│   └── README.md
│
├── tools/                       # 工具代码
│   └── fake_tb_server/          # 模拟 TB 服务器
│
├── alerts/                      # 告警配置
├── dashboards/                  # Dashboard 配置
│
├── .env.example                 # 环境变量示例
├── .gitignore
├── .gitattributes
└── .dockerignore
```

### 5. 执行步骤

```bash
# 1. 清理不需要的文件
rm -rf 云平台前端/
rm -f 云平台前端.zip
rm -f docker-compose.dev.v1.yml
rm -f docker-compose.local.yml

# 2. 创建 deploy 目录
mkdir -p deploy/docker

# 3. 移动 docker-compose 文件
mv docker-compose*.yml deploy/docker/

# 4. 移动文档
mv INTEGRATION_SUMMARY.md docs/
mv QUICKSTART.md docs/

# 5. 创建主 README.md
# (需要编写内容)

# 6. 创建 docs/README.md 索引
# (需要编写内容)

# 7. 更新 .gitignore
echo "docker-compose.override.yml" >> .gitignore
```

## ✅ 整理后的优势

1. **清晰的目录结构** - 一目了然
2. **文档集中管理** - docs/ 目录
3. **部署配置分离** - deploy/ 目录
4. **无中文文件名** - 避免编码问题
5. **标准 Go 项目结构** - 符合社区规范

## 📝 注意事项

1. **docker-compose.override.yml**
   - 这是本地覆盖配置，不应该提交到 Git
   - 应该创建 `.override.yml.example` 作为模板

2. **环境变量**
   - `.env` 不应该提交到 Git
   - 只提交 `.env.example`

3. **前端 node_modules**
   - 确保在 .gitignore 中

4. **迁移顺序**
   - 先备份
   - 逐步迁移
   - 测试每一步
