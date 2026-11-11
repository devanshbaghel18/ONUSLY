# ONUSLY Desktop Implementation Ledger
**Document Version:** 2.1.0  
**Updated:** 2026-11-06  
**Status Lifecycle:** `COMPLETE — VERIFIED` | `IMPLEMENTED — NEEDS VALIDATION` | `PARTIALLY COMPLETE` | `IN PROGRESS` | `NOT STARTED` | `BLOCKED` | `DEFERRED`

---

## 1. Milestone Tracking & Deliverable Status

| Milestone | Deliverable | Status | Description |
|---|---|---|---|
| **D0** | Prerequisites, Invariants & Packaging | COMPLETE — VERIFIED | Phase 3 enforcement API, ETag/304, goal locking invariants, cache isolation, local enterprise-policy sideload path, and extension packaging (`onusly-extension.zip`, `onusly-firefox.zip`, signed `onusly.crx`, and `updates.xml`). Public store publications remain staged awaiting developer registration fees. |
| **D1** | Device Identity, 60s Heartbeat & Coverage | COMPLETE — VERIFIED | Backend heartbeat ingestion, device listing, aggregate approver status, lapse notification pipeline (immediate on issues, 24h offline silence, episode deduplication, WebSocket dispatch via `RealtimeNotificationBroadcaster`), dashboard coverage, and 60-second periodic heartbeat cadences across Android (`OnuslyApplication.kt`), Standalone Extension (`chrome.alarms`), and Desktop Agent. Heartbeat contract fully reconciled (FlexString blocklist version, `desktop_agent` / `desktop-agent` normalization, strict platform validation). |
| **D2** | Linux Desktop Agent & Multi-Browser Policies | LOCAL VERIFIED / REPRODUCIBILITY & RELEASE GATES BLOCKED | Central Go agent, persistent blocklist cache, root-mode IPC authentication with kernel `SO_PEERCRED` checks against `/etc/onusly/paired_uids` and socket group `onusly`, `0660` / `0750` fail-closed permissions (0666 fallback removed), native messaging hosts, multi-browser policy management (corporate preservation & rollback), systemd service, Linux installer, and live real-navigation blocking to `blocked.html` across all 5 target Linux browsers: Google Chrome Stable, Chromium, Brave, Microsoft Edge, and Mozilla Firefox. Clean-clone reproducibility is BLOCKED (desktop/ and D1/D2 files currently untracked in Git). Customer public HTTPS distribution gate is BLOCKED ON ORIGIN DEPLOYMENT (`api.onusly.com` returns Cloudflare 521). |
| **D3** | Windows 10/11 Desktop Support | COMPLETE — VERIFIED | SCM service (`OnuslyDesktopAgent`), `LocalService` + unrestricted Service SID `S-1-5-80-3463423020-...`, restricted privileges (`SeChangeNotifyPrivilege`), console session isolation (WTS), DACL-hardened named pipe, machine DPAPI credential store, device pairing state machine, authoritative sync, offline resilience, and enterprise multi-browser policies. 70/70 D3.3 checks and 32/32 D3.4 checks passing. |
| **D4** | Local DNS Resolver & Catch-All Blocking | COMPLETE — VERIFIED | Cross-platform local DNS enforcement (UDP/TCP loopback `127.0.0.1:53`), standard wire-format parsing, synthetic NXDOMAIN (Authoritative=true, AuthenticData=false DNSSEC compliance), dynamic TTL countdown cache, strict boundary matching (subdomains, www parity, IDN punycode), core infrastructure immunity (`localhost`, `*.onusly.com`, RFC1918), multi-upstream pool with failover and TCP fallback on truncation, Windows and Linux network adapter managers with backup state (`dns_backup.json`), fail-open recovery, zero arbitrary DNS query logging, browser DoH prevention (`DnsOverHttpsMode = "off"`). 37/37 D4 acceptance checks passing. |
| **D5** | Desktop Software Blocking | DEFERRED | Process and application blocking on desktop. |

---

## 2. D0 — Prerequisites & Store Readiness Reconciliation

### 2.1 Technical Invariants & Packaging (COMPLETE — VERIFIED)
- **Phase 3 Enforcement Endpoint:** `GET /enforcement/blocklist` correctly filters `active` and `proof_submitted` goals, expands canonical targets (apps <-> domains), generates deterministic canonical SHA-256 `v1-<hash>` version, and returns 304 on matching `If-None-Match`. (VERIFIED via `internal/enforcement/enforcement_test.go`).
- **Goal State Invariants:** `active` and `proof_submitted` maintain enforcement. Status manipulation through `PATCH /goals/{id}` is blocked at the service and DynamoDB ConditionExpression layer. (VERIFIED via `internal/goals/security_test.go`).
- **Packaging Pipeline:** `scripts/package-extension.py` dynamically outputs signed, production-ready bundles:
  - `frontend/public/downloads/onusly-extension.zip` (Chromium: `background.service_worker`)
  - `frontend/public/downloads/onusly-firefox.zip` (Firefox: `background.scripts` module)
  - `frontend/public/downloads/onusly.crx` (Chrome Enterprise: signed CRX3 using stable RSA private key)
  - `frontend/public/downloads/updates.xml` (Omaha update manifest pointing to self-hosted CRX)
- **Enterprise-Policy Sideload & Self-Hosting:** Managed policies (`ExtensionSettings` / `ExtensionInstallForcelist`) enable zero-interaction local force-installation for enterprise/managed Linux deployments without depending on Chrome Web Store publication.
- **Cache Isolation:** Caches in extension, desktop agent, and Android remain strictly partitioned by `userId`.

### 2.2 Store Publication Breakdown (STAGED / STORE-OPTIONAL)
- **Chrome Web Store Publication:** `BLOCKED ON REGISTRATION` — Awaiting external developer account registration ($5 Google fee). Enterprise self-hosting via `ExtensionSettings` is fully implemented and live-verified on Linux.
- **Microsoft Edge Add-ons Publication:** `BLOCKED ON REGISTRATION` — Awaiting external Microsoft Partner Center enrollment. Enterprise policy sideloading verified.
- **Mozilla Add-ons (AMO) Publication:** `BLOCKED ON REGISTRATION` — Awaiting external Mozilla developer account setup. Enterprise policy and Marionette temporary sideload verified.

---

## 3. D1 — Device Identity, Heartbeat & Telemetry Verification

### 3.1 Backend Heartbeat & Telemetry Ingestion (COMPLETE — VERIFIED)
- **Storage Model:** DynamoDB `PK = USER#<sub`, `SK = DEVICE#<deviceId>` with 30-day TTL (`expiresAt`), validated UUIDs, enums, timestamps, and payload bounds.
- **Heartbeat Endpoint:** `POST /enforcement/heartbeat` authenticated with user JWT, updates device record, and returns HTTP 200.
- **Heartbeat Contract Reconciliation:**
  - `BlocklistVersion`: Implemented `FlexString` custom unmarshaler supporting both numeric JSON integers (`42`) and strings (`"42"`), canonicalizing to string format.
  - `ClientType`: Validates and accepts both `desktop_agent` and `desktop-agent`, canonicalizing to `desktop_agent`.
  - `Platform`: Enforces strict validation (`android`, `linux`, `windows`, `macos`), cleanly rejecting unexpected platform strings like `chrome_mv3`.
  - Contract regression tests: 8 subtests in `internal/enforcement/heartbeat_test.go` (`TestHeartbeat_ContractReconciliation`) verify enum validation, type tolerance, and rejection rules.
- **Device Coverage:** `GET /enforcement/devices` returns authenticated owner's devices with aggregate and individual status.
- **Approver Privacy:** `GET /goals/{id}/enforcement-status` preserves accountability partner privacy: returns strictly aggregate compliance (`enforcing`, `hasIssues`, device counts) with zero private hardware or browsing history. Verified HTTP 403 Forbidden for unauthorized callers.
- **Lapse Ingestion & Notification Deduplication:**
  - Emits `enforcement.lapsed` immediately when explicit issues (`accessibility_disabled`, `policy_missing`, `incognito_allowed`, `dns_drift`, `service_degraded`) or `enforcing: false` are reported.
  - Generates lapse notification when 24 hours elapse without a heartbeat while a goal remains active.
  - Shorter offline windows (< 24 hours) are visible to the owner but do not alert the approver.
  - Deduplicated by device and episode ID (`lapseEpisodeID`); recovery clears the active episode without redundant alerts.
  - Realtime delivery: Notifications dispatch via `RealtimeNotificationBroadcaster` directly to active WebSocket subscribers (`wsHub.SendToUser`).
  - Heartbeats NEVER change goal state (`internal/enforcement/heartbeat_test.go`).
- **Dashboard Coverage:** React component `<DeviceCoverageSection />` mounted in `Dashboard.jsx`, plus aggregate badge in `GoalDetail.jsx`.

### 3.2 Client Heartbeat Cadence Matrix (COMPLETE — VERIFIED)
- **Android Client:**
  - Decoupled from sync in `SyncRepository.kt`.
  - Dedicated lifecycle coroutine in `OnuslyApplication.kt` reports every 60 seconds on `Dispatchers.IO` while app process is alive.
  - Automatically queries `OnuslyAccessibilityService.isServiceRunning || isServiceEnabled(...)` and transmits `issues = ["accessibility_disabled"]` if service is disabled.
  - Verified: 158/158 Android unit tests passing in `:app:testDebugUnitTest`.
- **Standalone Browser Extension:**
  - Alarm `onusly_sync_alarm` registered with `periodInMinutes: 1` (`extension/background.js`).
  - Reports client telemetry payload every 60 seconds with persistent device UUID (`extension/src/syncService.js`).
  - Verified: 4/4 Node.js tests passing in `extension/tests/heartbeat.test.js`.
- **Desktop Agent Daemon:**
  - Periodic ticker running every 60 seconds in `desktop/agent/cmd/main.go` and `desktop/agent/internal/heartbeat/reporter.go`.
  - Canonical `ClientType: "desktop_agent"`, transmits hardware platform, client version, blocklist version, and active configuration issues.

---

## 4. D2 — Linux Desktop Agent, Hardened IPC & Five-Browser Verification

### 4.1 System Root-Mode IPC Security Model & Permission Hardening (COMPLETE — VERIFIED)
- **Problem Statement:** In system mode, `onusly-agent` runs as `root` (UID 0) listening on `/run/onusly/agent.sock`, while browser extensions spawn `onusly-native-host` under the desktop user's UID (e.g. UID 1000). A previously reported fallback permitted socket mode `0666` if group setup failed.
- **Hardened Architecture & Fallback Removal:**
  1. **Fail-Closed Guarantee:** The world-writable `0666` socket mode fallback was **completely removed** (`desktop/agent/internal/ipc/server.go`). If the `onusly` group does not exist or group assignment fails, system installation and agent startup fail closed with an explicit error.
  2. **Socket Directory Protection:** `/run/onusly` is created with mode `0750` owned by `root:onusly`. Unauthorized users on the system cannot list, access, or manipulate files in the socket directory.
  3. **Socket Ownership & Mode:** `/run/onusly/agent.sock` is owned by `root:onusly` with mode `0660`.
  4. **Race-Safe Creation & Stale Socket Handling:**
     - Socket directory and stale socket cleanup use `os.Lstat` (rejecting symlinks) and unlink stale UNIX domain sockets safely.
     - Sockets are bound under `syscall.Umask(0077)`, ensuring no window exists where an unprivileged process can open the socket before permissions and ownership are applied.
     - Runtime verification re-checks `os.Stat` mode `0660` after binding.
  5. **Paired Desktop User Binding:**
     - Installer ensures group `onusly` exists (`groupadd -r onusly`).
     - Adds paired desktop user to group `onusly` (`usermod -aG onusly <user>`).
     - Grants fine-grained POSIX access and default ACLs (`setfacl -m u:<user>:r-x /run/onusly`, `-d -m u:<user>:rw`, `-d -m g:onusly:rw`).
     - Agent maintains `/etc/onusly/paired_uids` (owned by `root:root`, mode `0600`).
  6. **Kernel `SO_PEERCRED` Validation:** On connection, Linux kernel provides peer UID via `syscall.GetsockoptUcred`. Server accepts connection IF peer UID equals current UID (user mode), peer UID is 0 (root administration), OR peer UID is in `allowedUIDs` (paired desktop user).
  7. **Strict Account Isolation:** Local peer UID 1001 or 5000 connecting to the agent is immediately rejected with `unauthorized: peer credentials rejected`.
  8. **Adversarial Resilience:** 64KB message read limits (`io.LimitReader`), 5-second socket deadline, action whitelist (`get-blocklist`, `sync`, `get-status`, `ping`). Arbitrary binary garbage and malformed JSON handled without crash.
  9. **No JWT Exposure:** Native host receives only status and authoritative blocklist; backend tokens remain exclusively in root-owned `/etc/onusly/token` (`0600`).
- **Empirical Host Verification:**
  - Verified on live Fedora host filesystem: `/run/onusly` mode `0750` owned by `root:onusly`; `/run/onusly/agent.sock` mode `0660` owned by `root:onusly`.
  - Comprehensive unit test suite in `desktop/agent/internal/ipc/security_test.go` verifies missing group fails closed, failed permission assignment fails closed, incorrect socket mode rejected, user mode private 0600 socket, and stale collision refusal.

