# Runbook: Secure Software Update Protocol, Verification & Rollback Procedures

## 1. Executive Overview & Security Invariants

ONUSLY enforces strict desktop accountability contracts across Windows and Linux. The software update architecture preserves system security, process immunity, credential isolation, and fail-closed accountability guarantees:

- **Unprivileged Staging Isolation:** The running background service (`NT AUTHORITY\LocalService` on Windows, unprivileged daemon context on Linux) never modifies installed binaries in `C:\Program Files\Onusly` or `/opt/onusly`. Updates are retrieved and cryptographically verified in unprivileged staging space (`%ProgramData%\Onusly\staging` or `/var/lib/onusly/staging`).
- **Cryptographic Elevation Boundary:** Elevated installers (`desktop/installer/windows/install.ps1` and `desktop/installer/linux/main.go`) perform mandatory pre-execution signature and artifact digest re-verification. Unsigned, untrusted, or tampered payloads are rejected prior to service stoppage or file modification.
- **Ed25519 Canonical Verification:** All release manifests are signed using standard RFC 8032 Ed25519 over deterministic, canonical JSON (lexicographically ordered keys, compact delimiters, no trailing newlines, zeroed signature field).
- **Anti-Rollback Protection:** Every update verifies that incoming releases strictly exceed or equal the current release SemVer 2.0 (`release_version`) and monotonic security version (`security_version`). Downgrades are rejected unless explicitly authorized via operator override (`--allow-downgrade`).
- **Identity & State Invariance:** Device identity (`device_id`), DPAPI/secret credentials, local DNS recovery state (`dns_backup.json`), and LKG policy caches are strictly preserved across upgrades and transactional rollbacks.
- **Transactional Rollback:** If binary replacement, ACL configuration, SCM restart, or IPC health validation fails at any of the 11 defined failure boundaries, the installer executes immediate, deterministic rollback restoring previous binaries, configurations, and service states.

---

## 2. Signed Update Manifest Architecture

### 2.1 Manifest Schema (Specification v1)

Update manifests are distributed as `manifest.json` accompanied by a hex-encoded Ed25519 signature embedded within the manifest structure or served via authenticated endpoints.

```json
{
  "schema_version": 1,
  "product": "onusly",
  "release_version": "1.0.0",
  "security_version": 1,
  "platform": "windows",
  "architecture": "amd64",
  "channel": "stable",
  "created_at": "2026-11-11T12:00:00Z",
  "expires_at": "2026-12-11T12:00:00Z",
  "components": {
    "agent": {
      "filename": "onusly-agent.exe",
      "sha256": "4b84dd004818fa8db3463a5647565d774847e9282367cb6eb8838234bb4fbc0d",
      "size_bytes": 14508032,
      "target_path": "onusly-agent.exe"
    },
    "native_host": {
      "filename": "onusly-native-host.exe",
      "sha256": "8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d0e9f8a7b",
      "size_bytes": 8912896,
      "target_path": "onusly-native-host.exe"
    }
  },
  "signature": "3f7a1b...<128-char-hex-encoded-ed25519-signature>..."
}
```

### 2.2 Canonical JSON Serialization
To guarantee byte-for-byte reproducibility regardless of JSON encoder formatting, signature generation and verification execute over canonical JSON:
1. Manifest fields are serialized without indentation or extraneous whitespace (separators `,` and `:`).
2. All keys are sorted lexicographically.
3. The `signature` field is excluded or set to empty string `""`.
4. No trailing newline or carriage return is appended.

### 2.3 Signing Procedure (Release Pipeline)
```bash
# Example automated signing script using onusly-agent release builder:
onusly-agent update build-manifest \
  --dir ./build/windows_amd64 \
  --version 1.0.0 \
  --security-version 1 \
  --platform windows \
  --arch amd64 \
  --channel stable \
  --valid-days 30 \
  --private-key-file /run/secrets/release_ed25519.key \
  --output ./dist/manifest.json
```

