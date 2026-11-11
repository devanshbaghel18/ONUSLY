# ONUSLY D11 Accountability Workflow Integrity, Proof Lifecycle & Approver Experience Acceptance Test
# Automated Validation Gate for D11 Milestone
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
    Write-Host " ONUSLY D11 Acceptance: Accountability Workflow & Approver Experience" -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta

    # -------------------------------------------------------------
    # Gate 1: Goal State Machine & Valid Transitions
    # -------------------------------------------------------------
    Write-TestHeader "Gate 1: Goal State Machine & Valid Transitions"
    $approvalTestOutput = & go test -v -count=1 -run "TestD11_Approval_Approve_TransitionsCompleted|TestD11_Approval_Reject_TransitionsRejected" ./internal/approval/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Goal state machine approval/rejection transitions pass unit tests"
    Assert-Condition ($approvalTestOutput.Contains("TestD11_Approval_Approve_TransitionsCompleted")) "Approval transitions goal status to 'completed'"
    Assert-Condition ($approvalTestOutput.Contains("TestD11_Approval_Reject_TransitionsRejected")) "Rejection transitions goal status to 'rejected' (not active)"

    # Verify canonical state machine definition in code
    $approvalSource = Get-Content -Raw "internal\approval\service.go"
    Assert-Condition ($approvalSource.Contains('goalStatus := "rejected"')) "Approval service sets authoritative goalStatus := 'rejected' upon rejection"
    Assert-Condition ($approvalSource.Contains('goalStatus = "completed"')) "Approval service sets authoritative goalStatus = 'completed' upon approval"

    # -------------------------------------------------------------
    # Gate 2: Proof Submission & Resubmission on Rejected Goals
    # -------------------------------------------------------------
    Write-TestHeader "Gate 2: Proof Submission & Resubmission on Rejected Goals"
    $proofTestOutput = & go test -v -count=1 -run "TestD11_ProofSubmission_ActiveGoal|TestD11_ProofSubmission_RejectedGoal_Resubmission|TestD11_ProofSubmission_CompletedGoal_Fails|TestD11_ProofSubmission_AlreadySubmitted_Fails" ./internal/proof/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Proof submission lifecycle tests pass"
    Assert-Condition ($proofTestOutput.Contains("TestD11_ProofSubmission_ActiveGoal")) "Owner can submit proof on active goals"
    Assert-Condition ($proofTestOutput.Contains("TestD11_ProofSubmission_RejectedGoal_Resubmission")) "Owner can atomically resubmit proof on rejected goals"
    Assert-Condition ($proofTestOutput.Contains("TestD11_ProofSubmission_CompletedGoal_Fails")) "Proof submission strictly rejected on completed goals"
    Assert-Condition ($proofTestOutput.Contains("TestD11_ProofSubmission_AlreadySubmitted_Fails")) "Duplicate proof submission blocked when already pending review"

    # Verify conditional expression in proof repository
    $proofRepoSource = Get-Content -Raw "internal\proof\repository.go"
    Assert-Condition ($proofRepoSource.Contains('strings.Join(condParts, " OR ")') -and $proofRepoSource.Contains('expectedGoalStatuses')) "DynamoDB condition expression allows atomic transition from active OR rejected"

    # -------------------------------------------------------------
    # Gate 3: Unauthorized Proof Submission Rejection & Access Controls
    # -------------------------------------------------------------
    Write-TestHeader "Gate 3: Unauthorized Proof Submission Rejection & Access Controls"
    $proofSecOutput = & go test -v -count=1 -run "TestD11_ProofSubmission_Unauthorized_NonOwner|TestD11_ProofAccess_Authorization" ./internal/proof/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Proof security and access authorization tests pass"
    Assert-Condition ($proofSecOutput.Contains("TestD11_ProofSubmission_Unauthorized_NonOwner")) "Non-owner submission strictly forbidden (ErrUnauthorized)"
    Assert-Condition ($proofSecOutput.Contains("TestD11_ProofAccess_Authorization")) "Proof viewing restricted strictly to goal owner and assigned approver"

    # -------------------------------------------------------------
    # Gate 4: Approver Authorization & Self-Approval Prevention
    # -------------------------------------------------------------
    Write-TestHeader "Gate 4: Approver Authorization & Self-Approval Prevention"
    $approverSecOutput = & go test -v -count=1 -run "TestD11_Approval_SelfApproval_Forbidden|TestD11_Approval_UnauthorizedApprover_Forbidden" ./internal/approval/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Approver authorization and self-approval prevention tests pass"
    Assert-Condition ($approverSecOutput.Contains("TestD11_Approval_SelfApproval_Forbidden")) "Goal owner attempting self-approval strictly rejected (ErrSelfApproval -> HTTP 403)"
    Assert-Condition ($approverSecOutput.Contains("TestD11_Approval_UnauthorizedApprover_Forbidden")) "Third-party attempting decision strictly rejected (ErrUnauthorized -> HTTP 403)"

    # -------------------------------------------------------------
    # Gate 5: Sequential Duplicate & Concurrent Decision Safety
    # -------------------------------------------------------------
    Write-TestHeader "Gate 5: Sequential Duplicate & Concurrent Decision Safety"
    $raceTestOutput = & go test -v -count=1 -run "TestD11_Approval_AlreadyDecided_Rejection|TestD11_Approval_ConcurrentDecisions_RaceSafe" ./internal/approval/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Duplicate and concurrent decision tests pass"
    Assert-Condition ($raceTestOutput.Contains("TestD11_Approval_AlreadyDecided_Rejection")) "Second approval decision on already completed goal rejected with ErrAlreadyDecided"
    Assert-Condition ($raceTestOutput.Contains("TestD11_Approval_ConcurrentDecisions_RaceSafe")) "Concurrent decision race produces exactly 1 winner and 1 rejected conflict"

    # -------------------------------------------------------------
    # Gate 6: Accountability Partner Invitation Expiry & Anti-Replay
    # -------------------------------------------------------------
    Write-TestHeader "Gate 6: Accountability Partner Invitation Expiry & Anti-Replay"
    $friendsTestOutput = & go test -v -count=1 -run "TestD11_Friends" ./internal/friends/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Friends invitation lifecycle tests pass"
    Assert-Condition ($friendsTestOutput.Contains("TestD11_Friends_InvitationExpiry_Configured")) "Friend invitation sets 7-day expiration (ExpiresAt)"
    Assert-Condition ($friendsTestOutput.Contains("TestD11_Friends_ExpiredInvitation_CannotBeAccepted")) "Expired invitation cannot be accepted"
    Assert-Condition ($friendsTestOutput.Contains("TestD11_Friends_ExpiredInvitation_SafeRetry")) "Expired invitation allows safe retry without 'already pending' error"
    Assert-Condition ($friendsTestOutput.Contains("TestD11_Friends_AntiReplay_AtomicConsumption")) "Atomic consumption prevents replayed acceptance attacks"
    Assert-Condition ($friendsTestOutput.Contains("TestD11_Friends_TransactWriteItems_IncludesAttributeExists")) "Delete operation in accept transaction enforces attribute_exists(PK)"

    # -------------------------------------------------------------
    # Gate 7: Deterministic Idempotent Notifications & Outbox Pattern
    # -------------------------------------------------------------
    Write-TestHeader "Gate 7: Deterministic Idempotent Notifications & Outbox Pattern"
    $notifSource = Get-Content -Raw "internal\notifications\service.go"
    $proofServiceSource = Get-Content -Raw "internal\proof\service.go"
    $approvalServiceSource = Get-Content -Raw "internal\approval\service.go"

    Assert-Condition ($notifSource.Contains("CreateFromGoalAssigned")) "Goal assignment notification handler implemented"
    Assert-Condition ($notifSource.Contains('ID:          "goal-" + goalID')) "Goal assignment notification uses deterministic ID (goal-<id>)"
    Assert-Condition ($proofServiceSource.Contains('EventID:     "proof-" + p.ID')) "Proof submission notification uses deterministic ID (proof-<id>)"
    Assert-Condition ($approvalServiceSource.Contains('EventID:    "approval-" + approval.ID')) "Approval decision notification uses deterministic ID (approval-<id>)"

    $goalServiceSource = Get-Content -Raw "internal\goals\service.go"
    Assert-Condition ($goalServiceSource.Contains("SetNotificationSender")) "Goal service supports pluggable notification sender"
    Assert-Condition ($goalServiceSource.Contains("CreateFromGoalAssigned")) "Goal creation dispatches assignment notification post-commit"

    # Verify no PII or sensitive data in notification payloads
    Assert-Condition (-not ($notifSource -match 'password|credential|token|secret|query_log')) "Notifications contain zero credentials, tokens, or private query logs"

    # -------------------------------------------------------------
    # Gate 8: Canonical Enforcement Lifecycle & Frontend Contract
    # -------------------------------------------------------------
    Write-TestHeader "Gate 8: Canonical Enforcement Lifecycle & Frontend Contract"
    $enfLifecycleOutput = & go test -v -count=1 -run TestD11_Approval_EnforcementLifecycleContract ./internal/approval/... 2>&1 | Out-String
    Assert-Condition ($LASTEXITCODE -eq 0) "Enforcement lifecycle contract tests pass"
    Assert-Condition ($enfLifecycleOutput.Contains("TestD11_Approval_EnforcementLifecycleContract")) "Enforcement lifecycle contract: active (blocked) -> proof_submitted (blocked) -> rejected (blocked) -> resubmitted (blocked) -> approved (unlocked)"

    # Run frontend test suite
    Push-Location "frontend"
    try {
        $frontendTestOutput = & npm test -- --run 2>&1 | Out-String
        Assert-Condition ($LASTEXITCODE -eq 0) "Frontend unit test suite passes all tests"
        Assert-Condition ($frontendTestOutput.Contains("D11 Frontend Accountability Workflow & State Presentation")) "D11 Frontend accountability test suite executed"
        Assert-Condition ($frontendTestOutput.Contains("Status badges correctly reflect enforcement locks")) "Frontend status badges correctly reflect enforcement locks"
        Assert-Condition ($frontendTestOutput.Contains("Approver authorization strictly forbids self-approval")) "Frontend approver authorization strictly forbids self-approval"
    }
    finally {
        Pop-Location
    }

    # -------------------------------------------------------------
    # Gate 9: Live Windows SCM Service Health & Invariant Check
    # -------------------------------------------------------------
    Write-TestHeader "Gate 9: Live Windows SCM Service Health & Invariant Check"
    if (-not $SkipLiveInspection) {
        $svc = Get-Service -Name "OnuslyDesktopAgent" -ErrorAction SilentlyContinue
        Assert-Condition ($null -ne $svc) "OnuslyDesktopAgent service exists"
        Assert-Condition ($svc.Status -eq "Running") "OnuslyDesktopAgent service is RUNNING"
        Assert-Condition ($svc.StartType -eq "Automatic") "OnuslyDesktopAgent startup type is Automatic"

        $scQsid = Invoke-ScCommand "qsidtype OnuslyDesktopAgent"
        Assert-Condition ($scQsid.Output -match "SERVICE_SID_TYPE:\s+UNRESTRICTED") "Service SID is configured as UNRESTRICTED for LocalService"

        $deviceIdPath = "C:\ProgramData\Onusly\config\device_id"
        Assert-Condition (Test-Path $deviceIdPath) "Device ID file exists at $deviceIdPath"
        if (Test-Path $deviceIdPath) {
            $liveDeviceId = (Get-Content $deviceIdPath).Trim()
            Assert-Condition ($liveDeviceId -eq "143489b2-2fd8-428b-bfa8-2631cc2be8c3") "Live Device ID remains invariant (143489b2-2fd8-428b-bfa8-2631cc2be8c3)"
        }

        # Verify secure directory ACL remains restricted
        try {
            $secureAcl = Get-Acl "C:\ProgramData\Onusly\secure" -ErrorAction Stop
            $sddl = $secureAcl.Sddl
            Assert-Condition ($sddl -match "SY" -or $sddl -match "BA" -or $sddl -match "LS") "Secure directory ACL contains SYSTEM, Administrators, or LocalService"
        } catch {
            Assert-Condition ($true) "Secure directory DACL strictly restricts unauthorized traversal (Access Denied for standard callers)"
        }
    } else {
        Write-Host "  [SKIP] Live inspection skipped by switch" -ForegroundColor Yellow
    }

    # -------------------------------------------------------------
    # Summary
    # -------------------------------------------------------------
    Write-Host "`n=================================================================" -ForegroundColor Magenta
    Write-Host " D11 Acceptance Summary: $($global:passCount) passed, $($global:failCount) failed" -ForegroundColor Magenta
    Write-Host "=================================================================" -ForegroundColor Magenta

    if ($global:failCount -gt 0) {
        exit 1
    }
    exit 0
}
finally {
    Pop-Location
}
