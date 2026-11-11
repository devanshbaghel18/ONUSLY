# ONUSLY Windows Production Installer
# D3.3 Milestone: Safe Idempotent Service Upgrade, Strict ACLs & Enterprise Browser Policies

[CmdletBinding()]
param(
    [string]$ApiUrl = "https://api.onusly.com",
    [string]$UpdateUrl = "https://api.onusly.com/downloads/updates.xml",
    [string]$SourceBinDir = "",
    [string]$InstallDir = "${env:ProgramFiles}\Onusly",
    [string]$DataRoot = "${env:ProgramData}\Onusly",
    [string]$ServiceName = "OnuslyDesktopAgent",
    [string]$ServiceSid = "S-1-5-80-3463423020-2351545225-1748562058-3133284821-4013307172",
    [switch]$Repair,
    [string]$SimulateFailureStage = "",
    [switch]$SkipElevationCheck,
    [switch]$SkipRegistry,
    [switch]$NonInteractive,
    [switch]$SkipServiceStart
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host "`n>>> [ONUSLY Installer] $Message" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "  [OK] $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "  [WARN] $Message" -ForegroundColor Yellow
}

function Write-Fail {
    param([string]$Message)
    Write-Host "  [ERROR] $Message" -ForegroundColor Red
    exit 1
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

function Test-ValidPE {
    param([Parameter(Mandatory=$true)][string]$Path)
    if (-not (Test-Path $Path)) {
        return $false
    }
    try {
        $stream = [System.IO.File]::OpenRead($Path)
        try {
            $reader = New-Object System.IO.BinaryReader($stream)
            if ($stream.Length -lt 64) { return $false }
            $dos = $reader.ReadBytes(64)
            if ($dos[0] -ne 0x4D -or $dos[1] -ne 0x5A) { return $false } # 'MZ'
            $peOffset = [System.BitConverter]::ToUInt32($dos, 0x3C)
            if ($peOffset -lt 64 -or $peOffset -gt ($stream.Length - 24)) { return $false }
            $stream.Position = $peOffset
            $peSig = $reader.ReadBytes(4)
            if ($peSig[0] -ne 0x50 -or $peSig[1] -ne 0x45 -or $peSig[2] -ne 0 -or $peSig[3] -ne 0) { return $false } # 'PE\0\0'
            $machine = $reader.ReadUInt16()
            return ($machine -eq 0x8664) # 0x8664 = IMAGE_FILE_MACHINE_AMD64
        } finally {
            $stream.Close()
            $stream.Dispose()
        }
    } catch {
        return $false
    }
}

function Test-FileUnlocked {
    param([Parameter(Mandatory=$true)][string]$Path)
    if (-not (Test-Path $Path)) { return $true }
    try {
        $fileStream = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
        if ($fileStream) {
            $fileStream.Close()
            $fileStream.Dispose()
            return $true
        }
    } catch {
        return $false
    }
    return $false
}

function Wait-FileLockRelease {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [int]$TimeoutSeconds = 5
    )
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        if (Test-FileUnlocked -Path $Path) {
            return $true
        }
        Start-Sleep -Milliseconds 150
    }
    return (Test-FileUnlocked -Path $Path)
}

function Wait-ServiceState {
    param(
        [Parameter(Mandatory=$true)][string]$TargetServiceName,
        [Parameter(Mandatory=$true)][string]$TargetState, # "RUNNING" or "STOPPED"
        [int]$TimeoutSeconds = 10
    )
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        $qex = Invoke-ScCommand "queryex $TargetServiceName"
        if ($TargetState -eq "STOPPED") {
            if ($qex.ExitCode -eq 1060 -or $qex.Output -match "STATE\s*:\s*1\s+STOPPED") {
                return $true
            }
        } elseif ($TargetState -eq "RUNNING") {
            if ($qex.Output -match "STATE\s*:\s*4\s+RUNNING") {
                return $true
            }
        }
        Start-Sleep -Milliseconds 200
    }
    $finalQex = Invoke-ScCommand "queryex $TargetServiceName"
    if ($TargetState -eq "STOPPED") {
        return ($finalQex.ExitCode -eq 1060 -or $finalQex.Output -match "STATE\s*:\s*1\s+STOPPED")
    } elseif ($TargetState -eq "RUNNING") {
        return ($finalQex.Output -match "STATE\s*:\s*4\s+RUNNING")
    }
    return $false
}

# 1. Preflight Checks
Write-Step "Performing Preflight Checks..."

if (-not $SkipElevationCheck) {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) {
        Write-Fail "Administrator elevation is required to install the ONUSLY Windows service and enterprise browser policies. Please run from an elevated PowerShell session."
    }
    Write-Success "Elevated Administrator session verified."
} else {
    Write-Warn "Skipping elevation check (-SkipElevationCheck specified for testing)."
}

# Validate Update URL format
if ([string]::IsNullOrWhiteSpace($UpdateUrl)) {
    Write-Fail "Update URL cannot be empty. Please provide a valid update manifest or package URL via -UpdateUrl."
}
try {
    $uri = [System.Uri]::new($UpdateUrl)
    if ($uri.Scheme -ne "http" -and $uri.Scheme -ne "https") {
        Write-Fail "Invalid Update URL scheme '$($uri.Scheme)'. Only http and https are permitted."
    }
    if ([string]::IsNullOrWhiteSpace($uri.Host)) {
        Write-Fail "Invalid Update URL: missing host name."
    }
} catch {
    Write-Fail "Failed to parse Update URL '$UpdateUrl': $_"
}
Write-Success "Extension Update URL format verified: $UpdateUrl"

# 2. Locate or Build Production Binaries
Write-Step "Resolving Production Binaries..."

