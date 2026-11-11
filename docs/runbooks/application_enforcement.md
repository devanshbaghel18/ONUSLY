# Runbook: Native Application Enforcement & Process Intelligence

## 1. Subsystem Architecture
The ONUSLY Native Application Enforcement Controller monitors running processes and terminates executables restricted by active ONUSLY goals.
- **Current Operational Tier:** Tier 1 — Detection & Response (user-mode polling and snapshot diffing).
- **Enforcement Action:** Immediate process termination via `OpenProcess(PROCESS_TERMINATE)` + `TerminateProcess()`.
- **TOCTOU & PID Reuse Protection:** Before sending termination signals, the controller queries `QueryFullProcessImageName` and creation timestamps to ensure the target PID has not been recycled by an unrelated system or user process.

---

## 2. Process Matching Precedence
When an authoritative goal restricts an application (e.g. `discord`, `steam`, `spotify`):
1. **Exact Path Match (Confidence: `exact_path`):** Matches the normalized absolute executable path (e.g. `C:\Users\User\AppData\Local\Discord\app-1.0.9000\Discord.exe`).
2. **Directory & Binary Match (Confidence: `dir_binary`):** Matches parent folder and executable name.
3. **Basename Match (Confidence: `basename`):** Case-insensitive match on binary name without extension (e.g. `discord.exe` matches `discord`).

### D6.1 Cross-Platform Target Isolation Rule
Desktop enforcement consumes only targets with `target_type == "desktop_app"`. Reverse-DNS Android package names (e.g. `com.discord`) are strictly ignored by the Windows/Linux desktop matcher and do not match desktop executables.

---

## 3. Strict Protected Immunity List
The engine refuses to terminate any process that appears on the immunity list, regardless of goal configuration:
- System PIDs: `PID <= 4` (System Idle Process, System).
- Windows Core System Binaries:
  - `explorer.exe`, `lsass.exe`, `csrss.exe`, `services.exe`, `svchost.exe`, `smss.exe`, `wininit.exe`, `winlogon.exe`.
- Shell and Terminal Interpreters:
  - `cmd.exe`, `powershell.exe`, `pwsh.exe`, `bash.exe`, `wsl.exe`.
- ONUSLY Binaries:
  - `onusly-agent.exe`, `onusly-native-host.exe`, self PID.

---

## 4. Operational Commands & Inventory Management

### 4.1 Querying Installed Applications
List detected applications from Windows Uninstall Registry keys (`HKLM` and `HKCU`):
```powershell
& "C:\Program Files\Onusly\onusly-agent.exe" apps inventory
```
Output columns:
- Name
- Publisher
- Display Version
- Install Location / Executable Path
- Identifier Tag

### 4.2 Forcing an Inventory Refresh
Rescan registry keys and update the local application catalog:
```powershell
& "C:\Program Files\Onusly\onusly-agent.exe" apps refresh
```

### 4.3 Querying App Enforcement Subsystem Status
```powershell
& "C:\Program Files\Onusly\onusly-agent.exe" apps status
```
Output fields:
- Subsystem Active: `YES`
- Capability Tier: `Tier 1 - Detection & Response`
- Monitored Processes Count
- Active App Restrictions Count
- Enforced Terminations Total
- Enforcement Failures Total

---

## 5. Troubleshooting & Diagnostics

### 5.1 Restricted App Not Being Blocked
1. Check goal status: Ensure the goal status is `active`, `proof_submitted`, or `rejected`. If `completed`, execution is allowed.
2. Check policy sync: Run `onusly-agent query status` to ensure the agent has synchronized the latest backend policy snapshot.
3. Check target type: Verify that the target was authored as a `desktop_app` and not an `android_package`.
4. Check process executable name: Run `Get-Process <name> | Select-Object Name, Path`. Confirm the executable name aligns with the target identifier.
5. Check immunity: Ensure the process is not on the protected immunity list.

### 5.2 Termination Denied / Access Denied Errors
- Ensure the desktop agent is executing under `NT AUTHORITY\LocalService` with `SeDebugPrivilege` or appropriate permissions to query and open target user processes.
- If a target app is running elevated as Administrator while the agent is running without sufficient privilege, process termination will fail with `Access is denied`. The service must run as `LocalService` with service SID.

