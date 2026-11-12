#!/bin/sh
# 容器首次初始化：显式以 utf8mb4 连接执行迁移，避免中文按 latin1 写入产生乱码。
set -e
for f in /migrations/*.sql; do
  tmp="/tmp/$(basename "$f")"
  # 去除可能的 BOM，MySQL CLI 不识别
  sed '1s/^\xEF\xBB\xBF//' "$f" > "$tmp"
  echo "applying $f"
  mysql --default-character-set=utf8mb4 -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" < "$tmp"
done
echo "migrations done"
