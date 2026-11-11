# Runbook: Component Repair, Uninstallation & Data Retention

## 1. Overview
The ONUSLY installer and uninstaller provide transactional, deterministic management of binaries, services, registry keys, and local state.
- **Installer Script:** `desktop\installer\windows\install.ps1`
- **Uninstaller Script:** `desktop\installer\windows\uninstall.ps1`
- **Execution Requirement:** Must be run from an elevated Administrator prompt.

---

## 2. In-Place Component Repair

### 2.1 When to Use
Use component repair if:
- SCM service configuration is corrupted.
- File system permissions or ACLs on `C:\ProgramData\Onusly` have been modified.
- Browser native messaging manifests are missing or registry entries deleted.
- Service binary executable has been corrupted or overwritten.

### 2.2 Executing Repair
Run the installer with the `-Repair` parameter:
```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\desktop\installer\windows\install.ps1 -Repair -Verbose
```
Actions performed by Repair:
1. Validates PE binary signatures of `onusly-agent.exe` and `onusly-native-host.exe`.
2. Restores binaries if corrupted.
3. Re-applies secure ACLs to `C:\Program Files\Onusly` and `C:\ProgramData\Onusly`.
4. Re-configures SCM service `OnuslyDesktopAgent` with `LocalService`, service SID, and recovery actions.
5. Re-registers Chrome and Edge native messaging registry entries and creates manifests.
6. Preserves existing device identity (`device_id`) and DPAPI credentials.
7. Restarts the service and verifies pipe creation.

---

## 3. Uninstallation Lifecycle & Data Retention Policies

### 3.1 Retention Policies
The uninstaller supports three distinct retention modes:

#### Policy A: Default Retention (`KeepData`)
Leaves device identity (`device_id`), credentials, and configuration intact in `C:\ProgramData\Onusly` so future upgrades or reinstalls automatically resume without re-pairing.
```powershell
.\desktop\installer\windows\uninstall.ps1
```

#### Policy B: Full Purge (`RemoveAll` / `-Purge`)
Removes all traces of ONUSLY from the host:
- Stops and unregisters SCM service.
- Restores original network adapter DNS settings.
- Removes native messaging registry keys and enterprise policies.
- Deletes `C:\Program Files\Onusly`.
- Deletes `C:\ProgramData\Onusly` (including device identity and credentials).
```powershell
.\desktop\installer\windows\uninstall.ps1 -Purge
```

#### Policy C: Archive Data (`ArchiveData`)
Backs up `C:\ProgramData\Onusly` to `C:\ProgramData\Onusly_Backup_<yyyyMMdd_HHmmss>` before removing the active installation.
```powershell
.\desktop\installer\windows\uninstall.ps1 -ArchiveData
```

---

## 4. Fail-Open Cleanup Invariants

Regardless of retention policy, uninstallation **strictly enforces fail-open network recovery**:
1. **DNS Reversion:**
   - Reads `C:\ProgramData\Onusly\dns_backup.json` and restores original adapter settings.
   - Scans all active adapters for orphaned `127.0.0.1` and resets them to DHCP automatically.
   - Flushes Windows DNS cache (`ipconfig /flushdns`).
2. **Process Teardown:**
   - Gracefully stops `OnuslyDesktopAgent` with bounded 10s timeout.
   - Ensures any locked process is terminated before file removal.
3. **Browser Registration Cleanup:**
   - Deletes `com.onusly.native_host` registry keys from Chrome and Edge.

---

## 5. Post-Uninstall Verification Checklist
Verify the machine is in a clean, pristine state:
```powershell
# 1. Service does not exist
sc.exe query OnuslyDesktopAgent
# Expected: [SC] EnumQueryServicesStatus:OpenService FAILED 1060: The specified service does not exist

# 2. No loopback DNS adapters
Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object { $_.ServerAddresses -contains "127.0.0.1" }
# Expected: empty output

# 3. Native host registry keys removed
Test-Path "HKLM:\SOFTWARE\Google\Chrome\NativeMessagingHosts\com.onusly.native_host"
Test-Path "HKLM:\SOFTWARE\Microsoft\Edge\NativeMessagingHosts\com.onusly.native_host"
# Expected: False

# 4. Program Files directory removed
Test-Path "C:\Program Files\Onusly"
# Expected: False
```

