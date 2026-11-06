<#
.SYNOPSIS
    ONUSLY D3 Windows 11 Validation Probe Automation Script.
    Executes diagnostic tests as a genuine temporary Windows SCM Service
    under NT AUTHORITY\LocalService with SCM privilege stripping,
    runs both elevated and unelevated diagnostic clients for named-pipe SQOS verification,
    and executes a second-process generic LocalService access denial test.

.DESCRIPTION
    1. Verifies Administrator elevation.
    2. Proactively checks whether "OnuslyValidationProbeService" or "OnuslyGenericLSTestService" exists.
       Refuses to overwrite any pre-existing service with those names.
    3. Compiles or verifies onusly-probe.exe.
    4. Registers primary SCM test service: "OnuslyValidationProbeService".
    5. Configures service logon identity: "NT AUTHORITY\LocalService".
    6. Configures service SID type: SERVICE_SID_TYPE_UNRESTRICTED (0x1).
    7. Configures required privileges: strips SeImpersonatePrivilege, retains SeChangeNotifyPrivilege.
    8. Starts the service via SCM (Start-Service). Process enters Running state promptly and executes diagnostics.
    9. Executes dual interactive named-pipe clients:
       a) Elevated client (& onusly-probe.exe --client-elevated) from admin shell.
       b) Unelevated client via Windows Explorer COM shell (Shell.Application.ShellExecute)
          or displays explicit manual console launch instructions.
    10. Waits for %ProgramData%\OnuslyProbe\probe_completed.marker with finite timeout (30s).
    11. Executes second-process negative test: registers "OnuslyGenericLSTestService" under LocalService
        WITHOUT sidtype unrestricted, starts it via SCM to execute --test-negative-access, and verifies Win32 Error 5.
    12. Stops both services cleanly and parses results.
    13. Guaranteed stop/wait/delete cleanup in a finally block with surfaced deletion errors,
        post-cleanup SCM state verification (confirming 1060 ERROR_SERVICE_DOES_NOT_EXIST),
        and explicit detection of services left marked for deletion.

.RECOVERY INSTRUCTIONS (VM Crash / Forced Termination)
    If PowerShell is forcibly terminated (taskkill / VM power off) before cleanup:
    1. The service definitions remain in SCM:
       HKLM\SYSTEM\CurrentControlSet\Services\OnuslyValidationProbeService
       HKLM\SYSTEM\CurrentControlSet\Services\OnuslyGenericLSTestService
    2. Because start= demand, services do not run automatically on reboot.
    3. To clean up manually from an elevated prompt:
       Stop-Service -Name OnuslyValidationProbeService -Force -ErrorAction SilentlyContinue
       sc.exe delete OnuslyValidationProbeService
       Stop-Service -Name OnuslyGenericLSTestService -Force -ErrorAction SilentlyContinue
       sc.exe delete OnuslyGenericLSTestService
       Remove-Item -Recurse -Force "$env:ProgramData\OnuslyProbe"
    4. Note on DELETE_FLAG (Win32 Error 1072):
       If Services MMC (services.msc) or Process Explorer was open, the service is marked for deletion
       and will only be purged after all open handles are closed or after a VM reboot.
    5. Mandatory testing protocol: Execute inside a disposable Windows 11 VM snapshot and revert upon completion.
#>

[CmdletBinding()]
param(
    [string]$BinaryPath = ".\onusly-probe.exe"
)

$ErrorActionPreference = "Stop"
$ServiceName = "OnuslyValidationProbeService"
$NegServiceName = "OnuslyGenericLSTestService"
$LogDir = "$env:ProgramData\OnuslyProbe"
$LogFile = "$LogDir\probe_results.json"
$MarkerFile = "$LogDir\probe_completed.marker"
$ClientElevatedLog = "$LogDir\client_elevated.json"
$ClientUnelevatedLog = "$LogDir\client_unelevated.json"
$NegLogFile = "$LogDir\negative_test.json"
$ShutdownLog = "$LogDir\shutdown_status.json"

