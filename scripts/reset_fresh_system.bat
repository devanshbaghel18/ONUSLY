@echo off
:: =========================================================================
:: ONUSLY: Complete System & Connection Reset (Make It Fresh)
:: Automatically requests Administrator privileges if not elevated
:: =========================================================================

net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [ONUSLY] Administrator elevation required. Requesting permissions...
    powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process cmd -ArgumentList '/c \"\"%~f0\"\"' -Verb RunAs"
    exit /b
)

title ONUSLY Fresh Reset Controller
cls

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0reset_fresh_system.ps1"

echo.
pause
