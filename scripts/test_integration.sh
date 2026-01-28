#!/bin/bash
# 测试前后端连接

set -e

BACKEND_URL="http://localhost:8080"
FRONTEND_URL="http://localhost:5173"

echo "🧪 测试前后端集成..."
echo ""

# 测试后端健康检查
echo "1️⃣ 测试后端健康检查..."
if curl -s -f "${BACKEND_URL}/healthz" > /dev/null; then
    echo "   ✅ 后端运行正常"
else
    echo "   ❌ 后端未运行或无法访问"
    echo "   请先启动后端: docker-compose -f docker-compose.dev.yml up app"
    exit 1
fi

# 测试 CORS
echo ""
echo "2️⃣ 测试 CORS 配置..."
CORS_RESPONSE=$(curl -s -I -X OPTIONS \
    -H "Origin: ${FRONTEND_URL}" \
    -H "Access-Control-Request-Method: GET" \
    -H "Access-Control-Request-Headers: Content-Type" \
    "${BACKEND_URL}/api/v1/stats")

if echo "$CORS_RESPONSE" | grep -q "Access-Control-Allow-Origin"; then
    echo "   ✅ CORS 配置正确"
else
    echo "   ⚠️  CORS 可能未正确配置"
fi

# 测试 API 端点（无认证）
echo ""
echo "3️⃣ 测试 API 端点..."
API_RESPONSE=$(curl -s -w "\n%{http_code}" "${BACKEND_URL}/api/v1/stats?station_id=station-demo-001&from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z&granularity=hour")
HTTP_CODE=$(echo "$API_RESPONSE" | tail -n1)

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "401" ]; then
    echo "   ✅ API 端点可访问 (HTTP $HTTP_CODE)"
else
    echo "   ❌ API 端点异常 (HTTP $HTTP_CODE)"
fi

# 测试前端
echo ""
echo "4️⃣ 测试前端..."
if curl -s -f "${FRONTEND_URL}" > /dev/null 2>&1; then
    echo "   ✅ 前端运行正常"
else
    echo "   ⚠️  前端未运行或无法访问"
    echo "   启动前端: docker-compose -f docker-compose.dev.yml up frontend"
fi

echo ""
echo "✅ 集成测试完成！"
echo ""
echo "📍 访问地址："
echo "   前端: ${FRONTEND_URL}"
echo "   后端: ${BACKEND_URL}"
echo ""
