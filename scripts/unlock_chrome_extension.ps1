# ONUSLY Chrome Force-Lock Revert Controller
# Removes Chrome Enterprise Policy and unlocks the extension back to normal.
# Requires Administrator elevation

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "[ELEVATION REQUIRED] Relaunching PowerShell as Administrator..." -ForegroundColor Yellow
    Start-Process powershell -Verb RunAs -ArgumentList "-NoExit", "-File `"$PSCommandPath`""
    exit
}

$workspaceRoot = Split-Path -Parent $PSScriptRoot
Set-Location $workspaceRoot
$binPath = "$workspaceRoot\build\bin\onusly-agent.exe"
if (-not (Test-Path $binPath)) {
    go build -o $binPath ./desktop/agent/cmd
}

Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host "       ONUSLY Chrome Extension Revert Controller        " -ForegroundColor Cyan
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host ""

& $binPath policy remove

Write-Host ""
Write-Host "Verifying status..." -ForegroundColor DarkGray
& $binPath policy status

Write-Host ""
Write-Host "[INFO] Open chrome://extensions in Chrome. The extension is now unlocked and can be modified normally." -ForegroundColor Green
Read-Host "Press Enter to exit"
