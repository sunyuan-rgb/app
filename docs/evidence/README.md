# 证据归档目录

按阶段归档证据，供验收与视频讲解引用。**下方"目录"列说明现状**。

| 目录 | 内容 | 现状 |
|---|---|---|
| `requirements/` | 需求分析、状态说明、假设确认记录 | 正文见 `docs/requirements.md`、`docs/assumptions.md`；阶段原始记录仍缺 |
| `prototype/` | 原型文件 | 页面契约见 `docs/prototype.md`；原型阶段文件仍缺 |
| `plan/` | 任务清单 | 正文见 `docs/tasks.md`；施工时原始清单仍缺 |
| `dev/` | 开发会话、审核修正记录 | AI 分工与审核见 `docs/ai-usage.md`；会话导出仍缺 |
| `test/` | **已完成**：`gofmt.txt`、`vet.txt`、`test-race.log`（全量 -race 原始输出）、`coverage-raw.out`、`coverage-summary.txt`（按包+门禁）、`per-package-coverage.log`（各包自测） | 已归档（2026-11-12 复跑，`-p 2` 规避 Windows 页面文件限制） |
| `acceptance/` | **已完成**：`acceptance-report-2026-11-12.md`（原始验收报告）、`整改对照表-2026-11-12.md`（逐条回应与证据） | 已归档 |
| `deploy/` | **已完成**：`deploy-check-2026-11-12.log`（docker compose 部署 + 健康检查 + 事件→发奖→汇总冒烟）、`deploy-script-2026-11-12.log`（`scripts/deploy.sh` 实测） | 已归档 |
| `ci/` | **已完成**：`ci-run-2026-11-12.md` + Actions 官方日志压缩包（quality+deploy 全绿，run 43162716685） | 已归档 |
| `skills/` | **已完成**：`skill-usage.md`（两个自编 Skill 的说明与结论）、`../T10/skill-invocation-consistency-2026-11-12.log`（一致性 SQL 实跑输出） | 已归档 |
| `sessions/` | 会话导出与结构化事件清单 | 待学生补（平台会话未导出） |
| `T10/` | 跨环节一致性核查报告、必演场景演示日志、Skill 实跑输出 | 已归档 |
| `gaps.md` | 材料缺口清单与原因（不补造） | 已归档 |

## 复现命令

```bash
# 质量证据（-p 2：Windows 上 -race 并发链接过多会耗尽页面文件）
gofmt -l . && go vet ./... \
  && go test -race -p 2 -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./... \
  && go run ./scripts/covergate coverage.out

# 部署证据
cp .env.example .env    # 填写 MYSQL_ROOT_PASSWORD
MYSQL_PORT=3307 docker compose up -d --build
curl localhost:8080/healthz
```

> 空文件（0 字节）即代表该命令输出为空、检查通过（`gofmt.txt`、`vet.txt`）。
> 若并行度提高后出现 `fork/exec ... link.exe: The paging file is too small`，属本机虚拟内存不足，不是测试失败；降低 `-p` 或增大页面文件后重跑即可。
