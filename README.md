# ONUSLY

> A goal accountability platform where completing a goal requires
> submitting proof and getting it verified by an assigned approver.

## Overview

ONUSLY separates **doing a goal** from **proving a goal**.

A user creates a goal and assigns another registered user as the
approver. When the owner believes the goal is complete, they submit
proof. The assigned approver reviews that proof and either approves or
rejects it.

The current implementation uses a **single fixed approver per goal**.
Community approval, multiple approvers, and quorum-based approval are
future capabilities.

## Current Workflow

``` text
Create Goal
    |
    v
  active
    |
    | submit proof
    v
proof_submitted
    |
    |----------------------|
    |                      |
 approve                 reject
    |                      |
    v                      v
completed                active
                           |
                           | submit new proof
                           v
                    proof_submitted
```

A rejection returns the goal to `active`, allowing the owner to resubmit
proof.

## Features Implemented

### Authentication

-   Google OAuth / Google ID token verification
-   Automatic user creation in DynamoDB
-   JWT authentication using HS256
-   JWT claims include:
    -   `sub`
    -   `email`
    -   `exp`
-   Protected API routes use: `Authorization: Bearer <JWT>`

### Goals

-   Create goals
-   List goals
-   Get a goal
-   Update title and description
-   Delete goals
-   Assign an approver by email
-   Resolve and store the approver's user ID
-   Prevent self-approval

### Proof

Supported proof types:

-   `text`
-   `link`
-   `photo`

Submitting proof moves the goal from:

``` text
active -> proof_submitted
```

### Approval

-   Only the assigned approver can decide (`403 Forbidden` if caller != approver)
-   Self-approval strictly prevented (`403 Forbidden` if owner == approver)
-   Approve proof: atomic transition `proof_submitted -> completed`
-   Reject proof: atomic transition `proof_submitted -> active`
-   Approval comments and decision timestamps
-   Duplicate decisions return HTTP `409 Conflict`

### Security Hardening & Concurrency Guarantees

-   **Owner Isolation**: Partition key `USER#<ownerID>` ensures goals, proofs, and approvals are strictly isolated.
-   **No Status Bypass**: Normal goal updates (`PATCH /goals/{id}`) cannot modify `status`, `ownerId`, or `approverId`.
-   **Accountability Freeze**: Accountability settings (`approvalType`, `approverEmail`) cannot be modified unless the goal is `active`.
-   **Atomic Proof Submission**: DynamoDB `TransactWriteItems` writes the proof record and updates goal status from `active` to `proof_submitted` conditionally (`#status = "active"`). Duplicate submissions while pending return HTTP `409 Conflict`.
-   **Atomic Approval Decision**: DynamoDB `TransactWriteItems` writes the approval record and updates goal status conditionally (`#status = "proof_submitted"`). Concurrent approval requests race at the DynamoDB transaction layer; the losing request deterministically receives HTTP `409 Conflict` ("proof has already been decided").
-   **Protected Updates**: Goal updates use `ConditionExpression: attribute_exists(PK)` preventing resurrection or phantom insertions on nonexistent goals (`404 Not Found`).

## Architecture

``` text
                    +----------------------+
                    |      React Frontend  |
                    | React 19 / Vite 8    |
                    | Tailwind CSS 4        |
                    +----------+-----------+
                               |
                         HTTP + JWT
                               |
                               v
                    +----------------------+
                    |       Go API         |
                    |      net/http        |
                    +----------+-----------+
                               |
              +----------------+----------------+
              |                |                |
              v                v                v
           Auth             Goals            Proof
              |                |                |
              +----------------+----------------+
                               |
                               v
                         Approval Service
                               |
                               v
                    +----------------------+
                    |      DynamoDB        |
                    |       Onusly         |
                    |     eu-north-1       |
                    +----------------------+
```

## Technology Stack

### Backend

-   Go
-   `net/http`
-   AWS SDK for Go v2
-   Amazon DynamoDB
-   Google ID token verification
-   JWT / HS256
-   `godotenv`
-   UUIDs

### Frontend

-   React 19
-   Vite 8
-   Tailwind CSS 4

### Infrastructure

-   AWS EC2
-   Amazon Linux 2023
-   Nginx
-   systemd
-   DynamoDB
-   AWS IAM

## Repository Structure

