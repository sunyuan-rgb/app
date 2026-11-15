@echo off
chcp 65001 >nul
title 阅读活动系统 - 停止
cd /d %~dp0
echo 正在停止服务容器（数据保留，下次启动不丢失）...
docker compose down
echo 已停止。重新启动请运行 启动.bat
pause