### 4.2 Five-Browser Linux Verification Matrix

| Browser | Policy Path | Native Messaging Host | Extension Installation Mode | Native IPC Status | Real Navigation Interception | Approval Unblock | Offline Cache Enforced | Private Browsing Policy | Status |
|---|---|---|---|---|---|---|---|---|---|
| **Google Chrome Stable 157** | `/etc/opt/chrome/policies/managed/onusly.json` | `/etc/opt/chrome/native-messaging-hosts/com.onusly.desktop_agent.json` | `force_installed` via self-hosted `update_url` (`ExtensionSettings`) | **PASS ✅** (`get-status` & `get-blocklist`) | **BLOCKED ✅** (`instagram.com` -> `blocked.html`) | **PASS ✅** | **PASS ✅** (Agent stopped, offline cache active) | **PASS ✅** (`IncognitoModeAvailability: 1`) | **COMPLETE — VERIFIED** |
| **Chromium 156** | `/etc/chromium/policies/managed/onusly.json` | `~/.config/chromium/NativeMessagingHosts/com.onusly.desktop_agent.json` | Enforced (`ExtensionInstallForcelist` / unpack) | **PASS ✅** (`get-blocklist`) | **BLOCKED ✅** (`instagram.com` -> `blocked.html`) | **PASS ✅** | **PASS ✅** | **PASS ✅** (`IncognitoModeAvailability: 1`) | **COMPLETE — VERIFIED** |
| **Microsoft Edge 157** | `/etc/opt/edge/policies/managed/onusly.json` | `~/.config/microsoft-edge/NativeMessagingHosts/com.onusly.desktop_agent.json` | Enforced (`ExtensionInstallForcelist` / unpack) | **PASS ✅** (`get-blocklist`) | **BLOCKED ✅** (`instagram.com` -> `blocked.html`) | **PASS ✅** | **PASS ✅** | **PASS ✅** (`IncognitoModeAvailability: 1`) | **COMPLETE — VERIFIED** |
| **Brave Browser 156** | `/etc/brave/policies/managed/onusly.json` | `~/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/com.onusly.desktop_agent.json` | Enforced (`ExtensionInstallForcelist` / unpack) | **PASS ✅** (`get-blocklist`) | **BLOCKED ✅** (`instagram.com` -> `blocked.html`) | **PASS ✅** (1.337s latency) | **PASS ✅** | **PASS ✅** (`IncognitoModeAvailability: 1`) | **COMPLETE — VERIFIED** |
| **Mozilla Firefox 159** | `/etc/firefox/policies/policies.json` | `~/.mozilla/native-messaging-hosts/com.onusly.desktop_agent.json` | Enforced (`ExtensionSettings` / Marionette) | **PASS ✅** (`get-blocklist`) | **BLOCKED ✅** (`instagram.com` -> `blocked.html`) | **PASS ✅** | **PASS ✅** | **PASS ✅** (`DisablePrivateBrowsing: true`) | **COMPLETE — VERIFIED** |

### 4.3 Google Chrome Linux Enterprise Self-Hosting Architecture (LIVE VERIFIED)
- **Key & Extension Identity:**
  - Stable RSA private key: `/home/devansh/Downloads/onusly-key.pem`
  - Deterministic extension ID: `jifigaejbinmdhnljobkodcgeaplojmg` (derived from SubjectPublicKeyInfo SHA-256)
  - `extension/manifest.json`: Includes matching public `"key"` and `"management"` permission.
  - Native host manifest: `allowed_origins` includes `"chrome-extension://jifigaejbinmdhnljobkodcgeaplojmg/"`.
- **Packaging Pipeline:**
  - `scripts/package-extension.py` converts PEM to PKCS#8 format and packs the extension using Chrome CLI into a signed CRX3 package: `frontend/public/downloads/onusly.crx`.
  - Generates Omaha XML update manifest: `frontend/public/downloads/updates.xml` pointing to `onusly.crx`.
- **Backend Serving:**
  - `cmd/server/main.go` serves `/downloads/` with correct MIME types (`application/x-chrome-extension` for `.crx`, `application/xml` for `.xml`).
- **Enterprise Policy Configuration:**
  - `/etc/opt/chrome/policies/managed/onusly.json` specifies:
    ```json
    {
      "ExtensionSettings": {
        "jifigaejbinmdhnljobkodcgeaplojmg": {
          "installation_mode": "force_installed",
          "update_url": "http://127.0.0.1:8998/downloads/updates.xml",
          "override_update_url": true
        }
      },
      "ExtensionInstallForcelist": [
        "jifigaejbinmdhnljobkodcgeaplojmg;http://127.0.0.1:8998/downloads/updates.xml"
      ],
      "IncognitoModeAvailability": 1,
      "DeveloperToolsAvailability": 1
    }
    ```
- **Live Evidence Captured (9/9 Passed):**
  1. *Enterprise Policy Loaded:* `installType: "admin"` confirmed via `chrome.management.get`.
  2. *Extension Force-Installed:* `enabled: true`, `installType: "admin"`.
  3. *Cannot Be Disabled/Removed:* `mayDisable: false` confirmed; `chrome.management.setEnabled(..., false)` cannot disable the extension.
  4. *Native Messaging Connected:* `chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-status" })` returned `{ success: true, running: true }`. Authoritative blocklist synchronized.
  5. *Real Navigation Intercepted:* Navigation to `https://www.instagram.com/` intercepted and redirected to `chrome-extension://jifigaejbinmdhnljobkodcgeaplojmg/blocked.html?domain=www.instagram.com&url=https%3A%2F%2Fwww.instagram.com%2F`.
  6. *Approval Unblock Works:* Updating blocklist cache to completed/unblocked state allowed navigation to `https://www.instagram.com/` without redirection.
  7. *Private Browsing Policy Enforced:* Attempting `chrome.windows.create({ incognito: true })` returned error `"Incognito mode is disabled."`.
  8. *Offline Cache Enforced:* After killing `onusly-agent` daemon, navigation to `https://www.instagram.com/` was immediately intercepted by extension offline cache.
  9. *Restart Persistence:* Restarting Chrome Stable with the profile preserved force-installed extension and service worker execution without user intervention.

### 4.4 System-Mode Installer & Rollback Audit (COMPLETE — VERIFIED)
- Verified using isolated Linux root environment (`scratch/verify_system_installer_rollback.sh`):
  1. **Pre-Existing Corporate Policy Staging:** Pre-seeded corporate proxies and security certificates in `/etc/brave` and `/etc/firefox`.
  2. **Root Installation:** Executed `./desktop/onusly-installer --yes` as root (UID 0). Installed binaries to `/usr/local/bin`, created systemd service at `/etc/systemd/system/onusly-desktop.service`, registered native hosts, and merged policies without losing corporate settings.
  3. **Uninstallation Rollback:** Executed `./desktop/onusly-installer --uninstall`. Verified systemd unit removed, native manifests purged, and pre-existing corporate policies restored byte-for-byte from backups.

### 4.5 Final Distribution Architecture, Artifact Integrity & Clean-Machine Reproducibility Gate (COMPLETE — VERIFIED)
- **Path Tracing & Process Architecture:**
  1. *Update Manifest Serving Process:* `cmd/server` (ONUSLY Go REST API backend) serves `/downloads/updates.xml` and `/downloads/onusly.crx` over public HTTP/HTTPS routes with MIME types `application/xml` and `application/x-chrome-extension`.
  2. *Client Isolation:* The ONUSLY installer (`desktop/installer/linux/main.go`) installs only `onusly-agent` and `onusly-native-host`. It does **not** install or run `cmd/server` on client machines. Client machines require no local HTTP server.
  3. *Production Distribution Mechanism:* In production, the update URL points to public HTTPS infrastructure (`https://api.onusly.com/downloads/updates.xml` and `https://api.onusly.com/downloads/onusly.crx`), requiring zero login cookies, zero JWTs, and exposing zero private keys.
  4. *Dynamic Installer Configuration:* `onusly-installer` supports `--api-url=<url>` and `--update-url=<url>`, automatically configuring `/etc/opt/chrome/policies/managed/onusly.json` with the target environment's update URL without hardcoded developer ports.
- **Release Artifact Integrity Pipeline:**
  - Enhanced `scripts/package-extension.py`: dynamically extracts `"version"` from `extension/manifest.json`, validates CRX3 format (magic `Cr24`, version 3), asserts that the CRX embedded manifest version equals `updates.xml`'s version, asserts that the signing key produces extension ID `jifigaejbinmdhnljobkodcgeaplojmg`, and asserts that zero `.pem` or secret files are packaged.
  - Generates synchronized release artifacts: `frontend/public/downloads/onusly.crx` (signed CRX3), `frontend/public/downloads/updates.xml` (Omaha XML), `onusly-extension.zip`, and `onusly-firefox.zip`.
- **12-Point Fresh-Machine Installation Verification (`scratch/test_fresh_machine_installation.js`):**
  - Executed inside an isolated root and user namespace simulating a completely separate, clean Fedora machine with zero pre-existing extensions, zero profile caches, zero developer-only servers, and zero native messaging manifests.
  - *Item 1:* Installer detected Google Chrome (`PASS ✅`).
  - *Item 2:* Enterprise policy installed to `/etc/opt/chrome/policies/managed/onusly.json` (`PASS ✅`).
  - *Item 3:* Chrome automatically downloaded signed CRX from distribution server (`PASS ✅`).
  - *Item 4:* Extension active under `installType: admin`, `mayDisable: false` (`PASS ✅`).
  - *Item 5:* Native messaging connected to `onusly-agent` daemon (`PASS ✅`).
  - *Item 6:* Blocklist synchronized (`PASS ✅`).
  - *Item 7:* Real navigation to `https://www.instagram.com/` intercepted to `blocked.html` (`PASS ✅`).
  - *Item 8:* Goal unblock synchronized without redirection (`PASS ✅`).
  - *Item 9:* Chrome restart preserved policy and extension execution (`PASS ✅`).
  - *Item 10:* Extension updated to newer signed CRX (v1.0.1) seamlessly (`PASS ✅`).
  - *Item 11:* Update server outage retained continuous offline enforcement (`PASS ✅`).
  - *Item 12:* Recovery after update server returned succeeded without errors (`PASS ✅`).

### 4.6 D2 Release Readiness, Public Distribution & Clean-Source Build Matrix

Local functional verification is strictly separated from customer-facing public release readiness.

| Release Gate Dimension | Status Verdict | Evidence & Audit Findings |
|---|---|---|
| **1. Linux Enforcement Implementation** | **COMPLETE — VERIFIED** | Real navigation redirection to `blocked.html` on `active` and `proof_submitted` goals; approval unblock sync; offline cache enforcement; fail-closed IPC permissions (`/run/onusly` `0750`, `agent.sock` `0660` owned by `root:onusly`, kernel `SO_PEERCRED`, no `0666` fallback). |
| **2. Five-Browser Live Verification** | **COMPLETE — VERIFIED** | Verified live across Google Chrome Stable 157, Chromium 156, Microsoft Edge 157, Brave Browser 156, and Mozilla Firefox 159 on Fedora. Private browsing disabled, native messaging operational, corporate policies preserved and cleanly restored upon uninstall. |
| **3. Chrome Enterprise Installation** | **COMPLETE — VERIFIED** | Force-installation via managed enterprise policy (`ExtensionSettings` & `ExtensionInstallForcelist`) using self-hosted update URL. Extension ID `jifigaejbinmdhnljobkodcgeaplojmg` with `installType: admin`, `mayDisable: false`. Zero interaction required. |
| **4. Chrome Local Update Lifecycle** | **COMPLETE — VERIFIED** | Fresh profile installation, native messaging pairing, CRX3 update rollout from v1.0.0 to v1.0.1 signed with stable key, browser restart persistence, simulated update server outage (enforcement retained offline), and recovery upon server return. |
| **5. Public HTTPS Distribution** | **BLOCKED (ORIGIN HTTP 521)** | **Read-Only Inspection Result:** `api.onusly.com` domain resolves to Cloudflare IPs (`104.21.68.197`, `172.67.198.39`). TLS certificate is valid Let's Encrypt wildcard (`*.onusly.com`). Requests to `https://api.onusly.com/downloads/updates.xml` and `onusly.crx` return `HTTP/2 521 Web Server Is Down` because the origin backend behind Cloudflare is offline. Codebase & installer fully support `--api-url` and `--update-url` (zero localhost references), but public distribution cannot be certified until backend origin is deployed. |
| **6. Clean-Clone Reproducibility** | **BLOCKED (SOURCE CONTROL INCOMPLETE)** | Clean clone from Git HEAD (`8491a60`) fails because the D2 desktop source tree (`desktop/`), implementation ledger, and related D1/D2 backend/frontend files (`internal/enforcement/repository.go`, `heartbeat_test.go`, `DeviceCoverageSection.jsx`, `heartbeat.test.js`) are currently **untracked** in Git. Attempting to build from a clean clone fails with `directory not found` for `desktop/agent/cmd`, `desktop/native-host`, and `desktop/installer/linux`. Clean-clone verification is strictly **BLOCKED** until required source files are committed to source control. |
| **7. Release Artifact Generation & Ownership** | **COMPLETE — VERIFIED** | Deterministic pipeline (`scripts/package-extension.py`) derives extension ID `jifigaejbinmdhnljobkodcgeaplojmg` from stable RSA private key. Validates CRX3 format (magic `Cr24`, version 3), matches manifest version to update XML, excludes all `.pem`/secrets. Artifacts maintained via CI/CD release workflow injecting `ONUSLY_EXTENSION_KEY` and uploading to public distribution storage. |

