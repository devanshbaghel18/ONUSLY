# ONUSLY Phase 2 Remediation Report

**Branch:** `fix/onusly-audit-remediation` (from `main` @ `db9cb1121cc7736310dc37222a42da80fa113c52`)  
**Reference Audit:** `issue.txt`  
**Deployment Runbook:** `docs/runbooks/PRODUCTION_REMEDIATION_DEPLOYMENT.md`  
**Live Production Safety:** Zero changes made to live running `/opt/onusly/server`, `onusly.service`, `/etc/nginx/conf.d/onusly.conf`, or live AWS resources during remediation. Code synchronized to EC2 strictly inside an isolated checkout directory.

---

## Executive Remediation Summary

Across all **20 findings** from `issue.txt`:

- **7 Issues (`FIXED AND VERIFIED`):** `API-001`, `SECURITY-001`, `WEB-002`, `AWS-003`, `SECURITY-002`, `WEB-003`, `GOAL-001`
- **4 Issues (`FIXED — RUNTIME VERIFICATION PENDING`):** `PROOF-001`, `WEB-001`, `API-002` (live EC2 Nginx reload deferred until maintenance window), `WIN-001`, `WIN-003` (**5 total including Windows**; cross-compiled/vetted cleanly for `windows/amd64`, awaiting Windows 10/11 VM execution)
- **7 Issues (`PARTIALLY FIXED`):** `AWS-001`, `AWS-002`, `AWS-004`, `WIN-002`, `SECURITY-003`, `PERF-001`, `INFO-001` (all code/deploy-script pieces fixed; live EBS expansion, live SQS/CloudWatch provisioning, CloudFront CDN, Authenticode signing, and legacy localStorage bridge removal documented as explicit follow-ups)
- **1 Issue (`NOT A BUG — INVESTIGATED`):** `INFO-002` (intentional asymmetric hero art bleed with zero document horizontal overflow)

---

## Issue-by-Issue Remediation Matrix

