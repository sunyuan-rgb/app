# 任务清单（题目 10）

> 事后整理版（2026-11-12）。任务编号与 `docs/requirements.md` 规则号、测试文件、页面区域对应，用于验收逐项核对。
> 说明：本清单在实现完成后整理，不声称当时已按此编号逐步执行；历史执行顺序见检查报告第 4 节。

## 阶段与依赖

```
T01 环境与骨架
 └─ T02 迁移与数据模型
     └─ T03 活动发布（R1）
         ├─ T04 事件接入校验（R2）
         │   └─ T05 去重与进度（R3）
         │       └─ T06 发奖事务（R4/R7）
         │           ├─ T07 用户查询与申诉（R5）
         │           └─ T08 客服核查/驳回/补入（R6）
         │               └─ T09 结束汇总与不一致（R8）
         ├─ T10 三端页面
         └─ T11 模拟阅读服务
T12 分层测试（单测/集成/并发）
T13 质量门禁（gofmt/vet/race/覆盖率）
T14 容器化与自动部署
T15 演示脚本与证据归档
T16 自编 Skill（一致性 / 验收）
```

## 任务表

| 编号 | 任务 | 依赖 | 规则 | 完成条件 | 对应验证 |
|---|---|---|---|---|---|
| T01 | Go 模块、配置与时钟、路由骨架 | — | 共同1.1/1.9 | `go build ./...` 通过，`/healthz` 200，`TEACH_CLOCK_OFFSET` 生效 | `internal/clock`、`internal/config` 单测 |
| T02 | 迁移 001~004：活动/事件/进度/发奖/流水/余额/申诉/账号/可信记录 | T01 | 共同1.3 | 迁移可重复执行；唯一键就位 | `testutil.Setup` 每包独立库迁移 |
| T03 | 活动发布：时间校验、规则冻结、重复发布拒绝 | T02 | R1 | 发布成功返回 ID；二次发布 409；结束早于当前 400 | `TestOps_*` |
| T04 | 事件接入：token 鉴权、字段校验、活动期与接收时间校验、拒绝留痕 | T03 | R2 | 401/400/rejected 均有留痕 | `TestEvent_RejectedEventsArePersisted` |
| T05 | 去重与进度：事件唯一、同用户同日同文章一次、同编号换内容冲突 | T04 | R3 | `duplicated`/`dup_article`/`conflict` 三态正确 | `TestEvent_DuplicateEventIdempotent`、`TestEvent_SameIdDifferentContentConflict` |
| T06 | 发奖事务：门槛判定、reward 唯一键、reward+txn+balance 同事务 | T05 | R4/R7 | 达标只发一次；流水合计=余额；并发不重复 | `TestEvent_NormalThresholdReward`、`TestEvent_RewardWhenCountAboveThreshold`、并发用例 |
| T07 | 用户端：活动/进度/流水查询、一条未结申诉 | T06 | R5 | 只读本人；重复未结申诉被拒 | `TestUser_*`、handler 测试 |
| T08 | 客服端：详情三证据、驳回必写依据、补入复用统一事件路径 | T06 | R6/R7 | 补入触发同一发奖规则；驳回留依据与操作者；二次处理 409 | `TestAgent_*` |
| T09 | 结束汇总：达标/发奖人数、积分总额、申诉结果、不一致明细、窗口关闭判定 | T06/T08 | R8 | `window_closed` 按结束+24h 判定；脏数据可追溯 | `TestOps_Summary*` |
| T10 | 三端页面 + 入口导航（纯 HTML + fetch） | T07/T08 | 共同1.2 | 五页可访问并完成主流程与失败提示 | 页面 200 探测 + 演示脚本 |
| T11 | 模拟阅读服务：上报、可信查询、失败注入 | T04 | 共同1.5 | 成功/失败/超时/重复通知四态可触发 | `TestHTTPReadingService_*` |
| T12 | 分层测试：store/handler/service/readingsvc/server 各自有用例 | T06~T11 | 共同6.1/6.2 | 覆盖正常、边界、非法状态、重复、权限、异常 | 各包 `go test` |
| T13 | 质量门禁：gofmt、vet、-race、覆盖率 ≥85% 且核心包单独达标 | T12 | 共同6.1/6.5 | `covergate` 输出"门禁结果: 通过" | `docs/evidence/test/*` |
| T14 | 容器化与流水线：Dockerfile、compose、迁移初始化、CI 两阶段（含部署冒烟） | T13 | 共同7 | 本地 compose 起服务并冒烟通过；CI 配置含部署与健康检查 | `deploy-check-*.log`、`.github/workflows/ci.yml` |
| T15 | 演示脚本与证据归档：S1~S5 可复跑、时钟模式、汇总收尾 | T14 | 共同6.4/10 | 脚本可重复执行并输出可核对结果 | `docs/evidence/T10/*.log` |
| T16 | 自编 Skill：业务一致性核查 + 验收检查 | T13 | 共同4 | 两 Skill 含七要素，且实际产出核查报告 | `docs/evidence/T10/consistency-check-*.md`、`docs/evidence/skills/skill-usage.md` |

## 与检查报告缺口的对应

| 检查项 | 本清单相关任务 | 当前状态 |
|---|---|---|
| B1-02 需求说明 | 本文档 + `docs/requirements.md` | 补齐 |
| B1-03 原型与状态说明 | `docs/prototype.md` | 补齐（原型阶段文件仍缺，历史问题保留） |
| B1-04 任务清单 | 本文档 | 补齐 |
| A3-05 缺陷登记 | `docs/defects.md` | 补齐记录；飞书 Bug 单仍需学生登记 |
| B2-03/04/05 过程导出 | `docs/ai-usage.md`；会话导出仍缺 | 部分补齐 |
| B4 提交与 PR | 未开始（需授权与工作项编号） | 未补齐 |
