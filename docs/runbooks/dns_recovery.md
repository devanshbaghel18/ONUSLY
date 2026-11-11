# Runbook: DNS Enforcement & Fail-Open Recovery

## 1. Subsystem Overview
The ONUSLY Desktop Agent intercepts domain resolution for restricted domains via a local DNS proxy listening on `127.0.0.1:53` (UDP and TCP).
- **Enforcement Rules:** Blocked domains resolve to `0.0.0.0` or `127.0.0.1` (configurable sinkhole).
- **Pass-through Rules:** Non-blocked queries are forwarded to the host's configured upstream DNS servers.
- **Fail-Open Invariant:** Network connectivity must never be broken if the ONUSLY service terminates abnormally.

---

## 2. DNS Adapter Configuration & Backup Storage
Before reconfiguring network adapter DNS settings to point to `127.0.0.1`, the agent takes a snapshot of the original adapter configuration:
- **Backup Path:** `C:\ProgramData\Onusly\dns_backup.json`
- **Protection Guard:** The agent verifies the discovered upstream servers and skips backing up any adapter that already points to loopback (`127.0.0.1` or `::1`), preventing loopback pollution of backup state.

Inspect current DNS backup state:
```powershell
Get-Content -Path "C:\ProgramData\Onusly\dns_backup.json" | ConvertFrom-Json | Format-Table
```

---

## 3. Fail-Open Recovery Procedures

### 3.1 Automatic Recovery on Service Shutdown
When `OnuslyDesktopAgent` receives a graceful stop signal, it automatically restores the original adapter DNS addresses from `dns_backup.json` and deletes the backup file upon successful restoration.

### 3.2 Recovery After Abnormal Termination / Hard Crash
If the service process was forcibly terminated or system rebooted while DNS was bound to loopback:
1. **Per-Adapter Restoration from Backup:**
   The uninstaller and installer rollback scripts inspect `C:\ProgramData\Onusly\config\dns_backup.json` (or `C:\ProgramData\Onusly\dns_backup.json`) and restore each adapter to its exact recorded state:
   - If originally **DHCP**: Executes `Set-DnsClientServerAddress -InterfaceIndex <idx> -ResetServerAddresses`
   - If originally **Static DNS**: Executes `Set-DnsClientServerAddress -InterfaceIndex <idx> -ServerAddresses <original-ips>` (e.g. `1.1.1.1`, `8.8.8.8`).
2. **Missing or Corrupt Backup Safety Guarantee:**
   If the backup file is missing or corrupted, ONUSLY strictly **refuses to blindly reset all adapters to DHCP**, which would destroy pre-existing static configurations. Instead, the recovery process reports the affected adapter and provides explicit manual restoration commands:
   ```powershell
   # To restore to DHCP (if the adapter originally obtained DNS automatically):
   Set-DnsClientServerAddress -InterfaceAlias "Wi-Fi" -ResetServerAddresses

   # To restore to Static DNS (preserving your custom DNS servers):
   Set-DnsClientServerAddress -InterfaceAlias "Wi-Fi" -ServerAddresses "1.1.1.1","8.8.8.8"
   ```
3. **Administrator Force Override:**
   In automated enterprise provisioning environments where DHCP is standard across all interfaces, administrators may explicitly pass `-ForceResetDNS` to `uninstall.ps1` to force all adapters pointing to `127.0.0.1` back to DHCP.

### 3.3 Emergency Command-Line Reset (Single Adapter)
If internet access is completely stalled due to DNS resolution failure:
```powershell
# Reset Wi-Fi or Ethernet adapter directly to automatic DHCP
netsh interface ip set dns name="Wi-Fi" source=dhcp
netsh interface ip set dns name="Ethernet" source=dhcp
ipconfig /flushdns
```

---

## 4. Verification and Health Testing
1. Query DNS through the local resolver:
   ```powershell
   Resolve-DnsName -Name google.com -Server 127.0.0.1
   ```
2. Verify blocked domain sinkhole behavior:
   ```powershell
   # Assuming 'youtube.com' is restricted by an active goal:
   Resolve-DnsName -Name youtube.com -Server 127.0.0.1
   # Expected IP4Address: 0.0.0.0 or 127.0.0.1
   ```
3. Verify upstream pass-through for unrestricted domains:
   ```powershell
   Resolve-DnsName -Name github.com -Server 127.0.0.1
   # Expected IP4Address: Public IP of GitHub
   ```

