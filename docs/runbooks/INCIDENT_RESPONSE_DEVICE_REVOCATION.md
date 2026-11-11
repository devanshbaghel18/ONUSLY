# Incident Response Runbook: Device Revocation & Unauthorized Access Remediation

## 1. Incident Classification & Severity
- **Incident Code**: `INC-SEC-003`
- **Severity**: **SEV-1 (Critical)** (Potential device theft, unauthorized access, or intentional de-authorization)
- **Component**: Device Identity, Device JWT Authentication, and Backend Enforcement Gateway
- **Impact**: All future policy synchronization and heartbeat reporting for the revoked device are rejected with HTTP 403.

---

## 2. Detection & Alerting Signals
1. **Dashboard & Web UI**:
   - Device displays `Revoked` badge in `Active Device & Browser Coverage`.
   - Aggregate goal enforcement flags `lapsed` if no other active paired devices remain.
2. **Prometheus Metrics**:
   - `onusly_auth_failures_total{reason="device_revoked"}` or `onusly_auth_failures_total{reason="revoked"}` elevated.
   - `onusly_lapse_episodes_started_total` increments if accountability goals lapse.
3. **Desktop Agent Status**:
   - `onusly-agent status` outputs:
     ```
     Health State:    DEVICE_REVOKED
     Pairing State:   REVOKED
     Status:          REVOKED (Device revoked by account owner. Run 'onusly-agent pair' to re-pair.)
     Enforcing:       false
     ```
4. **Structured Logs**:
   - Agent logs:
     ```json
     {"severity":"WARN","component":"auth","op":"sync","error_category":"auth","message":"device authorization has been revoked by the account owner"}
     ```

---

## 3. Operator & User Workflow: Legitimate vs Compromised Device

### Case A: Device Was Stolen, Lost, or Compromised
1. **Immediate Revocation Confirmation**:
   Verify in Dashboard that the device has status `REVOKED`. If not yet revoked, revoke immediately via:
   - Dashboard: Click **Revoke** on the compromised device card.
   - API: `POST /enforcement/devices/{deviceId}/revoke` with `{"reason":"compromised"}`.
2. **Account Token Invalidation**:
   The backend increments the user's `CredentialVersion`, invalidating all existing device JWTs issued to that device.
3. **Audit Verification**:
   Verify that any heartbeat or blocklist sync request from that device ID returns:
   ```json
   {"error":"device has been revoked by account owner"}
   ```

### Case B: Accidental Revocation or Hardware Recommissioning
If the user intentionally decommissioned the device or wants to securely re-authorize the same hardware:
1. **Preserve Canonical Device Identity**:
   Do NOT delete the local `device_id` file (`C:\ProgramData\Onusly\config\device_id` or `/etc/onusly/device_id`).
   ONUSLY's D8 re-pairing contract guarantees that re-pairing from the same machine reuses the canonical `device_id` and restores device history.
2. **Initiate Desktop Agent Pairing**:
   Run:
   ```powershell
   onusly-agent pair
   ```
3. **Confirm in Dashboard**:
   Enter the 6-character short code in the dashboard under **Pair ONUSLY Desktop Agent**.
4. **Verify Clean Transition**:
   - The device status transitions from `REVOKED` back to `PAIRED`.
   - New device JWT is issued and securely stored in Windows DPAPI / Linux keyring.
   - Initial blocklist sync is triggered immediately.

---

## 4. Post-Incident Verification
1. Run `onusly-agent status`:
   ```
   Health State: HEALTHY
   Pairing State: PAIRED
   Enforcing: true
   ```
2. Verify accountability partners receive automatic recovery notification (`enforcement.recovered`).

