@echo off
setlocal enabledelayedexpansion

REM Aellus file transfer service - Windows start/stop script
REM Usage: run.bat [start|stop|status]

set "DIR=%~dp0"
set "PID_FILE=%DIR%server.pid"
set "LOG_FILE=%DIR%server.log"
set "ACTION=%1"
if "%ACTION%"=="" set "ACTION=start"

REM Get LAN IP (socket method, same as server.py) - extracted to get_lan_ip.ps1

if "%ACTION%"=="start" goto :start
if "%ACTION%"=="stop" goto :stop
if "%ACTION%"=="status" goto :status
echo Usage: run.bat [start^|stop^|status]
goto :end

:start
REM Check if already running
if exist "%PID_FILE%" (
    set /p OLD_PID=<"%PID_FILE%"
    tasklist /FI "PID eq !OLD_PID!" 2>nul | find "!OLD_PID!" >nul
    if not errorlevel 1 (
        echo Already running (PID: !OLD_PID!)
        goto :end
    )
)
REM Start in background (merge stdout/stderr to log)
pushd "%DIR%"
start "" /b python -u server.py > server.log 2>&1
popd
timeout /t 2 /nobreak >nul
REM Get python server.py PID
set "PID="
for /f %%i in ('powershell -NoProfile -ExecutionPolicy Bypass -File "%DIR%get_server_pid.ps1"') do set "PID=%%i"
if "!PID!"=="" (
    echo [FAIL] Start failed, check log: %LOG_FILE%
    goto :end
)
echo !PID!>"%PID_FILE%"
REM Get LAN IP
set "LAN_IP="
for /f "tokens=*" %%i in ('powershell -NoProfile -ExecutionPolicy Bypass -File "%DIR%get_lan_ip.ps1" 2^>nul') do set "LAN_IP=%%i"
echo [OK] Started (PID: !PID!)
if defined LAN_IP (echo [URL] http://!LAN_IP!:8000) else (echo [URL] http://<LAN_IP>:8000)
goto :end

:stop
if not exist "%PID_FILE%" (
    echo Not running
    goto :end
)
set /p PID=<"%PID_FILE%"
tasklist /FI "PID eq !PID!" 2>nul | find "!PID!" >nul
if not errorlevel 1 (
    taskkill /PID !PID! /F >nul
    echo [OK] Stopped
) else (
    echo Not running
)
del "%PID_FILE%" 2>nul
goto :end

:status
if not exist "%PID_FILE%" (
    echo [FAIL] Not running
    goto :end
)
set /p PID=<"%PID_FILE%"
tasklist /FI "PID eq !PID!" 2>nul | find "!PID!" >nul
if errorlevel 1 (
    echo [FAIL] Not running
    del "%PID_FILE%" 2>nul
    goto :end
)
echo [OK] Running (PID: !PID!)
set "LAN_IP="
for /f "tokens=*" %%i in ('powershell -NoProfile -ExecutionPolicy Bypass -File "%DIR%get_lan_ip.ps1" 2^>nul') do set "LAN_IP=%%i"
if defined LAN_IP (echo [URL] http://!LAN_IP!:8000) else (echo [URL] http://<LAN_IP>:8000)
goto :end

:end
endlocal
