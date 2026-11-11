# ONUSLY Staging & Controlled Beta Release Checklist

**Release Candidate:** v1.0.0-rc1  
**Target Environment:** Isolated Staging / Controlled Beta  
**Safety Invariant:** All items must be signed off before deploying a build to beta cohorts.

---

## 1. Environment & Secret Isolation Checklist

- [ ] **Staging Preflight Validation:** Run `scripts/staging_preflight.ps1 -Environment "staging"` and confirm exit code 0.
- [ ] **Database Table Segregation:** Verify `DYNAMODB_TABLE` is set to an isolated table (e.g. `Onusly-Staging`), strictly NOT `Onusly`.
- [ ] **Cryptographic Secret Hygiene:** Verify `JWT_SECRET` is at least 32 characters and contains no weak dictionary words. Ensure secrets are never printed in logs or terminal output.
- [ ] **CORS & Allowed Origins:** Ensure staging allowed origins do not point to production domains (`onusly.com`) or insecure external endpoints.
- [ ] **Toolchain Requirements:** Confirm Go $\ge 1.22$, Node.js $\ge 18$, and PowerShell $\ge 5.1$.

---

## 2. Release Build & Artifact Packaging Checklist

- [ ] **Automated Packaging Workflow:** Run `scripts/build_staging_release.ps1` to assemble all components into `build/staging/`.
- [ ] **Windows AMD64 Binaries:** Verify `onusly-agent-windows-amd64.exe` and `onusly-native-host-windows-amd64.exe` are compiled.
- [ ] **Linux AMD64 Binaries:** Verify `onusly-agent-linux-amd64` and `onusly-native-host-linux-amd64` are cross-compiled.
- [ ] **Mobile Android Verification:** Verify `android/` Gradle project and manifest validity.
- [ ] **Cryptographic Manifest Signing:** Confirm Ed25519 update manifests are signed and signature verification passes.
- [ ] **Checksum Integrity:** Confirm `CHECKSUMS.txt` contains SHA-256 digests matching every binary and metadata file.
- [ ] **Software Bill of Materials (SBOM):** Confirm `sbom-go.json` and `sbom-npm.json` are present and non-empty.

---

## 3. Automated Regression & Acceptance Gates

- [ ] **Database Backup & Disaster Recovery Rehearsal:** Run `go test ./internal/enforcement -run TestD15_DatabaseBackupAndRestoreRehearsal`.
- [ ] **Staging User Journey Acceptance Test:** Run `desktop/installer/windows/test_d15_staging_acceptance.ps1`.
- [ ] **D14 SRE Observability Suite:** Run `desktop/installer/windows/test_d14_observability_acceptance.ps1`.
- [ ] **D13 Application Security Suite:** Run `desktop/installer/windows/test_d13_security_acceptance.ps1`.
- [ ] **D12 Android Integration Suite:** Run `desktop/installer/windows/test_d12_android_acceptance.ps1`.
- [ ] **D11 Accountability Workflow Suite:** Run `desktop/installer/windows/test_d11_workflow_acceptance.ps1`.
- [ ] **D10 Backend Consistency Suite:** Run `desktop/installer/windows/test_d10_backend_acceptance.ps1`.
- [ ] **D9 Software Update Suite:** Run `desktop/installer/windows/test_d9_update_acceptance.ps1`.
- [ ] **D8 Device Lifecycle Suite:** Run `desktop/installer/windows/test_d8_lifecycle_acceptance.ps1`.
- [ ] **D7 Platform Fault Injection Suite:** Run `desktop/installer/windows/test_d7_fault_injection.ps1`.
- [ ] **D6 Desktop Enforcement Suite:** Run `desktop/installer/windows/test_d6_acceptance.ps1`.

---

## 4. Operational Observability & Support Readiness

- [ ] **Prometheus Metrics Exposition:** Confirm `GET /metrics` responds with valid Prometheus text metric output.
- [ ] **Support Bundle CLI Tool:** Test `onusly-agent support bundle --yes` and verify generated JSON is privacy-safe.
- [ ] **Incident Response Runbooks:** Confirm all 5 runbooks exist in `docs/runbooks/` (`INCIDENT_RESPONSE_*.md`).
- [ ] **Disaster Recovery Runbook:** Confirm `DATABASE_BACKUP_AND_RESTORE.md` exists and RTO/RPO targets are specified.
- [ ] **Beta Rollout Strategy:** Confirm `BETA_ROLLOUT_PLAN.md` exists and automated rollback thresholds are defined.

---

## 5. Live Service & Device Identity Invariant

- [ ] **SCM Service Status:** Confirm Windows service `OnuslyDesktopAgent` is in `Running` state under `NT AUTHORITY\LocalService`.
- [ ] **Canonical Device ID:** Confirm `C:\ProgramData\Onusly\config\device_id` matches `143489b2-2fd8-428b-bfa8-2631cc2be8c3`.

