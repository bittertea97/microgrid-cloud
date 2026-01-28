#!/bin/bash
# 前后端一键启动脚本

set -e

echo "🚀 启动微电网云平台（前后端）..."

# 检查 Docker 是否运行
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker 未运行，请先启动 Docker"
    exit 1
fi

# 启动所有服务
echo "📦 启动 Docker Compose 服务..."
docker-compose -f docker-compose.dev.yml up -d

echo ""
echo "✅ 服务启动成功！"
echo ""
echo "📍 访问地址："
echo "   前端界面: http://localhost:5173"
echo "   后端 API: http://localhost:8080"
echo "   健康检查: http://localhost:8080/healthz"
echo ""
echo "📝 查看日志："
echo "   所有服务: docker-compose -f docker-compose.dev.yml logs -f"
echo "   前端日志: docker-compose -f docker-compose.dev.yml logs -f frontend"
echo "   后端日志: docker-compose -f docker-compose.dev.yml logs -f app"
echo ""
echo "🛑 停止服务："
echo "   docker-compose -f docker-compose.dev.yml down"
echo ""
