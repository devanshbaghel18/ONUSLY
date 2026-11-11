# ONUSLY D14 - SRE Observability, Operational Intelligence, Incident Response and Support Acceptance Suite
#
# Tests and confirms the 8 observability and operational reliability domains:
# 1. Live SCM Service Invariant and Identity
# 2. Authoritative Health State Architecture and Taxonomy
# 3. Desktop CLI Health and Diagnostics
# 4. Backend Observability and Prometheus Exposition (/metrics)
# 5. Heartbeat Lapse Deduplication and Recovery Notifications
# 6. Structured Logging, Bounded File Rotation and Secret Redaction
# 7. Dashboard Health Experience and Frontend State Verification
# 8. SRE Incident Response Runbooks and Operational SLO Targets

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..\..")
$PassCount = 0
$FailCount = 0

function Report-Pass($Title) {
    Write-Host "[PASS] $Title" -ForegroundColor Green
    $script:PassCount++
}

function Report-Fail($Title, $Details) {
    Write-Host "[FAIL] $Title - $Details" -ForegroundColor Red
    $script:FailCount++
}

Write-Host "================================================================" -ForegroundColor Cyan
Write-Host "  ONUSLY D14: SRE OBSERVABILITY, LOGGING AND INCIDENT ACCEPTANCE " -ForegroundColor Cyan
Write-Host "================================================================" -ForegroundColor Cyan

# -----------------------------------------------------------------------------
# 1. LIVE SCM SERVICE INVARIANT AND IDENTITY
# -----------------------------------------------------------------------------
Write-Host "`n--- 1. Live SCM Service Invariant and Identity ---" -ForegroundColor Yellow

$service = Get-Service -Name "OnuslyDesktopAgent" -ErrorAction SilentlyContinue
if ($service -and $service.Status -eq "Running") {
    Report-Pass "Live service OnuslyDesktopAgent is RUNNING"
} else {
    Report-Fail "Live service OnuslyDesktopAgent is not running" "Status: $($service.Status)"
}

$scConfig = sc.exe qc OnuslyDesktopAgent | Out-String
if ($scConfig -match "SERVICE_START_NAME\s*:\s*NT AUTHORITY\\LocalService") {
    Report-Pass "Service runs under NT AUTHORITY\LocalService"
} else {
    Report-Fail "Service is not running under LocalService" $scConfig
}

$idFile = "C:\ProgramData\Onusly\config\device_id"
if (Test-Path $idFile) {
    $devID = (Get-Content -Raw $idFile).Trim()
    if ($devID -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3") {
        Report-Pass "Canonical device ID matches invariant ($devID)"
    } else {
        Report-Fail "Device ID mismatch" "Found: $devID"
    }
} else {
    Report-Fail "Canonical device_id file missing at $idFile" "File not found"
}

# -----------------------------------------------------------------------------
# 2. AUTHORITATIVE HEALTH STATE ARCHITECTURE AND TAXONOMY
# -----------------------------------------------------------------------------
Write-Host "`n--- 2. Authoritative Health State Architecture and Taxonomy ---" -ForegroundColor Yellow

$coreModels = Get-Content -Raw (Join-Path $RepoRoot "desktop\core\models.go")
$requiredHealthStates = @(
    "HealthStateHealthy",
    "HealthStateDegraded",
    "HealthStateOffline",
    "HealthStateAuthenticationRequired",
    "HealthStateDeviceRevoked",
    "HealthStatePolicyStale",
    "HealthStatePolicyMissing",
    "HealthStateDNSUnavailable",
    "HealthStateBrowserDegraded",
    "HealthStateAppEnforcementDegraded"
)

$allFound = $true
foreach ($state in $requiredHealthStates) {
    if ($coreModels -notmatch $state) {
        Report-Fail "Missing authoritative health state in desktop/core/models.go" $state
        $allFound = $false
    }
}
if ($allFound) {
    Report-Pass "All 10 authoritative health states defined in desktop/core/models.go"
}

$enfModels = Get-Content -Raw (Join-Path $RepoRoot "internal\enforcement\models.go")
if ($enfModels -match "HealthStatus\s+string" -and $enfModels -match "HealthStateHealthy") {
    Report-Pass "Backend enforcement models contain typed HealthStatus and health states"
} else {
    Report-Fail "Backend enforcement models missing HealthStatus field" ""
}

