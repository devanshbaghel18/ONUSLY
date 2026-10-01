# AGENTS.md

## Project

ONUSLY is an accountability and goal-verification platform.

The application consists of:

-   Go backend
-   React frontend
-   Google OAuth
-   JWT authentication
-   DynamoDB
-   Single-approver proof verification

Read this file before modifying the project.

------------------------------------------------------------------------

## Core Rule

**Inspect existing code before implementing anything.**

Do not rebuild an existing feature.

Do not replace working backend behavior just to make the frontend
easier.

Do not change architecture without understanding the current
implementation.

------------------------------------------------------------------------

## Architecture

``` text
React frontend
      |
      | HTTP + JWT
      v
Go REST API
      |
      v
DynamoDB
```

The browser must never communicate directly with DynamoDB.

AWS credentials belong to backend/development environments, never
frontend bundles.

------------------------------------------------------------------------

## Backend Stack

-   Go
-   `net/http`
-   AWS SDK for Go v2
-   DynamoDB
-   Google ID token verification
-   JWT HS256
-   UUID
-   godotenv

Backend entry point:

``` text
cmd/server/main.go
```

Backend modules:

``` text
internal/auth/
internal/goals/
internal/proof/
internal/approval/
internal/middleware/
internal/config/
internal/shared/
```

------------------------------------------------------------------------

## Frontend Stack

-   React 19
-   Vite 8
-   Tailwind CSS 4

Frontend directory:

``` text
frontend/
```

Development server:

``` text
http://localhost:5173
```

Backend:

``` text
http://localhost:8080
```

Use:

``` env
VITE_API_BASE_URL=http://localhost:8080
```

Do not hardcode the API URL throughout the application.

------------------------------------------------------------------------

## Authentication

Google authentication is already implemented in the backend.

Flow:

``` text
Google
  |
  v
Google ID token
  |
  v
POST /auth/google
  |
  v
Verify Google token
  |
  v
Find/create DynamoDB user
  |
  v
Generate JWT
```

JWT uses HS256.

JWT claims:

``` text
sub
email
exp
```

Protected requests use:

``` http
Authorization: Bearer <JWT>
```

### Important

Never expose `JWT_SECRET` to the frontend.

Never put AWS credentials in:

``` text
VITE_*
```

Never commit credentials.

------------------------------------------------------------------------

## User Identity

The JWT is the source of truth for authenticated identity.

Do not trust frontend values for authorization.

In particular, never use a request body value such as:

``` json
{
  "ownerId": "..."
}
```

as proof that the caller owns the resource.

The backend must derive authenticated identity from middleware/JWT and
validate resource ownership.

------------------------------------------------------------------------

## Goal Workflow

Current statuses:

``` text
active
proof_submitted
completed
```

Workflow:

``` text
active
  |
  | submit proof
  v
proof_submitted
  |
  +---- approve ----> completed
  |
  +---- reject -----> active
```

A rejected goal can receive new proof.

Do not introduce a `rejected` goal status without an explicit product
decision.

------------------------------------------------------------------------

## Goal Security

`PATCH /goals/{id}` can update:

``` text
title
description
```

It must NOT allow:

``` text
status
ownerId
approverId
```

Do not reintroduce status updates into normal goal editing.

The owner must not be able to do:

``` json
{
  "status": "completed"
}
```

to bypass approval.

This was explicitly tested.

------------------------------------------------------------------------

## Approval Model

The current implementation is:

**single approver per goal.**

A goal stores:

``` text
ApproverID
```

At goal creation:

``` text
approverEmail
     |
     v
Find user by email
     |
     v
ApproverID stored on goal
```

Self-approval is rejected.

There is currently no:

``` text
approval_mode
community_id
quorum
multiple approvers
```

Do not implement community/quorum behavior unless the task explicitly
asks for it.

------------------------------------------------------------------------

## Approval Security

Approval must verify:

1.  Goal exists.
2.  Authenticated user is the assigned approver.
3.  Goal is currently `proof_submitted`.
4.  Proof exists.
5.  Proof belongs to the goal.
6.  Proof belongs to the owner.

Unauthorized approver:

``` text
HTTP 403
```

Invalid decision:

``` text
HTTP 400
```

Invalid proof:

``` text
HTTP 400
```

Already decided:

``` text
HTTP 409
```

Do not weaken these checks.

------------------------------------------------------------------------

## Duplicate Decisions

The system currently prevents duplicate decisions by requiring the goal
to be:

``` text
proof_submitted
```

before a decision.

Once approved:

``` text
completed
```

Once rejected:

``` text
active
```

A second decision therefore returns:

``` text
proof has already been decided
```

with HTTP 409.

This behavior has been manually tested.

------------------------------------------------------------------------

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
PK
SK
```

### User

``` text
PK = USER#<google-sub>
SK = PROFILE
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

GSI1 resolves users by email:

``` text
GSI1PK = EMAIL#<email>
GSI1SK = PROFILE
```

------------------------------------------------------------------------

## API

### Auth

``` text
POST /auth/google
```

### Goals

