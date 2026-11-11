# ONUSLY Windows Device Pairing, Credential Lifecycle & Session Resilience Acceptance Test
# Automated Validation Gate for D3.4 Milestone
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
    Write-Host " ONUSLY D3.4 WINDOWS DEVICE PAIRING & CREDENTIAL ACCEPTANCE      " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host "Repository: $repoRoot"
    Write-Host "Binaries:   $SourceBinDir"

    # -------------------------------------------------------------
    # 1. Binary Integrity and Prerequisites
    # -------------------------------------------------------------
    Write-TestHeader "1. Binary Integrity and Prerequisites"
    $agentExe = "$SourceBinDir\onusly-agent.exe"
    $nativeHostExe = "$SourceBinDir\onusly-native-host.exe"

    Assert-Condition (Test-Path $agentExe) "Agent executable exists at $agentExe"
    Assert-Condition (Test-Path $nativeHostExe) "Native host executable exists at $nativeHostExe"

    if (Test-Path $agentExe) {
        $bytes = [IO.File]::ReadAllBytes($agentExe)
        $isPE = ($bytes.Length -gt 64 -and $bytes[0] -eq 0x4D -and $bytes[1] -eq 0x5A)
        Assert-Condition $isPE "onusly-agent.exe has valid MZ header (Windows PE)"
    }
    if (Test-Path $nativeHostExe) {
        $bytes = [IO.File]::ReadAllBytes($nativeHostExe)
        $isPE = ($bytes.Length -gt 64 -and $bytes[0] -eq 0x4D -and $bytes[1] -eq 0x5A)
        Assert-Condition $isPE "onusly-native-host.exe has valid MZ header (Windows PE)"
    }

    # -------------------------------------------------------------
    # 2. DPAPI Credential Store Hardening Tests
    # -------------------------------------------------------------
    Write-TestHeader "2. DPAPI Machine Scope Credential Store Hardening"
    $dpapiTest = & go test -v -run "TestDPAPICredentialStore_" ./desktop/agent/internal/auth/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "DPAPI unit tests passed (Save/Get/Overwrite/Clear/Empty/Corrupt/Bak)"
    Assert-Condition ($dpapiTest -match "PASS: TestDPAPICredentialStore_SaveAndGet") "DPAPI round-trip encryption verified"
    Assert-Condition ($dpapiTest -match "PASS: TestDPAPICredentialStore_EmptyTokenRejection") "Empty token rejection verified"
    Assert-Condition ($dpapiTest -match "PASS: TestDPAPICredentialStore_CorruptDataRecovery") "Corrupt data recovery verified"

    # -------------------------------------------------------------
    # 3. Pairing State Lifecycle Transitions
    # -------------------------------------------------------------
    Write-TestHeader "3. Pairing State Lifecycle Transitions"
    $stateTest = & go test -v -run "TestPairingStateManager_" ./desktop/agent/internal/auth/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Pairing state manager tests passed"
    Assert-Condition ($stateTest -match "PASS: TestPairingStateManager_LifecycleTransitions") "State transitions (UNPAIRED -> PENDING -> COMMIT -> PAIRED -> AUTH_REQ) verified"
    Assert-Condition ($stateTest -match "PASS: TestPairingStateManager_RecoveryInterruptedCommit") "Interrupted commit recovery logic verified"

    # -------------------------------------------------------------
    # 4. Windows Named-Pipe Security & IPC Pairing Protocols
    # -------------------------------------------------------------
    Write-TestHeader "4. Windows Named-Pipe Security & IPC Pairing Protocols"
    $pipeTest = & go test -v -run "TestWindowsNamedPipe_" ./desktop/agent/internal/ipc/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Windows named pipe pairing and security tests passed"
    Assert-Condition ($pipeTest -match "PASS: TestWindowsNamedPipe_AuthorizedPairedSID") "Authorized paired SID access verified"
    Assert-Condition ($pipeTest -match "PASS: TestWindowsNamedPipe_UnauthorizedWrongSID") "Unauthorized foreign SID rejection verified"
    Assert-Condition ($pipeTest -match "PASS: TestWindowsNamedPipe_WrongSessionRejection") "Non-console session rejection verified"
    Assert-Condition ($pipeTest -match "PASS: TestWindowsNamedPipe_GuaranteedRevertToSelf") "Guaranteed RevertToSelf on return paths verified"
    Assert-Condition ($pipeTest -match "PASS: TestWindowsNamedPipe_PairingFlow") "IPC two-phase pairing flow over named pipe verified"
    Assert-Condition ($pipeTest -match "PASS: TestWindowsNamedPipe_PairCancel") "Pair cancellation rollback verified"

    # -------------------------------------------------------------
    # 5. Failure & Recovery Matrix
    # -------------------------------------------------------------
    Write-TestHeader "5. Failure & Recovery Matrix"
    $failTest = & go test -v -run "TestWindowsFailure_" ./desktop/agent/internal/ipc/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Failure and recovery matrix tests passed"
    Assert-Condition ($failTest -match "PASS: TestWindowsFailure_InterruptedCommitRecovery") "Interrupted two-phase commit auto-recovery passed"
    Assert-Condition ($failTest -match "PASS: TestWindowsFailure_OfflineResilience") "CRITICAL INVARIANT: Offline resilience preserves cached restrictions"
    Assert-Condition ($failTest -match "PASS: TestWindowsFailure_ETag304NotModified") "ETag and 304 Not Modified cache freshness passed"
    Assert-Condition ($failTest -match "PASS: TestWindowsFailure_TokenExpirationHandling") "Token expiration detection and AUTHENTICATION_REQUIRED transition passed"

    # -------------------------------------------------------------
    # 6. Complete End-to-End Acceptance Gate
    # -------------------------------------------------------------
    Write-TestHeader "6. Complete End-to-End Acceptance Gate"
    $e2eTest = & go test -v -run "TestWindows_EndToEndAcceptance" ./desktop/agent/internal/ipc/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "End-to-end acceptance test passed"
    Assert-Condition ($e2eTest -match "PASS: TestWindows_EndToEndAcceptance") "Full lifecycle (DAEMON -> PAIR -> CONFIRM -> SYNC -> ENFORCE -> NATIVE_HOST -> RESTART) verified"

    # -------------------------------------------------------------
    # 7. Live Production Service Invariant Verification
    # -------------------------------------------------------------
    if (-not $SkipLiveInspection) {
        Write-TestHeader "7. Live Production Service Invariants"
        $liveQex = Invoke-ScCommand "queryex OnuslyDesktopAgent"
        Assert-Condition ($liveQex.ExitCode -eq 0) "Live service OnuslyDesktopAgent exists in SCM"
        Assert-Condition ($liveQex.Output -match "STATE\s*:\s*4\s+RUNNING") "Live service is in RUNNING state"

        $sidQ = Invoke-ScCommand "showsid OnuslyDesktopAgent"
        Assert-Condition ($sidQ.Output -match "S-1-5-80-3463423020-2351545225-1748562058-3133284821-4013307172") "Live service SID matches canonical unrestricted SID"

        $prodDeviceIdPath = "C:\ProgramData\Onusly\config\device_id"
        Assert-Condition (Test-Path $prodDeviceIdPath) "Production device_id file exists"
        if (Test-Path $prodDeviceIdPath) {
            $devId = (Get-Content $prodDeviceIdPath).Trim()
            Assert-Condition ($devId -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3") "Production device identity is preserved: $devId"
        }

        # Live service pipe status check
        $liveStatusOut = & $agentExe status 2>&1 | Out-String
        Assert-Condition ($LASTEXITCODE -eq 0) "Live agent query status exited with 0"
        Assert-Condition ($liveStatusOut -match "ONUSLY DESKTOP AGENT \(Live Service\)") "Connected to live service over \\.\pipe\OnuslyAgentIPC"
    }

    # -------------------------------------------------------------
    # Summary
    # -------------------------------------------------------------
    Write-Host "`n=================================================================" -ForegroundColor $(if ($global:failCount -eq 0) { "Green" } else { "Red" })
    Write-Host " ONUSLY D3.4 ACCEPTANCE SUMMARY: $global:passCount PASSED, $global:failCount FAILED" -ForegroundColor $(if ($global:failCount -eq 0) { "Green" } else { "Red" })
    Write-Host "=================================================================`n"

    if ($global:failCount -gt 0) {
        exit 1
    } else {
        exit 0
    }
} finally {
    Pop-Location
}
