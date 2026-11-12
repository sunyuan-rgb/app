# 题目 10 验收报告（资讯 App 阅读活动）

- 验收日期：2026-11-12
- 验收方式：独立验收会话（reading-activity-acceptance-check Skill），依据原始题面《Go项目业务场景题.md》题目 10 及第一部分通用要求，逐项核对本仓库工作区
- 验收版本：**dev/10 HEAD = fe82c82**（该提交仅含 README 与 .gitignore；cmd/、internal/、migrations/、scripts/、AGENTS.md、Makefile、.github/ 等全部业务代码与配置**均处于未提交（untracked）状态**）

## 最终结论：**不通过**

代码层八条规则实现质量较高、红线项无违反，但 **Git 流程完全未建立（业务代码零提交）、覆盖率实测未达标、质量证据未归档、部署未发生**，按题面"任一关键项缺失即不得判定通过"判定不通过。

---

## A. 八条业务规则（代码层逐项）

| # | 条款 | 结论 | 证据位置 | 差距 |
|---|---|---|---|---|
| R1 | 发布后规则固定，不改规则、不撤销积分 | 通过 | `internal/service/ops_service.go:41-51,63-64`（发布校验+重复发布拒绝）；`internal/server/router.go:77-81`（无任何活动编辑路由）；`migrations/002_seed.sql:4-6`（预置活动固定） | 实现方式是"无编辑接口"而非显式字段级禁改校验，效果等价，备注即可 |
| R2 | 事件鉴权+全局唯一编号+发生时间∈活动期且≤接收时间 | 通过 | `internal/server/router.go:86-95`（X-Source-Token 401）；`internal/server/handler/event_handler.go:25-30`（字段必填）；`internal/service/event_service.go:65-70`（未来时间拒绝）、`:79-81`（活动期外拒绝） | 事件内 user_id 依赖投递方令牌可信（教学假设 A-01，已记录） |
| R3 | 同用户同日同文章一次；同编号换内容拒绝；迟到≤24h 归发生日 | 通过 | `internal/service/event_service.go:107-129,131-147,213-215`；唯一索引 `migrations/001_init.sql:30,50`；冲突留痕表 `001_init.sql:34-41`；`internal/model/model.go:11-13`（biz_date=发生时间北京日期）；数据层实测 Q1/Q7/Q9 均 0 行异常 | 无 |
| R4 | 达标 3 篇只发一次 30 分；流水余额一致；结束后窗口内仍受理 | 通过 | `internal/service/event_service.go:85-180`（留痕+进度+reward+txn+balance 单事务）；发奖唯一键 `001_init.sql:60`；测试 `TestEvent_AfterActivityEndWithinWindow` | 无 |
| R5 | 按日期查进度、申诉（每日 1 条未结）；不能看他人数据；客服不能改事件/余额 | 通过 | `internal/service/user_service.go:70-163`；每日一条未结为数据库级约束 `migrations/003_users_appeal.sql:13-16`（open_flag 生成列+唯一键）；身份服务端注入 `router.go:97-117`（uid 不可伪造）；`agent_handler.go` 全文无改余额/改事件接口；全库无 `UPDATE user_balance` 直改语句 | 无 |
| R6 | 客服结合事件+可信查询核查；驳回说明；补入走相同规则且受窗口约束 | 通过 | `internal/service/agent_service.go:67-108`（核查详情含 readingsvc 可信查询 `internal/readingsvc/readingsvc.go:177-201`）；`:111-131`（空依据驳回拒绝）；`:142-168`（补入复用 `HandleEvent`，同受活动期/24h 窗口约束）；测试 `TestAgent_CompensateValidation`（超窗补入拒绝） | 无 |
| R7 | 自动入账与申诉处理并发只发一次；已获奖告知流水；留痕 | 通过 | `internal/store/event_store.go:90-103`（1062 幂等）；`internal/store/agent_store.go:78-94`（结案乐观锁 status=0）；`internal/service/agent_service.go:173-184`（返回 RewardID+TxnNote）；结案写 conclusion/operator_id/handled_at | 无 |
| R8 | 窗口关闭后运营汇总，不把事件条数当达标 | 通过（代码） | `internal/service/ops_service.go:92-159`（WindowClosed+逐日汇总+不一致明细）；`internal/store/ops_store.go:61-66`（`COUNT(DISTINCT article_id) >= 门槛`，注释明示不按事件条数）；`ops_store.go:93-118`（双向不一致检出，测试 `TestOps_SummaryDetectsInconsistency`） | **窗口关闭后的实际汇总演示缺失**（见 S5） |

## B. 五个必演场景（实际操作 + 持久化结果）

演示记录：`docs/evidence/T10/demo-scenarios-2026-11-12.log`；脚本：`scripts/demo_scenarios.ps1`；数据库留痕已由验收会话直接查询复验（appeal #13/#14、reward #7/#9、balance user3=30/user4=30、txn ref_id 对应一致）。

