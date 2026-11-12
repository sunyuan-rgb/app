#!/bin/sh
# 一键部署与回滚（教学环境/本机通用）。
#
# 用法：
#   ./scripts/deploy.sh              # 构建 + 启动 + 健康检查 + 业务冒烟；失败自动回滚
#   ./scripts/deploy.sh --rollback   # 回到上一次记入 .deploy/last-good 的版本
#
# 版本对应：部署成功后把当前提交写入 .deploy/last-good，供回滚与报告引用。
# 依赖：docker compose、curl；git 可选（无 git 时版本记为 no-git）。
# 环境变量：APP_PORT（默认 8080）、PROJECT（默认 t10）、USER_TOKEN（默认 user-token-1）。
set -u

PROJECT="${PROJECT:-t10}"
APP_PORT="${APP_PORT:-8080}"
USER_TOKEN="${USER_TOKEN:-user-token-1}"
# 构建期模块代理：默认国内可达地址，可用环境变量覆盖
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
BASE="http://127.0.0.1:${APP_PORT}"
STATE_DIR=".deploy"
LAST_GOOD="${STATE_DIR}/last-good"
LOG_DIR="${STATE_DIR}/logs"

fail() { echo "[deploy] $*" >&2; exit 1; }

command -v docker >/dev/null 2>&1 || fail "未找到 docker 命令，请确认 Docker Desktop/CLI 在 PATH 中"
mkdir -p "$STATE_DIR" "$LOG_DIR"

compose() { docker compose -p "$PROJECT" "$@"; }

current_rev() {
	if command -v git >/dev/null 2>&1; then
		git rev-parse --short HEAD 2>/dev/null || echo "no-git"
	else
		echo "no-git"
	fi
}

# 等健康检查通过（最多 90 秒）
wait_healthy() {
	i=0
	while [ "$i" -lt 45 ]; do
		if curl -fsS "${BASE}/healthz" >/dev/null 2>&1; then
			return 0
		fi
		i=$((i + 1))
		sleep 2
	done
	return 1
}

# 业务冒烟：以用户身份读活动与积分（只读，不改数据）
smoke() {
	curl -fsS "${BASE}/api/v1/users/activity" -H "Authorization: Bearer ${USER_TOKEN}" >/dev/null 2>&1 || return 1
	curl -fsS "${BASE}/api/v1/users/points" -H "Authorization: Bearer ${USER_TOKEN}" >/dev/null 2>&1 || return 1
	return 0
}

dump_logs() {
	compose logs --no-color --tail 200 >"${LOG_DIR}/deploy-fail-$(date +%Y%m%d-%H%M%S).log" 2>&1 || true
}

deploy() {
	rev="$(current_rev)"
	echo "[deploy] 版本 ${rev} 开始构建"
	compose build || { echo "[deploy] 构建失败" >&2; return 1; }
	compose up -d --remove-orphans || { echo "[deploy] 启动失败" >&2; return 1; }
	if wait_healthy && smoke; then
		printf '%s\t%s\n' "$rev" "$(date '+%Y-%m-%d %H:%M:%S')" >"$LAST_GOOD"
		echo "[deploy] 成功：${rev}（${BASE}/healthz 通过，冒烟通过）"
		return 0
	fi
	echo "[deploy] 失败：健康检查或业务冒烟未通过" >&2
	dump_logs
	return 1
}

rollback() {
	if [ ! -f "$LAST_GOOD" ]; then
		echo "[rollback] 无 ${LAST_GOOD}，无法回滚；请人工指定版本" >&2
		exit 1
	fi
	prev_rev="$(cut -f1 "$LAST_GOOD")"
	echo "[rollback] 回到上一可用版本 ${prev_rev}"
	if command -v git >/dev/null 2>&1 && [ "$prev_rev" != "no-git" ]; then
		git checkout "$prev_rev" || { echo "[rollback] 切换版本失败" >&2; exit 1; }
	fi
	compose build || { echo "[rollback] 构建失败" >&2; exit 1; }
	compose up -d --remove-orphans || { echo "[rollback] 启动失败" >&2; exit 1; }
	if wait_healthy; then
		echo "[rollback] 完成：${prev_rev} 已恢复可用"
	else
		echo "[rollback] 仍不健康，请查看 ${LOG_DIR}" >&2
		dump_logs
		exit 1
	fi
}

case "${1:-}" in
--rollback)
	rollback
	;;
"")
	if deploy; then
		exit 0
	fi
	echo "[deploy] 尝试自动回滚" >&2
	rollback
	;;
*)
	echo "用法: $0 [--rollback]" >&2
	exit 2
	;;
esac