$repoRoot = (Resolve-Path "$PSScriptRoot\..\..\..").Path
$agentExe = ""
$nativeHostExe = ""

if (-not [string]::IsNullOrWhiteSpace($SourceBinDir) -and (Test-Path "$SourceBinDir\onusly-agent.exe") -and (Test-Path "$SourceBinDir\onusly-native-host.exe")) {
    $agentExe = "$SourceBinDir\onusly-agent.exe"
    $nativeHostExe = "$SourceBinDir\onusly-native-host.exe"
} else {
    # Build directly from repository
    Write-Host "  Building onusly-agent.exe from repository..."
    $buildAgent = Start-Process -FilePath "go" -ArgumentList "build", "-o", "$env:TEMP\onusly-agent.exe", "./desktop/agent/cmd" -WorkingDirectory $repoRoot -NoNewWindow -Wait -PassThru
    if ($buildAgent.ExitCode -ne 0) {
        Write-Fail "Failed to build onusly-agent.exe"
    }

    Write-Host "  Building onusly-native-host.exe from repository..."
    $buildHost = Start-Process -FilePath "go" -ArgumentList "build", "-o", "$env:TEMP\onusly-native-host.exe", "./desktop/native-host" -WorkingDirectory $repoRoot -NoNewWindow -Wait -PassThru
    if ($buildHost.ExitCode -ne 0) {
        Write-Fail "Failed to build onusly-native-host.exe"
    }

    $agentExe = "$env:TEMP\onusly-agent.exe"
    $nativeHostExe = "$env:TEMP\onusly-native-host.exe"
}

if (-not (Test-ValidPE -Path $agentExe)) {
    Write-Fail "Source onusly-agent.exe is not a valid 64-bit Windows PE executable ($agentExe)."
}
if (-not (Test-ValidPE -Path $nativeHostExe)) {
    Write-Fail "Source onusly-native-host.exe is not a valid 64-bit Windows PE executable ($nativeHostExe)."
}
Write-Success "Production binaries resolved and validated: $agentExe, $nativeHostExe"

# 3. Target Installation Directories and Installation Mode
$manifestDir = "$InstallDir\manifests"
$stagingDir = "$InstallDir\.staging"
$binBackupDir = "$InstallDir\.backup"
$configDir = "$DataRoot\config"
$storageDir = "$DataRoot\data"
$logsDir = "$DataRoot\logs"
$secureDir = "$DataRoot\secure"
$backupDir = "$DataRoot\policy-backups"

$existingAgentBin = "$InstallDir\onusly-agent.exe"
$existingHostBin = "$InstallDir\onusly-native-host.exe"

Write-Step "Detecting Existing Installation and Service State..."
$serviceExists = $false
$serviceWasRunning = $false
$servicePID = 0

$qex = Invoke-ScCommand "queryex $ServiceName"
if ($qex.ExitCode -eq 0) {
    $serviceExists = $true
    if ($qex.Output -match "STATE\s*:\s*4\s+RUNNING") {
        $serviceWasRunning = $true
    }
    if ($qex.Output -match "PID\s*:\s*(\d+)") {
        $servicePID = [int]$Matches[1]
    }
}

$binariesExist = (Test-Path $existingAgentBin) -or (Test-Path $existingHostBin)

if ($Repair) {
    $installMode = "Repair"
} elseif (-not $serviceExists -and -not $binariesExist) {
    $installMode = "FreshInstall"
} elseif ($serviceWasRunning) {
    $installMode = "UpgradeRunningService"
} else {
    $installMode = "UpgradeStoppedService"
}

if ($SimulateFailureStage -and $installMode -eq "Repair") {
    $installMode = "UpgradeStoppedService"
}
Write-Success "Installation Mode: $installMode (serviceExists=$serviceExists, serviceWasRunning=$serviceWasRunning, PID=$servicePID)"