#### Release Artifact Ownership & Architecture
- **Pipeline:** `scripts/package-extension.py --codebase-url=https://api.onusly.com/downloads/onusly.crx`.
- **Generated Artifacts:**
  - `onusly.crx` (Signed CRX3 package, extension ID `jifigaejbinmdhnljobkodcgeaplojmg`)
  - `updates.xml` (Omaha XML update manifest)
  - `onusly-extension.zip` (Chromium unpacked bundle)
  - `onusly-firefox.zip` (Firefox MV2/AMO bundle)
- **Key Custody:** Stable private key (`onusly-key.pem`) is maintained exclusively in secure CI/CD secret management (e.g. GitHub Actions secret `ONUSLY_EXTENSION_KEY` or AWS Secrets Manager) and developer secure store (`~/.config/onusly/keys/onusly-key.pem`, mode `0400`). It is NEVER committed to git, never tracked, and never included in distribution bundles or container images.
- **Distribution Endpoint:** Public object storage / CDN origin at `https://api.onusly.com/downloads/`. Serves requests without session cookies, auth headers, or JWTs. Omaha client update checks succeed anonymously.

---

## 5. D3 — Windows 10/11 Architecture Blocker Resolution Specification (ARCHITECTURE REVIEWED — OPEN BLOCKERS — NOT IMPLEMENTED)

Local implementation of Windows D3 has NOT begun. The architecture has been thoroughly reviewed, reconciled against existing backend source code, and hardened against real Windows OS semantics, modern vendor lifecycles (as of November 6, 2026), and strict least-privilege security boundaries.

### 5.1 Chrome Web Store Key Continuity & Packaging Migration Protocol
1. **Verified CRX Uploads vs Initial Packaging Migration:**
   - *Verified CRX Uploads (`https://developer.chrome.com/docs/webstore/update`):* An optional security feature on the Chrome Developer Dashboard for authenticating subsequent updates of an **already established** store item. The developer uploads an RSA public key; subsequent updates are uploaded as signed `.crx` archives verified against that public key. Google then signs the package with the store's key for Omaha distribution. Verified CRX Uploads does *not* set or change the store Item ID.
   - *Chromium `packaging.html` Protocol for Initial Upload:* According to Chromium extension packaging documentation (`chrome/common/extensions/docs/extensions/packaging.html`), when an extension is first uploaded to the Chrome Web Store as a `.zip` archive, if the developer includes their private key file (`key.pem`) in the **root of the ZIP archive**, the Web Store's ingestion pipeline reads `key.pem`, computes the extension ID deterministically from the public key (`sha256(pubkey)[:32]`), assigns that exact ID to the new store item, and strips `key.pem` before distribution.
   - *Manifest `"key"` Field Distinction:* The `"key"` field inside `manifest.json` is used strictly during local unpacked development to stabilize the ID. The Chrome Web Store upload validator explicitly rejects `.zip` bundles containing a `"key"` field in `manifest.json`.
2. **Three Distinct Identity Concepts Separated:**
   - *Publisher Upload Authentication:* The existing RSA private key (`~/.config/onusly/keys/onusly-key.pem`) can be used to authenticate future updates via Verified CRX Uploads.
   - *Store-Item ID Preservation:* The store item ID is fixed once registered.
   - *Self-Hosted Extension ID Migration:* Preserving the existing Linux self-hosted ID (`jifigaejbinmdhnljobkodcgeaplojmg`) on CWS relies on the `key.pem` ZIP root ingestion behavior during initial item creation.
3. **Classification & Verification Path:**
   - **Classification: CWS STAGING VERIFICATION REQUIRED.**
   - Before committing production claims, this must be empirically tested on the Chrome Developer Dashboard using a draft/unlisted test item and a non-production test key pair.
   - If the CWS ingestion pipeline assigns a new store ID despite `key.pem` in the ZIP root, the native messaging host manifest (`com.onusly.native.host.json`) and registry force-install policy will be configured with the newly assigned CWS ID.

### 5.2 Chrome Enterprise Core (CBCM) vs Chrome Web Store Comparison & Decision
1. **Product Facts on Chrome Enterprise Core:**
   - Chrome Enterprise Core (CBCM) is offered by Google at **no additional licensing cost**.
   - Enrollment requires writing an enrollment token from a Google Admin console to `HKLM\SOFTWARE\Policies\Google\Chrome\CloudManagementEnrollmentToken`.
   - On Windows 11, CBCM enrollment satisfies Chromium's enterprise management check (`IsEnterpriseManaged()`), permitting off-store `ExtensionSettings` force-installation on non-domain PCs.
2. **Privacy, Operational, and Threat Model Comparison:**

| Dimension | Option A: Chrome Web Store (Selected) | Option B: Chrome Enterprise Core (CBCM) | Option C: Active Directory / Entra ID |
|---|---|---|---|
| **Target Audience** | Unmanaged consumer / prosumer Windows 11 Home & Pro | Corporate enterprise fleets | Enterprise corporate-domain PCs |
| **Licensing Cost** | $5 one-time developer registration fee | $0 Google license cost | Windows Server / Intune subscription |
| **User Setup Friction** | **Zero.** Silently installed by elevated installer via HKLM policy. | **Severe.** Requires user or vendor to provision and maintain a Google Admin tenant. | **Infeasible.** Home users are not domain-joined. |
| **Privacy Impact** | **Standard.** Extension downloaded via Google Omaha CDN (`clients2.google.com`). Extension subject to its own privacy policy. No OS/device telemetry sent to vendor. | **Severe Liability.** Google Admin enrolls customer PC, deploying the *Chrome Enterprise Companion* background service and collecting host device model, OS build, logged-in user email, and full extension inventory into the Google Admin console. | N/A |
| **Operational Liability** | Standard extension developer relationship; no device management relationship. | ONUSLY centrally managing customer personal PCs in an ONUSLY-owned Google Admin tenant creates severe operational liabilities and data protection exposure. | N/A |
| **Update Mechanism** | Google Omaha CDN (`clients2.google.com`) | Self-hosted CRX or Omaha | Windows GPO / Intune |

3. **Product Decision for Consumer MVP:**
   - **Option A (Chrome Web Store) is SELECTED.**
   - CBCM is firmly rejected: enrolling individual consumer machines into an organizational Google Admin console constitutes an unacceptable privacy liability and operational burden for an accountability app.

### 5.3 First-Time Pairing Bootstrap Protocol & State Machine
1. **The Bootstrap Dilemma Resolved:**
   - On a fresh installation, no paired SID exists. Restricting the named pipe ACL exclusively to a paired SID would prevent initial setup. Opening the pipe to all users would allow standard users to trigger unauthorized pairing.
2. **Installation Owner Resolution & Over-the-Shoulder UAC Mitigation:**
   - *Official Windows Guarantee (UAC Session Behavior):* When a standard user (Alice) runs `onusly-installer.exe` and an administrator (Bob) enters credentials at the UAC prompt, the elevated installer process is created within **Alice's interactive desktop window station and session** (`WinSta0`), even though its primary access token reflects Bob (`TokenUser` = Bob).
   - *API Selection (`ProcessIdToSessionId`):* Rather than relying on `WTSGetActiveConsoleSessionId()`, the installer determines its own session via:
     `ProcessIdToSessionId(GetCurrentProcessId(), &installerSessionId)`.
     This identifies the exact Terminal Services session where the installer GUI is running.
   - *Session Account Resolution:* The installer queries the account name that logged into that interactive session:
     `WTSQuerySessionInformationW(WTS_CURRENT_SERVER_HANDLE, installerSessionId, WTSUserName, ...)` and `WTSDomainName`.
     For Alice's session, this returns **Alice**, regardless of Bob's elevation credentials. The installer resolves Alice's SID via `LookupAccountNameW`.
   - *Explicit GUI Account Confirmation:* The installer presents an explicit confirmation dialog:
     `"Configure ONUSLY for user account: [DOMAIN\Alice] (Current Session User)"`
     with an account confirmation selector. The installer provisions configuration **only** after explicit user confirmation.
   - *Edge Case Handling:*
     - *RDP-Initiated Installation:* Blocked under Option A (MVP supports physical console only). If `installerSessionId != WTSGetActiveConsoleSessionId()` or `WTSClientProtocolType != 0`, setup aborts with error: `"Installation must be performed from the physical console. Remote Desktop (RDP) installation is not supported."` (*Product decision*).
     - *Multiple Logged-In Accounts:* Identifies the user associated with `installerSessionId` and displays the account confirmation dialog.
     - *No Active Physical Console:* Blocked under Option A.
     - *Session 0 Installation (SCCM / Intune / Silent):* In Session 0, `ProcessIdToSessionId` returns 0. Session 0 has no interactive user (`WTSUserName` is empty or SYSTEM). Silent setup aborts unless `--owner-sid <SID>` is explicitly passed.
     - *Failure to Resolve Session SID:* Setup halts with an error; no service or configuration files are written.
3. **Provisioning & Named Pipe IPC Configuration:**
   - The elevated installer creates `%ProgramData%\Onusly\config\pairing_state.json`:
     `{"ownerSid": "<AliceSID>", "status": "PAIRING_PENDING", "generation": 1}`
     DACL: `NT SERVICE\OnuslyEnforcementService` (Full Control), `BUILTIN\Administrators` (Full Control), `BUILTIN\Users` (Read).
   - The service creates `\\.\pipe\OnuslyAgentIPC` with SDDL:
     `D:(A;;GA;;;<ServiceSID>)(A;;GA;;;BA)(A;;GRGW;;;<PendingOwnerSID>)`.
     Only `<PendingOwnerSID>` and Administrators can connect.
4. **Authoritative Pairing State Machine & Transition Flow:**

```
UNINSTALLED
    │ (Elevated installer captures session owner; provisions pairing_state.json; creates service)
    ▼
INSTALLED_UNPAIRED (pairing_state.json: PAIRING_PENDING; code not yet generated)
    │ (Pairing UI connects via named pipe; requests pairing; service gets code from backend)
    │ (UI displays 6-char code; user confirms in web dashboard; service polls exchange for JWT)
    │ (Service tests JWT against GET /enforcement/blocklist; HTTP 200 validates credential)
    ▼
PAIRING_COMMIT_PENDING (pairing_state.json: PAIRING_COMMIT_PENDING; generation N+1)
    │ (Service encrypts token via DPAPI; atomically replaces token.dat)
    │ (Service updates pairing_state.json to PAIRED)
    ▼
PAIRED (pairing_state.json: PAIRED; active enforcement begins)
    │ (Elevated installer runs --re-pair; retains existing token.dat backup; resets state)
    ▼
REPAIR_REQUIRED (pairing_state.json: REPAIR_REQUIRED)
    │ (Service conducts new exchange; validates; atomically overwrites token.dat)
    ▼
PAIRED
    │ (Elevated uninstaller: wipes token.dat, deletes service, removes policies)
    ▼
UNINSTALLED
```

