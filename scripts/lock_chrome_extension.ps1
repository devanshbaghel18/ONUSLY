# ONUSLY Chrome Force-Lock Controller
# Applies Chrome Enterprise Policy to lock the extension so it cannot be disabled or removed.
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
Write-Host "       ONUSLY Chrome Extension Force-Lock Controller     " -ForegroundColor Cyan
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host ""

& $binPath policy apply

Write-Host ""
Write-Host "Verifying status..." -ForegroundColor DarkGray
& $binPath policy status

Write-Host ""
Write-Host "[INFO] Open chrome://extensions in Chrome. The toggle switch will be locked ON and the Remove button will be gone." -ForegroundColor Green
Read-Host "Press Enter to exit"
