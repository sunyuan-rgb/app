# 资讯 App 阅读活动系统（题目 10）

题目 10《资讯 App 阅读活动的发布、奖励、申诉与结算核查》的教学实现。
完整业务实现、测试与证据见 `AGENTS.md` 与 `docs/evidence/`。

## 功能范围

- 运营：创建并发布活动、查看发放汇总与异常核查（窗口关闭判定、不一致明细）
- 用户：查看活动与每日进度、积分流水、提交申诉
- 模拟阅读服务：发送阅读事件、提供可信阅读记录查询、演示页面 `/`
- 客服：核查申诉（含可信记录交叉核对）、驳回（须写依据）或补入缺失记录（复用统一发奖规则）

## 技术栈

- Go 1.26 + Gin
- MySQL（唯一索引承载去重/幂等约束；业务日按北京时间、事件发生日期归属）
- 简单 HTML 页面（Go html/template）

## 快速开始

```bash
cp .env.example .env        # 填写 MYSQL_ROOT_PASSWORD / APP_DSN
cp .env.example .env.test   # 填写 TEST_DSN（测试库基础连接串，以 / 结尾）
make db-migrate             # 对已运行的 MySQL 应用 001~003 迁移
make run                    # 主服务 :8080
make run-reading            # 模拟阅读服务 :8081
```

一键部署（MySQL + 双服务）：

```bash
cp .env.example .env && vi .env      # 设置 MYSQL_ROOT_PASSWORD
MYSQL_PORT=3307 docker compose up -d --build   # 3307 仅本机端口映射（避免与本机 MySQL 冲突）
curl localhost:8080/healthz
```

部署与回滚脚本（构建 → 启动 → 健康检查 → 业务冒烟 → 记录版本；失败自动回滚）：

```bash
MYSQL_PORT=3307 ./scripts/deploy.sh             # 成功后在 .deploy/last-good 记录版本
MYSQL_PORT=3307 ./scripts/deploy.sh --rollback  # 回到上一可用版本
```

> 口令一律通过 `.env` / `.env.test` / 环境变量注入，仓库内不含任何口令。
> 若 `3306` 已被本机 MySQL 占用，MySQL 容器会因端口绑定失败而无法接入网络（表现为服务 500、日志 `lookup mysql ... no such host`），用 `MYSQL_PORT=3307` 重新 `up -d` 即可。

## 测试与质量门禁

```bash
make test     # go test -race ./...（各包独立测试库，可并行）
make cover    # 覆盖率 profile
go run ./scripts/covergate coverage.out   # 门禁：业务总覆盖 ≥85%，核心包各自 ≥85%
```

## 演示场景脚本

| 脚本 | 内容 |
|---|---|
| `scripts/demo_scenarios.ps1` | S2 去重/超窗、S3 申诉成立补发、S4 驳回、S5 越权、S1/S8 汇总 |
| `scripts/demo_scenarios_full.ps1` | S1 从运营发布开始全链路、S2 跨日迟到/活动结束后窗口、S5 并发与窗口关闭终局核查 |
| `scripts/smoke_agent.ps1` | 客服端冒烟 |
| `scripts/consistency_queries.sql` | 跨环节一致性核查（Q1~Q13，确定性 SQL） |

教学时钟：`TEACH_CLOCK_OFFSET=50h`（或 74h）启动服务即可演示跨日迟到与窗口关闭；偏移超过 ±48h 会被配置校验拒绝。

## 分支

- `main`：受保护主分支，仅经 PR 合并
- `dev/10`：题目 10 开发分支

## 证据索引

- `docs/evidence/test/`：`gofmt` / `go vet` / `-race` 原始输出、覆盖率 profile 与汇总
- `docs/evidence/T10/`：一致性核查报告、必演场景演示日志
- `docs/evidence/acceptance/`：验收报告与整改对照