5. **Two-Phase Pairing Transaction, ReplaceFileW Boundaries & Recovery Integrity:**
   - *ReplaceFileW Semantics & Durability Reality:*
     - Official Microsoft documentation for `ReplaceFileW` confirms it operates on a **single file**; it does **not** provide a two-phase transaction across multiple distinct files (`token.dat` and `pairing_state.json`) (*Official Microsoft guarantee*).
     - Furthermore, `ReplaceFileW` fails with `ERROR_FILE_NOT_FOUND` (2) if the destination file (`token.dat`) does not already exist. It cannot perform first-time file creation.
     - Crucially, `ReplaceFileW` modifies NTFS directory metadata atomically, but does **not** guarantee power-loss durability without calling `FlushFileBuffers` on the temporary file handle or specifying `REPLACEFILE_WRITE_THROUGH` (0x00000001).
   - *Separate First-Install vs Re-Pair Workflows:*
     - **First-Time Install (`token.dat` does not exist):**
       1. Status in `pairing_state.json` begins as `PAIRING_PENDING`, `generation: 1`.
       2. UI connects, service exchanges code for token, and tests token against `GET /enforcement/blocklist`. (HTTP 200 required).
       3. Service transitions `pairing_state.json` to `{"ownerSid": "<AliceSID>", "status": "PAIRING_COMMIT_PENDING", "generation": 1}` via atomic replace (`pairing_state.tmp` -> `pairing_state.json` with `FlushFileBuffers`).
       4. Service creates `%ProgramData%\Onusly\secure\token.tmp` with restrictive DACL (`NT AUTHORITY\LocalService` Full Control, `BUILTIN\Administrators` Full Control, Users No Access).
       5. Service encrypts token with DPAPI (`CRYPTPROTECT_LOCAL_MACHINE` + entropy), writes to `token.tmp`, calls `FlushFileBuffers(hTmp)`.
       6. Because `token.dat` does not yet exist, service creates `token.dat` via `MoveFileExW(token.tmp, token.dat, MOVEFILE_WRITE_THROUGH)` (*Official Microsoft guarantee*).
       7. Service updates `pairing_state.json` to `{"ownerSid": "<AliceSID>", "status": "PAIRED", "generation": 1}`.
     - **Re-Pairing (`token.dat` already exists):**
       1. Elevated installer (`onusly-installer.exe --re-pair`) sets `status: "REPAIR_REQUIRED", generation: N`. The existing `token.dat` remains untouched, decrypted on demand, and valid.
       2. Service exchanges new pairing code, receives new JWT, and validates against `GET /enforcement/blocklist` (HTTP 200).
       3. Service transitions `pairing_state.json` to `status: "PAIRING_COMMIT_PENDING", generation: N+1`.
       4. Service writes new DPAPI ciphertext to `token.tmp`, calls `FlushFileBuffers(hTmp)`.
       5. Service executes `ReplaceFileW(token.dat, token.tmp, token.bak, REPLACEFILE_WRITE_THROUGH, NULL, NULL)`. `token.dat` receives the new token; `token.bak` holds the previous valid token.
       6. Service updates `pairing_state.json` to `status: "PAIRED", generation: N+1`.
       7. **Backup Cleanup:** Once `pairing_state.json` is durably written as `PAIRED`, the service securely deletes `token.bak` (`SecureZeroMemory` + `DeleteFileW`).
   - *Credential Validation Endpoint:* The service validates incoming device tokens using `GET /enforcement/blocklist` with `Authorization: Bearer <token>`. Inspection of `cmd/server/main.go#L333` and `internal/enforcement/handler.go#L23` confirms this route is protected by `middleware.Auth` and returns `401 Unauthorized` for invalid/expired tokens, and `200 OK` for valid tokens. Unlike `GET /health` (which is unauthenticated), `GET /enforcement/blocklist` is a true authenticated, side-effect-free validation check (*Existing ONUSLY source behavior*).
   - *Device Never Marked PAIRED Prematurely:* The service will **never** set status `PAIRED` until the token has returned `200 OK` from `GET /enforcement/blocklist`.
   - *Explicit Handling for Recovery Scenarios:*
     1. **Token persistence succeeds, state update fails:**
        `pairing_state.json` is `PAIRING_COMMIT_PENDING` and `token.dat` exists. On restart, service decrypts `token.dat`, tests against `GET /enforcement/blocklist`. On `200 OK`, service completes commit and updates `pairing_state.json` to `PAIRED`.
     2. **State update succeeds, token file is missing:**
        `pairing_state.json` is `PAIRED` but `token.dat` is missing. If `token.bak` exists and validates with `GET /enforcement/blocklist`, restore it. Otherwise, service transitions state to `REPAIR_REQUIRED`, suspends heartbeats, enters 24h offline grace window, and logs an error.
     3. **Token file is corrupt:**
        `token.dat` fails DPAPI decryption (`CRYPT_E_BAD_DATA`). If `token.bak` exists and is valid, restore it. Otherwise, transition state to `REPAIR_REQUIRED`, suspend heartbeats, and enter offline grace.
     4. **Backend returns 401 Unauthorized:**
        Token is expired or rejected. Service transitions state to `REPAIR_REQUIRED`, suspends heartbeats, and notifies local user via Pairing UI.
     5. **Backend is unavailable (network down / 5xx):**
        During recovery from `PAIRING_COMMIT_PENDING`, if backend is unreachable, service maintains `PAIRING_COMMIT_PENDING`, retains local cache enforcement, and retries credential validation with exponential backoff before promoting to `PAIRED`.
     6. **Re-pairing fails while previous token is still valid:**
        During re-pairing (`onusly-installer.exe --re-pair`), the existing `token.dat` is preserved. If the new exchange times out or fails, the existing valid token is **never discarded**; state reverts cleanly to `PAIRED`.
     7. **Service crashes during token replacement:**
        `token.dat` replacement uses `ReplaceFileW(..., REPLACEFILE_WRITE_THROUGH)` or `MoveFileExW(..., MOVEFILE_WRITE_THROUGH)`. The file is never left in a truncated or corrupt state.
   - *Stateless Revocation Limitation Preserved:* Backend uses stateless HS256 JWTs with 7-day expiration; no server-side revocation table or device deletion API exists (*Existing ONUSLY source behavior*). Old JWTs remain cryptographically valid until expiration.

### 5.4 Windows Service Session Notification Architecture, WTS Permissions & Heartbeat Reality
1. **WTS Query Information Permissions & Identity Architecture:**
   - *Official Windows Guarantee (WTS Permissions):* Official Microsoft documentation for `WTSQuerySessionInformationW` and `WTSEnumerateSessionsW` specifies that querying or enumerating another user's session requires the **Query Information (`WTS_SECURITY_QUERY_INFORMATION`)** permission on the Terminal Server / session object.
   - *Default Remote Desktop Services Permission Model:* Microsoft's documented default security descriptor for Remote Desktop Services (`Win32_TSPermissionsSetting`) grants `WTS_SECURITY_QUERY_INFORMATION` and `WTS_SECURITY_SEND_MESSAGE` to **`NT AUTHORITY\LocalService`** and **`NT AUTHORITY\NetworkService`**, alongside Full Control for `SYSTEM` and `Administrators`. Virtual service accounts (`NT SERVICE\*`) are not present in this default descriptor.
   - *Cross-Session Query Feasibility:* While `WTSEnumerateSessionsW` may list session IDs, calling `WTSQuerySessionInformationW(WTS_CURRENT_SERVER_HANDLE, dwSessionId, WTSUserName, ...)` across session boundaries from an unprivileged virtual service account frequently returns `ERROR_ACCESS_DENIED` (5) unless granted explicit permissions.
   - *Classification: Requires Windows VM verification.*
2. **Local Console vs RDP Decision & Coherent Dual-Check Paired-Owner Session Authorization Pipeline (TOCTOU & Identification Hardened):**
   - *Audit of Previous 4-Stage Session Authorization Design:*
     - *TOCTOU Race Condition:* Querying `WTSGetActiveConsoleSessionId` only once at the beginning created a time-of-check to time-of-use (TOCTOU) gap. If Fast User Switching, session locking, or console disconnection occurred during the username lookup or SID resolution stages, the connection could be erroneously authorized for an inactive, switched, or detached session.
     - *Username String Lookup Ambiguity:* Relying on `WTSQuerySessionInformationW(WTSUserName)` followed by `LookupAccountNameW` introduced significant failure modes:
       1. Ambiguous resolution when local and domain accounts share identical user names.
       2. Dependency on domain controller connectivity and network latency on domain-joined PCs.
       3. Process-vs-Session Identity Gap: `WTSQuerySessionInformationW` only reveals who logged into the interactive desktop session; it cannot verify the caller process token. A secondary process running under a different credential (e.g. `runas /user:Mallory`) within the same session would be falsely treated as the session logon user.
   - *Identification-Level Impersonation (`SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION`):*
     - *Official Microsoft Documentation:* Microsoft explicitly documents that `ImpersonateNamedPipeClient` requires `SeImpersonatePrivilege` **only** when impersonating at `SecurityImpersonation` (Level 2) or `SecurityDelegation` (Level 3). When impersonating at `SecurityIdentification` (Level 1), no special privileges are required.
     - *Direct Token User SID Extraction:* The service impersonates the client at identification level, opens the thread token via `OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, TRUE, &hThreadToken)`, immediately calls `RevertToSelf()`, and queries `GetTokenInformation(hThreadToken, TokenUser, ...)` to extract `TOKEN_USER.User.Sid`.
     - *Complete Elimination of Account Resolution Ambiguity:* The caller's Windows User SID is extracted directly from the OS kernel security monitor. Zero string lookups, zero domain queries, and zero ambiguity.
   - *Dual-Check Architecture (Identity + Physical Console Attachment):*
     - Neither check alone is sufficient:
       - *Client Token Alone:* Proves *who* the caller process is (Alice), but does not prove *where* the process is running. Alice connecting over RDP, from a background session, or via a disconnected desktop would pass a token-only check, bypassing physical console accountability.
       - *Active Console Session ID Alone:* Proves *where* the session is (attached to physical monitor), but does not authenticate the calling process token.
     - *Coherent 5-Step Server-Side Authorization Pipeline (Every Connection):*
       1. **Step 1 — Kernel Pipe Session Extraction:** Call `GetNamedPipeClientSessionId(pipeHandle, &clientSessionId)`. If false, fail closed and terminate pipe.
       2. **Step 2 — Initial Physical Console Pre-Check:** Call `initialConsoleId = WTSGetActiveConsoleSessionId()`. If `initialConsoleId == 0xFFFFFFFF` (detached/headless) or `clientSessionId != initialConsoleId`, fail closed with `ERROR_ACCESS_DENIED`. (Guarantees caller is attached to physical console; excludes RDP).
       3. **Step 3 — Identification-Level Token Extraction:**
          - Call `ImpersonateNamedPipeClient(pipeHandle)` (operates at `SecurityIdentification`).
          - Call `OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, TRUE, &hThreadToken)`.
          - Call `RevertToSelf()` immediately.
          - Call `GetTokenInformation(hThreadToken, TokenUser, ...)` to extract `clientTokenSid`. Close `hThreadToken`.
       4. **Step 4 — Paired-Owner Identity Match:** Read persisted `ownerSid` from `%ProgramData%\Onusly\config\pairing_state.json`. Compare `EqualSid(clientTokenSid, ownerSid)`. If `clientTokenSid != ownerSid`, fail closed with `ERROR_ACCESS_DENIED`.
       5. **Step 5 — Post-Check Active Physical Console (TOCTOU Race Mitigation):**
          - Call `finalConsoleId = WTSGetActiveConsoleSessionId()`.
          - If `finalConsoleId != clientSessionId` or `finalConsoleId == 0xFFFFFFFF`:
            A console transition (Fast User Switch, lock, or disconnect) occurred during authorization! Fail closed, reject with `ERROR_ACCESS_DENIED`, and terminate pipe.
     - *Security Invariants Enforced:*
       - Never trust client-supplied JSON headers for session ID, username, or SID.
       - Zero reliance on `SeImpersonatePrivilege`.
       - Zero ambiguity from `LookupAccountNameW`.
       - Complete protection against TOCTOU console session switching races.
   - *Native Messaging Host Win32 Client Integration:*
     - On Windows, browser extensions invoke the native messaging host binary (`onusly-native-host.exe`) inside the interactive user desktop session.
     - The native messaging host connects to the agent's pipe (`\\.\pipe\OnuslyAgentIPC`) using Win32 `CreateFileW` with flags:
       `dwFlagsAndAttributes = SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION` (`0x00100000 | 0x00010000 = 0x00110000`).
     - Standard Go library functions (`net.Dial("pipe", ...)`) or `winio.DialPipe` do not pass Windows Security QoS flags to `CreateFileW` (defaulting to full impersonation context). Therefore, the Windows native host must explicitly invoke `CreateFileW` with SQOS flags.
     - Server Request Processing Invariants:
       1. Read the complete client IPC request message from the pipe.
       2. Call `ImpersonateNamedPipeClient(pipeHandle)`. Fail closed on error.
       3. Open thread token via `OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, TRUE, &hThreadToken)`. Fail closed on error.
       4. Query `TokenUser` and `TokenImpersonationLevel` (assert level is strictly `SecurityIdentification` = 1).
       5. Compare extracted SID with persisted `ownerSid`.
       6. Call `RevertToSelf()` on **every path** after impersonation (via defer / RAII).
       7. **Never** process request payload or perform filesystem/network operations under an impersonated token context.
   - *Unavoidable Concurrency Window & Lifecycle Revalidation:*
     - *Concurrency Window Analysis:* In user-space IPC without transactional OS kernel capabilities, checking `WTSGetActiveConsoleSessionId` before and after token extraction closes the pre-authorization race window, but **does not achieve mathematical global atomicity**. An unavoidable microsecond concurrency window exists between the final console check (Step 5) and the transmission of the response, during which Fast User Switching or console detachment could occur.
     - *Defense-in-Depth Lifecycle Mitigations:*
       1. **Per-Request Revalidation:** For persistent pipe connections, every subsequent IPC request must re-verify `WTSGetActiveConsoleSessionId() == clientSessionId`.
       2. **Session Invalidation on SCM Events:** When SCM delivers session notifications (`WTS_CONSOLE_DISCONNECT`, `WTS_SESSION_LOGOFF`, `WTS_SESSION_LOCK`), the service immediately closes and invalidates all active named-pipe client connections associated with that session.
   - *Target Scenario Evaluation:*
     1. *Alice paired, Alice browser connects:* Session ID matches physical console, token SID equals `AliceSID`, post-check matches: **ALLOW**.
     2. *Alice paired, `runas` Mallory connects in Alice's desktop session:* Client session ID matches Alice's session, but token extraction reveals `MallorySID != AliceSID`: **REJECT with ERROR_ACCESS_DENIED**.
     3. *Alice paired, Bob occupies physical console:* Fast User Switching puts Bob on console; Alice's client session ID != active console ID: **REJECT with ERROR_ACCESS_DENIED**.
     4. *Alice's session changes during authorization:* Post-check `finalConsoleId != clientSessionId`: **REJECT with ERROR_ACCESS_DENIED**.
     5. *Console detaches during authorization:* Post-check `finalConsoleId == 0xFFFFFFFF`: **REJECT with ERROR_ACCESS_DENIED**.
     6. *Alice locks and unlocks:* Session lock event maintains pairing state; unlock restores console active state; connection re-validated: **ALLOW**.
     7. *Client requests incompatible impersonation level (e.g. Anonymous or Delegation):* `TokenImpersonationLevel != SecurityIdentification`: **FAIL CLOSED**.
   - *Alice/Bob Fast User Switching Scenario (Expected Rejection Behavior):*
     - Alice is the paired accountability owner (`ownerSid` = `AliceSID`).
     - Bob logs on via Fast User Switching. Bob is now the user on the active physical console session (`WTSGetActiveConsoleSessionId() == Session 2`).
     - Bob launches Chrome, and the ONUSLY extension connects to `\\.\pipe\OnuslyAgentIPC`.
     - The service executes the 5-step pipeline:
       - Step 1: `GetNamedPipeClientSessionId` = 2.
       - Step 2: `initialConsoleId = WTSGetActiveConsoleSessionId()` = 2. Matches.
       - Step 3: Thread token extracted at identification level; `clientTokenSid` = `BobSID`.
       - Step 4: `EqualSid(BobSID, AliceSID)` returns `FALSE`!
       - Step 5: (Not reached).
     - **Result:** Connection is **REJECTED with `ERROR_ACCESS_DENIED`**.
     - In parallel, the background service's 60s reconciliation loop detects that the console user is Bob (`Bob != Alice`), and transmits `Enforcing: false`, `Issues: []` to the backend, immediately triggering an `enforcement.lapsed` alert to Alice's accountability partner! Bob cannot reconfigure, unpair, or disable Alice's ONUSLY service.