``` text
ONUSLY/
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   ├── auth/
│   │   ├── handler.go
│   │   ├── interfaces.go
│   │   ├── models.go
│   │   ├── repository.go
│   │   └── service.go
│   │
│   ├── goals/
│   │   ├── handler.go
│   │   ├── models.go
│   │   ├── repository.go
│   │   └── service.go
│   │
│   ├── proof/
│   │   ├── handler.go
│   │   ├── models.go
│   │   ├── repository.go
│   │   └── service.go
│   │
│   ├── approval/
│   │   ├── handler.go
│   │   ├── models.go
│   │   ├── repository.go
│   │   └── service.go
│   │
│   ├── middleware/
│   │   └── auth.go
│   │
│   ├── config/
│   │   └── config.go
│   │
│   └── shared/
│       └── dynamodb.go
│
├── frontend/
│   ├── src/
│   │   ├── App.jsx
│   │   ├── main.jsx
│   │   ├── index.css
│   │   └── assets/
│   ├── public/
│   ├── package.json
│   ├── package-lock.json
│   └── vite.config.js
│
├── docs/
├── proto/
├── README.md
└── AGENTS.md
```

## API

### Authentication

#### `POST /auth/google`

Verifies a Google ID token and returns an application JWT.

### Goals

  Method   Endpoint        Purpose
  -------- --------------- ----------------------------------
  POST     `/goals`        Create goal
  GET      `/goals`        List authenticated owner's goals
  GET      `/goals/{id}`   Get goal
  PATCH    `/goals/{id}`   Update title/description
  DELETE   `/goals/{id}`   Delete goal

### Proof

  Method   Endpoint                         Purpose
  -------- -------------------------------- --------------
  POST     `/goals/{id}/proofs`             Submit proof
  GET      `/goals/{id}/proofs`             List proofs
  GET      `/goals/{id}/proofs/{proofId}`   Get proof

### Approval

  -----------------------------------------------------------------------------------------
  Method                  Endpoint                                  Purpose
  ----------------------- ----------------------------------------- -----------------------
  POST                    `/goals/{id}/proofs/{proofId}/approval`   Approve/reject proof

  GET                     `/goals/{id}/approvals`                   List approvals

  GET                     `/goals/{id}/approvals/{approvalId}`      Get approval
  -----------------------------------------------------------------------------------------

### Health

``` text
GET /health
```

Expected response:

``` json
{
  "status": "ok"
}
```

## DynamoDB

Table:

``` text
Onusly
```

Region:

``` text
eu-north-1
```

Primary key:

``` text
PK (String)
SK (String)
```

### User

``` text
PK = USER#<google-sub>
SK = PROFILE
```

GSI1:

``` text
GSI1PK = EMAIL#<email>
GSI1SK = PROFILE
```

### Goal

``` text
PK = USER#<ownerID>
SK = GOAL#<goalID>
```

### Proof

``` text
PK = USER#<ownerID>
SK = PROOF#<goalID>#<proofID>
```

### Approval

``` text
PK = USER#<ownerID>
SK = APPROVAL#<goalID>#<approvalID>
```

## Local Development

### Backend

From the repository root:

``` bash
go run ./cmd/server
```

The API runs on:

``` text
http://localhost:8080
```

Verify:

``` bash
curl http://localhost:8080/health
```

### Frontend

``` bash
cd frontend
npm install
npm run dev
```

The Vite development server runs on:

``` text
http://localhost:5173
```

Use:

``` env
VITE_API_BASE_URL=http://localhost:8080
```

Do not expose backend secrets through `VITE_*` variables.

## Environment Variables

Backend configuration currently includes:

``` env
GOOGLE_CLIENT_ID=...
JWT_SECRET=...
```

AWS SDK configuration should use the normal AWS credential/provider
chain.

Never commit:

-   `.env`
-   AWS access keys
-   AWS secret keys
-   JWT secrets
-   Google private credentials

## Testing

Run the Go test suite:

``` bash
go test ./...
```

Format Go code:

``` bash
gofmt -w .
```

Build the frontend:

``` bash
cd frontend
npm run build
```

Lint the frontend:

``` bash
npm run lint
```

## Security Notes

The backend is the source of truth for authorization.

Do not trust frontend-provided:

-   `ownerId`
-   `userId`
-   `approverId`
-   `status`

The authenticated identity must come from the verified JWT.

The current approval implementation is intentionally single-approver:

``` text
Goal.ApproverID
```

