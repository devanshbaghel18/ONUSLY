<#
.SYNOPSIS
    ONUSLY D15 Staging Environment Preflight & Configuration Separation Validator.
.DESCRIPTION
    Validates staging/production environment configuration, database table isolation,
    secret complexity/length (without logging raw secrets), network origins, and toolchain readiness.
.PARAMETER Environment
    Target environment: 'staging' (default), 'production', 'development', or 'test'.
.PARAMETER SkipToolchainCheck
    Skips compiler and runtime toolchain verification.
.PARAMETER JsonReportPath
    Optional file path to output structured JSON preflight report.
#>

param(
    [string]$Environment = "staging",
    [switch]$SkipToolchainCheck,
    [string]$JsonReportPath = ""
)

$ErrorActionPreference = "Stop"
$Script:AllPassed = $true
$Script:Checks = @()

function Record-Check {
    param(
        [string]$Name,
        [string]$Status, # "PASS", "FAIL", "WARN"
        [string]$Message,
        [hashtable]$Details = @{}
    )
    $color = switch ($Status) {
        "PASS" { "Green" }
        "FAIL" { "Red" }
        "WARN" { "Yellow" }
        Default { "White" }
    }
    Write-Host "[$Status] $Name - $Message" -ForegroundColor $color
    if ($Status -eq "FAIL") {
        $Script:AllPassed = $false
    }
    $Script:Checks += [PSCustomObject]@{
        Name      = $Name
        Status    = $Status
        Message   = $Message
        Details   = $Details
        Timestamp = (Get-Date).ToString("o")
    }
}

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "  ONUSLY D15 Staging Preflight & Environment Validator   " -ForegroundColor Cyan
Write-Host "  Target Environment: $Environment" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# 1. Environment & Config Isolation
$envName = $Environment.ToLower().Trim()
$validEnvs = @("development", "test", "testing", "staging", "production")
if ($validEnvs -contains $envName) {
    Record-Check -Name "EnvironmentName" -Status "PASS" -Message "Valid target environment: $envName"
}
else {
    Record-Check -Name "EnvironmentName" -Status "FAIL" -Message "Invalid target environment: $envName. Must be one of: $($validEnvs -join ', ')"
}

# 2. DynamoDB Table Isolation Check
$dynamoTable = [System.Environment]::GetEnvironmentVariable("DYNAMODB_TABLE")
if ([string]::IsNullOrWhiteSpace($dynamoTable)) {
    $dynamoTable = "Onusly"
}

if ($envName -eq "staging") {
    if ($dynamoTable -eq "Onusly") {
        Record-Check -Name "DynamoTableIsolation" -Status "FAIL" -Message "Staging environment is targeting production table 'Onusly'. Must set DYNAMODB_TABLE to an isolated table (e.g. Onusly-Staging)." -Details @{ Table = $dynamoTable }
    }
    elseif ($dynamoTable -match "staging" -or $dynamoTable -match "test" -or $dynamoTable -match "beta") {
        Record-Check -Name "DynamoTableIsolation" -Status "PASS" -Message "Staging table is isolated: $dynamoTable" -Details @{ Table = $dynamoTable }
    }
    else {
        Record-Check -Name "DynamoTableIsolation" -Status "WARN" -Message "DYNAMODB_TABLE '$dynamoTable' is non-default, verify it is truly isolated from production." -Details @{ Table = $dynamoTable }
    }
}
elseif ($envName -eq "production") {
    if ($dynamoTable -match "staging" -or $dynamoTable -match "test") {
        Record-Check -Name "DynamoTableIsolation" -Status "FAIL" -Message "Production environment cannot target staging/test table: $dynamoTable" -Details @{ Table = $dynamoTable }
    }
    else {
        Record-Check -Name "DynamoTableIsolation" -Status "PASS" -Message "Production table configured: $dynamoTable" -Details @{ Table = $dynamoTable }
    }
}
else {
    Record-Check -Name "DynamoTableIsolation" -Status "PASS" -Message "Dev/Test table configured: $dynamoTable" -Details @{ Table = $dynamoTable }
}

