# 告警通知（Notification）

本项目告警通知由 `internal/alarms/notify` 提供，Shadowrun 通知由 `internal/shadowrun/notify` 提供，均使用统一 webhook 负载格式（`msgtype=text`）。

## 通知渠道
- SSE 实时流：`GET /api/v1/alarms/stream`（由 `internal/alarms/interfaces/http/stream.go` 提供）。
- Webhook：`ALARM_WEBHOOK_URL` 指向钉钉/企微通用 webhook，使用 `msgtype=text` 的 JSON 负载。
- Shadowrun webhook：`SHADOWRUN_WEBHOOK_URL` 使用相同格式，正文为差异摘要。

## 配置（环境变量）
- `ALARM_WEBHOOK_URL`：Webhook 地址（为空则不启用 webhook 通知）。
- `ALARM_NOTIFY_TEMPLATE`：自定义通知模板（Go `text/template`）。为空使用默认模板。
- `ALARM_ESCALATION_AFTER`：升级/重发延迟，例如 `10m`。
- `ALARM_NOTIFY_COOLDOWN`：冷却时间（同一告警 + 同一事件类型在该时间内只发送一次）。
- `ALARM_NOTIFY_DEDUP_WINDOW`：去重窗口（内容完全一致的通知在窗口内只发送一次）。
- `ALARM_NOTIFY_TIMEOUT`：升级检查时读取告警状态的超时，例如 `5s`。
- `ALARM_REPORT_LOOKBACK_DAYS`：shadowrun 报告回溯天数（>0 时启用报告链接）。
- `ALARM_REPORT_BASE_URL`：报告链接的公共前缀（若为空，建议与 `SHADOWRUN_PUBLIC_BASE_URL` 保持一致）。

示例：
```bash
ALARM_WEBHOOK_URL="https://oapi.dingtalk.com/robot/send?access_token=xxx"
ALARM_ESCALATION_AFTER=10m
ALARM_NOTIFY_COOLDOWN=2m
ALARM_NOTIFY_DEDUP_WINDOW=10m
ALARM_NOTIFY_TIMEOUT=5s
ALARM_REPORT_LOOKBACK_DAYS=7
ALARM_REPORT_BASE_URL="http://localhost:8080"
```

## 模板字段
默认模板位于 `internal/alarms/notify/template.go`，可使用以下字段：
- `Station` / `StationID`
- `Rule` / `RuleID`
- `TriggerValue`
- `Threshold`
- `StartTime`
- `Status` / `StatusCode`
- `Severity`
- `Suggestion`
- `ReportURL`（当存在 shadowrun 报告时）
- `Event` / `EventLabel`

## 升级策略
- 当告警 `severity >= high` 且持续超过 `ALARM_ESCALATION_AFTER` 仍未 cleared，触发一次 `escalated` 通知。
- 冷却时间与去重窗口在 `internal/alarms/notify/notifier.go` 中执行，避免刷屏：
  - 冷却时间：同一告警 + 同一事件类型在冷却窗口内只发送一次。
  - 去重窗口：内容完全一致的通知在窗口内只发送一次。

## Shadowrun 报告链接
当 `ALARM_REPORT_LOOKBACK_DAYS > 0` 且配置了 `ALARM_REPORT_BASE_URL` 时：
- 通知会尝试为同一站点查找最近一次 shadowrun 报告，并拼接下载链接：
  `ALARM_REPORT_BASE_URL/api/v1/shadowrun/reports/{id}/download`
- 若没有报告或查询失败，则 `ReportURL` 为空。

## 测试
- Webhook payload 断言：
  `go test ./internal/alarms/notify -run TestWebhookNotifierPayload`
- 升级策略、冷却/去重测试：
  `go test ./internal/alarms/notify -run TestNotifier`