# -------------------------------------------------------------
# REPAIR MODE: Surgical component-level verification & repair
# -------------------------------------------------------------
if ($installMode -eq "Repair") {
    Write-Step "Executing Component-Level Repair Mode (Preserving Identity & Pairing State)..."
    $repairsMade = @()

    # Preflight check on directories
    New-Item -ItemType Directory -Path $InstallDir, $manifestDir, $configDir, $storageDir, $logsDir, $secureDir, $backupDir -Force | Out-Null

    # 1. Verify and repair agent binary
    if (-not (Test-Path $existingAgentBin) -or -not (Test-ValidPE -Path $existingAgentBin)) {
        Write-Host "  [REPAIR] Agent binary missing or invalid. Restoring $existingAgentBin..." -ForegroundColor Yellow
        Copy-Item -Path $agentExe -Destination $existingAgentBin -Force
        $repairsMade += "AgentBinaryRestored"
    } else {
        Write-Host "  [OK] Agent binary is present and valid PE AMD64." -ForegroundColor Green
    }

    # 2. Verify and repair native host binary
    if (-not (Test-Path $existingHostBin) -or -not (Test-ValidPE -Path $existingHostBin)) {
        Write-Host "  [REPAIR] Native host binary missing or invalid. Restoring $existingHostBin..." -ForegroundColor Yellow
        Copy-Item -Path $nativeHostExe -Destination $existingHostBin -Force
        $repairsMade += "NativeHostBinaryRestored"
    } else {
        Write-Host "  [OK] Native host binary is present and valid PE AMD64." -ForegroundColor Green
    }

    # 3. Verify and repair SCM service
    if (-not $SkipServiceStart) {
        $qc = Invoke-ScCommand "qc $ServiceName"
        $qsid = Invoke-ScCommand "qsidtype $ServiceName"
        $qprivs = Invoke-ScCommand "qprivs $ServiceName"
        $scmRepairNeeded = $false

        if ($qc.ExitCode -ne 0 -or $qc.Output -notmatch "SERVICE_START_NAME\s*:\s*NT AUTHORITY\\LocalService" -or $qc.Output -notmatch "START_TYPE\s*:\s*2\s+AUTO_START") {
            $scmRepairNeeded = $true
        }
        if ($qsid.Output -notmatch "SERVICE_SID_TYPE\s*:\s*UNRESTRICTED" -and $qsid.Output -notmatch "UNRESTRICTED") {
            $scmRepairNeeded = $true
        }
        if ($qprivs.Output -notmatch "SeChangeNotifyPrivilege") {
            $scmRepairNeeded = $true
        }

        if ($scmRepairNeeded) {
            Write-Host "  [REPAIR] SCM service configuration damaged or non-compliant. Repairing service in SCM..." -ForegroundColor Yellow
            $svcRepairProc = Start-Process -FilePath $existingAgentBin -ArgumentList "service", "install" -NoNewWindow -Wait -PassThru
            if ($svcRepairProc.ExitCode -eq 0) {
                $repairsMade += "SCMConfigurationRepaired"
            } else {
                Write-Warn "Native service repair exited with code $($svcRepairProc.ExitCode)."
            }
        } else {
            Write-Host "  [OK] SCM service configuration is intact." -ForegroundColor Green
        }
    }

    # 4. Verify and repair directory DACLs
    if (-not $SkipElevationCheck) {
        $effectiveAcl = Get-Acl $secureDir -ErrorAction SilentlyContinue
        $aclRepairNeeded = $false
        if ($null -eq $effectiveAcl) {
            $aclRepairNeeded = $true
        } else {
            $sddl = $effectiveAcl.Sddl
            if (-not ($sddl -match "D:P" -or $sddl -match "D:PAI") -or $sddl -match ";;;LS\)" -or $sddl -match ";;;S-1-5-19\)" -or -not ($sddl -match [regex]::Escape($ServiceSid))) {
                $aclRepairNeeded = $true
            }
        }
        if ($aclRepairNeeded) {
            Write-Host "  [REPAIR] Secure credential directory DACL damaged or non-compliant. Re-applying protected DACL..." -ForegroundColor Yellow
            $secureSDDL = "D:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)(A;OICI;GA;;;$ServiceSid)"
            $acl = Get-Acl $secureDir
            $acl.SetSecurityDescriptorSddlForm($secureSDDL)
            Set-Acl -Path $secureDir -AclObject $acl
            $repairsMade += "SecureDirectoryDACLRepaired"
        } else {
            Write-Host "  [OK] Secure credential directory DACL is intact." -ForegroundColor Green
        }
    }

    # 5. Verify and repair Native Messaging Manifests
    $hostName = "com.onusly.desktop_agent"
    $chromeManifestFile = "$manifestDir\${hostName}_chrome.json"
    $firefoxManifestFile = "$manifestDir\${hostName}_firefox.json"
    $manifestRepairNeeded = (-not (Test-Path $chromeManifestFile)) -or (-not (Test-Path $firefoxManifestFile))

    if ($manifestRepairNeeded) {
        Write-Host "  [REPAIR] Native messaging host manifests missing. Regenerating..." -ForegroundColor Yellow
        $chromiumExtId = "jifigaejbinmdhnljobkodcgeaplojmg"
        $legacyExtId = "gkichkblphmdbojdjnonljlegnmjcnhg"
        $firefoxExtId = "extension@onusly.com"

        $chromeManifest = @{
            name = $hostName
            description = "ONUSLY Desktop Enforcement Native Messaging Host"
            path = $existingHostBin
            type = "stdio"
            allowed_origins = @(
                "chrome-extension://$chromiumExtId/",
                "chrome-extension://$legacyExtId/",
                "chrome-extension://admccjkmockfdflocgggjfgdacdodkdf/"
            )
        }
        $chromeManifest | ConvertTo-Json -Depth 4 | Set-Content -Path $chromeManifestFile -Encoding UTF8

        $firefoxManifest = @{
            name = $hostName
            description = "ONUSLY Desktop Enforcement Native Messaging Host"
            path = $existingHostBin
            type = "stdio"
            allowed_extensions = @(
                $firefoxExtId
            )
        }
        $firefoxManifest | ConvertTo-Json -Depth 4 | Set-Content -Path $firefoxManifestFile -Encoding UTF8
        $repairsMade += "NativeMessagingManifestsRegenerated"
    } else {
        Write-Host "  [OK] Native messaging manifests are intact." -ForegroundColor Green
    }

    # 6. Verify and ensure Service running
    if (-not $SkipServiceStart) {
        $qex = Invoke-ScCommand "queryex $ServiceName"
        if ($qex.Output -notmatch "STATE\s*:\s*4\s+RUNNING") {
            Write-Host "  [REPAIR] Service is not running. Starting service..." -ForegroundColor Yellow
            Start-Process -FilePath $existingAgentBin -ArgumentList "service", "start" -NoNewWindow -Wait -PassThru | Out-Null
            $running = Wait-ServiceState -TargetServiceName $ServiceName -TargetState "RUNNING" -TimeoutSeconds 10
            if ($running) {
                $repairsMade += "ServiceStarted"
            }
        } else {
            Write-Host "  [OK] Service is already in RUNNING state." -ForegroundColor Green
        }
    }

    Write-Host "`n=================================================================" -ForegroundColor Green
    Write-Host "             ONUSLY WINDOWS REPAIR COMPLETE                      " -ForegroundColor Green
    Write-Host "=================================================================" -ForegroundColor Green
    Write-Host "Repairs Applied: $(if ($repairsMade.Count -gt 0) { $repairsMade -join ', ' } else { 'None (system was fully intact)' })"
    Write-Host "Device Identity: Preserved (untouched)"
    Write-Host "Credentials:     Preserved (untouched)"
    Write-Host "=================================================================`n"
    exit 0
}

