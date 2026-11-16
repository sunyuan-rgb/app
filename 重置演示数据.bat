@echo off
chcp 65001 >nul
title 阅读活动系统 - 重置演示数据
cd /d %~dp0

echo 将清空活动 / 阅读 / 发奖 / 申诉等业务数据（账号 user1~5、agent1、ops1 保留）。
echo 用途：系统同时只允许存在一个活动，遇到「409 已存在已发布活动」时运行本脚本即可重新发布。
echo.
set /p sure=确认重置？(Y/N)：
if /i not "%sure%"=="Y" goto :end

docker exec t10-mysql sh -c "mysql -uroot -p\$MYSQL_ROOT_PASSWORD \$MYSQL_DATABASE -e 'SET FOREIGN_KEY_CHECKS=0; TRUNCATE appeal; TRUNCATE daily_progress; TRUNCATE event_conflict; TRUNCATE point_txn; TRUNCATE reading_event; TRUNCATE reward; TRUNCATE trusted_record; TRUNCATE user_balance; DELETE FROM activity; SET FOREIGN_KEY_CHECKS=1;'"

echo.
echo 已重置。请刷新页面（Ctrl+F5），用运营端 ops1 登录重新发布活动。
:end
pause
