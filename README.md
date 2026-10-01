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

-   Only the assigned approver can decide
-   Approve proof: `proof_submitted -> completed`
-   Reject proof: `proof_submitted -> active`
-   Approval comments
-   Decision timestamps
-   Duplicate decisions return HTTP `409 Conflict`

### Security Hardening

The normal goal update endpoint cannot modify:

-   `status`
-   `ownerId`
-   `approverId`

The owner therefore cannot bypass approval by sending:

``` json
{
  "status": "completed"
}
```

A duplicate approval is also rejected.

These protections have been manually tested.

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

## Roadmap

### Current

-   Google authentication
-   JWT authentication
-   DynamoDB persistence
-   Goal CRUD
-   Proof submission
-   Single-approver workflow
-   Approval/rejection
-   Status-bypass protection
-   Duplicate-decision protection
-   Initial React frontend

### Next

-   Complete frontend authentication
-   API client
-   Goal dashboard
-   Goal creation/editing UI
-   Proof submission UI
-   Approver dashboard
-   Approval/rejection UI
-   Better error/loading states
-   Production frontend integration

### Future

-   Community-based approval
-   Multiple approvers
-   Quorum approval
-   More proof types
-   Notifications
-   Better audit history
-   Atomic approval transactions
-   Stronger concurrency handling

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
acting as the reverse proxy.

Production architecture:

``` text
Internet
   |
   v
Nginx :80
   |
   v
Go API :8080
   |
   v
DynamoDB
```

The production environment should keep secrets outside the repository,
for example through a protected systemd environment file.

## License

Project-specific/private unless a separate license is added.
