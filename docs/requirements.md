# 需求说明：题目 10 资讯 App 阅读活动的发布、奖励、申诉与结算核查

> 来源：教师题面 `assignment.md`（快照 2026-11-11）共同要求第 1~10 节 + 题目 10。
> 本文档为事后整理版（整理日期 2026-11-12），用于把题面要求落到可验证条件；不改变原始实施时间。
> 假设与未明确事项见 `docs/assumptions.md`。

## 1. 业务目标

运营发布一个固定规则的阅读活动：用户每日读完 3 篇不同文章得 30 积分。系统要能解释"每一笔奖励为什么发放或没有发放"，并在活动结束后核对各日达标人数、发奖人数、积分总额与申诉结果。

## 2. 角色与诉求

| 角色 | 入口 | 诉求 | 不做什么 |
|---|---|---|---|
| 运营 ops | `/ops` | 创建/发布活动、查看发放汇总与不一致明细 | 不中途改规则、不撤销已发积分 |
| 用户 user | `/user` | 看活动、看每日进度、看积分流水、提交一条未结申诉 | 不查他人数据、不改余额 |
| 模拟阅读服务 | `readingsvc`（:8081） | 上报有效阅读事件、按用户+日期提供可信记录查询 | 不实现反作弊 |
| 客服 agent | `/agent` | 核查申诉证据、驳回并说明、补入缺失有效记录、留操作者与依据 | 不改原始事件、不直接改余额 |

## 3. 状态与不变量

```
活动：draft(未发布) → published(已发布，规则冻结)
事件：accepted / duplicated / dup_article / rejected / conflict
进账：reward 唯一(user_id, activity_id, biz_date) → 只发一次 30 分
申诉：open(0) → compensed(1) / rejected(2)（单向，不可二次处理）
```

不变量（任何时刻都成立）：

1. `sum(point_txn.points) == user_balance.balance`（同一事务内写 reward + point_txn + balance）。
2. 每个达标用户日最多 1 条 `reward`，因此最多 1 条发奖流水。
3. `daily_progress` 的 `(user_id, biz_date, article_id)` 唯一，`reading_event.event_id` 全局唯一。
4. 申诉结案必然带 `operator_id` 与结论文本。

## 4. 规则拆解与可验证条件

| 规则 | 题面要点 | 实现位置 | 可验证条件 | 验证方式 |
|---|---|---|---|---|
| R1 | 开始前编辑并发布；时间/门槛/积分发布后固定 | `internal/service/ops_service.go` | 首次发布成功；重复发布 409；无改规则接口 | 接口探测 + 单元测试 |
| R2 | 事件须鉴权，含唯一编号/用户/文章/发生时间；发生时间在活动期内且不晚于接收时间 | `event_service.go` + `router.go` requireToken | 无 `X-Source-Token` 401；越界时间 `rejected` 且留痕 | 单测 + 演示脚本 |
| R3 | 同用户同日同文章只计一次；同编号换内容拒绝；最多迟到 24h，按发生日归属 | `store.InsertProgress` 唯一键 + `event_service.go` 校验 | 重复事件 `duplicated`；重复文章 `dup_article`；23h59m 受理、24h01m 拒绝；`biz_date=发生日` | 单测边界 + 时钟模式演示 |
| R4 | 达 3 篇只发一次 30 分；活动结束后仍受理窗口内迟到事件，不计入接收日 | `event_service.go` 事务段 + `reward` 唯一键 | 并发只出 1 条 reward；结束后 10h 送达归发生日 | 20 路并发单测 + 双进程 curl |
| R5 | 用户按日期查进度、发起一条未结申诉；不能查他人 | `user_service.go` + `router.go` requireAccount | 跨用户 403；同用户已有未结申诉再提交被拒 | 接口探测 |
| R6 | 客服结合已接收事件 + 可信查询核查；证据不足驳回并说明；可信则补入并触发同一规则 | `agent_service.go` | 驳回必须写依据（空串报错）；补入走 `HandleEvent`，受活动期/窗口/去重约束 | 单测 + S3/S4 演示 |
| R7 | 自动与申诉并发只发一次；已获奖告知流水不重复补发；留处理结果/依据/操作者 | `store.InsertReward` 唯一键 + `ResolveAppeal` | 双进程并发仅 1 条 reward；已发奖再补发 409 | S5 演示 + 单测 |
| R8 | 活动结束且过 24h 窗口后，运营查各日达标/发奖人数、积分总额、申诉结果，不一致可追溯 | `ops_service.go` Summary + `Inconsistency` | `window_closed=true`；`inconsistencies=[]`（正常）或给出明细（脏数据） | S5 尾段 + 单测 |

## 5. 五组必演场景的验收条件

| 场景 | 起点 | 关键断言 |
|---|---|---|
| S1 正常活动 | 运营发布 A2（两业务日） | 第 3 篇返回 `rewarded`；余额=流水=30；汇总 `reached_users == rewarded_users` |
| S2 重复与延迟 | 已有 2 篇 | 第 3 篇重复文章 `dup_article`；重放 `duplicated`；超窗 `rejected`；+50h 迟到归 11-13；结束后窗口内受理 |
| S3 申诉成立 | 用户仅 2 篇，提交申诉 | 客服补入 → `rewarded`、`resolved=true`、`reward_id` 有值；原事件重送不重复发奖 |
| S4 申诉不成立 | 用户仅 2 篇或记录超窗 | 驳回写依据；积分不变；二次处理 409 |
| S5 并发与收尾 | 用户 2 篇 + 已提交申诉 | 自动第 3 篇与客服补入并发 → 仅 1 条 reward；越权 403；+74h `window_closed=true` |

## 6. 非功能与质量门槛

- 单元覆盖率：整体（业务代码）≥85%，各核心包 ≥85%（`scripts/covergate` 判定，排除 `cmd/*`、`internal/testutil`、`scripts/covergate`）。
- 格式 `gofmt -l` 无输出、`go vet ./...` 无问题、含共享并发逻辑执行 `-race`。
- 无明文口令入库：DSN 由 `APP_DSN` / `TEST_DSN` 注入。
- 可运行：`docker compose up -d` 起 mysql + app + readingsvc，迁移自动执行，`/healthz` 200。

## 7. 明确不做

注册登录全套、真实短信/推送、积分提现与广告结算、反作弊判定、复杂活动引擎、活动中途改规则。
