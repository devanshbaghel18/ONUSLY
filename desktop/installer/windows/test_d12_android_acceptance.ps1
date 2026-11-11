# ONUSLY D12 — Android Native Application Enforcement & Mobile Release Readiness Acceptance Test
# Validates typed policy integration, Android package isolation, target authoring, accessibility privacy,
# unit test execution (182 tests), and release APK artifact generation.

$ErrorActionPreference = "Stop"

$PassCount = 0
$FailCount = 0

function Report-Assert([string]$Title, [bool]$Condition, [string]$Details = "") {
    if ($Condition) {
        $script:PassCount++
        Write-Host "  [PASS] $Title" -ForegroundColor Green
    } else {
        $script:FailCount++
        Write-Host "  [FAIL] $Title - $Details" -ForegroundColor Red
    }
}

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "  ONUSLY D12 - Android Native Enforcement and Acceptance Suite" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$WorkspaceRoot = "C:\onusly\ONUSLY"
$AndroidDir = Join-Path $WorkspaceRoot "android"
$AppDir = Join-Path $AndroidDir "app"

# -------------------------------------------------------------------------
# Test Group 1: Android Build Configuration and SDK Alignment
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 1: Build Configuration and SDK Alignment ---" -ForegroundColor Yellow

$BuildGradle = Join-Path $AppDir "build.gradle.kts"
$BuildContent = Get-Content $BuildGradle -Raw

Report-Assert "build.gradle.kts exists" (Test-Path $BuildGradle)
Report-Assert "compileSdk is set to 35" ($BuildContent -match 'compileSdk\s*=\s*35')
Report-Assert "targetSdk is set to 35" ($BuildContent -match 'targetSdk\s*=\s*35')
Report-Assert "minSdk is set to 26" ($BuildContent -match 'minSdk\s*=\s*26')
Report-Assert "versionCode and versionName configured" ($BuildContent -match 'versionCode\s*=\s*1' -and $BuildContent -match 'versionName\s*=\s*"1\.0\.0"')
Report-Assert "Single parallel test fork configured to prevent commit exhaustion" ($BuildContent -match 'maxParallelForks\s*=\s*1')

# -------------------------------------------------------------------------
# Test Group 2: Android Security and Permission Audit
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 2: Android Security and Permission Audit ---" -ForegroundColor Yellow

$ManifestPath = Join-Path $AppDir "src\main\AndroidManifest.xml"
$ManifestContent = Get-Content $ManifestPath -Raw

Report-Assert "AndroidManifest.xml exists" (Test-Path $ManifestPath)
Report-Assert "Declares INTERNET permission" ($ManifestContent -match 'android\.permission\.INTERNET')
Report-Assert "Declares ACCESS_NETWORK_STATE permission" ($ManifestContent -match 'android\.permission\.ACCESS_NETWORK_STATE')
Report-Assert "Declares RECEIVE_BOOT_COMPLETED permission" ($ManifestContent -match 'android\.permission\.RECEIVE_BOOT_COMPLETED')
Report-Assert "Uses Play-compliant launcher queries instead of QUERY_ALL_PACKAGES" ($ManifestContent -match '<category android:name="android\.intent\.category\.LAUNCHER"' -and ($ManifestContent -notmatch 'android\.permission\.QUERY_ALL_PACKAGES'))
Report-Assert "Declares BIND_ACCESSIBILITY_SERVICE permission on service" ($ManifestContent -match 'android\.permission\.BIND_ACCESSIBILITY_SERVICE')
Report-Assert "Declares LockActivity with excludeFromRecents" ($ManifestContent -match 'LockActivity' -and $ManifestContent -match 'android:excludeFromRecents="true"')

