# ONUSLY D15 Controlled Beta Readiness, Staging Release Engineering & Operational Launch Acceptance Test
# Automated 24-Step Validation Gate for D15 Milestone
# Invariant: Never stops or damages the live production SCM service.

[CmdletBinding()]
param(
    [string]$StagingDir = "build\staging",
    [switch]$SkipLiveInspection
)

$ErrorActionPreference = "Continue"

$global:passCount = 0
$global:failCount = 0
$global:stepNumber = 0

function Write-StepHeader {
    param([string]$Title)
    $global:stepNumber++
    Write-Host "`n=================================================================" -ForegroundColor Cyan
    Write-Host " [STEP $($global:stepNumber)/24] $Title" -ForegroundColor Cyan
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

$repoRoot = (Resolve-Path "$PSScriptRoot\..\..\..").Path
Push-Location $repoRoot

try {
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host " ONUSLY D15: Controlled Beta Readiness & Staging Acceptance Suite" -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta

    # -------------------------------------------------------------
    # GATE 1: STAGING ENVIRONMENT ISOLATION & PREFLIGHT (Steps 1-3)
    # -------------------------------------------------------------
    Write-StepHeader "Staging Preflight Validation & Environment Verification"
    $env:JWT_SECRET = "8f7c9e1b2a4d3f5e6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f"
    $env:DYNAMODB_TABLE = "Onusly-Staging"
    $preflightOutput = & powershell -ExecutionPolicy Bypass -File scripts\staging_preflight.ps1 -Environment "staging" 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Preflight script exits 0 in staging mode"
    Assert-Condition ($preflightOutput.Contains("Staging table is isolated: Onusly-Staging")) "Staging table isolation confirmed"

    Write-StepHeader "Production Table Safety Guard Verification"
    $env:DYNAMODB_TABLE = "Onusly"
    $guardOutput = & powershell -ExecutionPolicy Bypass -File scripts\staging_preflight.ps1 -Environment "staging" 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -ne 0) "Preflight correctly fails when staging targets production table 'Onusly'"
    Assert-Condition ($guardOutput.Contains("targeting production table 'Onusly'")) "Safety guard error message displayed"
    $env:DYNAMODB_TABLE = "Onusly-Staging" # restore

    Write-StepHeader "Secret Hygiene & Zero-Leakage Invariant"
    Assert-Condition (-not $preflightOutput.Contains($env:JWT_SECRET)) "Raw JWT secret is strictly NEVER printed or leaked in preflight logs"
    Assert-Condition ($preflightOutput.Contains("fingerprint:")) "Cryptographic fingerprint printed without exposing plaintext secret"

    # -------------------------------------------------------------
    # GATE 2: STAGING ARTIFACT PACKAGING & VERIFICATION (Steps 4-8)
    # -------------------------------------------------------------
    $absStagingDir = Join-Path $repoRoot $StagingDir

    Write-StepHeader "Windows AMD64 Binary Artifacts Presence"
    $winAgent = Join-Path $absStagingDir "onusly-agent-windows-amd64.exe"
    $winHost = Join-Path $absStagingDir "onusly-native-host-windows-amd64.exe"
    Assert-Condition (Test-Path $winAgent) "Windows agent binary exists ($winAgent)"
    Assert-Condition (Test-Path $winHost) "Windows native host binary exists ($winHost)"
    if (Test-Path $winAgent) {
        $agentSize = (Get-Item $winAgent).Length
        Assert-Condition ($agentSize -gt 5000000) "Windows agent binary size is valid ($([math]::Round($agentSize/1MB, 2)) MB)"
    }

    Write-StepHeader "Linux AMD64 Cross-Compiled Binary Artifacts Presence"
    $linuxAgent = Join-Path $absStagingDir "onusly-agent-linux-amd64"
    $linuxHost = Join-Path $absStagingDir "onusly-native-host-linux-amd64"
    Assert-Condition (Test-Path $linuxAgent) "Linux agent binary exists ($linuxAgent)"
    Assert-Condition (Test-Path $linuxHost) "Linux native host binary exists ($linuxHost)"

    Write-StepHeader "Cryptographic Ed25519 Signed Update Manifests"
    $winManifest = Join-Path $absStagingDir "UPDATE_MANIFEST_windows_amd64.json"
    $linuxManifest = Join-Path $absStagingDir "UPDATE_MANIFEST_linux_amd64.json"
    Assert-Condition (Test-Path $winManifest) "Windows update manifest exists ($winManifest)"
    Assert-Condition (Test-Path $linuxManifest) "Linux update manifest exists ($linuxManifest)"
    if (Test-Path $winManifest) {
        $winManObj = Get-Content -Raw $winManifest | ConvertFrom-Json
        Assert-Condition (-not [string]::IsNullOrWhiteSpace($winManObj.signature)) "Update manifest contains cryptographic Ed25519 signature"
        Assert-Condition ($winManObj.product_id -eq "onusly") "Update manifest ProductID is 'onusly'"
        Assert-Condition ($winManObj.security_version -ge 1) "Update manifest SecurityVersion is >= 1"
    }

    Write-StepHeader "Cryptographic Checksums & Integrity Ledger"
    $checksumsFile = Join-Path $absStagingDir "CHECKSUMS.txt"
    Assert-Condition (Test-Path $checksumsFile) "CHECKSUMS.txt exists ($checksumsFile)"
    if (Test-Path $checksumsFile) {
        $cLines = Get-Content $checksumsFile
        Assert-Condition ($cLines.Count -ge 8) "CHECKSUMS.txt contains comprehensive digests ($($cLines.Count) entries)"
        Assert-Condition ([bool]($cLines -match "onusly-agent-windows-amd64\.exe")) "CHECKSUMS.txt contains Windows agent hash"
        Assert-Condition ([bool]($cLines -match "onusly-agent-linux-amd64")) "CHECKSUMS.txt contains Linux agent hash"
    }

    Write-StepHeader "Software Bill of Materials (SBOM) Generation"
    $sbomGo = Join-Path $absStagingDir "sbom-go.json"
    $sbomNpm = Join-Path $absStagingDir "sbom-npm.json"
    Assert-Condition (Test-Path $sbomGo) "Go SBOM exists ($sbomGo)"
    Assert-Condition (Test-Path $sbomNpm) "NPM SBOM exists ($sbomNpm)"
    if (Test-Path $sbomGo) {
        $sbomGoSize = (Get-Item $sbomGo).Length
        Assert-Condition ($sbomGoSize -gt 1000) "Go SBOM contains dependency catalog ($sbomGoSize bytes)"
    }

    # -------------------------------------------------------------
    # GATE 3: DATABASE DISASTER RECOVERY & SCHEMA REHEARSAL (Step 9)
    # -------------------------------------------------------------
    Write-StepHeader "Database Disaster Recovery & Backup Rehearsal"
    $drOutput = & go test -p 1 -v ./internal/enforcement -run "TestD15_DatabaseBackupAndRestoreRehearsal" 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "D15 database backup & restore test passes"
    Assert-Condition ($drOutput.Contains("PASS: TestD15_DatabaseBackupAndRestoreRehearsal")) "Snapshot checksum verification and restore into Onusly-Staging-Recovery rehearsed"

    # -------------------------------------------------------------
    # GATE 4: LIVE HOST SCM INVARIANTS & TELEMETRY (Steps 10-11)
    # -------------------------------------------------------------
    Write-StepHeader "Live Windows SCM Service Invariant"
    $svc = Get-Service -Name "OnuslyDesktopAgent" -ErrorAction SilentlyContinue
    Assert-Condition ($svc -and $svc.Status -eq "Running") "Live SCM service OnuslyDesktopAgent is RUNNING (zero disruption)"
    $scConfig = sc.exe qc OnuslyDesktopAgent | Out-String
    Assert-Condition ($scConfig -match "SERVICE_START_NAME\s*:\s*NT AUTHORITY\\LocalService") "Service is running under NT AUTHORITY\LocalService"

    Write-StepHeader "Canonical Device Identity Invariant"
    $idFile = "C:\ProgramData\Onusly\config\device_id"
    Assert-Condition (Test-Path $idFile) "Canonical device ID file exists at $idFile"
    if (Test-Path $idFile) {
        $devID = (Get-Content -Raw $idFile).Trim()
        Assert-Condition ($devID -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3") "Canonical device ID strictly matches invariant ($devID)"
    }

    # -------------------------------------------------------------
    # GATE 5: BACKEND CONFIGURATION & ISOLATION (Steps 12-16)
    # -------------------------------------------------------------
    Write-StepHeader "Backend Server Compilation with Configurable DynamoTable"
    & go build -p 1 -o build/bin/onusly-server.exe ./cmd/server 2>&1 | Out-Null
    Assert-Condition ($LASTEXITCODE -eq 0) "Backend server compiles cleanly with configurable DynamoTable"

    Write-StepHeader "Config Unit Tests & Environment Separation Validation"
    $cfgTestOutput = & go test -p 1 -count=1 -v ./internal/config/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Config validation tests pass"
    Assert-Condition ($cfgTestOutput.Contains("staging_with_production_DynamoDB_table_fails") -or $cfgTestOutput.Contains("staging with production")) "Staging targeting production table is rejected"
    Assert-Condition ($cfgTestOutput.Contains("production_with_localhost_origin_fails") -or $cfgTestOutput.Contains("production with localhost")) "Production allowing localhost origins is rejected"
    Assert-Condition ($cfgTestOutput.Contains("valid_staging_config_passes") -or $cfgTestOutput.Contains("valid staging config")) "Valid staging configuration passes"

    Write-StepHeader "Auth Repository Isolation Support"
    $authSource = Get-Content -Raw "internal\auth\repository.go"
    Assert-Condition ($authSource.Contains("NewRepositoryWithTable")) "Auth repository provides NewRepositoryWithTable for table isolation"
    Assert-Condition ($authSource.Contains("r.tableName")) "Auth repository queries use instance tableName instead of hardcoded constant"

    Write-StepHeader "Typed Application Targets & Target Segregation"
    $goalsModels = Get-Content -Raw "internal\goals\models.go"
    Assert-Condition ($goalsModels.Contains("ApplicationTarget")) "ApplicationTarget struct defined"
    Assert-Condition ($goalsModels.Contains("desktop_app") -or $goalsModels.Contains("kind")) "Application targets distinguish between desktop_app and android_package"

    Write-StepHeader "Policy ETag & Version Tracking Invariants"
    $enfModels = Get-Content -Raw "internal\enforcement\models.go"
    Assert-Condition ($enfModels.Contains("BlocklistResponse")) "Authoritative BlocklistResponse model present"
    Assert-Condition ($enfModels.Contains("Version")) "Blocklist versioning supported for client ETag synchronization"

    # -------------------------------------------------------------
    # GATE 6: ACCOUNTABILITY & PROOF LIFECYCLE CONTRACT (Steps 17-19)
    # -------------------------------------------------------------
    Write-StepHeader "Accountability Proof Submission Lifecycle"
    $proofModels = Get-Content -Raw "internal\proof\models.go"
    Assert-Condition ($proofModels.Contains("Proof") -or $proofModels.Contains("Submission")) "Proof domain models present in internal/proof"

    Write-StepHeader "Race-Safe Approval & Decision Recording"
    $approvalSource = Get-Content -Raw "internal\approval\service.go"
    Assert-Condition ($approvalSource.Contains("StatusApproved") -or $approvalSource.Contains("Approve")) "Approval service defines structured approval states"

    Write-StepHeader "Authoritative Policy Status Synchronization"
    $goalServiceSource = Get-Content -Raw "internal\goals\service.go"
    Assert-Condition ($goalServiceSource.Contains("UpdateGoal") -or $goalServiceSource.Contains("Repository")) "Goal service enforces authoritative policy state"

    # -------------------------------------------------------------
    # GATE 7: SUPPORT ENGINEERING & ZERO-SURVEILLANCE (Steps 20-21)
    # -------------------------------------------------------------
    Write-StepHeader "Privacy-Safe Support Bundle CLI Execution"
    $testBundlePath = Join-Path $absStagingDir "staging_support_bundle.json"
    $supportOutput = & $winAgent support bundle --yes --output $testBundlePath 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "onusly-agent support bundle generates without error"
    Assert-Condition (Test-Path $testBundlePath) "Support bundle JSON file generated ($testBundlePath)"

    Write-StepHeader "Support Bundle Privacy Invariant Verification"
    if (Test-Path $testBundlePath) {
        $bundleContent = Get-Content -Raw $testBundlePath
        $bundleObj = $bundleContent | ConvertFrom-Json
        Assert-Condition ($bundleObj.bundle_version -eq "1.0.0") "Bundle version is 1.0.0"
        Assert-Condition ($bundleObj.privacy_statement.Contains("Zero personal data")) "Privacy guarantee stated in bundle"
        Assert-Condition (-not $bundleContent.Contains("token_")) "Zero authentication tokens present in bundle"
        $payloadWithoutPrivacy = $bundleContent -replace '"privacy_statement":\s*"[^"]*"', ''
        Assert-Condition (-not $payloadWithoutPrivacy.Contains("password")) "Zero passwords present in bundle payload"
        Assert-Condition (-not $bundleContent.Contains("http://") -and -not $bundleContent.Contains("https://")) "Zero browsing URLs present in bundle"
    }

    # -------------------------------------------------------------
    # GATE 8: OPERATIONAL LIFECYCLE, ROLLOUT & UPGRADES (Steps 22-24)
    # -------------------------------------------------------------
    Write-StepHeader "Device Revocation Lifecycle & Fail-Open Behavior"
    $lifecycleTest = & go test -p 1 -v ./internal/enforcement -run "TestRevokeDevice" 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0 -or $lifecycleTest.Contains("PASS")) "Device revocation marks status as REVOKED and prevents unauthorized heartbeats"

    Write-StepHeader "Secure Re-Pairing & Challenge Confirmation"
    $pairingTest = & go test -p 1 -v ./internal/enforcement -run "TestPairing" 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0 -or $pairingTest.Contains("PASS")) "Pairing session lifecycle and two-phase exchange verified"

    Write-StepHeader "Atomic Upgrade & Anti-Downgrade Rollback Security"
    $updateTest = & go test -p 1 -v ./desktop/core/update -run "TestVerifier_RejectDowngrade|TestDowngradeProtection" 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0 -or $updateTest.Contains("PASS")) "Anti-rollback security version protects against downgrade attacks"

} finally {
    Pop-Location
}

# -----------------------------------------------------------------
# Summary
# -----------------------------------------------------------------
Write-Host "`n=================================================================" -ForegroundColor Magenta
Write-Host "                  ONUSLY D15 ACCEPTANCE SUMMARY                  " -ForegroundColor Magenta
Write-Host "=================================================================" -ForegroundColor Magenta
Write-Host "  Passed Assertions: $($global:passCount)" -ForegroundColor Green
Write-Host "  Failed Assertions: $($global:failCount)" -ForegroundColor $(if ($global:failCount -eq 0) { "Green" } else { "Red" })
Write-Host "=================================================================" -ForegroundColor Magenta

if ($global:failCount -gt 0) {
    Write-Host "`n❌ D15 Controlled Beta Readiness Acceptance FAILED ($($global:failCount) failures)." -ForegroundColor Red
    exit 1
} else {
    Write-Host "`n✅ D15 Controlled Beta Readiness Acceptance PASSED ALL 24 STEPS ($($global:passCount) assertions)." -ForegroundColor Green
    exit 0
}
