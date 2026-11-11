# Incident Response Runbook: Application Enforcement Degradation

## 1. Incident Classification & Severity
- **Incident Code**: `INC-APP-004`
- **Severity**: **SEV-2 (High)** (Desktop application restriction failed to terminate or monitor processes)
- **Component**: Application Enforcement Controller (`desktop/agent/internal/apps`)
- **Impact**: A restricted desktop application (e.g. `discord.exe`, `steam.exe`) continues running despite active focus goals.

---

## 2. Detection & Alerting Signals
1. **Desktop Agent Status**:
   - `onusly-agent status` reports:
     ```
     Health State: APP_ENFORCEMENT_DEGRADED
     App Enforcer: DEGRADED (Enforcement Failed: > 0)
     ```
2. **CLI App Diagnostics**:
   - `onusly-agent apps status` reports:
     ```
     Subsystem Active:     ACTIVE
     Enforced Terminated:  ...
     Enforcement Failed:   > 0
     ```
3. **Structured Logs**:
   - Logs with:
     ```json
     {"severity":"ERROR","component":"apps","op":"terminate_process","error_category":"permission","message":"failed to terminate process 4120: Access is denied"}
     ```

---

## 3. Root Cause Diagnosis
1. **Process Privilege Boundary (Admin / Elevated Process)**:
   - If a target application was launched "Run as Administrator", and the agent service is running with insufficient elevation, `TerminateProcess` or `SIGKILL` fails with `Access is denied`.
   - **Remedy**: Verify `OnuslyDesktopAgent` runs as a high-integrity system service (`NT AUTHORITY\LocalService` with `SeChangeNotifyPrivilege` or `root` on Linux).
2. **Anti-Cheat / Kernel Protection**:
   - Games with kernel-level Anti-Cheat (e.g. EasyAntiCheat, BattlEye, Vanguard) block user-space process handle duplication (`PROCESS_TERMINATE`).
   - **Remedy**: Flag in goal notes or rely on DNS blocklist sinkhole for online multiplayer connectivity disruption.
3. **Target Typification Mismatch**:
   - Check if an Android package identifier (e.g. `com.discord`) was mistakenly evaluated as a desktop executable name.
   - **Remedy**: Verify target has explicit `desktop_app` typing in `applicationTargets`.

---

## 4. Remediation Steps
1. Re-scan application inventory:
   ```powershell
   onusly-agent apps scan
   ```
2. Inspect live process match diagnostics:
   ```powershell
   onusly-agent apps status
   onusly-agent apps diagnostics
   ```
3. If permissions were impaired, restart the service to refresh process handle privileges:
   ```powershell
   Restart-Service -Name OnuslyDesktopAgent
   ```

---

## 5. Post-Incident Verification
1. Verify `onusly-agent status` reports:
   ```
   Health State: HEALTHY
   App Enforcer: HEALTHY
   ```
2. Confirm `AppsFailedCount` is reset or ceases incrementing.

