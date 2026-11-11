# Runbook: Offline Operation, Policy Caching & Network Resilience

## 1. Overview
The ONUSLY Desktop Agent is designed for offline-first policy enforcement. Once a paired device synchronizes an authoritative policy snapshot from the backend, the policy is persisted locally in an atomic, crash-resilient disk cache.
- **Cache Path:** `C:\ProgramData\Onusly\policy_cache.json`
- **Ownership & ACLs:** Owned by `NT AUTHORITY\LocalService` and Administrators; Users have read-only access.
- **Persistence:** Survives system restarts, network disconnects, and service reboots.

---

## 2. Policy Cache Durability & Atomic Updates

### 2.1 Atomic Write Protocol
To prevent cache corruption during unexpected power outages or hard reboots, all disk writes adhere to the following sequence:
1. Write snapshot to a unique temporary file:
   `policy_cache.json.tmp.<PID>.<UnixNano>`
2. Issue `os.File.Sync()` (flushing OS buffers to physical disk).
3. Close the temporary file.
4. Atomically replace the target file via atomic rename (`os.Rename` / `MoveFileEx(MOVEFILE_REPLACE_EXISTING)`).

### 2.2 Cache Content
The persisted cache contains:
- Schema version (`version`).
- Backend synchronization ETag (`etag`).
- Timestamp of last successful sync (`cached_at`).
- Normalized target lists:
  - `domains`: List of restricted domain strings for DNS sinkhole.
  - `application_targets`: List of typed application objects (`target_type`, `identifier`, `display_name`).
  - `goal_states`: Map of goal IDs to status (`active`, `proof_submitted`, `rejected`, `completed`).

---

## 3. Offline Mode Behavior

### 3.1 Startup Without Network
1. When the agent service boots without internet access:
   - It reads and parses `policy_cache.json`.
   - It immediately activates DNS sinkholing for cached domains.
   - It activates native application monitoring and process termination for cached `desktop_app` targets.
2. The agent logs: `[ONUSLY Windows] Loaded offline cache: N target(s) (version ...)`.
3. The IPC status reports:
   - `pairing_state: PAIRED`
   - `sync_status: OFFLINE_CACHED`
   - `cache_valid: true`

### 3.2 Network Reconnection & Online Synchronization
- The agent maintains an active sync loop with exponential backoff and jitter (interval: 30s–300s).
- When network connectivity is restored:
  - The agent sends an HTTP `GET /api/v1/enforcement/authoritative` with `If-None-Match: <etag>`.
  - If policy has not changed, backend returns `304 Not Modified`: zero allocations, zero churn.
  - If policy has changed, backend returns `200 OK` with updated snapshot: the agent writes the new cache atomically and applies diffs to DNS and app controllers.

---

## 4. Cache Migration, Corruption Recovery & Health Semantics

### 4.1 Automatic Legacy Cache Migration
When upgrading from earlier versions that utilized `blocklist_cache.json`:
- If the primary cache path (`policy_cache.json` or `blocklist_cache.json`) does not exist, the cache manager automatically searches for the alternate legacy file in the data directory.
- If valid JSON is detected, it migrates the data into the primary path and sets `health.Migrated = true`.
- Upgrades preserve the user's active restrictions without requiring re-sync.

### 4.2 Last-Known-Good (LKG) Preservation & Fallback
On every successful atomic save, the cache manager updates a secondary Last-Known-Good snapshot: `policy_cache.json.lkg`.
- If the primary cache file is ever corrupted or damaged:
  1. The corrupt primary file is automatically quarantined to `policy_cache.json.corrupt`.
  2. The agent attempts fallback to `policy_cache.json.lkg`.
  3. If LKG is valid, it loads the last-known-good restrictions, maintaining offline protection while reporting `isLkg: true` and `degraded: true`.

### 4.3 Explicit Degradation Reporting & Heartbeat Lapsing
If both primary and LKG caches are missing or corrupted:
1. The agent boots in fail-open unblocked mode to prevent system lockouts.
2. The agent **strictly does NOT report `AUTHENTICATED & SYNCHRONIZED`**.
3. Status reports:
   - `Status: CONNECTED (ENFORCEMENT_DEGRADED - Fail-Open Active, Account: <user>)`
   - `Enforcing: false`
   - `Issues: [policy_missing, service_degraded]`
4. Heartbeat telemetry reports `enforcing: false` and `issues: ["policy_missing", "service_degraded"]`, immediately alerting the backend lapse pipeline (`enforcement.lapsed`).
5. As soon as network connectivity is available, the syncer issues a full sync without an `If-None-Match` tag, downloads a fresh authoritative snapshot, and restores `HEALTHY_ENFORCING` status.

### 4.4 Clearing Local Cache Manually
To force the agent to clear its cached policy:
```powershell
Stop-Service -Name OnuslyDesktopAgent
Remove-Item "C:\ProgramData\Onusly\data\blocklist_cache.json*" -Force -ErrorAction SilentlyContinue
Remove-Item "C:\ProgramData\Onusly\policy_cache.json*" -Force -ErrorAction SilentlyContinue
Start-Service -Name OnuslyDesktopAgent
```
Upon restart, the agent will fetch a fresh authoritative policy directly from the backend.

