# Skill 调用记录（题目 10）

## 1. reading-activity-consistency-check

- **触发原因**：事件接入、发奖、申诉代码变更完成后，按技能要求做跨环节一致性核查
- **输入**：仓库 `app/`（代码层）+ `reading_activity` 库（数据层）
- **执行**：新建并运行 `scripts/consistency_queries.sql`（Q1~Q13 确定性查询）于容器 dev-mysql
- **输出**：`docs/evidence/T10/consistency-check-2026-11-12.md`
- **结论**：通过（阻塞项 0）。Q1~Q6、Q9~Q12 违规 0 行；Q13 `txn_total = balance_total = 30`、`user_days=1` 对应 `reward_rows=1`
- **发现并记录**：1 条低风险备注（达标判定 `==`，建议改 `>=`，不阻塞）

## 2. reading-activity-acceptance-check

- **触发原因**：验收报告（`acceptance/acceptance-report-2026-11-12.md`）判定不通过，需按原始题面逐项复核
- **输入**：原始题面八条规则、五个必演场景、质量门禁与红线清单
- **执行**：
  - 代码层：逐条比对实现位置（发布固定、鉴权、去重、发奖事务、申诉补发、越权、留痕、结算核查）
  - 数据层：`reading_activity` 库实测（事件/申诉/奖励/流水/余额）
  - 场景层：`scripts/demo_scenarios.ps1`（S2/S3/S4/S5）、`scripts/demo_scenarios_full.ps1`（S1 发布链路、S2 跨日与结束后窗口、S5 并发与窗口关闭）
  - 质量层：`-race` 全量、覆盖率门禁 `scripts/covergate`
- **输出**：
  - `docs/evidence/T10/demo-scenarios-2026-11-12.log`（S2/S3/S4/S5 实际操作留痕）
  - `docs/evidence/T10/demo-scenarios-full-2026-11-12.log`（S1/S2 跨日/S5 并发与窗口关闭）
  - `docs/evidence/acceptance/整改对照表-2026-11-12.md`（逐条回应）
- **结论**：八条业务规则全部符合；五个必演场景全部可复演且留痕；质量门禁通过（业务总覆盖 92.0%，核心包均 ≥85%）；流程项（Git 提交/PR/Actions 运行记录）需学生确认后完成

## 3. 其他技能

- `lsp-code-analysis`：用于定位 handler/service/store 的符号与引用（开发期）
- 未使用代码生成类技能（业务代码全部手写，避免同题相似性）
