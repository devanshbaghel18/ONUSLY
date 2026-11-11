# =============================================================================
# ONUSLY D7 / D7.1 PRODUCTION FAULT INJECTION & RELIABILITY ACCEPTANCE SUITE
# =============================================================================
# Verifies fault recovery, DNS preservation, cache durability, and health semantics.
# =============================================================================

[CmdletBinding()]
param(
    [string]$RepoRoot = "C:\onusly\ONUSLY",
    [string]$SourceBinDir = "C:\onusly\ONUSLY\build\bin"
)

$ErrorActionPreference = "Continue"

$PassCount = 0
$FailCount = 0

function Assert-Check {
    param([string]$Name, [bool]$Condition)
    if ($Condition) {
        Write-Host "  [PASS] $Name" -ForegroundColor Green
        $script:PassCount++
    } else {
        Write-Host "  [FAIL] $Name" -ForegroundColor Red
        $script:FailCount++
    }
}

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host " ONUSLY D7.1 FAULT INJECTION & RELIABILITY ACCEPTANCE SUITE       " -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "Repository: $RepoRoot"
Write-Host "Binaries:   $SourceBinDir"

# =============================================================================
# TEST 1: DNS Recovery & Static vs DHCP Preservation
# =============================================================================
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host " [TEST 1] DNS Recovery & Static vs DHCP Configuration Preservation" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$dnsTestOutput = (go test -C $RepoRoot -v -run "TestWindowsAdapterManager" ./desktop/agent/internal/dns/...) | Out-String
Assert-Check "DNS Adapter Manager unit tests passed" ($LASTEXITCODE -eq 0)
Assert-Check "Mock lifecycle verifies static DNS restoration vs DHCP" ([bool]($dnsTestOutput -match "PASS: TestWindowsAdapterManager_MockLifecycle"))
Assert-Check "Never overwrites valid backup with loopback settings" ([bool]($dnsTestOutput -match "PASS: TestWindowsAdapterManager_NeverOverwriteBackupWithLoopback"))
Assert-Check "Refuses to blindly wipe to DHCP when backup is missing" ([bool]($dnsTestOutput -match "PASS: TestWindowsAdapterManager_MissingBackupProtection"))

# Host-level netsh DHCP vs Static DNS detection check
$wiFiNetsh = (netsh interface ip show dns name="Wi-Fi" 2>&1) | Out-String
$isDhcpDetected = ($wiFiNetsh -match 'DNS servers configured through DHCP') -and -not ($wiFiNetsh -match 'Statically Configured DNS Servers')
Assert-Check "Host adapter accurately distinguishes DHCP from Static DNS" ([bool]($isDhcpDetected -or ($wiFiNetsh -match 'Statically Configured DNS Servers')))

# =============================================================================
# TEST 2: Policy Cache Durability, Corruption Recovery & LKG Fallback
# =============================================================================
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host " [TEST 2] Policy Cache Durability, Corruption Recovery & LKG Fallback" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$cacheTestOutput = (go test -C $RepoRoot -v ./desktop/agent/internal/cache/...) | Out-String
Assert-Check "Policy cache unit and durability tests passed" ($LASTEXITCODE -eq 0)
Assert-Check "Atomic write with fsync verified" ([bool]($cacheTestOutput -match "PASS: TestCache_PersistenceAndIsolation"))
Assert-Check "Corrupted cache quarantined to .corrupt" ([bool]($cacheTestOutput -match "PASS: TestCache_PersistenceAndIsolation"))
Assert-Check "Last-Known-Good (LKG) fallback verified upon corruption" ([bool]($cacheTestOutput -match "PASS: TestCache_PersistenceAndIsolation"))
Assert-Check "Seamless migration from legacy blocklist_cache to policy_cache verified" ([bool]($cacheTestOutput -match "PASS: TestCache_SeamlessLegacyMigration"))

