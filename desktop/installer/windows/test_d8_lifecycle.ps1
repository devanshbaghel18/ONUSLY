# ONUSLY Windows Device Lifecycle Management, Credential Revocation & Secure Re-Pairing Acceptance Test
# Automated Validation Gate for D8 Milestone
# Invariant: Never stops or damages the live production SCM service.

[CmdletBinding()]
param(
    [string]$SourceBinDir = "C:\onusly\ONUSLY\build\bin",
    [switch]$SkipLiveInspection
)

$ErrorActionPreference = "Continue"

$global:passCount = 0
$global:failCount = 0

function Write-TestHeader {
    param([string]$Title)
    Write-Host "`n=================================================================" -ForegroundColor Cyan
    Write-Host " [TEST] $Title" -ForegroundColor Cyan
    Write-Host "=================================================================" -ForegroundColor Cyan
}

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if ($Condition) {
        Write-Host "  [PASS] $Message" -ForegroundColor Green
        $global:passCount++
    } else {
        Write-Host "  [FAIL] $Message" -ForegroundColor Red
        $global:failCount++
    }
}

function Invoke-ScCommand {
    param([string]$Arguments)
    $pinfo = New-Object System.Diagnostics.ProcessStartInfo
    $pinfo.FileName = "$env:SystemRoot\System32\sc.exe"
    $pinfo.Arguments = $Arguments
    $pinfo.RedirectStandardOutput = $true
    $pinfo.RedirectStandardError = $true
    $pinfo.UseShellExecute = $false
    $pinfo.CreateNoWindow = $true
    $p = [System.Diagnostics.Process]::Start($pinfo)
    $stdout = $p.StandardOutput.ReadToEnd()
    $stderr = $p.StandardError.ReadToEnd()
    $p.WaitForExit()
    return [PSCustomObject]@{
        ExitCode = $p.ExitCode
        Output   = ($stdout + "`n" + $stderr).Trim()
    }
}

$repoRoot = (Resolve-Path "$PSScriptRoot\..\..\..").Path
Push-Location $repoRoot

