@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

REM DropLAN 文件互传服务 - Windows 启停脚本
REM 用法: run.bat [start|stop|status]

set "DIR=%~dp0"
set "PID_FILE=%DIR%server.pid"
set "LOG_FILE=%DIR%server.log"
set "ACTION=%1"
if "%ACTION%"=="" set "ACTION=start"

REM 跨平台获取局域网 IP（socket 连接法，与 server.py 一致）
set "PS_GET_IP=powershell -NoProfile -Command "$s=New-Object System.Net.Sockets.Socket([System.Net.Sockets.AddressFamily]::InterNetwork,[System.Net.Sockets.SocketType]::Dgram); $s.Connect('8.8.8.8',80); $s.LocalEndPoint.Address.IPAddressToString; $s.Close()"

if "%ACTION%"=="start" goto :start
if "%ACTION%"=="stop" goto :stop
if "%ACTION%"=="status" goto :status
echo 用法: run.bat [start^|stop^|status]
goto :end

:start
REM 检查是否已在运行
if exist "%PID_FILE%" (
    set /p OLD_PID=<"%PID_FILE%"
    tasklist /FI "PID eq !OLD_PID!" 2>nul | find "!OLD_PID!" >nul
    if not errorlevel 1 (
        echo 服务已在运行 (PID: !OLD_PID!)
        goto :end
    )
)
REM 后台启动（合并 stdout/stderr 到日志）
pushd "%DIR%"
start "" /b python server.py > server.log 2>&1
popd
timeout /t 2 /nobreak >nul
REM 查询 python 跑 server.py 的 PID
set "PID="
for /f %%i in ('powershell -NoProfile -Command "(Get-CimInstance Win32_Process -Filter \"Name='python.exe'\" | Where-Object { $_.CommandLine -like '*server.py*' } | Select-Object -Last 1).ProcessId"') do set "PID=%%i"
if "!PID!"=="" (
    echo ❌ 启动失败，查看日志: %LOG_FILE%
    goto :end
)
echo !PID!>"%PID_FILE%"
REM 查询局域网 IP
set "LAN_IP="
for /f "tokens=*" %%i in ('!PS_GET_IP! 2^>nul') do set "LAN_IP=%%i"
echo ✅ 服务已启动 (PID: !PID!)
if defined LAN_IP (echo 🌐 访问地址: http://!LAN_IP!:8000) else (echo 🌐 访问地址: http://<本机IP>:8000)
goto :end

:stop
if not exist "%PID_FILE%" (
    echo 服务未在运行
    goto :end
)
set /p PID=<"%PID_FILE%"
tasklist /FI "PID eq !PID!" 2>nul | find "!PID!" >nul
if not errorlevel 1 (
    taskkill /PID !PID! /F >nul
    echo ✅ 服务已停止
) else (
    echo 服务未在运行
)
del "%PID_FILE%" 2>nul
goto :end

:status
if not exist "%PID_FILE%" (
    echo ❌ 未运行
    goto :end
)
set /p PID=<"%PID_FILE%"
tasklist /FI "PID eq !PID!" 2>nul | find "!PID!" >nul
if errorlevel 1 (
    echo ❌ 未运行
    del "%PID_FILE%" 2>nul
    goto :end
)
echo ✅ 运行中 (PID: !PID!)
set "LAN_IP="
for /f "tokens=*" %%i in ('!PS_GET_IP! 2^>nul') do set "LAN_IP=%%i"
if defined LAN_IP (echo 🌐 http://!LAN_IP!:8000) else (echo 🌐 http://<本机IP>:8000)
goto :end

:end
endlocal