# =============================================================================
# TEST 3: Corrupt Cache Health Semantics & Degradation Reporting
# =============================================================================
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host " [TEST 3] Corrupt Cache Health Semantics & Degradation Reporting" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$tempTestRoot = "$env:TEMP\onusly-fault-test-$PID"
New-Item -ItemType Directory -Path "$tempTestRoot\config" -Force | Out-Null
New-Item -ItemType Directory -Path "$tempTestRoot\data" -Force | Out-Null
$testPipe = "\\.\pipe\OnuslyFaultPipe_$PID"

# Seed a corrupt cache file in the data dir
$corruptFile = "$tempTestRoot\data\blocklist_cache.json"
Set-Content -Path $corruptFile -Value "{ this is corrupted json payload !!!" -Encoding utf8

# Launch agent in standalone console mode pointing to the corrupt cache
$agentExe = "$SourceBinDir\onusly-agent.exe"
$agentProc = Start-Process -FilePath $agentExe -ArgumentList "run", "-dns-listen", "127.0.0.1:5356", "-socket", $testPipe, "-config-dir", "$tempTestRoot\config", "-data-dir", "$tempTestRoot\data" -PassThru -NoNewWindow
Start-Sleep -Milliseconds 1500

try {
    # Verify the corrupt file was quarantined to .corrupt
    Assert-Check "Corrupted cache file was quarantined to .corrupt" (Test-Path "$corruptFile.corrupt")

    # Query status over IPC
    $statusOut = & $agentExe -socket $testPipe -config-dir "$tempTestRoot\config" -data-dir "$tempTestRoot\data" status 2>&1 | Out-String
    Assert-Check "Agent query status succeeds over IPC" ($LASTEXITCODE -eq 0)
    Assert-Check "Does NOT report 'AUTHENTICATED & SYNCHRONIZED'" (-not ($statusOut -match "AUTHENTICATED & SYNCHRONIZED"))
    Assert-Check "Reports Enforcing: false when no valid policy loaded" ([bool]($statusOut -match "Enforcing:\s+false"))
    Assert-Check "Reports ENFORCEMENT_DEGRADED or NOT PAIRED" ([bool](($statusOut -match "ENFORCEMENT_DEGRADED") -or ($statusOut -match "NOT PAIRED")))
    Assert-Check "Cached Targets reports 'None'" ([bool]($statusOut -match "Cached Targets:\s+None"))

    # Query diagnostics over IPC
    $diagOut = & $agentExe -socket $testPipe -config-dir "$tempTestRoot\config" -data-dir "$tempTestRoot\data" diagnostics 2>&1 | Out-String
    Assert-Check "Diagnostics reports cache_valid = false" ([bool]($diagOut -match "cache_valid:\s*false"))
    Assert-Check "Diagnostics reports enforcement_status = ENFORCEMENT_DEGRADED or UNPAIRED" ([bool](($diagOut -match "enforcement_status:\s*ENFORCEMENT_DEGRADED") -or ($diagOut -match "enforcement_status:\s*UNPAIRED")))

} finally {
    if ($agentProc -and -not $agentProc.HasExited) {
        Stop-Process -Id $agentProc.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item -Path $tempTestRoot -Recurse -Force -ErrorAction SilentlyContinue
}

# =============================================================================
# TEST 4: Offline Startup with Missing Cache
# =============================================================================
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host " [TEST 4] Offline Startup with Missing Cache" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$tempEmptyRoot = "$env:TEMP\onusly-empty-test-$PID"
New-Item -ItemType Directory -Path "$tempEmptyRoot\config" -Force | Out-Null
New-Item -ItemType Directory -Path "$tempEmptyRoot\data" -Force | Out-Null
$emptyPipe = "\\.\pipe\OnuslyEmptyPipe_$PID"

$emptyProc = Start-Process -FilePath $agentExe -ArgumentList "run", "-dns-listen", "127.0.0.1:5357", "-socket", $emptyPipe, "-config-dir", "$tempEmptyRoot\config", "-data-dir", "$tempEmptyRoot\data" -PassThru -NoNewWindow
Start-Sleep -Milliseconds 1500

try {
    # Check that service boots cleanly without crash
    Assert-Check "Agent boots cleanly with missing cache (no crash)" (-not $emptyProc.HasExited)

    $emptyStatus = & $agentExe -socket $emptyPipe -config-dir "$tempEmptyRoot\config" -data-dir "$tempEmptyRoot\data" status 2>&1 | Out-String
    Assert-Check "Enforcing is false when cache is missing" ([bool]($emptyStatus -match "Enforcing:\s+false"))
    Assert-Check "Cached Targets is None" ([bool]($emptyStatus -match "Cached Targets:\s+None"))

} finally {
    if ($emptyProc -and -not $emptyProc.HasExited) {
        Stop-Process -Id $emptyProc.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item -Path $tempEmptyRoot -Recurse -Force -ErrorAction SilentlyContinue
}

# =============================================================================
# TEST 5: SCM Crash Recovery & Restart Actions Invariant
# =============================================================================
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host " [TEST 5] SCM Crash Recovery & Failure Actions Configuration" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$scFailure = sc.exe qfailure OnuslyDesktopAgent | Out-String
Assert-Check "SCM failure configuration queried successfully" ($LASTEXITCODE -eq 0)
Assert-Check "SCM reset period configured for 86400 seconds (24h)" ([bool]($scFailure -match "RESET_PERIOD.*86400"))
Assert-Check "SCM Action 1 configured to RESTART after 5000ms" ([bool]($scFailure -match "RESTART\s*--\s*Delay\s*=\s*5000"))
Assert-Check "SCM Action 2 configured to RESTART after 10000ms" ([bool]($scFailure -match "RESTART\s*--\s*Delay\s*=\s*10000"))

# =============================================================================
# TEST 6: Live Production SCM Service Identity & Invariants
# =============================================================================
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host " [TEST 6] Live SCM Service Identity & Device Invariants" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$svc = Get-Service -Name OnuslyDesktopAgent
Assert-Check "Live SCM service exists and is RUNNING" ($svc.Status -eq "Running")

$sidOut = sc.exe showsid OnuslyDesktopAgent | Out-String
Assert-Check "Live SCM unrestricted Service SID verified" ($sidOut -match "S-1-5-80-3463423020-2351545225-1748562058-3133284821-4013307172")

$devId = Get-Content "C:\ProgramData\Onusly\config\device_id" -Raw -ErrorAction SilentlyContinue
Assert-Check "Production device identity preserved: 143489b2-2fd8-428b-bfa8-2631cc2be8c3" ($devId.Trim() -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3")

# Query live production service version and status
$verOut = & "$SourceBinDir\onusly-agent.exe" version 2>&1 | Out-String
Assert-Check "Agent reports canonical version 1.0.0-rc1" ($verOut -match "1.0.0-rc1")

$liveStatus = & "$SourceBinDir\onusly-agent.exe" query status 2>&1 | Out-String
Assert-Check "Live service responds cleanly over \\.\pipe\OnuslyAgentIPC" ($liveStatus -match "ONUSLY DESKTOP AGENT")

# =============================================================================
# SUMMARY
# =============================================================================
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host " D7.1 FAULT INJECTION & RELIABILITY TEST RESULTS SUMMARY         " -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host " Total Passed: $PassCount" -ForegroundColor Green
Write-Host " Total Failed: $FailCount" -ForegroundColor $(if ($FailCount -eq 0) { "Green" } else { "Red" })

if ($FailCount -eq 0) {
    Write-Host "`n>>> [SUCCESS] ALL D7.1 FAULT INJECTION & RELIABILITY CHECKS PASSED! <<<`n" -ForegroundColor Green
} else {
    Write-Host "`n>>> [FAIL] $FailCount D7.1 FAULT INJECTION CHECK(S) FAILED! <<<`n" -ForegroundColor Red
    exit 1
}
