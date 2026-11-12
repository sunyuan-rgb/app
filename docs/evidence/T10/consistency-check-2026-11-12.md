# 题目 10 阅读活动跨环节一致性检查报告

- 检查日期：2026-11-12
- 检查范围：`D:/gopath/2307a/app`（代码层）+ `reading_activity` 库（数据层，容器 dev-mysql）
- 核查脚本：`scripts/consistency_queries.sql`（Q1~Q13，确定性查询）
- 结论：**通过**（阻塞项 0；1 条低风险备注，不阻塞）

## 一、代码层逐项

### 规则组 1：活动时间与业务日期归属

| 规则 | 结论 | 证据 | 差距 |
|---|---|---|---|
| 业务时区为北京时间 | 符合 | `internal/model/model.go:8`（`Beijing = FixedZone("UTC+8", 8*3600)`），`:11-13`（`BizDate` 按北京时区取日期） | 无 |
| 业务日按"发生时间"而非接收时间归属 | 符合 | `internal/service/event_service.go:83`（`bizDate := model.BizDate(in.OccurAt)`）；拒绝留痕同样按发生时间归日 `:202` | 无 |
| 迟到窗口 ≤ 24 小时 | 符合 | `event_service.go:17`（`LateWindow = 24h`）、`:68-70`（`now.Sub(in.OccurAt) > LateWindow` 拒绝并留痕） | 无 |
| 发生时间不得晚于接收时间 | 符合 | `event_service.go:65-67` | 无 |
| 发生时间 ∈ [活动开始, 结束] | 符合 | `event_service.go:79-81`（闭区间；结束时刻 2026-11-13 00:00+08 当日事件可入账，次日拒绝） | 无 |
| 跨日迟到事件归发生日 | 符合 | 归日取 `OccurAt`（`:83`），窗口判定独立（`:68-70`），两者解耦；测试 `TestEvent_LateWindowBoundary`、`TestEvent_LateEventAttributedToOccurDate` 验证 | 无 |
| 数据库无越界记录 | 符合 | 数据层 Q4/Q9/Q10 均 0 行 | 无 |

### 规则组 2：去重规则

| 规则 | 结论 | 证据 | 差距 |
|---|---|---|---|
| 同用户同日同文章只计一次（数据库唯一键，非内存判断） | 符合 | 唯一键 `migrations/001_init.sql:50`（`uk_user_date_article (user_id, biz_date, article_id)`）；`internal/store/event_store.go:66-79`（`InsertProgress` ON DUPLICATE KEY，`RowsAffected==1` 判定新增）；`event_service.go:131-147` | 无 |
| 同事件编号同内容重复投递 → 幂等返回首次结果 | 符合 | `event_service.go:107-120`（`sameContent :213-215` 比对 user/article/occur_at） | 无 |
| 同事件编号换内容 → 拒绝并留痕 `event_conflict` | 符合 | `event_service.go:121-129`；留痕表 `001_init.sql:34-41`；数据层 Q7 实际留痕 1 行（payload 含冲突请求全文） | 无 |

### 规则组 3：奖励发放

| 规则 | 结论 | 证据 | 差距 |
|---|---|---|---|
| 每日 3 篇只发一次 30 积分 | 符合 | 达标判定 `event_service.go:150-155`（`CountArticles` 恰达门槛时尝试发奖）；幂等唯一键 `001_init.sql:60`（`uk_user_activity_date`）；`event_store.go:91-103`（duplicate → 不再发） | 见备注 A |
| 奖励记录 + 积分流水 + 余额同事务 | 符合 | `event_service.go:86-90`（BeginTx/defer Rollback）、`:156-173`（reward → InsertPointTxn → AddBalance 同事务）、`:180`（单次 Commit） | 无 |
| 客服补发复用统一发奖路径 | 符合 | `internal/service/agent_service.go:160-165`（补入走 `HandleEvent`，`source=model.SourceAgent`，注释明示复用统一校验与发奖规则） | 无 |
| 并发只发一次 | 符合 | `internal/service/event_concurrency_test.go`：20 路并发同日同用户、以及 20 路并发同事件，`-race` 下 `reward` 仅 1 行、流水/余额一致 | 无 |
| 数据层一致性 | 符合 | Q2=0、Q3=0、Q6=0、Q12=0；Q13：`txn_total=30 = balance_total=30`，`user_days=1` 对应 `reward_rows=1` | 无 |