| ID | Severity | Status | Key Changes | Verification Evidence |
|---|---|---|---|---|
| **API-001** | CRITICAL | `FIXED AND VERIFIED` | `internal/realtime/handler.go` emits `auth.success` immediately on cookie/`Authorization` pre-auth upgrade; `internal/realtime/hub.go` handles idempotent `case "auth":` without error | `TestServeWS_CookiePreAuthEmitsAuthSuccessAndSupportsIdempotentAuthFrame` (`go test -race`) |
| **PROOF-001** | CRITICAL | `FIXED — RUNTIME VERIFICATION PENDING` | `deploy/nginx/onusly.conf` adds `location ^~ /uploads/` before regex rules and restricts 1y static asset caching strictly to `location ^~ /assets/` | Inspected routing precedence; live EC2 Nginx reload deferred per production freeze |
| **SECURITY-001** | CRITICAL | `FIXED AND VERIFIED` | Added `internal/proof/upload_acl.go`, `cmd/server/uploads.go`, and updated `internal/proof/service.go` (`0700`/`0600` dirs/files, owner+goal approver ACLs, `.meta` dotdir block, owner verification before binding uploads) | `TestUploadsACLAuthorization` (401 unauth, 200 owner, 200 approver, 403 unrelated user, `.meta` rejected) |
| **WEB-001** | HIGH | `FIXED — RUNTIME VERIFICATION PENDING` | `deploy/nginx/onusly.conf` adds `$onusly_serve_spa` `map` rewriting browser HTML `GET`/`HEAD` requests (no `Authorization` header) on shared SPA/API routes to `/index.html`, while preserving `/auth/google*`, `/auth/logout`, `/health`, `/uploads/`, `/downloads/` | Verified SPA/API route branching; live EC2 Nginx cutover deferred |
| **WEB-002** | HIGH | `FIXED AND VERIFIED` | `frontend/src/components/film.css` hides nav links below `700px`, wraps `.film-timeline nav`, prevents horizontal stage bulge (`overflow-x: clip`) | Headless Chrome CDP check on built `vite preview`: `390x844 -> scrollWidth=390`, `320x700 -> scrollWidth=320` |
| **AWS-001** | HIGH | `PARTIALLY FIXED` | `deploy/setup_ec2.sh` clears Go build cache (`go clean -cache -testcache`), prunes `.bak` to latest 2, tightens `/uploads` perms (`0700`); runbook documents EBS volume expansion & encryption | `bash -n deploy/setup_ec2.sh`; live AWS EBS volume untouched |
| **AWS-002** | HIGH | `PARTIALLY FIXED` | Added `deploy/aws/setup_queues_and_alarms.sh` to idempotently create `proof-events`, `approval-events`, and DLQs + `docs/runbooks/PRODUCTION_REMEDIATION_DEPLOYMENT.md` env setup | `bash -n deploy/aws/setup_queues_and_alarms.sh`; live AWS SQS untouched |
| **AWS-003** | HIGH | `FIXED AND VERIFIED` | `internal/goals/handler.go` propagates DB `List` errors with `503`; `internal/goals/repository.go` gates table `Scan` fallback behind `ENABLE_LEGACY_GOAL_SCAN=true`; `proof/approval/enforcement/repository.go` propagate write errors & use `3s` timeout | `TestGoal_UntargetedCommunityRejectedAndListErrorsPropagate` + `go test -race ./...` |
| **SECURITY-002** | HIGH | `FIXED AND VERIFIED` | `cmd/server/main.go` enforces `RequireUserToken` across all user mutations/sensitive reads; `internal/middleware/auth.go` validates `token_version` for both `user` & `device` tokens and fails closed (`503`) on DB error; `internal/auth/repository.go` & `internal/realtime/handler.go` fail closed; `internal/enforcement/handler.go` forwards `X-Device-ID` for revocation check | `TestD13_DeviceTokenVersionRevocationAndFailClosedValidator`, `TestD8_GetBlocklistRejectsRevokedXDeviceIDOnUserToken` |
| **WIN-001** | HIGH | `FIXED — RUNTIME VERIFICATION PENDING` | `desktop/installer/cmd/installer/main.go` sets `win.SECURITY_SQOS_PRESENT | win.SECURITY_IDENTIFICATION` in `dialNamedPipe` and removes dead `net.Dial("unix")` call | `GOOS=windows GOARCH=amd64 go vet ./desktop/...`; no Windows VM available |
| **WIN-002** | HIGH | `PARTIALLY FIXED` | `desktop/installer/cmd/installer/main.go` adds `-uninstall` flag and HKCU Uninstall registry entry; `desktop/agent/cmd/switcher/main.go` supports both `C:\ProgramData` and `%LOCALAPPDATA%` `dns_backup.json` and updates default URL to `https://onusly.duckdns.org`. Full UAC `LocalService` elevation & Authenticode signing remain follow-ups | `GOOS=windows GOARCH=amd64 go vet ./desktop/...` |
| **AWS-004** | MEDIUM | `PARTIALLY FIXED` | Redacted raw user email/handle PII from `internal/realtime/hub.go` logs; added CloudWatch EC2 StatusCheckFailed & CPUUtilization alarm creation to `deploy/aws/setup_queues_and_alarms.sh` | `go test -race ./internal/realtime`; live CW setup pending |
| **SECURITY-003** | MEDIUM | `PARTIALLY FIXED` | Added `Strict-Transport-Security` header in `deploy/nginx/onusly.conf`; fixed `frontend/src/lib/auth.js` `isAuthenticated()` fail-closed validation; removed legacy unused GCP client ID `316293252544-...` in `internal/auth/service.go`; release builds in `AuthManager.kt` throw `SecurityException` instead of plaintext fallback | `frontend/tests/auth.test.js`, Android `./android/gradlew testDebugUnitTest`, `go test ./internal/auth`; extension `localStorage` bridge keys kept for compatibility |
| **WEB-003** | MEDIUM | `FIXED AND VERIFIED` | Fixed `"Windows & Mac / Linux"` -> `"Windows & Linux"` (`LandingEnforcers.jsx`), added Android `.apk` card and `/privacy`/`/terms` footer links (`LandingFooter.jsx`), created `Legal.jsx`, generated `frontend/public/favicon.ico` | `npm run lint && npm run build`, Chrome `/privacy`, `/terms`, `/favicon.ico` HTTP 200 |
| **GOAL-001** | MEDIUM | `FIXED AND VERIFIED` | Fixed `"rejected"` status handling across `Goals.jsx`, `Dashboard.jsx`, `AGENTS.md`, `docs/openapi.yaml`; rejected untargeted `"community"` goals in `goals/service.go`; updated `CreateGoalModal.jsx` and `goals_test.go` | `TestGoal_UntargetedCommunityRejectedAndListErrorsPropagate` + `node --test frontend/tests/*.test.js` |
| **API-002** | MEDIUM | `FIXED — RUNTIME VERIFICATION PENDING` | `deploy/nginx/onusly.conf` serves `/downloads/` directly from Nginx with `300s` proxy fallback; `cmd/server/main.go` extends `WriteTimeout` via `http.NewResponseController(w).SetWriteDeadline` on `/downloads/` | `TestDownloadsRouteIntegration`; live Nginx reload deferred |
| **WIN-003** | MEDIUM | `FIXED — RUNTIME VERIFICATION PENDING` | `desktop/native-host/dial_windows.go` makes `\\.\pipe\OnuslyAgentIPC` primary; `desktop/installer/windows/install.ps1` updates default API/Update URLs to `https://onusly.duckdns.org` | `TestWindowsDefaultPipeMatchesAgentIPC` cross-compiled cleanly (`go test -c`) |
| **PERF-001** | LOW | `PARTIALLY FIXED` | `frontend/src/App.jsx` lazy-loads workspace routes and `Legal` via `React.lazy` + `Suspense`, splitting main bundle to `298.54 KB` (`91.47 KB` gzipped). CloudFront edge CDN deferred to AWS infra cutover | `npm run build` chunk output verified |
| **INFO-001** | INFO | `PARTIALLY FIXED` | Added `frontend/public/favicon.ico` (HTTP 200); updated `deploy/setup_ec2.sh` to prune `.bak` to latest 2 and purge Go caches. Untracked local scratch/log files preserved safely | HTTP 200 `/favicon.ico` check; `bash -n deploy/setup_ec2.sh` |
| **INFO-002** | INFO | `NOT A BUG — INVESTIGATED` | Confirmed intentional asymmetric desktop hero bleed in `lock.css` does not create horizontal scroll overflow on `1440x900` or `1280x800` | Headless Chrome `scrollWidth <= innerWidth` across all 4 viewports |