There is no quorum or community approval logic yet.

## Implementation Status & Roadmap

### Phase 1 — IMPLEMENTED (Backend Core & Security Hardening Complete)

-   **Authentication & User Management**:
    -   Google ID token verification (`POST /auth/google`)
    -   Automatic user profile persistence in DynamoDB
    -   JWT issuance (HS256) with claim validation (`sub`, `email`, `exp`)
    -   Strict Bearer token middleware derivation (no trust in request bodies for user identity)
-   **Goal CRUD & Isolation**:
    -   Full CRUD (`POST`, `GET`, `PATCH`, `DELETE /goals`)
    -   Strict owner isolation via `PK = USER#<ownerID>`
    -   Status bypass prevention (`PATCH` cannot modify `status`, `ownerId`, or `approverId`)
    -   Accountability settings protection (`approvalType` / `approverEmail` locked once proof is submitted)
    -   Conditional updates preventing resurrection or phantom creations (`ConditionExpression: attribute_exists(PK)`)
-   **Proof Submission**:
    -   Evidence submission (`POST /goals/{id}/proofs`) with payload validation (`text`, `link`, `photo`)
    -   Atomic transition `active -> proof_submitted` via DynamoDB `TransactWriteItems` (`#status = "active"`)
    -   Rejection of duplicate submissions while proof is pending (`409 Conflict`)
-   **Single-Approver Approval Workflow**:
    -   Approver resolution by email at goal creation
    -   Self-approval strictly rejected (`403 Forbidden`)
    -   Non-assigned approver calls rejected (`403 Forbidden`)
    -   Atomic decision execution (`POST /goals/{id}/proofs/{proofId}/approval`) via DynamoDB `TransactWriteItems`
    -   `approved`: atomic transition `proof_submitted -> completed`
    -   `rejected`: atomic transition `proof_submitted -> active`
    -   Concurreny-safe duplicate resolution: simultaneous racing approval requests handled at DynamoDB transaction layer; losing requests return HTTP `409 Conflict`
-   **API Documentation**:
    -   Complete OpenAPI 3.0 specification available at `docs/openapi.yaml`
-   **Verification & Test Suites**:
    -   Automated suites for authentication, goal isolation, proof submission concurrency, and approval race conditions (`internal/*/*_test.go`)
-   **Initial Frontend**:
    -   Authenticated dashboard, goal management, inline editing, and proof submission UI

### Phase 2+ — NOT YET IMPLEMENTED (Future Product Phases)

-   **Phase 2: Real-Time & Reliability (Deferred)**:
    -   Instant unlock via API Gateway WebSockets / persistent push
    -   SQS queues for background event processing
    -   Notification worker with Firebase Cloud Messaging (FCM) push
-   **Phase 3: Platform Blocking (Deferred)**:
    -   Android `AccessibilityService` module for native app blocking
    -   Desktop browser extension (Chrome Manifest V3) for site blocking
    -   Active enforcement daemon on user devices
-   **Phase 4: Communities & Quorum (Deferred)**:
    -   Community creation, invite codes, and group memberships in backend
    -   Multi-approver voting and consensus/quorum logic
    -   Real-time community chat and activity streams

## Design Decisions

### Single approver

Each goal has exactly one assigned approver. The approver is resolved
from an email during goal creation and stored as a user ID.

### Rejection is resubmittable

A rejection does not permanently set the goal to `rejected`.

Instead:

``` text
proof_submitted -> active
```

This allows the owner to improve and resubmit proof.

### Status changes are workflow-controlled

Normal goal editing cannot change status. Status changes happen through
the proof/approval workflow.

## Production

The backend can be deployed as a Go binary under systemd, with Nginx
acting as the reverse proxy. An Nginx configuration template with WebSocket
upgrade proxy headers (`Upgrade` / `Connection`) is located at `deploy/nginx/onusly.conf`.

Production architecture:

``` text
Internet
   |
   v
Nginx :80 / :443 (HTTP + WebSocket reverse proxy)
   |
   +---> /ws       --> Go WebSocket Gateway (:8080)
   +---> /goals... --> Go REST API (:8080)
   +---> /         --> Static SPA Assets (/var/www/onusly/frontend/dist)
   |
   v
DynamoDB
```

The production environment should keep secrets outside the repository,
for example through a protected systemd environment file.

## License

Project-specific/private unless a separate license is added.
