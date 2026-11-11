# ONUSLY Operational Runbook: Database Backup, Export & Disaster Recovery Rehearsal

**Document ID:** ONUSLY-RB-D15-01  
**Target Audience:** SREs, DevOps Engineers, Infrastructure Operators  
**Applicability:** Production, Staging, and Disaster Recovery Environments  
**Authoritative Control Plane:** AWS DynamoDB (Single-Table Design)  
**Safety Invariant:** Restorations MUST rehearse into isolated tables (`Onusly-Staging-Recovery`) before any production cutover.

---

## 1. Objectives & Service Level Objectives (SLOs)

| Metric | Target | Description |
|---|---|---|
| **Recovery Point Objective (RPO)** | $\le 5$ minutes | Continuous Point-in-Time Recovery (PITR) window |
| **Recovery Time Objective (RTO)** | $\le 30$ minutes | Full table restoration and policy cache re-indexing |
| **Integrity Verification** | 100% | Zero loss of device ownership, typed application targets, or policy versions |

---

## 2. Backup Mechanisms

### 2.1 Continuous Point-in-Time Recovery (PITR)
Production DynamoDB tables (`Onusly`) have PITR enabled, maintaining incremental change journals for 35 days:
```bash
aws dynamodb update-continuous-backups \
    --table-name Onusly \
    --point-in-time-recovery-specification PointInTimeRecoveryEnabled=true
```

### 2.2 On-Demand Snapshot Creation
Prior to any major release, migration, or staging rehearsal, trigger an immediate on-demand backup:
```bash
aws dynamodb create-backup \
    --table-name Onusly \
    --backup-name "Onusly-PreRelease-$(date +%Y%m%d%H%M%S)"
```

### 2.3 S3 Export for Disaster Recovery Archive
For cold-storage archival and offline analysis:
```bash
aws dynamodb export-table-to-point-in-time \
    --table-arn "arn:aws:dynamodb:us-east-1:123456789012:table/Onusly" \
    --s3-bucket "onusly-backups-us-east-1" \
    --s3-prefix "exports/$(date +%Y-%m-%d)/" \
    --export-format DYNAMODB_JSON
```

---

## 3. Schema & Integrity Validation Invariants

Every backup archive or restored snapshot must satisfy the following structural invariants:

1. **Device Identity & State**:
   - `PK` format: `USER#<user-uuid>`
   - `SK` format: `DEVICE#<device-uuid>`
   - `Status`: `paired` | `revoked` | `auth_required`
   - Active devices must have valid `HealthStatus` and `LastHeartbeatAt`.
   - Revoked devices must retain `RevokedAt` and `RevocationReason`.
2. **Accountability Goals & Typed Targets**:
   - `PK`: `GOAL#<goal-uuid>`, `SK`: `METADATA`
   - Explicitly typed application targets must be segregated:
     - `desktop_app`: Binary executables (`.exe`, ELF binaries).
     - `android_package`: Android reverse-domain package names (`com.*`, `org.*`).
   - Domain targets: FQDNs without protocol schemes (`example.com`).
   - Goal `Version`: Integer sequence counter for ETag policy matching.
3. **Cryptographic Checksum Verification**:
   - Backup manifests must contain SHA-256 hashes matching payload items.

---

## 4. Staging Restoration Dry-Run Rehearsal

### Step 1: Restore to Isolated Staging Target Table
Never restore directly into the production table name. Restore into `Onusly-Staging-Recovery`:
```bash
aws dynamodb restore-table-to-point-in-time \
    --source-table-name Onusly \
    --target-table-name Onusly-Staging-Recovery \
    --use-latest-restorable-time
```
Or restore from on-demand backup ARN:
```bash
aws dynamodb restore-table-from-backup \
    --target-table-name Onusly-Staging-Recovery \
    --backup-arn "arn:aws:dynamodb:us-east-1:123456789012:table/Onusly/backup/0123456789"
```

### Step 2: Configure Staging Backend to Target Recovery Table
Update the staging environment variable:
```bash
export DYNAMODB_TABLE=Onusly-Staging-Recovery
export ENV=staging
```

### Step 3: Execute Automated Integrity Test Suite
Run the D15 automated rehearsal test suite:
```powershell
go test -v ./internal/enforcement -run TestD15_DatabaseBackupAndRestoreRehearsal
```

### Step 4: Verification Checklist
Verify via queries:
1. Active device count matches pre-disaster metrics:
   ```bash
   aws dynamodb query \
       --table-name Onusly-Staging-Recovery \
       --key-condition-expression "PK = :pk" \
       --expression-attribute-values '{":pk":{"S":"USER#test-user"}}'
   ```
2. Policy ETag versions match previous snapshot values.
3. Heartbeat listener accepts reports against restored records without generating false lapse alerts.

---

## 5. Escalation & Incident Contacts
- **Primary On-Call:** SRE Team (`sre-oncall@onusly.internal`)
- **Backend Lead:** Platform Reliability Team
- **Security Lead:** Application Security (`security@onusly.internal`)

