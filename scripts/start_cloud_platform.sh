#!/bin/bash
# 启动微电网云平台服务

set -e

echo "🚀 启动微电网云平台服务..."

# 检查是否在正确的目录
if [ ! -f "main.go" ]; then
    echo "❌ 错误：请在项目根目录运行此脚本"
    exit 1
fi

# 设置环境变量
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/microgrid_cloud?sslmode=disable"
export HTTP_ADDR=":8081"
export TENANT_ID="tenant-demo"
export STATION_ID="station-demo-001"
export PRICE_PER_KWH="1.0"
export CURRENCY="CNY"
export EXPECTED_HOURS="24"
export TB_BASE_URL="http://localhost:8080"
export TB_TOKEN=""
export AUTH_JWT_SECRET="dev-secret-change-me"
export INGEST_HMAC_SECRET="dev-ingest-secret"
export INGEST_MAX_SKEW_SECONDS="300"
export SHADOWRUN_PUBLIC_BASE_URL="http://localhost:8081"
export OUTBOX_DISPATCH_INTERVAL="200ms"
export OUTBOX_DISPATCH_BATCH="200"

# 创建数据库（如果不存在）
echo "📦 创建数据库..."
docker exec postgres psql -U postgres -c "CREATE DATABASE microgrid_cloud;" 2>/dev/null || echo "数据库已存在"

# 运行迁移
echo "🔄 运行数据库迁移..."
if command -v migrate &> /dev/null; then
    migrate -path migrations -database "$DATABASE_URL" up
else
    echo "⚠️  migrate 工具未安装，跳过迁移"
    echo "   安装方法: https://github.com/golang-migrate/migrate"
fi

# 启动服务
echo "▶️  启动云平台服务..."
echo "   监听地址: $HTTP_ADDR"
echo "   ThingsBoard: $TB_BASE_URL"
echo ""

go run main.go
