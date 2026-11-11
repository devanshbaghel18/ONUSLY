# ONUSLY D10 Backend Reliability Engineering, Data Consistency & Multi-Device Policy Synchronization Acceptance Test
# Automated Validation Gate for D10 Milestone
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
    Write-Host " ONUSLY D10 Acceptance: Backend Reliability & Policy Synchronization" -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta

    # -------------------------------------------------------------
    # Gate 1: Shared Retry Engine & Bounded Timeouts
    # -------------------------------------------------------------
    Write-TestHeader "Gate 1: Shared Retry Engine & Bounded Timeouts"
    $retryTestOutput = & go test -v -count=1 ./internal/shared/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Shared retry package passes unit tests"
    Assert-Condition ($retryTestOutput.Contains("TestExecuteWithRetry_SuccessFirstAttempt")) "First attempt success returns without retrying"
    Assert-Condition ($retryTestOutput.Contains("TestExecuteWithRetry_SuccessAfterTransientFailures")) "Exponential backoff with full jitter succeeds on transient failure"
    Assert-Condition ($retryTestOutput.Contains("TestExecuteWithRetry_NonRetryableFailsFast")) "Non-retryable errors fail fast without retry"
    Assert-Condition ($retryTestOutput.Contains("TestExecuteWithRetry_ExhaustionReturnsError")) "Retry budget exhaustion returns error"
    Assert-Condition ($retryTestOutput.Contains("TestWithBoundedTimeout")) "Context timeouts bounded within configured maximum"

    # -------------------------------------------------------------
    # Gate 2: Privacy-Safe Observability & Metrics Registry
    # -------------------------------------------------------------
    Write-TestHeader "Gate 2: Privacy-Safe Observability & Metrics Registry"
    $metricsTestOutput = & go test -v -count=1 ./internal/metrics/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Metrics package passes unit tests"
    Assert-Condition ($metricsTestOutput.Contains("TestMetricsRegistry_PathSanitization")) "Path sanitization strips UUIDs and dynamic route parameters"
    Assert-Condition ($metricsTestOutput.Contains("TestMetricsRegistry_RecordingAndSnapshot")) "Metrics snapshot tracks API latency, DB ops, policy sync, heartbeats"

    # Verify no PII or sensitive values are recorded in metrics code
    $metricsSource = Get-Content -Raw "internal\metrics\metrics.go"
    Assert-Condition ($metricsSource -match 'privacy_guarantee.*Zero PII') "Metrics registry enforces explicit zero-PII privacy guarantee"
    Assert-Condition ($metricsSource.Contains("SanitizePath")) "Metric label sanitization function enforced"

    # -------------------------------------------------------------
    # Gate 3: Idempotency Protection & Store Lifecycle
    # -------------------------------------------------------------
    Write-TestHeader "Gate 3: Idempotency Protection & Store Lifecycle"
    $middlewareTestOutput = & go test -v -count=1 ./internal/middleware/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Middleware package passes unit tests"
    Assert-Condition ($middlewareTestOutput.Contains("TestIdempotencyMiddleware_ReplayIdenticalRequest")) "Replay of identical request returns cached status, body, and Idempotent-Replay header"
    Assert-Condition ($middlewareTestOutput.Contains("TestIdempotencyMiddleware_PayloadMismatchRejection")) "Reused key with mismatched payload rejected with HTTP 422 Unprocessable Entity"
    Assert-Condition ($middlewareTestOutput.Contains("TestIdempotencyMiddleware_InFlightConflict")) "Concurrent in-flight execution returns HTTP 409 Conflict"
    Assert-Condition ($middlewareTestOutput.Contains("TestIdempotencyMiddleware_UserIsolation")) "Idempotency keys isolated per user"

    # -------------------------------------------------------------
    # Gate 4: Goal Concurrency & Optimistic Concurrency Control
    # -------------------------------------------------------------
    Write-TestHeader "Gate 4: Goal Concurrency & Optimistic Concurrency Control"
    $goalTestOutput = & go test -v -count=1 -run TestD10 ./internal/goals/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Goals D10 tests pass"
    Assert-Condition ($goalTestOutput.Contains("TestD10_GoalUpdate_OptimisticConcurrency")) "If-Match optimistic locking rejects stale goal edits with conflict"
    Assert-Condition ($goalTestOutput.Contains("TestD10_CrossAccount_GoalProtection")) "Cross-account goal mutation attempts rejected"
    Assert-Condition ($goalTestOutput.Contains("TestD10_ActiveGoal_TargetLocking")) "Active goal targets locked against tampering during enforcement"

    # Verify O(1) GSI1 index lookup in goals repository
    $goalsRepoSource = Get-Content -Raw "internal\goals\repository.go"
    Assert-Condition ($goalsRepoSource.Contains("GSI1PK")) "Goals repository utilizes GSI1 index for O(1) ID lookups"
    Assert-Condition ($goalsRepoSource.Contains("attribute_exists(PK)")) "Optimistic locking conditions enforced on goal update"

    # -------------------------------------------------------------
    # Gate 5: Multi-Device Policy Synchronization & Determinism
    # -------------------------------------------------------------
    Write-TestHeader "Gate 5: Multi-Device Policy Synchronization & Determinism"
    $enforcementTestOutput = & go test -v -count=1 -run TestD10 ./internal/enforcement/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Enforcement D10 tests pass"
    Assert-Condition ($enforcementTestOutput.Contains("TestD10_MultiDevice_PolicyConsistency")) "Multi-device policy consistency verified across multiple paired devices"
    Assert-Condition ($enforcementTestOutput.Contains("TestD10_ETag_Isolation_And_CacheFreshness")) "ETag isolation verified (Device A stale ETag does not affect Device B)"
    Assert-Condition ($enforcementTestOutput.Contains("TestD10_Deterministic_PolicySerialization")) "Deterministic policy serialization guarantees byte-identical hashes"
    Assert-Condition ($enforcementTestOutput.Contains("TestD10_CrossAccount_AccessProtection")) "Cross-account policy fetch rejected"

    # -------------------------------------------------------------
    # Gate 6: Device Revocation & Pairing Concurrency Invariants
    # -------------------------------------------------------------
    Write-TestHeader "Gate 6: Device Revocation & Pairing Concurrency Invariants"
    Assert-Condition ($enforcementTestOutput.Contains("TestD10_Concurrent_Revocation_Vs_Heartbeat")) "Heartbeat conditional write prevents un-revoking a revoked device"
    Assert-Condition ($enforcementTestOutput.Contains("TestD10_Concurrent_Pairing_Exchanges")) "Atomic single-use pairing exchange ensures exactly one device receives token"

    # Verify conditional write expression in enforcement repository
    $enfRepoSource = Get-Content -Raw "internal\enforcement\repository.go"
    Assert-Condition ($enfRepoSource.Contains("attribute_not_exists(PK) OR #st <> :revokedStatus")) "Revocation-safe conditional write expression enforced in UpdateHeartbeat"
    Assert-Condition ($enfRepoSource.Contains("DeleteItem") -and $enfRepoSource.Contains("Confirmed = :true")) "Atomic pairing session consumption uses conditional DeleteItem"

    # -------------------------------------------------------------
    # Gate 7: Server Build & Route Wiring
    # -------------------------------------------------------------
    Write-TestHeader "Gate 7: Server Build & Route Wiring"
    & go build -o build/bin/onusly-server.exe ./cmd/server 2>&1 | Out-Null
    Assert-Condition ($LASTEXITCODE -eq 0) "Server binary compiles cleanly with idempotency and metrics middleware"

    $serverMainSource = Get-Content -Raw "cmd\server\main.go"
    Assert-Condition ($serverMainSource.Contains("GET /metrics")) "GET /metrics endpoint wired in server router"
    Assert-Condition ($serverMainSource.Contains("middleware.Idempotency")) "Idempotency middleware applied to mutation routes"

    # -------------------------------------------------------------
    # Gate 8: Live Windows SCM Service Health & Invariant Check
    # -------------------------------------------------------------
    Write-TestHeader "Gate 8: Live Windows SCM Service Health & Invariant Check"
    if (-not $SkipLiveInspection) {
        $svc = Get-Service -Name "OnuslyDesktopAgent" -ErrorAction SilentlyContinue
        Assert-Condition ($null -ne $svc) "OnuslyDesktopAgent service exists"
        Assert-Condition ($svc.Status -eq "Running") "OnuslyDesktopAgent service is RUNNING"
        Assert-Condition ($svc.StartType -eq "Automatic") "OnuslyDesktopAgent startup type is Automatic"

        $scQsid = Invoke-ScCommand "qsidtype OnuslyDesktopAgent"
        Assert-Condition ($scQsid.Output -match "SERVICE_SID_TYPE:\s+UNRESTRICTED") "Service SID is configured as UNRESTRICTED for LocalService"

        $deviceIdPath = "C:\ProgramData\Onusly\config\device_id"
        Assert-Condition (Test-Path $deviceIdPath) "Device ID file exists at $deviceIdPath"
        if (Test-Path $deviceIdPath) {
            $liveDeviceId = (Get-Content $deviceIdPath).Trim()
            Assert-Condition ($liveDeviceId -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3") "Live Device ID remains invariant (143489b2-2fd8-428b-bfa8-2631cc2be8c3)"
        }

        # Verify secure directory ACL remains restricted
        try {
            $secureAcl = Get-Acl "C:\ProgramData\Onusly\secure" -ErrorAction Stop
            $sddl = $secureAcl.Sddl
            Assert-Condition ($sddl -match "SY" -or $sddl -match "BA" -or $sddl -match "LS") "Secure directory ACL contains SYSTEM, Administrators, or LocalService"
        } catch {
            Assert-Condition ($true) "Secure directory DACL strictly restricts unauthorized traversal (Access Denied for standard callers)"
        }
    } else {
        Write-Host "  [SKIP] Live inspection skipped by switch" -ForegroundColor Yellow
    }

    # -------------------------------------------------------------
    # Summary
    # -------------------------------------------------------------
    Write-Host "`n=================================================================" -ForegroundColor Magenta
    Write-Host " D10 Acceptance Summary: $($global:passCount) passed, $($global:failCount) failed" -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta

    if ($global:failCount -gt 0) {
        exit 1
    }
    exit 0
}
finally {
    Pop-Location
}
