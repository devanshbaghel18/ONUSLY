# ONUSLY Windows Secure Software Update Protocol, Artifact Verification & Atomic Upgrade Acceptance Suite
# Automated Validation Gate for D9 Milestone
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
    Write-Host " ONUSLY D9 SECURE UPDATE & ARTIFACT VERIFICATION ACCEPTANCE      " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host "Repository: $repoRoot"

    # -------------------------------------------------------------
    # 1. Cryptographic Signature & Manifest Verification Suite
    # -------------------------------------------------------------
    Write-TestHeader "1. Cryptographic Signature & Manifest Verification Suite"
    $updateGoOut = & go test -v ./desktop/core/update/... 2>&1 | Out-String

    Assert-Condition ($updateGoOut -match "PASS: TestUpdateVerifier_Comprehensive12PointChecks") "12-point manifest & artifact security checks passed"
    Assert-Condition ($updateGoOut -match "PASS: TestStagedUpdateDownloader_MockServerAndFailureModes") "Mock update server, tamper rejection & truncated download recovery passed"
    Assert-Condition ($updateGoOut -match "PASS: TestSemVer_ParsingAndComparison") "SemVer 2.0 version comparison and pre-release ordering passed"
    Assert-Condition ($updateGoOut -match "PASS: TestAntiRollbackManager_Lifecycle") "Anti-rollback monotonic security version and SemVer downgrade prevention passed"

    # -------------------------------------------------------------
    # 2. CLI Update Command & Tooling Verification
    # -------------------------------------------------------------
    Write-TestHeader "2. CLI Update Verification & Anti-Rollback Tooling"
    $tempTestDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "onusly_d9_cli_" + [System.Guid]::NewGuid().ToString("N"))
    [System.IO.Directory]::CreateDirectory($tempTestDir) | Out-Null
    $stagingDir = Join-Path $tempTestDir "staging"
    $configDir = Join-Path $tempTestDir "config"
    [System.IO.Directory]::CreateDirectory($stagingDir) | Out-Null
    [System.IO.Directory]::CreateDirectory($configDir) | Out-Null

    try {
        # Generate disposable Ed25519 keypair for automated test execution
        $keyGenGo = @"
package main
import (
    "crypto/ed25519"
    "crypto/rand"
    "encoding/hex"
    "fmt"
)
func main() {
    pub, priv, _ := ed25519.GenerateKey(rand.Reader)
    fmt.Printf("%s\n%s\n", hex.EncodeToString(pub), hex.EncodeToString(priv))
}
"@
        $keyScriptPath = Join-Path $tempTestDir "keygen.go"
        [System.IO.File]::WriteAllText($keyScriptPath, $keyGenGo)
        $keyLines = (& go run $keyScriptPath) -split "`r?`n"
        $pubHex = $keyLines[0].Trim()
        $privHex = $keyLines[1].Trim()

        # Set environment variable for test execution
        $env:ONUSLY_UPDATE_PUBLIC_KEY = $pubHex

        # Create dummy artifacts
        $agentArtPath = Join-Path $stagingDir "onusly-agent-windows-amd64.exe"
        $hostArtPath = Join-Path $stagingDir "onusly-native-host-windows-amd64.exe"
        [System.IO.File]::WriteAllText($agentArtPath, "dummy agent update v1.1.0")
        [System.IO.File]::WriteAllText($hostArtPath, "dummy host update v1.1.0")

        $stagingDirFwd = $stagingDir.Replace('\', '/')
        $manifestPathFwd = (Join-Path $stagingDir "manifest.json").Replace('\', '/')

        # Create manifest builder script using our update package
        $manifestBuilderGo = @"
package main
import (
    "crypto/ed25519"
    "encoding/hex"
    "encoding/json"
    "os"
    "time"
    "github.com/devanshbaghel18/ONUSLY/desktop/core/update"
)
func main() {
    privBytes, _ := hex.DecodeString("$privHex")
    priv := ed25519.PrivateKey(privBytes)
    manifest, err := update.BuildAndSignManifest(
        "$stagingDirFwd",
        "1.1.0",
        2,
        "stable",
        "windows",
        "amd64",
        24*time.Hour,
        priv,
    )
    if err != nil {
        panic(err)
    }
    data, _ := json.MarshalIndent(manifest, "", "  ")
    _ = os.WriteFile("$manifestPathFwd", data, 0644)
}
"@
        $builderScriptPath = Join-Path $tempTestDir "build_manifest.go"
        [System.IO.File]::WriteAllText($builderScriptPath, $manifestBuilderGo)
        & go run $builderScriptPath
        Assert-Condition (Test-Path (Join-Path $stagingDir "manifest.json")) "Signed manifest generated with disposable test key"

        # Verify CLI verify command on valid manifest
        $verifyCmd = & go run ./desktop/agent/cmd update verify (Join-Path $stagingDir "manifest.json") $stagingDir 2>&1 | Out-String
        Assert-Condition ($verifyCmd -match "MANIFEST CRYPTOGRAPHICALLY VERIFIED") "CLI update verify: valid manifest and artifact digests verified"

        # Test tamper rejection: alter artifact
        [System.IO.File]::WriteAllText($agentArtPath, "TAMPERED PAYLOAD")
        $tamperCmd = & go run ./desktop/agent/cmd update verify (Join-Path $stagingDir "manifest.json") $stagingDir 2>&1 | Out-String
        Assert-Condition ($tamperCmd -match "artifact verification failed") "CLI update verify: tampered artifact rejected"

        # Restore valid artifact
        [System.IO.File]::WriteAllText($agentArtPath, "dummy agent update v1.1.0")

        # Test downgrade prevention: create a 1.0.0-rc0 manifest (older than 1.0.0-rc1)
        $downgradeManifestPathFwd = (Join-Path $stagingDir "manifest_downgrade.json").Replace('\', '/')
        $downgradeBuilderGo = @"
package main
import (
    "crypto/ed25519"
    "encoding/hex"
    "encoding/json"
    "os"
    "time"
    "github.com/devanshbaghel18/ONUSLY/desktop/core/update"
)
func main() {
    privBytes, _ := hex.DecodeString("$privHex")
    priv := ed25519.PrivateKey(privBytes)
    manifest, _ := update.BuildAndSignManifest(
        "$stagingDirFwd",
        "1.0.0-rc0",
        1,
        "stable",
        "windows",
        "amd64",
        24*time.Hour,
        priv,
    )
    data, _ := json.MarshalIndent(manifest, "", "  ")
    _ = os.WriteFile("$downgradeManifestPathFwd", data, 0644)
}
"@
        $downgradeScriptPath = Join-Path $tempTestDir "build_downgrade.go"
        [System.IO.File]::WriteAllText($downgradeScriptPath, $downgradeBuilderGo)
        & go run $downgradeScriptPath

        # Seed anti-rollback state with 1.0.0-rc1
        $antiRollbackFile = Join-Path $configDir "anti_rollback.json"
        $antiRollbackInit = @"
{
  "highest_release_version": "1.0.0-rc1",
  "highest_security_version": 2,
  "last_accepted_at": "2026-11-11T12:00:00Z"
}
"@
        [System.IO.File]::WriteAllText($antiRollbackFile, $antiRollbackInit)

        # Verify downgrade rejection
        $downgradeCmd = & go run ./desktop/agent/cmd --config-dir=$configDir update verify (Join-Path $stagingDir "manifest_downgrade.json") $stagingDir 2>&1 | Out-String
        Assert-Condition ($downgradeCmd -match "rollback prevented") "CLI update verify: downgrade attempt rejected by anti-rollback"

        # Verify authorized downgrade with --allow-downgrade
        $allowDowngradeCmd = & go run ./desktop/agent/cmd --config-dir=$configDir update verify (Join-Path $stagingDir "manifest_downgrade.json") $stagingDir --allow-downgrade 2>&1 | Out-String
        Assert-Condition ($allowDowngradeCmd -match "MANIFEST CRYPTOGRAPHICALLY VERIFIED") "CLI update verify: --allow-downgrade permits authorized recovery"

    } finally {
        $env:ONUSLY_UPDATE_PUBLIC_KEY = ""
        if (Test-Path $tempTestDir) {
            Remove-Item -Recurse -Force $tempTestDir -ErrorAction SilentlyContinue
        }
    }

    # -------------------------------------------------------------
    # 3. Installer Elevation Boundary Verification
    # -------------------------------------------------------------
    Write-TestHeader "3. Installer Elevation Boundary & Staged Update Integration"
    $sandboxRoot = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "onusly_d9_install_" + [System.Guid]::NewGuid().ToString("N"))
    [System.IO.Directory]::CreateDirectory($sandboxRoot) | Out-Null
    $sandboxInstallDir = Join-Path $sandboxRoot "ProgramFiles\Onusly"
    $sandboxDataRoot = Join-Path $sandboxRoot "ProgramData\Onusly"
    $sandboxStagingDir = Join-Path $sandboxRoot "staging"
    [System.IO.Directory]::CreateDirectory($sandboxInstallDir) | Out-Null
    [System.IO.Directory]::CreateDirectory($sandboxDataRoot) | Out-Null
    [System.IO.Directory]::CreateDirectory($sandboxStagingDir) | Out-Null

    try {
        # Stage valid binaries into sandbox staging
        Copy-Item "$repoRoot\build\bin\onusly-agent.exe" (Join-Path $sandboxStagingDir "onusly-agent-windows-amd64.exe")
        Copy-Item "$repoRoot\build\bin\onusly-native-host.exe" (Join-Path $sandboxStagingDir "onusly-native-host-windows-amd64.exe")

        # Re-sign manifest for real build binaries using test key
        $keyScriptPath = Join-Path $sandboxRoot "keygen.go"
        [System.IO.File]::WriteAllText($keyScriptPath, $keyGenGo)
        $keyLines = (& go run $keyScriptPath) -split "`r?`n"
        $pubHex = $keyLines[0].Trim()
        $privHex = $keyLines[1].Trim()
        $env:ONUSLY_UPDATE_PUBLIC_KEY = $pubHex

        $sandboxStagingDirFwd = $sandboxStagingDir.Replace('\', '/')
        $realManifestPathFwd = (Join-Path $sandboxStagingDir "manifest.json").Replace('\', '/')

        $realBuilderGo = @"
package main
import (
    "crypto/ed25519"
    "encoding/hex"
    "encoding/json"
    "os"
    "time"
    "github.com/devanshbaghel18/ONUSLY/desktop/core/update"
)
func main() {
    privBytes, _ := hex.DecodeString("$privHex")
    priv := ed25519.PrivateKey(privBytes)
    manifest, _ := update.BuildAndSignManifest(
        "$sandboxStagingDirFwd",
        "1.0.0-rc2",
        2,
        "stable",
        "windows",
        "amd64",
        24*time.Hour,
        priv,
    )
    data, _ := json.MarshalIndent(manifest, "", "  ")
    _ = os.WriteFile("$realManifestPathFwd", data, 0644)
}
"@
        $realBuilderScriptPath = Join-Path $sandboxRoot "build_real.go"
        [System.IO.File]::WriteAllText($realBuilderScriptPath, $realBuilderGo)
        & go run $realBuilderScriptPath

        # Seed device identity and mock DNS backup in sandbox
        $cfgDir = Join-Path $sandboxDataRoot "config"
        [System.IO.Directory]::CreateDirectory($cfgDir) | Out-Null
        $mockDevId = "d9-test-device-uuid-12345"
        [System.IO.File]::WriteAllText((Join-Path $cfgDir "device_id"), $mockDevId)
        $mockBackup = @{ IsActive = $false; OriginalDns = @() } | ConvertTo-Json
        [System.IO.File]::WriteAllText((Join-Path $cfgDir "dns_backup.json"), $mockBackup)

        # Run install.ps1 with -StagedDir in sandbox mode
        $installOut = & powershell -ExecutionPolicy Bypass -File "$repoRoot\desktop\installer\windows\install.ps1" `
            -InstallDir $sandboxInstallDir `
            -DataRoot $sandboxDataRoot `
            -StagedDir $sandboxStagingDir `
            -SkipElevationCheck `
            -SkipRegistry `
            -SkipServiceStart `
            -NonInteractive 2>&1 | Out-String

        Assert-Condition ($installOut -match "Staged update manifest and artifact digests verified successfully") "Installer verified staged update at elevation boundary"
        Assert-Condition (Test-Path (Join-Path $sandboxInstallDir "onusly-agent.exe")) "Installer deployed agent binary from staged update"
        Assert-Condition (Test-Path (Join-Path $sandboxInstallDir "onusly-native-host.exe")) "Installer deployed native host binary from staged update"

        # Verify device identity strictly preserved
        $preservedId = (Get-Content (Join-Path $cfgDir "device_id")).Trim()
        Assert-Condition ($preservedId -eq $mockDevId) "Device identity preserved across staged installer upgrade"

        # Verify DNS backup strictly preserved
        Assert-Condition (Test-Path (Join-Path $cfgDir "dns_backup.json")) "DNS backup file preserved across staged installer upgrade"

    } finally {
        $env:ONUSLY_UPDATE_PUBLIC_KEY = ""
        if (Test-Path $sandboxRoot) {
            Remove-Item -Recurse -Force $sandboxRoot -ErrorAction SilentlyContinue
        }
    }

    # -------------------------------------------------------------
    # 4. Browser Extension Packaging & Integrity
    # -------------------------------------------------------------
    Write-TestHeader "4. Browser Extension Distribution & Packaging Verification"
    $updatesXmlPath = "$repoRoot\frontend\public\downloads\updates.xml"
    $chromeZipPath = "$repoRoot\frontend\public\downloads\onusly-extension.zip"
    $firefoxZipPath = "$repoRoot\frontend\public\downloads\onusly-firefox.zip"

    Assert-Condition (Test-Path $updatesXmlPath) "Enterprise Chrome updates.xml exists"
    if (Test-Path $updatesXmlPath) {
        $xmlContent = Get-Content $updatesXmlPath -Raw
        Assert-Condition ($xmlContent -match "protocol='2.0'") "Update manifest specifies protocol='2.0'"
        Assert-Condition ($xmlContent -match "appid='jifigaejbinmdhnljobkodcgeaplojmg'") "Update manifest binds canonical extension ID"
        Assert-Condition ($xmlContent -match "version='1.0.0'") "Update manifest specifies valid release version"
    }

    Assert-Condition (Test-Path $chromeZipPath) "Packaged Chrome extension zip exists"
    Assert-Condition (Test-Path $firefoxZipPath) "Packaged Firefox extension zip exists"

    # -------------------------------------------------------------
    # 5. Live Production SCM Service Health Invariant
    # -------------------------------------------------------------
    Write-TestHeader "5. Production Windows Service Identity & SCM Health Invariant"
    if (-not $SkipLiveInspection) {
        $scQ = Invoke-ScCommand "query OnuslyDesktopAgent"
        $scQc = Invoke-ScCommand "qc OnuslyDesktopAgent"

        Assert-Condition ($scQ.Output -match "STATE\s+:\s+4\s+RUNNING") "Live OnuslyDesktopAgent service is RUNNING"
        Assert-Condition ($scQc.Output -match "SERVICE_START_NAME\s+:\s+NT AUTHORITY\\LocalService") "Execution identity is NT AUTHORITY\LocalService"
        
        $cfgDir = "C:\ProgramData\Onusly\config"
        $prodDevId = if (Test-Path "$cfgDir\device_id") { (Get-Content "$cfgDir\device_id").Trim() } else { "" }
        Assert-Condition ($prodDevId -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3") "Canonical device ID strictly maintained: 143489b2-2fd8-428b-bfa8-2631cc2be8c3"
    }

    Write-Host "`n=================================================================" -ForegroundColor Magenta
    Write-Host " ONUSLY D9 ACCEPTANCE SUITE SUMMARY                              " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    $failColor = if ($global:failCount -eq 0) { "Green" } else { "Red" }
    Write-Host "  Passed: $global:passCount" -ForegroundColor Green
    Write-Host "  Failed: $global:failCount" -ForegroundColor $failColor

    if ($global:failCount -gt 0) {
        Write-Error "D9 Acceptance Suite FAILED ($global:failCount failures)"
        exit 1
    } else {
        Write-Host "`n>>> ONUSLY D9 SECURE UPDATE ACCEPTANCE: 100% PASS <<<`n" -ForegroundColor Green
    }
} finally {
    Pop-Location
}
