# 🚀 快速参考

## 一键启动

```bash
./scripts/start_dev.sh
```

## 访问地址

- 前端: http://localhost:5173
- 后端: http://localhost:8080

## 生成 Token

```bash
source scripts/lib_auth.sh
jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600
```

## 常用命令

```bash
# 启动所有服务
docker-compose -f docker-compose.dev.yml up

# 查看日志
docker-compose -f docker-compose.dev.yml logs -f

# 停止服务
docker-compose -f docker-compose.dev.yml down

# 测试集成
./scripts/test_integration.sh
```

## 详细文档

- 集成指南: `docs/FRONTEND_INTEGRATION.md`
- 完整总结: `INTEGRATION_SUMMARY.md`
