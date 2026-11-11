# Runbook: Service Recovery & Crash Resilience

## 1. Service Architecture & SCM Configuration
The ONUSLY Desktop Agent (`OnuslyDesktopAgent`) executes under the Windows Service Control Manager (SCM) as `NT AUTHORITY\LocalService` with an unrestricted service SID.

### Automated Failure Recovery Actions
The service is registered with the following SCM recovery policy:
- **Reset Period:** 86,400 seconds (24 hours).
- **First Failure:** Restart service after 5,000 milliseconds (5s).
- **Second Failure:** Restart service after 10,000 milliseconds (10s).
- **Subsequent Failures:** SCM stops automatic looping to prevent CPU thrashing; alerts logged to Windows Application Event Log.

Verify current SCM failure configuration:
```powershell
sc.exe qfailure OnuslyDesktopAgent
```

---

## 2. Emergency Health Inspection

### 2.1 Diagnostic Data Dump
Run the built-in diagnostic inspector over named pipe IPC:
```powershell
& "C:\Program Files\Onusly\onusly-agent.exe" diagnostics
```
The output provides:
- Service uptime, PID, and memory footprint.
- Pairing status and device ID.
- Dynamic cache status (`cache_valid: true`, `target_count`, `etag`).
- Subsystem states: DNS resolver (`active`), Native App Controller (`active`, `tier: 1`), Native messaging host connectivity.
- SCM recovery counter.

### 2.2 Event Log Inspection
Review Windows Event Log for crashes, unhandled exceptions, or service termination:
```powershell
Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='OnuslyDesktopAgent'} -MaxEvents 50 | Format-Table TimeCreated, LevelDisplayName, Message -Wrap
```

---

## 3. Manual Recovery Procedures

### 3.1 Service Stopped or Hanging
If the service entered a stopped state or is unresponsive:
```powershell
# Attempt graceful stop with 10s bounded timeout
Stop-Service -Name OnuslyDesktopAgent -Force -ErrorAction SilentlyContinue

# If PID remains locked, kill orphaned process safely
$p = Get-Process -Name "onusly-agent" -ErrorAction SilentlyContinue
if ($p) {
    Stop-Process -Id $p.Id -Force
}

# Start the service cleanly
Start-Service -Name OnuslyDesktopAgent

# Verify operational state
Get-Service -Name OnuslyDesktopAgent
```

### 3.2 Service Stuck in Restart Loop
If bad configuration or corrupted state causes rapid crashing:
1. Stop the service:
   ```powershell
   sc.exe stop OnuslyDesktopAgent
   ```
2. Inspect cache file integrity in `C:\ProgramData\Onusly\policy_cache.json`.
3. If JSON is corrupted or invalid, move it to quarantine:
   ```powershell
   Move-Item "C:\ProgramData\Onusly\policy_cache.json" "C:\ProgramData\Onusly\policy_cache.json.corrupt"
   ```
4. Restart service:
   ```powershell
   Start-Service -Name OnuslyDesktopAgent
   ```
   The service will boot in fail-open mode, query the backend for a fresh policy snapshot, and atomically write a new valid cache.

---

## 4. Bounded Shutdown & Resource Leak Validation
- Shutdown deadline: 5 seconds bounded graceful shutdown on `SIGTERM` / SCM `SERVICE_CONTROL_STOP`.
- On shutdown, the agent closes IPC listeners, halts process monitors, releases network sockets, and restores DNS settings.