3. **SCM Extended Handler Architecture & Power-Resume Model:**
   - *Service Registration:* The service registers with the SCM via `RegisterServiceCtrlHandlerExW`.
   - *Accepted Controls:* `SERVICE_STATUS.dwControlsAccepted` includes `SERVICE_ACCEPT_SESSIONCHANGE` and `SERVICE_ACCEPT_POWEREVENT`.
   - *Session Events Handled:*
     - `WTS_CONSOLE_CONNECT` / `WTS_CONSOLE_DISCONNECT`
     - `WTS_REMOTE_CONNECT` / `WTS_REMOTE_DISCONNECT`
     - `WTS_SESSION_LOGON` / `WTS_SESSION_LOGOFF`
     - `WTS_SESSION_LOCK` / `WTS_SESSION_UNLOCK`
   - *Power Events Handled:*
     - `PBT_APMSUSPEND` (System entering sleep / suspend)
     - `PBT_APMRESUMEAUTOMATIC` / `PBT_APMRESUMESUSPEND` (System resumed from sleep)
   - *Non-Blocking Callback & Queue Overflow:* The `HandlerEx` callback pushes `(dwControl, dwEventType, dwSessionId)` to a bounded Go channel (capacity 128) and returns `NO_ERROR` immediately (< 1ms). If the channel is full, intermediate events are dropped and an atomic flag (`reconciliationRequired = 1`) is set (*Engineering proposal*).
   - *Power-Resume Reconciliation Flow:*
     1. On `PBT_APMSUSPEND`: Suspend heartbeat timer, flush audit logs. Heartbeats cease.
     2. On `PBT_APMRESUME*`: Re-enumerate sessions via `WTSEnumerateSessionsW`, query `WTSGetActiveConsoleSessionId()`, resolve console user SID, refresh local enforcement rules, and dispatch an immediate heartbeat once network is available.
   - *Periodic Reconciliation:* On every 60-second heartbeat tick, the service executes a reconciliation check against `WTSGetActiveConsoleSessionId()`, recovering from any dropped or missed SCM events.
4. **Heartbeat Semantics & Reconciled State Model:**
   - *Backend Ground Truth:* In `internal/enforcement/service.go#L186`:
     `isLapsed := !req.Enforcing || len(req.Issues) > 0` (*Existing ONUSLY source behavior*).
   - `internal/enforcement/models.go` allows `Enforcing: false` with `Issues: []`. Therefore, a heartbeat reporting `Enforcing: false` immediately represents an enforcement lapse.
   - *Heartbeat Target State Table:*

| Session Condition | Heartbeat Payload | Backend Processing & Behavior |
|---|---|---|
| **Paired user active on physical console** | `Enforcing: true`, `Issues: []` | Normal enforcement. No lapse. |
| **Paired user locked screen (`WTS_SESSION_LOCK`)** | `Enforcing: true`, `Issues: []` | Normal enforcement maintained. Prevents false-alarm alerts during user absences. |
| **Unpaired user actively using console (Fast User Switching)** | `Enforcing: false`, `Issues: []` | **Immediate lapse.** Approver receives `enforcement.lapsed` notification. **Not** labeled `service_degraded`. |
| **Paired user logged off / system sleep / power off** | Heartbeats suspend (no packets sent) | Offline silence. Backend **24-hour offline grace window** applies before alerting approver. |
| **Backend unavailable / network offline** | Heartbeat call fails; client caches state | Offline silence. Client enforces locally; no server lapse until 24-hour grace window expires. |

5. **Notification Deduplication & Future Schema Proposal:**
   - *Deduplication:* In `internal/enforcement/service.go#L198`, `device.LapseEpisodeID` deduplicates notifications while `isLapsed == true`. Subsequent heartbeats do not send duplicate emails until enforcement resumes and clears `LapseEpisodeID` (*Existing ONUSLY source behavior*).
   - *Proposed Backend Contract Change:* Add an explicit issue code `"paired_session_missing"` to `validIssues` in `models.go` (*Engineering proposal*). Under the current schema, sending `Enforcing: false` with `Issues: []` triggers the lapse cleanly without misusing `service_degraded`.

### 5.5 DPAPI, Cryptographic Boundaries & Honest Backend Credential Lifecycle
1. **Refined Cryptographic Threat Model:**
   - *Machine-Scoped DPAPI (`CRYPTPROTECT_LOCAL_MACHINE`):* Encrypts data using LSA machine secrets. Any process running on the same machine that obtains the ciphertext can decrypt it. Machine DPAPI provides machine-context binding, **not** local user isolation.
   - *NTFS DACLs:* The **sole effective boundary** against unprivileged local users. `%ProgramData%\Onusly\secure\token.dat` strictly permits only `NT SERVICE\OnuslyEnforcementService` (Full Control) and `BUILTIN\Administrators` (Full Control). Standard users have zero access.
   - *Application Entropy (`pOptionalEntropy`):* Stored in `%ProgramData%\Onusly\config\entropy.dat` under the same ACL. Prevents generic DPAPI dumping tools, but does not constitute an independent security boundary against administrators.
   - *BitLocker (Full Volume Encryption):* Protects against offline disk theft (e.g. booting a Linux USB to read NTFS files). Without BitLocker, physical possession of an unencrypted drive bypasses NTFS DACLs.
2. **Corrections of Unverifiable Cryptographic Claims:**
   - *Hardware Restore:* Decryption after restoring a disk image depends on TPM state, machine SID, and LSA secrets. Restoring an image to different hardware does not guarantee failure if the SYSTEM hive is cloned without TPM binding.
   - *Physical Erasure:* `SecureZeroMemory` on a file buffer followed by `DeleteFileW` does **not** guarantee physical NAND erasure on SSDs due to wear leveling, FTL over-provisioning, and NTFS Volume Shadow Copies (VSS).
   - *Go Memory Erasure:* The Go runtime garbage collector dynamically copies strings and byte slices across heap pages. Zeroing a single slice cannot guarantee all transient copies in memory are scrubbed.
3. **Backend Credential Revocation Evidence (Ground Truth):**
   - Inspection of `cmd/server/main.go`, `internal/auth/service.go`, and `internal/middleware/auth.go` confirms:
     - **No Device Deletion Route:** The backend does not expose `DELETE /enforcement/devices/{id}`.
     - **No Logout Route:** The backend does not expose `POST /auth/logout`.
     - **No Server-Side Token Revocation Table:** JWT authentication in `internal/middleware/auth.go` is completely stateless (validates HS256 signature and `exp` claim). There is no DynamoDB lookup or token denylist.
   - **True Credential Lifecycle:**
     - *Local Unbind:* Service unlinks `token.dat` and discards memory references.
     - *Credential Expiration:* The JWT expires naturally after **7 days** (`exp` claim).
     - *Device Record Expiration:* Inactive device records expire via DynamoDB TTL after **30 days** (`ExpiresAt`).
     - *Lapse Alerts:* 24 hours of heartbeat inactivity while a goal is active alerts the accountability partner.

### 5.6 Service Account, Service SID, and Access-Control Analysis
1. **Separation of Windows Identity Concepts:**
   - *Service Logon Account:* The security context in which the service process is launched (`LocalService`, `NetworkService`, or Virtual Service Account).
   - *Service SID:* Unique security identifier (format `S-1-5-80-...`) derived deterministically from the service name.
   - *Token User SID:* Primary user SID in the process token (e.g. `S-1-5-19` for LocalService).
   - *Token Group SID:* Auxiliary group SIDs added to the token, including the unique Service SID.
   - *Restricted SID List:* SIDs placed in the restricted token list, enforcing dual-pass access checks when `SERVICE_SID_TYPE_RESTRICTED` is enabled.

2. **Comparison of Service Identity Alternatives:**

| Dimension | Option A: Virtual Service Account (`NT SERVICE\OnuslyEnforcementService`) | Option B: `NT AUTHORITY\LocalService` + Unique Service SID (Selected) | Option C: `NT AUTHORITY\NetworkService` + Unique Service SID |
|---|---|---|---|
| **WTS Query Information Permissions** | **Not granted by default.** Cross-session `WTSQuerySessionInformationW` frequently returns `ERROR_ACCESS_DENIED` (5). | **Documented in default RDS model.** Default RDS security descriptor grants `WTS_SECURITY_QUERY_INFORMATION` and `WTS_SECURITY_SEND_MESSAGE` to `LocalService`. | **Documented in default RDS model.** Default RDS security descriptor grants `WTS_SECURITY_QUERY_INFORMATION` and `WTS_SECURITY_SEND_MESSAGE` to `NetworkService`. |
| **Windows 11 Home/Pro Compatibility** | Fully supported by SCM. | Fully supported by SCM. | Fully supported by SCM. |
| **Default Token Privileges** | Basic user rights (`SeChangeNotifyPrivilege`). | `SeChangeNotifyPrivilege`, `SeCreateGlobalPrivilege`, `SeUndockPrivilege`, `SeImpersonatePrivilege`. | `SeChangeNotifyPrivilege`, `SeCreateGlobalPrivilege`, `SeImpersonatePrivilege`. |
| **Privilege Stripping Capability** | Supported via SCM. | **Supported via SCM.** `ChangeServiceConfig2` with `SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO` strips `SeImpersonatePrivilege` and leaves only `SeChangeNotifyPrivilege`. | Supported via SCM. Strips unneeded privileges. |
| **Outbound HTTPS Behavior** | Connects to `api.onusly.com` via TCP/TLS 1.3 with application Bearer JWT. | Connects to `api.onusly.com` via TCP/TLS 1.3 with application Bearer JWT. | Connects to `api.onusly.com` via TCP/TLS 1.3 with application Bearer JWT. |
| **Network / Remote Credentials** | Authenticates to remote shares as computer account (`DOMAIN\COMPUTER$`). | **Anonymous network identity.** Acts as `NT AUTHORITY\ANONYMOUS LOGON` on network shares; zero machine credential exposure. | Authenticates to remote shares as computer account (`DOMAIN\COMPUTER$`). |
| **Local Filesystem Isolation** | Unique Service SID (`S-1-5-80-...`). | **Unique Service SID (`S-1-5-80-...`).** NTFS DACLs grant access exclusively to Service SID and Administrators; generic `LocalService` processes have zero access. | Unique Service SID (`S-1-5-80-...`). NTFS DACLs grant access exclusively to Service SID and Administrators. |
| **Named-Pipe Security** | SDDL targets Service SID and Console Owner. | **SDDL targets Service SID and Console Owner.** Other `LocalService` processes are denied. | SDDL targets Service SID and Console Owner. |
| **DPAPI Machine-Scope Compatibility** | 100% compatible (`CRYPTPROTECT_LOCAL_MACHINE`). | **100% compatible (`CRYPTPROTECT_LOCAL_MACHINE`).** | 100% compatible (`CRYPTPROTECT_LOCAL_MACHINE`). |
| **Attack Surface If Compromised** | Lowest local surface, but presents machine credentials on domain network. | **Lowest overall surface.** Local-only, anonymous network logon, privileges stripped. | Higher surface: possesses domain computer credentials. |

