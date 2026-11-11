# Incident Response Runbook: DNS Resolver Failure & DNS Drift

## 1. Incident Classification & Severity
- **Incident Code**: `INC-DNS-001`
- **Severity**: **SEV-2 (High)** (Degraded local enforcement with active fail-open containment)
- **Component**: Desktop Local DNS Interceptor / Loopback Stub Resolver (`127.0.0.1:53` / `[::1]:53`)
- **Impact**: Domain restrictions cannot be enforced locally, or network adapters lose internet connectivity if fail-open fails.

---

## 2. Detection & Alerting Signals
An operator or automated monitor identifies this incident through any of the following authoritative signals:

1. **Prometheus Metrics**:
   - `onusly_heartbeats_total{status="dns_unavailable"}` counter incrementing.
   - `onusly_api_failures_total{path="/enforcement/heartbeat"}` elevated with `dns_drift` or `service_degraded`.
2. **Desktop Agent Status**:
   - `onusly-agent status` outputs:
     ```
     Health State:    DNS_UNAVAILABLE
     DNS Resolver:    UNAVAILABLE (Running: false, Blocked: ..., Fwd: ...)
     Issues:          [dns_drift]
     ```
3. **Structured Logs**:
   - Log entries with:
     ```json
     {"severity":"ERROR","component":"dns","op":"bind_adapter","error_category":"driver|permission","message":"failed to bind loopback socket..."}
     ```
4. **Dashboard Device Coverage**:
   - Device displays `DNS Unavailable` or `Degraded` badge with issue tag `dns drift`.

---

## 3. Immediate Containment & Fail-Open Verification
ONUSLY operates on an inviolable **fail-open** contract: under no circumstances should an enforcement failure cut off user internet access.

1. **Verify Fail-Open Redirection State**:
   Run the CLI DNS status check:
   ```powershell
   onusly-agent dns status
   ```
   Or on Linux:
   ```bash
   sudo onusly-agent dns status
   ```
2. **If Network is Unreachable (Fail-Closed Drift Detected)**:
   Immediately force restore original system DNS configuration from the automated backup:
   ```powershell
   # Windows (Elevated PowerShell)
   onusly-agent dns restore
   ```
   ```bash
   # Linux (Systemd)
   sudo onusly-agent dns restore
   ```
   Verify adapter returns to standard DHCP / public resolver:
   ```powershell
   netsh interface ipv4 show dnsservers
   ```

---

## 4. Root Cause Diagnosis
Investigate why the DNS interceptor failed:

1. **Port 53 Conflict**:
   Check if another process (e.g. `systemd-resolved`, `Acrylic DNS`, or a VPN daemon) has bound `0.0.0.0:53`:
   ```powershell
   # Windows
   Get-NetTCPConnection -LocalPort 53
   Get-NetUDPEndpoint -LocalPort 53
   ```
   ```bash
   # Linux
   sudo ss -ulpn | grep :53
   ```
2. **Service Privilege & Elevation**:
   Ensure the `OnuslyDesktopAgent` Windows service is running under `NT AUTHORITY\LocalService` with network socket privileges:
   ```powershell
   Get-Service -Name OnuslyDesktopAgent
   ```
3. **Corrupted Network Adapter Backup**:
   Inspect backup state at `C:\ProgramData\Onusly\config\dns_backup.json` (or `/etc/onusly/dns_backup.json`). Ensure file is readable and valid JSON.

---

## 5. Step-by-Step Remediation
1. **Restart Agent Service**:
   ```powershell
   Restart-Service -Name OnuslyDesktopAgent
   ```
   ```bash
   sudo systemctl restart onusly-agent
   ```
2. **Re-apply DNS Redirection**:
   Once the daemon is running cleanly:
   ```powershell
   onusly-agent dns redirect
   ```
3. **Trigger Immediate Policy Heartbeat**:
   ```powershell
   onusly-agent status
   ```

---

## 6. Post-Incident Verification
1. Verify `onusly-agent status` reports:
   ```
   Health State: HEALTHY
   DNS Resolver: HEALTHY (Running: true)
   ```
2. Verify resolution:
   - Forwarded query: `Resolve-DnsName -Name google.com -Server 127.0.0.1` succeeds with upstream IP.
   - Blocked query: `Resolve-DnsName -Name <blocked-domain> -Server 127.0.0.1` returns `127.0.0.1` (sinkhole).
3. Confirm Prometheus metrics `/metrics` reflect recovered heartbeats.

