```md
# AGENT.md  
## Analytics Service 多 Agent 并行开发指南

---

## 0. 本文件的目的（所有 Agent 必读）

本文件用于指导 **Analytics Service** 的多 Agent 并行开发。  
所有 Agent **必须严格遵守 DDD 分层与 Context 边界**，否则将导致统计不可回补、逻辑耦合、系统不可演进。

> ⚠️ 本项目中：
> - 统计不是一次性计算
> - 统计是业务事实
> - 统计结果必须可回补、可审计、可重算

---

## 1. 项目 Context 共识（不可争议）

### Analytics Context 的职责
- 遥测 → 小时统计（唯一事实源头）
- 小时 → 日 → 月 → 年 Rollup
- 统计结果完成后冻结
- 对外发布统计完成事件

### Analytics Context 禁止做的事情
- ❌ 接入设备  
- ❌ 编写 ThingsBoard 规则链  
- ❌ 直接消费 MQTT  
- ❌ 直接处理电价 / 收益规则  
- ❌ 与前端展示耦合  

---

## 2. 架构分层（强制）

```

┌──────────────────────────┐
│ Interface Layer          │  HTTP / MQ / GRPC
├──────────────────────────┤
│ Application Layer        │  用例 / 事务 / 协调
├──────────────────────────┤
│ Domain Layer             │  聚合 / 规则 / 不变量
├──────────────────────────┤
│ Infrastructure Layer     │  DB / MQ / Cache
└──────────────────────────┘

```

### 分层硬性规则
- Domain 层禁止依赖任何 Infra
- Application 层禁止写计算公式
- Infra 层禁止写业务判断

---

## 3. 核心领域模型共识（所有 Agent 必须理解）

### StatisticAggregate（聚合根）

- 唯一键：
```

subjectId + timeType + timeKey

```

- TimeType：
- HOUR / DAY / MONTH / YEAR

- 生命周期：
```

INIT → COMPLETED（冻结）

```

- 关键特性：
- 幂等
- 不可变
- 可回补（覆盖式写入）

---

## 4. 可回补设计铁律（违反即拒绝合并）

- 小时统计是唯一事实源头
- 日 / 月 / 年只能 Rollup
- 禁止累加更新统计结果
- 禁止存储中间态
- 统计写入必须 Upsert
- 回补 = 正常路径的一次重算

---

## 5. Agent 角色与任务说明

---

## 🧠 Agent-A：领域模型（Domain）

### 你的身份
DDD 领域建模负责人

### 你的职责
- 实现 StatisticAggregate
- 实现 StatisticFact、TimeType、TimeKey
- 维护领域不变量
- 保证统计不可变、幂等、可回补

### 你必须遵守
- ❌ 不引入 DB / HTTP / MQ
- ❌ 不依赖其他 Context
- ✅ 只写 Domain 代码
- ✅ 可单元测试

### 输出物
- `internal/analytics/domain/statistic/*`
- 可编译 Go 代码

---

## 🧩 Agent-B：小时统计用例（Application）

### 你的身份
小时统计用例负责人

### 你的职责
- 实现 `HourlyStatisticAppService`
- 消费 `TelemetryWindowClosed` 事件
- 协调：
- 遥测查询
- Domain 计算
- Repository 保存
- 发布 `StatisticCalculated(HOUR)`

### 你必须遵守
- ❌ 不写任何统计公式
- ❌ 不直接操作数据库
- ✅ 只做用例编排
- ✅ 幂等处理

### 输出物
- `HourlyStatisticAppService`
- 相关接口定义

---

## 🧮 Agent-C：日统计 Rollup

### 你的身份
统计汇总（Rollup）负责人

### 你的职责
- 实现 `DailyRollupAppService`
- 监听 `StatisticCalculated(HOUR)`
- 查询当日 Hour 统计
- 校验完整性
- 生成 Day `StatisticAggregate`
- 发布 `StatisticCalculated(DAY)`

### 你必须遵守
- ❌ 不读取遥测
- ❌ 不跳过 Hour 统计
- ✅ 只 Rollup
- ✅ 支持回补

### 输出物
- `DailyRollupAppService`
- `DailyRollupService`（领域服务）

---

## 🗄 Agent-D：Repository（Postgres）

### 你的身份
数据持久化负责人

### 你的职责
- 实现 `StatisticRepository(Postgres)`
- 设计统计表结构
- 支持 `Exists / Save / FindAll`
- 支持 Upsert（回补）

### 你必须遵守
- ❌ 不写业务规则
- ❌ 不理解统计语义
- ✅ 只关心一致性与性能

### 输出物
- SQL 表结构
- Go Repository 实现

---

## 📡 Agent-E：事件与契约

### 你的身份
跨 Context 通信负责人

### 你的职责
- 定义 `TelemetryWindowClosed`
- 定义 `StatisticCalculated`
- 明确事件时间语义
- 支持事件重放（回补）

### 你必须遵守
- ❌ 不绑定 MQ 实现
- ❌ 不耦合其他 Context
- ✅ 事件即契约

### 输出物
- 事件定义文档
- JSON 示例
- Schema（可选）

---

## 6. Agent 协作规则（非常重要）

- Agent 之间只通过：
- 接口
- 事件
进行协作
- 禁止跨 Agent 直接修改代码
- 禁止“为了快”破坏边界

---

## 7. PR 合并前强制自检清单

每个 PR 必须回答：

1. 是否破坏统计可回补？
2. 是否违反 DDD 分层？
3. 是否引入隐式状态？
4. 是否让 Hour 以外成为事实源？

任何一项为 ❌ → PR 不合并

---

## 8. 终极目标（一句话）

> **Analytics Service 是一个可以被反复验证、回放、审计的统计事实系统，而不是一次性计算脚本。**

---

## 9. 给所有 Agent 的最后一句话

> **如果你觉得“这样写更简单”，  
> 请先想一想：  
> 👉 一年后还能不能回补？**
```
