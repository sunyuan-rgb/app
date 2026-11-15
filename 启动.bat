@echo off
chcp 65001 >nul
title 阅读活动系统 - 启动
cd /d %~dp0

docker info >nul 2>&1
if not errorlevel 1 goto ready

echo [1/3] Docker 未运行，正在启动 Docker Desktop（首次约需 1 分钟）...
start "" "C:\Program Files\Docker\Docker\Docker Desktop.exe"
set /a tries=0
:waitloop
timeout /t 6 /nobreak >nul
docker info >nul 2>&1
if not errorlevel 1 goto ready
set /a tries+=1
if %tries% lss 24 goto waitloop
echo Docker 启动超时，请手动打开 Docker Desktop 后重新运行本脚本
pause
exit /b 1

:ready
echo [2/3] 启动服务容器（mysql / server / readingsvc）...
docker compose up -d
echo.
docker compose ps
echo.
echo [3/3] 打开系统页面...
start http://localhost:8090/
echo.
echo ============================================
echo   系统已就绪：
echo     主服务    http://localhost:8090/  （用户端 / 客服端 / 运营端）
echo     阅读服务  http://localhost:8081/
echo     预置账号  user1 ~ user5，密码 123456（也可注册新账号）
echo   常用命令（在本目录打开终端执行）：
echo     docker compose logs -f server    实时查看服务日志
echo     docker compose ps                查看容器状态
echo     停止.bat                         停止全部服务
echo ============================================
pause
