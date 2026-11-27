# ONUSLY Production Remediation Deployment Runbook

This runbook documents the exact operator steps to promote verified fixes from branch `fix/onusly-audit-remediation` into live production (`13.49.49.2` / `onusly.duckdns.org` in `eu-north-1`) safely, with rollback procedures.

> **Do not deploy during active peak user traffic without taking a snapshot/backup first.**

---

## 1. Pre-Flight Infrastructure Prerequisites

### 1.1 Resolve EC2 Root EBS Capacity & Encryption (`AWS-001`)
Live root EBS volume `vol-087b33f32d9b0d6ca` (`8 GiB`) sits at `91%` utilization (`~810 MB` free). Before running any build on EC2:

1. Expand the EBS volume in AWS (`eu-north-1`) to at least `20 GiB`:
   ```bash
   aws ec2 modify-volume --region eu-north-1 --volume-id vol-087b33f32d9b0d6ca --size 20
   ```
2. On EC2 (`ssh -i <key.pem> ec2-user@13.49.49.2`), grow the partition and XFS/ext4 filesystem:
   ```bash
   sudo growpart /dev/nvme0n1 1
   sudo xfs_growfs -d /   # or resize2fs /dev/nvme0n1p1
   df -h /
   ```
3. *(Recommended security hardening)* Take an encrypted snapshot of the expanded volume and launch/reattach an encrypted EBS root volume during a planned maintenance window so `/opt/onusly/uploads/proofs/` is encrypted at rest (`SECURITY-001` / `AWS-001`).

### 1.2 Provision SQS Queues & CloudWatch Alarms (`AWS-002`, `AWS-004`)
Run the repository provisioning script with appropriate IAM permissions (`sqs:*`, `cloudwatch:PutMetricAlarm`):

```bash
ONUSLY_EC2_INSTANCE_ID=i-<instance-id> ./deploy/aws/setup_queues_and_alarms.sh
```

Append the output queue URLs into `/etc/onusly/onusly.env` on EC2:

```bash
PROOF_EVENTS_QUEUE_URL=https://sqs.eu-north-1.amazonaws.com/631690098190/proof-events
APPROVAL_EVENTS_QUEUE_URL=https://sqs.eu-north-1.amazonaws.com/631690098190/approval-events
```

---

## 2. Controlled Application Deployment Cutover

Once `fix/onusly-audit-remediation` is merged into `main` (or deploying its exact reviewed commit SHA):

1. Check pre-deployment disk space and free space in `/opt/onusly` if needed (`df -h /`).
2. Run the production deployment script pinning the exact reviewed commit SHA:
   ```bash
   cd /opt/onusly
   sudo ./deploy/setup_ec2.sh --commit <VERIFIED_COMMIT_SHA>
   ```
   `deploy/setup_ec2.sh` performs:
   - Go server build (`server.new` atomic swap + `server.bak` retained to latest 2 + `go clean -cache -testcache`)
   - Windows `.exe` cross-compile into `frontend/public/downloads/`
   - Vite frontend build & atomic copy into `/var/www/onusly/frontend/dist`
   - Nginx config install (`deploy/nginx/onusly.conf`), `nginx -t` validation, and automatic rollback on failure
   - Strict `0700` permissions on `/opt/onusly/uploads` and `/opt/onusly/uploads/proofs/.meta`
   - `systemd` restart & keep-warm check

3. *(Wait on `/opt/onusly/uploads/proofs/` legacy files)* Any proof uploaded **before** this release lacks a `.meta/<filename>.json` sidecar; authorized approvers/owners passing `?goalId=<id>` continue to work via `CanAccessUpload`.

---

## 3. Post-Deployment Verification Checklist

Run these non-destructive verification checks immediately after cutover:

```bash
# 1. Backend & Nginx health
curl -fsS https://onusly.duckdns.org/health

# 2. SPA refresh routes return HTML index (WEB-001)
for p in /goals /approvals /friends /communities /wellbeing /privacy /terms; do
  code=$(curl -s -o /dev/null -w "%{http_code}" -H "Accept: text/html" "https://onusly.duckdns.org${p}")
  echo "$p -> HTTP $code"
done

# 3. HSTS + Favicon + Static Downloads
curl -fsSI https://onusly.duckdns.org/ | grep -i strict-transport-security
curl -fsSI https://onusly.duckdns.org/favicon.ico | head -n 1
curl -fsSI https://onusly.duckdns.org/downloads/OnuslySetup.exe | head -n 1

# 4. Internal boundaries blocked externally
curl -s -o /dev/null -w "/metrics=%{http_code} /health/warm=%{http_code}\n" \
  https://onusly.duckdns.org/metrics https://onusly.duckdns.org/health/warm

# 5. Live WebSocket cookie pre-auth test (API-001) & PNG/JPG proof test (PROOF-001)
# Sign in on https://onusly.duckdns.org with a QA test account and confirm:
# - DevTools Network -> /ws remains Status 101 Switching Protocols with "auth.success" frame
# - Uploading + viewing a .png/.jpg proof renders HTTP 200 image/png or image/jpeg
# - Unrelated user account receives HTTP 403 on the proof image URL
```

---

## 4. Rollback Procedure

If any verification step fails:

```bash
# Restore previous Go binary and Nginx config atomically on EC2:
sudo cp -f "$(ls -1t /opt/onusly/server.bak.* | head -n1)" /opt/onusly/server
sudo cp -f "$(ls -1t /etc/nginx/conf.d/onusly.conf.bak.* | head -n1)" /etc/nginx/conf.d/onusly.conf
sudo nginx -t && sudo systemctl restart nginx
sudo systemctl restart onusly.service
```
