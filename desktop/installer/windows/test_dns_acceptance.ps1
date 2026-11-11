# ONUSLY Windows Local DNS Enforcement, Resolver Integration & Network Lifecycle Acceptance Test
# Automated Validation Gate for D4 Milestone
# Invariant: Never leaves network adapters in a broken state; strictly fail-open.

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
    Write-Host " ONUSLY D4 WINDOWS LOCAL DNS ENFORCEMENT ACCEPTANCE SUITE        " -ForegroundColor Magenta
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
    # 2. DNS Resolver Unit and Policy Matching Tests
    # -------------------------------------------------------------
    Write-TestHeader "2. DNS Resolver Engine & Wire-Format Verification"
    $dnsTests = & go test -v ./desktop/agent/internal/dns/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "All DNS unit and acceptance tests passed"
    Assert-Condition ($dnsTests -match "PASS: TestDNSCache_TTLAndExpiration") "TTL dynamic adjustment and expiration verified"
    Assert-Condition ($dnsTests -match "PASS: TestDNSCache_CapacityEviction") "Cache capacity eviction verified"
    Assert-Condition ($dnsTests -match "PASS: TestDNSCache_InvalidateTargetDomains") "Instantaneous cache invalidation on blocklist update verified"
    Assert-Condition ($dnsTests -match "PASS: TestNormalizeDomain") "Domain normalization (lowercase, trailing dot, punycode) verified"
    Assert-Condition ($dnsTests -match "PASS: TestIsDomainMatch") "Strict boundary domain matching verified (no substring bypasses)"
    Assert-Condition ($dnsTests -match "PASS: TestIsProtectedDomain") "Core infrastructure immunity verified (localhost, *.onusly.com, RFC1918)"
    Assert-Condition ($dnsTests -match "PASS: TestUpstreamPool_RejectionOfHostnames") "Rejection of hostnames in upstream pool (loopback deadlock prevention) verified"
    Assert-Condition ($dnsTests -match "PASS: TestUpstreamPool_Failover") "Upstream pool automatic failover and TC fallback verified"

    # -------------------------------------------------------------
    # 3. D4 Goal Status Enforcement Matrix
    # -------------------------------------------------------------
    Write-TestHeader "3. D4 Goal Status Enforcement Contract"
    Assert-Condition ($dnsTests -match "PASS: TestD4_GoalStatusEnforcementContract") "Goal contract verified: active/proof_submitted/rejected blocked, completed unblocked"
    Assert-Condition ($dnsTests -match "PASS: TestD4_SubdomainAndDomainMatchingRules") "Subdomain, www parity, and IDN punycode matching verified"
    Assert-Condition ($dnsTests -match "PASS: TestD4_RecordTypesBlocked") "Record types (A, AAAA, CNAME, TXT, MX, SRV, PTR) blocked with synthetic NXDOMAIN"
    Assert-Condition ($dnsTests -match "PASS: TestD4_ProtectedDomainsImmunity") "Protected domains immunity verified against malicious/accidental rules"
    Assert-Condition ($dnsTests -match "PASS: TestD4_ZeroQueryLoggingPrivacy") "Zero query logging privacy invariant verified (no DNS surveillance)"

    # -------------------------------------------------------------
    # 4. Adapter Management & Fail-Open Verification (Mock Isolation)
    # -------------------------------------------------------------
    Write-TestHeader "4. Adapter Management & Fail-Open Rollback (Mock Isolation)"
    Assert-Condition ($dnsTests -match "PASS: TestWindowsAdapterManager_MockLifecycle") "Windows adapter backup, loopback redirect, and fail-open restore verified"

    # Run CLI dns status to verify adapter discovery on real system
    $cliStatus = & "$SourceBinDir\onusly-agent.exe" dns status 2>&1 | Out-String
    Assert-Condition ($cliStatus -match "ONUSLY DNS Redirection Status") "CLI 'onusly-agent dns status' executes cleanly"
    Assert-Condition ($cliStatus -match "Discovered Network Adapters:") "CLI discovered active network adapters on Windows host"

    # -------------------------------------------------------------
    # 5. Standalone Agent Runtime with DNS Resolver & Named Pipe
    # -------------------------------------------------------------
    Write-TestHeader "5. Standalone Agent Runtime with DNS Resolver & Named Pipe"
    $tempTestRoot = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "onusly-dns-test-$PID")
    $tempConfig = "$tempTestRoot\config"
    $tempData = "$tempTestRoot\data"
    New-Item -ItemType Directory -Path $tempConfig -Force | Out-Null
    New-Item -ItemType Directory -Path $tempData -Force | Out-Null

    $testPipe = "\\.\pipe\OnuslyDnsAccPipe_$PID"
    $testDnsAddr = "127.0.0.1:53"

    Write-Host "Launching standalone test agent on pipe $testPipe and DNS $testDnsAddr..."
    $proc = Start-Process -FilePath $agentExe -ArgumentList "run", "-dns-listen", $testDnsAddr, "-socket", $testPipe, "-config-dir", $tempConfig, "-data-dir", $tempData -NoNewWindow -PassThru

    try {
        # Wait up to 5s for pipe to become available
        $pipeReady = $false
        for ($i = 0; $i -lt 10; $i++) {
            Start-Sleep -Milliseconds 300
            $probeStatus = & $agentExe diagnostics -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
            if ($probeStatus -match "dns_resolver_running:\s*true") {
                $pipeReady = $true
                break
            }
        }
        Assert-Condition $pipeReady "Standalone agent started and initialized DNS resolver"

        # Check diagnostics
        $diagOut = & $agentExe diagnostics -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
        Assert-Condition ($diagOut -match "dns_resolver_running:\s*true") "Agent reports dns_resolver_running = true"
        Assert-Condition ($diagOut -match "dns_listen_address:\s*127.0.0.1:53") "Agent reports dns_listen_address = 127.0.0.1:53"

        # -------------------------------------------------------------
        # 6. Live Loopback DNS Query Resolution (127.0.0.1:53)
        # -------------------------------------------------------------
        Write-TestHeader "6. Live Loopback DNS Query Resolution (127.0.0.1:53)"
        try {
            $resolveRes = @(Resolve-DnsName -Name "google.com" -Server "127.0.0.1" -Type A -DnsOnly -ErrorAction Stop)
            $hasIP = [bool]($resolveRes.Count -gt 0 -and $resolveRes[0].IPAddress)
            Assert-Condition $hasIP "Loopback resolver successfully resolved google.com (IP: $($resolveRes[0].IPAddress))"
        } catch {
            Assert-Condition $false "Loopback DNS resolution failed: $_"
        }

        # Verify query counters incremented in agent diagnostics
        $diagOutAfter = & $agentExe diagnostics -socket $testPipe -config-dir $tempConfig -data-dir $tempData 2>&1 | Out-String
        Assert-Condition ($diagOutAfter -match "dns_queries_total:\s*[1-9]") "Aggregate queriesTotal counter incremented"
        Assert-Condition ($diagOutAfter -match "dns_queries_forwarded:\s*[1-9]") "Aggregate queriesForwarded counter incremented"
    } finally {
        if ($proc -and -not $proc.HasExited) {
            Write-Host "Stopping standalone test agent process $($proc.Id)..."
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
            Start-Sleep -Milliseconds 500
        }
        Remove-Item -Path $tempTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }

    # -------------------------------------------------------------
    # 7. Live Production Service Invariants (SCM, LocalService, SID)
    # -------------------------------------------------------------
    Write-TestHeader "7. Live Production Service Invariants"
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

    # Live service pipe status check
    $liveStatusOut = & $agentExe status 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Live agent query status exited with 0"
    Assert-Condition ($liveStatusOut -match "ONUSLY DESKTOP AGENT \(Live Service\)") "Connected to live service over \\.\pipe\OnuslyAgentIPC"

    # -------------------------------------------------------------
    # 8. Enterprise Browser DoH Policy Verification
    # -------------------------------------------------------------
    Write-TestHeader "8. Enterprise Browser DoH Policy Verification"
    $policyTests = & go test -v ./desktop/policy/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Browser policy engine tests passed"
    Assert-Condition ($policyTests -match "PASS: TestWindowsPolicy_MultiBrowserApplicationAndMerge") "Multi-browser policy merge and DnsOverHttpsMode='off' verified"
    Assert-Condition ($policyTests -match "PASS: TestWindowsPolicy_IdempotentInstallationAndRemoval") "Idempotent policy installation and clean surgical rollback verified"

    # -------------------------------------------------------------
    # Acceptance Summary
    # -------------------------------------------------------------
    Write-Host "`n=================================================================" -ForegroundColor Magenta
    Write-Host " D4 ACCEPTANCE TEST RESULTS SUMMARY                              " -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta
    Write-Host " Total Passed: $global:passCount" -ForegroundColor Green
    Write-Host " Total Failed: $global:failCount" -ForegroundColor $(if ($global:failCount -eq 0) { "Green" } else { "Red" })

    if ($global:failCount -eq 0) {
        Write-Host "`n>>> [SUCCESS] ALL D4 ACCEPTANCE GATES PASSED! <<<\n" -ForegroundColor Green
        exit 0
    } else {
        Write-Host "`n>>> [FAILURE] D4 ACCEPTANCE CRITERIA NOT MET. Check failed assertions above. <<<\n" -ForegroundColor Red
        exit 1
    }

} finally {
    Pop-Location
}
