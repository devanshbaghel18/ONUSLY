# ONUSLY Windows Native Desktop Software Enforcement & Process Control Acceptance Test
# Automated Validation Gate for D5 Milestone
# Invariant: Never terminates system processes; enforces PID reuse safety; truthful capability tiering.

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
    Write-Host " ONUSLY D5 WINDOWS NATIVE APP ENFORCEMENT ACCEPTANCE SUITE       " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host "Repository: $repoRoot"
    Write-Host "Binaries:   $SourceBinDir"

    # -------------------------------------------------------------
    # 1. Build and Binary Validation
    # -------------------------------------------------------------
    Write-TestHeader "1. Binary Compilation & Header Integrity"
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
    # 2. Go Apps Package Unit and Acceptance Tests
    # -------------------------------------------------------------
    Write-TestHeader "2. Apps Package Unit & Lifecycle Tests"
    $appsTests = & go test -v ./desktop/agent/internal/apps/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "All internal/apps unit and integration tests passed"
    Assert-Condition ($appsTests -match "PASS: TestAppEnforcementController_LifecycleAndEnforcement") "Controller lifecycle, monitor, and reconciliation verified"
    Assert-Condition ($appsTests -match "PASS: TestWindowsInventoryScanner_LiveScan") "Windows registry inventory scanner verified on host"
    Assert-Condition ($appsTests -match "PASS: TestIsProtectedSystemProcess") "Protected system processes immunity list verified"
    Assert-Condition ($appsTests -match "PASS: TestMatchProcess_PrecedenceAndEvasion") "Process matching precedence (exact_path -> dir_binary -> basename) verified"
    Assert-Condition ($appsTests -match "PASS: TestWindowsProcessMonitor_LiveSnapshot") "Process snapshot inspection via OpenProcess/QueryFullProcessImageName verified"
    Assert-Condition ($appsTests -match "PASS: TestWindowsProcessMonitor_MockLifecycleEvents") "Process start/exit event diffing verified"
    Assert-Condition ($appsTests -match "PASS: TestAppPolicyManager_GoalStatusContract") "D5 Goal status contract (active/proof/rejected -> Block, completed -> Allow) verified"
    Assert-Condition ($appsTests -match "PASS: TestImmunityProtection") "Strict immunity: refusing to enforce against explorer, agent, self, or PID <= 4"
    Assert-Condition ($appsTests -match "PASS: TestLiveProcessEnforcementAndPIDReuse") "Live process termination and TOCTOU PID reuse race protection verified"

    # -------------------------------------------------------------
    # 3. CLI Apps Subcommand Verification
    # -------------------------------------------------------------
    Write-TestHeader "3. CLI Application Subcommands"
    
    # 3a. apps inventory
    $invOut = & "$agentExe" apps inventory 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "CLI 'onusly-agent apps inventory' executed successfully"
    Assert-Condition ($invOut -match "APP ID\s+DISPLAY NAME") "Application inventory prints structured table header"
    Assert-Condition ($invOut -match "Total Discovered Applications:\s+[1-9]") "Discovered installed third-party applications on host"

    # 3b. apps refresh
    $refreshOut = & "$agentExe" apps refresh 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "CLI 'onusly-agent apps refresh' executed successfully"
    Assert-Condition ($refreshOut -match "\[OK\] Discovered \d+ installed applications on host") "Inventory refresh successfully scanned registry keys"

    # -------------------------------------------------------------
    # 4. Standalone Agent Runtime with App Enforcement & IPC
    # -------------------------------------------------------------
    Write-TestHeader "4. Standalone Agent with App Enforcement & IPC"
    $tempTestRoot = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "onusly-apps-test-$PID")
    $tempConfig = "$tempTestRoot\config"
    $tempData = "$tempTestRoot\data"
    New-Item -ItemType Directory -Path $tempConfig -Force | Out-Null
    New-Item -ItemType Directory -Path $tempData -Force | Out-Null

    $testPipe = "\\.\pipe\OnuslyAppsAccPipe_$PID"
    $testDnsAddr = "127.0.0.1:53"

    Write-Host "Launching standalone test agent on pipe $testPipe..."
    $proc = Start-Process -FilePath $agentExe -ArgumentList "run", "-dns-listen", $testDnsAddr, "-socket", $testPipe, "-config-dir", $tempConfig, "-data-dir", $tempData -NoNewWindow -PassThru

    try {
        # Wait up to 5s for pipe to become available
        $pipeReady = $false
        for ($i = 0; $i -lt 15; $i++) {
            Start-Sleep -Milliseconds 300
            $probeStatus = & $agentExe apps status -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
            if ($probeStatus -match "Subsystem Active:\s*YES") {
                $pipeReady = $true
                break
            }
        }
        Assert-Condition $pipeReady "Standalone agent started and initialized App Enforcement Controller"

        # Check apps status via CLI
        $appsStatus = & $agentExe apps status -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
        Assert-Condition ($appsStatus -match "Subsystem Active:\s*YES \(ACTIVE\)") "apps status reports Subsystem Active = YES"
        Assert-Condition ($appsStatus -match "Capability Tier:\s*Tier 1 - Detection & Response") "apps status reports Capability Tier = Tier 1 - Detection & Response"
        Assert-Condition ($appsStatus -match "Monitored Processes:\s*[1-9]") "apps status reports active Monitored Processes > 0"

        # Check diagnostics metrics
        $diagOut = & $agentExe apps diagnostics -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
        Assert-Condition ($diagOut -match "apps_controller_running\s+true") "apps diagnostics reports apps_controller_running = true"
        Assert-Condition ($diagOut -match "apps_capability_tier\s+Tier 1 - Detection & Response") "apps diagnostics reports apps_capability_tier"
        Assert-Condition ($diagOut -match "apps_inventory_count\s+[1-9]") "apps diagnostics reports apps_inventory_count > 0"

        # -------------------------------------------------------------
        # 5. Live Application Detection and Termination Lifecycle
        # -------------------------------------------------------------
        Write-TestHeader "5. Live Application Enforcement Lifecycle"
        
        # Compile a harmless dummy test application
        $dummySrc = "$tempTestRoot\blocked_app.go"
        $dummyExe = "$tempTestRoot\blocked_app.exe"
        @'
package main
import "time"
func main() {
    time.Sleep(30 * time.Second)
}
'@ | Set-Content -Path $dummySrc -Encoding UTF8

        & go build -o $dummyExe $dummySrc 2>&1 | Out-Null
        Assert-Condition (Test-Path $dummyExe) "Compiled dummy test application blocked_app.exe"

        # Populate a test blocklist restricting blocked_app
        $blocklistFile = "$tempData\blocklist_cache.json"
        $blocklistJson = @"
{
  "userId": "",
  "version": "apps-test-v1",
  "generatedAt": "$(Get-Date -Format o)",
  "lastSyncAt": "$(Get-Date -Format o)",
  "targets": [
    {
      "goalId": "goal-apps-test-1",
      "goalTitle": "Focus Block Apps",
      "status": "active",
      "apps": ["blocked_app"],
      "domains": []
    }
  ]
}
"@
        Set-Content -Path $blocklistFile -Value $blocklistJson -Encoding UTF8
        Assert-Condition (Test-Path $blocklistFile) "Staged active goal restriction for 'blocked_app'"

        # Restart standalone agent with the populated blocklist cache
        Write-Host "Reloading standalone agent with active restriction..."
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 600

        $proc = Start-Process -FilePath $agentExe -ArgumentList "run", "-dns-listen", $testDnsAddr, "-socket", $testPipe, "-config-dir", $tempConfig, "-data-dir", $tempData -NoNewWindow -PassThru
        
        for ($i = 0; $i -lt 15; $i++) {
            Start-Sleep -Milliseconds 300
            $probeStatus = & $agentExe apps status -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
            if ($probeStatus -match "Restricted Targets:\s*1") {
                break
            }
        }

        $reloadedStatus = & $agentExe apps status -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
        Assert-Condition ($reloadedStatus -match "Restricted Targets:\s*1") "Agent loaded active restriction for blocked_app"

        # Now launch the restricted dummy app
        Write-Host "Launching restricted process $dummyExe..."
        $dummyProc = Start-Process -FilePath $dummyExe -PassThru
        $dummyPid = $dummyProc.Id
        Write-Host "Launched dummy process with PID: $dummyPid"

        # Wait for agent controller reconciliation loop to detect and terminate it
        $terminated = $false
        for ($i = 0; $i -lt 10; $i++) {
            Start-Sleep -Milliseconds 600
            $checkProc = Get-Process -Id $dummyPid -ErrorAction SilentlyContinue
            if (-not $checkProc -or $checkProc.HasExited) {
                $terminated = $true
                break
            }
        }
        Assert-Condition $terminated "Controller automatically detected and terminated restricted process (PID: $dummyPid)"

        # Verify enforcement counter incremented in status
        $afterStatus = & $agentExe apps status -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
        Assert-Condition ($afterStatus -match "Enforced Terminated:\s*[1-9]") "Controller incremented Enforced Terminated counter"
        Assert-Condition ($afterStatus -match "Enforcement Failed:\s*0") "Enforcement Failed remains 0 (clean termination)"

    } finally {
        if ($proc -and -not $proc.HasExited) {
            Write-Host "Stopping standalone test agent process $($proc.Id)..."
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
            Start-Sleep -Milliseconds 500
        }
        Remove-Item -Path $tempTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }

    # -------------------------------------------------------------
    # 6. SCM Service Invariants (LocalService, Unrestricted SID)
    # -------------------------------------------------------------
    Write-TestHeader "6. SCM Production Service Invariants"
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
    # 7. Zero-Regression Cross-Subsystem Verification
    # -------------------------------------------------------------
    Write-TestHeader "7. Zero-Regression Verification (DNS, Browser Policy, Native Host, IPC)"
    
    $dnsCheck = & go test ./desktop/agent/internal/dns/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "D4 DNS subsystem tests passed without regression"

    $policyCheck = & go test ./desktop/policy/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "D3 Browser policy subsystem tests passed without regression"

    $hostCheck = & go test ./desktop/native-host/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "D3 Native messaging host tests passed without regression"

    $ipcCheck = & go test ./desktop/agent/internal/ipc/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Desktop IPC server tests passed without regression"

    # -------------------------------------------------------------
    # Acceptance Summary
    # -------------------------------------------------------------
    Write-Host "`n=================================================================" -ForegroundColor Magenta
    Write-Host " D5 ACCEPTANCE TEST RESULTS SUMMARY                              " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host " Total Passed: $global:passCount" -ForegroundColor Green
    Write-Host " Total Failed: $global:failCount" -ForegroundColor $(if ($global:failCount -eq 0) { "Green" } else { "Red" })

    if ($global:failCount -eq 0) {
        Write-Host "`n>>> [SUCCESS] ALL D5 ACCEPTANCE GATES PASSED! <<<\n" -ForegroundColor Green
        exit 0
    } else {
        Write-Host "`n>>> [FAILURE] D5 ACCEPTANCE CRITERIA NOT MET. Check failed assertions above. <<<\n" -ForegroundColor Red
        exit 1
    }

} finally {
    Pop-Location
}