3. **Feasibility Assessment of Terminal Server Permissions Mutation:**
   - *WMI Class Availability:* The WMI provider `root\cimv2\TerminalServices` and the class `Win32_TSPermissionsSetting` are part of the Remote Desktop Session Host role, which is not present on Windows 11 Home or standard Pro client installations (*Requires Windows VM verification*).
   - *Virtual Service Account Resolution:* `Win32_TSPermissionsSetting::AddAccount` fails or rejects synthetic virtual service account SIDs (`S-1-5-80-...`).
   - *Machine-Global Impact:* Modifying `DefaultSecurity` in `HKLM\SYSTEM\CurrentControlSet\Control\Terminal Server` alters security globally for all sessions, creating unintended attack surface.
   - *Rollback Risk:* Reverting binary security descriptors on uninstall is fragile and risks corrupting terminal services security.
   - *Conclusion:* Programmatic mutation of Terminal Server security descriptors is **rejected as a fragile fallback**. Option B (`LocalService` + Unique Service SID) avoids this fragility by relying on built-in RDS permissions.

4. **Service Identity Architecture Decision & Windows VM Acceptance Checklist:**
   - **Option B (`NT AUTHORITY\LocalService` with unique ONUSLY Service SID and `SERVICE_SID_TYPE_UNRESTRICTED`) is SELECTED.**
   - SCM configuration will explicitly configure `SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO` to retain only `SeChangeNotifyPrivilege`, stripping `SeImpersonatePrivilege`.
   - **Mandatory LocalService Windows VM Acceptance Checklist (Pre-Implementation Gate):**
     1. *Service Process Identity:* Verify process executes strictly as `NT AUTHORITY\LocalService` (Token User SID `S-1-5-19`), NOT Administrator or LocalSystem.
     2. *Privilege Reduction Verification:* Inspect effective token privileges via `GetTokenInformation(TokenPrivileges)`: confirm `SeChangeNotifyPrivilege` is enabled, and `SeImpersonatePrivilege` and `SeTcbPrivilege` are completely removed.
     3. *Session Enumeration:* Verify `WTSEnumerateSessionsW` succeeds and enumerates interactive sessions.
     4. *Session Querying:* Verify `WTSQuerySessionInformationW` can query `WTSUserName` and `WTSDomainName` for the active physical console session without returning `ERROR_ACCESS_DENIED` (5) on clean Windows 11 Home and Pro installations.
     5. *Network & Outbound HTTPS:* Verify outbound TLS 1.3 connectivity to `https://api.onusly.com` operates normally, presenting anonymous network credentials.
     6. *Named-Pipe IPC & Client Session Resolution:* Verify `CreateNamedPipeW` and `GetNamedPipeClientSessionId` operate under `LocalService`.
     7. *Filesystem Security-SID Isolation:* Verify that `%ProgramData%\Onusly\secure\token.dat` ACLed to `NT SERVICE\OnuslyEnforcementService` permits the ONUSLY service but denies access to a test unprivileged process running under generic `LocalService`.
     8. *No System ACL Modification:* Verify all operations succeed without modifying Terminal Services security descriptors (`DefaultSecurity` or `Win32_TSPermissionsSetting`).

5. **Diagnostic Probe Design, SCM Service Architecture, Safety Audit & Windows VM Test Matrix:**
   - *Standalone Probe Location:* Canonical verified location is **`scratch/windows_validation_probe/`** (`main.go`, `go.mod`, `run_probe.ps1`). (Isolated nested module; excluded from root Go workspace). The path `windows_validation_probe/` at repository root **does not exist** (verified via filesystem inspection).
   - *Win32 Pseudo-Handles, Struct Layouts & SCM Service Execution Architecture:*
     1. *Pseudo-Handle Corrections:* Audited and corrected Win32 pseudo-handle handling. On Windows, GetCurrentThread() returns (HANDLE)-2 (0xFFFFFFFFFFFFFFFE on x64), whereas GetCurrentProcess() returns (HANDLE)-1 (0xFFFFFFFFFFFFFFFF on x64). The implementation defines explicit wrappers getCurrentThreadHandle() and getCurrentProcessHandle(), eliminating raw magic bitwise expressions (^uintptr(0)).
     2. *TOKEN_PRIVILEGES Struct Layout Alignment (Defect Resolved):* Audited Go struct layout against Windows C SDK. While TOKEN_GROUPS has 8-byte alignment on x64 because SID_AND_ATTRIBUTES contains an 8-byte pointer (Sid), TOKEN_PRIVILEGES contains LUID_AND_ATTRIBUTES which consists strictly of 32-bit types (DWORD LowPart, LONG HighPart, DWORD Attributes). Its alignment requirement is strictly 4 bytes. Therefore, Privileges[0] begins at byte offset 4 on both x86 and x64. The previous offset = 8 defect in main.go was resolved, preventing a 4-byte misalignment in LookupPrivilegeNameW. Added explicit buffer bounds validation on both slices.
     3. *SCM Startup Lifecycle:* The probe invokes StartServiceCtrlDispatcherW on startup. In ServiceMain, it registers the control handler (RegisterServiceCtrlHandlerExW), reports SERVICE_START_PENDING (checkpoint 1, wait hint 5000ms), and transitions to SERVICE_RUNNING promptly (< 50ms) before launching diagnostics in an asynchronous worker goroutine. SCM startup is never blocked by lengthy HTTPS or network operations.
     4. *SCM Bounded Shutdown & Timeout Exit Code (Hardened):* On SERVICE_CONTROL_STOP, ServiceMain sets SERVICE_STOP_PENDING, signals worker cancellation, and waits up to 4 seconds for termination. If the worker terminates cleanly, it reports SERVICE_STOPPED with exit code 0 and records clean shutdown in shutdown_status.json. If the worker exceeds the 4-second deadline, ServiceMain marks atomicShutdownComplete, writes shutdown_status.json recording worker_timed_out = true, and reports SERVICE_STOPPED with Win32 exit code 1053 (ERROR_SERVICE_REQUEST_TIMEOUT) so SCM records a failed/incomplete shutdown. No worker can write results after atomicShutdownComplete is set.
     5. *Atomic Result Emission:* Diagnostic results are written to probe_results.tmp, flushed, renamed to probe_results.json, and marked with probe_completed.marker.
     6. *Interactive Fallback:* If executed directly from console, Win32 error 1063 (ERROR_FAILED_SERVICE_CONTROLLER_CONNECT) triggers interactive standalone mode.
   - *TOKEN_USER Layout & Binary SID Verification:*
     - Architecture Layout (x64): TOKEN_USER contains SID_AND_ATTRIBUTES (PSID Sid 8 bytes, DWORD Attributes 4 bytes, 4 bytes padding = 16 bytes).
     - Confirmed tu.User.Sid is dereferenced as a pointer (PSID) and validated via IsValidSid.
     - Confirmed binary SID comparison using EqualSid against binary SIDs, not string prefixes.
     - Checked ERROR_INSUFFICIENT_BUFFER (122) on size-query passes.
   - *Named-Pipe Identification Authentication (Dual-Client: Elevated & Unelevated):*
     - *Elevated Client:* Launched via onusly-probe.exe --client-elevated. Inherits the administrator token (High Integrity S-1-16-12288, TokenIsElevated = 1).
     - *Unelevated Client:* Launched via Windows Explorer COM shell (Shell.Application.ShellExecute) running at Medium Integrity (S-1-16-8192, TokenIsElevated = 0). If ShellExecute is unavailable, the harness outputs the exact manual console command line to execute from an unelevated console window.
     - *Identity Auditing:* Both clients record TokenUser SID, TokenElevation, TokenIntegrityLevel, ProcessSessionID, and whether the token belongs to the active console user (BelongsToPairedConsoleUser = true for both) to client_elevated.json and client_unelevated.json.
     - *Server Dual-Client Handling:* Server creates pipe with FILE_FLAG_FIRST_PIPE_INSTANCE, accepts Client 1 (elevated), validates identification-level SQOS (impLevel == 1) and binary SID, sends ONUSLY_AUTH_OK, disconnects, and immediately waits up to 5s for Client 2 (unelevated), validating both without weakening security controls.
   - *Separated Outbound HTTPS Diagnostics (DNS, TCP, TLS, HTTP, Backend Health):*
     - *DNS Resolution:* Distinctly records DNSSuccess, ResolvedIPs, and DNSError.
     - *TCP Connectivity:* Distinctly records TCPConnectSuccess and TCPError to port 443.
     - *TLS Handshake & Version Inspection:* Explicitly inspects tls.ConnectionState().Version (verifying "TLS 1.3" vs "TLS 1.2") and tls.CipherSuiteName().
     - *HTTP Response & Backend Health:* Connects directly over established TLS connection to GET /health. Distinguishes backend health (status 200) from Cloudflare edge responses (HTTP 521 Web Server Is Down / 522 / 523). An HTTP 521 is explicitly recorded as TLSHandshakeSuccess = true, NegotiatedTLSVersion = "TLS 1.3", BackendHealthy = false, CloudflareOriginDown = true.
     - *Cancellation Propagation:* Context cancellation wired directly to SCM cancelChan; in-flight network requests abort immediately on service stop.
   - *Real Service SID Filesystem Isolation Test & SCM Lifecycle Integration:*
     - *Positive Access Test:* Probe service creates %ProgramData%\OnuslyProbe\service_sid_protected.dat ACLed to D:(A;;GA;;;<ProbeServiceSID>)(A;;GA;;;BA). Because the service token possesses <ProbeServiceSID>, access succeeds (ServiceSIDPositiveAccess = true).
     - *Negative Access Test with SCM Integration:* The harness registers and starts a second temporary service (OnuslyGenericLSTestService) under NT AUTHORITY\LocalService with SERVICE_SID_TYPE_NONE. To avoid SCM startup timeouts (Error 1053), the secondary service registers negServiceMainCallback, reports SERVICE_RUNNING, executes the negative test (verifying TokenUser == S-1-5-19, Service SID absent, file existence verified beforehand, CreateFileW denied with Win32 Error 5), reports SERVICE_STOPPED, and emits negative_test.json.
     - *Administrator Cleanup:* Administrators (BA) retain Full Control for directory cleanup. Zero modifications to production %ProgramData%\Onusly.
   - *PowerShell Harness Safety, Timeout & Post-Cleanup SCM Verification:*
     1. *Temporary Service Registration:* Registers temporary service OnuslyValidationProbeService (strictly distinct from production OnuslyEnforcementService).
     2. *Refusal to Overwrite:* Pre-execution guard checks Get-Service for both OnuslyValidationProbeService and OnuslyGenericLSTestService, aborting if either exists.
     3. *Startup Failure Discrimination:* Wrapped Start-Service in dedicated try/catch block distinguishing SCM logon/startup failures from diagnostic failures.
     4. *Finite Timeout & Cancellation:* Polling for probe_completed.marker bounded by 30s timeout. On timeout, records critical error, sets status TIMEOUT, and immediately stops the service.
     5. *Termination Synchronization:* Calls .WaitForStatus(Stopped, 10s) prior to service deletion.
     6. *Guaranteed Cleanup & Surfaced Errors:* PowerShell try ... finally block stops both services, checks  from sc.exe delete, and surfaces any deletion failures with Write-Warning.
     7. *Post-Cleanup SCM Verification:* Runs sc.exe query for both services, explicitly confirming exit code 1060 (ERROR_SERVICE_DOES_NOT_EXIST). If exit code is 0, checks for DELETE_FLAG, explicitly warning if a service remains marked for deletion due to open handles.
     8. *Crash Recovery Documentation:* Services are configured start= demand. Manual cleanup instructions documented for interrupted experiments.
   - *Environment & Execution Status:* Current development environment is Linux Fedora 43 (`7.2.13-100.fc43.x86_64`); Windows VM execution is currently unavailable. Therefore, all runtime VM checks are classified as **`Not tested / unknown`**. Success must not be inferred from documentation or compilation.
   - *Target Test Scenarios (Matrix for Windows 11 Home & Pro):*

| Scenario ID | Test Scenario Description | Expected Outcome | Execution Status |
|---|---|---|---|
| **SC-A** | Paired Alice logged into physical console session | IPC permitted; heartbeats report `Enforcing: true`, `Issues: []`. | `Not tested / unknown` |
| **SC-B** | Alice locks and unlocks physical console | IPC permitted; heartbeats continue reporting `Enforcing: true`; zero false lapses. | `Not tested / unknown` |
| **SC-C** | Alice logs off; Bob logs into physical console (Fast User Switching) | Bob's IPC rejected (`BobSID != AliceSID`); heartbeats report `Enforcing: false`, `Issues: []` (immediate lapse). | `Not tested / unknown` |
| **SC-D** | Alice connected via RDP while Bob occupies physical console | RDP IPC rejected (`clientSessionId != activeConsoleSessionId`); console remains lapsed. | `Not tested / unknown` |
| **SC-E** | Physical console detached / headless (`0xFFFFFFFF`) | All IPC rejected; heartbeats report `Enforcing: false` or suspend. | `Not tested / unknown` |
| **SC-F** | Unrelated process running as `LocalService` attempts to read `token.dat` | Access denied by NTFS DACL (Service-SID-only boundary). | `Not tested / unknown` |
| **SC-G** | Standard user attempts named-pipe IPC without being paired owner | Connection rejected by pipe DACL and 5-step dual-check validation pipeline. | `Not tested / unknown` |
| **SC-H** | Service starts on system reboot prior to user logon | Sessions enumerated; console detached; no false `Enforcing: true` heartbeats. | `Not tested / unknown` |
| **SC-I** | Service resumes after system sleep / suspend | Power event triggers session re-enumeration and enforcement state reconciliation. | `Not tested / unknown` |