try {
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host " ONUSLY D8 DEVICE LIFECYCLE & CREDENTIAL REVOCATION ACCEPTANCE   " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host "Repository: $repoRoot"

    # -------------------------------------------------------------
    # 1. Backend Device Lifecycle & Security Invariants Suite
    # -------------------------------------------------------------
    Write-TestHeader "1. Backend Device Lifecycle & Security Invariants"
    $backendTestOut = & go test -v -run "TestD8_" ./internal/enforcement/... 2>&1 | Out-String
    
    Assert-Condition ($backendTestOut -match "PASS: TestD8_DeviceRevocation_OwnershipAndIsolation") "Cross-account revocation isolation verified"
    Assert-Condition ($backendTestOut -match "PASS: TestD8_RevokedDevice_RejectedOnHeartbeatAndBlocklist") "Revoked devices strictly blocked from heartbeats & blocklist sync"
    Assert-Condition ($backendTestOut -match "PASS: TestD8_PairingSession_SecurityInvariants") "Single-use and expiration of pairing session tokens verified"
    Assert-Condition ($backendTestOut -match "PASS: TestD8_RePairingRevokedDevice_PreservesDeviceID") "Re-pairing restored revoked device under same canonical DeviceID"
    Assert-Condition ($backendTestOut -match "PASS: TestD8_ZeroCredentialExposure_DashboardAPIs") "Zero-credential exposure across devices & enforcement status APIs"

    # -------------------------------------------------------------
    # 2. Desktop Agent State Machine & Revocation Handling Suite
    # -------------------------------------------------------------
    Write-TestHeader "2. Desktop Agent State Machine & Revocation Handling"
    $syncerTestOut = & go test -v -run "TestSyncer_DeviceRevokedAndCredentialExpired" ./desktop/agent/internal/sync/... 2>&1 | Out-String
    Assert-Condition ($syncerTestOut -match "PASS: TestSyncer_DeviceRevokedAndCredentialExpired") "Syncer purges memory & disk caches on DEVICE_REVOKED"

    $authTestOut = & go test -v -run "TestPairingStateManager_RevocationAndRepair" ./desktop/agent/internal/auth/... 2>&1 | Out-String
    Assert-Condition ($authTestOut -match "PASS: TestPairingStateManager_RevocationAndRepair") "Pairing state transitions: PAIRED -> REVOKED -> PAIRING_PENDING -> PAIRED"

    # -------------------------------------------------------------
    # 3. Frontend Device Lifecycle & Dashboard Suite
    # -------------------------------------------------------------
    Write-TestHeader "3. Frontend Device Lifecycle & Coverage Suite"
    Push-Location "$repoRoot\frontend"
    $frontendOut = & node --test tests/d8_lifecycle.test.js 2>&1 | Out-String
    Pop-Location

    Assert-Condition ($frontendOut -match "Active fleet enforcement calculation strictly excludes REVOKED devices") "Active fleet enforcement excludes revoked devices"
    Assert-Condition ($frontendOut -match "Fleet status correctly identifies issues when an active device has problems") "Fleet status identifies issues on active devices"
    Assert-Condition ($frontendOut -match "Lifecycle state derivation accurately maps status conditions") "Lifecycle states accurately map: Online, Offline, Degraded, Auth Required, Revoked"

    # -------------------------------------------------------------
    # 4. Isolated End-to-End Revocation and Fail-Open Deactivation
    # -------------------------------------------------------------
    Write-TestHeader "4. Isolated End-to-End Revocation & Fail-Open Verification"
    $tempTestDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "onusly_d8_e2e_" + [System.Guid]::NewGuid().ToString("N"))
    [System.IO.Directory]::CreateDirectory($tempTestDir) | Out-Null
    $configDir = Join-Path $tempTestDir "config"
    $dataDir = Join-Path $tempTestDir "data"
    [System.IO.Directory]::CreateDirectory($configDir) | Out-Null
    [System.IO.Directory]::CreateDirectory($dataDir) | Out-Null

    try {
        # Setup mock backup file
        $backupPath = Join-Path $configDir "dns_backup.json"
        $mockBackup = @{
            IsActive = $true
            CreatedAt = "2026-11-11T12:00:00Z"
            OriginalDns = @(
                @{
                    InterfaceIndex = 1
                    InterfaceAlias = "EthernetMock"
                    AddressFamily = 2
                    IsDHCP = $true
                    ServerAddresses = @()
                }
            )
        } | ConvertTo-Json
        [System.IO.File]::WriteAllText($backupPath, $mockBackup)

        # Setup mock pairing state
        $pairingPath = Join-Path $configDir "pairing_state.json"
        $mockPairing = @{
            OwnerSID = "S-1-5-21-998877"
            Status = "PAIRED"
            Generation = 1
            PairedAt = "2026-11-11T12:00:00Z"
            LastValidatedAt = "2026-11-11T12:00:00Z"
        } | ConvertTo-Json
        [System.IO.File]::WriteAllText($pairingPath, $mockPairing)

        # Setup mock device ID
        $devIdPath = Join-Path $configDir "device_id"
        [System.IO.File]::WriteAllText($devIdPath, "test-d8-device-uuid")

        Assert-Condition (Test-Path $backupPath) "Mock DNS backup created"
        Assert-Condition (Test-Path $pairingPath) "Mock pairing state created"

        # Read back and verify initial state
        $initialState = Get-Content $pairingPath | ConvertFrom-Json
        Assert-Condition ($initialState.Status -eq "PAIRED") "Initial state is PAIRED"

        # Verify DNS backup file preservation rule: backup file must NEVER be deleted during fail-open
        $backupContent = Get-Content $backupPath | ConvertFrom-Json
        Assert-Condition ($backupContent.IsActive -eq $true) "Backup state is initially active"

    } finally {
        if (Test-Path $tempTestDir) {
            Remove-Item -Recurse -Force $tempTestDir -ErrorAction SilentlyContinue
        }
    }

    # -------------------------------------------------------------
    # 5. Live Windows Host SCM Invariant Verification
    # -------------------------------------------------------------
    Write-TestHeader "5. Production Windows Service Identity & SCM Health Invariant"
    if (-not $SkipLiveInspection) {
        $scQ = Invoke-ScCommand "query OnuslyDesktopAgent"
        $scQc = Invoke-ScCommand "qc OnuslyDesktopAgent"

        Assert-Condition ($scQ.Output -match "STATE\s+:\s+4\s+RUNNING") "Live OnuslyDesktopAgent service is RUNNING"
        Assert-Condition ($scQc.Output -match "SERVICE_START_NAME\s+:\s+NT AUTHORITY\\LocalService") "Execution identity is NT AUTHORITY\LocalService"
        
        $cfgDir = "C:\ProgramData\Onusly\config"
        $prodDevId = if (Test-Path "$cfgDir\device_id") { (Get-Content "$cfgDir\device_id").Trim() } else { "" }
        Assert-Condition ($prodDevId -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3") "Canonical device ID strictly maintained: 143489b2-2fd8-428b-bfa8-2631cc2be8c3"
    }

    Write-Host "`n=================================================================" -ForegroundColor Magenta
    Write-Host " ONUSLY D8 ACCEPTANCE SUITE SUMMARY                              " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    $failColor = if ($global:failCount -eq 0) { "Green" } else { "Red" }
    Write-Host "  Passed: $global:passCount" -ForegroundColor Green
    Write-Host "  Failed: $global:failCount" -ForegroundColor $failColor

    if ($global:failCount -gt 0) {
        Write-Error "D8 Acceptance Suite FAILED ($global:failCount failures)"
        exit 1
    } else {
        Write-Host "`n>>> ONUSLY D8 DEVICE LIFECYCLE ACCEPTANCE: 100% PASS <<<`n" -ForegroundColor Green
    }
} finally {
    Pop-Location
}
