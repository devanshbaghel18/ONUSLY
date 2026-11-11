# Runbook: Installation and Pairing

## 1. Scope and Prerequisites
This runbook covers the standard installation and pairing lifecycle for the ONUSLY Desktop Agent on Windows (and Linux daemons).

### Prerequisites
- Windows 10/11 (x64) with Administrator privileges for installation.
- PowerShell 5.1+ or PowerShell 7+.
- Network access to the ONUSLY backend API (`https://api.onusly.com` or local dev instance).
- Valid Pairing Token generated from the ONUSLY Web Dashboard (`https://app.onusly.com/devices/pair`).

---

## 2. Windows Installation Procedure

### 2.1 Fresh Installation
Run the installer PowerShell script from an elevated Administrator prompt:
```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\desktop\installer\windows\install.ps1 -Verbose
```

### 2.2 Pre-Flight & Verification
Verify that the service is running and properly isolated:
```powershell
# 1. Verify SCM Service status
Get-Service -Name OnuslyDesktopAgent

# 2. Verify LocalService Token & Service SID
$proc = Get-CimInstance Win32_Process -Filter "Name = 'onusly-agent.exe'"
Get-Process -Id $proc.ProcessId -IncludeUserName

# 3. Verify Named Pipe Creation
[System.IO.Directory]::GetFiles("\\.\\pipe\\") -match "OnuslyAgentIPC"

# 4. Verify ProgramData Directory ACLs
Get-Acl "C:\ProgramData\Onusly" | Format-List
```
Expected Invariants:
- Service Status: `Running`
- Process Account: `NT AUTHORITY\LocalService`
- Service SID: Unrestricted service SID present (`S-1-5-80-3463423020-2351545225-1748562058-3133284821-4013307172`)
- Pipe Name: `\\.\pipe\OnuslyAgentIPC`
- Access Control: Administrators (Full), LocalSystem (Full), LocalService (Read/Write/Execute), Users (Read only).

---

## 3. Device Pairing Workflow

### 3.1 Generating a Pairing Code
1. Log into the ONUSLY Dashboard.
2. Navigate to **Settings** > **Devices** > **Add Desktop Device**.
3. Copy the 64-character pairing token (valid for 15 minutes).

### 3.2 Executing Pairing
In an administrative or user console, issue the pairing command:
```powershell
& "C:\Program Files\Onusly\onusly-agent.exe" pair <PAIRING_TOKEN>
```

### 3.3 Verifying Pairing Success
Query the live agent over the authenticated named pipe:
```powershell
& "C:\Program Files\Onusly\onusly-agent.exe" query status
```
Expected Output:
```json
{
  "service_status": "running",
  "pairing_state": "PAIRED",
  "device_id": "143489b2-2fd8-428b-bfa8-2631cc2be8c3",
  "account_bound": true,
  "sync_status": "HEALTHY",
  "version": "1.0.0-rc1"
}
```

---

## 4. Troubleshooting Pairing Failures

| Symptom | Probable Cause | Action |
|---|---|---|
| `Pairing token expired or invalid` | Token expired after 15m or typo | Generate a new pairing token in dashboard and retry. |
| `Cannot connect to pipe \\.\pipe\OnuslyAgentIPC` | Agent service not running or access denied | Verify service with `sc.exe query OnuslyDesktopAgent`. Start service if stopped. |
| `Clock skew detected` | System clock out of sync (>5m) | Run `w32tm /resync` to sync system time. |
| `TLS certificate validation failed` | Corporate proxy or missing root CA | Verify backend URL reachability; check corporate TLS inspection. |
| `Device identity mismatch` | Existing credentials belong to another user | If re-pairing to a new user, pass `-Force` or run `install.ps1 -Repair`. |

