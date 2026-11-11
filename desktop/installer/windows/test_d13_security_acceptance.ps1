# ONUSLY D13 — Application Security Hardening, Privacy Protection & Abuse Resistance
# Acceptance & Verification Suite
#
# Tests and confirms the 11 security domains:
# 1. Live SCM Service Invariant & Identity
# 2. Windows Named Pipe Security & Impersonation Prevention
# 3. Native Messaging Host Framing Bounds & Action Allowlist
# 4. Strict JWT Algorithm Allowlist & Token Type Separation
# 5. Security Headers, CORS Origin Whitelist & Request Body Bounds
# 6. Abuse Resistance & Rate Limiting (429 Too Many Requests)
# 7. Proof Validation, MIME Screening & Cross-Account Isolation
# 8. Realtime WebSocket Origin Verification & CSWSH Defense
# 9. Android Security, Cleartext Restrictions & EncryptedSharedPreferences
# 10. Secrets & Repository Sanitization
# 11. Privacy Verification (Zero Keylogging, Zero Screen Capture, Zero Private DNS Logging)

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
Write-Host "  ONUSLY D13: APPLICATION SECURITY, PRIVACY & ABUSE ACCEPTANCE  " -ForegroundColor Cyan
Write-Host "================================================================" -ForegroundColor Cyan

# -----------------------------------------------------------------------------
# 1. LIVE SCM SERVICE INVARIANT & IDENTITY
# -----------------------------------------------------------------------------
Write-Host "`n--- 1. Live SCM Service Invariant & Identity ---" -ForegroundColor Yellow

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
    Report-Fail "device_id file missing at $idFile" ""
}

# -----------------------------------------------------------------------------
# 2. WINDOWS NAMED PIPE DACL & IMPERSONATION SQOS
# -----------------------------------------------------------------------------
Write-Host "`n--- 2. Windows Named Pipe DACL & Impersonation SQOS ---" -ForegroundColor Yellow

$pipeSource = Get-Content -Raw (Join-Path $RepoRoot "desktop\agent\internal\ipc\server_windows.go")
if ($pipeSource -match 'sddl := "D:\(A;;GA;;;BA\)\(A;;GA;;;SY\)\(A;;GA;;;LS\)\(A;;GRGW;;;IU\)"') {
    Report-Pass "Named pipe server specifies explicit SDDL DACL restricting access to BA, SY, LS, IU"
} else {
    Report-Fail "Named pipe SDDL DACL missing or invalid" ""
}

if ($pipeSource -match 'ProcImpersonateNamedPipeClient' -and $pipeSource -match 'ProcRevertToSelf') {
    Report-Pass "Named pipe server includes guaranteed RevertToSelf cleanup"
} else {
    Report-Fail "RevertToSelf missing in named pipe server" ""
}

$dialSource = Get-Content -Raw (Join-Path $RepoRoot "desktop\native-host\dial_windows.go")
if ($dialSource -match 'win\.SECURITY_SQOS_PRESENT\s*\|\s*win\.SECURITY_IDENTIFICATION') {
    Report-Pass "Native host connects using SECURITY_IDENTIFICATION SQOS preventing privilege escalation"
} else {
    Report-Fail "Native host client lacks SECURITY_IDENTIFICATION" ""
}

# -----------------------------------------------------------------------------
# 3. NATIVE MESSAGING HOST FRAMING BOUNDS & ACTION ALLOWLIST
# -----------------------------------------------------------------------------
Write-Host "`n--- 3. Native Messaging Host Framing Bounds & Action Allowlist ---" -ForegroundColor Yellow

Push-Location (Join-Path $RepoRoot "desktop\native-host")
try {
    $nativeTestOut = go test -v . 2>&1 | Out-String
    if ($LASTEXITCODE -eq 0 -and $nativeTestOut -match "PASS") {
        Report-Pass "Native messaging host tests pass (framing, bounds, allowlist)"
    } else {
        Report-Fail "Native messaging host tests failed" $nativeTestOut
    }
} finally {
    Pop-Location
}

