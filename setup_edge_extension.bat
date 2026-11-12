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
echo 1. Opening Microsoft Edge Extensions page (edge://extensions)...
start msedge edge://extensions
echo 2. Opening the ONUSLY extension directory in File Explorer...
explorer "C:\onusly\ONUSLY\extension"
echo.
echo =========================================================================
echo Quick 10-Second Setup:
echo   1. In Microsoft Edge, turn ON "Developer mode" in the left sidebar.
echo   2. Click the "Load unpacked" button at the top.
echo   3. Select the extension folder opened in File Explorer (C:\onusly\ONUSLY\extension).
echo   4. Go to http://localhost:5173/dashboard in Edge and click "Connect Extension".
echo =========================================================================
echo.
pause

