@echo off
title ONUSLY - Setup Browser Extension in Microsoft Edge
echo =========================================================================
echo   ONUSLY - Enable YouTube Shorts Blocking in Microsoft Edge
echo =========================================================================
echo.
echo Machine-wide DNS blocks domains (Instagram, Twitter, etc.) on all browsers.
echo To block YouTube Shorts while keeping regular YouTube accessible,
echo Microsoft Edge needs the ONUSLY browser extension loaded.
echo.
set "EXT_DIR=%~dp0extension"
if not exist "%EXT_DIR%\manifest.json" set "EXT_DIR=%ProgramFiles%\Onusly\extension"
if not exist "%EXT_DIR%\manifest.json" set "EXT_DIR=%LOCALAPPDATA%\Onusly\extension"
echo 1. Opening Microsoft Edge Extensions page (edge://extensions)...
start msedge edge://extensions
echo 2. Opening the ONUSLY extension directory in File Explorer...
if exist "%EXT_DIR%" (
  explorer "%EXT_DIR%"
) else (
  echo [!] Extension folder not found locally. Download onusly-extension.zip from your ONUSLY Dashboard.
)
echo.
echo =========================================================================
echo Quick 10-Second Setup:
echo   1. In Microsoft Edge, turn ON "Developer mode" in the left sidebar.
echo   2. Click the "Load unpacked" button at the top.
echo   3. Select the extracted extension folder (%EXT_DIR%).
echo   4. Go to https://onusly.duckdns.org/dashboard (or https://onusly.app/dashboard) in Edge and click "Connect Extension".
echo =========================================================================
echo.
pause