# -----------------------------------------------------------------------------
# 3. DESKTOP CLI HEALTH AND DIAGNOSTICS
# -----------------------------------------------------------------------------
Write-Host "`n--- 3. Desktop CLI Health and Diagnostics ---" -ForegroundColor Yellow

$agentBin = Join-Path $RepoRoot "build\bin\onusly-agent.exe"
if (Test-Path $agentBin) {
    $statusOut = & $agentBin status | Out-String
    if ($statusOut -match "ONUSLY DESKTOP AGENT" -and $statusOut -match "Device ID:") {
        Report-Pass "CLI onusly-agent status returns structured agent status"
    } else {
        Report-Fail "CLI onusly-agent status output malformed" $statusOut
    }

    $diagOut = & $agentBin diagnostics | Out-String
    if ($diagOut -match "ONUSLY AGENT DIAGNOSTICS" -and $diagOut -match "device_id:") {
        Report-Pass "CLI onusly-agent diagnostics returns structured key-value diagnostics"
    } else {
        Report-Fail "CLI onusly-agent diagnostics output malformed" $diagOut
    }

    # Verify zero token leakage in CLI output
    if ($statusOut -notmatch "eyJ[A-Za-z0-9_-]{10,}" -and $diagOut -notmatch "eyJ[A-Za-z0-9_-]{10,}") {
        Report-Pass "Zero credential or token leakage in CLI status and diagnostics"
    } else {
        Report-Fail "SECURITY DEFECT: JWT or credential leaked in CLI output!" ""
    }
} else {
    Report-Fail "onusly-agent binary not found at $agentBin" ""
}

# -----------------------------------------------------------------------------
# 4. BACKEND OBSERVABILITY AND PROMETHEUS EXPOSITION (/metrics)
# -----------------------------------------------------------------------------
Write-Host "`n--- 4. Backend Observability and Prometheus Exposition ---" -ForegroundColor Yellow

Push-Location $RepoRoot
try {
    $metricsTestOutput = go test -v ./internal/metrics/... | Out-String
    if ($LASTEXITCODE -eq 0 -and $metricsTestOutput -match "PASS") {
        Report-Pass "internal/metrics unit tests passed cleanly (Prometheus and JSON exposition)"
    } else {
        Report-Fail "internal/metrics unit tests failed" $metricsTestOutput
    }
} finally {
    Pop-Location
}

$metricsCode = Get-Content -Raw (Join-Path $RepoRoot "internal\metrics\metrics.go")
if ($metricsCode -match "PrometheusFormat\(\)" -and $metricsCode -match "onusly_api_requests_total" -and $metricsCode -match "onusly_auth_failures_total") {
    Report-Pass "Metrics registry provides standard Prometheus text exposition"
} else {
    Report-Fail "PrometheusFormat missing required metrics counters" ""
}

if ($metricsCode -match "SanitizePath" -and $metricsCode -match ":id") {
    Report-Pass "Cardinality bounded via SanitizePath scrubbing UUIDs to :id"
} else {
    Report-Fail "SanitizePath missing UUID cardinality scrubber" ""
}

# -----------------------------------------------------------------------------
# 5. HEARTBEAT LAPSE DEDUPLICATION AND RECOVERY NOTIFICATIONS
# -----------------------------------------------------------------------------
Write-Host "`n--- 5. Heartbeat Lapse Deduplication and Recovery Notifications ---" -ForegroundColor Yellow

Push-Location $RepoRoot
try {
    $lapseTestOutput = go test -v -run TestD14_LapseDeduplicationAndRecoveryNotification ./internal/enforcement/... | Out-String
    if ($lapseTestOutput -match "PASS: TestD14_LapseDeduplicationAndRecoveryNotification") {
        Report-Pass "Lapse deduplication and recovery notifications verified via automated test"
    } else {
        Report-Fail "Lapse deduplication test failed" $lapseTestOutput
    }
} finally {
    Pop-Location
}

# -----------------------------------------------------------------------------
# 6. STRUCTURED LOGGING, BOUNDED FILE ROTATION AND SECRET REDACTION
# -----------------------------------------------------------------------------
Write-Host "`n--- 6. Structured Logging and Bounded File Rotation ---" -ForegroundColor Yellow