| 场景 | 结论 | 证据 | 差距 |
|---|---|---|---|
| S1 正常活动 | **部分** | 汇总接口返回 + DB reward #7/#9、user3/4 各 30（demo log 39-48 行；DB 实查一致） | "运营发布→阅读→自动发奖"完整链路未从发布步骤演示（依赖 seed 预置）；用户查流水仅在 S3 段落出现 |
| S2 重复与延迟 | **部分** | 重复文章 `dup_article`、同编号幂等 `duplicated`、超窗拒绝并留痕（demo log 5-9 行） | **跨日迟到按发生日归属、活动结束后窗口内事件受理**两项未演示（需教学时钟 TEACH_CLOCK_OFFSET，log 与脚本均无对应步骤） |
| S3 申诉成立 | 通过 | demo log 11-24 行；DB：appeal#13 status=1 operator_id=6、reward#9、重送幂等 `duplicated`、重复补入 409、余额=流水=30 | 无 |
| S4 申诉不成立 | 通过 | demo log 26-32 行；DB：appeal#14 status=2、user5 balance=0 无流水 | 无 |
| S5 并发与收尾 | **部分** | 越权 403/事件缺令牌 401 已演示（demo log 34-37 行） | **自动入账 vs 客服补发并发仅单元测试**（`TestConcurrent_AutoAndAgentPaths`），无实际操作演示；**窗口结束后核清未演示**（汇总返回 `window_closed:false`，未见 window_closed=true 的终局核查结果） |

按技能标准："仅文档说明/仅测试代码、无实际操作+持久化结果"的 S2 迟到段与 S5 并发/收尾段按**未证实**处理。

## C. 质量证据

| 项 | 要求 | 结论 | 证据 |
|---|---|---|---|
| C1 | 总覆盖 ≥85%，核心包分别 ≥85% | **不通过** | 验收会话实测（`go test -p 1 -coverprofile -covermode atomic`）：**总覆盖 67.2%**；核心包 `internal/service` **84.0%**、`internal/store` **50.5%**、`internal/server/handler` **77.2%** 均未达 85%；其余：readingsvc 91.0%、server 96.7%、clock/config/model 100% |
| C2 | 覆盖正常/边界/非法/重复/权限/异常路径 | 通过（用例设计层面） | 迟到边界、跨日归属、超窗、换内容冲突、并发、越权 403、申诉唯一等均有用例（`event_service_test.go`、`agent_service_test.go`、`router_test.go` 等） |
| C3 | 并发可重复 + `-race` 通过 | **不通过** | ① 本机 Windows 无 gcc/cgo，`-race` 无法运行，**无任何归档的 -race 通过原始输出**；② 实测发现严重缺陷：`go test ./...` 并行执行时，`internal/service` 与 `internal/store` 等包**共用同一测试库 `reading_activity_test`**（`internal/testutil/testutil.go:18`），各包 Setup 并行 drop/重建表互相干扰，导致随机失败（实测复现：`Duplicate column name 'open_flag'`、`Table 'reading_activity_test.activity' doesn't exist`，service 59.9%/store 46.8% FAIL），必须 `-p 1` 串行才稳定 → "并发验证可重复执行"不成立 |
| C4 | 测试原始输出、coverage 原始文件、可读报告在案 | **不通过** | `docs/evidence/README.md:9` 声明的 `test/` 目录不存在；coverage.out、测试输出、缺陷复测记录均未归档 |
| C5 | gofmt、go vet 通过 | **不通过** | `gofmt -l` 实测 5 个文件未格式化：`internal/testutil/errdb.go`、`errdb_test.go`、`internal/store/store_error_test.go`、`internal/service/service_error_test.go`、`internal/server/handler/handler_error_test.go`；`go vet` 通过；`.github/workflows/ci.yml:20` 有 gofmt 门禁但无运行记录 |

## D. 流程与部署证据

| 项 | 要求 | 结论 | 证据 |
|---|---|---|---|
| D1 | GitHub MCP 建仓记录；业务代码在 dev/10；main 禁直推 | **不通过** | remote 为 `github.com/sunyuan-rgb/app`，但**无任何 MCP 建仓证据归档**；`git log` 仅 2 个提交（main "Initial commit"、dev/10 "[docs] 初始化…"），**全部业务代码 untracked**，未进入 dev/10 分支历史；main 分支保护规则无截图/读取记录 |
| D2 | 提交符合 commit-convention；meegle 核验留痕 | **不通过** | 无任何 feat/fix 提交 → 无版本号、无 `#编号`、无 meegle CLI 核验输出；唯一 dev/10 提交为 docs 类型（可无编号，勉强合规） |
| D3 | PR 附编号/测试/覆盖率/验收报告；版本哈希对应 | **不通过** | 无 PR；验收前无覆盖率与测试结果可附 |
| D4 | 流水线含质量检查→覆盖率门禁→构建→部署→健康检查→冒烟 | **不通过** | `.github/workflows/ci.yml` 仅 4 步（format/vet/test+coverage 门禁/build），**缺部署、健康检查、冒烟阶段**；无任何流水线运行记录 |
| D5 | 实际部署；部署版本=验收版本；回滚/撤销方案 | **不通过** | 无部署记录、无服务地址、无回滚与首次失败撤销方案 |
| D6 | AGENTS.md 生效记录；两个自编 Skill；skill-usage.md | **部分不通过** | `AGENTS.md` v1.0（2026-11-11）针对本题、内容完整；两个自编 Skill 存在于 `.codebuddy/skills/reading-activity-acceptance-check/SKILL.md`、`.codebuddy/skills/reading-activity-consistency-check/SKILL.md`；但 **`skill-usage.md` 缺失**，除一致性检查报告（consistency-check-2026-11-12.md，质量较高）外**无 Skill 调用记录归档** |
| 补充 | 需求分析/原型/任务清单 | **缺失** | `docs/evidence/README.md:5-13` 声明的 `requirements/`、`prototype/`、`plan/`、`dev/`、`sessions/` 等目录全部不存在，需求说明、原型、任务清单产物缺失 |

