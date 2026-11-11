# Incident Response Runbook: Stale Policy & Blocklist Synchronization Failures

## 1. Incident Classification & Severity
- **Incident Code**: `INC-SYNC-002`
- **Severity**: **SEV-3 (Moderate)** (Enforcement continues via Last-Known-Good cache, but goal updates are delayed)
- **Component**: Blocklist Syncer (`desktop/agent/internal/sync`) and Backend Control Plane (`/enforcement/blocklist`)
- **Impact**: Added or completed goals are not reflected on the client device until sync recovers.

---

## 2. Detection & Alerting Signals
1. **Prometheus Metrics**:
   - `onusly_sync_outcomes_total{outcome="error"}` or `onusly_sync_outcomes_total{outcome="rejected"}` rising.
   - `onusly_api_failures_total{path="/enforcement/blocklist"}` elevated.
2. **Desktop Agent Status**:
   - `onusly-agent status` outputs:
     ```
     Health State:     POLICY_STALE (or DEGRADED)
     Policy Freshness: STALE (LKG Active: true)
     Last Sync:        > 24 hours ago
     ```
3. **Structured Logs**:
   - Log entries with:
     ```json
     {"severity":"WARN","component":"sync","op":"fetch_blocklist","error_category":"network|auth","message":"failed to refresh blocklist from control plane..."}
     ```
4. **Dashboard Device Coverage**:
   - Device displays `Degraded` or `Policy Stale` warning tag.

---

## 3. Immediate Containment & Fail-Safe Verification
ONUSLY protects user commitments during network outages via **Last-Known-Good (LKG)** caching:
1. Verify LKG cache remains valid:
   ```powershell
   onusly-agent status
   ```
   Check that `Active Targets > 0` and `Cache Valid: true`.
2. Ensure the daemon has NOT flushed the cache to an empty state during transient HTTP 5xx or network drops.

---

## 4. Root Cause Diagnosis
1. **Network Connectivity & Partition**:
   Verify the desktop agent host can reach the API endpoint:
   ```powershell
   Invoke-WebRequest -Uri "http://localhost:8080/health" -Method GET
   ```
2. **Device Authentication Credentials**:
   Inspect whether the device JWT has expired or failed signature verification:
   - Check structured logs for: `error_category="auth"`
   - If device credential is invalid, HTTP 401 is returned.
3. **ETag / Conditional Request Deserialization**:
   If the server returns HTTP 304, verify that `304 Not Modified` is correctly recognized by the agent as a healthy cache confirmation rather than an error.

---

## 5. Step-by-Step Remediation
1. **Manual Synchronize Probe**:
   Trigger an explicit synchronization:
   ```powershell
   onusly-agent status
   ```
2. **If Credential Expired or Corrupted**:
   Initiate device re-pairing:
   ```powershell
   onusly-agent pair
   ```
   Enter the 6-character code in the dashboard. The backend preserves the canonical `device_id` and refreshes the device JWT.
3. **If Cache File Corrupted**:
   Inspect cache file at `C:\ProgramData\Onusly\data\blocklist_cache.json`. If unreadable, remove it; the agent will re-fetch a fresh copy from the control plane on the next sync cycle.

---

## 6. Post-Incident Verification
1. Verify `onusly-agent status` reports:
   ```
   Health State:     HEALTHY
   Policy Freshness: HEALTHY (LKG Active: true)
   Active Targets:   > 0
   Last Sync:        Just now
   ```
2. Confirm `onusly_sync_outcomes_total{outcome="success"}` increments in `/metrics`.