Push-Location $RepoRoot
try {
    $loggingTestOutput = go test -v ./desktop/agent/internal/logging/... | Out-String
    if ($LASTEXITCODE -eq 0 -and $loggingTestOutput -match "PASS") {
        Report-Pass "Structured logging and bounded file rotation tests passed cleanly"
    } else {
        Report-Fail "Logging tests failed" $loggingTestOutput
    }
} finally {
    Pop-Location
}

$rotatorCode = Get-Content -Raw (Join-Path $RepoRoot "desktop\agent\internal\logging\rotator.go")
if ($rotatorCode -match "Redact\(" -and $rotatorCode -match "rotateLocked\(" -and $rotatorCode -match "writeErrors") {
    Report-Pass "Log rotator implements secret redaction, index shifting, and fail-open write protection"
} else {
    Report-Fail "Log rotator missing essential reliability mechanisms" ""
}

# -----------------------------------------------------------------------------
# 7. DASHBOARD HEALTH EXPERIENCE AND FRONTEND TESTS
# -----------------------------------------------------------------------------
Write-Host "`n--- 7. Dashboard Health Experience and Frontend State Verification ---" -ForegroundColor Yellow

$coverageCode = Get-Content -Raw (Join-Path $RepoRoot "frontend\src\components\DeviceCoverageSection.jsx")
if ($coverageCode -match "DNS_UNAVAILABLE" -and $coverageCode -match "APP_ENFORCEMENT_DEGRADED" -and $coverageCode -match "getLifecycleBadge") {
    Report-Pass "Dashboard device coverage maps component and aggregate health states"
} else {
    Report-Fail "Dashboard missing component health state mapping" ""
}

Push-Location (Join-Path $RepoRoot "frontend")
try {
    $frontendTestOutput = npm test | Out-String
    if ($frontendTestOutput -match "pass 21" -and $frontendTestOutput -notmatch "fail [1-9]") {
        Report-Pass "All 21 frontend unit tests passed cleanly"
    } else {
        Report-Fail "Frontend unit tests failed" $frontendTestOutput
    }
} finally {
    Pop-Location
}

# -----------------------------------------------------------------------------
# 8. SRE INCIDENT RESPONSE RUNBOOKS AND OPERATIONAL SLO TARGETS
# -----------------------------------------------------------------------------
Write-Host "`n--- 8. SRE Incident Response Runbooks and Operational Targets ---" -ForegroundColor Yellow

$runbookFiles = @(
    "INCIDENT_RESPONSE_DNS_FAILURE.md",
    "INCIDENT_RESPONSE_POLICY_STALE.md",
    "INCIDENT_RESPONSE_DEVICE_REVOCATION.md",
    "INCIDENT_RESPONSE_APP_ENFORCEMENT_DEGRADED.md",
    "SLO_OPERATIONAL_TARGETS.md"
)

$runbooksValid = $true
foreach ($rb in $runbookFiles) {
    $rbPath = Join-Path $RepoRoot (Join-Path "docs\runbooks" $rb)
    if (Test-Path $rbPath) {
        $content = Get-Content -Raw $rbPath
        if ($content.Length -gt 500) {
            # Runbook is substantial
        } else {
            Report-Fail "Runbook $rb is suspiciously short" "Length: $($content.Length)"
            $runbooksValid = $false
        }
    } else {
        Report-Fail "Missing operational runbook" $rb
        $runbooksValid = $false
    }
}
if ($runbooksValid) {
    Report-Pass "All 5 operational incident response runbooks and SLO definitions verified"
}

# -----------------------------------------------------------------------------
# SUMMARY
# -----------------------------------------------------------------------------
Write-Host "`n================================================================" -ForegroundColor Cyan
Write-Host "  D14 OBSERVABILITY ACCEPTANCE SUMMARY: $PassCount PASSED, $FailCount FAILED" -ForegroundColor $(if ($FailCount -eq 0) { "Green" } else { "Red" })
Write-Host "================================================================" -ForegroundColor Cyan

if ($FailCount -gt 0) {
    exit 1
}
exit 0