# 3. Secret Hygiene (Privacy-safe: NEVER print raw secret)
$jwtSecret = [System.Environment]::GetEnvironmentVariable("JWT_SECRET")
if ([string]::IsNullOrWhiteSpace($jwtSecret)) {
    # Check if .env file exists in current or ONUSLY directory
    $envFile = Join-Path (Get-Location) ".env"
    if (-not (Test-Path $envFile)) {
        $envFile = Join-Path (Get-Location) "ONUSLY\.env"
    }
    if (Test-Path $envFile) {
        $match = Select-String -Path $envFile -Pattern "^JWT_SECRET=(.+)$" | Select-Object -First 1
        if ($match) {
            $jwtSecret = $match.Matches[0].Groups[1].Value.Trim()
        }
    }
}

if ([string]::IsNullOrWhiteSpace($jwtSecret)) {
    Record-Check -Name "JWTSecretConfigured" -Status "FAIL" -Message "JWT_SECRET is not set in environment or .env"
}
else {
    $secLen = $jwtSecret.Length
    $sha256 = [System.Security.Cryptography.SHA256]::Create()
    $hashBytes = $sha256.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($jwtSecret))
    $hashHex = -join ($hashBytes | ForEach-Object { "{0:x2}" -f $_ })
    $fingerprint = "$($hashHex.Substring(0, 8))...$($hashHex.Substring($hashHex.Length - 8))"

    if ($envName -in @("staging", "production")) {
        if ($secLen -lt 32) {
            Record-Check -Name "JWTSecretStrength" -Status "FAIL" -Message "$envName JWT_SECRET must be at least 32 characters (found: $secLen chars)" -Details @{ Length = $secLen; Fingerprint = $fingerprint }
        }
        else {
            $weakTokens = @("secret", "password", "development", "staging", "1234567890123456")
            $hasWeak = $false
            $lower = $jwtSecret.ToLower()
            foreach ($bad in $weakTokens) {
                if ($lower.Contains($bad)) {
                    $hasWeak = $true
                    Record-Check -Name "JWTSecretStrength" -Status "FAIL" -Message "$envName JWT_SECRET contains weak dictionary token '$bad'" -Details @{ Fingerprint = $fingerprint }
                    break
                }
            }
            if (-not $hasWeak) {
                Record-Check -Name "JWTSecretStrength" -Status "PASS" -Message "JWT_SECRET meets cryptographic complexity ($secLen chars, fingerprint: $fingerprint)" -Details @{ Length = $secLen; Fingerprint = $fingerprint }
            }
        }
    }
    else {
        if ($secLen -lt 16) {
            Record-Check -Name "JWTSecretStrength" -Status "FAIL" -Message "JWT_SECRET must be at least 16 characters (found: $secLen chars)" -Details @{ Length = $secLen }
        }
        else {
            Record-Check -Name "JWTSecretStrength" -Status "PASS" -Message "JWT_SECRET meets dev requirement ($secLen chars)" -Details @{ Length = $secLen }
        }
    }
}

# 4. Allowed Origins Check
$allowedOrigins = [System.Environment]::GetEnvironmentVariable("ALLOWED_ORIGINS")
if ($envName -eq "production") {
    if ([string]::IsNullOrWhiteSpace($allowedOrigins)) {
        Record-Check -Name "AllowedOrigins" -Status "FAIL" -Message "Production requires explicit ALLOWED_ORIGINS configuration."
    }
    elseif ($allowedOrigins -match "localhost" -or $allowedOrigins -match "127\.0\.0\.1") {
        Record-Check -Name "AllowedOrigins" -Status "FAIL" -Message "Production ALLOWED_ORIGINS cannot contain localhost or loopback addresses." -Details @{ Origins = $allowedOrigins }
    }
    else {
        Record-Check -Name "AllowedOrigins" -Status "PASS" -Message "Production origins validated: $allowedOrigins" -Details @{ Origins = $allowedOrigins }
    }
}
else {
    $originsDesc = if ([string]::IsNullOrWhiteSpace($allowedOrigins)) { "Default localhost dev origins active" } else { $allowedOrigins }
    Record-Check -Name "AllowedOrigins" -Status "PASS" -Message "Allowed origins: $originsDesc"
}

