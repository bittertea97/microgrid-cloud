# ThingsBoard 集成配置指南

## 目标

将 ThingsBoard 中的设备遥测数据转发到云平台进行业务处理。

## 架构说明

```
TB 设备 → TB 遥测 → [Rule Chain/Integration] → 云平台 /ingest/thingsboard/telemetry
                                                ↓
                                         telemetry_points 表
                                                ↓
                                         窗口关闭事件
                                                ↓
                                         小时统计 → 日汇总 → 结算
                                                ↓
                                         前端显示
```

## 步骤 1: 在云平台注册站点

### 1.1 生成 JWT Token

```bash
cd /home/spdms/microgrid-cloud/microgrid-cloud
source scripts/lib_auth.sh
jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600
```

### 1.2 调用 Provisioning API

```bash
TOKEN="<your-jwt-token>"

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
    "tb_asset_id": "<your-tb-asset-id>",
    "devices": [
      {
        "id": "device-001",
        "tb_entity_id": "<your-tb-device-id>",
        "device_type": "battery",
        "name": "储能设备1"
      }
    ],
    "point_mappings": [
      {
        "device_id": "device-001",
        "point_key": "soc",
        "semantic": "battery_soc",
        "unit": "%",
        "factor": 1.0
      },
      {
        "device_id": "device-001",
        "point_key": "power",
        "semantic": "battery_power",
        "unit": "kW",
        "factor": 1.0
      }
    ]
  }'
```

## 步骤 2: 配置 ThingsBoard Integration

### 方式 A: 使用 HTTP Integration（推荐）

1. 登录 ThingsBoard
2. 进入 **Integrations** → **Add Integration**
3. 选择 **HTTP**
4. 配置：

```
Name: Microgrid Cloud Platform
Type: HTTP
Enabled: ✓

Base URL: http://<your-cloud-platform-host>:8081/ingest/thingsboard/telemetry

Headers:
  Content-Type: application/json
  X-Signature: <计算的 HMAC 签名>

Uplink data converter:
  使用默认或自定义转换器
```

### 方式 B: 使用 Rule Chain

1. 进入 **Rule Chains** → 创建新的 Rule Chain
2. 添加节点：

```
[Message Type Switch] → [Script Transform] → [REST API Call]
```

3. **REST API Call** 节点配置：

```
Endpoint URL: http://<your-cloud-platform-host>:8081/ingest/thingsboard/telemetry
Request method: POST
Headers:
  Content-Type: application/json
  X-Signature: ${metadata.signature}
```

4. **Script Transform** 节点（计算签名）：

```javascript
var crypto = require('crypto');
var secret = 'dev-ingest-secret'; // 与云平台配置一致

var timestamp = Date.now();
var payload = JSON.stringify(msg);
var message = timestamp + '.' + payload;
var signature = crypto.createHmac('sha256', secret).update(message).digest('hex');

metadata.timestamp = timestamp.toString();
metadata.signature = signature;

return {msg: msg, metadata: metadata, msgType: msgType};
```

## 步骤 3: 配置点位映射

云平台需要知道如何解析 TB 的遥测数据。在 `point_mappings` 表中配置：

| point_key | semantic | unit | factor |
|-----------|----------|------|--------|
| soc | battery_soc | % | 1.0 |
| power | battery_power | kW | 1.0 |
| voltage | battery_voltage | V | 1.0 |
| current | battery_current | A | 1.0 |

**语义 (semantic) 说明**：
- `battery_soc`: 电池 SOC
- `battery_power`: 电池功率（正=充电，负=放电）
- `battery_charge_kwh`: 充电电量
- `battery_discharge_kwh`: 放电电量

## 步骤 4: 测试数据流

### 4.1 在 TB 中发送测试遥测

```bash
# 使用 TB REST API 发送遥测
TB_TOKEN="<your-tb-token>"
DEVICE_ID="<your-device-id>"

curl -X POST "http://localhost:18080/api/v1/$DEVICE_ID/telemetry" \
  -H "Content-Type: application/json" \
  -d '{
    "soc": 85.5,
    "power": 50.2,
    "voltage": 380.5,
    "current": 132.1
  }'
```

### 4.2 检查云平台是否收到数据

```bash
# 查看后端日志
docker logs -f microgrid-cloud-dev-app-1

# 应该看到类似的日志：
# telemetry received: station=station-demo-001 device=device-001 points=4
```

### 4.3 触发窗口关闭（生成统计）

```bash
TOKEN="<your-jwt-token>"

curl -X POST http://localhost:8081/analytics/window-close \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "stationId": "station-demo-001",
    "windowStart": "2024-01-28T00:00:00Z"
  }'
```

### 4.4 在前端查看统计数据

1. 打开 http://localhost:5173
2. 粘贴 JWT Token
3. 选择站点 `station-demo-001`
4. 设置时间范围
5. 点击"刷新统计"

## 步骤 5: 配置自动窗口关闭（可选）

如果需要自动触发统计计算，可以配置定时任务：

```bash
# 添加 cron job
crontab -e

# 每小时触发一次窗口关闭
0 * * * * curl -X POST http://localhost:8081/analytics/window-close \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"stationId":"station-demo-001","windowStart":"'$(date -u -d '1 hour ago' +%Y-%m-%dT%H:00:00Z)'"}'
```

## 故障排查

### 问题 1: 云平台收不到数据

检查：
1. TB Integration 是否启用
2. 网络连接是否正常
3. HMAC 签名是否正确
4. 查看云平台日志

### 问题 2: 数据收到但没有统计

检查：
1. 站点是否已 provision
2. 点位映射是否配置
3. 是否触发了窗口关闭
4. 查看 `telemetry_points` 表是否有数据

### 问题 3: 前端显示空数据

检查：
1. JWT Token 是否有效
2. 站点 ID 是否正确
3. 时间范围是否包含数据
4. 查看浏览器控制台错误

## 数据验证

### 检查原始遥测数据

```sql
SELECT * FROM telemetry_points
WHERE station_id = 'station-demo-001'
ORDER BY ts DESC
LIMIT 10;
```

### 检查小时统计

```sql
SELECT * FROM statistics
WHERE station_id = 'station-demo-001'
  AND granularity = 'hour'
ORDER BY period_start DESC
LIMIT 10;
```

### 检查日结算

```sql
SELECT * FROM settlements
WHERE station_id = 'station-demo-001'
ORDER BY day_start DESC
LIMIT 10;
```

## 参考

- 云平台 API 文档: `docs/FRONTEND_INTEGRATION.md`
- 事件驱动架构: `docs/OUTBOX_TROUBLESHOOTING.md`
- 性能优化: `docs/PERF.md`
