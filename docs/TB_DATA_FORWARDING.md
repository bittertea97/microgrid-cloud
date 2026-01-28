# TB 数据转发到云平台 - 完整配置指南

## 📋 系统状态

### 已完成
- ✅ 数据库创建完成 (`microgrid_cloud`)
- ✅ 数据库迁移完成 (所有表已创建)
- ✅ 前端服务运行中 (http://localhost:5173)
- ⏳ 云平台服务启动中 (http://localhost:8081)

### 服务信息
- **ThingsBoard**: http://localhost:8080
- **云平台后端**: http://localhost:8081
- **前端界面**: http://localhost:5173
- **数据库**: PostgreSQL (localhost:5432/microgrid_cloud)

## 🔧 配置步骤

### 步骤 1: 等待云平台服务启动

检查服务状态：
```bash
docker logs -f microgrid-cloud-platform
```

等待看到类似的日志：
```
http listening on :8081
```

测试服务：
```bash
curl http://localhost:8081/healthz
# 应该返回: ok
```

### 步骤 2: 在 ThingsBoard 中配置 Integration

#### 2.1 登录 ThingsBoard
- URL: http://localhost:8080
- 用户名: `tenant@thingsboard.org`
- 密码: `tenant`

#### 2.2 创建 HTTP Integration

1. 进入 **Integrations** → **Add Integration** → **HTTP**

2. 基本配置：
   ```
   Name: Microgrid Cloud Platform
   Type: HTTP
   Enabled: ✓
   ```

3. HTTP 配置：
   ```
   Base URL: http://host.docker.internal:8081/ingest/thingsboard/telemetry
   HTTP Method: POST

   Headers:
     Content-Type: application/json
   ```

4. Uplink Data Converter (Custom):
   ```javascript
   var crypto = require('crypto');
   var secret = 'dev-ingest-secret';

   var timestamp = Date.now();
   var payload = JSON.stringify(msg);
   var message = timestamp + '.' + payload;
   var signature = crypto.createHmac('sha256', secret).update(message).digest('hex');

   metadata.timestamp = timestamp.toString();
   metadata.signature = signature;

   return {msg: msg, metadata: metadata, msgType: msgType};
   ```

5. 点击 **Add** 保存

### 步骤 3: 在云平台中 Provision 站点

#### 3.1 生成 JWT Token
```bash
cd /home/spdms/microgrid-cloud/microgrid-cloud
source scripts/lib_auth.sh
TOKEN=$(jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600)
echo $TOKEN
```

#### 3.2 创建站点
```bash
curl -X POST http://localhost:8081/api/v1/provisioning/stations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "station": {
      "id": "station-demo-001",
      "tenant_id": "tenant-demo",
      "name": "演示站点",
      "timezone": "Asia/Shanghai",
      "station_type": "microgrid",
      "region": "华东"
    },
    "tb_asset_id": "",
    "devices": [
      {
        "id": "device-battery-001",
        "tb_entity_id": "",
        "device_type": "battery",
        "name": "储能设备1"
      }
    ],
    "point_mappings": [
      {
        "device_id": "device-battery-001",
        "point_key": "soc",
        "semantic": "battery_soc",
        "unit": "%",
        "factor": 1.0
      },
      {
        "device_id": "device-battery-001",
        "point_key": "power",
        "semantic": "battery_power",
        "unit": "kW",
        "factor": 1.0
      },
      {
        "device_id": "device-battery-001",
        "point_key": "charge_kwh",
        "semantic": "battery_charge_kwh",
        "unit": "kWh",
        "factor": 1.0
      },
      {
        "device_id": "device-battery-001",
        "point_key": "discharge_kwh",
        "semantic": "battery_discharge_kwh",
        "unit": "kWh",
        "factor": 1.0
      }
    ]
  }'
```

### 步骤 4: 在 TB 中创建设备并发送测试数据

#### 4.1 创建设备
1. 在 TB 中进入 **Devices** → **Add Device**
2. 设备名称: `Battery Device 001`
3. 设备类型: `Battery`
4. 保存并获取 Access Token

#### 4.2 发送测试遥测数据
```bash
# 替换为你的设备 Access Token
DEVICE_TOKEN="your-device-access-token"

curl -X POST http://localhost:8080/api/v1/$DEVICE_TOKEN/telemetry \
  -H "Content-Type: application/json" \
  -d '{
    "soc": 85.5,
    "power": 50.2,
    "charge_kwh": 100.5,
    "discharge_kwh": 80.3
  }'
```

### 步骤 5: 验证数据流

#### 5.1 检查云平台日志
```bash
docker logs -f microgrid-cloud-platform | grep -i telemetry
```

应该看到类似的日志：
```
telemetry received: station=station-demo-001 device=device-battery-001 points=4
```

#### 5.2 检查数据库
```bash
docker exec postgres psql -U postgres -d microgrid_cloud -c \
  "SELECT * FROM telemetry_points ORDER BY ts DESC LIMIT 5;"
```

#### 5.3 触发窗口关闭（生成统计）
```bash
TOKEN="your-jwt-token"

curl -X POST http://localhost:8081/analytics/window-close \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "stationId": "station-demo-001",
    "windowStart": "'$(date -u -d '1 hour ago' +%Y-%m-%dT%H:00:00Z)'"
  }'
```

#### 5.4 在前端查看数据
1. 打开 http://localhost:5173
2. 粘贴 JWT Token
3. 选择站点 `station-demo-001`
4. 设置时间范围
5. 点击"刷新统计"

## 🔍 故障排查

### 问题 1: 云平台收不到数据

**检查清单：**
- [ ] TB Integration 是否启用
- [ ] Base URL 是否正确 (注意 Docker 网络)
- [ ] HMAC 签名是否正确
- [ ] 云平台服务是否运行

**调试命令：**
```bash
# 检查云平台服务
curl http://localhost:8081/healthz

# 查看云平台日志
docker logs -f microgrid-cloud-platform

# 手动测试 ingest 端点
./scripts/test_ingest.sh
```

### 问题 2: 数据收到但没有统计

**检查清单：**
- [ ] 站点是否已 provision
- [ ] 点位映射是否配置
- [ ] 是否触发了窗口关闭

**调试命令：**
```bash
# 检查原始遥测数据
docker exec postgres psql -U postgres -d microgrid_cloud -c \
  "SELECT COUNT(*) FROM telemetry_points WHERE station_id='station-demo-001';"

# 检查统计数据
docker exec postgres psql -U postgres -d microgrid_cloud -c \
  "SELECT * FROM statistics WHERE station_id='station-demo-001' ORDER BY period_start DESC LIMIT 5;"
```

### 问题 3: 前端显示空数据

**检查清单：**
- [ ] JWT Token 是否有效
- [ ] 站点 ID 是否正确
- [ ] 时间范围是否包含数据
- [ ] 浏览器控制台是否有错误

## 📊 数据流验证

完整的数据流：
```
TB 设备发送遥测
  ↓
TB 接收并触发 Integration
  ↓
TB Integration 计算 HMAC 签名
  ↓
HTTP POST 到云平台 /ingest/thingsboard/telemetry
  ↓
云平台验证签名
  ↓
存储到 telemetry_points 表
  ↓
发布 TelemetryReceived 事件
  ↓
触发窗口关闭 (手动或定时)
  ↓
计算小时统计 → 发布 StatisticCalculated 事件
  ↓
计算日汇总 → 发布 StatisticCalculated(DAY) 事件
  ↓
计算结算 → 发布 SettlementCalculated 事件
  ↓
前端查询 /api/v1/stats 和 /api/v1/settlements
  ↓
显示在界面上
```

## 🚀 快速测试脚本

创建测试脚本：
```bash
cat > /tmp/test_full_flow.sh << 'EOF'
#!/bin/bash
set -e

echo "🧪 测试完整数据流..."

# 1. 发送测试数据到 TB
echo "1️⃣ 发送测试数据到 TB..."
DEVICE_TOKEN="your-device-token"
curl -X POST http://localhost:8080/api/v1/$DEVICE_TOKEN/telemetry \
  -H "Content-Type: application/json" \
  -d '{"soc": 85.5, "power": 50.2, "charge_kwh": 100.5, "discharge_kwh": 80.3}'

sleep 2

# 2. 检查云平台是否收到
echo "2️⃣ 检查云平台数据..."
docker exec postgres psql -U postgres -d microgrid_cloud -c \
  "SELECT COUNT(*) FROM telemetry_points WHERE station_id='station-demo-001' AND ts > NOW() - INTERVAL '1 minute';"

# 3. 触发窗口关闭
echo "3️⃣ 触发窗口关闭..."
TOKEN="your-jwt-token"
curl -X POST http://localhost:8081/analytics/window-close \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"stationId": "station-demo-001", "windowStart": "'$(date -u -d '1 hour ago' +%Y-%m-%dT%H:00:00Z)'"}'

sleep 2

# 4. 检查统计数据
echo "4️⃣ 检查统计数据..."
docker exec postgres psql -U postgres -d microgrid_cloud -c \
  "SELECT * FROM statistics WHERE station_id='station-demo-001' ORDER BY period_start DESC LIMIT 3;"

echo "✅ 测试完成！"
EOF

chmod +x /tmp/test_full_flow.sh
```

## 📚 相关文档

- 完整集成指南: `docs/TB_INTEGRATION_GUIDE.md`
- 前后端集成: `docs/FRONTEND_INTEGRATION.md`
- 事件驱动架构: `docs/OUTBOX_TROUBLESHOOTING.md`