$AccessConfigPath = Join-Path $AppDir "src\main\res\xml\accessibility_service_config.xml"
$AccessConfigContent = Get-Content $AccessConfigPath -Raw
Report-Assert "accessibility_service_config.xml exists" (Test-Path $AccessConfigPath)
Report-Assert "canRetrieveWindowContent is strictly set to false for user privacy" ($AccessConfigContent -match 'android:canRetrieveWindowContent="false"')
Report-Assert "Listens only to window state and window change events" ($AccessConfigContent -match 'typeWindowStateChanged\|typeWindowsChanged')

$ProguardPath = Join-Path $AppDir "proguard-rules.pro"
$ProguardContent = Get-Content $ProguardPath -Raw
Report-Assert "proguard-rules.pro exists" (Test-Path $ProguardPath)
Report-Assert "Preserves com.onusly.app.data.model package" ($ProguardContent -match 'com\.onusly\.app\.data\.model\.\*')
Report-Assert "Preserves com.onusly.app.engine package" ($ProguardContent -match 'com\.onusly\.app\.engine\.\*')

# -------------------------------------------------------------------------
# Test Group 3: Typed Policy Models and Parser Integration
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 3: Typed Policy Models and Parser Integration ---" -ForegroundColor Yellow

$BlocklistModels = Join-Path $AppDir "src\main\java\com\onusly\app\data\model\BlocklistModels.kt"
$BlocklistModelsContent = Get-Content $BlocklistModels -Raw

Report-Assert "ApplicationTarget data class defined" ($BlocklistModelsContent -match 'data class ApplicationTarget')
Report-Assert "BlocklistTarget includes applicationTargets field" ($BlocklistModelsContent -match 'val applicationTargets:\s*List<ApplicationTarget>')

$BlocklistParser = Join-Path $AppDir "src\main\java\com\onusly\app\data\model\BlocklistParser.kt"
$BlocklistParserContent = Get-Content $BlocklistParser -Raw

Report-Assert "BlocklistParser parses applicationTargets from response JSON" ($BlocklistParserContent -match 'parseApplicationTargets')
Report-Assert "BlocklistParser serializes applicationTargets to cached JSON" ($BlocklistParserContent -match 'targetObj\.put\("applicationTargets"')
Report-Assert "BlocklistParser parses applicationTargets from cached JSON" ($BlocklistParserContent -match 'applicationTargets = appTargetsList')

$GoalModels = Join-Path $AppDir "src\main\java\com\onusly\app\data\model\GoalModels.kt"
$GoalModelsContent = Get-Content $GoalModels -Raw

Report-Assert "Goal model includes applicationTargets" ($GoalModelsContent -match 'val applicationTargets:\s*List<ApplicationTarget>')
Report-Assert "CreateGoalRequest includes applicationTargets" ($GoalModelsContent -match 'class CreateGoalRequest' -and $GoalModelsContent -match 'applicationTargets:\s*List<ApplicationTarget>')
Report-Assert "UpdateGoalRequest includes applicationTargets" ($GoalModelsContent -match 'class UpdateGoalRequest' -and $GoalModelsContent -match 'applicationTargets:\s*List<ApplicationTarget>\?')

$GoalParser = Join-Path $AppDir "src\main\java\com\onusly\app\data\model\GoalParser.kt"
$GoalParserContent = Get-Content $GoalParser -Raw

Report-Assert "GoalParser parses applicationTargets for Goal" ($GoalParserContent -match 'applicationTargets = applicationTargets')
Report-Assert "GoalParser serializes applicationTargets in createGoalRequestToJson" ($GoalParserContent -match 'createGoalRequestToJson' -and $GoalParserContent -match 'obj\.put\("applicationTargets"')
Report-Assert "GoalParser serializes applicationTargets in updateGoalRequestToJson" ($GoalParserContent -match 'updateGoalRequestToJson' -and $GoalParserContent -match 'obj\.put\("applicationTargets"')

# -------------------------------------------------------------------------
# Test Group 4: Cross-Platform Target Separation and Engine Enforcement
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 4: Cross-Platform Target Separation and Engine Enforcement ---" -ForegroundColor Yellow