# -------------------------------------------------------------
# FRESH INSTALL & UPGRADE WITH TRANSACTIONAL ROLLBACK
# -------------------------------------------------------------

# 4. Graceful Service Stop (if running)
if ($serviceWasRunning) {
    Write-Step "Gracefully Stopping Running Service for Upgrade..."
    $stopRes = Invoke-ScCommand "stop $ServiceName"
    $stopped = Wait-ServiceState -TargetServiceName $ServiceName -TargetState "STOPPED" -TimeoutSeconds 10
    if (-not $stopped) {
        Write-Warn "Service did not reach STOPPED within 10s. Checking PID $servicePID..."
        if ($servicePID -gt 0 -and (Get-Process -Id $servicePID -ErrorAction SilentlyContinue)) {
            Write-Warn "Terminating unresponsive process $servicePID as documented last resort..."
            Stop-Process -Id $servicePID -Force -ErrorAction SilentlyContinue
            Start-Sleep -Seconds 1
        }
    }
    $unlocked = Wait-FileLockRelease -Path $existingAgentBin -TimeoutSeconds 5
    if (-not $unlocked) {
        Write-Fail "Executable file '$existingAgentBin' remains locked after service stop."
    }
    Write-Success "Service stopped gracefully and executable lock released."
}

# 5. Staging and Safe Binary Backup
$binariesReplaced = $false
$policiesModified = $false
$hasBackup = $false
$backupStateFile = "$backupDir\backup_state.json"