---

## 3. Production Key Management Plan

### 3.1 Key Architecture & Cryptographic Primitives
- **Algorithm:** Ed25519 (Edwards-curve Digital Signature Algorithm, RFC 8032).
- **Public Key Size:** 32 bytes (64 hex characters).
- **Signature Size:** 64 bytes (128 hex characters).
- **Pinned Key Embedding:** Desktop agent binaries compile with an embedded root public key or read authorized keys from cryptographically protected registry/configuration paths (`HKLM:\SOFTWARE\Onusly\UpdatePublicKey`).

### 3.2 Key Lifecycle & HSM / Cloud KMS Integration
1. **Root Signing Key Generation:**
   - Production Ed25519 private keys **must never** reside on operator workstations, build runners, or developer laptops.
   - Keys are generated directly inside FIPS 140-2 Level 3 Hardware Security Modules (HSMs) or Cloud KMS providers (AWS KMS asymmetric signing, Google Cloud KMS, or Azure Key Vault HSM).
2. **Multi-Operator Quorum (M-of-N):**
   - Release signing operations in production require approval from at least two authorized security officers.
   - Build automation generates un-signed candidate manifests; release gate triggers HSM signing only upon signed Git tag verification and passing automated CI/CD staging tests.
3. **Environment Segregation:**
   - `development`: Ephemeral test keys generated during test runs and discarded immediately.
   - `staging`: Dedicated staging Ed25519 key pair embedded in staging builds.
   - `production`: Dedicated production Ed25519 key pair with strict hardware isolation.

### 3.3 Key Rotation Procedure
1. **Pre-flight Trust Anchor Distribution:**
   - Manifest schema supports dual-key rotation grace periods.
   - The desktop agent supports secondary trusted public keys during scheduled transition windows.
   - Deploy release `N` embedding both the current public key (`Key-A`) and future public key (`Key-B`).
2. **Cut-Over:**
   - Release `N+1` manifests are signed with `Key-B`.
   - Release `N+2` removes `Key-A` trust anchor.
3. **Emergency Key Compromise Playbook:**
   - If a signing key compromise is suspected:
     1. Revoke the key immediately in backend device sync policies (`/api/v1/enforcement/device/policy`).
     2. Dispatch an out-of-band enterprise policy update via MDM/GPO updating `UpdatePublicKey` registry values.
     3. Issue emergency hotfix signed with secondary recovery key invalidating all prior manifests via elevated `security_version` counter.

---

## 4. 12-Point Cryptographic & Policy Verification Matrix

Prior to accepting any staged update, `desktop/core/update/verifier.go` enforces 12 distinct criteria. If any check fails, the update is rejected with an explicit error code:

| Point | Verification Step | Description | Error Code |
|---|---|---|---|
| **1** | Non-empty Signature | Manifest contains valid 128-hex-character Ed25519 signature | `ErrSignatureMissing` |
| **2** | Canonical Ed25519 Signature | Cryptographic verification over canonical JSON using trusted public key | `ErrSignatureInvalid` |
| **3** | Schema Version | `schema_version` equals current supported schema (`1`) | `ErrUnsupportedSchema` |
| **4** | Product Match | `product` equals canonical identifier (`onusly`) | `ErrInvalidProduct` |
| **5** | Platform Match | `platform` matches runtime operating system (`windows` or `linux`) | `ErrPlatformMismatch` |
| **6** | Architecture Match | `architecture` matches runtime CPU (`amd64` or `arm64`) | `ErrArchitectureMismatch` |
| **7** | Manifest Expiry | `expires_at` timestamp has not passed (rejects stale manifests) | `ErrManifestExpired` |
| **8** | Creation Timestamp | `created_at` timestamp is not from the future (max clock drift: 5 min) | `ErrManifestFutureTimestamp` |
| **9** | Monotonic Security Version | `security_version` >= local persisted security counter | `ErrAntiRollbackViolation` |
| **10** | SemVer 2.0 Version Order | `release_version` >= local installed version (unless `--allow-downgrade`) | `ErrVersionDowngrade` |
| **11** | Path Traversal Protection | All component `filename` and `target_path` strings contain no directory traversal (`..`, `/`, `\`) | `ErrPathTraversalDetected` |
| **12** | Artifact Digest & Size Match | Component files match exact byte count and lowercase SHA-256 digest | `ErrArtifactDigestMismatch` |

---

## 5. Windows Operator Update Workflow

### 5.1 Step 1: Agent Background Staging (Unprivileged)
The unprivileged background service or operator CLI downloads and validates the update payload in `%ProgramData%\Onusly\staging`:

```powershell
# Check for available updates
onusly-agent.exe update check --manifest-url https://updates.onusly.com/v1/windows/amd64/manifest.json

# Download and cryptographically stage components to %ProgramData%\Onusly\staging
onusly-agent.exe update stage \
  --manifest-url https://updates.onusly.com/v1/windows/amd64/manifest.json \
  --staging-dir "C:\ProgramData\Onusly\staging"
```

### 5.2 Step 2: Elevation Boundary Verification & Installation
An elevated administrator or scheduled deployment script executes the upgrade using the staged directory:

```powershell
# Run from elevated Administrator PowerShell prompt:
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\desktop\installer\windows\install.ps1 -Upgrade -StagedDir "C:\ProgramData\Onusly\staging" -Verbose
```

### 5.3 Automated Actions During Windows Upgrade
1. **Pre-flight Boundary Dual-Verification:** Runs `onusly-agent update verify` on the staged directory. If signature or digests do not match, aborts immediately without touching active services or files.
2. **Service Cessation:** Stops `OnuslyDesktopAgent` SCM service cleanly (waiting up to 30s).
3. **Transactional Backup:** Copies active binaries from `C:\Program Files\Onusly` to `C:\Program Files\Onusly\backup_<timestamp>`.
4. **Atomic Binary Deployment:** Moves staged, verified binaries into `C:\Program Files\Onusly`.
5. **ACL Enforcement:** Restores strict ACLs (`SYSTEM` & `Administrators`: Full Control; `LocalService`: Read & Execute).
6. **SCM Service Start:** Launches `OnuslyDesktopAgent` under `NT AUTHORITY\LocalService`.
7. **Health Verification:** Verifies service status is `RUNNING` and probes named pipe `\\.\pipe\onusly-agent`.
8. **Cleanup / Rollback:** On success, removes staging files and temporary backups. On failure, triggers automatic transactional rollback.

---

## 6. Linux Operator Update Workflow

### 6.1 Step 1: Daemon Background Staging (Unprivileged)
```bash
# Stage update components in /var/lib/onusly/staging
/usr/local/bin/onusly-agent update stage \
  --manifest-url https://updates.onusly.com/v1/linux/amd64/manifest.json \
  --staging-dir /var/lib/onusly/staging
```

### 6.2 Step 2: Elevated Installer Execution
```bash
# Execute elevated upgrade as root:
sudo /usr/local/bin/onusly-installer \
  -upgrade \
  -staged-dir /var/lib/onusly/staging
```

### 6.3 Automated Actions During Linux Upgrade
1. **Ed25519 & Digest Verification:** Installer verifies manifest and component SHA-256 digests against pinned public key before stopping services.
2. **Backup Generation:** Moves `/usr/local/bin/onusly-agent` to `/usr/local/bin/onusly-agent.bak`.
3. **Atomic File Replace:** Copies staged binary into `/usr/local/bin/` with mode `0755` root ownership.
4. **Service Reload & Restart:** Executes `systemctl restart onusly-agent.service`.
5. **IPC Socket Health Probe:** Verifies `/run/onusly/agent.sock` is active and responsive within 10 seconds.
6. **Automatic Rollback:** If health probe fails, restores `.bak` binary and restarts service.

---

## 7. Anti-Rollback & Version Downgrade Policy

### 7.1 SemVer 2.0 Evaluation Rules
Version comparisons strictly adhere to SemVer 2.0.0 Rule 9 (pre-release precedence):
- `1.0.0` > `1.0.0-rc2` > `1.0.0-rc1` > `1.0.0-beta` > `0.9.0`.
- An update from `1.0.0` to `1.0.0-rc1` is classified as a downgrade and rejected.

### 7.2 Monotonic Security Version Counter
- Every manifest carries a non-decreasing integer `security_version`.
- Persisted locally in `%ProgramData%\Onusly\anti_rollback.json` or `/var/lib/onusly/anti_rollback.json`.
- Even if a binary vulnerability is packaged with a higher nominal SemVer version (e.g. through build metadata tampering), any downgrade in `security_version` is permanently blocked.

### 7.3 Authorized Operator Downgrade
In emergency situations where a hotfix must be rolled back to an earlier stable release:
```powershell
# Windows
.\desktop\installer\windows\install.ps1 -Upgrade -StagedDir "C:\ProgramData\Onusly\staging" -AllowDowngrade -Verbose

# Linux
sudo /usr/local/bin/onusly-installer -upgrade -staged-dir /var/lib/onusly/staging -allow-downgrade
```
*Note: `--allow-downgrade` permits nominal SemVer downgrades, but monotonic `security_version` downgrade remains blocked to prevent exploitation of known cryptographic vulnerabilities.*

---

## 8. Automated & Emergency Rollback Procedures

### 8.1 Transactional Rollback across 11 Failure Boundaries
The installer implements deterministic rollback handling for each stage of upgrade execution:

1. `ManifestStaging`: Staging directory cleanup; active service untouched.
2. `SignatureVerification`: Abort with exit code 1; zero file changes.
3. `DigestVerification`: Abort with exit code 1; zero file changes.
4. `ServiceStop`: Restart original service if shutdown times out.
5. `BackupCreation`: Remove partial backup folder; abort.
6. `AgentBinaryDeploy`: Restore `onusly-agent.exe` from backup directory.
7. `NativeHostBinaryDeploy`: Restore `onusly-native-host.exe` from backup directory.
8. `AclApplication`: Restore original filesystem ACLs.
9. `ScmConfiguration`: Re-apply original service binary path and configuration.
10. `ServiceStart`: If new binary fails to enter `RUNNING` state within 30s, restore backup binaries and restart original service.
11. `HealthVerification`: If named pipe / Unix socket fails to accept IPC connections, execute full file and service rollback.

### 8.2 Manual Emergency Recovery Playbook

If an unrecoverable crash or power interruption occurs mid-upgrade:

#### Windows Manual Recovery
```powershell
# 1. Stop orphaned processes
Stop-Process -Name onusly-agent, onusly-native-host -Force -ErrorAction SilentlyContinue

# 2. Check for backup directories
Get-ChildItem "C:\Program Files\Onusly\backup_*" | Sort-Object CreationTime -Descending | Select-Object -First 1

# 3. Restore binaries from latest backup
Copy-Item "C:\Program Files\Onusly\backup_<timestamp>\*.exe" "C:\Program Files\Onusly\" -Force

# 4. Execute component repair
.\desktop\installer\windows\install.ps1 -Repair -Verbose

# 5. Verify service health
Get-Service OnuslyDesktopAgent
onusly-agent.exe status
```

#### Linux Manual Recovery
```bash
# 1. Stop active unit
sudo systemctl stop onusly-agent.service

# 2. Restore backup binary
if [ -f /usr/local/bin/onusly-agent.bak ]; then
    sudo mv /usr/local/bin/onusly-agent.bak /usr/local/bin/onusly-agent
    sudo chmod 0755 /usr/local/bin/onusly-agent
fi

# 3. Restart and verify
sudo systemctl restart onusly-agent.service
sudo systemctl status onusly-agent.service
```

