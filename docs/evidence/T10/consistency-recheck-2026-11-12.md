# 跨环节一致性复核（reading-activity-consistency-check，2026-11-12）

> 触发方式：学生指定四条规则与输出格式（提示词原文见 `../sessions/04-审计对话提示词-学生提供.md`）。
> 代码层为本轮实时复核（文件:行号均为当前 main=67c6569 工作区实际读取）；数据层引用 2026-11-12 上午 `consistency_queries.sql` 的真实运行归档（`consistency-check-2026-11-12.md`），本轮未重跑数据库（容器检查被跳过）。

**检查结论：通过（无阻塞项）**

## 规则 1｜活动时间与业务日期归属 —— 符合

| 约束点 | 结论 | 证据 |
|---|---|---|
| 业务日按北京时间换算 | 符合 | `internal/model/model.go:6-13`（`Beijing = FixedZone("UTC+8")`，`BizDate = t.In(Beijing)`）；单测 `internal/model/model_test.go:8-35`（跨日换算 + 带 +08:00 输入不受本地时区影响） |
| 按发生日期而非接收日期归属 | 符合 | `internal/service/event_service.go:83`（`bizDate := model.BizDate(in.OccurAt)`，入事件行 :99）；拒绝路径同样用 OccurAt（:204）——跨日迟到（23:50 发生、次日 0:10 接收）归发生日 |
| 迟到 24h 窗口 | 符合 | `event_service.go:16-17`（`LateWindow = 24*time.Hour`，注释标注题面规则 3）；:68-70（`now.Sub(OccurAt) > LateWindow` → 拒绝并留痕） |
| 活动期校验按发生时间 | 符合 | `event_service.go:79-81`（`OccurAt` 对比 `StartAt/EndAt`，不在活动期 → rejected）；活动结束+24h 后双条件均拒绝（:68-81） |

差距：无。

## 规则 2｜去重规则 —— 符合

| 约束点 | 结论 | 证据 |
|---|---|---|
| 同用户同日同文章只计一次（数据库级） | 符合 | 唯一键 `migrations/001_init.sql:50`（`uk_user_date_article`）；应用层走库不靠内存：`event_service.go:131-147`（`InsertProgress` ON DUPLICATE → `dup_article`）；`internal/store/event_store.go:66-79`（RowsAffected==1 判新增） |
| 同事件编号同内容重复投递 → 幂等返回 | 符合 | `event_service.go:107-120`（查已有事件 + `sameContent` :215-217 比对 user/article/occur_at → `duplicated`） |
| 同编号换内容 → 拒绝并留痕 | 符合 | `event_service.go:121-129`（`InsertEventConflict` 写 payload 全文 → `conflict`）；`event_id` 唯一键 `001_init.sql:30`；冲突表 `001_init.sql:34-41` |
| 数据层佐证（归档运行） | 符合 | Q1 文章去重 0 行；Q7 `event_conflict` 实际留痕 1 行（`docs/evidence/T10/consistency-check-2026-11-12.md`） |

差距：无。

## 规则 3｜奖励发放 —— 符合

| 约束点 | 结论 | 证据 |
|---|---|---|
| 每日 3 篇只发一次 30 积分 | 符合 | 达标判定 `event_service.go:157`（`count >= activity.DailyThreshold`，`>=` 为 D-03 修复项，见 `docs/defects.md:40-42`）；幂等唯一键 `001_init.sql:60`（`uk_user_activity_date`）；`store/event_store.go:91-103`（duplicate → 不再发） |
| 奖励记录 + 流水 + 余额同一事务 | 符合 | `event_service.go:86-90`（BeginTx/defer Rollback）；:163-175（reward 行 → `InsertPointTxn` → `AddBalance` 同一 tx）；:182 单次 Commit——中途失败三者同回滚 |
| 并发只发一次 | 符合 | `internal/service/event_concurrency_test.go:16+`：20 路并发同日第三篇 / 20 路并发同事件，`-race` 下 reward 仅 1 行、流水=余额 |
| 数据层佐证（归档运行） | 符合 | 达标人数 = 发奖人数一一对应；余额 = 流水合计（`consistency-check-2026-11-12.md` 数据层节） |

差距：无。

## 规则 4｜申诉补发 —— 符合

| 约束点 | 结论 | 证据 |
|---|---|---|
| 客服补入复用统一事件路径 | 符合 | `internal/service/agent_service.go:142-165`（`CompensateAppeal` → `s.events.HandleEvent(..., model.SourceAgent)`，注释明示复用统一校验与发奖规则） |
| 补入仍受活动期与迟到窗口约束 | 符合 | 补入走 `HandleEvent`，规则 1 的全部校验（:65-81 窗口/活动期）对其同样生效；测试 `agent_service_test.go`（`CompensateValidation` 等） |
| 补入不重复发奖 | 符合 | 发奖幂等在 `HandleEvent` 内部（规则 3 唯一键兜底）；仅 `ResultRewarded` 才结案（`agent_service.go:171-192`）；补入当日已计文章 → `dup_article` 不结案（`agent_service_test.go:200-206`） |
| 同用户同日仅一条未结申诉（数据库级） | 符合 | 生成列唯一键 `migrations/003_users_appeal.sql:13-16`（`open_flag` + `uk_user_day_open`）；`internal/store/user_store.go:107-120`（duplicate → false）；`user_service.go:129-161`（含活动期校验与 `ErrAppealExists`） |
| 客服无直接改余额/改原始事件路径 | 符合 | `agent_service.go` 全文仅 `ResolveAppeal`（只改 appeal 表）与 `HandleEvent`；无 `AddBalance`、无 UPDATE `reading_event` 调用点 |
| 驳回必须写明依据 | 符合 | `agent_service.go:111-115`（空依据拒绝） |
| 数据层佐证（归档运行） | 符合 | Q11 重复未结申诉 0 行 |

差距：无。

## 问题清单

无阻塞项。备注两点（非缺陷）：
1. 数据层为本轮引用归档运行结果，未实时重跑；如需现场复核可执行 `scripts/consistency_queries.sql`（Q1~Q11）。
2. `>=` 达标判定（D-03 修复）在进度超过门槛（批量补入）时会触发补发，行为由 reward 唯一键兜底为"只发一次"，符合题意且已有专项测试。