$scriptStatus = "NORMAL_COMPLETION"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " ONUSLY D3 - Windows 11 LocalService Validation Probe" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# 1. Check Administrator Privileges
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Error "This validation script must be run from an elevated Administrator PowerShell prompt."
    exit 1
}

# 2. Refuse to overwrite pre-existing services
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Error "Service '$ServiceName' already exists on this system. Aborting to avoid overwriting pre-existing state. Remove it manually or use a fresh VM snapshot before re-running."
    exit 1
}
if (Get-Service -Name $NegServiceName -ErrorAction SilentlyContinue) {
    Write-Error "Service '$NegServiceName' already exists on this system. Aborting to avoid overwriting pre-existing state. Remove it manually or use a fresh VM snapshot before re-running."
    exit 1
}

# 3. Check Probe Binary
if (-not (Test-Path $BinaryPath)) {
    Write-Host "[Compiling Probe] Running 'go build -o onusly-probe.exe main.go'..." -ForegroundColor Yellow
    go build -o $BinaryPath main.go
    if (-not (Test-Path $BinaryPath)) {
        Write-Error "Failed to locate or compile $BinaryPath"
        exit 1
    }
}
$FullBinaryPath = (Resolve-Path $BinaryPath).Path

try {
    # 4. Prepare isolated test directory & remove stale files
    Write-Host "[1/8] Preparing isolated test directory in ProgramData..." -ForegroundColor Yellow
    if (-not (Test-Path $LogDir)) {
        New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
    }
    Remove-Item -Path "$LogDir\*" -Force -Recurse -ErrorAction SilentlyContinue

    # 5. Create Primary Probe Service under LocalService with Unrestricted Service SID
    Write-Host "[2/8] Registering primary SCM test service: $ServiceName..." -ForegroundColor Yellow
    $scResult = sc.exe create $ServiceName binPath= "$FullBinaryPath" obj= "NT AUTHORITY\LocalService" type= own start= demand
    Write-Host $scResult
    $sidResult = sc.exe sidtype $ServiceName unrestricted
    Write-Host $sidResult
    $privResult = sc.exe privs $ServiceName SeChangeNotifyPrivilege
    Write-Host $privResult

    # 6. Start Primary Service via SCM (Enters Running state promptly; distinguishes start failure)
    Write-Host "[3/8] Starting primary service under NT AUTHORITY\LocalService via SCM..." -ForegroundColor Yellow
    try {
        Start-Service -Name $ServiceName
    } catch {
        $scriptStatus = "START_FAILURE"
        Write-Error "CRITICAL: Primary service '$ServiceName' failed to start under NT AUTHORITY\LocalService: $_"
        throw
    }
    Start-Sleep -Milliseconds 500

    # 7. Execute Dual Interactive Diagnostic Clients (Elevated & Unelevated)
    Write-Host "[4/8] Running interactive diagnostic named-pipe clients (SQOS Identification)..." -ForegroundColor Yellow

    # 7a. Elevated Client (inherits administrator shell token)
    Write-Host "  [Client 1: Elevated Administrator]" -ForegroundColor Cyan
    & $FullBinaryPath --client-elevated
    Start-Sleep -Milliseconds 300

    if (Test-Path $ClientElevatedLog) {
        $elevInfo = Get-Content $ClientElevatedLog | ConvertFrom-Json
        Write-Host "    TokenUser SID:                 $($elevInfo.token_user_sid)" -ForegroundColor Gray
        Write-Host "    Token Elevation:               $($elevInfo.is_elevated)" -ForegroundColor Gray
        Write-Host "    Token Integrity Level:         $($elevInfo.integrity_level_name) (RID $($elevInfo.integrity_level_rid))" -ForegroundColor Gray
        Write-Host "    Process Session ID:            $($elevInfo.session_id)" -ForegroundColor Gray
        Write-Host "    Belongs to Paired Console User: $($elevInfo.belongs_to_paired_console_user)" -ForegroundColor Gray
        Write-Host "    Server Auth Response:          $($elevInfo.server_response)" -ForegroundColor Gray
    } else {
        Write-Warning "client_elevated.json was not generated."
    }

    # 7b. Unelevated Client (launched via Windows Explorer COM shell)
    Write-Host "  [Client 2: Unelevated Console User (Browser Host Simulation)]" -ForegroundColor Cyan
    $unelevatedLaunched = $false
    try {
        $shell = New-Object -ComObject "Shell.Application"
        $shell.ShellExecute($FullBinaryPath, "--client-unelevated", "", "open", 1)
        $unelevatedLaunched = $true
        # Wait up to 3 seconds for unelevated client execution
        $uTimer = [System.Diagnostics.Stopwatch]::StartNew()
        while (-not (Test-Path $ClientUnelevatedLog) -and $uTimer.Elapsed.TotalSeconds -lt 3) {
            Start-Sleep -Milliseconds 300
        }
    } catch {
        Write-Warning "Shell.Application.ShellExecute failed to launch unelevated client: $_"
    }

    if (Test-Path $ClientUnelevatedLog) {
        $unelevInfo = Get-Content $ClientUnelevatedLog | ConvertFrom-Json
        Write-Host "    TokenUser SID:                 $($unelevInfo.token_user_sid)" -ForegroundColor Gray
        Write-Host "    Token Elevation:               $($unelevInfo.is_elevated)" -ForegroundColor Gray
        Write-Host "    Token Integrity Level:         $($unelevInfo.integrity_level_name) (RID $($unelevInfo.integrity_level_rid))" -ForegroundColor Gray
        Write-Host "    Process Session ID:            $($unelevInfo.session_id)" -ForegroundColor Gray
        Write-Host "    Belongs to Paired Console User: $($unelevInfo.belongs_to_paired_console_user)" -ForegroundColor Gray
        Write-Host "    Server Auth Response:          $($unelevInfo.server_response)" -ForegroundColor Gray
    } else {
        Write-Warning "Automated unelevated client execution was not completed."
        Write-Warning "MANUAL PROCEDURE: To validate with a genuine unelevated Medium Integrity client:"
        Write-Warning "  Open a standard, NON-elevated Command Prompt or PowerShell in this console session and run:"
        Write-Warning "    `"$FullBinaryPath`" --client-unelevated"
    }

    # 8. Wait for Primary Probe Completion Marker (Finite Timeout)
    Write-Host "[5/8] Waiting for diagnostic completion marker (max 30s timeout)..." -ForegroundColor Yellow
    $timeoutSeconds = 30
    $timer = [System.Diagnostics.Stopwatch]::StartNew()
    while (-not (Test-Path $MarkerFile) -and $timer.Elapsed.TotalSeconds -lt $timeoutSeconds) {
        Start-Sleep -Milliseconds 500
    }

    if (-not (Test-Path $MarkerFile)) {
        $scriptStatus = "TIMEOUT"
        Write-Error "TIMEOUT: Primary probe did not complete within $timeoutSeconds seconds. Diagnostic tests failed or hung."
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    } elseif (Test-Path $LogFile) {
        Write-Host "----------------------------------------------------------" -ForegroundColor Green
        Write-Host " PRIMARY SCM SERVICE DIAGNOSTIC RESULTS:" -ForegroundColor Green
        Get-Content $LogFile | Write-Host
        Write-Host "----------------------------------------------------------" -ForegroundColor Green
    } else {
        Write-Warning "Completion marker found but $LogFile was not readable."
    }

    # 9. Register & Execute Negative Access Test Service (Generic LocalService without Service SID)
    Write-Host "[6/8] Executing second-process negative access test (Generic LocalService without Service SID)..." -ForegroundColor Yellow
    $negBinPath = "$FullBinaryPath --test-negative-access"
    $createNeg = sc.exe create $NegServiceName binPath= "$negBinPath" obj= "NT AUTHORITY\LocalService" type= own start= demand
    # Note: NO sidtype unrestricted applied! SCM assigns SERVICE_SID_TYPE_NONE.
    try {
        Start-Service -Name $NegServiceName
    } catch {
        Write-Warning "Negative access test service start encountered: $_"
    }
    Start-Sleep -Milliseconds 500
    Stop-Service -Name $NegServiceName -Force -ErrorAction SilentlyContinue

    # 10. Parse Negative Test Result
    Write-Host "[7/8] Verifying generic LocalService denial..." -ForegroundColor Yellow
    if (Test-Path $NegLogFile) {
        Write-Host "----------------------------------------------------------" -ForegroundColor Cyan
        Write-Host " NEGATIVE ACCESS TEST RESULT (GENERIC LOCALSERVICE DENIAL):" -ForegroundColor Cyan
        Get-Content $NegLogFile | Write-Host
        Write-Host "----------------------------------------------------------" -ForegroundColor Cyan
    } else {
        Write-Warning "Negative access test log was not generated."
    }

    # 11. Stop Primary Service Cleanly (Bounded wait for service stop)
    Write-Host "[8/8] Stopping primary probe service via SCM..." -ForegroundColor Yellow
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Stopped) {
        Stop-Service -Name $ServiceName -Force
        $svc.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(10))
    }

    if (Test-Path $ShutdownLog) {
        Write-Host "  Service Shutdown Audit:" -ForegroundColor Cyan
        Get-Content $ShutdownLog | Write-Host
    }
}
catch {
    if ($scriptStatus -eq "NORMAL_COMPLETION") {
        $scriptStatus = "ABORTED"
    }
    Write-Error "Validation run aborted with error: $_"
}
finally {
    # 12. Guaranteed Cleanup of Disposable Services with surfaced deletion status and post-cleanup verification
    Write-Host "[Cleanup] Initiating cleanup (Trigger reason: $scriptStatus)..." -ForegroundColor Yellow
    foreach ($sName in @($ServiceName, $NegServiceName)) {
        $svc = Get-Service -Name $sName -ErrorAction SilentlyContinue
        if ($svc) {
            if ($svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Stopped) {
                Stop-Service -Name $sName -Force -ErrorAction SilentlyContinue
                try {
                    $svc.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(5))
                } catch {
                    Write-Warning "Wait for $sName to stop timed out."
                }
            }
            $delOut = sc.exe delete $sName 2>&1 | Out-String
            if ($LASTEXITCODE -ne 0) {
                Write-Warning "sc.exe delete $sName failed with exit code $LASTEXITCODE: $delOut"
            }
        }
    }

    # 13. Post-cleanup verification: check whether services are truly purged from SCM
    Write-Host "[Cleanup Verification] Checking post-cleanup SCM state..." -ForegroundColor Yellow
    $residualServices = @()
    foreach ($sName in @($ServiceName, $NegServiceName)) {
        $qOut = sc.exe query $sName 2>&1 | Out-String
        if ($LASTEXITCODE -eq 1060) {
            Write-Host "  Service '$sName' confirmed fully purged (1060 ERROR_SERVICE_DOES_NOT_EXIST)." -ForegroundColor Green
        } elseif ($LASTEXITCODE -eq 0) {
            if ($qOut -match "DELETE_FLAG") {
                Write-Warning "  Service '$sName' is MARKED FOR DELETION (handles still held open). Not yet purged."
            } else {
                Write-Warning "  Service '$sName' STILL EXISTS in SCM."
            }
            $residualServices += $sName
        } else {
            Write-Warning "  sc.exe query $sName returned code $LASTEXITCODE: $qOut"
            $residualServices += $sName
        }
    }

    if ($residualServices.Count -gt 0) {
        Write-Warning "=========================================================="
        Write-Warning " RESIDUAL SERVICE WARNING: $($residualServices -join ', ')"
        Write-Warning " Services were marked for deletion but have not purged."
        Write-Warning " Ensure all mmc.exe / services.msc / procexp windows are closed."
        Write-Warning " Manual purge: Close open handles or reboot disposable VM."
        Write-Warning "=========================================================="
    } else {
        Write-Host "[Complete] Environment cleanup verified clean." -ForegroundColor Green
    }
}