## E. 红线检查

| 红线 | 结论 | 证据 |
|---|---|---|
| 客服直接修改余额或原始事件 | 未违反 | `agent_service.go` 仅调 `ResolveAppeal` 与 `HandleEvent`；全库无 `UPDATE user_balance` 直改、无 UPDATE `reading_event` 已有记录 |
| 发奖未与流水/余额同事务 | 未违反 | `event_service.go:86-180` 单事务（BeginTx/Commit/defer Rollback） |
| 重复发奖 | 未违反（代码层） | 发奖唯一键 `001_init.sql:60` + 1062 捕获；数据层实测 user3/user4 各恰 1 条 reward、1 条 txn、余额相等 |
| 越权访问未被拒绝 | 未违反 | 服务端 uid 注入 + role 校验；实测 403/401（demo log 34-37 行） |
| 静态页面/写死结果 | 未违反 | 4 个真实页面（index/user/agent/ops + readingsvc read 页），业务逻辑真实落库（DB 实查复验一致） |
| 密钥/敏感信息入库 | **高风险（尚未入库但按现状提交即入库）** | `Makefile:17` 含 root 口令 `Root@123456`；`internal/testutil/testutil.go:28` 含 `reading:Reading123!`；`internal/config/config.go` 默认令牌、`migrations/003_users_appeal.sql` 固定 token。当前因未提交而暂未进仓库，但均不在 .gitignore 排除范围，**一旦按现状提交即违反题面红线与 AGENTS.md 第 5 节** |

---

## 不通过项整改清单（按优先级）

1. **【流程，阻塞级】将全部业务代码按 commit-convention 提交到 dev/10**：拆分为 feat（关联真实需求编号，提交前 meegle CLI 核验并留输出）、test、docs 等多次提交；提交前先处理敏感信息（见第 6 条）。
2. **【质量，阻塞级】覆盖率提升至门槛**：总覆盖 67.2%，`internal/store` 50.5%、`internal/service` 84.0%、`internal/server/handler` 77.2% 需补测试（store 层错误分支与 GetEventByID/GetAppealByID 等低覆盖函数为首要缺口）。
3. **【质量，阻塞级】修复测试基建并发缺陷**：多个测试包共用 `reading_activity_test` 库且 Setup 互相 drop/重建表，`go test ./...` 并行必现随机失败（验收会话已复现）。方案：每包独立库名（如 `reading_activity_test_<pkg>`）或全局串行化建库；修复后在有 cgo 的环境（或 CI ubuntu）跑 `go test -race ./...` 并归档原始输出。
4. **【质量】gofmt 格式化 5 个文件**（`make fmt` 即可），归档 gofmt/vet/测试/coverage.out 原始输出至 `docs/evidence/test/`。
5. **【演示，阻塞级】补齐必演场景缺口**：用 `TEACH_CLOCK_OFFSET` 教学时钟实际演示 S2 跨日迟到归属、活动结束后窗口内事件；实际操作演示 S5 自动入账 vs 客服补发并发、以及窗口关闭后（window_closed=true）的终局汇总核查；补 S1 从"运营发布"起步的完整链路；全部更新 demo log 并确认 DB 留痕。
6. **【红线】敏感信息出库**：Makefile/testutil 中的口令改为环境变量或 `.env.example` 占位，`003` 种子 token 注明仅教学环境（已有注释，保持），再提交。
7. **【流程】补齐证据归档**：MCP 建仓记录、main 分支保护设置截图、流水线运行记录（ci.yml 需补部署/健康检查/冒烟阶段）、部署地址与版本、回滚方案、skill-usage.md、需求说明/原型/任务清单归档到 docs/evidence/ 对应目录。
8. 完成整改后从 dev/10 发起 PR（附编号、测试结果、覆盖率、验收报告），合并后对 main 实际版本重跑流水线并部署，再对受影响条款复验并记录复验版本哈希。

## 复验说明

- 本报告基于工作区 fe82c82 + untracked 代码。业务代码一旦提交（产生新哈希），本报告全部结论需对照新哈希复验受影响条款。
- C3 的 -race 项需在支持 cgo 的环境复跑后才能改判。
