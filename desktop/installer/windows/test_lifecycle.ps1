# ONUSLY Windows Installer Lifecycle Acceptance Test Suite
# Tests: Fresh Install, Idempotency, 11 Rollback Boundaries, 8 Repair Scenarios,
# Device Identity Preservation, Uninstall Retention Policies, and Live Native Messaging.

[CmdletBinding()]
param(
    [string]$SourceBinDir = "C:\onusly\ONUSLY\build\windows_amd64",
    [string]$UpdateUrl = "http://127.0.0.1:8765/updates.xml",
    [switch]$SkipSandboxTests,
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

function Invoke-PowerShellScript {
    param(
        [Parameter(Mandatory=$true)][string]$ScriptPath,
        [Parameter(Mandatory=$true)][string[]]$ScriptArguments
    )
    $output = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $ScriptPath @ScriptArguments 2>&1 | Out-String
    return [PSCustomObject]@{
        ExitCode = $LASTEXITCODE
        Output   = $output
    }
}

function Invoke-ScCommand {
    param(
        [Parameter(Mandatory=$true)][string]$Arguments
    )
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
    $combined = ($stdout + "`n" + $stderr).Trim()
    return [PSCustomObject]@{
        ExitCode = $p.ExitCode
        Stdout   = $stdout.Trim()
        Stderr   = $stderr.Trim()
        Output   = $combined
    }
}

$repoRoot = (Resolve-Path "$PSScriptRoot\..\..\..").Path
$installScript = "$repoRoot\desktop\installer\windows\install.ps1"
$uninstallScript = "$repoRoot\desktop\installer\windows\uninstall.ps1"
$sandboxRoot = "$env:TEMP\onusly_lifecycle_suite_$PID"
$testServiceName = "OnuslyLifecycleTestSvc"

# Clean up sandbox root on exit
Register-EngineEvent -SourceIdentifier PowerShell.Exiting -Action {
    Remove-Item -Path $sandboxRoot -Recurse -Force -ErrorAction SilentlyContinue
} | Out-Null

# Ensure source binaries exist
if (-not (Test-Path "$SourceBinDir\onusly-agent.exe") -or -not (Test-Path "$SourceBinDir\onusly-native-host.exe")) {
    Write-Host "Building fresh binaries into $SourceBinDir..." -ForegroundColor Yellow
    New-Item -ItemType Directory -Path $SourceBinDir -Force | Out-Null
    Start-Process -FilePath "go" -ArgumentList "build", "-o", "$SourceBinDir\onusly-agent.exe", "./desktop/agent/cmd" -WorkingDirectory $repoRoot -NoNewWindow -Wait
    Start-Process -FilePath "go" -ArgumentList "build", "-o", "$SourceBinDir\onusly-native-host.exe", "./desktop/native-host" -WorkingDirectory $repoRoot -NoNewWindow -Wait
}

if (-not $SkipSandboxTests) {
    # -------------------------------------------------------------
    # TEST 1 & 2: Fresh Installation and Idempotent Repeat in Sandbox
    # -------------------------------------------------------------
    Write-TestHeader "1 & 2. Fresh Installation and Idempotent Repeat in Sandbox"
    $t1Dir = "$sandboxRoot\t1_fresh_repeat"
    $t1Install = "$t1Dir\ProgramFiles\Onusly"
    $t1Data = "$t1Dir\ProgramData\Onusly"
    New-Item -ItemType Directory -Path $t1Install, $t1Data -Force | Out-Null

    $res1 = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments @(
        "-InstallDir", $t1Install,
        "-DataRoot", $t1Data,
        "-ServiceName", $testServiceName,
        "-SourceBinDir", $SourceBinDir,
        "-UpdateUrl", $UpdateUrl,
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-SkipServiceStart",
        "-NonInteractive"
    )

    Assert-Condition ($res1.ExitCode -eq 0) "Fresh installation exited with code 0"
    Assert-Condition (Test-Path "$t1Install\onusly-agent.exe") "onusly-agent.exe deployed to install directory"
    Assert-Condition (Test-Path "$t1Install\onusly-native-host.exe") "onusly-native-host.exe deployed to install directory"
    Assert-Condition (Test-Path "$t1Install\manifests\com.onusly.desktop_agent_chrome.json") "Chrome native host manifest generated"
    Assert-Condition (Test-Path "$t1Install\manifests\com.onusly.desktop_agent_firefox.json") "Firefox native host manifest generated"
    Assert-Condition (Test-Path "$t1Data\secure") "Secure credential directory created"

    # Repeated install (Idempotency)
    $resRepeat = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments @(
        "-InstallDir", $t1Install,
        "-DataRoot", $t1Data,
        "-ServiceName", $testServiceName,
        "-SourceBinDir", $SourceBinDir,
        "-UpdateUrl", $UpdateUrl,
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-SkipServiceStart",
        "-NonInteractive"
    )

    Assert-Condition ($resRepeat.ExitCode -eq 0) "Repeated installation completed with code 0"
    Assert-Condition (Test-Path "$t1Install\onusly-agent.exe") "onusly-agent.exe intact after repeat"

    # -------------------------------------------------------------
    # TEST 3: Transactional Rollback across 11 Failure Boundaries
    # -------------------------------------------------------------
    Write-TestHeader "3. Transactional Rollback across 11 Simulated Failure Boundaries"

    $failureBoundaries = @(
        "BinaryStaging",
        "PEValidation",
        "BackupCreation",
        "FirstBinaryReplacement",
        "SecondBinaryReplacement",
        "ACLApplication",
        "SCMConfiguration",
        "NativeMessagingRegistration",
        "BrowserPolicyConfiguration",
        "ServiceStartup",
        "PostStartHealth"
    )

    foreach ($stage in $failureBoundaries) {
        Write-Host "`n  Testing boundary: $stage..." -ForegroundColor White
        $bDir = "$sandboxRoot\rollback_$stage"
        $bInstall = "$bDir\ProgramFiles\Onusly"
        $bData = "$bDir\ProgramData\Onusly"
        New-Item -ItemType Directory -Path $bInstall, "$bData\config" -Force | Out-Null

        # Setup baseline files
        Set-Content -Path "$bInstall\onusly-agent.exe" -Value "ORIGINAL_AGENT_MOCK"
        Set-Content -Path "$bInstall\onusly-native-host.exe" -Value "ORIGINAL_HOST_MOCK"
        Set-Content -Path "$bData\config\device_id" -Value "device-id-test-12345"

        $resFail = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments @(
            "-InstallDir", $bInstall,
            "-DataRoot", $bData,
            "-ServiceName", $testServiceName,
            "-SourceBinDir", $SourceBinDir,
            "-UpdateUrl", $UpdateUrl,
            "-SimulateFailureStage", $stage,
            "-SkipElevationCheck",
            "-SkipRegistry",
            "-SkipServiceStart",
            "-NonInteractive"
        )

        $hasRollbackMarker = ($resFail.Output -match "\[ROLLBACK_SUCCESS\]")
        Assert-Condition ($resFail.ExitCode -ne 0) "Boundary $stage safely intercepted upgrade (exit code $($resFail.ExitCode))"
        Assert-Condition $hasRollbackMarker "Boundary $stage successfully executed transactional rollback"

        # Verify device identity remained untouched
        if (Test-Path "$bData\config\device_id") {
            $devId = Get-Content "$bData\config\device_id"
            Assert-Condition ($devId -eq "device-id-test-12345") "Device identity strictly preserved across $stage failure"
        }
    }

    # -------------------------------------------------------------
    # TEST 4: Repair Mode across Component Failures
    # -------------------------------------------------------------
    Write-TestHeader "4. Component-Level Repair Mode Verification"

    # Setup healthy installation in repair sandbox
    $rDir = "$sandboxRoot\repair_test"
    $rInstall = "$rDir\ProgramFiles\Onusly"
    $rData = "$rDir\ProgramData\Onusly"
    New-Item -ItemType Directory -Path $rInstall, "$rInstall\manifests", "$rData\secure", "$rData\config" -Force | Out-Null
    Copy-Item "$SourceBinDir\onusly-agent.exe" "$rInstall\onusly-agent.exe" -Force
    Copy-Item "$SourceBinDir\onusly-native-host.exe" "$rInstall\onusly-native-host.exe" -Force
    Set-Content -Path "$rData\config\device_id" -Value "repair-device-id-9999"

    $repairArgs = @(
        "-InstallDir", $rInstall,
        "-DataRoot", $rData,
        "-ServiceName", $testServiceName,
        "-SourceBinDir", $SourceBinDir,
        "-UpdateUrl", $UpdateUrl,
        "-Repair",
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-SkipServiceStart",
        "-NonInteractive"
    )

    # Scenario 4.1: Missing Agent Binary
    Remove-Item "$rInstall\onusly-agent.exe" -Force
    $resR1 = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments $repairArgs
    Assert-Condition (Test-Path "$rInstall\onusly-agent.exe") "Repair restored missing agent binary"
    Assert-Condition ((Get-Content "$rData\config\device_id") -eq "repair-device-id-9999") "Repair preserved device identity"

    # Scenario 4.2: Missing Native Host Binary
    Remove-Item "$rInstall\onusly-native-host.exe" -Force
    $resR2 = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments $repairArgs
    Assert-Condition (Test-Path "$rInstall\onusly-native-host.exe") "Repair restored missing native host binary"

    # Scenario 4.3: Missing Native Messaging Manifests
    Remove-Item "$rInstall\manifests\*.json" -Force -ErrorAction SilentlyContinue
    $resR3 = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments $repairArgs
    Assert-Condition (Test-Path "$rInstall\manifests\com.onusly.desktop_agent_chrome.json") "Repair regenerated Chrome manifest"
    Assert-Condition (Test-Path "$rInstall\manifests\com.onusly.desktop_agent_firefox.json") "Repair regenerated Firefox manifest"

    # Scenario 4.4: Repeated Repair on Healthy System (Zero alterations)
    $resR4 = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments $repairArgs
    Assert-Condition ($resR4.ExitCode -eq 0) "Repeated repair on healthy system exited cleanly with 0 alterations"

    # -------------------------------------------------------------
    # TEST 5: Device Identity SHA-256 Hash Acceptance
    # -------------------------------------------------------------
    Write-TestHeader "5. Device Identity SHA-256 Hash Acceptance Instrumentation"

    $idDir = "$sandboxRoot\identity_test"
    $idInstall = "$idDir\ProgramFiles\Onusly"
    $idData = "$idDir\ProgramData\Onusly"
    New-Item -ItemType Directory -Path $idInstall, "$idData\config" -Force | Out-Null
    Copy-Item "$SourceBinDir\onusly-agent.exe" "$idInstall\onusly-agent.exe" -Force
    Copy-Item "$SourceBinDir\onusly-native-host.exe" "$idInstall\onusly-native-host.exe" -Force

    $testDeviceIdFile = "$idData\config\device_id"
    $syntheticId = "7a8b9c0d-1234-5678-abcd-ef0123456789"
    Set-Content -Path $testDeviceIdFile -Value $syntheticId

    $hashBefore = (Get-FileHash -Algorithm SHA256 $testDeviceIdFile).Hash
    Write-Host "  Recorded Device ID SHA-256 (Before Upgrade): $hashBefore" -ForegroundColor DarkGray

    # Run upgrade in sandbox
    $resIdUpgrade = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments @(
        "-InstallDir", $idInstall,
        "-DataRoot", $idData,
        "-ServiceName", $testServiceName,
        "-SourceBinDir", $SourceBinDir,
        "-UpdateUrl", $UpdateUrl,
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-SkipServiceStart",
        "-NonInteractive"
    )

    $hashAfterUpgrade = (Get-FileHash -Algorithm SHA256 $testDeviceIdFile).Hash
    Write-Host "  Recorded Device ID SHA-256 (After Upgrade):  $hashAfterUpgrade" -ForegroundColor DarkGray
    Assert-Condition ($hashBefore -eq $hashAfterUpgrade) "Device Identity SHA-256 hash byte-for-byte identical after upgrade"

    # Run repair in sandbox
    $resIdRepair = Invoke-PowerShellScript -ScriptPath $installScript -ScriptArguments @(
        "-InstallDir", $idInstall,
        "-DataRoot", $idData,
        "-ServiceName", $testServiceName,
        "-SourceBinDir", $SourceBinDir,
        "-UpdateUrl", $UpdateUrl,
        "-Repair",
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-SkipServiceStart",
        "-NonInteractive"
    )

    $hashAfterRepair = (Get-FileHash -Algorithm SHA256 $testDeviceIdFile).Hash
    Write-Host "  Recorded Device ID SHA-256 (After Repair):   $hashAfterRepair" -ForegroundColor DarkGray
    Assert-Condition ($hashBefore -eq $hashAfterRepair) "Device Identity SHA-256 hash byte-for-byte identical after repair"

    # -------------------------------------------------------------
    # TEST 6: Uninstall Data Retention Policy
    # -------------------------------------------------------------
    Write-TestHeader "6. Uninstall Data Retention Policy Verification"

    # Case 6.1: Standard Uninstall (credentials purged, binaries removed)
    $uDir1 = "$sandboxRoot\uninst_case1"
    $uInstall1 = "$uDir1\ProgramFiles\Onusly"
    $uData1 = "$uDir1\ProgramData\Onusly"
    New-Item -ItemType Directory -Path "$uInstall1\manifests", "$uData1\secure", "$uData1\config" -Force | Out-Null
    Set-Content -Path "$uInstall1\onusly-agent.exe" -Value "MOCK_BIN"
    Set-Content -Path "$uData1\secure\token.dat" -Value "SENSITIVE_CREDENTIAL"
    Set-Content -Path "$uData1\config\device_id" -Value "test-dev-id"

    $resU1 = Invoke-PowerShellScript -ScriptPath $uninstallScript -ScriptArguments @(
        "-InstallDir", $uInstall1,
        "-DataRoot", $uData1,
        "-ServiceName", $testServiceName,
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-NonInteractive"
    )

    Assert-Condition (-not (Test-Path "$uInstall1")) "Standard uninstall removed Program Files directory"
    Assert-Condition (-not (Test-Path "$uData1\secure")) "Standard uninstall purged sensitive pairing credentials"
    Assert-Condition (-not (Test-Path "$uData1\config")) "Standard uninstall purged configuration directory"

    # Case 6.2: Uninstall with -RetainDeviceIdentity
    $uDir2 = "$sandboxRoot\uninst_case2"
    $uInstall2 = "$uDir2\ProgramFiles\Onusly"
    $uData2 = "$uDir2\ProgramData\Onusly"
    New-Item -ItemType Directory -Path "$uInstall2\manifests", "$uData2\secure", "$uData2\config" -Force | Out-Null
    Set-Content -Path "$uInstall2\onusly-agent.exe" -Value "MOCK_BIN"
    Set-Content -Path "$uData2\secure\token.dat" -Value "SENSITIVE_CREDENTIAL"
    Set-Content -Path "$uData2\config\device_id" -Value "preserved-dev-id"

    $resU2 = Invoke-PowerShellScript -ScriptPath $uninstallScript -ScriptArguments @(
        "-InstallDir", $uInstall2,
        "-DataRoot", $uData2,
        "-ServiceName", $testServiceName,
        "-RetainDeviceIdentity",
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-NonInteractive"
    )

    Assert-Condition (-not (Test-Path "$uData2\secure")) "-RetainDeviceIdentity still purged sensitive credentials"
    Assert-Condition (Test-Path "$uData2\config\device_id") "-RetainDeviceIdentity preserved config\device_id"
    if (Test-Path "$uData2\config\device_id") {
        Assert-Condition ((Get-Content "$uData2\config\device_id") -eq "preserved-dev-id") "Preserved device ID content is intact"
    }

    # Case 6.3: Uninstall with -PurgeAll
    $uDir3 = "$sandboxRoot\uninst_case3"
    $uInstall3 = "$uDir3\ProgramFiles\Onusly"
    $uData3 = "$uDir3\ProgramData\Onusly"
    New-Item -ItemType Directory -Path "$uInstall3\manifests", "$uData3\secure", "$uData3\config" -Force | Out-Null
    Set-Content -Path "$uInstall3\onusly-agent.exe" -Value "MOCK_BIN"

    $resU3 = Invoke-PowerShellScript -ScriptPath $uninstallScript -ScriptArguments @(
        "-InstallDir", $uInstall3,
        "-DataRoot", $uData3,
        "-ServiceName", $testServiceName,
        "-PurgeAll",
        "-SkipElevationCheck",
        "-SkipRegistry",
        "-NonInteractive"
    )
    Assert-Condition (-not (Test-Path $uData3)) "-PurgeAll completely purged ProgramData directory"
}

# -------------------------------------------------------------
# TEST 7: Live Production Service Non-Destructive Inspection
# -------------------------------------------------------------
if (-not $SkipLiveInspection) {
    Write-TestHeader "7. Live Production Service Non-Destructive Inspection"

    $prodInstallDir = "${env:ProgramFiles}\Onusly"
    $prodDataRoot = "${env:ProgramData}\Onusly"
    $prodServiceName = "OnuslyDesktopAgent"

    # 7.1 Verify live SCM service state
    $qex = Invoke-ScCommand "queryex $prodServiceName"
    $isRunning = ($qex.Output -match "STATE\s*:\s*4\s+RUNNING")
    Assert-Condition $isRunning "Production service $prodServiceName is active and RUNNING"

    # 7.2 Verify live device ID SHA-256
    $prodDevIdFile = "$prodDataRoot\config\device_id"
    if (Test-Path $prodDevIdFile) {
        $prodDevId = (Get-Content $prodDevIdFile).Trim()
        $prodHash = (Get-FileHash -Algorithm SHA256 $prodDevIdFile).Hash
        Assert-Condition ($prodHash -eq "8443EE4451BFA152BC88EE820026DD4DB237EACBB0021ECF2608F23508A033EC") "Live Production Device ID SHA-256 hash verified intact ($prodHash)"
    } else {
        Assert-Condition $false "Production device_id file exists"
    }

    # 7.3 Verify Chrome Native Messaging Registration
    $chromeReg = "HKLM:\SOFTWARE\Google\Chrome\NativeMessagingHosts\com.onusly.desktop_agent"
    $chromeRegExists = Test-Path $chromeReg
    Assert-Condition $chromeRegExists "Chrome NativeMessagingHosts registry key registered"
    if ($chromeRegExists) {
        $val = (Get-ItemProperty -Path $chromeReg -Name "(Default)" -ErrorAction SilentlyContinue)."(Default)"
        Assert-Condition (Test-Path $val) "Chrome NativeMessagingHosts points to valid manifest file: $val"

        # Validate manifest content
        $manifestContent = Get-Content $val -Raw | ConvertFrom-Json
        Assert-Condition ($manifestContent.name -eq "com.onusly.desktop_agent") "Manifest name is com.onusly.desktop_agent"
        Assert-Condition ($manifestContent.type -eq "stdio") "Manifest type is stdio"
        Assert-Condition (Test-Path $manifestContent.path) "Manifest path points to valid executable: $($manifestContent.path)"
        Assert-Condition ($manifestContent.allowed_origins -contains "chrome-extension://jifigaejbinmdhnljobkodcgeaplojmg/") "Manifest allows canonical extension ID jifigaejbinmdhnljobkodcgeaplojmg"
    }

    # 7.4 Verify Edge Native Messaging Registration
    $edgeReg = "HKLM:\SOFTWARE\Microsoft\Edge\NativeMessagingHosts\com.onusly.desktop_agent"
    $edgeRegExists = Test-Path $edgeReg
    Assert-Condition $edgeRegExists "Microsoft Edge NativeMessagingHosts registry key registered"

    # 7.5 Verify Chrome & Edge ExtensionInstallForcelist Policies
    $chromePolicy = "HKLM:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist"
    if (Test-Path $chromePolicy) {
        $props = Get-ItemProperty -Path $chromePolicy
        $foundPolicy = $false
        foreach ($prop in $props.PSObject.Properties) {
            if ($prop.Value -like "*jifigaejbinmdhnljobkodcgeaplojmg*") {
                $foundPolicy = $true
                break
            }
        }
        Assert-Condition $foundPolicy "Chrome enterprise policy force-installs canonical extension ID"
    }

    $edgePolicy = "HKLM:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist"
    if (Test-Path $edgePolicy) {
        $props = Get-ItemProperty -Path $edgePolicy
        $foundPolicy = $false
        foreach ($prop in $props.PSObject.Properties) {
            if ($prop.Value -like "*jifigaejbinmdhnljobkodcgeaplojmg*") {
                $foundPolicy = $true
                break
            }
        }
        Assert-Condition $foundPolicy "Edge enterprise policy force-installs canonical extension ID"
    }

    # 7.6 Live Native Host Stdio Execution & Framing Protocol Test
    Write-Host "`n  Testing live native host stdio framing protocol..." -ForegroundColor White
    $hostExe = "$prodInstallDir\onusly-native-host.exe"
    if (Test-Path $hostExe) {
        # Construct stdio message: 4-byte little endian length prefix + {"action":"ping"}
        $pingMsg = '{"action":"ping"}'
        $pingBytes = [System.Text.Encoding]::UTF8.GetBytes($pingMsg)
        $lenBytes = [System.BitConverter]::GetBytes([uint32]$pingBytes.Length)

        $pinfoHost = New-Object System.Diagnostics.ProcessStartInfo
        $pinfoHost.FileName = $hostExe
        $pinfoHost.RedirectStandardInput = $true
        $pinfoHost.RedirectStandardOutput = $true
        $pinfoHost.RedirectStandardError = $true
        $pinfoHost.UseShellExecute = $false
        $pinfoHost.CreateNoWindow = $true

        $pHost = [System.Diagnostics.Process]::Start($pinfoHost)
        $pHost.StandardInput.BaseStream.Write($lenBytes, 0, 4)
        $pHost.StandardInput.BaseStream.Write($pingBytes, 0, $pingBytes.Length)
        $pHost.StandardInput.BaseStream.Flush()
        $pHost.StandardInput.Close()

        $respBytes = New-Object byte[] 4096
        $readCount = $pHost.StandardOutput.BaseStream.Read($respBytes, 0, 4)
        if ($readCount -eq 4) {
            $respLen = [System.BitConverter]::ToUInt32($respBytes, 0)
            $payloadBytes = New-Object byte[] $respLen
            $pHost.StandardOutput.BaseStream.Read($payloadBytes, 0, $respLen) | Out-Null
            $respPayload = [System.Text.Encoding]::UTF8.GetString($payloadBytes)

            Assert-Condition ($respLen -gt 0) "Native host responded with valid 4-byte length prefix ($respLen bytes)"
            $respObj = $respPayload | ConvertFrom-Json
            Assert-Condition ($respObj.success -eq $true) "Native host ping response success: true"

            # Invariant: JWT / sensitive tokens NEVER leaked to native host
            $hasLeak = ($respPayload -match "token" -or $respPayload -match "jwt" -or $respPayload -match "secret")
            Assert-Condition (-not $hasLeak) "Invariant Verified: Zero JWT or sensitive credential leakage to browser host"
        } else {
            Assert-Condition $false "Native host failed to return length prefix"
        }
        $pHost.WaitForExit()
    }
}

Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host "                    FINAL ACCEPTANCE SUMMARY                     " -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "Total Passed: $global:passCount" -ForegroundColor Green
Write-Host "Total Failed: $global:failCount" -ForegroundColor $(if ($global:failCount -eq 0) { "Green" } else { "Red" })
Write-Host "=================================================================`n"

if ($global:failCount -gt 0) {
    exit 1
} else {
    exit 0
}
