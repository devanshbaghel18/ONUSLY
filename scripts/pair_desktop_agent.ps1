# ONUSLY Desktop Agent Pairing Helper
# Connects to the running desktop daemon and requests a 6-character pairing code

$ErrorActionPreference = "Stop"

$workspaceRoot = Split-Path -Parent $PSScriptRoot
Set-Location $workspaceRoot

$binPath = "$workspaceRoot\build\bin\onusly-agent.exe"
$configDir = "$env:LOCALAPPDATA\Onusly\config"
$dataDir = "$env:LOCALAPPDATA\Onusly\data"
$pipePath = "\\.\pipe\OnuslyDesktopPipe"

& $binPath pair -socket $pipePath -config-dir $configDir -data-dir $dataDir -api-url "http://localhost:8080"