$nativeMainSource = Get-Content -Raw (Join-Path $RepoRoot "desktop\native-host\main.go")
if ($nativeMainSource -match 'msgLen > 1024\*1024') {
    Report-Pass "Native messaging host caps message length to 1MB ceiling"
} else {
    Report-Fail "Native host message length ceiling missing" ""
}

if ($nativeMainSource -match 'case "get-blocklist", "status", "version", "heartbeat":') {
    Report-Pass "Native messaging host enforces strict action allowlist"
} else {
    Report-Fail "Native host action allowlist missing" ""
}

# -----------------------------------------------------------------------------
# 4. STRICT JWT ALGORITHM ALLOWLIST & TOKEN TYPE SEPARATION
# -----------------------------------------------------------------------------
Write-Host "`n--- 4. Strict JWT Algorithm Allowlist & Token Type Separation ---" -ForegroundColor Yellow

Push-Location $RepoRoot
try {
    $midTestOut = go test -v -run "TestD13_" ./internal/middleware/... 2>&1 | Out-String
    if ($LASTEXITCODE -eq 0 -and $midTestOut -match "PASS") {
        Report-Pass "Middleware security unit tests pass (alg allowlist, token type, headers, limits, rate limiting)"
    } else {
        Report-Fail "Middleware security unit tests failed" $midTestOut
    }
} finally {
    Pop-Location
}

$authMidSource = Get-Content -Raw (Join-Path $RepoRoot "internal\middleware\auth.go")
if ($authMidSource -match 'token\.Method != jwt\.SigningMethodHS256' -and $authMidSource -match 'alg != "HS256"') {
    Report-Pass "Middleware rejects algorithm confusion (requires HS256 explicitly)"
} else {
    Report-Fail "Algorithm check missing or weak in auth middleware" ""
}

if ($authMidSource -match 'RequireUserToken' -and $authMidSource -match 'RequireDeviceToken') {
    Report-Pass "Token type separation middleware implemented (RequireUserToken, RequireDeviceToken)"
} else {
    Report-Fail "Token type middleware missing" ""
}

# -----------------------------------------------------------------------------
# 5. SECURITY HEADERS, CORS ORIGIN WHITELIST & BODY BOUNDS
# -----------------------------------------------------------------------------
Write-Host "`n--- 5. Security Headers, CORS Origin Whitelist & Body Bounds ---" -ForegroundColor Yellow

$serverMain = Get-Content -Raw (Join-Path $RepoRoot "cmd\server\main.go")
if ($serverMain -match 'isAllowedOrigin' -and $serverMain -match 'Access-Control-Allow-Credentials') {
    Report-Pass "Backend enforces strict whitelisted CORS with credential protection"
} else {
    Report-Fail "CORS origin check missing in main.go" ""
}

if ($serverMain -match 'middleware\.SecurityHeaders' -and $serverMain -match 'middleware\.MaxBytesLimiter\(1024\*1024\)') {
    Report-Pass "Backend root handler applies SecurityHeaders and 1MB MaxBytesLimiter"
} else {
    Report-Fail "SecurityHeaders or MaxBytesLimiter missing from root handler" ""
}

if ($serverMain -match 'cleanPath, "\.\."' -and $serverMain -match 'cleanPath, "%2e"') {
    Report-Pass "Downloads handler includes path-traversal guards (rejects '..' and '%2e')"
} else {
    Report-Fail "Path-traversal check missing in /downloads/ handler" ""
}

# -----------------------------------------------------------------------------
# 6. ABUSE RESISTANCE & RATE LIMITING
# -----------------------------------------------------------------------------
Write-Host "`n--- 6. Abuse Resistance & Rate Limiting ---" -ForegroundColor Yellow

if ($serverMain -match 'authLimiter' -and $serverMain -match 'pairingLimiter') {
    Report-Pass "Rate limiters applied to public auth and device pairing endpoints"
} else {
    Report-Fail "Rate limiters missing on sensitive endpoints" ""
}

$rateMid = Get-Content -Raw (Join-Path $RepoRoot "internal\middleware\ratelimit.go")
if ($rateMid -match 'http\.StatusTooManyRequests' -and $rateMid -match 'Retry-After') {
    Report-Pass "Rate limiter emits HTTP 429 Too Many Requests with Retry-After header"
} else {
    Report-Fail "Rate limiter missing 429 or Retry-After" ""
}

