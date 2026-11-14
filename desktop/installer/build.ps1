# ==============================================================================
# ONUSLY Windows Installer Build Script
# Compiles onusly-agent and onusly-native-host into embedded installer assets
# ==============================================================================

$root = (Resolve-Path "$PSScriptRoot\..\..").Path
$assetsDir = "$PSScriptRoot\cmd\installer\assets"

Write-Host ">>> Creating assets directory: $assetsDir"
New-Item -ItemType Directory -Force -Path $assetsDir | Out-Null

Push-Location $root
try {
    Write-Host ">>> Building onusly-agent.exe..."
    go build -p 2 -o "$assetsDir\onusly-agent.exe" ./desktop/agent/cmd
    if ($LASTEXITCODE -ne 0) { throw "Failed to build onusly-agent.exe" }

    Write-Host ">>> Building onusly-native-host.exe..."
    go build -p 2 -o "$assetsDir\onusly-native-host.exe" ./desktop/native-host
    if ($LASTEXITCODE -ne 0) { throw "Failed to build onusly-native-host.exe" }

    Write-Host ">>> Building self-contained onusly-setup.exe installer..."
    go build -p 2 -o "$root\onusly-setup.exe" ./desktop/installer/cmd/installer
    if ($LASTEXITCODE -ne 0) { throw "Failed to build onusly-setup.exe" }

    Write-Host ">>> [SUCCESS] Built onusly-setup.exe successfully!"
} finally {
    Pop-Location
}