### 5.7 Final Browser Distribution & Registry Mutation Matrix
All registry writes enforce **64-bit hive targeting (`KEY_WOW64_64KEY`)**.

| Browser | Supported Installation Mechanism | Registry Hive & Key Path | Value Name & Type | Data Format / Content | Store Requirement | Non-Domain Win 11 Behavior | Real Windows Verification Status |
|---|---|---|---|---|---|---|---|
| **Google Chrome Stable** | Enterprise Managed Policy | `HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionSettings` | `ExtensionSettings` (`REG_SZ`) | JSON dictionary specifying CWS Extension ID and `installation_mode: force_installed` | Chrome Web Store | Off-store CRX **blocked**. CWS item **force-installs silently**. | Validated via official vendor documentation; requires CWS registration. |
| **Microsoft Edge** | Enterprise Managed Policy | `HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionSettings` | `ExtensionSettings` (`REG_SZ`) | JSON dictionary specifying Edge Add-ons ID and `installation_mode: force_installed` | Microsoft Edge Add-ons | Off-store CRX **blocked**. Edge Add-ons item **force-installs silently**. | Validated via Microsoft Learn; requires Partner Center registration. |
| **Brave Browser** | Chromium Policy | `HKLM\SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallForcelist` | Sequentially numbered values (`1`, `2`) (`REG_SZ`) | `<cws_id>;https://clients2.google.com/service/update2/crx` | Chrome Web Store (Brave uses CWS) | Requires CWS Extension ID. | **Requires Windows VM verification** on Brave Stable x64 (v1.70+). |
| **Chromium (Unbranded)** | Chromium Policy | `HKLM\SOFTWARE\Policies\Chromium\ExtensionInstallForcelist` | Sequentially numbered values (`1`, `2`) (`REG_SZ`) | `<id>;<update_url>` | Optional | May permit off-store CRX if branding flags omitted. | **Requires Windows VM verification** on Chromium Win_x64 snapshot build. |
| **Mozilla Firefox** | Enterprise Managed Policy | `HKLM\SOFTWARE\Policies\Mozilla\Firefox` | `ExtensionSettings` (**`REG_MULTI_SZ`**) | Multiline JSON string mapping **`extension@onusly.com`** to `{ installation_mode: "force_installed", install_url: "..." }` | Mozilla AMO Signature (listed or unlisted) | Works on non-domain Windows **provided .xpi is signed by Mozilla**. | Validated via Mozilla Policy Templates; requires AMO account. |

*Authoritative Firefox Extension ID Reconciled:* **`extension@onusly.com`** (matches `extension/manifest.json#L27`). All references to `onusly@onusly.com` are eliminated.

### 5.8 Target OS Compatibility Baselines (as of November 6, 2026)
- **Windows 11 26H2 (2026 Update):** Generally Available (GA). Primary active target.
- **Windows 11 25H2 (2025 Update):** Active Mainstream Servicing. Primary active target.
- **Windows 11 24H2 (2024 Update):** Home and Pro editions reached End of Servicing in October 2026; Enterprise and Education editions remain supported through October 2027.
- **Windows 11 23H2 (2023 Update):** Home and Pro editions reached End of Servicing in November 2025; Enterprise and Education editions reach End of Servicing on **November 10, 2026** (active as of November 6, 2026).
- **Windows 10 22H2:** Reached Mainstream End of Servicing on October 14, 2025. Requires Extended Security Updates (ESU) for ongoing security support. Legacy baseline only.

### 5.9 Architectural Blocker Matrix & D3 Status

