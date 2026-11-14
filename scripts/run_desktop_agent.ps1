# ONUSLY Desktop Agent Console Runner
# Starts the background process monitor (Steam, Epic Games, Discord, games) and IPC server

$ErrorActionPreference = "Stop"

$workspaceRoot = Split-Path -Parent $PSScriptRoot
Set-Location $workspaceRoot

$binPath = "$workspaceRoot\build\bin\onusly-agent.exe"
if (-not (Test-Path $binPath)) {
    Write-Host "[BUILD] Compiling onusly-agent binary..." -ForegroundColor Cyan
    go build -o $binPath ./desktop/agent/cmd
}

$configDir = "$env:LOCALAPPDATA\Onusly\config"
$dataDir = "$env:LOCALAPPDATA\Onusly\data"
$pipePath = "\\.\pipe\OnuslyDesktopPipe"

New-Item -ItemType Directory -Force -Path $configDir, $dataDir | Out-Null

Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host "           ONUSLY Desktop Daemon Controller             " -ForegroundColor Cyan
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host " • Config: $configDir" -ForegroundColor White
Write-Host " • Data:   $dataDir" -ForegroundColor White
Write-Host " • IPC:    $pipePath" -ForegroundColor White
Write-Host " • Monitored Games/Apps: Steam, Epic Games, Discord, etc." -ForegroundColor Green
Write-Host ""
Write-Host "Starting agent daemon in console mode (Press Ctrl+C to stop)..." -ForegroundColor Yellow
Write-Host "---------------------------------------------------------" -ForegroundColor DarkGray

& $binPath run -config-dir $configDir -data-dir $dataDir -socket $pipePath -api-url "http://localhost:8080"

