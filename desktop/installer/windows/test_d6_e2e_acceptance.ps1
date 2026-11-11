# ONUSLY Windows D6 Native Application Policy Control Plane, Goal Authoring & End-to-End Acceptance Suite
# Automated Full Acceptance Gate for D6 Milestone
# Preserves SCM LocalService, Unrestricted SID, Device ID, and Zero Regressions across D0-D5.

[CmdletBinding()]
param(
    [string]$SourceBinDir = "C:\onusly\ONUSLY\build\bin"
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
    Write-Host " ONUSLY D6 END-TO-END APPLICATION CONTROL PLANE ACCEPTANCE SUITE  " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host "Repository: $repoRoot"
    Write-Host "Binaries:   $SourceBinDir"

    # -------------------------------------------------------------
    # 1. Binary Build & PE Validation
    # -------------------------------------------------------------
    Write-TestHeader "1. Production Binary Build & PE Signature"
    New-Item -ItemType Directory -Path $SourceBinDir -Force | Out-Null
    
    $buildAgent = & go build -o "$SourceBinDir\onusly-agent.exe" ./desktop/agent/cmd 2>&1
    Assert-Condition ($LASTEXITCODE -eq 0) "Built onusly-agent.exe successfully"

    $buildHost = & go build -o "$SourceBinDir\onusly-native-host.exe" ./desktop/native-host 2>&1
    Assert-Condition ($LASTEXITCODE -eq 0) "Built onusly-native-host.exe successfully"

    $agentExe = "$SourceBinDir\onusly-agent.exe"
    $hostExe = "$SourceBinDir\onusly-native-host.exe"

    if (Test-Path $agentExe) {
        $bytes = [IO.File]::ReadAllBytes($agentExe)
        $isPE = ($bytes.Length -gt 64 -and $bytes[0] -eq 0x4D -and $bytes[1] -eq 0x5A)
        Assert-Condition $isPE "onusly-agent.exe is valid Windows PE binary (MZ signature)"
    }
    if (Test-Path $hostExe) {
        $bytes = [IO.File]::ReadAllBytes($hostExe)
        $isPE = ($bytes.Length -gt 64 -and $bytes[0] -eq 0x4D -and $bytes[1] -eq 0x5A)
        Assert-Condition $isPE "onusly-native-host.exe is valid Windows PE binary (MZ signature)"
    }

    # -------------------------------------------------------------
    # 2. Backend Goal Validation & Target Control Plane
    # -------------------------------------------------------------
    Write-TestHeader "2. Backend Goal Validation & Target Control Plane Tests"
    $goalTests = & go test -v -run "TestNormalizeAndValidateApps|TestNormalizeAndValidateDomains|TestCatalog|TestNormalizeAndValidateApplicationTargets" ./internal/goals/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Backend goal validation and catalog tests passed"
    Assert-Condition ($goalTests -match "PASS: TestNormalizeAndValidateApps") "NormalizeAndValidateApps accepts desktop app IDs and rejects protected/shell inputs"
    Assert-Condition ($goalTests -match "PASS: TestNormalizeAndValidateDomains") "NormalizeAndValidateDomains preserves domain sanitization rules"
    Assert-Condition ($goalTests -match "PASS: TestNormalizeAndValidateApplicationTargets") "D6.1: ApplicationTargets validates desktop_app vs android_package with strict isolation"

    $enfTests = & go test -v -run "TestEnforcement_VersionRules|TestEnforcement_FilteringAndOrdering|TestEnforcement_TargetTypingSeparation" ./internal/enforcement/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Backend enforcement service tests passed"
    Assert-Condition ($enfTests -match "PASS: TestEnforcement_VersionRules") "Authoritative blocklist version rules enforce active, proof_submitted, rejected, and completed lifecycle"
    Assert-Condition ($enfTests -match "PASS: TestEnforcement_TargetTypingSeparation") "D6.1: Authoritative blocklist generates typed ApplicationTargets and deterministic ETags"

    # -------------------------------------------------------------
    # 3. Frontend Control Plane & Capability Disclosure Tests
    # -------------------------------------------------------------
    Write-TestHeader "3. Frontend Control Plane & Capability Disclosure Tests"
    Push-Location "$repoRoot\frontend"
    try {
        $feTests = & npm test -- --run 2>&1 | Out-String
        Assert-Condition ($LASTEXITCODE -eq 0) "Frontend node:test suite passed successfully"
        Assert-Condition ($feTests -match "D6 Frontend Application & Target Control Plane Suite") "Frontend target control plane suite executed"
        Assert-Condition ($feTests -match "isProtectedProcess strictly identifies OS components") "Frontend protected process immunity verified"
        Assert-Condition ($feTests -match "normalizeDesktopAppTarget trims whitespace") "Frontend target normalization verified"
        Assert-Condition ($feTests -match "WebSocketManager Authentication & Connection State Suite") "Frontend WebSocket manager verified"
    } finally {
        Pop-Location
    }

    # -------------------------------------------------------------
    # 4. Full 12-Step End-to-End Application Policy Lifecycle & D6.1 Cross-Platform Isolation
    # -------------------------------------------------------------
    Write-TestHeader "4. Full 12-Step End-to-End Application Policy Lifecycle & Target Isolation"
    $e2eOut = & go test -v -run "TestD6_EndToEndApplicationPolicyLifecycle|TestAppPolicyManager_TargetTypingAndCrossPlatformIsolation|TestMatcher_CrossPlatformReverseDNSDoesNotMatchWindowsExe" ./desktop/agent/internal/apps/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Automated 12-step end-to-end and target typing integration tests passed"
    Assert-Condition ($e2eOut -match "Step 1 & 2 PASS") "Step 1 & 2: Authoritative policy published and verified"
    Assert-Condition ($e2eOut -match "Step 3 PASS") "Step 3: Desktop agent synchronized authoritative policy snapshot"
    Assert-Condition ($e2eOut -match "Step 4 & 5 PASS") "Step 4 & 5: D5 enforcer detected and terminated restricted process (active)"
    Assert-Condition ($e2eOut -match "Step 6 PASS") "Step 6: Restricted process terminated under proof_submitted status"
    Assert-Condition ($e2eOut -match "Step 7 PASS") "Step 7: Restricted process terminated under rejected status"
    Assert-Condition ($e2eOut -match "Step 8, 9, 10 PASS") "Step 8, 9, 10: Completed goal synchronized -> Application allowed"
    Assert-Condition ($e2eOut -match "Step 11 PASS") "Step 11: Unrelated application remains completely unaffected"
    Assert-Condition ($e2eOut -match "Step 12 PASS") "Step 12: Domain and website targets remain coherent across updates"
    Assert-Condition ($e2eOut -match "PASS: TestAppPolicyManager_TargetTypingAndCrossPlatformIsolation") "D6.1: Desktop policy manager consumes only desktop_app targets and ignores android_package targets"
    Assert-Condition ($e2eOut -match "PASS: TestMatcher_CrossPlatformReverseDNSDoesNotMatchWindowsExe") "D6.1: Reverse-DNS Android identifiers strictly do not match Windows executables"

    # -------------------------------------------------------------
    # 5. SCM Production Service Invariants (LocalService, Unrestricted SID)
    # -------------------------------------------------------------
    Write-TestHeader "5. SCM Production Service Invariants"
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

    # Verify live production service IPC status
    $liveStatusOut = & $agentExe status 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Live agent query status exited with 0"
    Assert-Condition ($liveStatusOut -match "ONUSLY DESKTOP AGENT \(Live Service\)") "Connected to live service over \\.\pipe\OnuslyAgentIPC"

    # -------------------------------------------------------------
    # 6. Linux Static Cross-Compilation Verification
    # -------------------------------------------------------------
    Write-TestHeader "6. Linux Static Cross-Compilation Check"
    $env:GOOS = "linux"
    $linuxVet = & go vet ./desktop/agent/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Linux desktop agent code compiles and passes go vet cleanly"
    $env:GOOS = "windows"

    # -------------------------------------------------------------
    # Acceptance Summary
    # -------------------------------------------------------------
    Write-Host "`n=================================================================" -ForegroundColor Magenta
    Write-Host " D6 ACCEPTANCE TEST RESULTS SUMMARY                              " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host " Total Passed: $global:passCount" -ForegroundColor Green
    Write-Host " Total Failed: $global:failCount" -ForegroundColor $(if ($global:failCount -eq 0) { "Green" } else { "Red" })

    if ($global:failCount -eq 0) {
        Write-Host "`n>>> [SUCCESS] ALL D6 ACCEPTANCE GATES PASSED! <<<\n" -ForegroundColor Green
        exit 0
    } else {
        Write-Host "`n>>> [FAILURE] D6 ACCEPTANCE CRITERIA NOT MET. Check failed assertions above. <<<\n" -ForegroundColor Red
        exit 1
    }

} finally {
    $env:GOOS = "windows"
    Pop-Location
}