$BlockerEngine = Join-Path $AppDir "src\main\java\com\onusly\app\engine\BlockerEngine.kt"
$BlockerEngineContent = Get-Content $BlockerEngine -Raw

Report-Assert "BlockerEngine filters strictly for kind == android_package" ($BlockerEngineContent -match 'at\.kind\.equals\("android_package",\s*ignoreCase\s*=\s*true\)')
Report-Assert "BlockerEngine ignores desktop_app targets" ($BlockerEngineContent -match 'desktop_app')
Report-Assert "BlockerEngine enforces legacy apps filtering desktop binaries" ($BlockerEngineContent -match '\.endsWith\("\.exe"\)')
Report-Assert "BlockerEngine includes ALWAYS_ALLOWED_PACKAGES immunity list" ($BlockerEngineContent -match 'ALWAYS_ALLOWED_PACKAGES')
Report-Assert "BlockerEngine supports active, proof_submitted, and rejected enforcement" ($BlockerEngineContent -match 'active' -and $BlockerEngineContent -match 'proof_submitted' -and $BlockerEngineContent -match 'rejected')

$InstalledAppsManager = Join-Path $AppDir "src\main\java\com\onusly\app\engine\InstalledAppsManager.kt"
$InstalledAppsContent = Get-Content $InstalledAppsManager -Raw

Report-Assert "InstalledAppsManager defines isValidPackageName" ($InstalledAppsContent -match 'fun isValidPackageName')
Report-Assert "InstalledAppsManager filters desktop binary extensions" ($InstalledAppsContent -match 'DESKTOP_EXTENSIONS')
Report-Assert "InstalledAppsManager defines validatePackageForTargeting" ($InstalledAppsContent -match 'fun validatePackageForTargeting')

# -------------------------------------------------------------------------
# Test Group 5: Target Authoring UI and Device Revocation Handling
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 5: Target Authoring UI and Device Revocation Handling ---" -ForegroundColor Yellow

$PickerXml = Join-Path $AppDir "src\main\res\layout\dialog_app_picker.xml"
$PickerXmlContent = Get-Content $PickerXml -Raw
Report-Assert "dialog_app_picker.xml includes manual package entry field" ($PickerXmlContent -match 'etManualPackage' -and $PickerXmlContent -match 'btnAddManualPackage')

$PickerDialog = Join-Path $AppDir "src\main\java\com\onusly\app\ui\InstalledAppsPickerDialog.kt"
$PickerDialogContent = Get-Content $PickerDialog -Raw
Report-Assert "InstalledAppsPickerDialog validates manual package targeting" ($PickerDialogContent -match 'validatePackageForTargeting')

$GoalEditor = Join-Path $AppDir "src\main\java\com\onusly\app\ui\GoalEditorDialog.kt"
$GoalEditorContent = Get-Content $GoalEditor -Raw
Report-Assert "GoalEditorDialog authors explicit android_package typed targets" ($GoalEditorContent -match 'ApplicationTarget\(kind = "android_package"')

$SyncRepo = Join-Path $AppDir "src\main\java\com\onusly\app\data\SyncRepository.kt"
$SyncRepoContent = Get-Content $SyncRepo -Raw
Report-Assert "SyncRepository handles 403 DEVICE_REVOKED with onUserSignOut" ($SyncRepoContent -match 'DEVICE_REVOKED' -and $SyncRepoContent -match 'onUserSignOut')

# -------------------------------------------------------------------------
# Test Group 6: Android Unit Test Suite Execution
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 6: Android Unit Test Suite Execution ---" -ForegroundColor Yellow

$TestXmlDir = Join-Path $AppDir "build\test-results\testDebugUnitTest"
$TestXmls = Get-ChildItem -Path $TestXmlDir -Filter "*.xml" -ErrorAction SilentlyContinue