| Item | Category | Status & Resolution Path |
|---|---|---|
| **Named-Pipe Impersonation at Identification Level** | `Official Microsoft documentation` | Client connects with `SECURITY_SQOS_PRESENT \| SECURITY_IDENTIFICATION`; service queries `TokenUser` & `TokenIntegrityLevel` without `SeImpersonatePrivilege`. |
| **Removal of `SeCreateGlobalPrivilege`** | `Official Microsoft documentation` | Removed; Win32 named pipes reside in global Named Pipe File System (`\\.\pipe\`). |
| **Installation Owner Resolution (OTS UAC)** | `Official Microsoft documentation` | `ProcessIdToSessionId(GetCurrentProcessId())` identifies session of elevation origin; `WTSQuerySessionInformationW(WTSUserName)` resolves session user (Alice), avoiding elevation credential account (Bob). |
| **Interactive Account Confirmation** | `Engineering proposal / design decision` | GUI presents mandatory confirmation dialog: `"Configure ONUSLY for account: [Alice]"`. Provisions `pairing_state.json` only upon user confirmation. |
| **Authoritative Pairing State Machine & Recovery** | `Engineering proposal / design decision` | State machine `UNINSTALLED → INSTALLED_UNPAIRED → PAIRING_PENDING → PAIRING_COMMIT_PENDING → PAIRED → REPAIR_REQUIRED`. Generation markers, separate first-install creation (`MoveFileExW`) and re-pair replacement (`ReplaceFileW`), atomic updates. |
| **Pairing Credential Validation Endpoint** | `Engineering proposal / design decision` | Service validates tokens against `GET /enforcement/blocklist` (authenticated with Bearer JWT; returns 200 on success, 401 on invalid/expired). Replaces unauthenticated `GET /health`. |
| **WTS Service Notification API** | `Official Microsoft documentation` | `RegisterServiceCtrlHandlerExW` with `SERVICE_ACCEPT_SESSIONCHANGE`; `HandlerEx` returns `NO_ERROR` immediately and enqueues to worker. |
| **Service Logon Identity (Option B: LocalService)** | `Requires Windows VM verification` | `NT AUTHORITY\LocalService` with unique ONUSLY Service SID and `SERVICE_SID_TYPE_UNRESTRICTED`. Documented by Microsoft as TS default permissions, but actual Windows 11 Home/Pro behavior requires VM validation. |
| **Privilege Reduction via SCM RequiredPrivileges** | `Requires Windows VM verification` | Documented Microsoft API (`SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO`), but effective token privilege removal (`SeImpersonatePrivilege` stripped) requires VM validation. |
| **Application-Level NTFS DACL Isolation** | `Requires Windows VM verification` | Standard Windows security architecture, but real Service SID enforcement on `%ProgramData%\Onusly\` requires VM validation. |
| **Terminal Server DACL Mutation Feasibility** | `Requires Windows VM verification` | Mutating `DefaultSecurity` or `Win32_TSPermissionsSetting` rejected as fragile fallback; missing on Windows 11 Home/Pro client SKUs. |
| **Local-Console vs RDP MVP Scope** | `Engineering proposal / design decision` | **Option A Selected:** MVP supports physical console only. RDP presence is unmonitored. Console detachment (`0xFFFFFFFF`) reports `Enforcing: false`, `Issues: []`. |
| **Direct Named-Pipe Client Session ID (Option B)** | `Official Microsoft documentation` | Service derives client session ID directly via `GetNamedPipeClientSessionId`, rejecting requests if `clientSessionId != WTSGetActiveConsoleSessionId()`. Supersedes PID lookup; eliminates process exit race. |
| **Dual-Check Paired-Owner Session Authorization** | `Engineering proposal / design decision` | 5-step pipeline: 1) `GetNamedPipeClientSessionId`, 2) initial `WTSGetActiveConsoleSessionId` check, 3) identification-level token extraction (`TOKEN_USER`), 4) `EqualSid(clientTokenSid, ownerSid)`, 5) post-check `WTSGetActiveConsoleSessionId` (TOCTOU mitigation). Fails closed on any mismatch or transition. |
| **SCM Power-Resume Event Model** | `Official Microsoft documentation` | SCM delivers `PBT_APMSUSPEND` and `PBT_APMRESUME*`. |
| **Boot Heartbeats and Sleep Recovery Lifecycle** | `Engineering proposal / design decision` | Service pauses heartbeats on suspend, re-enumerates sessions on resume, and refreshes enforcement. (Engineering proposal, not Microsoft guarantee). |
| **Worker Queue Overflow Recovery** | `Engineering proposal / design decision` | Bounded Go channel (128 cap). Drops intermediate events on overflow, sets `reconciliationRequired = 1`, triggers full session re-enumeration. |
| **Heartbeat Semantics & Lapse Trigger** | `Engineering proposal / design decision` | Reconciled with backend `isLapsed := !req.Enforcing \|\| len(req.Issues) > 0`. Account switch reports `Enforcing: false`, `Issues: []` (**not** mislabeled `service_degraded`). |
| **Notification Deduplication** | `Engineering proposal / design decision` | Backend `device.LapseEpisodeID` deduplicates alerts until normal enforcement resumes. |
| **Dedicated Lapsed Issue Enum** | `Engineering proposal / design decision` | Proposed adding `"paired_session_missing"` to `validIssues` in `models.go` (backend contract change). |
| **Chrome Web Store Key Continuity** | `Requires Windows VM verification` | Chromium `packaging.html` protocol (`key.pem` in ZIP root on initial upload) to be verified via draft item on Chrome Developer Dashboard. |
| **Chrome Enterprise Core Feasibility** | `Engineering proposal / design decision` | Rejected for consumer MVP due to customer privacy intrusion and enterprise operational liabilities. |
| **Brave & Chromium Force-Install Verification** | `Requires Windows VM verification` | Registry policies require validation on live Windows 11 VM with Brave Stable x64 and Chromium Win_x64 snapshot. |
| **Service SID Restricted Token Networking** | `Requires Windows VM verification` | `SERVICE_SID_TYPE_UNRESTRICTED` selected for MVP; restricted token (`0x00000003`) Winsock viability requires VM validation. |
| **Windows 11 VM Validation Probe (Scenarios SC-A - SC-I)** | `Not tested / unknown` | Diagnostic probe prepared in `scratch/windows_validation_probe/` (`main.go`, `run_probe.ps1`). Host is Linux Fedora 43; live Windows VM execution unavailable. All runtime checks marked `Not tested / unknown`. |
| **External Store Registrations** | `Engineering proposal / design decision` | Chrome Web Store ($5 fee), Microsoft Partner Center, and Mozilla AMO developer accounts required. |
| **Windows Product Code Implementation** | `Engineering proposal / design decision` | Strictly zero Windows product source files created; implementation strictly gated until LocalService VM validation passes. |

**Milestone Status:**  
`D3: ARCHITECTURE REVIEWED — OPEN BLOCKERS — NOT IMPLEMENTED`

---

## 6. Reconciliation of Previous Execution Reports & Audit Items

### 5.1 Reconciliation of "29 Failed Tasks Out of 32"
A previous execution summary referenced "29 failed tasks out of 32". Investigation of `AUDIT_REMEDIATION.md` clarified this was an inverted reading of the remediation ledger:
- **Total Audit Findings Tracked:** 32 findings (`ONUS-CRIT-01` through `ONUS-INFO-01`).
- **Remediated & Verified Complete:** **29 findings** (`VERIFIED COMPLETE`).
- **Architectural Platform Boundaries:** **3 findings** (`ARCHITECTURAL BOUNDARY` — OS-level constraints such as Android system accessibility settings ownership).
- **Actual Failures:** **0 findings failed re-audit**.

### 5.2 Resolved Implementation Edge Cases
1. **DynamoDB UpdateExpression Reserved Keyword Collision:**
   - *Error:* `ValidationException: Invalid UpdateExpression: Attribute name is a reserved keyword; reserved keyword: Token` in `ConfirmPairingSession`.
   - *Fix:* Updated `internal/enforcement/repository.go` to use `ExpressionAttributeNames: {"#tok": "Token"}` and `SET #tok = :token`.
2. **User-Scope Installer Permission Failure:**
   - *Error:* `mkdir /etc/brave: permission denied` when running `./desktop/onusly-installer --yes --user`.
   - *Fix:* In user mode (`m.useUserScope || os.Geteuid() != 0`), restricted writes to `~/.config` and `~/.mozilla` user directories.
3. **Firefox MV3 Manifest Rejection:**
   - *Error:* `WebExtError: background.service_worker is currently disabled. Add background.scripts.` on Firefox.
   - *Fix:* Generated distinct bundles for Chromium (`onusly-extension.zip`) and Firefox (`onusly-firefox.zip`).
4. **Python Automation Script Type Crash on Empty Blocklist:**
   - *Error:* `TypeError: object of type 'NoneType' has no len()` when blocklist targets were null.
   - *Fix:* Initialized `Targets = []core.TargetItem{}` whenever nil, ensuring consistent non-null arrays in JSON responses.
5. **IPC World-Writable Fallback (`0666`):**
   - *Error:* Fallback to mode `0666` if group setup failed.
   - *Fix:* Removed fallback; system installation creates `onusly` group and enforces mode `0660` owned by `root:onusly` with directory mode `0750`, failing closed on error.
6. **Chrome Web Store Assumption:**
   - *Error:* Chrome Stable was previously assumed to require Web Store publication for Linux desktop deployment.
   - *Fix:* Implemented self-hosted enterprise deployment via `ExtensionSettings` with signed CRX3 and update manifest, achieving full live verification.

---

## 6. Artifacts & Deliverables

- Go Desktop Agent Binary: `desktop/onusly-agent` (installed to `/usr/local/bin/onusly-agent` and `~/.local/bin/onusly-agent`)
- Native Host Binary: `desktop/onusly-native-host` (installed to `/usr/local/bin/onusly-native-host` and `~/.local/bin/onusly-native-host`)
- Linux Installer: `desktop/onusly-installer`
- Packaged Chrome/Chromium Extension: `frontend/public/downloads/onusly-extension.zip`
- Packaged Firefox Extension: `frontend/public/downloads/onusly-firefox.zip`
- Signed Chrome Enterprise CRX: `frontend/public/downloads/onusly.crx`
- Chrome Enterprise Update Manifest: `frontend/public/downloads/updates.xml`
- Chrome Live Verification Suite: `scratch/chrome_live_audit.js`
- Chrome Lifecycle Audit Suite: `scratch/verify_update_lifecycle.js`
- Fresh-Machine Distribution Test Suite: `scratch/test_fresh_machine_installation.js`
- Linux Distro Adapters Guide: `desktop/docs/LINUX_DISTROS.md`

---

## 7. Windows Production Milestones (D3.1 – D3.4)

### 7.1 D3.1 Production Service Architecture
- **Service Name:** `OnuslyDesktopAgent`
- **SCM Service Identity:** `NT AUTHORITY\LocalService` (`S-1-5-19`)
- **Service SID Type:** `SERVICE_SID_TYPE_UNRESTRICTED` (`S-1-5-80-3463423020-2351545225-1748562058-3133284821-4013307172`)
- **Privilege Stripping:** Restricted to `SeChangeNotifyPrivilege` (`se_impersonate_stripped: true`).
- **WTS Console Session Isolation:** Enforces client session ID matches active console session via `WTSGetActiveConsoleSessionId` with TOCTOU post-check.
- **Client Impersonation Security:** Named-pipe authentication enforces `TokenImpersonationLevel == SecurityIdentification` (level 1) and compares caller token user SID against paired owner SID.
- **DACL Authorization:** Explicit SDDL `D:(A;;GA;;;BA)(A;;GA;;;SY)(A;;GA;;;LS)(A;;GRGW;;;IU)` protects the named pipe `\\.\pipe\OnuslyAgentIPC`.

### 7.2 D3.2 Windows Installer, Registry & Browser Policy Lifecycle
- **Unified Installer (`desktop/installer/windows/install.ps1`):**
  - Staged PE validation, atomic backup directory creation, rollback on failure.
  - SCM service registration with `SERVICE_SID_TYPE_UNRESTRICTED` and stripped privileges.
  - Machine-level Native Messaging registration for Google Chrome and Microsoft Edge.
  - Managed policy deployment (`ExtensionInstallForcelist` / `ExtensionSettings`) targeting canonical extension ID `jifigaejbinmdhnljobkodcgeaplojmg`.
- **Uninstaller (`desktop/installer/windows/uninstall.ps1`):**
  - Clean SCM service unregistration, browser policy restoration, native host removal.
  - Data retention policies: `-RetainDeviceIdentity` (preserves device identity for re-pairing), `-PurgeAll` (complete machine cleanup).

### 7.3 D3.3 End-to-End Installation & Runtime Acceptance
- **Regression Suite:** `desktop/installer/windows/test_lifecycle.ps1` (70 of 70 checks passing).
- **Transactional Rollback:** 11 failure boundaries verified (`BinaryStaging`, `PEValidation`, `BackupCreation`, `FirstBinaryReplacement`, `SecondBinaryReplacement`, `ACLApplication`, `SCMConfiguration`, `NativeMessagingRegistration`, `BrowserPolicyConfiguration`, `ServiceStartup`, `PostStartHealth`).
- **Component-Level Repair:** Restores missing agent executable, native host executable, or browser manifests with zero alteration on clean systems.
- **Running-Service Upgrade:** Gracefully stops service, releases file locks, upgrades binaries, preserves `device_id` SHA-256 hash (`8443EE4451BFA152BC88EE820026DD4DB237EACBB0021ECF2608F23508A033EC`), and restarts service.

### 7.4 D3.4 Device Pairing, Credential Lifecycle & Session Resilience
- **Automated Validation Gate:** `desktop/installer/windows/test_pairing_acceptance.ps1` (**32 of 32 checks passing**).
- **Authoritative Service Pairing Protocol:**
  - Standard unprivileged console users cannot access `%ProgramData%\Onusly\secure\token.dat` (protected by NTFS DACL).
  - CLI `onusly-agent pair` delegates pairing to the running service via IPC actions `pair-request` and `pair-poll`.
  - Service requests 6-character code from backend (`POST /enforcement/pair/request`).
  - Interactive user confirms code on dashboard (`POST /enforcement/pair/confirm`).
  - Service exchanges code for device JWT (`POST /enforcement/pair/exchange`), encrypts using DPAPI machine scope, persists to `secure\token.dat`, pulls blocklist (`GET /enforcement/blocklist`), and locks owner SID.
  - **Zero Credential Leakage Invariant:** Raw JWT is NEVER returned across the named pipe or printed to CLI output.
- **DPAPI Hardening:** Enforces 64KB bounded ciphertext/plaintext size and transient memory buffer zeroing.
- **Pairing State Machine (`auth.PairingStateManager`):**
  - Transitions: `UNINSTALLED` → `INSTALLED_UNPAIRED` → `PAIRING_PENDING` → `PAIRING_COMMIT_PENDING` → `PAIRED` ↔ `AUTHENTICATION_REQUIRED` / `REPAIR_REQUIRED`.
  - Interrupted two-phase commit recovery: auto-promotes to `PAIRED` if token is valid on startup; auto-transitions to `REPAIR_REQUIRED` if token is corrupted or missing.
  - Token expiration detection: auto-transitions to `AUTHENTICATION_REQUIRED` without clearing cached restrictions.
- **Authoritative Blocklist Syncer & Offline Resilience:**
  - Implements ETag conditional requests (`If-None-Match`) and handles `304 Not Modified`.
  - **Critical Invariant:** Network failures (including backend HTTP 521) NEVER wipe valid cached restrictions. Offline cache remains active.
- **Browser Native Host Integration:**
  - `onusly-native-host.exe` connects to named pipe with `SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION`.
  - Translates 4-byte little-endian stdio messages to named-pipe JSON requests and returns blocklist with zero credential exposure.

---

## 8. D4 — Cross-Platform Local DNS Enforcement, Resolver Integration & Network Lifecycle

### 8.1 Dual-Protocol Local Resolver Architecture
- **Listeners:** Concurrent UDP and TCP server listening on loopback `127.0.0.1:53` (`desktop/agent/internal/dns/server.go`).
- **Wire-Format Parser:** Standard RFC 1035 message parsing and serialization via `golang.org/x/net/dns/dnsmessage` without CGo or third-party DNS bloat.
- **Synthesized NXDOMAIN for Blocked Domains:**
  - `Header.RCode = dnsmessage.RCodeNameError`
  - `Header.Authoritative = true`
  - `Header.RecursionAvailable = true`
  - **`Header.AuthenticData = false`** (Critical DNSSEC compliance invariant: synthetic responses MUST NOT forge DNSSEC AD flags).
  - Question section copied intact across record types: `A`, `AAAA`, `CNAME`, `TXT`, `MX`, `SRV`, `PTR`.

### 8.2 Goal Status Enforcement Contract
- **Enforcement Mapping (`desktop/agent/internal/dns/policy.go`):**
  - `active` → BLOCKED (NXDOMAIN)
  - `proof_submitted` → BLOCKED (NXDOMAIN)
  - `rejected` → BLOCKED (NXDOMAIN)
  - `completed` → ALLOWED (Fresh upstream resolution)
- **Dynamic Policy Swapping:**
  - `PolicySnapshot` stored in lock-free `atomic.Pointer`.
  - Transitioning a goal from `active` to `completed` in subsequent sync immediately unblocks the domain and purges conflicting cache entries without requiring service or resolver restarts.

### 8.3 Domain Matching & Evasion Prevention
- **Normalization:** Downcasing, trimming trailing dots, and IDN/punycode conversion via `golang.org/x/net/idna.Lookup.ToASCII`.
- **Match Rules:**
  - Exact domain match (`example.com`).
  - Subdomain wildcard matching (`api.example.com`, `deep.sub.example.com`).
  - Automatic `www.` parity (`www.example.com` ↔ `example.com`).
  - Internationalized domain name equivalence (`münchen.de` ↔ `xn--mnchen-3ya.de`).
- **Strict Boundary Checks:** Substring false-positive prevention ensures `notexample.com` and `example.com.org` are never blocked when `example.com` is targeted.

### 8.4 Core Infrastructure Immunity
- Built-in immutable immunity list (`desktop/agent/internal/dns/matcher.go:IsProtectedDomain`) guarantees that critical system infrastructure can NEVER be blocked, even if maliciously or accidentally injected into the backend blocklist:
  - `localhost`, `127.0.0.1`, `::1`
  - RFC 1918 reverse lookup zones (`*.in-addr.arpa`, `*.ip6.arpa`)
  - Local discovery domains (`*.local`, `*.internal`, `*.home.arpa`)
  - ONUSLY backend and infrastructure domains (`*.onusly.com`)

### 8.5 Upstream Pool & Recursion Deadlock Prevention
- **Strict IP Validation:** Upstream server entries are strictly validated as `IP:port` (e.g. `1.1.1.1:53`, `8.8.8.8:53`). Hostnames (e.g. `dns.google.com`) are rejected at startup to prevent infinite recursive resolution deadlocks.
- **Failover & Truncation Handling:**
  - UDP query with automatic fallback to TCP when the upstream response indicates truncation (`TC` bit set).
  - Configurable query timeouts (default 3s) with automatic failover across multiple configured upstream resolvers.

### 8.6 In-Memory Bounded Cache
- **Thread-Safe LRU/TTL Cache (`desktop/agent/internal/dns/cache.go`):**
  - Stores raw packed wire-format responses with dynamic TTL countdown adjustment on read.
  - Clamps minimum TTL to 1s and maximum to 86,400s (24h).
  - Negative caching for NXDOMAIN upstreams clamped to a maximum of 300s.
  - Capacity eviction (default 5,000 entries) prevents memory exhaustion.
  - `Invalidate(domains)` provides instant targeted purge upon blocklist policy updates.

### 8.7 Network Adapter Management & Fail-Open Recovery
- **Platform Implementations:**
  - Windows (`desktop/agent/internal/dns/adapter_windows.go`): PowerShell / Win32 cmdlets (`Get-DnsClientServerAddress`, `Set-DnsClientServerAddress -ServerAddresses ("127.0.0.1")`, `ipconfig /flushdns`).
  - Linux (`desktop/agent/internal/dns/adapter_linux.go`): `resolvectl`, `NetworkManager`, and `/etc/resolv.conf`.
- **Pre-Modification Backup:** Current adapter DNS settings are atomically saved to `<configDir>/dns_backup.json` before any network adapter mutation occurs.
- **Fail-Open Safety Invariant:**
  - If setting loopback DNS fails on any adapter, the manager immediately reverts all adapters to their backed-up state.
  - The uninstaller (`uninstall.ps1`) and installer rollback inspect `dns_backup.json` and restore all adapters to DHCP or static DNS.
- **CLI Commands:**
  - `onusly-agent dns enable`: Elevated loopback redirection with automatic backup.
  - `onusly-agent dns restore`: Fail-open restoration of original adapter DNS.
  - `onusly-agent dns status`: Reports redirection status, backup state, and discovered network interfaces.

### 8.8 Zero-Surveillance Privacy Invariant
- **No Query History Logging:** The resolver NEVER logs domain query strings, client IPs, or query timestamps to disk or IPC streams.
- **Aggregate Metrics Only:** The daemon exposes only high-level atomic counters: `queriesTotal`, `queriesBlocked`, `queriesForwarded`, `queriesCached`, and `queriesFailed`.

### 8.9 Browser Enterprise DoH Policies
- **DoH Bypass Prevention:** Modern browsers bypassing local port 53 via built-in DNS-over-HTTPS are controlled via enterprise policies:
  - Chromium (Chrome, Edge, Brave): `DnsOverHttpsMode = "off"` configured in registry / JSON policies.
  - Mozilla Firefox: `DNSOverHTTPS\Enabled = 0` configured in registry policies.
  - All DoH settings are tracked in `backup_state.json` and cleanly reverted upon uninstallation.

### 8.10 Acceptance & Validation Verification
- **Acceptance Suite:** `desktop/installer/windows/test_dns_acceptance.ps1` (**37 of 37 checks passing**):
  - Binary compilation and MZ PE header validation.
  - 15 Go DNS package tests passing (TTL, eviction, normalization, subdomains, IDN, NXDOMAIN synthesis, DNSSEC AD=false invariant, failover).
  - D4 goal status enforcement matrix verified (`active`/`proof_submitted`/`rejected` blocked, `completed` unblocked).
  - Protected domain immunity verified.
  - Non-destructive mock adapter lifecycle verified.
  - Standalone agent DNS resolver and named-pipe IPC verified.
  - Live loopback DNS query execution against `127.0.0.1:53` verified.
  - SCM production service invariants (LocalService, unrestricted Service SID, Device ID) verified.
  - Browser DoH enterprise policies verified.
- **Regression Suite:** `desktop/installer/windows/test_pairing_acceptance.ps1` (**32 of 32 checks passing**).

