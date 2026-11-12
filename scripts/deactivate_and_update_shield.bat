@echo off
:: =========================================================================
:: ONUSLY: Deactivate Machine DNS Shield & Update Background Service Binary
:: =========================================================================

:: Check for Administrator elevation
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [ONUSLY] Elevation required. Requesting Administrator permissions...
    powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process cmd -ArgumentList '/c \"\"%~f0\"\"' -Verb RunAs"
    exit /b
)

title ONUSLY Shield Manager

echo =========================================================================
echo  [ONUSLY] Deactivating DNS Shield & Updating Desktop Service...
echo =========================================================================
echo.

echo [Step 1/3] Restoring original network adapter DNS settings (Deactivating shield)...
powershell -NoProfile -ExecutionPolicy Bypass -Command "& 'C:\onusly\ONUSLY\build\bin\onusly-agent.exe' dns restore"
if %errorlevel% neq 0 (
    echo [Notice] Falling back to DHCP reset for physical adapters...
    powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-NetAdapter | Where-Object { $_.Status -eq 'Up' } | Set-DnsClientServerAddress -ResetServerAddresses"
)

echo.
echo [Step 2/3] Updating OnuslyDesktopAgent service binary in Program Files...
net stop OnuslyDesktopAgent >nul 2>&1
timeout /t 1 /nobreak >nul
copy /y "C:\onusly\ONUSLY\build\bin\onusly-agent.exe" "C:\Program Files\Onusly\onusly-agent.exe"
if %errorlevel% neq 0 (
    echo [ERROR] Failed to replace C:\Program Files\Onusly\onusly-agent.exe.
) else (
    echo [SUCCESS] Binary updated with IPC remote toggle support!
)

echo.
echo [Step 3/3] Restarting ONUSLY Desktop Enforcement Service...
net start OnuslyDesktopAgent
echo.
echo =========================================================================
echo  [DONE] Network DNS is restored, and the service is upgraded!
echo  You can now toggle the shield directly from http://localhost:5173
echo =========================================================================
echo.
pause