# -----------------------------------------------------------------------------
# 7. PROOF VALIDATION, MIME SCREENING & CROSS-ACCOUNT ISOLATION
# -----------------------------------------------------------------------------
Write-Host "`n--- 7. Proof Validation, MIME Screening & Cross-Account Isolation ---" -ForegroundColor Yellow

Push-Location $RepoRoot
try {
    $proofTestOut = go test -v -run "TestD13_ProofValidationSecurity" ./internal/proof/... 2>&1 | Out-String
    if ($LASTEXITCODE -eq 0 -and $proofTestOut -match "PASS") {
        Report-Pass "Proof security validation tests pass (javascript/data URI screening, 2MB size limit)"
    } else {
        Report-Fail "Proof validation tests failed" $proofTestOut
    }
} finally {
    Pop-Location
}

$proofSvc = Get-Content -Raw (Join-Path $RepoRoot "internal\proof\service.go")
if ($proofSvc -match 'url\.ParseRequestURI\(externalLink\)' -and $proofSvc -match 'validMIMEPrefixes') {
    Report-Pass "Proof service validates URL schemes and restricts data URIs to verified MIME types"
} else {
    Report-Fail "Proof URL/MIME validation missing in service.go" ""
}

# -----------------------------------------------------------------------------
# 8. REALTIME WEBSOCKET ORIGIN VERIFICATION & CSWSH DEFENSE
# -----------------------------------------------------------------------------
Write-Host "`n--- 8. Realtime WebSocket Origin Verification & CSWSH Defense ---" -ForegroundColor Yellow

$wsHandler = Get-Content -Raw (Join-Path $RepoRoot "internal\realtime\handler.go")
if ($wsHandler -match 'isAllowedWebSocketOrigin' -and $wsHandler -match 'CheckOrigin:\s*isAllowedWebSocketOrigin') {
    Report-Pass "WebSocket upgrader enforces origin check against allowed origins (prevents CSWSH)"
} else {
    Report-Fail "WebSocket upgrader missing origin validation" ""
}

if ($wsHandler -match 'token\.Method != jwt\.SigningMethodHS256' -and $wsHandler -match 'alg != "HS256"') {
    Report-Pass "WebSocket token validator enforces strict HS256 algorithm allowlist"
} else {
    Report-Fail "WebSocket token validator missing strict algorithm check" ""
}

# -----------------------------------------------------------------------------
# 9. ANDROID SECURITY, CLEARTEXT RESTRICTIONS & ENCRYPTED PREFERENCES
# -----------------------------------------------------------------------------
Write-Host "`n--- 9. Android Security, Cleartext Restrictions & Encrypted Preferences ---" -ForegroundColor Yellow

$netSecConfig = Get-Content -Raw (Join-Path $RepoRoot "android\app\src\main\res\xml\network_security_config.xml")
if ($netSecConfig -match '10\.0\.2\.2' -and $netSecConfig -match 'localhost' -and -not ($netSecConfig -match 'cleartextTrafficPermitted="true".*>\s*\*')) {
    Report-Pass "Android network security config permits cleartext only for local test hosts (production requires HTTPS)"
} else {
    Report-Fail "Android network security config has unsafe cleartext rules" ""
}

$androidManifest = Get-Content -Raw (Join-Path $RepoRoot "android\app\src\main\AndroidManifest.xml")
if ($androidManifest -match 'android:allowBackup="false"' -and $androidManifest -match '(?s)LockActivity.*android:exported="false"') {
    Report-Pass "Android manifest enforces allowBackup=false and protects non-exported activities"
} else {
    Report-Fail "Android manifest security attributes missing" ""
}

$authMgr = Get-Content -Raw (Join-Path $RepoRoot "android\app\src\main\java\com\onusly\app\data\auth\AuthManager.kt")
if ($authMgr -match 'EncryptedSharedPreferences\.create' -and $authMgr -match 'AES256_GCM') {
    Report-Pass "Android AuthManager uses EncryptedSharedPreferences with AES256_GCM"
} else {
    Report-Fail "Android AuthManager missing EncryptedSharedPreferences" ""
}

