# 缺陷登记与复测记录（题目 10）

> 整理日期 2026-11-12。**未登记飞书 Bug 工作项**，因此下表没有真实 Bug 编号，也不能作为 `fix` 类型提交的依据；如需合规提交，须先在飞书登记并回填编号。
> 记录原则：只登记有复现或复测证据的缺陷，不补造数量。

## 汇总

| 编号 | 缺陷 | 严重度 | 发现阶段 | 修复 | 复测 | 状态 |
|---|---|---|---|---|---|---|
| D-01 | 多个测试包共用同一测试库，`go test ./...` 并行互删表导致随机失败 | 阻塞 | 独立验收 | `testutil` 按包派生库名 | 并行 `-race` 全绿 | 已修复 |
| D-02 | 容器初始化迁移按 latin1 执行，活动名中文乱码 | 高 | 本机部署 | `scripts/initdb.sh` 显式 utf8mb4 + server 字符集 | 重建数据卷后活动名正确 | 已修复 |
| D-03 | 达标判定用 `==`，历史进度已达标但发奖缺失时不补发 | 中 | 自编一致性 Skill + 审计 | 改为 `>=`，由 reward 唯一键兜底 | 新增 `TestEvent_RewardWhenCountAboveThreshold` 通过 | 已修复 |
| D-04 | 发奖前进程崩溃导致"有进度无 reward"无法自愈 | 中（同 D-03 根因） | 代码走查 | 同 D-03（下次事件到达即补发） | 同 D-03 用例 | 已修复 |
| D-05 | `gofmt -l` 5 个文件未格式化 | 低 | 独立验收 | 全部格式化 | `gofmt -l` 无输出 | 已修复 |
| D-06 | Makefile / testutil 存在明文口令 | 高 | 独立验收 | 口令外置到 `.env` / `.env.test`（gitignored） | 仓库扫描无明文 | 已修复 |
| D-07 | Docker 构建无 GOPROXY、compose 端口写死 | 中 | 本机部署 | `ARG GOPROXY`、`MYSQL_PORT`/`APP_PORT` 参数化 | 构建成功、端口冲突可绕 | 已修复 |
| D-08 | store 测试期望 4 条流水，实际 3 条 | 低（测试自身） | 补测阶段 | 修正断言为 3 条并说明来源 | 该用例稳定通过 | 已修复 |

## 明细

### D-01 测试基建并发缺陷

- 现象：`go test ./...`（不带 `-p 1`）随机失败，报 `Duplicate column name 'open_flag'`、`Table 'reading_activity_test.activity' doesn't exist`；service 59.9%、store 46.8% FAIL。
- 根因：`internal/testutil/testutil.go` 固定库名 `reading_activity_test`，各包 `Setup` 并行 drop/重建互相踩踏。
- 修复：按 `os.Args[0]` 派生 `reading_activity_test_<pkg>`，非法字符清洗并限长 40。
- 复测：`go test -count=1 -race ./...` 并行通过；日志 `docs/evidence/test/test-race.log`。
- 遗留：无（该缺陷没有 Bug 单编号）。

### D-02 迁移字符集导致中文乱码

- 现象：compose 首次启动后活动名返回 `æ¯æ—¥é˜…è¯»…`。
- 根因：容器内 `mysql` 客户端默认 latin1 执行 `001_init.sql`，中文被按 latin1 解释。
- 修复：新增 `scripts/initdb.sh`，逐文件 `--default-character-set=utf8mb4` 执行；MySQL 启动参数加 `--character-set-server=utf8mb4`。
- 复测：`docker compose down -v && up -d` 后活动名正确，`deploy-check-2026-11-12.log` 记录。

### D-03 达标判定应使用下限而非等值

- 现象：进度数 `count` 恰等于门槛才发奖；一旦某日进度已 ≥ 门槛但 reward 缺失（发奖前失败、人工修数、客服批量补入跳档），后续新增事件不会补发。
- 根因：`internal/service/event_service.go` 判定 `count == activity.DailyThreshold`。
- 修复：改为 `count >= activity.DailyThreshold`；重复发奖仍由 `reward` 唯一键 `uk_user_activity_date` 保证幂等。
- 复测：`TestEvent_RewardWhenCountAboveThreshold`（3 篇进度无 reward → 第 4 篇补发 → 第 5 篇不重复）通过。
- 影响面：仅收敛为更宽松的补发条件，正常路径行为不变（原 93 个用例全绿）。

### D-05 / D-06 / D-07

- D-05：验收阶段 `gofmt -l` 列出 5 个文件；执行 `gofmt -w` 后为空输出。
- D-06：`Makefile:17`、`internal/testutil/testutil.go` 曾硬编码 DSN 口令；改为 `APP_DSN` / `TEST_DSN` 注入，`.env` / `.env.test` 进 `.gitignore`。
- D-07：Dockerfile 增加 `ARG GOPROXY`（默认 `goproxy.cn`）；compose 使用 `${MYSQL_PORT:-3306}`、`${APP_PORT:-8080}`。

### D-08 前端 POST 请求丢失 Authorization 头（2026-11-13 部署演示中发现）

- 现象：运营端点「发布」提示 `401 缺少访问令牌`；页面内令牌（`ops-token-1`）实际存在，GET 类查询全部正常。
- 根因：`internal/server/web/{ops,user,agent}.html` 的 `api()` 封装写法为 `{headers:{Authorization:...}, ...opts}`——当调用方传入 `opts.headers`（POST 必带 Content-Type）时，展开顺序导致整个 `headers` 被覆盖，`Authorization` 丢失。
- 修复：三端统一改为 `{...opts, headers:{Authorization:..., ...(opts.headers||{})}}`，令牌头恒保留且允许调用方附加头。
- 复测：容器重建后（`docker compose up -d --build`）运营端发布成功返回 200 `已发布 #id`；用户端申诉提交、客服端补入/驳回同步验证通过。
- 影响面：纯前端 JS 封装，接口与鉴权逻辑无改动。

## 待补动作（合规所需）

1. 在飞书项目 `r17te9` 逐条登记上述缺陷，取得真实 Bug 编号。
2. 用 `meegle workitem get --work-item-id <编号> --project-key r17te9` 核验后，以 `[fix] [v1.0.x] [#编号] 中文描述` 提交修复。
3. 登记后回填本表的"Bug 编号"列，并把复测日志挂到对应提交说明。