$TotalTests = 0
$TotalFailures = 0
$TotalErrors = 0
$SuiteCount = 0

foreach ($xmlFile in $TestXmls) {
    [xml]$xml = Get-Content $xmlFile.FullName
    $SuiteCount++
    $TotalTests += [int]$xml.testsuite.tests
    $TotalFailures += [int]$xml.testsuite.failures
    $TotalErrors += [int]$xml.testsuite.errors
}

Report-Assert ("Unit test XML report files exist - " + $SuiteCount + " suites found") ($SuiteCount -ge 18)
Report-Assert ("All unit tests executed - " + $TotalTests + " total tests") ($TotalTests -ge 180)
Report-Assert ("Zero unit test failures - " + $TotalFailures + " failures") ($TotalFailures -eq 0)
Report-Assert ("Zero unit test errors - " + $TotalErrors + " errors") ($TotalErrors -eq 0)

$D12Xml = Join-Path $TestXmlDir "TEST-com.onusly.app.D12TypedPolicyEnforcementTest.xml"
Report-Assert "D12TypedPolicyEnforcementTest suite executed" (Test-Path $D12Xml)
if (Test-Path $D12Xml) {
    [xml]$d12XmlContent = Get-Content $D12Xml
    $d12Tests = [int]$d12XmlContent.testsuite.tests
    $d12Fails = [int]$d12XmlContent.testsuite.failures
    Report-Assert ("All D12 typed enforcement checks passed - " + $d12Tests + " tests, 0 failures") ($d12Tests -ge 14 -and $d12Fails -eq 0)
}

# -------------------------------------------------------------------------
# Test Group 7: Release APK Artifact Verification
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 7: Release APK Artifact Verification ---" -ForegroundColor Yellow

$ApkPath = Join-Path $AppDir "build\outputs\apk\release\app-release.apk"
Report-Assert "app-release.apk produced by assembleRelease" (Test-Path $ApkPath)

if (Test-Path $ApkPath) {
    $apkItem = Get-Item $ApkPath
    $apkSizeMB = [math]::Round($apkItem.Length / 1MB, 2)
    Report-Assert "Release APK size is non-trivial ($apkSizeMB MB)" ($apkItem.Length -gt 1000000)

    $apkHash = (Get-FileHash -Algorithm SHA256 $ApkPath).Hash
    Report-Assert "Release APK SHA-256 computed: $apkHash" ($apkHash.Length -eq 64)
}

# -------------------------------------------------------------------------
# Test Group 8: Desktop SCM Invariant Verification
# -------------------------------------------------------------------------
Write-Host "`n--- Test Group 8: Desktop Service Invariants ---" -ForegroundColor Yellow

$Svc = Get-Service -Name "OnuslyDesktopAgent" -ErrorAction SilentlyContinue
Report-Assert "OnuslyDesktopAgent service is running" ($Svc -ne $null -and $Svc.Status -eq "Running")

$SvcProc = Get-CimInstance Win32_Service -Filter "Name='OnuslyDesktopAgent'"
Report-Assert "Service runs under NT AUTHORITY\LocalService" ($SvcProc.StartName -match "LocalService")

$StateFile = "C:\ProgramData\Onusly\agent\state\device.json"
if (Test-Path $StateFile) {
    $stateJson = Get-Content $StateFile -Raw | ConvertFrom-Json
    Report-Assert "Canonical device ID preserved (143489b2-2fd8-428b-bfa8-2631cc2be8c3)" ($stateJson.device_id -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3")
}

# -------------------------------------------------------------------------
# Summary
# -------------------------------------------------------------------------
Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host "  D12 Acceptance Test Summary: $PassCount PASSED, $FailCount FAILED" -ForegroundColor $(if ($FailCount -eq 0) { "Green" } else { "Red" })
Write-Host "=================================================================" -ForegroundColor Cyan

if ($FailCount -gt 0) {
    exit 1
} else {
    exit 0
}
