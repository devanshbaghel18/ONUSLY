# =========================================================================
# ONUSLY: Complete System & Connection Reset (Make It Fresh)
# Requires Administrator elevation
# =========================================================================

$ErrorActionPreference = "Continue"

Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host "       ONUSLY: Complete Fresh System & ID Reset          " -ForegroundColor Cyan
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host ""

# 1. Stop background services and lingering processes
Write-Host "[1/6] Stopping ONUSLY services and background processes..." -ForegroundColor Yellow
Stop-Service -Name "OnuslyDesktopAgent" -Force -ErrorAction SilentlyContinue
taskkill /F /IM "onusly-agent.exe" /T 2>$null
taskkill /F /IM "onusly-native-host.exe" /T 2>$null
Write-Host "      Done." -ForegroundColor Green

# 2. Restore Network Adapter DNS (Disable loopback sinkhole)
Write-Host "[2/6] Restoring all network adapters to standard DHCP DNS..." -ForegroundColor Yellow
$adapters = Get-NetAdapter -ErrorAction SilentlyContinue
foreach ($ad in $adapters) {
    try {
        Set-DnsClientServerAddress -InterfaceIndex $ad.InterfaceIndex -ResetServerAddresses -ErrorAction SilentlyContinue
        netsh interface ipv4 set dns name="$($ad.Name)" dhcp 2>$null
        netsh interface ipv6 set dns name="$($ad.Name)" dhcp 2>$null
        Write-Host "      [-] Restored $($ad.Name)" -ForegroundColor Gray
    } catch {}
}
ipconfig /flushdns | Out-Null
Write-Host "      Flushed Windows DNS cache." -ForegroundColor Green

# 3. Remove Browser Enterprise Force-Lock Policies
Write-Host "[3/6] Removing Chrome & Edge enterprise force-install policies..." -ForegroundColor Yellow
Remove-Item -Path "HKLM:\Software\Policies\Google\Chrome\ExtensionInstallForcelist" -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -Path "HKLM:\Software\Policies\Microsoft\Edge\ExtensionInstallForcelist" -Recurse -Force -ErrorAction SilentlyContinue
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "OnuslyDesktopAgent" -ErrorAction SilentlyContinue
Write-Host "      Enterprise policies and auto-run entries removed." -ForegroundColor Green

# 4. Wipe Local Blocklist Cache, DPAPI Tokens & Old Pairing State
Write-Host "[4/6] Wiping old local task blocklists and security tokens..." -ForegroundColor Yellow
$programData = "C:\ProgramData\Onusly"

# Remove blocklist caches
Remove-Item -Path "$programData\data\blocklist_cache.json" -Force -ErrorAction SilentlyContinue
Remove-Item -Path "$programData\data\blocklist_cache.json.lkg" -Force -ErrorAction SilentlyContinue

# Clear secure DPAPI tokens
takeown /F "$programData\secure" /R /A 2>$null | Out-Null
icacls "$programData\secure" /grant Administrators:F /T 2>$null | Out-Null
Remove-Item -Path "$programData\secure\*" -Force -Recurse -ErrorAction SilentlyContinue

# Reset pairing_state.json to UNPAIRED
$unpairedState = @{
    status = "UNPAIRED"
    generation = 0
} | ConvertTo-Json -Depth 2
Set-Content -Path "$programData\config\pairing_state.json" -Value $unpairedState -Encoding UTF8 -Force

# Generate fresh Device ID
$newDeviceID = [guid]::NewGuid().ToString()
Set-Content -Path "$programData\config\device_id" -Value $newDeviceID -Encoding UTF8 -Force

# Reset dns_backup.json
$dnsBackupFile = "$programData\config\dns_backup.json"
if (Test-Path $dnsBackupFile) {
    try {
        $backup = Get-Content $dnsBackupFile -Raw | ConvertFrom-Json
        $backup.isActive = $false
        $backup | ConvertTo-Json -Depth 5 | Set-Content $dnsBackupFile -Encoding UTF8 -Force
    } catch {}
}
Write-Host "      Local caches wiped. Fresh Device ID generated: $newDeviceID" -ForegroundColor Green

# 5. Reset Desktop Service Startup
Write-Host "[5/6] Resetting OnuslyDesktopAgent service..." -ForegroundColor Yellow
Set-Service -Name "OnuslyDesktopAgent" -StartupType Manual -ErrorAction SilentlyContinue
Write-Host "      Service set to Manual (prevents background loop until freshly paired)." -ForegroundColor Green

# 6. Verification
Write-Host "[6/6] Verifying system status..." -ForegroundColor Yellow
$dnsCheck = Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object { $_.ServerAddresses -contains "127.0.0.1" }
if ($dnsCheck) {
    Write-Host "      [WARNING] 127.0.0.1 DNS still present on some adapters!" -ForegroundColor Red
} else {
    Write-Host "      [PASS] Standard upstream DNS restored across all adapters." -ForegroundColor Green
}

$policyCheck = Get-ItemProperty "HKLM:\Software\Policies\Google\Chrome\ExtensionInstallForcelist" -ErrorAction SilentlyContinue
if ($policyCheck) {
    Write-Host "      [WARNING] Chrome policy still present!" -ForegroundColor Red
} else {
    Write-Host "      [PASS] Chrome enterprise force-lock removed." -ForegroundColor Green
}

Write-Host ""
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host "      SYSTEM & CONNECTION RESET COMPLETED! FRESH READY   " -ForegroundColor Green
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host "Your Windows system is completely clean:" -ForegroundColor White
Write-Host " 1. All network DNS sinkholes removed (standard Internet active)" -ForegroundColor White
Write-Host " 2. Old local 'dsa' task & blocklist completely cleared" -ForegroundColor White
Write-Host " 3. Chrome/Edge extensions unlocked" -ForegroundColor White
Write-Host " 4. Device ID and pairing state reset to fresh UNPAIRED" -ForegroundColor White
Write-Host ""
Write-Host "To connect to your newly deployed ONUSLY instance:" -ForegroundColor Cyan
Write-Host " â€¢ Open your deployed Web App" -ForegroundColor White
Write-Host " â€¢ Go to Device Coverage / Reconnect" -ForegroundColor White
Write-Host " â€¢ Run the agent with your deployed API URL:" -ForegroundColor White
Write-Host "   & 'C:\Program Files\Onusly\onusly-agent.exe' run -api-url <YOUR_DEPLOYED_URL>" -ForegroundColor Yellow
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host ""
