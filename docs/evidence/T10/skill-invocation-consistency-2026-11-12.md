# Skill 实际调用记录：reading-activity-consistency-check

- Skill 路径：`.codebuddy/skills/reading-activity-consistency-check/`
- 触发场景：事件接入、发奖、申诉补入改动后，核查跨环节一致性（活动时间、业务日期、去重、发奖幂等、申诉补发）
- 执行时间：2026-11-12（北京时间）
- 输入：仓库 `internal/**`、迁移 `migrations/*.sql`、目标库 `reading_activity_demo2`
- 执行方式：`docker exec dev-mysql sh -c "mysql --default-character-set=utf8mb4 reading_activity_demo2 < /tmp/cq.sql"`
- 原始输出：`docs/evidence/T10/skill-invocation-consistency-2026-11-12.log`

## 1. 本次执行结果

```
accepted_events	distinct_articles	rewards	txn_points
0	0	0	0
```

说明：本次复跑时演示库 `reading_activity_demo2` 已在上一轮 S5 收尾后被清空（`down -v` 重建），因此 Q1~Q13 全为 0，属"无数据即无不一致"的空库基线，**不能**当作业务正确性的正面证据。带数据的一致性结论见 `docs/evidence/T10/consistency-check-2026-11-12.md`（当日上午在 S1/S5 数据上执行）。

## 2. Skill 发现的问题与闭环

| 发现 | 位置 | 处理 | 复验 |
|---|---|---|---|
| 达标判定用 `==`，进度数超过门槛（历史缺口、跳档、批量补入）时不会补发 | `internal/service/event_service.go` | 改为 `>=`，重复发奖仍由 `reward` 唯一键 `uk_user_activity_date` 兜底 | `TestEvent_RewardWhenCountAboveThreshold`：3 篇进度无 reward → 第 4 篇补发 30 分 → 第 5 篇不重复；全量 `-race` 通过 |
| 申诉补入未复用统一事件路径会绕过窗口与去重 | `internal/service/agent_service.go` | 复核确认已复用 `HandleEvent`，无需改动 | `TestAgent_Compensate*` 现有用例 |
| 发布后仍可通过接口改规则 | `internal/service/ops_service.go` | 复核确认无更新入口，重复发布 409 | S1 演示 + `TestOps_*` |

## 3. 使用帮助与局限

- 帮助：把"流水=余额""达标=发奖""去重幂等"三组关系变成可执行 SQL，定位到 `count == 门槛` 这一处逻辑缺口，并转成回归用例。
- 局限：脚本只做数据一致性，不判断业务语义（例如"未读满三篇"是否合理）；空库执行无意义，必须先有演示数据；SQL 结果需要人工判读，不能自动给出通过结论。

## 4. 复跑步骤

```bash
# 1) 造数据后执行（示例库名 reading_activity_demo2）
docker cp scripts/consistency_queries.sql dev-mysql:/tmp/cq.sql
docker exec dev-mysql sh -c "mysql --default-character-set=utf8mb4 reading_activity_demo2 < /tmp/cq.sql"
# 2) 期望：Q13 中 txn_total = balance_total；Q5/Q6 无输出（无重复发奖/无冲突）
```