try {
    Write-Step "Setting up Destination and Staging Directories..."
    New-Item -ItemType Directory -Path $InstallDir, $manifestDir, $stagingDir, $binBackupDir, $configDir, $storageDir, $logsDir, $secureDir, $backupDir -Force | Out-Null
    try { Start-Transcript -Path "$logsDir\install.log" -Append -Force -ErrorAction SilentlyContinue | Out-Null } catch {}

    # FAILURE INJECTION BOUNDARY 1: BinaryStaging
    if ($SimulateFailureStage -eq "BinaryStaging") {
        throw "SIMULATED_FAILURE: Failed to stage binaries into staging directory."
    }

    # Clear previous staging contents if any
    Get-ChildItem -Path $stagingDir -Recurse -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue

    $stagedAgent = "$stagingDir\onusly-agent.exe"
    $stagedHost = "$stagingDir\onusly-native-host.exe"

    Copy-Item -Path $agentExe -Destination $stagedAgent -Force
    Copy-Item -Path $nativeHostExe -Destination $stagedHost -Force

    # FAILURE INJECTION BOUNDARY 2: PEValidation
    if ($SimulateFailureStage -eq "PEValidation") {
        throw "SIMULATED_FAILURE: Staged binary PE signature check failed."
    }

    if (-not (Test-ValidPE -Path $stagedAgent)) {
        throw "Staged onusly-agent.exe failed PE validation."
    }
    if (-not (Test-ValidPE -Path $stagedHost)) {
        throw "Staged onusly-native-host.exe failed PE validation."
    }
    Write-Success "Binaries staged on target volume and validated."

    # FAILURE INJECTION BOUNDARY 3: BackupCreation
    if ($SimulateFailureStage -eq "BackupCreation") {
        throw "SIMULATED_FAILURE: Failed to create binary backup in backup directory."
    }

    # Back up existing binaries
    if (Test-Path $existingAgentBin) {
        Copy-Item -Path $existingAgentBin -Destination "$binBackupDir\onusly-agent.exe.bak" -Force
        $hasBackup = $true
    }
    if (Test-Path $existingHostBin) {
        Copy-Item -Path $existingHostBin -Destination "$binBackupDir\onusly-native-host.exe.bak" -Force
        $hasBackup = $true
    }
    if ($hasBackup) {
        Write-Success "Existing binaries backed up to $binBackupDir"
    }

    # Deploy staged binaries to production location
    Write-Step "Deploying Validated Binaries to Program Files..."

    # FAILURE INJECTION BOUNDARY 4: FirstBinaryReplacement
    if ($SimulateFailureStage -eq "FirstBinaryReplacement") {
        throw "SIMULATED_FAILURE: Failed during replacement of onusly-agent.exe."
    }
    Copy-Item -Path $stagedAgent -Destination $existingAgentBin -Force

    # FAILURE INJECTION BOUNDARY 5: SecondBinaryReplacement
    if ($SimulateFailureStage -eq "SecondBinaryReplacement") {
        $binariesReplaced = $true
        throw "SIMULATED_FAILURE: Failed during replacement of onusly-native-host.exe."
    }
    Copy-Item -Path $stagedHost -Destination $existingHostBin -Force
    $binariesReplaced = $true

    Remove-Item -Path $stagingDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Success "Binaries successfully deployed to $InstallDir"

    # 6. Critical Credential ACL Configuration
    Write-Step "Configuring Secure Directory ACLs (Blocking Generic LocalService)..."

    # FAILURE INJECTION BOUNDARY 6: ACLApplication
    if ($SimulateFailureStage -eq "ACLApplication") {
        throw "SIMULATED_FAILURE: Failed to apply protected DACL to secure directory."
    }

    if (-not $SkipElevationCheck) {
        # Apply Protected DACL to $secureDir:
        # Only SYSTEM (SY), Administrators (BA), and the unique Service SID are granted access.
        # Generic LocalService (S-1-5-19) is NOT granted access.
        $secureSDDL = "D:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)(A;OICI;GA;;;$ServiceSid)"
        $acl = Get-Acl $secureDir
        $acl.SetSecurityDescriptorSddlForm($secureSDDL)
        Set-Acl -Path $secureDir -AclObject $acl

        # Verify effective DACL on disk
        $effectiveAcl = Get-Acl $secureDir
        $effectiveSddl = $effectiveAcl.Sddl

        if (-not ($effectiveSddl -match "D:P" -or $effectiveSddl -match "D:PAI")) {
            throw "Secure directory inheritance is NOT protected! SDDL: $effectiveSddl"
        }
        if ($effectiveSddl -match ";;;LS\)" -or $effectiveSddl -match ";;;S-1-5-19\)") {
            throw "CRITICAL SECURITY DEFECT: Generic LocalService has been granted access to $secureDir!"
        }
        if (-not ($effectiveSddl -match [regex]::Escape($ServiceSid))) {
            throw "Secure directory is missing the ONUSLY Service SID grant!"
        }
        Write-Success "Secure credential directory ACL verified: Accessible ONLY to SYSTEM, Administrators, and Service SID ($ServiceSid)."

        # Apply Restrictive ACLs to Config, Data, Logs:
        # Administrators and Service SID full control; Users read-only
        $generalSDDL = "D:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)(A;OICI;GA;;;$ServiceSid)(A;OICI;GRGX;;;BU)"
        foreach ($d in @($configDir, $storageDir, $logsDir)) {
            $a = Get-Acl $d
            $a.SetSecurityDescriptorSddlForm($generalSDDL)
            Set-Acl -Path $d -AclObject $a
        }
        Write-Success "Configuration and data directory ACLs configured with tamper resistance."
    } else {
        Write-Host "  Skipping protected DACL configuration (-SkipElevationCheck specified)." -ForegroundColor DarkGray
    }

    # 7. Windows SCM Service Registration / Update
    Write-Step "Configuring Windows SCM Service ($ServiceName)..."

    # FAILURE INJECTION BOUNDARY 7: SCMConfiguration
    if ($SimulateFailureStage -eq "SCMConfiguration") {
        throw "SIMULATED_FAILURE: Failed to configure SCM service attributes."
    }

    if (-not $SkipServiceStart) {
        Write-Host "  Registering/updating service in SCM via native Win32 APIs..."
        $installSvcProc = Start-Process -FilePath $existingAgentBin -ArgumentList "service", "install" -NoNewWindow -Wait -PassThru
        if ($installSvcProc.ExitCode -ne 0) {
            throw "Native service registration failed with exit code $($installSvcProc.ExitCode)."
        }

        # Verify SCM Configuration
        $qcOutput = (Invoke-ScCommand "qc $ServiceName").Output
        $qsidOutput = (Invoke-ScCommand "qsidtype $ServiceName").Output
        $qprivsOutput = (Invoke-ScCommand "qprivs $ServiceName").Output

        if ($qcOutput -notmatch "SERVICE_START_NAME\s*:\s*NT AUTHORITY\\LocalService") {
            throw "SCM configuration defect: Service account is not NT AUTHORITY\LocalService: $qcOutput"
        }
        if ($qsidOutput -notmatch "SERVICE_SID_TYPE\s*:\s*UNRESTRICTED" -and $qsidOutput -notmatch "UNRESTRICTED") {
            throw "SCM configuration defect: Service SID type is not UNRESTRICTED: $qsidOutput"
        }
        if ($qprivsOutput -notmatch "SeChangeNotifyPrivilege") {
            throw "SCM configuration defect: Required privileges missing SeChangeNotifyPrivilege: $qprivsOutput"
        }
        Write-Success "Service registered/updated in SCM with LocalService account, unrestricted Service SID, and restricted privileges."
    } else {
        Write-Host "  Skipping SCM service registration (-SkipServiceStart specified)." -ForegroundColor DarkGray
    }

    # 8. Native-Messaging Host Manifests & Registration
    Write-Step "Configuring Native-Messaging Host Manifests..."

    # FAILURE INJECTION BOUNDARY 8: NativeMessagingRegistration
    if ($SimulateFailureStage -eq "NativeMessagingRegistration") {
        throw "SIMULATED_FAILURE: Failed to register native messaging host."
    }

    $hostName = "com.onusly.desktop_agent"
    $chromiumExtId = "jifigaejbinmdhnljobkodcgeaplojmg"
    $legacyExtId = "gkichkblphmdbojdjnonljlegnmjcnhg"
    $firefoxExtId = "extension@onusly.com"

    # Chromium Manifest
    $chromeManifestFile = "$manifestDir\${hostName}_chrome.json"
    $chromeManifest = @{
        name = $hostName
        description = "ONUSLY Desktop Enforcement Native Messaging Host"
        path = $existingHostBin
        type = "stdio"
        allowed_origins = @(
            "chrome-extension://$chromiumExtId/",
            "chrome-extension://$legacyExtId/",
            "chrome-extension://admccjkmockfdflocgggjfgdacdodkdf/"
        )
    }
    $chromeManifest | ConvertTo-Json -Depth 4 | Set-Content -Path $chromeManifestFile -Encoding UTF8

    # Firefox Manifest
    $firefoxManifestFile = "$manifestDir\${hostName}_firefox.json"
    $firefoxManifest = @{
        name = $hostName
        description = "ONUSLY Desktop Enforcement Native Messaging Host"
        path = $existingHostBin
        type = "stdio"
        allowed_extensions = @(
            $firefoxExtId
        )
    }
    $firefoxManifest | ConvertTo-Json -Depth 4 | Set-Content -Path $firefoxManifestFile -Encoding UTF8
    Write-Success "Native messaging manifests generated."

    # 9. Browser Detection & Enterprise Policy Merge
    Write-Step "Detecting Installed Browsers and Applying Enterprise Policies..."

    # FAILURE INJECTION BOUNDARY 9: BrowserPolicyConfiguration
    if ($SimulateFailureStage -eq "BrowserPolicyConfiguration") {
        $policiesModified = $true
        throw "SIMULATED_FAILURE: Failed to apply browser force-install policies."
    }

    if (-not $SkipRegistry) {
        $backupEntries = @()
        if (Test-Path $backupStateFile) {
            try {
                $existingState = Get-Content $backupStateFile -Raw | ConvertFrom-Json
                $backupEntries = @($existingState.entries)
            } catch {
                $backupEntries = @()
            }
        }

    $browserTargets = @(
        @{
            Type = "chrome"
            Name = "Google Chrome"
            PolicyKey = "HKLM:\SOFTWARE\Policies\Google\Chrome"
            NativeKey = "HKLM:\SOFTWARE\Google\Chrome\NativeMessagingHosts\$hostName"
            Manifest = $chromeManifestFile
            BinPaths = @(
                "${env:ProgramFiles}\Google\Chrome\Application\chrome.exe",
                "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe"
            )
        },
        @{
            Type = "edge"
            Name = "Microsoft Edge"
            PolicyKey = "HKLM:\SOFTWARE\Policies\Microsoft\Edge"
            NativeKey = "HKLM:\SOFTWARE\Microsoft\Edge\NativeMessagingHosts\$hostName"
            Manifest = $chromeManifestFile
            BinPaths = @(
                "${env:ProgramFiles}\Microsoft\Edge\Application\msedge.exe",
                "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
            )
        },
        @{
            Type = "brave"
            Name = "Brave"
            PolicyKey = "HKLM:\SOFTWARE\Policies\BraveSoftware\Brave"
            NativeKey = "HKLM:\SOFTWARE\BraveSoftware\Brave\NativeMessagingHosts\$hostName"
            Manifest = $chromeManifestFile
            BinPaths = @(
                "${env:ProgramFiles}\BraveSoftware\Brave-Browser\Application\brave.exe",
                "${env:ProgramFiles(x86)}\BraveSoftware\Brave-Browser\Application\brave.exe"
            )
        },
        @{
            Type = "chromium"
            Name = "Chromium"
            PolicyKey = "HKLM:\SOFTWARE\Policies\Chromium"
            NativeKey = "HKLM:\SOFTWARE\Chromium\NativeMessagingHosts\$hostName"
            Manifest = $chromeManifestFile
            BinPaths = @(
                "${env:ProgramFiles}\Chromium\Application\chrome.exe",
                "${env:ProgramFiles(x86)}\Chromium\Application\chrome.exe"
            )
        },
        @{
            Type = "firefox"
            Name = "Mozilla Firefox"
            PolicyKey = "HKLM:\SOFTWARE\Policies\Mozilla\Firefox"
            NativeKey = "HKLM:\SOFTWARE\Mozilla\NativeMessagingHosts\$hostName"
            Manifest = $firefoxManifestFile
            BinPaths = @(
                "${env:ProgramFiles}\Mozilla Firefox\firefox.exe",
                "${env:ProgramFiles(x86)}\Mozilla Firefox\firefox.exe"
            )
        }
    )

    foreach ($b in $browserTargets) {
        $installed = $false
        foreach ($bin in $b.BinPaths) {
            if (Test-Path $bin) {
                $installed = $true
                break
            }
        }

        if (-not $installed) {
            Write-Host "  [-] $($b.Name): Not installed (skipping policy registration)." -ForegroundColor DarkGray
            continue
        }

        Write-Host "  [+] $($b.Name): Detected! Registering Native Host and Enterprise Policy..." -ForegroundColor White

        # 1. Register Native Messaging Host
        $nativeKeyExisted = Test-Path $b.NativeKey
        if (-not $nativeKeyExisted) {
            New-Item -Path $b.NativeKey -Force | Out-Null
        }
        $priorNativeVal = (Get-ItemProperty -Path $b.NativeKey -Name "(Default)" -ErrorAction SilentlyContinue)."(Default)"
        Set-ItemProperty -Path $b.NativeKey -Name "(Default)" -Value $b.Manifest

        $backupEntries += @{
            browser = $b.Type
            keyPath = $b.NativeKey
            valueName = "(Default)"
            priorType = if ($priorNativeVal) { "string" } else { "none" }
            priorValue = if ($priorNativeVal) { $priorNativeVal } else { "" }
            createdByUs = [bool](-not $priorNativeVal)
            keyCreated = [bool](-not $nativeKeyExisted)
        }

        # 2. Configure Enterprise Policy
        if ($b.Type -eq "firefox") {
            # Firefox Enterprise Settings
            $ffKeyExisted = Test-Path $b.PolicyKey
            if (-not $ffKeyExisted) {
                New-Item -Path $b.PolicyKey -Force | Out-Null
            }
            $priorPrivate = (Get-ItemProperty -Path $b.PolicyKey -Name "DisablePrivateBrowsing" -ErrorAction SilentlyContinue).DisablePrivateBrowsing
            Set-ItemProperty -Path $b.PolicyKey -Name "DisablePrivateBrowsing" -Value 1 -Type DWord
            $backupEntries += @{
                browser = "firefox"
                keyPath = $b.PolicyKey
                valueName = "DisablePrivateBrowsing"
                priorType = if ($null -ne $priorPrivate) { "dword" } else { "none" }
                priorValue = if ($null -ne $priorPrivate) { [string]$priorPrivate } else { "" }
                createdByUs = ($null -eq $priorPrivate)
                keyCreated = [bool](-not $ffKeyExisted)
            }

            $extKey = "$($b.PolicyKey)\ExtensionSettings\$firefoxExtId"
            $extKeyExisted = Test-Path $extKey
            if (-not $extKeyExisted) {
                New-Item -Path $extKey -Force | Out-Null
            }
            Set-ItemProperty -Path $extKey -Name "installation_mode" -Value "force_installed" -Type String
            Set-ItemProperty -Path $extKey -Name "install_url" -Value $UpdateUrl -Type String
            $backupEntries += @{
                browser = "firefox"
                keyPath = $extKey
                valueName = "installation_mode"
                priorType = "none"
                priorValue = ""
                createdByUs = $true
                keyCreated = [bool](-not $extKeyExisted)
            }
        } else {
            # Chromium ExtensionInstallForcelist Merge
            $forceKey = "$($b.PolicyKey)\ExtensionInstallForcelist"
            $forceKeyExisted = Test-Path $forceKey
            if (-not $forceKeyExisted) {
                New-Item -Path $forceKey -Force | Out-Null
            }

            $existingProps = Get-ItemProperty -Path $forceKey -ErrorAction SilentlyContinue
            $forceEntry = "$chromiumExtId;$UpdateUrl"
            $assignedName = ""
            $maxIndex = 0

            if ($existingProps) {
                foreach ($prop in $existingProps.PSObject.Properties) {
                    if ($prop.Name -match '^\d+$') {
                        $idx = [int]$prop.Name
                        if ($idx -gt $maxIndex) { $maxIndex = $idx }
                        if ($prop.Value -like "$chromiumExtId;*" -or $prop.Value -like "$legacyExtId;*") {
                            $assignedName = $prop.Name
                            break
                        }
                    }
                }
            }

            if (-not $assignedName) {
                $assignedName = [string]($maxIndex + 1)
                Set-ItemProperty -Path $forceKey -Name $assignedName -Value $forceEntry -Type String
                $backupEntries += @{
                    browser = $b.Type
                    keyPath = $forceKey
                    valueName = $assignedName
                    priorType = "none"
                    priorValue = ""
                    createdByUs = $true
                    keyCreated = [bool](-not $forceKeyExisted)
                }
            } else {
                $priorVal = $existingProps.$assignedName
                Set-ItemProperty -Path $forceKey -Name $assignedName -Value $forceEntry -Type String
                $backupEntries += @{
                    browser = $b.Type
                    keyPath = $forceKey
                    valueName = $assignedName
                    priorType = "string"
                    priorValue = $priorVal
                    createdByUs = $false
                    keyCreated = [bool](-not $forceKeyExisted)
                }
            }

            # Configure DnsOverHttpsMode = "off" to prevent DoH bypass of local DNS enforcement
            $policyRootExisted = Test-Path $b.PolicyKey
            if (-not $policyRootExisted) {
                New-Item -Path $b.PolicyKey -Force | Out-Null
            }
            $priorDoh = (Get-ItemProperty -Path $b.PolicyKey -Name "DnsOverHttpsMode" -ErrorAction SilentlyContinue).DnsOverHttpsMode
            Set-ItemProperty -Path $b.PolicyKey -Name "DnsOverHttpsMode" -Value "off" -Type String
            $backupEntries += @{
                browser = $b.Type
                keyPath = $b.PolicyKey
                valueName = "DnsOverHttpsMode"
                priorType = if ($null -ne $priorDoh) { "string" } else { "none" }
                priorValue = if ($null -ne $priorDoh) { [string]$priorDoh } else { "" }
                createdByUs = ($null -eq $priorDoh)
                keyCreated = [bool](-not $policyRootExisted)
            }
        }
    }

        # Save backup state
        @{
            timestamp = (Get-Date).ToString("o")
            entries = $backupEntries
        } | ConvertTo-Json -Depth 6 | Set-Content -Path $backupStateFile -Encoding UTF8
        $policiesModified = $true
        Write-Success "Enterprise browser policies configured and merge state recorded for clean rollback."
    } else {
        Write-Host "  Skipping registry enterprise policy configuration (-SkipRegistry specified)." -ForegroundColor DarkGray
    }

    # 10. Start Service (if requested or if it was previously running)
    # FAILURE INJECTION BOUNDARY 10: ServiceStartup
    if ($SimulateFailureStage -eq "ServiceStartup") {
        throw "SIMULATED_FAILURE: Failed to start service in SCM."
    }

    if ($serviceWasRunning -or -not $SkipServiceStart) {
        Write-Step "Starting $ServiceName Service..."
        $qex = Invoke-ScCommand "queryex $ServiceName"
        if ($qex.Output -match "STATE\s*:\s*4\s+RUNNING") {
            Write-Success "Service is already in SERVICE_RUNNING state."
        } else {
            $startSvcProc = Start-Process -FilePath $existingAgentBin -ArgumentList "service", "start" -NoNewWindow -Wait -PassThru
            if ($startSvcProc.ExitCode -ne 0) {
                # Fallback to Invoke-ScCommand
                $startRes = Invoke-ScCommand "start $ServiceName"
                if ($startRes.ExitCode -ne 0 -and $startRes.ExitCode -ne 1056) {
                    throw "Failed to start service (ExitCode: $($startRes.ExitCode)): $($startRes.Output)"
                }
            }
            $running = Wait-ServiceState -TargetServiceName $ServiceName -TargetState "RUNNING" -TimeoutSeconds 10
            if (-not $running) {
                throw "Service failed to reach SERVICE_RUNNING state within 10 seconds."
            }
            Write-Success "Service reached SERVICE_RUNNING state."
        }

        # Verify named pipe
        $pipeFound = $false
        $pipeDeadline = [DateTime]::UtcNow.AddSeconds(5)
        while ([DateTime]::UtcNow -lt $pipeDeadline) {
            try {
                $pipes = [System.IO.Directory]::GetFiles("\\.\pipe\")
                if ($pipes -contains "\\.\pipe\OnuslyAgentIPC") {
                    $pipeFound = $true
                    break
                }
            } catch {}
            Start-Sleep -Milliseconds 300
        }

        if ($pipeFound) {
            Write-Success "Verified production named pipe created: \\.\pipe\OnuslyAgentIPC"
        } else {
            Write-Warn "Named pipe \\.\pipe\OnuslyAgentIPC not yet visible in pipe directory (service may still be initializing)."
        }
    } else {
        Write-Host "  Skipping service start (-SkipServiceStart specified)." -ForegroundColor DarkGray
    }

    # FAILURE INJECTION BOUNDARY 11: PostStartHealth
    if ($SimulateFailureStage -eq "PostStartHealth") {
        throw "SIMULATED_FAILURE: Post-start health check failed (named pipe unreachable)."
    }
} catch {
    $err = $_
    Write-Warn "Installation/Upgrade step failed: $err"
    if ($hasBackup -or $binariesReplaced) {
        Write-Step "Executing Transactional Rollback..."
        $rollbackSuccess = $true
        try {
            # 1. Stop any partially started service
            Invoke-ScCommand "stop $ServiceName" | Out-Null
            Wait-FileLockRelease -Path $existingAgentBin -TimeoutSeconds 5 | Out-Null

            # 2. Revert browser enterprise policies if modified
            if ($policiesModified -and (Test-Path $backupStateFile)) {
                Write-Host "  Rolling back modified browser enterprise policies..."
                try {
                    $state = Get-Content $backupStateFile -Raw | ConvertFrom-Json
                    $entries = @($state.entries)
                    for ($i = $entries.Count - 1; $i -ge 0; $i--) {
                        $entry = $entries[$i]
                        $kPath = $entry.keyPath
                        if (-not (Test-Path $kPath)) { continue }
                        if ($entry.createdByUs) {
                            if ($entry.valueName -eq "(Default)" -or $entry.valueName -eq "") {
                                Remove-Item -Path $kPath -Recurse -Force -ErrorAction SilentlyContinue
                            } else {
                                Remove-ItemProperty -Path $kPath -Name $entry.valueName -ErrorAction SilentlyContinue
                            }
                        } elseif ($entry.priorType -eq "string") {
                            Set-ItemProperty -Path $kPath -Name $entry.valueName -Value $entry.priorValue -Type String -ErrorAction SilentlyContinue
                        } elseif ($entry.priorType -eq "dword") {
                            Set-ItemProperty -Path $kPath -Name $entry.valueName -Value ([int]$entry.priorValue) -Type DWord -ErrorAction SilentlyContinue
                        }
                    }
                } catch {
                    Write-Warn "Error reverting browser policies during rollback: $_"
                }
            }

            # 3. Restore previous binaries from .backup
            if (Test-Path "$binBackupDir\onusly-agent.exe.bak") {
                Copy-Item -Path "$binBackupDir\onusly-agent.exe.bak" -Destination $existingAgentBin -Force
                Write-Host "  Restored $existingAgentBin from backup."
            }
            if (Test-Path "$binBackupDir\onusly-native-host.exe.bak") {
                Copy-Item -Path "$binBackupDir\onusly-native-host.exe.bak" -Destination $existingHostBin -Force
                Write-Host "  Restored $existingHostBin from backup."
            }

            # 4. Clean up staging directory
            if (Test-Path $stagingDir) {
                Remove-Item -Path $stagingDir -Recurse -Force -ErrorAction SilentlyContinue
            }

            # 5. Restart previous service if it was running before upgrade
            if ($serviceWasRunning) {
                Write-Host "  Restarting previous service ($ServiceName)..."
                Invoke-ScCommand "start $ServiceName" | Out-Null
                $restarted = Wait-ServiceState -TargetServiceName $ServiceName -TargetState "RUNNING" -TimeoutSeconds 10
                if ($restarted) {
                    Write-Success "Previous service restarted and healthy."
                } else {
                    Write-Warn "Previous service failed to reach RUNNING state during rollback."
                    $rollbackSuccess = $false
                }
            }

            if ($rollbackSuccess) {
                Write-Host "`n>>> [ROLLBACK_SUCCESS] Transactional rollback completed successfully. System returned to pre-upgrade state." -ForegroundColor Yellow
                Write-Fail "Upgrade failed and was cleanly rolled back: $err"
            } else {
                Write-Fail "CRITICAL: Upgrade failed and rollback completed with warnings: $err"
            }
        } catch {
            Write-Fail "CRITICAL: Upgrade failed ($err) and rollback encountered an unrecoverable error: $_"
        }
    } else {
        # Failure before binaries were replaced: clean up staging
        if (Test-Path $stagingDir) {
            Remove-Item -Path $stagingDir -Recurse -Force -ErrorAction SilentlyContinue
        }
        if ($serviceWasRunning) {
            Invoke-ScCommand "start $ServiceName" | Out-Null
        }
        Write-Host "`n>>> [ROLLBACK_SUCCESS] No binaries were replaced; staging cleaned and previous state preserved." -ForegroundColor Yellow
        Write-Fail "Installation/Upgrade aborted cleanly before modifications: $err"
    }
}

Write-Host "`n=================================================================" -ForegroundColor Green
Write-Host "         ONUSLY WINDOWS DESKTOP INSTALLATION SUCCESSFUL          " -ForegroundColor Green
Write-Host "=================================================================" -ForegroundColor Green
Write-Host "Mode:         $installMode"
Write-Host "Service:      $ServiceName (Running as NT AUTHORITY\LocalService)"
Write-Host "Service SID:  $ServiceSid (Unrestricted)"
Write-Host "Binaries:     $InstallDir"
Write-Host "Data Root:    $DataRoot"
Write-Host "Next Step:    Run 'onusly-agent pair' or test browser native host."
Write-Host "=================================================================`n"
exit 0