---

## Automated Verification Results (Post-Final Edit)

- **Go Full Suite (`-race`):** `AWS_EC2_METADATA_DISABLED=true AWS_SHARED_CREDENTIALS_FILE=/dev/null AWS_CONFIG_FILE=/dev/null go test -race -count=1 ./...` — **PASS** (all internal, desktop, and `cmd/server` test packages green)
- **Windows Cross-Compilation & Tests:** `GOOS=windows GOARCH=amd64 go vet ./desktop/...` and `GOOS=windows GOARCH=amd64 go test -c ./desktop/native-host` — **PASS**
- **Frontend Lint & Production Build:** `cd frontend && npm run lint && npm run build` — **PASS** (0 lint errors/warnings, CRX/ZIP packages rebuilt cleanly)
- **Extension & Frontend JS Unit Suites:** `node --test extension/tests/*.test.js frontend/tests/*.test.js` — **143 / 143 PASS**
- **Android Unit Suite:** `./android/gradlew -p android testDebugUnitTest --no-daemon -Dorg.gradle.java.home=/opt/android-studio/jbr` — **PASS** (`BUILD SUCCESSFUL`)
- **Headless Chrome Viewport & Route Smoke Check (`vite preview`):** **PASS** (`1440x900`, `1280x800`, `390x844`, `320x700` all zero horizontal overflow; `/privacy`, `/terms`, `/favicon.ico` all return HTTP 200 with zero runtime exceptions)
