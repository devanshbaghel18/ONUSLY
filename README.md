# ONUSLY

> A privacy-first, multi-platform accountability platform where completing a goal requires submitting verified proof to an assigned partner, backed by active on-device distraction enforcement.

[![Release Candidate](https://img.shields.io/badge/release-v1.0.0--rc1-blue.svg)](docs/RELEASE_NOTES_v1.0.0-rc1.md)
[![Go Report Card](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](go.mod)
[![React 19](https://img.shields.io/badge/react-19-61DAFB.svg)](frontend/package.json)
[![Manifest V3](https://img.shields.io/badge/chrome%20extension-MV3-brightgreen.svg)](extension/manifest.json)
[![Android API 26+](https://img.shields.io/badge/android-API%2026%2B-3DDC84.svg)](android/app/build.gradle.kts)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## 1. Overview & Core Philosophy

ONUSLY separates **doing a goal** from **verifying a goal**.

Traditional habit trackers rely on honour-system checkboxes that users easily game when discipline wanes. ONUSLY enforces absolute accountability:
1. **Pre-commitment**: Users define specific goals and link distracting apps and websites (`domains`, `desktop_apps`, `android_packages`) to be blocked across all their devices during active goal progress.
2. **Assigned Partner Verification**: The goal owner designates a specific registered partner as the sole approver. Self-approval is strictly forbidden.
3. **Multi-Platform Shielding**: Distractions are actively blocked on Windows, Linux, Android, and Chromium/Edge browsers until verified proof is submitted and approved.
4. **Zero-Surveillance Privacy**: Enforcement operates through local package overlay suppression, browser navigation interception, and loopback DNS sinkholes. Screen contents, keystrokes, and browsing histories are **never** recorded or transmitted.

> [!IMPORTANT]
> **Single-Approver Architecture**: In the core verification model, each goal has exactly one assigned approver (`ApproverID`). Rejection returns the goal to `active` (allowing proof resubmission). Group discovery and community chat facilitate camaraderie, while goal verification remains strictly partner-to-partner.

---

## 2. Goal Lifecycle & Enforcement States

```text
                  +---------------------------+
                  |        Create Goal        |
                  | (Assign Approver & Targets)|
                  +-------------+-------------+
                                |
                                v
                   +-------------------------+
                   |         active          | <=============================+
                   |  [Shields Enforcing]    |                               |
                   +------------+------------+                               |
                                |                                            |
                         Submit | Proof                               Reject | Decision
                                v                                            |
                   +-------------------------+                               |
                   |     proof_submitted     |                               |
                   |  [Shields Enforcing]    | ------------------------------+
                   +------------+------------+
                                |
                        Approve | Decision
                                v
                   +-------------------------+
                   |        completed        |
                   |     [Shields Lifted]    |
                   +-------------------------+
```

- **Active**: The goal is pending completion. Desktop agents, mobile clients, and browser extensions actively block configured distraction targets.
- **Proof Submitted**: Evidence (documents, images, links, or text) has been submitted and is locked awaiting approver review. Shields remain **actively enforced** to prevent premature distraction.
- **Rejected**: The approver deemed the proof insufficient. The goal atomically transitions back to `active`, and shields stay active until new proof is submitted.
- **Completed**: The approver validated the proof. The goal atomically transitions to `completed`, and shields for this goal are immediately unlocked via real-time WebSocket push.

---

## 3. Platform Capabilities

### 3.1 Authentication & Session Security
- **Google OAuth 2.0**: Backend token verification via `POST /auth/google` with automatic DynamoDB user provisioning.
- **JWT (HS256)**: Authenticated identity (`sub`, `email`, `exp`) securely passed via `Authorization: Bearer <JWT>`. Backend middleware is the sole source of truth; client identity fields in payloads are ignored.
- **Session Revocation & Versioning**: Global session termination (`/auth/revoke-all`, `/auth/logout-all`) increments `TokenVersion`, invalidating tokens and immediately severing active WebSockets.
- **Granular Rate Limiting**: Sliding-window rate limiters protecting authentication (30 req/min), device pairing (20 req/min), and API queries (180 req/min).

### 3.2 Goals & Anti-Bypass Protections
- **Strict Owner Isolation**: Single-table partitioning (`USER#<ownerID>`) guarantees users can only modify their own goals.
- **Status Bypass Prevention**: Normal updates (`PATCH /goals/{id}`) can only edit title and description. Modifying `status`, `ownerId`, or `approverId` is strictly blocked at the service layer.
- **Accountability Lock**: Target distraction lists and approver designations are locked once a goal transitions out of `active`.
- **Phantom Check Protection**: Conditional DynamoDB updates (`attribute_exists(PK)`) prevent resurrection of deleted goals.

### 3.3 Evidence & Proof Engine
- **Rich Media Uploads**: Multi-file document and photo uploads (`POST /uploads`) supporting PDFs, images (PNG, JPG, WebP), and documents up to 32MB per file (50MB overall payload limit) with path-traversal safeguards and safe unique file generation.
- **Supported Proof Types**: Structured uploads (`photo`, `link`, `text`) stored with submission history timestamps.
- **Atomic Submission**: DynamoDB `TransactWriteItems` writes the proof record and transitions goal status (`active -> proof_submitted`) under condition `#status = "active"`. Duplicate submissions return `409 Conflict`.

### 3.4 Single-Approver Decision Engine & Approver Inbox
- **Self-Approval Guard**: Resolves approvers by email or handle; prevents assigning oneself (`403 Forbidden`).
- **Partner Verification**: Only the designated approver can approve or reject (`403 Forbidden`).
- **Atomic Decisions**: DynamoDB transactions write the approval record and update the goal status under condition `#status = "proof_submitted"`. Simultaneous racing decisions deterministically yield `409 Conflict`.
- **Approver Inbox (GSI2)**: O(1) indexed querying via `GSI2PK = APPROVER#<approverID|email|handle>` and `GSI2SK = STATUS#<status>#GOAL#<goalID>`, powering partner review dashboards.
- **Submissions Timeline**: Multi-attempt audit trail tracking all proof submissions and review notes for every goal cycle (`GET /goals/{id}/submissions`).

### 3.5 Multi-Platform Anti-Distraction Enforcement
- **Typed Target Contract**:
  - `domain`: Exact and wildcard domain matches (e.g. `reddit.com`, `instagram.com`).
  - `desktop_app`: Desktop executable names (e.g. `discord.exe`, `steam.exe`, `slack`).
  - `android_package`: Mobile package names (e.g. `com.discord`, `com.reddit.frontpage`).
  - Special granular targets: YouTube Shorts path-specific filtering (`/shorts` blocked while educational `/watch` remains open).
- **Windows Desktop Agent**: Native system service executing under `NT AUTHORITY\LocalService` with loopback DNS sinkhole, DPAPI credential storage, named-pipe IPC (`\\.\pipe\OnuslyAgentIPC`), and 1-click standalone installer with auto-pairing.
- **Linux Desktop Agent**: Systemd daemon with kernel keyring credential storage and UNIX domain socket IPC (`/run/onusly/onusly.sock`).
- **Android Native Client (`com.onusly.app`)**: Android `AccessibilityService` with `canRetrieveWindowContent="false"` for strict zero-surveillance privacy. Detects foreground packages, presents full-screen `LockActivity` blocker overlays, and maintains a local fail-safe Last-Known-Good (LKG) cache with ETag 304 synchronization.
- **Chromium / Edge Extension (Manifest V3)**: Page blocker, YouTube Shorts feed suppression, dashboard bridge, and native messaging IPC with anti-tamper disconnect lockdown.
- **Device Pairing & Remote Revocation**: Secure 6-digit PIN and QR code pairing (`/enforcement/pair/*`), periodic heartbeats (`/enforcement/heartbeat`), and instant centralized device revocation (`POST /enforcement/devices/{id}/revoke`).

### 3.6 Real-Time WebSocket Gateway & Event Pipeline
- **Persistent WebSocket Hub (`/ws`)**: Sub-second push delivery for goal status changes, instant unlock on approval, and chat messages.
- **Queue Abstraction**: Decoupled event publishing supporting AWS SQS in cloud production and an in-memory queue adapter for local development.
- **Background Notification Worker**: Consumes proof and approval event queues with Firebase Cloud Messaging (FCM) push support and in-app notifications.

### 3.7 Social Accountability & Community Chat
- **User Handles & Search**: Unique `@handle` registry with search (`/users/search`) and profile management (`/users/me`).
- **Mutual Friend Graph**: Friend requests, acceptance workflows, and friend listings (`/friends/*`).
- **Communities Discovery**: Curated habit-building communities (`/communities/trending`, `/communities/joined`) with instant join/leave actions.
- **Real-Time 1-on-1 & Community Chat**: Ephemeral and persistent chat history (`/chat/messages`) over WebSockets and REST.

### 3.8 Production SRE Observability & Hardening
- **Prometheus Metrics (`/metrics`)**: Production runtime telemetry, HTTP request latencies, and device status metrics.
- **Loopback Bind Enforcement**: Server startup guards (`validateAndResolveBindAddress`) strictly reject binding to `0.0.0.0` or non-loopback interfaces in production without explicit `ALLOW_INSECURE_BIND=true`.
- **Idempotency Layer**: Memory-backed idempotency middleware (`Idempotency-Key` header with 24-hour cache) preventing duplicate mutations from client retries.
- **Enterprise Extension Update Server**: Secondary local listener on `127.0.0.1:8765` serving signed CRX extension binaries and update manifests (`updates.xml`).
- **Cryptographic Anti-Rollback**: Ed25519-signed update manifests and security version validation preventing downgrade attacks.

---

## 4. System Architecture

```text
                           +----------------------------------------+
                           |           User Touchpoints             |
                           |  React 19 Dashboard / Android Client   |
                           |  Browser Extension / Windows & Linux   |
                           +-------------------+--------------------+
                                               |
                                     HTTPS + WSS / JWT
                                               |
                                               v
+-----------------------------------------------------------------------------------+
|                              Nginx Reverse Proxy                                  |
|                 Port 80/443 -> SSL Termination & WebSocket Upgrade                 |
+--------------------------------------+--------------------------------------------+
                                       |
                         127.0.0.1:8080 (Loopback Only)
                                       |
                                       v
+-----------------------------------------------------------------------------------+
|                                Go REST & WS Core                                  |
|                                                                                   |
|  [Security Headers] -> [MaxBytesLimiter] -> [CORS] -> [RateLimiter] -> [Auth]     |
|                                                                                   |
|  +--------------------+  +--------------------+  +--------------------+           |
|  |    Auth Service    |  |    Goals Service   |  |   Approval Service |           |
|  +--------------------+  +--------------------+  +--------------------+           |
|  | Enforcement Service|  |  Real-Time WS Hub  |  |    Chat Service    |           |
|  +--------------------+  +--------------------+  +--------------------+           |
|  |   Friends Service  |  | Notifications Serv |  |   Idempotency Store|           |
|  +--------------------+  +--------------------+  +--------------------+           |
+-----------------------------------+-----------------------------------------------+
                                    |
            +-----------------------+-----------------------+
            |                                               |
            v                                               v
+-----------------------+                       +-----------------------+
|    Amazon DynamoDB    |                       |      Event Queue      |
|    Single-Table       |                       | AWS SQS / In-Memory   |
| (PK, SK, GSI1, GSI2)  |                       +-----------+-----------+
+-----------------------+                                   |
                                                            v
                                                +-----------------------+
                                                |  Notification Worker  |
                                                |   In-App / FCM Push   |
                                                +-----------------------+
```

---

## 5. Technology Stack

| Layer | Technologies |
|---|---|
| **Backend Core** | Go 1.26+, `net/http`, AWS SDK for Go v2, Gorilla WebSocket, `godotenv`, `google-api-go-client` |
| **Frontend Web** | React 19, Vite 8, Tailwind CSS 4, Lucide Icons |
| **Database** | Amazon DynamoDB (Single-table design with PAY_PER_REQUEST, GSI1, GSI2) |
| **Asynchronous Events** | AWS SQS (Production) / In-Memory Queue (Development), Notification Worker |
| **Desktop Enforcement** | Go, Windows Service Control Manager (SCM), DPAPI, Named Pipes IPC, Linux systemd, UNIX Sockets |
| **Android Enforcement** | Kotlin, Android SDK (API 26–35), `AccessibilityService`, MockWebServer |
| **Browser Extensions** | Chromium / Chrome Manifest V3, Mozilla Firefox (WebExtensions), Native Messaging Host |
| **Infrastructure** | AWS EC2 (Amazon Linux 2023), Nginx, systemd, AWS IAM Least Privilege |

---

## 6. Repository Structure

```text
ONUSLY/
├── cmd/
│   ├── server/                   # Go REST API & WebSocket server entry point
│   └── manifest_generator/       # Ed25519 cryptographic update manifest generator
│
├── internal/
│   ├── approval/                 # Single-approver service, repository, and tests
│   ├── auth/                     # Google OAuth, JWT issuance, session management
│   ├── chat/                     # 1-on-1 and community chat engine & message store
│   ├── config/                   # Configuration loader and environment validator
│   ├── enforcement/              # Device pairing, blocklist distribution, heartbeats
│   ├── events/                   # Queue-backed event publishers & composite adapters
│   ├── friends/                  # Social relationships, friend requests, communities
│   ├── goals/                    # Goal CRUD, target validation, GSI2 inbox querying
│   ├── metrics/                  # Prometheus metrics registry and HTTP exporter
│   ├── middleware/               # Auth, rate limiting, idempotency, security headers
│   ├── notifications/            # Background notification worker & repository
│   ├── proof/                    # Evidence submission, validation, and multi-file uploads
│   ├── queue/                    # Queue abstraction (AWS SQS & In-Memory adapters)
│   ├── realtime/                 # WebSocket Hub, client lifecycles, and pub/sub
│   └── shared/                   # DynamoDB and SQS AWS SDK client singletons
│
├── desktop/
│   ├── agent/                    # Windows/Linux desktop enforcement daemon
│   ├── core/                     # Common models, updater, and platform adapters
│   ├── installer/                # Standalone 1-click Windows & Linux installers
│   ├── native-host/              # Browser native messaging host executable
│   └── policy/                   # Windows Registry & Linux policy managers
│
├── android/
│   ├── app/                      # Native Android client (Kotlin, API 26+)
│   │   ├── src/main/java/        # AccessibilityService, BlockerEngine, LockActivity
│   │   └── src/test/java/        # 182 automated unit tests across 18 test suites
│   └── build.gradle.kts          # Gradle build script
│
├── extension/
│   ├── src/                      # Background service worker, domainMatcher, blockerEngine
│   ├── tests/                    # 104 automated tests across 21 test suites
│   ├── manifest.json             # Manifest V3 configuration
│   └── popup.html / blocked.html # Browser extension UI & blocking interstitial
│
├── frontend/
│   ├── src/
│   │   ├── components/           # UI components (Coverage, Modals, Approvals, Timeline)
│   │   ├── pages/                # Dashboard, Goals, GoalDetail, Approvals, Communities
│   │   ├── lib/                  # API client, WebSocketManager, target normalizers
│   │   └── App.jsx               # React Router navigation and root layout
│   └── package.json              # React 19 / Vite 8 dependencies
│
├── deploy/
│   ├── aws/                      # IAM policies and EC2 instance profiles
│   ├── dynamodb/                 # Table definitions and migration scripts
│   ├── nginx/                    # Production reverse proxy template with WS upgrade
│   └── systemd/                  # onusly.service systemd unit configuration
│
├── docs/
│   ├── openapi.yaml              # Complete OpenAPI 3.0 specification
│   ├── RELEASE_NOTES_v1.0.0-rc1.md # Release Candidate notes & platform matrices
│   └── runbooks/                 # Incident response, backup/restore, support runbooks
│
├── scripts/                      # Build, packaging, pairing, and extension automation
├── AGENTS.md                     # Architecture rules and security constraints
└── README.md                     # Project documentation
```

---

## 7. API Reference

All protected endpoints require `Authorization: Bearer <JWT>`. Mutations accept an optional `Idempotency-Key: <UUID>` header.

### 7.1 Authentication & Profile
| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/auth/google` | Exchange Google ID token for application JWT |
| `GET` | `/auth/google/callback` | OAuth redirect callback handler |
| `POST` | `/auth/logout` | Terminate current user session |
| `POST` | `/auth/revoke-all` | Invalidate all active sessions & disconnect WebSockets |
| `GET` | `/users/me` | Fetch authenticated user profile & handles |
| `GET` | `/users/search` | Search users by email or `@handle` |
| `PATCH` | `/users/handle` | Claim or update personal unique `@handle` |

### 7.2 Goals & Accountability
| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/goals` | Create a goal with assigned approver and distraction targets |
| `GET` | `/goals` | List all goals owned by the caller |
| `GET` | `/goals/{id}` | Retrieve goal details by ID |
| `PATCH` | `/goals/{id}` | Update title or description (cannot bypass status) |
| `DELETE` | `/goals/{id}` | Delete goal and associated target bindings |
| `GET` | `/goals/{id}/enforcement-status` | Get active device enforcement state for a goal |

### 7.3 Evidence & Proof Uploads
| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/goals/{id}/proofs` | Submit evidence (`text`, `link`, `photo`) and lock goal |
| `GET` | `/goals/{id}/proofs` | List all submitted proofs for a goal |
| `GET` | `/goals/{id}/proofs/{proofId}` | Get proof item by ID |
| `POST` | `/uploads` | Upload multi-file proof evidence (PDF, images up to 32MB) |
| `GET` | `/uploads/proofs/{filename}` | Retrieve uploaded evidence file |

### 7.4 Verification & Approver Inbox
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/approvals/inbox` | List goals awaiting caller's verification (GSI2 query) |
| `POST` | `/goals/{id}/proofs/{proofId}/approval` | Approve (`completed`) or reject (`active`) proof |
| `GET` | `/goals/{id}/approvals` | List approval records for a goal |
| `GET` | `/goals/{id}/approvals/{approvalId}` | Get approval decision record |
| `GET` | `/goals/{id}/submissions` | Retrieve full multi-attempt submission history |

### 7.5 Multi-Platform Enforcement & Device Management
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/enforcement/blocklist` | Fetch active blocklist (ETag / If-None-Match cached) |
| `POST` | `/enforcement/heartbeat` | Report device health & active blocking telemetry |
| `GET` | `/enforcement/devices` | List all registered devices for caller |
| `POST` | `/enforcement/devices/{id}/revoke` | Revoke a device, immediately clearing local caches |
| `POST` | `/enforcement/pair/request` | Initiate device pairing (generates 6-digit PIN) |
| `POST` | `/enforcement/pair/exchange` | Exchange PIN for authentication token |
| `POST` | `/enforcement/pair/confirm` | Confirm pairing session from authenticated dashboard |
| `GET` | `/enforcement/desktop/status` | Query desktop agent daemon state |
| `POST` | `/enforcement/desktop/dns` | Toggle local loopback DNS sinkhole protection |
| `POST` | `/enforcement/desktop/sync` | Force immediate blocklist synchronization |
| `POST` | `/enforcement/desktop/pair` | Automated 1-click desktop pairing |

### 7.6 Real-Time WebSocket & Chat
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/ws` | Authenticated WebSocket stream for real-time push |
| `GET` | `/chat/messages` | Retrieve conversation history with pagination |
| `POST` | `/chat/messages` | Send 1-on-1 or community chat message |
| `DELETE` | `/chat/messages` | Delete user message by ID |

### 7.7 Friends & Communities
| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/friends/request` | Send friend request by handle or email |
| `GET` | `/friends/requests` | List incoming and outgoing friend requests |
| `POST` | `/friends/request/respond` | Accept or decline a friend request |
| `GET` | `/friends` | List mutual friends |
| `DELETE` | `/friends/` | Remove a friend connection |
| `GET` | `/communities/trending` | Discover curated accountability communities |
| `GET` | `/communities/joined` | List communities caller has joined |
| `POST` | `/communities/join` | Join a community |
| `POST` | `/communities/leave` | Leave a community |

### 7.8 System & Observability
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Service health probe (`{"status":"ok"}`) |
| `GET` | `/metrics` | Prometheus metrics scrape endpoint |
| `GET` | `/downloads/{file}` | Download desktop binaries, CRX, and updates.xml |

---

## 8. DynamoDB Single-Table Schema

- **Table Name**: `Onusly`
- **Region**: `eu-north-1` (Default) / Configurable via `AWS_REGION`
- **Billing Mode**: `PAY_PER_REQUEST` (On-Demand)

### 8.1 Primary Table Schema
| Entity | Partition Key (`PK`) | Sort Key (`SK`) | Notes |
|---|---|---|---|
| **User Profile** | `USER#<google-sub>` | `PROFILE` | Stores email, handle, token version |
| **Goal** | `USER#<ownerID>` | `GOAL#<goalID>` | Isolated by owner; GSI1/GSI2 indexed |
| **Proof** | `USER#<ownerID>` | `PROOF#<goalID>#<proofID>` | Evidence payloads & upload metadata |
| **Approval** | `USER#<ownerID>` | `APPROVAL#<goalID>#<approvalID>` | Decision, comment, timestamp |
| **Device** | `USER#<userID>` | `DEVICE#<deviceID>` | Hostname, OS, client type, heartbeat |
| **Pairing Session**| `PAIR#<code>` | `SESSION` | Ephemeral 6-digit pairing session |
| **Friend** | `USER#<userID>` | `FRIEND#<friendID>` | Bidirectional mutual friendship record |
| **Friend Request** | `USER#<recipientID>` | `FRIEND_REQ_IN#<reqID>` | Incoming friend request |
| **Friend Request** | `USER#<senderID>` | `FRIEND_REQ_OUT#<reqID>` | Outgoing friend request |
| **Community Member**| `USER#<userID>` | `COMMUNITY#<communityID>` | User community membership |
| **1-on-1 Chat** | `USER#<userKey>` | `CHAT#<peerKey>#<timestamp>#<msgID>` | Isolated sender/recipient copies |
| **Community Chat** | `COMMUNITY#<communityID>` | `MSG#<timestamp>#<msgID>` | Public community channel feed |
| **Notification** | `USER#<recipientID>` | `NOTIF#<notifID>` | In-app alerts and delivery state |

### 8.2 Global Secondary Indexes (GSIs)
- **GSI1 (Identifier Resolution)**:
  - User by Email: `GSI1PK = EMAIL#<email>`, `GSI1SK = PROFILE`
  - Goal by ID: `GSI1PK = GOAL#<goalID>`, `GSI1SK = METADATA`
- **GSI2 (Approver Inbox & Status Queries)**:
  - Approver Inbox: `GSI2PK = APPROVER#<approverID|email|handle>`, `GSI2SK = STATUS#<status>#GOAL#<goalID>`
  - Enables O(1) retrieval of all goals awaiting verification for any partner.

---

## 9. Security & Privacy Model

1. **Zero-Surveillance Architecture**:
   - The Android client configures `canRetrieveWindowContent="false"` in `accessibility_service_config.xml`. It inspects only `TYPE_WINDOW_STATE_CHANGED` package events and **never** reads on-screen text, form fields, or keystrokes.
   - Desktop DNS sinkholes filter targeted hostnames locally on loopback. Browsing history is **never** logged, collected, or uploaded.
2. **Backend as Source of Truth**:
   - Authentication context (`GetUserID`, `GetUserEmail`) extracted strictly from validated JWT claims.
   - Client-provided `ownerId` or `userId` in request bodies are ignored.
3. **Atomic Concurrency & Anti-Bypass**:
   - Status updates are guarded by DynamoDB condition expressions (`#status = "active"` for proof submission; `#status = "proof_submitted"` for approval decisions). Concurrent duplicate decisions race safely at the database layer and return `409 Conflict`.
   - Normal `PATCH /goals/{id}` cannot modify `status`, `ownerId`, or `approverId`.
4. **Credential Isolation**:
   - Desktop credentials are encrypted using Windows DPAPI or Linux Kernel Keyring.
   - Revoking a device terminates its session immediately, resets local caches, and transitions the client agent to a safe unblocked state.
5. **Anti-Tamper & Disconnect Lockdown**:
   - The browser extension strictly forbids disconnecting or logging out while goals are in `active`, `proof_submitted`, or `rejected` states.

---

## 10. Local Development

### Prerequisites
- **Go**: 1.26+ installed
- **Node.js**: 20+ and `npm` installed
- **Android SDK / JDK**: Java 17/21 and Android SDK Platform 35 (if building Android)

### 10.1 Backend Setup
From the repository root:
```bash
# Copy and configure environment variables
cp .env.example .env

# Run the Go API server (listens on 127.0.0.1:8080)
go run ./cmd/server
```

Verify backend health:
```bash
curl http://127.0.0.1:8080/health
```

### 10.2 Frontend Setup
```bash
cd frontend
npm install
npm run dev
```
The Vite development server runs at `http://localhost:5173`. Ensure `VITE_API_BASE_URL=http://localhost:8080` is set in `frontend/.env`.

### 10.3 Browser Extension Setup
1. Open Google Chrome or Microsoft Edge and navigate to `chrome://extensions`.
2. Enable **Developer mode** (top right).
3. Click **Load unpacked** and select the `extension/` directory.
4. Open the extension popup, log in, and pair with your local development environment.

### 10.4 Desktop Agent Setup
To test the desktop agent locally on Windows or Linux:
```bash
# Build desktop binaries
go build -o onusly-agent ./desktop/agent/cmd
go build -o onusly-native-host ./desktop/native-host

# Pair the desktop agent with your local backend
powershell ./scripts/pair_desktop_agent.ps1
```

### 10.5 Android Client Setup
```bash
cd android
./gradlew assembleDebug
```
Output APK: `android/app/build/outputs/apk/debug/app-debug.apk`.  
Install onto an emulator or connected device via `adb install -r <path-to-apk>`.

---

## 11. Testing & Verification

Run the comprehensive test suites across all layers of the monorepo:

### Backend Test Suite
```bash
go test -v ./...
```
Runs unit and integration tests covering auth isolation, goal validation, concurrency races, GSI2 queries, proof submissions, and notification workers.

### Frontend Test Suite & Build Check
```bash
cd frontend
npm test
npm run build
```
Executes target control plane suites, accountability workflow tests, multi-device telemetry tests, and WebSocket authentication lifecycle tests.

### Browser Extension Test Suite
```bash
cd extension
npm test
```
Executes 104 unit tests covering domain normalization, YouTube Shorts path-specific filtering, ETag cache synchronization, and disconnect lockdowns.

### Android Test Suite
```bash
cd android
./gradlew testDebugUnitTest
```
Executes 182 unit tests across 18 suites verifying typed package isolation, ETag durability, revocation handling, and blocker engine accuracy.

---

## 12. Production Deployment

The backend deploys as a native Go binary managed by systemd, with Nginx handling SSL termination and WebSocket reverse proxying.

### 12.1 Systemd Configuration
Service unit located at `deploy/systemd/onusly.service`:
```ini
[Unit]
Description=ONUSLY Core REST & Real-time Server
After=network.target

[Service]
Type=simple
User=ec2-user
WorkingDirectory=/var/www/onusly
ExecStart=/var/www/onusly/onusly-server
Restart=always
RestartSec=5s
EnvironmentFile=/etc/onusly/onusly.env

[Install]
WantedBy=multi-user.target
```

### 12.2 Nginx Reverse Proxy
An Nginx configuration template with WebSocket upgrade headers (`Upgrade` and `Connection`) is located at `deploy/nginx/onusly.conf`.

Production routing:
- `/ws` & `/ws/` -> `http://127.0.0.1:8080` (Go WebSocket Hub)
- `/goals`, `/auth`, `/enforcement`, etc. -> `http://127.0.0.1:8080` (Go REST API)
- `/downloads/` -> Static extension builds & update manifests
- `/` -> SPA static assets (`frontend/dist`)

### 12.3 Automated Deployment Scripts
- `deploy/setup_ec2.sh`: Provisions EC2 instance, sets up Nginx, systemd, and builds production binaries.
- `deploy/verify_production.sh`: Automated post-deployment smoke test verifying health, WebSocket endpoints, and security headers.

---

## 13. Implementation Status & Roadmap

### Milestone D0–D14 (v1.0.0-rc1) — IMPLEMENTED
- [x] **Authentication & Session Security**: Google OAuth, JWT issuance, session versioning, token revocation.
- [x] **Goal Accountability Core**: Goal CRUD, owner isolation, status bypass protection.
- [x] **Evidence & Media Submissions**: Multi-file uploads (PDFs, images up to 32MB) with path-traversal security.
- [x] **Single-Approver Workflow**: Approver resolution, self-approval prevention, atomic DynamoDB decisions (`TransactWriteItems`), GSI2 Approver Inbox.
- [x] **Multi-Platform Distraction Blocking**:
  - Windows SCM Service Agent with loopback DNS sinkhole and 1-click installer.
  - Linux systemd daemon with kernel keyring credential storage.
  - Android `AccessibilityService` client (`com.onusly.app`) with zero window inspection privacy.
  - Chromium / Edge Manifest V3 extension with YouTube Shorts path filtering.
- [x] **Real-Time Push & Events**: WebSocket Gateway (`/ws`), SQS & in-memory event queues, asynchronous notification worker.
- [x] **Social & Camaraderie**: User handles, friend requests, community discovery, 1-on-1 and community chat.
- [x] **SRE Observability**: Prometheus telemetry (`/metrics`), sliding-window rate limiters, idempotency middleware.

### Future Roadmap
- [ ] **Community Quorum Verification**: Optional community-wide voting or consensus approval modes for shared group challenges.
- [ ] **Accountability Stakes & Pledges**: Financial or habit staking with charity forfeiture upon goal failure.
- [ ] **macOS Desktop Agent**: Native daemon supporting macOS Network Extensions / pfctl.
- [ ] **iOS Enforcement Client**: iOS Screen Time API / FamilyControls integration.

---

## 14. License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
