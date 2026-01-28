#!/bin/bash
# TB 数据转发配置脚本

set -e

echo "🔧 配置 ThingsBoard 数据转发到云平台"
echo ""

# 配置参数
CLOUD_PLATFORM_URL="http://host.docker.internal:8081"
INGEST_SECRET="dev-ingest-secret"
TB_URL="http://localhost:8080"
TB_USERNAME="tenant@thingsboard.org"
TB_PASSWORD="tenant"

echo "📋 配置信息："
echo "   云平台 Ingest 端点: ${CLOUD_PLATFORM_URL}/ingest/thingsboard/telemetry"
echo "   ThingsBoard URL: $TB_URL"
echo ""

# 生成 TB 转发规则链配置
cat > /tmp/tb_integration_config.json << 'EOF'
{
  "name": "Microgrid Cloud Integration",
  "type": "HTTP",
  "enabled": true,
  "configuration": {
    "baseUrl": "http://host.docker.internal:8081/ingest/thingsboard/telemetry",
    "httpMethod": "POST",
    "headers": {
      "Content-Type": "application/json"
    },
    "enableSecurity": false
  },
  "uplinkDataConverter": {
    "type": "CUSTOM",
    "configuration": {
      "decoder": "var crypto = require('crypto');\nvar secret = 'dev-ingest-secret';\n\nvar timestamp = Date.now();\nvar payload = JSON.stringify(msg);\nvar message = timestamp + '.' + payload;\nvar signature = crypto.createHmac('sha256', secret).update(message).digest('hex');\n\nmetadata.timestamp = timestamp.toString();\nmetadata.signature = signature;\n\nreturn {msg: msg, metadata: metadata, msgType: msgType};"
    }
  }
}
EOF

echo "✅ 配置文件已生成: /tmp/tb_integration_config.json"
echo ""
echo "📝 手动配置步骤："
echo ""
echo "1. 登录 ThingsBoard"
echo "   URL: $TB_URL"
echo "   用户名: $TB_USERNAME"
echo "   密码: $TB_PASSWORD"
echo ""
echo "2. 创建 HTTP Integration"
echo "   - 进入 Integrations → Add Integration"
echo "   - 选择 HTTP"
echo "   - 配置如下："
echo ""
echo "   Name: Microgrid Cloud Platform"
echo "   Type: HTTP"
echo "   Enabled: ✓"
echo ""
echo "   Base URL: ${CLOUD_PLATFORM_URL}/ingest/thingsboard/telemetry"
echo "   HTTP Method: POST"
echo "   Headers:"
echo "     Content-Type: application/json"
echo ""
echo "3. 配置 Uplink Data Converter"
echo "   - 选择 Custom"
echo "   - 粘贴以下代码："
echo ""
cat << 'CONVERTER'
var crypto = require('crypto');
var secret = 'dev-ingest-secret';

var timestamp = Date.now();
var payload = JSON.stringify(msg);
var message = timestamp + '.' + payload;
var signature = crypto.createHmac('sha256', secret).update(message).digest('hex');

metadata.timestamp = timestamp.toString();
metadata.signature = signature;

return {msg: msg, metadata: metadata, msgType: msgType};
CONVERTER
echo ""
echo "4. 保存并启用 Integration"
echo ""
echo "5. 测试数据转发"
echo "   - 在 TB 中创建设备并发送遥测数据"
echo "   - 检查云平台日志确认收到数据"
echo ""
