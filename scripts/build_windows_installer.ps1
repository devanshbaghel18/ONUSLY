# ONUSLY Windows 1-Click Installer Build Script
# Packages onusly-agent, native-host, and pairing GUI into a single standalone OnuslySetup.exe

$ErrorActionPreference = "Stop"

$workspaceRoot = Split-Path -Parent $PSScriptRoot
Set-Location $workspaceRoot

$binDir = "$workspaceRoot\build\bin"
$assetsDir = "$workspaceRoot\desktop\installer\cmd\installer\assets"
$downloadsDir = "$workspaceRoot\frontend\public\downloads"

New-Item -ItemType Directory -Force -Path $binDir, $assetsDir, $downloadsDir | Out-Null

Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host "       ONUSLY Windows Standalone Installer Builder       " -ForegroundColor Cyan
Write-Host "=========================================================" -ForegroundColor Cyan

# 1. Compile core agent
Write-Host "[1/4] Compiling onusly-agent.exe..." -ForegroundColor Yellow
go build -ldflags "-s -w" -o "$binDir\onusly-agent.exe" ./desktop/agent/cmd
if ($LASTEXITCODE -ne 0) { throw "Failed to build onusly-agent.exe" }

# 2. Compile native messaging host
Write-Host "[2/4] Compiling onusly-native-host.exe..." -ForegroundColor Yellow
go build -ldflags "-s -w" -o "$binDir\onusly-native-host.exe" ./desktop/native-host
if ($LASTEXITCODE -ne 0) { throw "Failed to build onusly-native-host.exe" }

# 3. Stage embedded assets
Write-Host "[3/4] Staging embedded installer assets..." -ForegroundColor Yellow
Copy-Item "$binDir\onusly-agent.exe", "$binDir\onusly-native-host.exe" $assetsDir -Force

# 4. Compile OnuslySetup.exe (Pure Windows GUI subsystem, no command prompt)
Write-Host "[4/4] Compiling standalone OnuslySetup.exe..." -ForegroundColor Yellow
go build -ldflags "-H=windowsgui -s -w" -o "$binDir\OnuslySetup.exe" ./desktop/installer/cmd/installer
if ($LASTEXITCODE -ne 0) { throw "Failed to build OnuslySetup.exe" }

# Copy to frontend public downloads
Copy-Item "$binDir\OnuslySetup.exe" "$downloadsDir\OnuslySetup.exe" -Force

$fileSizeMB = [math]::Round(((Get-Item "$binDir\OnuslySetup.exe").Length / 1MB), 2)

Write-Host ""
Write-Host "=========================================================" -ForegroundColor Green
Write-Host " [SUCCESS] Standalone Installer built successfully!      " -ForegroundColor Green
Write-Host " Output:   $binDir\OnuslySetup.exe ($fileSizeMB MB)" -ForegroundColor White
Write-Host " Web URL:  /downloads/OnuslySetup.exe" -ForegroundColor White
Write-Host "=========================================================" -ForegroundColor Green

