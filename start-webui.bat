@echo off
setlocal EnableExtensions

cd /d "%~dp0"

set "BACKEND=%~dp0webui\backend"
set "VENV=%BACKEND%\.venv"
set "HOST=%SPROUTCLAW_CRON_WEB_HOST%"
set "PORT=%SPROUTCLAW_CRON_WEB_PORT%"

if not defined HOST set "HOST=0.0.0.0"
if not defined PORT set "PORT=8765"

if not exist "%VENV%\Scripts\python.exe" (
    echo [webui] creating venv...
    python -m venv "%VENV%"
    if errorlevel 1 (
        echo [webui] venv failed. Install Python and add it to PATH.
        exit /b 1
    )
    echo [webui] installing deps...
    "%VENV%\Scripts\python.exe" -m pip install -r "%BACKEND%\requirements.txt"
    if errorlevel 1 exit /b 1
)

set "URL=http://127.0.0.1:%PORT%/"
echo [webui] starting: %URL%
start "" "%URL%"

"%VENV%\Scripts\uvicorn.exe" main:app --app-dir "%BACKEND%" --host %HOST% --port %PORT% --reload --reload-dir "%BACKEND%" --reload-dir "%~dp0lib"

endlocal