# -----------------------------------------------------------------------------
# 10. SECRETS & REPOSITORY SANITIZATION
# -----------------------------------------------------------------------------
Write-Host "`n--- 10. Secrets & Repository Sanitization ---" -ForegroundColor Yellow

$gitPrivateKeys = git -C $RepoRoot grep -i "BEGIN RSA PRIVATE KEY\|BEGIN PRIVATE KEY\|BEGIN EC PRIVATE KEY" 2>$null
if (-not $gitPrivateKeys) {
    Report-Pass "Zero private keys found in repository tracking"
} else {
    Report-Fail "Exposed private keys detected in git repository" $gitPrivateKeys
}

Push-Location $RepoRoot
try {
    $configTestOut = go test -v ./internal/config/... 2>&1 | Out-String
    if ($LASTEXITCODE -eq 0 -and $configTestOut -match "PASS") {
        Report-Pass "Config security validation tests pass (rejects weak/short production secrets)"
    } else {
        Report-Fail "Config security tests failed" $configTestOut
    }
} finally {
    Pop-Location
}

# -----------------------------------------------------------------------------
# 11. PRIVACY VERIFICATION
# -----------------------------------------------------------------------------
Write-Host "`n--- 11. Privacy Verification ---" -ForegroundColor Yellow

# Verify zero keylogger hooks
$agentIpcApps = Get-ChildItem -Recurse (Join-Path $RepoRoot "desktop\agent") -Filter "*.go" | Select-String "SetWindowsHookEx|WH_KEYBOARD|GetAsyncKeyState"
if (-not $agentIpcApps) {
    Report-Pass "Zero keyboard logging hooks in desktop agent"
} else {
    Report-Fail "Suspicious keyboard hooks found in desktop agent" $agentIpcApps
}

# Verify zero window title harvesting
$windowTitleScan = Get-ChildItem -Recurse (Join-Path $RepoRoot "desktop\agent\internal\apps") -Filter "*.go" | Select-String "GetWindowText|EnumWindows"
if (-not $windowTitleScan) {
    Report-Pass "Zero window title harvesting in desktop application monitor"
} else {
    Report-Fail "Window title inspection APIs detected" $windowTitleScan
}

# Verify zero screenshot capture
$screenshotScan = Get-ChildItem -Recurse (Join-Path $RepoRoot "desktop\agent") -Filter "*.go" | Select-String "BitBlt|CreateCompatibleDC|GetDC"
if (-not $screenshotScan) {
    Report-Pass "Zero screenshot capture APIs in desktop agent"
} else {
    Report-Fail "Screenshot capture APIs detected in desktop agent" $screenshotScan
}

# Verify Android accessibility service content privacy
$accessXml = Get-Content -Raw (Join-Path $RepoRoot "android\app\src\main\res\xml\accessibility_service_config.xml")
if ($accessXml -match 'android:canRetrieveWindowContent="false"') {
    Report-Pass "Android accessibility service explicitly sets canRetrieveWindowContent=false"
} else {
    Report-Fail "Android accessibility service config allows window content retrieval" ""
}

# Verify zero private DNS query logging
$dnsServer = Get-Content -Raw (Join-Path $RepoRoot "desktop\agent\internal\dns\server.go")
if (-not ($dnsServer -match 'log\.Print.*rawQuery' -or $dnsServer -match 'log\.Print.*clientAddr.*msg')) {
    Report-Pass "DNS server preserves privacy (zero private domain/IP logging to disk/remote)"
} else {
    Report-Fail "DNS server contains private query logging" ""
}

# -----------------------------------------------------------------------------
# SUMMARY
# -----------------------------------------------------------------------------
Write-Host "`n================================================================" -ForegroundColor Cyan
Write-Host "  D13 SECURITY ACCEPTANCE SUMMARY: $PassCount PASSED, $FailCount FAILED  " -ForegroundColor $(if ($FailCount -eq 0) { "Green" } else { "Red" })
Write-Host "================================================================" -ForegroundColor Cyan

if ($FailCount -gt 0) {
    exit 1
}
exit 0