``` text
POST   /goals
GET    /goals
GET    /goals/{id}
PATCH  /goals/{id}
DELETE /goals/{id}
```

### Proof

``` text
POST /goals/{id}/proofs
GET  /goals/{id}/proofs
GET  /goals/{id}/proofs/{proofId}
```

### Approval

``` text
POST /goals/{id}/proofs/{proofId}/approval
GET  /goals/{id}/approvals
GET  /goals/{id}/approvals/{approvalId}
```

### Health

``` text
GET /health
```

------------------------------------------------------------------------

## Proof Types

Supported:

``` text
text
link
photo
```

Validation rules are implemented in the proof service.

Do not bypass proof validation in handlers.

------------------------------------------------------------------------

## Code Organization

Follow the existing separation:

``` text
handler
   ↓
service
   ↓
repository
   ↓
DynamoDB
```

### Handler

Responsible for:

-   HTTP parsing
-   authentication context access
-   request validation
-   HTTP response/status

### Service

Responsible for:

-   business rules
-   authorization-related business checks
-   workflow transitions
-   validation

### Repository

Responsible for:

-   DynamoDB operations
-   key construction
-   marshalling/unmarshalling

Do not put business logic into repositories.

Do not put DynamoDB code directly into handlers.

------------------------------------------------------------------------

## Frontend Rules

The frontend should:

-   use the existing API
-   use `VITE_API_BASE_URL`
-   attach JWT to protected requests
-   display server errors
-   show loading states
-   reflect real goal status
-   prevent UI actions that the backend will reject

However:

**Frontend restrictions are not security controls.**

The backend must enforce authorization even if the frontend hides a
button.

------------------------------------------------------------------------

## Environment Variables

Backend secrets may include:

``` env
GOOGLE_CLIENT_ID=...
JWT_SECRET=...
```

Never commit actual values.

Never expose:

``` text
JWT_SECRET
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
```

to the frontend.

------------------------------------------------------------------------

## AWS

Development AWS access should use an IAM identity with least privilege.

Do not use the AWS root account for application development.

Do not share another developer's AWS access keys.

Do not commit credentials.

For ONUSLY, DynamoDB access is through the backend.

------------------------------------------------------------------------

## Git Workflow

Work on feature branches.

Example:

``` bash
git switch -c feature/my-change
```

Before changing code:

``` bash
git status
git pull --rebase
```

After changes:

``` bash
go test ./...
git diff
git status
```

Frontend:

``` bash
cd frontend
npm run lint
npm run build
```

Commit with meaningful messages:

``` text
feat: add ...
fix: prevent ...
refactor: ...
docs: ...
```

Do not force-push shared branches unless explicitly agreed.

Do not commit:

``` text
.env
AWS credentials
private keys
JWT secrets
```

------------------------------------------------------------------------

## Validation Before PR

For backend changes:

``` bash
gofmt -w <changed-go-files>
go test ./...
```

For frontend changes:

``` bash
cd frontend
npm run lint
npm run build
```

For full integration:

1.  Start backend.
2.  Verify `/health`.
3.  Start frontend.
4.  Verify Google authentication.
5.  Verify goal creation.
6.  Verify proof submission.
7.  Verify approval/rejection.
8.  Verify unauthorized approval.
9.  Verify duplicate approval returns 409.
10. Verify owner cannot change status through PATCH.

------------------------------------------------------------------------

## Do Not Break Existing Security Tests

These are known-good behaviors:

### Status bypass

Attempt:

``` http
PATCH /goals/{id}
```

with:

``` json
{
  "status": "completed"
}
```

Expected:

``` text
status remains active
```

### Duplicate approval

Attempt to decide an already decided proof.

Expected:

``` text
HTTP 409 Conflict
proof has already been decided
```

If a change breaks either behavior, stop and fix it before continuing.

------------------------------------------------------------------------

## Current Product Scope

### Phase 1

Current:

-   authentication
-   goals
-   proof
-   single approver
-   approval/rejection

### Later

Potential future work:

-   community approval
-   multiple approvers
-   quorum
-   notifications
-   richer proof
-   audit history
-   stronger atomic/concurrent approval handling

Do not prematurely implement future approval modes in Phase 1 code.

------------------------------------------------------------------------

## Important Known Design Limitations

These are known areas for future hardening:

1.  Approval records and goal status changes are not currently one
    atomic DynamoDB transaction.
2.  Concurrent duplicate decisions need stronger
    conditional-write/transaction guarantees.
3.  The current approval data model is single-approver.
4.  Approval lookup/listing is currently owner-key oriented and may need
    redesign for approver dashboards.
5.  Existing goals created before `ApproverID` existed may not have an
    approver.

Treat these as known architectural follow-ups, not reasons to rewrite
the current system without a task.

------------------------------------------------------------------------

## Working Principle

Prefer:

``` text
small change
→ test
→ inspect diff
→ commit
```

over large rewrites.

When uncertain:

1.  Inspect existing code.
2.  Identify the current contract.
3.  Preserve working behavior.
4.  Make the smallest change that satisfies the requirement.
5.  Test the affected workflow.
