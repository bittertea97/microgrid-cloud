# 前后端集成指南

本文档说明如何将前端 React 应用与 Go 后端服务连接。

## 架构概览

- **后端**: Go 服务运行在 `http://localhost:8080`
- **前端**: React + Vite 应用运行在 `http://localhost:5173`
- **通信**: 前端通过 HTTP API 调用后端服务

## 已完成的集成工作

### 1. 后端 CORS 支持

在 `main.go` 中添加了 CORS 中间件，允许前端跨域访问：

```go
func corsMiddleware(next http.Handler) http.Handler {
    // 允许所有来源的跨域请求
    // 支持 GET, POST, PUT, DELETE, OPTIONS 方法
    // 允许 Content-Type 和 Authorization 头
}
```

### 2. 前端配置

- 默认 API 地址设置为 `http://localhost:8080`
- 支持通过 `.env` 文件配置 `VITE_API_BASE_URL`
- Vite 配置允许从任何主机访问（Docker 支持）

### 3. Docker Compose 集成

在 `docker-compose.dev.yml` 中添加了 `frontend` 服务：

```yaml
frontend:
  image: node:20-alpine
  working_dir: /app
  volumes:
    - ./frontend:/app
  command: sh -c "npm install && npm run dev"
  environment:
    VITE_API_BASE_URL: http://localhost:8080
  ports:
    - "5173:5173"
  depends_on:
    - app
```

## 启动方式

### 方式 1: 使用 Docker Compose（推荐）

启动所有服务（包括前后端）：

```bash
docker-compose -f docker-compose.dev.yml up
```

只启动前端（假设后端已运行）：

```bash
docker-compose -f docker-compose.dev.yml up frontend
```

### 方式 2: 本地开发

**启动后端：**

```bash
# 设置环境变量
export DATABASE_URL="postgres://microgrid:microgrid@localhost:5432/microgrid?sslmode=disable"
export HTTP_ADDR=":8080"
export TB_BASE_URL="http://localhost:18080"
export AUTH_JWT_SECRET="dev-secret-change-me"

# 运行后端
go run main.go
```

**启动前端：**

```bash
cd frontend
npm install
npm run dev
```

## API 端点

前端调用以下后端 API：

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/stats` | GET | 获取站点统计数据 |
| `/api/v1/settlements` | GET | 获取结算数据 |
| `/analytics/window-close` | POST | 触发窗口关闭 |

### 请求参数示例

**获取统计数据：**
```
GET /api/v1/stats?station_id=station-demo-001&from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z&granularity=hour
```

**获取结算数据：**
```
GET /api/v1/settlements?station_id=station-demo-001&from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z
```

**触发窗口关闭：**
```
POST /analytics/window-close
Content-Type: application/json

{
  "stationId": "station-demo-001",
  "windowStart": "2024-01-01T00:00:00Z"
}
```

## 认证

后端使用 JWT 认证。前端需要在请求头中携带 token：

```
Authorization: Bearer <jwt-token>
```

### 生成开发用 JWT Token

```bash
source scripts/lib_auth.sh
jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600
```

将生成的 token 粘贴到前端界面的 "JWT Token" 输入框中。

## 访问地址

- **前端界面**: http://localhost:5173
- **后端 API**: http://localhost:8080
- **健康检查**: http://localhost:8080/healthz
- **Metrics**: http://localhost:8080/metrics

## 故障排查

### 前端无法连接后端

1. 检查后端是否运行：`curl http://localhost:8080/healthz`
2. 检查前端配置：查看 `frontend/.env` 文件
3. 检查浏览器控制台的 CORS 错误

### CORS 错误

确保后端的 CORS 中间件已正确配置并应用到 HTTP 服务器。

### 认证失败

1. 确保 JWT secret 一致（后端 `AUTH_JWT_SECRET` 和生成 token 时使用的 secret）
2. 检查 token 是否过期
3. 确认 token 格式正确（Bearer + 空格 + token）

## 开发建议

1. **热重载**: 前端支持热重载，修改代码后自动刷新
2. **API 调试**: 使用浏览器开发者工具的 Network 标签查看 API 请求
3. **日志**: 后端日志会显示所有 HTTP 请求
4. **数据持久化**: 前端使用 localStorage 保存配置（API URL、Station ID、Token 等）

## 生产部署

生产环境建议：

1. 使用 Nginx 作为反向代理
2. 前端构建静态文件：`npm run build`
3. 配置 HTTPS
4. 使用环境变量管理配置
5. 限制 CORS 允许的来源

示例 Nginx 配置：

```nginx
server {
    listen 80;
    server_name example.com;

    # 前端静态文件
    location / {
        root /var/www/frontend/dist;
        try_files $uri $uri/ /index.html;
    }

    # 后端 API 代理
    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /analytics/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```