# 5. Toolchain & Environment Verification
if (-not $SkipToolchainCheck) {
    # Check Go
    try {
        $goVer = go version 2>&1
        if ($LASTEXITCODE -eq 0 -and $goVer -match "go(\d+\.\d+)") {
            $parsed = [version]$Matches[1]
            if ($parsed -ge [version]"1.22") {
                Record-Check -Name "ToolchainGo" -Status "PASS" -Message "Go version: $goVer" -Details @{ Version = $goVer }
            }
            else {
                Record-Check -Name "ToolchainGo" -Status "FAIL" -Message "Go version $parsed is below required 1.22"
            }
        }
        else {
            Record-Check -Name "ToolchainGo" -Status "FAIL" -Message "Go is not found in PATH"
        }
    }
    catch {
        Record-Check -Name "ToolchainGo" -Status "FAIL" -Message "Error checking Go: $_"
    }

    # Check Node.js
    try {
        $nodeVer = node --version 2>&1
        if ($LASTEXITCODE -eq 0 -and $nodeVer -match "v(\d+)") {
            $major = [int]$Matches[1]
            if ($major -ge 18) {
                Record-Check -Name "ToolchainNode" -Status "PASS" -Message "Node.js version: $nodeVer" -Details @{ Version = $nodeVer }
            }
            else {
                Record-Check -Name "ToolchainNode" -Status "FAIL" -Message "Node.js version $major is below required 18"
            }
        }
        else {
            Record-Check -Name "ToolchainNode" -Status "FAIL" -Message "Node.js is not found in PATH"
        }
    }
    catch {
        Record-Check -Name "ToolchainNode" -Status "FAIL" -Message "Error checking Node.js: $_"
    }
}

# 6. Service Invariant Validation (Live desktop agent must remain running)
try {
    $svc = Get-Service -Name "OnuslyDesktopAgent" -ErrorAction SilentlyContinue
    if ($svc) {
        if ($svc.Status -eq "Running") {
            Record-Check -Name "DesktopAgentService" -Status "PASS" -Message "Live SCM service OnuslyDesktopAgent is Running (invariant maintained)" -Details @{ Status = "$($svc.Status)" }
        }
        else {
            Record-Check -Name "DesktopAgentService" -Status "WARN" -Message "OnuslyDesktopAgent service exists but status is $($svc.Status)" -Details @{ Status = "$($svc.Status)" }
        }
    }
    else {
        Record-Check -Name "DesktopAgentService" -Status "WARN" -Message "OnuslyDesktopAgent service not registered on this host"
    }
}
catch {
    Record-Check -Name "DesktopAgentService" -Status "WARN" -Message "Could not query SCM service: $_"
}

Write-Host "----------------------------------------------------------" -ForegroundColor Gray
$passCount = @($Script:Checks | Where-Object { $_.Status -eq "PASS" }).Count
$failCount = @($Script:Checks | Where-Object { $_.Status -eq "FAIL" }).Count
$warnCount = @($Script:Checks | Where-Object { $_.Status -eq "WARN" }).Count

Write-Host "Preflight Summary: $passCount passed, $failCount failed, $warnCount warnings." -ForegroundColor $(if ($Script:AllPassed) { "Green" } else { "Red" })

if (-not [string]::IsNullOrWhiteSpace($JsonReportPath)) {
    $reportObj = [PSCustomObject]@{
        Environment = $Environment
        AllPassed   = $Script:AllPassed
        PassCount   = $passCount
        FailCount   = $failCount
        WarnCount   = $warnCount
        Timestamp   = (Get-Date).ToString("o")
        Checks      = $Script:Checks
    }
    $reportDir = Split-Path -Parent $JsonReportPath
    if (-not [string]::IsNullOrWhiteSpace($reportDir) -and -not (Test-Path $reportDir)) {
        New-Item -ItemType Directory -Path $reportDir -Force | Out-Null
    }
    $reportObj | ConvertTo-Json -Depth 5 | Set-Content -Path $JsonReportPath -Encoding UTF8
    Write-Host "Structured JSON report written to: $JsonReportPath" -ForegroundColor Cyan
}

if (-not $Script:AllPassed) {
    Write-Host "Preflight checks FAILED. Correct configuration before proceeding." -ForegroundColor Red
    exit 1
}

Write-Host "Staging preflight PASSED successfully." -ForegroundColor Green
exit 0