### 规则组 4：申诉补发

| 规则 | 结论 | 证据 | 差距 |
|---|---|---|---|
| 补入仍受活动期与迟到窗口约束 | 符合 | 补入入口即 `HandleEvent`（`agent_service.go:160`），活动期 `:79-81`、窗口 `:68-70`、未来事件 `:65-67` 全部生效；补入不绕过任何校验 | 无 |
| 补入不重复发奖 | 符合 | 同路径去重（`event_service.go:131-147` dup_article 不计数）+ 发奖唯一键（`:156`）；仅补入直接触发 `ResultRewarded` 才自动结案（`agent_service.go:172-192`），其余留待客服裁决 | 无 |
| 申诉业务日必须在活动期间 | 符合 | `internal/service/user_service.go:148-153`（`[BizDate(StartAt), BizDate(EndAt)]` 校验） | 无 |
| 同用户同日仅一条未结申诉（数据库级） | 符合 | 生成列唯一键 `migrations/003_users_appeal.sql:13-16`（`open_flag` + `uk_user_day_open`）；`internal/store/user_store.go:107-120`（duplicate → false）；`user_service.go:155-161`（false → `ErrAppealExists`） | 无 |
| 客服无直接改余额 / 原始事件路径 | 符合 | `agent_service.go` 全文仅调用 `ResolveAppeal`（更新 appeal 表）与 `HandleEvent`；无 `AddBalance`、无 UPDATE `reading_event` 调用点 | 无 |
| 驳回必须写明依据 | 符合 | `agent_service.go:111-115`（空依据拒绝） | 无 |
| 数据层 | 符合 | Q11（重复未结申诉）0 行 | 无 |

## 二、数据层逐项（reading_activity 库，2026-11-12 实测）

| 查询 | 期望 | 实测 | 结论 |
|---|---|---|---|
| Q1 文章去重（进度重复） | 0 行 | 0 行 | 一致 |
| Q2 发奖幂等（重复奖励） | 0 行 | 0 行 | 一致 |
| Q3 余额 = 流水合计 | 0 差异用户 | 0 行 | 一致 |
| Q4 biz_date = occur_at 北京日期 | 0 行 | 0 行 | 一致 |
| Q5 counted=1 须有进度行 | 0 行 | 0 行 | 一致 |
| Q6 达标⇔发奖对应 | 0 行 | 0 行 | 一致 |
| Q7 冲突留痕（列示） | — | 1 行：`sim-conflict-…`，payload 含冲突请求全文 | 留痕正常 |
| Q8 拒绝事件留痕（列示） | — | 未来事件 1 / 超 24h 窗口 2 / 活动期外 2 | 留痕正常 |
| Q9 超 24h 窗口受理 | 0 行 | 0 行 | 一致 |
| Q10 活动期外受理 | 0 行 | 0 行 | 一致 |
| Q11 同日多条未结申诉 | 0 行 | 0 行 | 一致 |
| Q12 孤儿奖励流水 | 0 行 | 0 行 | 一致 |
| Q13 总览 | txn_total=balance_total | progress_rows=4，user_days=1，reward_rows=1，accepted=6，rejected=5，txn_total=30，balance_total=30，conflict_rows=1 | 一致（1 个用户日达标 → 恰 1 条发奖；余额=流水） |

## 三、问题清单

| 编号 | 级别 | 位置 | 描述 | 建议 | 是否阻塞 |
|---|---|---|---|---|---|
| A | 低风险（备注） | `event_service.go:155` | 达标判定使用 `count == DailyThreshold`（恰好等于）。当前唯一入口每事件只新增 1 篇进度，不存在跳档；若未来新增"批量补进度"路径使 count 从 2 直接跳到 4，将漏发奖 | 改为 `count >= DailyThreshold && !rewarded`（发奖幂等键已保证不重复） | 否 |

其余：无违反项、无待人工确认项。
