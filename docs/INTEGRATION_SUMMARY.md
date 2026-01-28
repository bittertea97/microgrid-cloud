# 前后端集成完成总结

## 完成的工作

### 1. 后端修改 (main.go)

✅ **添加 CORS 中间件**
- 支持跨域请求
- 允许 GET, POST, PUT, DELETE, OPTIONS 方法
- 允许 Content-Type 和 Authorization 请求头
- 支持凭证传递

位置: `main.go:480-495`

### 2. 前端配置

✅ **修正 API 地址**
- 将默认 API 地址从 `http://localhost:8081` 改为 `http://localhost:8080`
- 文件: `frontend/src/App.jsx:3`

✅ **创建环境配置文件**
- 文件: `frontend/.env`
- 内容: `VITE_API_BASE_URL=http://localhost:8080`

✅ **更新 Vite 配置**
- 添加 `host: '0.0.0.0'` 支持 Docker 访问
- 文件: `frontend/vite.config.js`

✅ **更新 README**
- 修正文档中的端口号
- 添加 Docker Compose 使用说明
- 文件: `frontend/README.md`

### 3. Docker Compose 集成

✅ **添加前端服务**
- 使用 Node.js 20 Alpine 镜像
- 自动安装依赖并启动开发服务器
- 映射端口 5173
- 文件: `docker-compose.dev.yml`

### 4. 文档和脚本

✅ **集成指南**
- 完整的前后端集成文档
- 包含启动方式、API 说明、认证方法、故障排查
- 文件: `docs/FRONTEND_INTEGRATION.md`

✅ **一键启动脚本**
- 自动启动所有 Docker 服务
- 显示访问地址和常用命令
- 文件: `scripts/start_dev.sh`

✅ **集成测试脚本**
- 测试后端健康状态
- 测试 CORS 配置
- 测试 API 端点
- 测试前端可访问性
- 文件: `scripts/test_integration.sh`

## 使用方法

### 快速启动（推荐）

```bash
# 一键启动所有服务
./scripts/start_dev.sh

# 测试集成是否成功
./scripts/test_integration.sh
```

### 手动启动

```bash
# 启动所有服务
docker-compose -f docker-compose.dev.yml up

# 或只启动特定服务
docker-compose -f docker-compose.dev.yml up app frontend
```

### 本地开发

```bash
# 后端
go run main.go

# 前端（新终端）
cd frontend
npm install
npm run dev
```

## 访问地址

- **前端界面**: http://localhost:5173
- **后端 API**: http://localhost:8080
- **健康检查**: http://localhost:8080/healthz
- **Metrics**: http://localhost:8080/metrics

## API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/stats` | GET | 获取站点统计数据 |
| `/api/v1/settlements` | GET | 获取结算数据 |
| `/analytics/window-close` | POST | 触发窗口关闭 |

## 认证

生成开发用 JWT Token:

```bash
source scripts/lib_auth.sh
jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600
```

将生成的 token 粘贴到前端界面的 "JWT Token" 输入框。

## 技术栈

**后端:**
- Go 1.22
- PostgreSQL
- JWT 认证

**前端:**
- React 18
- Vite 5
- 原生 CSS

## 下一步

1. 启动服务: `./scripts/start_dev.sh`
2. 访问前端: http://localhost:5173
3. 生成 JWT token 并粘贴到界面
4. 开始使用！

## 故障排查

如果遇到问题，请查看:
1. `docs/FRONTEND_INTEGRATION.md` - 详细的集成指南
2. Docker 日志: `docker-compose -f docker-compose.dev.yml logs -f`
3. 运行测试脚本: `./scripts/test_integration.sh`

## 文件清单

```
修改的文件:
- main.go (添加 CORS 中间件)
- frontend/src/App.jsx (修正 API 地址)
- frontend/vite.config.js (添加 host 配置)
- frontend/README.md (更新文档)
- docker-compose.dev.yml (添加 frontend 服务)

新增的文件:
- frontend/.env (环境配置)
- docs/FRONTEND_INTEGRATION.md (集成指南)
- scripts/start_dev.sh (启动脚本)
- scripts/test_integration.sh (测试脚本)
```

## 注意事项

1. **端口冲突**: 确保 8080 和 5173 端口未被占用
2. **Docker**: 需要 Docker 和 Docker Compose
3. **数据库**: 后端需要 PostgreSQL 数据库
4. **认证**: 大部分 API 需要 JWT 认证

---

✅ 前后端已成功集成！
