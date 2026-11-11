# ONUSLY Release Candidate 1 (v1.0.0-rc1) Release Notes

**Release Identifier:** v1.0.0-rc1  
**Release Security Version:** 1  
**Build Commit:** 2c26d96  
**Build Date:** 2026-11-11  
**Target Release Channel:** Controlled Beta (`beta`)  
**Platforms Supported:** Windows 10/11 (AMD64), Linux (AMD64, systemd), Android (API 26+), Chromium / Edge Extension (Manifest V3)

---

## 1. Executive Summary

ONUSLY v1.0.0-rc1 represents the first fully integrated release candidate ready for private beta deployment. This release converges the foundational capabilities developed across milestones D0 through D14 into a hardened, observable, and multi-platform accountability platform.

Core architectural guarantees delivered in this release:
- **Zero-Surveillance Accountability:** Policy enforcement blocks pre-committed distraction targets without recording browsing histories or personal activity logs.
- **Multi-Platform Typed Application Parity:** Explicit segregation between desktop binaries (`desktop_app`) and mobile packages (`android_package`) prevents cross-platform collision.
- **Cryptographic Update Protocol:** Anti-rollback Ed25519 signed update manifests with atomic staging, two-stage upgrade verification, and safe rollback fallbacks.
- **Production-Grade Reliability & SRE Observability:** Standardized Prometheus exposition (`/metrics`), size-bounded log rotation with token redaction, and user-consented diagnostic support bundles.

---

## 2. Key Features & Platform Enhancements

### 2.1 Multi-Platform Desktop & Mobile Agents
- **Windows Desktop Agent:** System service executing under `NT AUTHORITY\LocalService` with loopback DNS resolver, DPAPI credential persistence, and IPC diagnostics via named pipes (`\\.\pipe\OnuslyAgentIPC`).
- **Linux Desktop Agent:** Native systemd service daemon with kernel keyring credential storage and loopback socket IPC (`/run/onusly/onusly.sock`).
- **Android Native Client:** Parity with typed `android_package` policy, Foreground Enforcement Service, Usage Access monitor, and device heartbeat reporter.
- **Browser Integrations:** Manifest V3 native messaging host with path-traversal safeguards and zero-unauthenticated-IPC boundaries.

### 2.2 Accountability Lifecycle & Consistency
- **Race-Safe Approvals:** Idempotent decision recording preventing double-approval or contradictory partner overrides.
- **Optimistic Concurrency Control (OCC):** DynamoDB condition expressions protect goal version counters and prevent desynchronized policy states.
- **Multi-Device Coherence:** Real-time WebSocket push distribution ensures policy adjustments propagate across all paired devices within sub-second latencies.

### 2.3 Security Hardening & Zero-Knowledge Defense
- **Device Credential Revocation:** Centralized device revocation immediately terminates device sessions, wipping local credentials and transitioning client agents to fail-open safe modes.
- **Secret Redaction:** Structured loggers strictly scrub JWT tokens (`[REDACTED_JWT]`), authorization headers, and passwords from all event streams.
- **Anti-Rollback Verification:** Client updaters reject any update payload carrying a lower `SecurityVersion` or missing Ed25519 signature.

### 2.4 Support & Operational Engineering
- **Privacy-Safe Support Bundle:** CLI subcommand `onusly-agent support bundle` collects non-sensitive operational health metrics with explicit user consent.
- **Authoritative Health State Taxonomy:** Synchronized 10-state health taxonomy (`HEALTHY`, `DEGRADED`, `OFFLINE`, `POLICY_STALE`, etc.) across desktop, backend, and dashboard.

---

## 3. Included Artifacts & Distribution Files

| Component | Target Platform | Architecture | Description |
|---|---|---|---|
| `onusly-agent-windows-amd64.exe` | Windows 10/11 | AMD64 | Windows SCM service daemon |
| `onusly-native-host-windows-amd64.exe` | Windows 10/11 | AMD64 | Browser native messaging host |
| `onusly-agent-linux-amd64` | Linux | AMD64 | Linux systemd service daemon |
| `onusly-native-host-linux-amd64` | Linux | AMD64 | Linux browser native messaging host |
| `RELEASE_MANIFEST.json` | All | N/A | Release metadata & component inventory |
| `CHECKSUMS.txt` | All | N/A | SHA-256 cryptographic verification hashes |
| `sbom-go.json` | All | N/A | Software Bill of Materials for Go dependencies |
| `sbom-npm.json` | All | N/A | Software Bill of Materials for NPM dependencies |

---

## 4. Upgrade & Rollback Procedures

- **Upgrades:** Upgrades are staged in isolated staging directories and verified with Ed25519 signatures before atomic binary replacement.
- **Downgrade Protection:** Downgrades to versions with lower security counters are cryptographically rejected.
- **Emergency Rollback:** If an emergency rollback is required, operators can issue an update manifest signed with an incremented security version pointing to the previous stable release binary as detailed in `UPDATE_ROLLBACK_RUNBOOK.md`.

---

## 5. Beta Feedback & Support

- Beta participants can submit sanitized support bundles generated via:
  ```powershell
  onusly-agent support bundle
  ```
- Support Manual: `docs/runbooks/BETA_SUPPORT_AND_TROUBLESHOOTING.md`
- Operational Incident Runbooks: `docs/runbooks/`

