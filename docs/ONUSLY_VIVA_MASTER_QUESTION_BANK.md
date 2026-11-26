# ONUSLY Master Viva Examination Defense & Question Bank
> **300 Comprehensive Technical Questions & Answers (50 from each Core Tech Stack)**
> Designed for Engineering Viva Defense, Architecture Reviews, and Deep Technical Audits.

---

## Table of Contents
1. [Tech Stack 1: Go (Golang 1.26+) Backend, REST APIs & Concurrency Systems (#50 Questions)](#tech-stack-1-go-golang-1.26-backend-rest-apis-&-concurrency-systems)
2. [Tech Stack 2: Amazon DynamoDB & Single-Table Database Architecture (#50 Questions)](#tech-stack-2-amazon-dynamodb-&-single-table-database-architecture)
3. [Tech Stack 3: React 19, Modern Frontend Web, Vite 8 & Tailwind CSS v4 (#50 Questions)](#tech-stack-3-react-19-modern-frontend-web-vite-8-&-tailwind-css-v4)
4. [Tech Stack 4: Android (Kotlin API 26-35) Mobile Systems & Zero-Surveillance Privacy (#50 Questions)](#tech-stack-4-android-kotlin-api-26-35-mobile-systems-&-zero-surveillance-privacy)
5. [Tech Stack 5: Desktop Systems (Windows/Linux) & Chromium Extension (Manifest V3) (#50 Questions)](#tech-stack-5-desktop-systems-windowslinux-&-chromium-extension-manifest-v3)
6. [Tech Stack 6: Cloud Infrastructure, SQS, Nginx Reverse Proxy & DevOps/SRE Security (#50 Questions)](#tech-stack-6-cloud-infrastructure-sqs-nginx-reverse-proxy-&-devopssre-security)

---

## Tech Stack 1: Go (Golang 1.26+) Backend, REST APIs & Concurrency Systems

> **Total Questions**: 50 | **Target Domain**: Core Language, Concurrency, Architecture, Edge Cases & Onusly Implementations

### Q1. What concurrency model does Go use, and how does it differ from traditional OS threads?

**Answer:**
Go uses the **Communicating Sequential Processes (CSP)** concurrency model, popularized by C.A.R. Hoare. Instead of communicating by sharing memory with complex locking, Go processes communicate by sharing channels.
Under the hood, Go uses lightweight user-space threads called **Goroutines**. While an OS thread has a fixed stack size of 1-8 MB and incurs high context-switching overhead in the kernel, a goroutine starts with a tiny stack of just **2 KB** that grows and shrinks dynamically on the heap. This allows Onusly's backend to handle tens of thousands of concurrent WebSocket connections and HTTP requests simultaneously on modest EC2 instances with negligible memory overhead.

---

### Q2. Explain the Go GMP Scheduler and how work-stealing operates.

**Answer:**
The Go runtime scheduler orchestrates goroutines using three abstractions: **G** (Goroutine), **M** (Machine / OS thread), and **P** (Processor / logical context, usually equal to `GOMAXPROCS` or CPU cores).
Each P maintains a local run-queue of runnable goroutines (max 256), and there is also a global run-queue. When a thread M executes on processor P, it pops goroutines from P's local queue without locking. If P's local run-queue becomes empty, the scheduler employs a **work-stealing algorithm**: P will attempt to steal half the runnable goroutines from another processor's local queue. If none are found, it checks the global queue or network poller. When a goroutine blocks on a system call (e.g., file I/O), M dissociates from P, and another thread M takes over P, ensuring CPU cores remain saturated.

---

### Q3. What is the difference between buffered and unbuffered channels in Go? How are they used in Onusly?

**Answer:**
**Unbuffered channels** have a capacity of 0. A send operation blocks until another goroutine performs a receive on that same channel, providing synchronous handoff and temporal synchronization.
**Buffered channels** have an internal ring buffer of capacity N. Sends proceed without blocking as long as the buffer is not full; receives proceed without blocking as long as the buffer is not empty.
In Onusly, unbuffered channels are used for synchronous acknowledgments and worker rendezvous, while buffered channels (e.g., in the WebSocket client's `send` channel with capacity 256) buffer outbound messages. If a slow client cannot consume frames in time and the buffer fills up, the server can detect the lag and close the connection rather than blocking the real-time hub goroutine.

---

### Q4. What happens if you send to or receive from a closed channel? What if the channel is nil?

**Answer:**
- **Closed Channel**: Sending to a closed channel causes an immediate **runtime panic**. Receiving from a closed channel never blocks: it immediately returns the zero value of the channel type and a boolean `false` (e.g., `val, ok := <-ch`).
- **Nil Channel**: Both sending to and receiving from a `nil` channel block **forever**.
In select statements, setting a channel variable to `nil` is a standard Go idiom to disable that case dynamically once its data stream is exhausted.

---

### Q5. How does `sync.RWMutex` differ from `sync.Mutex`, and where is it used in Onusly?

**Answer:**
`sync.Mutex` provides exclusive mutual exclusion: only one goroutine can hold the lock at any time (both readers and writers block each other).
`sync.RWMutex` provides reader/writer locking: multiple readers can hold `RLock()` concurrently as long as no writer holds `Lock()`. When a writer requests `Lock()`, new readers are blocked until the writer completes, preventing writer starvation.
In Onusly, `sync.RWMutex` is heavily utilized in `internal/realtime/hub.go` and `internal/approval/repository.go`. For instance, the WebSocket Hub's client maps (`users`, `emails`, `handles`) are read frequently on every broadcast or lookup using `RLock()`, while connections joining or leaving acquire the exclusive `Lock()`, ensuring minimal read contention.

---

### Q6. Explain Goroutine leaks and how Onusly prevents them using `context.Context`.

**Answer:**
A goroutine leak occurs when a spawned goroutine remains blocked indefinitely (e.g., waiting on an unbuffered channel that no one writes to, waiting on an uncancelled HTTP request, or hung in a loop). Because goroutine stacks and referenced memory cannot be garbage-collected, leaked goroutines gradually exhaust system memory.
Onusly prevents goroutine leaks by propagating `context.Context` through all service and repository layers. Using `context.WithTimeout` or `context.WithCancel`, every outbound call (such as DynamoDB calls with an 800ms deadline in `approval/repository.go`) is bounded. When the client disconnects or the timeout triggers, the context's `Done()` channel closes, prompting listening goroutines to exit immediately and clean up resources.

---

### Q7. What is the memory layout of a Go slice, and why is `append` sometimes dangerous?

**Answer:**
A slice in Go is a 24-byte header (on 64-bit systems) consisting of three words: a pointer to the underlying backing array, `len` (current number of elements), and `cap` (maximum capacity before reallocation).
`append` is dangerous if multiple functions or goroutines share a slice and expect modifications to be isolated: when `append` exceeds `cap`, Go allocates a new, larger backing array (typically doubling capacity) and copies the elements, severing the link to the original backing array. If `cap` is not exceeded, `append` mutates the shared underlying array in-place, which can lead to silent race conditions and data corruption across callers.

---

### Q8. How does Go's Garbage Collector work, and what is the Tri-Color Mark-and-Sweep algorithm?

**Answer:**
Go uses a concurrent, tri-color mark-and-sweep garbage collector designed for ultra-low latency (sub-millisecond STW pauses):
1. **White**: Objects that are unvisited and candidates for collection.
2. **Grey**: Objects visited by the GC whose pointed-to child objects have not yet been evaluated.
3. **Black**: Reachable objects whose references have all been scanned.
The GC starts by marking root objects (globals, stack variables) as Grey. While the application runs concurrently, the GC scans Grey objects, turns them Black, and marks their children Grey. A **write barrier** intercepts pointer writes made by active mutator goroutines during the phase to ensure no white object referenced by a black object is missed. Finally, any remaining White objects are swept and reclaimed.

---

### Q9. What is Escape Analysis in Go, and how does it impact performance?

**Answer:**
Escape analysis is a compile-time optimization where the Go compiler determines whether an allocated variable can safely reside on the stack or must 'escape' to the heap.
- **Stack allocation** is virtually free (advancing a stack pointer) and requires zero GC overhead since memory is automatically reclaimed when the function frame pops.
- **Heap allocation** requires dynamic runtime allocation and subsequent tracking and collection by the GC.
If a variable's address is returned outside its scope, stored in an interface, or dynamically resized beyond stack limits, it escapes to the heap. In Onusly, critical hot paths avoid heap allocations by pre-allocating slices with known capacities and using concrete types rather than boxing values into empty interfaces.

---

### Q10. Why does Onusly choose the standard library `net/http` router over external frameworks like Gin or Fiber?

**Answer:**
1. **Production Reliability & Zero Dependency Bloat**: Go's standard `net/http` (especially enhanced with method-based routing in modern Go) is battle-tested, secure, and has zero external dependencies that could introduce supply-chain vulnerabilities.
2. **Interface Consistency**: It strictly adheres to `http.Handler` and `http.HandlerFunc`, ensuring composability with standard middleware chains (MaxBytesLimiter, Security headers, CORS, rate limiting).
3. **Framework Lock-in Avoidance**: Frameworks like Fiber rely on `fasthttp`, which deviates from standard `net/http` semantics and breaks compatibility with many standard middleware and HTTP/2 libraries.

---

### Q11. Explain how Gorilla WebSocket is integrated into Onusly's real-time hub.

**Answer:**
In `internal/realtime`, the HTTP connection is upgraded to WebSocket via `websocket.Upgrader` (with origin checking). Each client connection spawns two dedicated goroutines:
1. **`readPump`**: Continuously reads incoming JSON frames, processes client heartbeats (ping/pong), updates read deadlines, and routes incoming messages (e.g., chat messages or pairing events) to the Hub.
2. **`writePump`**: Listens on a client-specific buffered channel (`send chan []byte`) and writes WebSocket text frames to the network socket, setting write deadlines.
The **`Hub`** maintains maps of connected clients (`users`, `emails`, `handles`) protected by a `sync.RWMutex`. When a goal status updates or proof is approved, the Hub serializes the event once and dispatches it concurrently to all active client channels of that user.

---

### Q12. How does Onusly handle ping/pong heartbeats to detect dead WebSocket connections?

**Answer:**
WebSocket connections can silently die due to dropped Wi-Fi, mobile handovers, or intermediate firewall timeouts without sending a TCP FIN packet.
Onusly's `readPump` sets a read deadline (e.g., `SetReadDeadline(time.Now().Add(pongWait))`) and registers a PongHandler (`SetPongHandler`). The `writePump` periodically sends a `websocket.PingMessage` on a ticker interval (less than `pongWait`, typically 54s vs 60s). If the client fails to return a Pong within the deadline, `readPump` errors out, triggering socket closure, unregistering the client from the Hub, and reclaiming memory.

---

### Q13. Describe the Middleware Chain pattern implemented in Onusly's HTTP pipeline.

**Answer:**
Middleware in Go wraps an `http.Handler` returning another `http.Handler`: `func(http.Handler) http.Handler`. Onusly executes incoming HTTP requests through a structured onion-architecture chain:
`SecurityHeaders -> MaxBytesLimiter -> CORS -> RateLimiter -> Idempotency -> AuthMiddleware -> Handler`.
Each middleware can inspect, mutate, or short-circuit the request. For example, if `MaxBytesLimiter` detects a request body exceeding 50 MB, or if `RateLimiter` detects a rate limit violation (e.g., >30 req/min for auth), it writes an error response immediately and returns, never invoking downstream business logic.

---

### Q14. What is `MaxBytesLimiter` and why is it critical for multi-file proof uploads?

**Answer:**
`MaxBytesLimiter` wraps the request body using `http.MaxBytesReader(w, r.Body, maxUploadSize)`.
In Onusly, proof submissions can include photos and PDF documents. If an attacker streams a multi-gigabyte payload, reading the stream into memory could cause an Out-Of-Memory (OOM) crash. `MaxBytesLimiter` enforces a hard ceiling of **50 MB** overall (and 32 MB per individual file). If the limit is crossed, reading returns an error, preventing memory exhaustion and denial-of-service.

---

### Q15. How does Onusly implement Sliding-Window Rate Limiting in Go?

**Answer:**
In `internal/middleware/ratelimit.go`, Onusly implements an in-memory sliding-window or token-bucket limiter keyed by client IP or User ID.
Different routes have specialized limits:
- Auth endpoints (`/auth/*`): 30 req/min to prevent credential stuffing.
- Device pairing (`/enforcement/pair/*`): 20 req/min to prevent 6-digit PIN brute-forcing.
- Standard API endpoints: 180 req/min.
State is tracked using atomic counters and timestamps protected by mutexes with periodic eviction routines to prevent memory leakage from stale IPs.

---

### Q16. Explain the Idempotency Layer in Onusly and how it handles retried mutations.

**Answer:**
Network unreliability often causes clients to retry mutating requests (e.g., creating a goal or approving proof). If the server processed the first request but the response was lost in transit, a retry could cause duplicate records.
Onusly provides idempotency middleware where clients pass an `Idempotency-Key: <UUID>` header. The server checks an in-memory/DynamoDB idempotency cache:
1. If the key exists with a completed response, the cached HTTP status code and response payload are returned immediately without executing the handler.
2. If the key is currently being processed, concurrent requests receive `409 Conflict` or wait.
3. If fresh, the key is stored with a 24-hour expiration.

---

### Q17. How is Google OAuth 2.0 verified in Go without trusting client-supplied claims?

**Answer:**
When a user logs in via Google, the frontend sends an `id_token` to `POST /auth/google`.
In `internal/auth/service.go`, Onusly uses the official `google.golang.org/api/idtoken` validator to verify the cryptographic signature of the token against Google's public JWKS certificates. It verifies that:
1. The token is cryptographically signed by Google.
2. The audience (`aud`) matches Onusly's `GOOGLE_CLIENT_ID`.
3. The token has not expired (`exp`).
Only after this validation does the backend extract the unique Google Subject ID (`sub`) and email, completely ignoring any unverified claims sent in the request body.

---

### Q18. How does Onusly issue and validate JWTs? Why HS256 instead of RS256?

**Answer:**
Onusly issues JWTs signed with **HS256** (HMAC-SHA256) using a 256-bit cryptographically secure server secret. The payload contains standard claims: `sub` (User ID), `email`, `exp` (expiration timestamp), and `token_version`.
**Why HS256?** For a monolithic/single-cluster backend where the issuing server is also the verifying server, HS256 offers significantly faster symmetric signature verification and encryption performance compared to asymmetric RS256/ES256, without needing external key distribution infrastructure.

---

### Q19. How does Session Revocation and `TokenVersion` work in Onusly?

**Answer:**
Because standard JWTs are stateless, revoking them before expiration usually requires an expensive blacklist. Onusly solves this using a **Token Versioning** pattern:
Each user profile in DynamoDB contains an integer field `TokenVersion`. When a JWT is minted, the current `TokenVersion` is embedded in the claims. When a user logs out everywhere (`POST /auth/revoke-all`), the backend atomically increments `TokenVersion` in DynamoDB.
In `AuthMiddleware`, the token's version is checked against the user's latest version. If mismatched, the request is rejected with `401 Unauthorized`, and all active WebSocket connections for that user are immediately disconnected.

---

### Q20. How does Onusly enforce loopback binding (`validateAndResolveBindAddress`) and why?

**Answer:**
In `cmd/server/main.go`, Onusly inspects the configured listen address before starting the HTTP server. In production mode, binding to `0.0.0.0` or public network interfaces is strictly forbidden unless overridden with `ALLOW_INSECURE_BIND=true`.
The server must bind exclusively to `127.0.0.1:8080`. This enforces **Zero-Trust Network Perimeter Architecture**: external traffic can only reach the application through Nginx, which handles TLS termination, rate-limiting, and WebSocket header upgrades. This prevents bypass attacks where an attacker bypasses Nginx rules by hitting the Go port directly.

---

### Q21. What is the Self-Approval Guard in Onusly, and how is it implemented?

**Answer:**
A foundational rule of Onusly is that no user can verify their own goal. In `internal/approval/service.go`, when an approval or rejection request is received (`Decide` method):
```go
if approverID == ownerID {
    return nil, ErrSelfApproval // 403 Forbidden
}
```
Furthermore, during goal creation (`internal/goals/service.go`), if the assigned approver email or handle resolves to the creator's own ID, creation fails with `ErrSelfAssignment`.

---

### Q22. How does Onusly handle panic recovery in HTTP handlers?

**Answer:**
If an unhandled nil-pointer dereference or slice index out-of-bounds occurs inside a goroutine, the entire Go process can crash. Onusly implements a panic recovery middleware using Go's `recover()` built-in inside a deferred function:
```go
defer func() {
    if r := recover(); r != nil {
        log.Printf("PANIC recovered: %v\n%s", r, debug.Stack())
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
    }
}()
```
This ensures a faulty request handler logs the stack trace and returns HTTP 500 without crashing the server for all other users.

---

### Q23. Explain the difference between `defer`, `panic`, and `recover` execution order.

**Answer:**
- `defer` schedules a function call to run immediately before the surrounding function returns, executed in **LIFO (Last-In-First-Out)** order. Deferred functions can read and modify named return parameters.
- `panic` halts normal function execution, unwinds the call stack of the current goroutine, running deferred functions at each level until the program terminates.
- `recover` can only be invoked inside a deferred function during stack unwinding. It intercepts the panic, restores normal execution, and returns the error value passed to `panic`.

---

### Q24. What is Table-Driven Testing in Go, and how is it applied in Onusly?

**Answer:**
Table-driven testing is the standard Go idiom where test cases are defined as an array or slice of anonymous structs containing input arguments and expected outputs:
```go
tests := []struct {
    name        string
    approverID  string
    ownerID     string
    expectedErr error
}{
    {"Valid Approver", "user-2", "user-1", nil},
    {"Self Approval", "user-1", "user-1", ErrSelfApproval},
}
for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) { ... })
}
```
Onusly uses this pattern across `internal/approval/approval_test.go` and `internal/friends/friends_test.go` to test dozens of authorization edge cases cleanly and reproducibly.

---

### Q25. How does Go's `context.WithTimeout` prevent cascading database failures?

**Answer:**
When database calls stall under high load, callers continue piling up, exhausting server connection pools and threads. By wrapping DynamoDB requests in a context with a strict deadline (e.g., `ctx, cancel := context.WithTimeout(parentCtx, 800*time.Millisecond)`):
If DynamoDB does not respond within 800ms, the context automatically cancels. The AWS SDK cancels the underlying HTTP transport socket, releases the client connection, and returns a `context.DeadlineExceeded` error, preventing worker thread starvation.

---

### Q26. How does Onusly mock external dependencies like DynamoDB or Google Auth in unit tests?

**Answer:**
Onusly defines small, focused Go interfaces for repositories and external clients (e.g., `ApprovalRepository`, `UserResolver`, `GoalVerifier`).
In tests, mock implementations of these interfaces (e.g., in-memory map repositories) are injected into the services rather than hitting live AWS or Google APIs. This allows lightning-fast, deterministic unit testing without external network dependencies.

---

### Q27. What is the difference between value receivers and pointer receivers on Go structs?

**Answer:**
- **Value receiver** (`func (s Service) Do()`): Operates on an independent copy of the struct. Any modifications to fields are lost upon return.
- **Pointer receiver** (`func (s *Service) Do()`): Operates directly on the memory address of the original struct. It allows mutative state changes, avoids copying large structs, and ensures consistent interface implementation across method sets.

---

### Q28. How does Onusly ensure clean graceful shutdown when the server receives a SIGINT or SIGTERM?

**Answer:**
In `cmd/server/main.go`, Onusly creates a signal channel listening for `os.Interrupt` and `syscall.SIGTERM`:
```go
stop := make(chan os.Signal, 1)
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
<-stop
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
server.Shutdown(ctx)
```
`server.Shutdown()` stops accepting new connections, allows in-flight HTTP requests and WebSocket write buffers to drain cleanly within 10 seconds, and shuts down background workers before process termination.

---

### Q29. What is the difference between `errors.Is` and `errors.As` in modern Go?

**Answer:**
- `errors.Is(err, target)` compares `err` against a target sentinel error (like `ErrSelfApproval`), unwrapping nested errors wrapped with `%w`.
- `errors.As(err, &target)` checks if an error in the unwrap chain matches a specific custom error type and assigns it to the target pointer, allowing access to custom error fields (e.g., extracting AWS request IDs from DynamoDB SDK errors).

---

### Q30. How are uploaded proof files validated against Path Traversal vulnerabilities in Go?

**Answer:**
When processing multi-file uploads in `internal/proof`, file paths provided by client multipart headers (`filename`) cannot be trusted. If a client sends `../../../../etc/passwd`, writing to disk could overwrite critical files.
Onusly sanitizes filenames using `filepath.Base()` and generates unique UUIDs (`uuid.New().String() + filepath.Ext(sanitized)`), storing files strictly within a configured sandboxed directory (`/uploads/proofs/`) and verifying that the canonical target path starts with the upload root directory.

---

### Q31. Explain how Go's `sync.WaitGroup` works and common pitfalls with its usage.

**Answer:**
`sync.WaitGroup` coordinates concurrent goroutines. It maintains an internal counter: `Add(delta)` increments the counter, `Done()` decrements it, and `Wait()` blocks until the counter reaches zero.
**Common Pitfalls:**
1. Calling `wg.Add(1)` inside the spawned goroutine instead of before launching it, causing a race condition where `Wait()` returns before goroutines start.
2. Passing `sync.WaitGroup` by value instead of by pointer, which copies the internal counter and causes deadlocks.

---

### Q32. How does Onusly prevent data races in Go, and how are they detected during build/test?

**Answer:**
Data races occur when two goroutines access the same memory location concurrently and at least one access is a write. Onusly prevents races using mutexes (`sync.RWMutex`), atomic operations, and channel-based ownership transfer.
During CI testing, all unit and integration tests are compiled and executed with the Go Race Detector enabled: `go test -race ./...`. The race detector instruments memory accesses with 8-byte shadow memory to detect unsynchronized accesses at runtime.

---

### Q33. What is the purpose of `init()` functions in Go, and why should they be used sparingly?

**Answer:**
`init()` functions run automatically once when a package is initialized, before `main()`. They accept no arguments and return no values.
While useful for registering drivers or immutable static lookups, they should be used sparingly because they hide implicit global state, execute without dependency injection, cannot return errors for graceful handling, and make unit testing difficult.

---

### Q34. How does Go's `iota` work, and how is it used in enum representations?

**Answer:**
`iota` is a compile-time predeclared identifier that represents successive untyped integer constants starting from 0 within a `const` block:
```go
const (
    TargetDomain TargetType = iota
    TargetDesktopApp
    TargetAndroidPackage
)
```
It simplifies defining distinct enumeration constants without manual numbering.

---

### Q35. What is the difference between `new()` and `make()` in Go?

**Answer:**
- `new(T)` allocates zeroed memory for type `T` and returns a pointer `*T`. It can be used on any type (structs, ints, etc.).
- `make(T, args)` is exclusively used for initializing the three built-in reference types: **slices, maps, and channels**. It allocates and initializes the internal runtime structures (e.g., hash buckets for maps, ring buffers for channels) and returns an initialized value of type `T` (not a pointer).

---

### Q36. How does Onusly implement mutual friendship requests in `internal/friends`?

**Answer:**
In `internal/friends/service.go`, when User A sends a friend request to User B, an outgoing request item (`FRIEND_REQ_OUT#<reqID>`) is created under User A and an incoming request item (`FRIEND_REQ_IN#<reqID>`) is created under User B.
When User B accepts, both request records are deleted and two bidirectional friendship records are created: `USER#A` / `FRIEND#B` and `USER#B` / `FRIEND#A`. This allows O(1) retrieval of friends for either user.

---

### Q37. Explain how the `internal/events` queue publisher decouples operations in Onusly.

**Answer:**
When a goal state transitions (e.g., proof submitted or goal approved), synchronous HTTP handlers should not wait for external push notifications (FCM) or audit pipelines to finish.
Onusly defines an `events.Publisher` interface. In production, this publishes an event payload to AWS SQS. In local development or testing, an in-memory or `NoopPublisher` is used. A decoupled background worker polls the queue and dispatches notifications asynchronously.

---

### Q38. How does Go's memory model define synchronization between goroutines?

**Answer:**
Go's memory model specifies the conditions under which reads of a variable in one goroutine are guaranteed to observe values produced by writes in another goroutine. Memory operations are ordered by **happens-before** relationships established through channel operations, mutex acquisitions/releases, and `sync/atomic` operations. Without these explicit synchronization points, the compiler and CPU are free to reorder memory accesses, causing reads of stale data.

---

### Q39. How are HTTP headers and response streaming secured against MIME-sniffing in Onusly?

**Answer:**
In `internal/middleware/security.go`, Onusly injects standard OWASP defensive headers on all responses:
- `X-Content-Type-Options: nosniff` (forces browsers to adhere strictly to the MIME type sent in `Content-Type`, preventing MIME confusion attacks).
- `X-Frame-Options: DENY` (prevents clickjacking attacks by blocking iframing).
- `Strict-Transport-Security: max-age=63072000; includeSubDomains` (enforces HTTPS).
- `Content-Security-Policy: default-src 'self'`.

---

### Q40. Why does Onusly use structured logging and context logging?

**Answer:**
Standard unstructured `log.Println` produces text blobs that are difficult to parse in log aggregators (e.g., CloudWatch, Datadog). Structured logging outputs log entries with machine-readable key-value pairs (e.g., `userID`, `goalID`, `requestID`, `duration_ms`). By propagating a `requestID` through `context.Context`, all log lines associated with a single user request can be correlated across services.

---

### Q41. What is the difference between string and byte slice in Go, and how can conversions cause allocations?

**Answer:**
A `string` in Go is an immutable 16-byte header containing a pointer to an underlying byte array and a length.
A `[]byte` is a mutable 24-byte header containing a pointer, length, and capacity.
Converting `[]byte(str)` or `string(bytes)` requires the runtime to copy the backing array to guarantee string immutability, which triggers heap allocation. Go optimizes conversions in specific scenarios (e.g., `m[string(b)]` map lookups), but in hot paths, unnecessary conversions should be avoided.

---

### Q42. How does Onusly manage database connection pooling with AWS SDK for Go v2?

**Answer:**
In AWS SDK for Go v2, the `dynamodb.Client` does not maintain a raw TCP connection pool itself; it relies on the underlying `http.Client` transport (`http.Transport`).
`http.Transport` manages a pool of idle persistent TCP connections (`MaxIdleConns`, `MaxIdleConnsPerHost`, `IdleConnTimeout`). Onusly configures a shared singleton client in `internal/shared` to reuse connections across requests, eliminating the overhead of repeated TCP 3-way handshakes and TLS negotiations.

---

### Q43. How does Onusly structure its Go packages to avoid circular dependencies?

**Answer:**
Go strictly forbids circular imports (`import cycle not allowed`). Onusly follows clean layered architecture:
- High-level orchestrators (`cmd/server`) import internal domains.
- Internal domain services (`approval`, `goals`, `proof`, `chat`) depend on abstractions and shared models (`internal/shared`, `internal/events`).
- When cross-domain communication is needed (e.g., approval needing to verify goal status), it depends on interfaces or domain services passed via constructor injection rather than package cross-imports.

---

### Q44. What is the function of the `manifest_generator` utility in `cmd/manifest_generator`?

**Answer:**
To protect desktop and browser extension clients from malicious tampering or unauthorized updates, Onusly includes a standalone CLI tool in `cmd/manifest_generator`. It signs release binaries and XML manifests using an **Ed25519** private key. The public key is hardcoded into client binaries, allowing clients to cryptographically verify update authenticity before executing an installer.

---

### Q45. How are query parameters safely parsed and sanitized in Go REST endpoints?

**Answer:**
In Onusly handlers, URL query parameters (e.g., `r.URL.Query().Get("status")`) are trimmed using `strings.TrimSpace()` and normalized (e.g., `strings.ToLower()`). Inputs are strictly matched against allowed whitelists (e.g., valid goal statuses: `active`, `proof_submitted`, `completed`). Unexpected or malformed query values are rejected immediately with `400 Bad Request`.

---

### Q46. Explain the concept of Zero Allocation string concatenation in Go.

**Answer:**
Using the `+` operator to concatenate strings in a loop creates a new string and copies data on every iteration, leading to $O(N^2)$ memory allocations. Instead, Go provides `strings.Builder`, which writes bytes directly into an internal growing buffer and returns the accumulated string using `unsafe` zero-copy conversion, achieving $O(N)$ performance with minimal allocations.

---

### Q47. How does Onusly implement sub-second push notifications over WebSockets?

**Answer:**
When an approver validates proof, the approval service executes `hub.BroadcastToUser(ownerID, event)`.
The Hub looks up the user's active client connections in memory in $O(1)$ time and pushes the serialized event frame directly into each client's non-blocking channel. The client's `writePump` immediately flushes the frame onto the TCP socket, delivering sub-second notifications to the user's dashboard and mobile app.

---

### Q48. What is the difference between buffered I/O and unbuffered I/O in Go (`bufio` package)?

**Answer:**
Unbuffered I/O issues a system call (like `read()` or `write()`) for every single byte or operation, which is CPU-expensive. `bufio.Reader` and `bufio.Writer` wrap an existing I/O stream with an in-memory buffer (typically 4 KB). Data is read or written in large chunks, dramatically reducing system call overhead when processing socket streams or file parsing.

---

### Q49. How does Onusly validate email addresses and user handles during registration?

**Answer:**
In `internal/auth` and `internal/friends`, emails are validated using standard RFC-compliant regex patterns and converted to lowercase to ensure case-insensitive uniqueness. User handles must conform to strict format rules: 3-20 characters, alphanumeric and underscores only (`^[a-zA-Z0-9_]{3,20}$`), preventing special character injection and URL encoding collisions.

---

### Q50. What is the role of `go.sum` and how does it prevent supply chain attacks?

**Answer:**
`go.sum` contains cryptographic SHA-256 checksums of the exact source code and `go.mod` files of every direct and indirect module dependency. When running `go build` or `go test`, the Go toolchain verifies downloaded packages against `go.sum`. If a remote dependency repository is compromised or modified upstream, the hash mismatch immediately halts compilation, preventing supply chain attacks.

---

## Tech Stack 2: Amazon DynamoDB & Single-Table Database Architecture

> **Total Questions**: 50 | **Target Domain**: Core Language, Concurrency, Architecture, Edge Cases & Onusly Implementations

### Q1. What is Single-Table Design in Amazon DynamoDB, and why did Onusly choose it over a relational database (e.g., PostgreSQL)?

**Answer:**
Single-Table Design is an architectural pattern where all application entities (Users, Goals, Proofs, Approvals, Devices, Friends, Chat messages) are stored in a single DynamoDB table rather than separate tables.
**Why Onusly chose it over PostgreSQL:**
1. **Predictable Single-Digit Millisecond Latency**: DynamoDB partitions data by hash key (`PK`). Regardless of table size (10 MB or 10 TB), lookup latency remains constant (3-5 ms).
2. **Zero Maintenance & Serverless Scaling**: No connection pool limits, no CPU throttling from complex multi-table joins, and zero manual database migrations.
3. **Atomic Co-located Transactions**: Entities belonging to the same partition (e.g., a Goal and its Proofs) can be retrieved or mutated in a single query or atomic transaction without foreign key overhead.

---

### Q2. Explain the Composite Primary Key (Partition Key and Sort Key) model in DynamoDB.

**Answer:**
A composite primary key consists of two elements:
1. **Partition Key (`PK`)**: DynamoDB passes the PK through an internal hash function to determine the exact physical storage partition where the item resides.
2. **Sort Key (`SK`)**: Within that physical partition, items are stored in physical sorted order according to the UTF-8 binary representation of the SK.
This enables range queries (e.g., `begins_with`, `between`, `<`, `>`) across items in the same partition.

---

### Q3. What is the primary table schema and naming convention used in Onusly?

**Answer:**
Onusly uses generic string attributes `PK` and `SK` with typed hierarchical prefixes:
- **User Profile**: `PK = USER#<google-sub>`, `SK = PROFILE`
- **Goal**: `PK = USER#<ownerID>`, `SK = GOAL#<goalID>`
- **Proof**: `PK = USER#<ownerID>`, `SK = PROOF#<goalID>#<proofID>`
- **Approval**: `PK = USER#<ownerID>`, `SK = APPROVAL#<goalID>#<approvalID>`
- **Device**: `PK = USER#<ownerID>`, `SK = DEVICE#<deviceID>`
- **Friend**: `PK = USER#<userID>`, `SK = FRIEND#<friendID>`
- **1-on-1 Chat**: `PK = USER#<userID>`, `SK = CHAT#<peerID>#<timestamp>#<msgID>`
- **Community Chat**: `PK = COMMUNITY#<communityID>`, `SK = MSG#<timestamp>#<msgID>`

---

### Q4. What is a Global Secondary Index (GSI), and how does it differ from a Local Secondary Index (LSI)?

**Answer:**
- **Local Secondary Index (LSI)**: Shares the same Partition Key as the base table but uses a different Sort Key. It must be defined at table creation time, has a 10 GB limit per partition key, and shares throughput with the base table.
- **Global Secondary Index (GSI)**: Can define a completely different Partition Key and Sort Key from the base table. It can be added or deleted at any time, has no partition size limit, and maintains its own read/write throughput allocation.
Onusly uses GSIs exclusively (GSI1 and GSI2) to maintain horizontal scalability.

---

### Q5. Explain the purpose and design of GSI1 in Onusly.

**Answer:**
GSI1 provides **Identifier Resolution** when the caller does not know the internal `ownerID`:
1. **User Resolution by Email**: `GSI1PK = EMAIL#<email>`, `GSI1SK = PROFILE`. Used when a user types an approver's email during goal creation or friend request.
2. **Goal Resolution by ID**: `GSI1PK = GOAL#<goalID>`, `GSI1SK = METADATA`. Used when fetching or verifying a goal without knowing its owner in advance.

---

### Q6. Explain the purpose and design of GSI2 (Approver Inbox) in Onusly.

**Answer:**
GSI2 powers the **Approver Inbox** dashboard:
- `GSI2PK = APPROVER#<approverID|email|handle>`
- `GSI2SK = STATUS#<status>#GOAL#<goalID>`
When an approver logs in and opens their verification queue, the backend queries GSI2 with `GSI2PK = APPROVER#<callerID>` and `GSI2SK begins_with STATUS#proof_submitted`. This yields all goals awaiting the caller's review in $O(1)$ query time without scanning the base table.

---

### Q7. What is a Sparse Index in DynamoDB, and how does Onusly leverage it?

**Answer:**
In DynamoDB, an item is only indexed in a GSI if the item contains the attributes designated as the GSI's Partition Key and Sort Key. If an attribute is missing, DynamoDB ignores the item.
This is called a **Sparse Index**. In Onusly, only active goals with an assigned approver contain `GSI2PK` and `GSI2SK`. Once a goal is archived or completed, or if a record is a chat message, it does not populate `GSI2PK`, keeping GSI2 lean, fast, and cost-effective.

---

### Q8. How does DynamoDB's `TransactWriteItems` ensure atomicity in Onusly's approval engine?

**Answer:**
In `internal/approval/repository.go`, approving or rejecting a goal requires updating the goal's status while simultaneously inserting an immutable audit record of the approval.
`TransactWriteItems` executes an all-or-nothing transaction (up to 100 items or 4 MB) across multiple records:
1. **Action 1 (Put)**: Inserts the `APPROVAL#<goalID>#<approvalID>` item.
2. **Action 2 (Update)**: Updates the goal item's `status` to `completed` (or `active` if rejected) and modifies `GSI2SK` accordingly.
Both operations succeed atomically, or the entire transaction is rolled back if any condition fails.

---

### Q9. How does Onusly prevent race conditions when two approvers or concurrent requests attempt to decide the same goal?

**Answer:**
In `CreateDecision`, the transaction includes a **Condition Expression** on the goal item:
`ConditionExpression: attribute_exists(PK) AND #status = :expected_status` (where `:expected_status = 'proof_submitted'`).
If two approvers submit decisions simultaneously, the first transaction commits and changes the status to `completed` or `active`. The second transaction fails DynamoDB's condition check, returning `TransactionCanceledException` (ConditionalCheckFailed), which the Go service deterministically maps to **HTTP 409 Conflict**.

---

### Q10. How does Onusly prevent 'Phantom Check' resurrection on deleted goals?

**Answer:**
In DynamoDB, executing an `UpdateItem` on a key that does not exist will, by default, create a new item with that primary key. If a user deletes a goal and an in-flight background worker or heartbeat attempts to update it, the goal could be 'resurrected' as a phantom item.
Onusly prevents this by adding `ConditionExpression: attribute_exists(PK)` on all update operations. If the goal was deleted, the update fails immediately with `ConditionalCheckFailedException`.

---

### Q11. What is the difference between `Query` and `Scan` operations in DynamoDB?

**Answer:**
- **Query**: Searches items using the Partition Key (`PK = value`) and optional Sort Key conditions (`begins_with`, range). It only reads items within that partition, consuming minimal Read Capacity Units (RCUs) in $O(\log N + K)$ time.
- **Scan**: Reads every single item in the entire table from start to finish, evaluating filter expressions only *after* reading data from disk. It consumes massive RCUs, increases latency linearly with table size, and is an anti-pattern in production.
Onusly uses `Query` for 100% of its regular operational paths and forbids `Scan`.

---

### Q12. What is the difference between Filter Expressions and Key Condition Expressions in DynamoDB?

**Answer:**
- **Key Condition Expression**: Dictates *which partition and sort key range* DynamoDB retrieves from storage. DynamoDB charges RCUs based solely on the data retrieved by the key condition.
- **Filter Expression**: Applied in-memory *after* DynamoDB reads the items from storage. Filter expressions do **not** reduce RCU consumption or query latency; they merely filter out items before sending the response back to the client.

---

### Q13. Explain Projection Expressions and why they are important for cost and performance.

**Answer:**
A `ProjectionExpression` specifies the exact subset of attributes to return from DynamoDB (e.g., `#id, #title, #status`), omitting large unneeded fields like full historical logs or target lists.
While DynamoDB still charges RCUs based on the full item size on disk (rounded up to the nearest 4 KB for strong consistency), projection expressions dramatically reduce network bandwidth, JSON serialization CPU overhead in Go, and memory allocations.

---

### Q14. What is the difference between Eventually Consistent Reads and Strongly Consistent Reads?

**Answer:**
- **Eventually Consistent Reads (Default)**: DynamoDB reads from one of three storage replicas. If a write just completed, the read might not immediately reflect the newest write (consistency usually within 1 second). Consumes **0.5 RCU** per 4 KB.
- **Strongly Consistent Reads**: Reads from a quorum of replicas, guaranteeing the response reflects all successful prior writes. Consumes **1 RCU** per 4 KB (double the cost).
*Note*: All reads on Global Secondary Indexes (GSIs) are strictly Eventually Consistent.

---

### Q15. Why does Onusly use `PAY_PER_REQUEST` (On-Demand) billing mode instead of Provisioned Capacity?

**Answer:**
`PAY_PER_REQUEST` charges purely for the read and write operations performed, with zero baseline cost when idle. Because user habits and accountability checks have spiky, non-uniform traffic patterns (e.g., spikes in morning and evening), Provisioned Capacity would either cause throttling during spikes or waste money during quiet hours. On-demand accommodates sudden 2x traffic bursts instantly without manual scaling.

---

### Q16. What is a Read Capacity Unit (RCU) and a Write Capacity Unit (WCU)?

**Answer:**
- **1 RCU**: Represents one Strongly Consistent read per second (or two Eventually Consistent reads per second) for an item up to 4 KB in size.
- **1 WCU**: Represents one write per second for an item up to 1 KB in size.
- **Transactional Reads**: Consume 2 RCUs per 4 KB.
- **Transactional Writes**: Consume 2 WCUs per 1 KB.

---

### Q17. What is the maximum item size in DynamoDB, and how does Onusly handle large proof files (images/PDFs)?

**Answer:**
DynamoDB enforces a hard limit of **400 KB** per item (including attribute names and values).
Storing raw images or multi-megabyte PDFs directly in DynamoDB is impossible. Onusly stores binary proof files (up to 32 MB) on the filesystem/S3 bucket, and stores only the metadata in DynamoDB (file name, relative URL, MIME type, file size, SHA-256 hash, and upload timestamp), keeping item size under 2 KB.

---

### Q18. Explain the Hot Partition problem in DynamoDB and how Onusly's schema prevents it.

**Answer:**
A single physical partition in DynamoDB has hard limits of **1,000 WCUs, 3,000 RCUs, and 10 GB of data**. If a large volume of traffic targets the same partition key (e.g., `PK = 'GOALS'`), that partition becomes a bottleneck and throttles all requests.
Onusly prevents hot partitions by partitioning data across individual users: `PK = USER#<ownerID>`. Since user actions are distributed across independent user partitions, no single partition absorbs disproportionate load.

---

### Q19. How does DynamoDB handle Time to Live (TTL), and how is it used in Onusly?

**Answer:**
DynamoDB TTL allows designating a numeric attribute containing a Unix timestamp (in seconds). A background DynamoDB process automatically scans and deletes expired items without consuming write capacity units.
Onusly leverages TTL for **Device Pairing Sessions** (`PAIR#<code>`), where 6-digit PIN sessions expire automatically after 10 minutes, ensuring stale pairing sessions never accumulate in the table.

---

### Q20. What is DynamoDB Streams, and how could it be used in Onusly?

**Answer:**
DynamoDB Streams captures a time-ordered log of item-level modifications (inserts, updates, deletes) in near real-time (within 24 hours).
In Onusly, DynamoDB Streams can trigger AWS Lambda functions or worker processes to emit audit events, update Elasticsearch/OpenSearch for full-text search, or synchronize cross-region replicas asynchronously.

---

### Q21. How are 1-on-1 chat messages modeled in Onusly's Single-Table design?

**Answer:**
To ensure both User A and User B have independent control over their chat views (e.g., deleting message history without affecting the other person), Onusly writes dual records:
1. Sender Copy: `PK = USER#A`, `SK = CHAT#B#<timestamp>#<msgID>`
2. Recipient Copy: `PK = USER#B`, `SK = CHAT#A#<timestamp>#<msgID>`
Querying `PK = USER#A AND SK begins_with CHAT#B` yields the complete conversation sorted chronologically.

---

### Q22. How are Community chat messages modeled?

**Answer:**
Community messages belong to a shared public room:
- `PK = COMMUNITY#<communityID>`
- `SK = MSG#<timestamp>#<msgID>`
Because Sort Keys are sorted chronologically by timestamp, querying `PK = COMMUNITY#<id>` retrieves the latest community feed with simple backward pagination using `ScanIndexForward = false`.

---

### Q23. What is Pagination in DynamoDB, and what is `LastEvaluatedKey`?

**Answer:**
DynamoDB limits query responses to a maximum of **1 MB** of data or the configured `Limit` parameter. If more matching items exist, the response includes a `LastEvaluatedKey`.
To fetch the next page, the client passes this key back as `ExclusiveStartKey` in the subsequent query. When `LastEvaluatedKey` is nil, pagination is complete.

---

### Q24. How does Onusly ensure strict owner isolation in DynamoDB?

**Answer:**
Because every user-owned entity uses `PK = USER#<ownerID>`, all queries executed by user endpoints bind the partition key to the authenticated user's ID extracted from the validated JWT claims. A user cannot query another user's partition, providing mathematical tenant isolation at the primary key level.

---

### Q25. What is AttributeValue in the AWS SDK for Go v2?

**Answer:**
DynamoDB does not use raw JSON internally; it uses typed attribute descriptors (e.g., `{"S": "hello"}`, `{"N": "42"}`, `{"BOOL": true}`).
The Go SDK provides `attributevalue.MarshalMap(goStruct)` and `attributevalue.UnmarshalMap(avMap, &goStruct)` to serialize and deserialize Go structs to and from DynamoDB attribute value maps automatically using struct tags (`dynamodbav:"fieldName"`).

---

### Q26. What is Optimistic Locking, and how does it compare to Pessimistic Locking in DynamoDB?

**Answer:**
- **Pessimistic Locking**: Locks a record on read so nobody else can touch it until the lock is released (not supported in DynamoDB, causes massive latency and deadlocks).
- **Optimistic Locking**: Allows concurrent reads and checks that the record has not changed before committing a write, using a version number or state attribute (`ConditionExpression: version = :currentVersion`). If changed, the write aborts and the client retries.

---

### Q27. What happens if a DynamoDB transaction encounters an error in one of its operations?

**Answer:**
In a `TransactWriteItems` call, DynamoDB guarantees **ACID atomicity**: if any single operation fails its condition check or experiences an error, all operations in the transaction are cancelled, no changes are committed to disk, and DynamoDB returns a `TransactionCanceledException` detailing the exact failure index.

---

### Q28. How does Onusly handle Goal status transitions (`active -> proof_submitted -> completed`) in DynamoDB?

**Answer:**
Status transitions are enforced via conditional expressions:
1. Submit Proof: `ConditionExpression: #status = :active`. Status becomes `proof_submitted`.
2. Approve Proof: `ConditionExpression: #status = :submitted`. Status becomes `completed`.
3. Reject Proof: `ConditionExpression: #status = :submitted`. Status returns to `active`.
This eliminates invalid state jumps (e.g., approving an already completed goal).

---

### Q29. Can you update an item's Partition Key or Sort Key in DynamoDB?

**Answer:**
**No.** In DynamoDB, the primary key (`PK` and `SK`) is immutable because it dictates physical storage location. To change a primary key, the application must execute a `PutItem` with the new key and a `DeleteItem` on the old key.

---

### Q30. What is an Item Collection in DynamoDB?

**Answer:**
An Item Collection is the set of all items in a table or Local Secondary Index that share the exact same Partition Key. In Onusly, all entities belonging to a single user (profile, goals, proofs, devices) form a single item collection under `PK = USER#<userID>`.

---

### Q31. What is the 10 GB Item Collection limit, and does it apply to Onusly?

**Answer:**
The 10 GB limit per item collection applies **only when a table has one or more Local Secondary Indexes (LSIs)**. Because Onusly uses **Global Secondary Indexes (GSIs)** and no LSIs, this 10 GB limit does not apply, allowing user item collections to grow without restriction.

---

### Q32. Explain DynamoDB's consistency across Global Secondary Indexes.

**Answer:**
GSI updates are **asynchronous and eventually consistent**. When an item is written to the base table, DynamoDB automatically propagates the change to the GSI within milliseconds. However, there is a tiny window where a read on the GSI might not reflect the base table write. Systems must not rely on GSI reads for strict transactional locking.

---

### Q33. How does Onusly retrieve the full submission and proof history of a goal?

**Answer:**
In `internal/proof/repository.go`, Onusly queries the base table using:
`PK = USER#<ownerID> AND SK begins_with PROOF#<goalID>#`.
This retrieves all proof attempts submitted for that specific goal in a single query.

---

### Q34. How are mutual friends queried in Onusly without scanning?

**Answer:**
Mutual friends are stored as `PK = USER#<userID>`, `SK = FRIEND#<friendID>`. To list all friends of a user, the backend performs a Query: `PK = USER#<callerID> AND SK begins_with FRIEND#`. This returns all friend IDs in a single round-trip.

---

### Q35. How does DynamoDB handle data encryption at rest and in transit?

**Answer:**
- **At Rest**: All user data stored in DynamoDB is encrypted by default using AWS KMS (Key Management Service) with 256-bit AES encryption (`AWS_OWNED_KMS_KEY` or customer-managed keys).
- **In Transit**: All API communication between the Onusly Go backend and DynamoDB endpoints is encrypted over TLS 1.3.

---

### Q36. What is Point-in-Time Recovery (PITR) in DynamoDB?

**Answer:**
PITR provides automatic continuous backups of DynamoDB table data. Once enabled, Onusly can restore table data to any single second within the preceding 35 days, protecting against accidental deletions or application corruption bugs.

---

### Q37. How does Onusly ensure that normal goal updates (`PATCH /goals/{id}`) cannot modify restricted fields?

**Answer:**
In `internal/goals/service.go`, the update handler accepts only `Title` and `Description`. The update expression explicitly updates `#title = :title, #desc = :desc`. Fields like `Status`, `OwnerID`, and `ApproverID` are omitted from the expression, making it impossible for a client to tamper with goal state via normal PATCH endpoints.

---

### Q38. What is the difference between `PutItem` and `UpdateItem` in DynamoDB?

**Answer:**
- `PutItem`: Completely overwrites the entire item with the new payload. If attributes existed previously on that item that are not in the new payload, they are erased.
- `UpdateItem`: Performs a partial update or creates the item if missing. It modifies specific attributes, leaves other existing attributes intact, and supports atomic counters (`ADD count :val`).

---

### Q39. How does DynamoDB calculate partition count?

**Answer:**
DynamoDB calculates partitions based on storage and throughput capacity:
$$\text{Partitions by Capacity} = \frac{\text{Total RCU}}{3000} + \frac{\text{Total WCU}}{1000}$$
$$\text{Partitions by Size} = \frac{\text{Total Data Size in GB}}{10}$$
$$\text{Total Partitions} = \max(\text{Partitions by Capacity}, \text{Partitions by Size})$$

---

### Q40. Why is Sort Key prefixing (`STATUS#active#GOAL#123`) a powerful technique?

**Answer:**
Sort Key prefixing enables compound hierarchical querying using `begins_with`. For example, querying `GSI2SK begins_with STATUS#proof_submitted` filters goals strictly by status in the approver's inbox, while preserving the goal ID for uniqueness.

---

### Q41. What is Exponential Backoff and Jitter in DynamoDB SDK calls?

**Answer:**
When DynamoDB returns a `ProvisionedThroughputExceededException` or transient 5xx error, retrying immediately causes thundering herd problems. AWS SDK for Go v2 automatically implements exponential backoff with full jitter: retries wait for an exponentially increasing delay randomized with jitter to distribute load evenly.

---

### Q42. How are device pairing codes generated and verified in DynamoDB?

**Answer:**
The desktop/mobile client calls `/enforcement/pair/request`. The backend generates a cryptographically random 6-digit PIN and stores an item: `PK = PAIR#<code>`, `SK = SESSION` with device metadata and a 10-minute TTL. The user confirms the PIN on their authenticated web dashboard, completing pairing.

---

### Q43. What is an Expression Attribute Name and why is it used?

**Answer:**
DynamoDB reserves hundreds of words (e.g., `status`, `year`, `data`, `time`, `connection`). If an entity contains an attribute named `status`, using it directly in an expression causes a syntax error. Expression Attribute Names alias the reserved word (e.g., `"#status": "status"`), bypassing the restriction.

---

### Q44. What is an Expression Attribute Value?

**Answer:**
Expression Attribute Values act as parameterized variables in DynamoDB queries and condition expressions (e.g., `":expected": {"S": "proof_submitted"}`). They prevent injection attacks and decouple expression logic from literal values.

---

### Q45. How does Onusly measure database latency and errors?

**Answer:**
The Go backend instruments AWS SDK v2 middleware handlers with Prometheus metrics (`/metrics`). Latencies of DynamoDB operations (`PutItem`, `GetItem`, `Query`, `TransactWriteItems`) are tracked in histogram buckets, and error codes are tracked in error counters.

---

### Q46. Can you perform joins in DynamoDB?

**Answer:**
**No.** DynamoDB does not support runtime joins. In Single-Table Design, joins are pre-computed at write time by co-locating related entities within the same Partition Key or duplicating denormalized data where appropriate.

---

### Q47. How does Onusly store target distraction lists on a Goal?

**Answer:**
Targets are stored as a typed List attribute inside the Goal item:
```json
"targets": [
  {"type": "domain", "value": "reddit.com"},
  {"type": "desktop_app", "value": "discord.exe"},
  {"type": "android_package", "value": "com.reddit.frontpage"}
]
```
When the desktop or mobile client syncs blocklists, the backend queries active goals and flattens these target arrays into a cached blocklist response.

---

### Q48. What is the DynamoDB pricing model for storage?

**Answer:**
In AWS, DynamoDB storage is billed per GB-month (first 25 GB free per month under AWS Free Tier, then approximately $0.25 per GB-month). Because Onusly uses single-table design with compact attributes, thousands of users consume only megabytes of storage.

---

### Q49. What happens if a DynamoDB write exceeds the 1 KB boundary for WCU calculation?

**Answer:**
DynamoDB rounds writes up to the nearest 1 KB. A write of 500 bytes consumes 1 WCU. A write of 1,050 bytes crosses the 1 KB boundary and consumes 2 WCUs. Keeping item schemas compact directly halves write costs.

---

### Q50. How would you migrate a DynamoDB table schema if requirements change?

**Answer:**
Because DynamoDB is schema-less (only `PK` and `SK` are rigid), adding new attributes requires zero table alteration. For changes to primary keys or global indexes, a migration script scans items and writes new formatted items, or a new GSI is deployed concurrently without downtime.

---

## Tech Stack 3: React 19, Modern Frontend Web, Vite 8 & Tailwind CSS v4

> **Total Questions**: 50 | **Target Domain**: Core Language, Concurrency, Architecture, Edge Cases & Onusly Implementations

### Q1. What major innovations did React 19 introduce, and how are they used in Onusly?

**Answer:**
React 19 introduces several fundamental architectural advancements:
1. **React Compiler**: Automatically memoizes components and hooks, eliminating the need for manual `useMemo` and `useCallback` boilerplate.
2. **Actions & `useActionState`**: First-class handling of async transitions and mutations, managing pending states, errors, and optimistic rollbacks natively.
3. **`useOptimistic`**: Built-in hook for rendering instant optimistic UI updates during background server requests.
4. **`use` API**: A new primitive allowing conditional reading of Promises and contexts directly in render.
Onusly uses React 19 across its web dashboard for goal creation, instant partner proof approvals, and real-time WebSocket state reconciliation.

---

### Q2. How does React 19's `useOptimistic` hook work, and where is it useful in Onusly?

**Answer:**
`useOptimistic(passthroughState, updateFn)` takes the current server state and a reducer function, returning an optimistic state and a trigger function.
In Onusly, when an approver clicks **Approve**, the UI immediately transitions the goal card to `completed` and unlocks the shield indicator without waiting for network round-trips. If the backend returns `409 Conflict` (e.g., racing approval) or an error, React automatically rolls back the UI to the actual server state.

---

### Q3. Explain `useActionState` in React 19.

**Answer:**
`useActionState(asyncAction, initialState)` replaces messy `useState` + `try/catch` patterns for forms and async mutations. It returns:
`[state, formAction, isPending]`.
When the action runs, `isPending` automatically switches to `true`, and errors or updated server data are set cleanly upon resolution. Onusly uses this for goal creation forms and profile handle updates.

---

### Q4. What is the React Compiler (formerly React Forget) and how does it change component optimization?

**Answer:**
Historically, React developers had to manually wrap functions and calculations in `useCallback` and `useMemo` with dependency arrays to prevent unnecessary re-renders. Mistakes in dependency arrays frequently caused stale closures or memory leaks.
The **React Compiler** is a build-time optimizing compiler that analyzes JavaScript semantics and automatically memoizes values, sub-trees, and closures at the compiler level. Components only re-render if their accessed values actually change, providing fine-grained reactivity without manual hooks.

---

### Q5. Explain how Vite 8 differs from traditional Webpack build systems.

**Answer:**
- **Development Mode**: Webpack bundles the entire application before starting the dev server, which scales poorly ($O(N)$ with codebase size). Vite uses **native browser ES Modules (ESM)**: the dev server starts instantly, and files are transformed on-the-fly when requested by the browser. Hot Module Replacement (HMR) stays lightning fast regardless of project size.
- **Production Mode**: Vite uses Rollup with optimized multi-page bundling, tree-shaking, and CSS code-splitting.

---

### Q6. What is new in Tailwind CSS v4, and how does `@tailwindcss/vite` optimize builds?

**Answer:**
Tailwind CSS v4 introduces the **Oxide engine** written in Rust, replacing PostCSS with a high-performance native pipeline powered by Lightning CSS.
- **Zero-Config CSS-First Setup**: Replaces `tailwind.config.js` with direct CSS `@theme` directives.
- **Full CSS Variables**: Exposes all colors, spacing, and font sizes as native CSS variables.
- **`@tailwindcss/vite` Plugin**: Compiles utility classes in-memory during Vite's transform phase, achieving up to 10x faster build speeds and zero runtime CSS overhead.

---

### Q7. How does Onusly's `WebSocketManager` handle real-time push updates in the frontend?

**Answer:**
In `frontend/src/lib/WebSocketManager.js`, Onusly implements a resilient WebSocket client singleton that:
1. Connects to `wss://<host>/ws` passing the JWT in authentication headers/protocols.
2. Implements **Exponential Backoff Reconnection** with random jitter when connections drop.
3. Dispatches incoming server events (`GOAL_STATUS_CHANGED`, `PROOF_APPROVED`, `NEW_CHAT_MSG`) to registered React event listeners.
4. Sends application-level ping frames to keep intermediary proxy connections alive.

---

### Q8. What is the Virtual DOM, and how does React reconcile changes?

**Answer:**
The Virtual DOM (VDOM) is a lightweight in-memory JavaScript representation of the real DOM tree.
When component state changes, React constructs a new VDOM tree and executes a **Diffing Algorithm** (heuristic $O(N)$ reconciliation):
1. Different element types produce entirely new trees.
2. Keys on list items allow React to match elements across renders without destroying DOM nodes.
3. Only the exact modified attributes or nodes are patched into the real DOM, avoiding expensive browser layout recalculations.

---

### Q9. What is the purpose of React `key` prop in lists, and why is `index` dangerous as a key?

**Answer:**
The `key` prop provides a stable identity for list elements across renders so React's reconciliation engine can track which items are inserted, deleted, or reordered.
**Why `index` is dangerous:** If a list is sorted, filtered, or prepended, the indices shift while keys remain tied to positions rather than items. This causes component state, input focus, or animations to be erroneously preserved on the wrong elements.

---

### Q10. How does Onusly structure client-side routing with React Router v7?

**Answer:**
In `frontend/src/App.jsx`, Onusly uses `react-router-dom` v7 with declarative route definitions:
- Public routes: `/login`
- Protected routes: `/`, `/goals`, `/goals/:id`, `/approvals`, `/communities`, `/settings`
Protected routes are wrapped in an `AuthGuard` component that checks for a valid local JWT; unauthenticated users are redirected to `/login`.

---

### Q11. How does React handle Cross-Site Scripting (XSS) defense?

**Answer:**
By default, React escapes all values embedded in JSX expressions before rendering them to the DOM (treating them as strings rather than executable HTML). This prevents attackers from injecting malicious `<script>` tags.
**The Exception:** `dangerouslySetInnerHTML` bypasses this protection. Onusly strictly avoids `dangerouslySetInnerHTML`, ensuring user-submitted proof text, comments, and handles are rendered safely.

---

### Q12. What are React Server Components (RSC) vs Client Components?

**Answer:**
- **Server Components (RSC)**: Render exclusively on the server and stream serialized UI (not HTML and not JavaScript bundles) to the client. They have direct access to backend resources and zero impact on client bundle size.
- **Client Components (`'use client'`)**: Traditional React components that execute on the browser, supporting client state (`useState`), effects (`useEffect`), browser APIs, and event listeners.

---

### Q13. Explain the concept of Stale Closures in React hooks.

**Answer:**
A stale closure occurs when a hook (like `useEffect` or `useCallback`) captures variables from an earlier render pass because they were omitted from the hook's dependency array.
When the effect or callback runs later, it reads the old captured values instead of the latest component state. In React 19, the React Compiler largely eliminates stale closures by tracking dependencies automatically.

---

### Q14. How does Onusly normalize distraction targets in the frontend before sending them to the API?

**Answer:**
In `frontend/src/lib/targetNormalizer.js`, user inputs are sanitized and classified into typed target objects:
- Strips protocols (`https://`), trailing slashes, and paths for domain targets (`https://www.reddit.com/r/all/` -> `reddit.com`).
- Detects Windows executables (`.exe`) -> `desktop_app`.
- Detects Android package identifiers (`com.example.app`) -> `android_package`.
- Preserves specialized subpaths (e.g., `youtube.com/shorts`).

---

### Q15. What is the difference between Controlled and Uncontrolled Components in React?

**Answer:**
- **Controlled Component**: Form input values are driven directly by React state (`value={state}` and `onChange={(e) => setState(e.target.value)}`). React is the single source of truth.
- **Uncontrolled Component**: Input values are managed by the DOM itself. React accesses values using a `ref` (`inputRef.current.value`) or reads `FormData` upon submission.

---

### Q16. How does `useTransition` work in React, and why is it important for responsive UIs?

**Answer:**
`useTransition` separates state updates into **urgent** (typing in an input, clicking a button) and **non-urgent / transitions** (filtering a large list, loading data).
By wrapping heavy state updates in `startTransition(() => { ... })`, React keeps the browser main thread responsive. If the user clicks or types while a transition is calculating, React yields and prioritizes the user input.

---

### Q17. How does Onusly prevent Cross-Site Request Forgery (CSRF) in its frontend architecture?

**Answer:**
CSRF attacks exploit browser auto-sending of session cookies across origins.
Onusly completely eliminates CSRF vulnerabilities by **not relying on ambient session cookies**. All API calls attach the JWT explicitly in the HTTP `Authorization: Bearer <token>` header via JavaScript. Because browsers do not automatically attach custom headers to cross-origin requests, malicious third-party sites cannot forge authenticated requests.

---

### Q18. What is Tree-Shaking, and how does Vite achieve it?

**Answer:**
Tree-shaking is a dead-code elimination technique that removes unused exported code from the final bundle.
Because ES Modules use static `import` and `export` statements, Vite's bundler (Rollup) can statically analyze the entire dependency graph. If Onusly imports only 5 icons from `lucide-react`, the compiler includes only those 5 icon SVG functions, discarding the remaining hundreds of unused icons.

---

### Q19. Explain how GSAP (GreenSock Animation Platform) is integrated into Onusly's frontend.

**Answer:**
While Tailwind CSS handles structural styling and micro-transitions, Onusly uses **GSAP** for high-performance physics-based animations:
- Unlock particle celebrations and shield dissolution effects upon goal completion.
- Coordinated timeline choreographies on modal entrances.
GSAP runs outside React's render loop, directly mutating DOM node transforms via `requestAnimationFrame` for guaranteed 60+ FPS performance.

---

### Q20. What is the difference between `useEffect` and `useLayoutEffect`?

**Answer:**
- `useEffect`: Runs **asynchronously after the browser paints** the screen. It is suitable for data fetching, subscriptions, and WebSocket setup without blocking visual rendering.
- `useLayoutEffect`: Runs **synchronously after DOM mutations but before the browser paints**. Used when measuring DOM layout (e.g., element height/width) or mutating DOM to prevent visual flickering before the user sees it.

---

### Q21. How does Onusly handle file uploads (PDF/images) in React?

**Answer:**
The proof submission component creates a `FormData` object, appends selected files (`formData.append('files', file)`), and issues a `POST /uploads` request using `fetch` with the `Authorization` header.
Before uploading, client-side checks validate file size (<32 MB per file, <50 MB total) and MIME types (PDF, PNG, JPG, WebP) to provide immediate feedback before transmitting data over the wire.

---

### Q22. What is Code-Splitting in React and how is it implemented with `React.lazy`?

**Answer:**
Code-splitting breaks the application bundle into smaller chunks loaded on-demand, reducing initial page load time.
In Onusly, secondary routes (like `/communities`, `/approvals`, or administrative tools) are loaded via `React.lazy()`:
```jsx
const ApprovalsPage = React.lazy(() => import('./pages/Approvals'));
```
Wrapped in `<Suspense fallback={<Spinner />}>`, the browser downloads the page's JavaScript bundle only when the user navigates to that route.

---

### Q23. What are React Portals and where are they used in Onusly?

**Answer:**
`createPortal(children, domNode)` renders React children into a different part of the DOM tree (outside the parent component's hierarchy), such as directly into `document.body`.
Onusly uses portals for **Modals and Fullscreen Proof Review Dialogs**. This guarantees that parent CSS styles (such as `overflow: hidden`, `z-index`, or CSS `transform`) do not clip or alter the stacking context of the modal.

---

### Q24. Explain the rules of React Hooks and why they exist.

**Answer:**
1. **Only call hooks at the top level**: Do not call hooks inside loops, conditions, or nested functions.
2. **Only call hooks from React function components or custom hooks**.
**Why:** React relies on the exact invocation order of hooks across render cycles to associate hook values with the component's internal fiber node. Conditional hook calls break the array indexing of state, causing state corruption.

---

### Q25. What is React Fiber?

**Answer:**
React Fiber is the complete rewrite of React's reconciliation engine (introduced in React 16). Prior to Fiber, reconciliation was recursive and could not be paused (blocking the browser thread for large updates).
Fiber models every component as a linked-list work unit ('fiber node'). It allows React to **pause, prioritize, and resume rendering work**, giving rise to Concurrent React features like transitions and suspense.

---

### Q26. How does Onusly store authenticated user tokens in the frontend?

**Answer:**
Upon successful login, Onusly stores the JWT in `localStorage` (or memory/sessionStorage) alongside the user's basic profile details.
An API wrapper interceptor retrieves this token on every outbound fetch request, automatically attaching it to the `Authorization` header and handling `401 Unauthorized` responses by redirecting to `/login`.

---

### Q27. How does Vite handle environment variables in frontend code?

**Answer:**
Vite exposes environment variables on the `import.meta.env` object.
Only variables prefixed with `VITE_` (e.g., `VITE_API_URL`, `VITE_GOOGLE_CLIENT_ID`) are statically replaced during build time and exposed to client-side code, preventing sensitive private server secrets from leaking into client bundles.

---

### Q28. What is the difference between shallow rendering and full DOM rendering in frontend testing?

**Answer:**
- **Shallow Rendering**: Renders only the component under test without rendering its child components, isolating unit tests from child component bugs.
- **Full DOM Rendering** (e.g., React Testing Library): Mounts the component and all nested children in a simulated DOM (`jsdom`), testing components as real users interact with them.

---

### Q29. What is Hydration in React, and what causes Hydration Mismatches?

**Answer:**
Hydration is the process where client-side React attaches event listeners and takes over an HTML markup tree that was pre-rendered on the server.
A **Hydration Mismatch** occurs when the HTML generated on the server differs from the initial client render (e.g., rendering current timestamp `Date.now()`, random numbers, or checking browser-only APIs like `window.innerWidth`).

---

### Q30. How does Onusly ensure accessible UI design (a11y) in its frontend components?

**Answer:**
- Using semantic HTML5 elements (`<nav>`, `<main>`, `<button>`, `<dialog>`).
- Explicit `aria-label` tags on icon-only buttons (like Lucide icons).
- Proper form labeling (`<label htmlFor="goal-title">`).
- Full keyboard navigability (managing focus traps in modals and supporting Escape key dismissal).

---

### Q31. What is Prop Drilling and how is it avoided in Onusly?

**Answer:**
Prop drilling occurs when data is passed down through several intermediate components that don't need the data, simply to deliver it to a deeply nested child.
Onusly avoids prop drilling by using React Context (for global Auth state and WebSocket connectivity) and modular compound component architectures.

---

### Q32. How does `useCallback` work and why is its necessity reduced in React 19?

**Answer:**
`useCallback(fn, deps)` returns a memoized version of a callback that only changes if dependencies change, preventing child components from re-rendering if they rely on reference equality.
With the **React Compiler** in React 19, function declarations are automatically memoized at build time, rendering manual `useCallback` largely redundant.

---

### Q33. What is the role of `useRef` and how does it differ from `useState`?

**Answer:**
- `useState`: Storing a new state triggers a component re-render.
- `useRef`: Returns a mutable object `{ current: value }` whose reference persists across renders. Mutating `ref.current` **does not trigger a re-render**. It is used for referencing real DOM elements, storing timer IDs, or tracking previous prop values.

---

### Q34. How does Onusly implement responsive design across mobile and desktop browsers?

**Answer:**
Using Tailwind CSS mobile-first utility breakpoints (`sm:`, `md:`, `lg:`, `xl:`):
- On mobile screens, the navigation switches to a bottom bar, goal cards stack vertically, and review dialogs take full viewport height.
- On desktop screens, a persistent sidebar layout and multi-column grid view are displayed.

---

### Q35. Explain how Vite's Fast Refresh works during development.

**Answer:**
Vite Fast Refresh provides near-instant feedback when editing React components without reloading the entire page. It injects modified component functions directly into the running browser runtime while **preserving component state** in memory.

---

### Q36. What is the difference between SPA (Single Page Application) and MPA (Multi-Page Application)?

**Answer:**
- **MPA**: Every link click or navigation requests a completely new HTML page from the server, causing full page reloads.
- **SPA**: The browser loads a single HTML page initially. Subsequent page transitions are handled client-side by JavaScript (React Router), swapping views dynamically without page reloads, providing fluid desktop-like interactions.

---

### Q37. What is the danger of mutating state directly in React (e.g., `state.push(item)`)?

**Answer:**
React relies on **shallow object reference equality** (`Object.is`) to detect state changes. If an array or object is mutated directly, its memory address remains identical. React concludes that no state change occurred and skips re-rendering, causing UI desynchronization.

---

### Q38. How does Onusly display the real-time 'Coverage' status of a user's devices?

**Answer:**
In `Coverage.jsx`, Onusly queries `/enforcement/devices` and listens to WebSocket heartbeat updates. It renders live status badges for registered Windows PCs, Linux machines, Android phones, and browser extensions, indicating whether shields are active and synchronized.

---

### Q39. What is `React.StrictMode` and what does it do in development?

**Answer:**
`StrictMode` is a development-only tool that highlights potential problems in an application:
- Intentionally **double-invokes component functions, reducers, and `useEffect` hooks** to detect side-effects and cleanup bugs.
- Warns about deprecated APIs or legacy lifecycle methods.

---

### Q40. How are modal dialogs dismissed cleanly using the `Escape` key in React?

**Answer:**
Inside the modal component, a `useEffect` registers a keydown event listener on `window`:
```javascript
useEffect(() => {
  const handleKeyDown = (e) => { if (e.key === 'Escape') onClose(); };
  window.addEventListener('keydown', handleKeyDown);
  return () => window.removeEventListener('keydown', handleKeyDown);
}, [onClose]);
```
Cleaning up the listener in the return function prevents memory leaks.

---

### Q41. What is the difference between `npm`, `yarn`, and `pnpm`?

**Answer:**
- `npm`: Default Node package manager, uses flat `node_modules`.
- `yarn`: Introduced lockfiles and parallel installation for deterministic builds.
- `pnpm`: Uses hard links and symlinks to a global content-addressable store on disk, saving gigabytes of disk space and preventing phantom dependency imports.

---

### Q42. What is an Error Boundary in React?

**Answer:**
An Error Boundary is a React class component that implements `componentDidCatch` or `static getDerivedStateFromError`. It catches JavaScript errors anywhere in its child component tree, logs the error, and displays a fallback UI instead of crashing the entire React app.

---

### Q43. How does Onusly ensure that Google OAuth login works smoothly in React?

**Answer:**
Using `@react-oauth/google`, Onusly embeds Google's official Sign-In button. Upon successful user authentication in Google's popup, the component receives an `id_token` and immediately posts it to Onusly's Go backend (`/auth/google`) for cryptographic verification and session creation.

---

### Q44. What is a Stale-While-Revalidate (SWR) caching strategy?

**Answer:**
A caching pattern where the client first serves cached data immediately (instant rendering), while asynchronously issuing a fetch request to revalidate and update the cache in the background. It provides instant perceived performance.

---

### Q45. Why does Onusly avoid large component libraries like MUI in favor of Tailwind CSS?

**Answer:**
Heavy UI component libraries (like Material UI) introduce massive JavaScript runtime overhead, CSS-in-JS emotion parsing costs, and rigid opinionated designs. Tailwind CSS produces zero runtime overhead, compiles to pure atomic CSS, and allows custom design systems.

---

### Q46. What is the purpose of `useId` hook in React?

**Answer:**
`useId()` generates unique, stable IDs across server and client renders for accessibility attributes (linking `<label>` and `<input>` via `htmlFor`), preventing hydration ID mismatches.

---

### Q47. How does Onusly handle token expiration in the frontend?

**Answer:**
The frontend decodes the JWT's `exp` timestamp to warn the user before expiration, and any API fetch returning `401 Unauthorized` automatically purges local credentials and routes the user to the login view.

---

### Q48. What is the difference between client-side routing and server-side routing?

**Answer:**
- **Server-Side Routing**: The server receives a request for `/goals` and responds with a fresh rendered HTML document.
- **Client-Side Routing**: The browser handles URL changes via the HTML5 History API (`pushState`), and React Router renders the matching component tree without making a new page request.

---

### Q49. What is a Closure in JavaScript?

**Answer:**
A closure is the combination of a function bundled together with references to its surrounding lexical environment. A closure gives an inner function access to an outer function's scope even after the outer function has finished executing.

---

### Q50. How does Onusly ensure high visual performance during rapid WebSocket events?

**Answer:**
By debouncing incoming rapid events and using React 19's concurrent batching, multiple WebSocket state transitions received within a single animation frame are batched together into a single DOM update.

---

## Tech Stack 4: Android (Kotlin API 26-35) Mobile Systems & Zero-Surveillance Privacy

> **Total Questions**: 50 | **Target Domain**: Core Language, Concurrency, Architecture, Edge Cases & Onusly Implementations

### Q1. Why does Onusly choose an Android `AccessibilityService` instead of `UsageStatsManager` for distraction blocking?

**Answer:**
1. **Instant Event-Driven Latency**: `AccessibilityService` intercepts package launches immediately via `TYPE_WINDOW_STATE_CHANGED` events in real-time (sub-10ms). In contrast, `UsageStatsManager` requires periodic polling (e.g., every 1-5 seconds), creating a delayed gap where users can view distracting content before blocking kicks in.
2. **System Kill Resilience**: `AccessibilityService` is treated as a core system accessibility provider by the Android OS; the system aggressively avoids killing it during low-memory conditions compared to standard background services.
3. **Granular Control**: Enables detecting window changes and launching blocking overlays without querying the kernel process table.

---

### Q2. What is Onusly's 'Zero-Surveillance Privacy Model' on Android, and how is it technically enforced in code?

**Answer:**
Standard accessibility blockers often request full screen inspection capabilities, allowing them to read keystrokes, personal messages, and sensitive content.
Onusly strictly enforces Zero-Surveillance by declaring `canRetrieveWindowContent="false"` in its `accessibility_service_config.xml`.
This makes it **technically impossible** for Onusly's code or any compromised sub-module to read window nodes, view hierarchies, typed text, or passwords. The service only receives the top-level package identifier (`event.packageName`), preserving complete user privacy.

---

### Q3. Explain how `OnuslyAccessibilityService.kt` detects blocked packages without entering an infinite loop.

**Answer:**
When a blocked package (e.g., `com.discord`) is brought to the foreground:
1. The service receives `AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED`.
2. The package name is checked against `cachedBlocklist.blockedPackages` in $O(1)$ time via a HashSet.
3. If blocked, it launches `LockActivity` with `FLAG_ACTIVITY_NEW_TASK`.
**Infinite Loop Prevention**: Launching `LockActivity` itself emits a `WINDOW_STATE_CHANGED` event for Onusly's own package (`com.onusly.app`). The service explicitly ignores events originating from its own package or launcher components and applies a timestamp debounce (e.g., 500ms), preventing re-entrant launch loops.

---

### Q4. How is `LockActivity` constructed to prevent users from simply bypassing the block?

**Answer:**
In `LockActivity.kt`, the blocker overlay employs several OS-level safeguards:
1. **Window Flags**: Configures `FLAG_SHOW_WHEN_LOCKED`, `FLAG_DISMISS_KEYGUARD`, and `FLAG_NOT_TOUCH_MODAL`.
2. **Back Button Override**: Overrides `onBackPressedDispatcher` or `onBackPressed()` to immediately return to the Android Home screen (`Intent.ACTION_MAIN, Intent.CATEGORY_HOME`) rather than popping back into the blocked app.
3. **Task Affinity**: Configured with a dedicated task affinity so closing or swiping it in Recents does not reveal the underlying blocked app.

---

### Q5. What is the Last-Known-Good (LKG) Cache in Onusly's Android client?

**Answer:**
If a user enters airplane mode or loses network connectivity, a habit blocker that depends purely on live API calls would fail open (allowing distractions) or fail closed (locking everything).
Onusly implements an **LKG Cache** using `BlocklistCache.kt`: the latest active blocklist is serialized to disk upon every successful sync. When offline, `BlockerEngine` evaluates packages against the LKG cache, ensuring distractions remain actively enforced even with zero network access.

---

### Q6. How does Onusly optimize network and battery usage during blocklist synchronization using ETag and HTTP 304?

**Answer:**
In `EnforcementApiClient.kt`, sync requests include the `If-None-Match: <etag>` HTTP header containing the hash of the local LKG cache.
If the user's active goals and targets have not changed on the server, the Go backend immediately returns **HTTP 304 Not Modified** with zero payload body. The Android client skips disk writes and parsing, dramatically preserving battery and cellular data.

---

### Q7. How are sensitive credentials stored on Android using `EncryptedSharedPreferences`?

**Answer:**
Plain `SharedPreferences` stores keys and values as unencrypted XML in the app sandbox, which is vulnerable on rooted devices.
Onusly uses `EncryptedSharedPreferences` backed by the **Android Keystore System**:
- Keys are deterministically encrypted using AES-256-SIV.
- Values are encrypted using AES-256-GCM.
The cryptographic master key is generated and stored inside hardware-backed storage (TEE or StrongBox), inaccessible to other apps.

---

### Q8. What are Kotlin Coroutines, and how do they differ from Java Threads?

**Answer:**
A Java thread maps directly to an OS kernel thread (1 MB stack, expensive context switching).
A Kotlin coroutine is a **lightweight, user-space execution unit** that executes on top of shared thread pools. Thousands of coroutines can run concurrently on a single thread. When a coroutine executes a `suspend` function (e.g., network I/O or delay), it suspends execution, freeing the underlying thread to do other work until the operation completes, without blocking.

---

### Q9. What is the difference between `Dispatchers.Main`, `Dispatchers.IO`, and `Dispatchers.Default`?

**Answer:**
- **`Dispatchers.Main`**: Bound to the Android UI thread for updating views and user interactions.
- **`Dispatchers.IO`**: Optimized for disk and network I/O, backed by an elastic thread pool (up to 64 threads or core count).
- **`Dispatchers.Default`**: Optimized for CPU-intensive work (sorting, JSON parsing, crypto), backed by a thread pool equal to the number of CPU cores.

---

### Q10. What is `SupervisorJob` and why is it used in `OnuslyAccessibilityService`?

**Answer:**
In standard coroutine hierarchy with a normal `Job`, if any child coroutine fails with an unhandled exception, it cancels its parent and all sibling coroutines.
A `SupervisorJob` isolates child failures: if a child coroutine (e.g., an individual background sync) fails, it does **not** cancel the supervisor or other sibling coroutines, ensuring the accessibility service remains running.

---

### Q11. How does `BootReceiver.kt` ensure continuous protection after a phone reboot?

**Answer:**
When an Android phone reboots, running services are terminated. `BootReceiver` is a `BroadcastReceiver` registered in `AndroidManifest.xml` for `android.intent.action.BOOT_COMPLETED`.
Upon receiving the broadcast, it initializes `BlockerEngine`, loads the LKG cache, and schedules background sync jobs. (If accessibility was enabled prior to reboot, the Android OS automatically restarts the `AccessibilityService`).

---

### Q12. What is the difference between `StateFlow` and `SharedFlow` in Kotlin?

**Answer:**
- **`StateFlow`**: A state-holder observable flow that always has an initial value and emits the current state immediately to new collectors (conflated, acts like LiveData).
- **`SharedFlow`**: A hot broadcast channel that emits events (e.g., one-off navigation events, snackbars) to all active collectors without necessarily retaining a single current value.

---

### Q13. How does Onusly's `InstalledAppsManager.kt` retrieve installed applications for the blocklist picker?

**Answer:**
It queries Android's `PackageManager.getInstalledApplications(PackageManager.GET_META_DATA)`. It filters out system apps (checking `ApplicationInfo.FLAG_SYSTEM`) and retains apps that have a launchable intent (`packageManager.getLaunchIntentForPackage(pkg)`), extracting the app label, package name, and icon for display in `InstalledAppsPickerDialog.kt`.

---

### Q14. What is the `QUERY_ALL_PACKAGES` permission, and why is it needed on Android 11+ (API 30+)?

**Answer:**
Starting with Android 11, Google introduced package visibility filtering: apps can no longer query all installed apps by default.
To display the complete list of target apps for blocking, Onusly declares `QUERY_ALL_PACKAGES` (or package queries in the manifest), allowing the user to select any installed game or social media app for enforcement.

---

### Q15. How does Onusly implement real-time WebSocket communication on Android?

**Answer:**
In `RealtimeWebSocketManager.kt`, Onusly uses **OkHttp's WebSocket** implementation. It maintains a persistent connection to `/ws`, listens to text frame callbacks (`onMessage`), parses JSON events, and updates `BlocklistCache` immediately when an approver approves a goal, unlocking the phone in real time.

---

### Q16. What is OkHttp Interceptor, and how is it used in Onusly?

**Answer:**
An Interceptor intercepts, mutates, or monitors HTTP requests and responses. Onusly uses an **Auth Interceptor** that automatically injects the `Authorization: Bearer <jwt>` header into every outgoing request and handles global 401 token refresh/logout.

---

### Q17. How does Android's Doze Mode affect background execution, and how does Onusly handle it?

**Answer:**
Doze Mode restricts background network access and CPU processing when the device is unplugged, stationary, and screen-off.
Onusly handles this by relying on **FCM (Firebase Cloud Messaging) high-priority messages** to wake the device for critical updates and using `WorkManager` / `JobScheduler` for deferred periodic syncs.

---

### Q18. What is the difference between a `Service`, an `IntentService`, and a `JobIntentService`?

**Answer:**
- `Service`: An application component that runs in the background on the main thread (blocking UI if heavy work is performed).
- `IntentService`: Deprecated service that handles requests on a single background worker thread and terminates itself when idle.
- `JobIntentService` / `WorkManager`: Modern replacements that integrate with Android's `JobScheduler` to respect OS battery and execution constraints.

---

### Q19. Explain Kotlin's Null Safety and the difference between `?.`, `?:`, and `!!`.

**Answer:**
- Safe call (`?.`): Executes method or accesses property only if the reference is non-null; otherwise returns `null`.
- Elvis operator (`?:`): Provides a fallback default value if the left expression evaluates to `null` (`val name = user?.name ?: "Anonymous"`).
- Not-null assertion (`!!`): Forces unwrap of a nullable type to non-null, throwing a `NullPointerException` if the value is `null`. (Strictly avoided in Onusly).

---

### Q20. What is a Sealed Class/Interface in Kotlin and how is it used for goal state modeling?

**Answer:**
A `sealed` hierarchy restricts sub-classing to a known set of types defined in the same package/file. It enables compile-time exhaustive `when` expressions without needing an `else` branch:
```kotlin
sealed interface GoalStatus {
    object Active : GoalStatus
    object ProofSubmitted : GoalStatus
    object Completed : GoalStatus
}
```
If a new status is added, the compiler forces all `when` blocks to handle it.

---

### Q21. How does Onusly unit test Android components using `MockWebServer`?

**Answer:**
`MockWebServer` (from Square) spins up a real in-process HTTP/HTTPS server during unit tests. In `src/test/java/`, Onusly points `EnforcementApiClient` to `mockWebServer.url("/")`, enqueueing mock JSON responses and verifying that headers (e.g., ETag), timeout behaviors, and 304 handling behave correctly without hitting live networks.

---

### Q22. What is the Android Activity Lifecycle, and how does `LockActivity` handle it?

**Answer:**
The lifecycle transitions through: `onCreate()` -> `onStart()` -> `onResume()` (active/interactive) -> `onPause()` -> `onStop()` -> `onDestroy()`.
`LockActivity` initializes views in `onCreate()`, locks window flags in `onResume()`, and ensures that if paused while the blocked app is still in foreground, it re-asserts itself.

---

### Q23. What is the difference between `val` and `var` in Kotlin?

**Answer:**
- `val`: An immutable (read-only) reference. Once initialized, it cannot be reassigned (equivalent to `final` in Java).
- `var`: A mutable reference that can be reassigned to different values of the same type.

---

### Q24. What is the role of `buildConfigField` in `build.gradle.kts`?

**Answer:**
`buildConfigField` injects dynamic constants into the generated `BuildConfig` Java class at compile time (e.g., injecting `GOOGLE_SERVER_CLIENT_ID` from environment variables), ensuring secrets are not hardcoded into source control.

---

### Q25. How does Onusly handle app permissions on Android (API 26 to 35)?

**Answer:**
Standard permissions (Internet, Network State) are granted at install time. Dangerous and special permissions (Notification permission on API 33+, Accessibility Service) require explicit runtime user consent with clear explanatory dialogs.

---

### Q26. What is an Android Fragment, and how does Onusly organize screens with Fragments?

**Answer:**
A `Fragment` represents a modular, reusable portion of an Activity's UI with its own lifecycle. Onusly uses a single-activity architecture (`MainActivity`) hosting fragments: `HomeFragment`, `GoalsFragment`, `ReviewsFragment`, `ChatFragment`, and `SettingsFragment`.

---

### Q27. What is the purpose of `RecyclerView` and `ViewHolder` pattern?

**Answer:**
`RecyclerView` efficiently displays large lists by recycling views that have scrolled off-screen rather than creating new views for every item. The `ViewHolder` caches references to view elements (`findViewById`), eliminating expensive layout traversals during scrolling.

---

### Q28. How does Onusly handle device pairing via 6-digit PIN on Android?

**Answer:**
In `EnforcementApiClient.kt`, the Android app calls `/enforcement/pair/request` to obtain a 6-digit PIN, displays it on screen for the user to confirm on their web dashboard, and polls `/enforcement/pair/exchange` to receive and store the device auth token.

---

### Q29. What is an `Intent` in Android, and what is the difference between Explicit and Implicit Intents?

**Answer:**
- **Explicit Intent**: Specifies the exact component class to launch (`Intent(context, LockActivity::class.java)`).
- **Implicit Intent**: Declares a general action to perform (e.g., `ACTION_VIEW` with a URL), letting the Android OS resolve which app handles it.

---

### Q30. What is ProGuard / R8, and how does it affect release builds?

**Answer:**
R8 is Android's code shrinker and obfuscator. It performs tree-shaking, dead-code removal, name obfuscation (renaming classes to `a.b.c`), and byte-code optimizations, reducing APK size and hindering reverse engineering.

---

### Q31. What is an Android `ViewModel` and why is it lifecycle-aware?

**Answer:**
A `ViewModel` persists data across configuration changes (such as screen rotations) that destroy and recreate Activities. It lives as long as the parent Activity/Fragment lifecycle scope exists.

---

### Q32. How does Kotlin's `lazy` delegate work?

**Answer:**
`val property by lazy { ... }` computes the value only upon its first access and caches the result for all subsequent calls. By default, it is thread-safe (`LazyThreadSafetyMode.SYNCHRONIZED`).

---

### Q33. What is the difference between `launch` and `async` coroutine builders?

**Answer:**
- `launch`: 'Fire and forget' builder returning a `Job`. Used when no return value is expected.
- `async`: Returns a `Deferred<T>` (a light promise). Used when a result is expected, retrieved by calling `.await()`.

---

### Q34. What is Android's StrictMode and what does it detect?

**Answer:**
`StrictMode` is a developer tool that detects accidental disk I/O or network calls executing on the main UI thread, helping prevent Application Not Responding (ANR) dialogs.

---

### Q35. How does Onusly format and parse JSON payloads on Android?

**Answer:**
Onusly utilizes custom parsers in `BlocklistParser.kt` and `GoalParser.kt` using `org.json.JSONObject` and `JSONArray` with defensive parsing and type checking to prevent crashes from malformed server responses.

---

### Q36. What is an Application Not Responding (ANR) error and how is it prevented?

**Answer:**
An ANR dialog appears if an application blocks the main UI thread for more than 5 seconds (or 10-20 seconds for BroadcastReceivers). Onusly prevents ANRs by offloading all network, file, and database operations to `Dispatchers.IO`.

---

### Q37. How does Android Keystore protect cryptographic keys?

**Answer:**
The Android Keystore lets apps store cryptographic keys in secure hardware (TEE/SE) where key material cannot be extracted from the device's application memory space; cryptographic operations are performed directly inside the hardware enclave.

---

### Q38. What is the difference between `Serializable` and `Parcelable` in Android?

**Answer:**
- `Serializable`: Standard Java interface relying on reflection, which is slow and creates many temporary heap allocations.
- `Parcelable`: Android-specific IPC serialization interface where developers explicitly serialize fields, offering vastly superior speed and efficiency.

---

### Q39. How does Onusly handle Google Sign-In on Android?

**Answer:**
Using `play-services-auth` (`GoogleSignInClient`), it launches the Google account picker, retrieves the Google ID Token (`account.idToken`), and sends it to the Go backend for verification.

---

### Q40. What is a Kotlin Extension Function?

**Answer:**
An extension function allows adding new methods to existing classes without subclassing them (e.g., `fun String.isValidDomain(): Boolean`). Under the hood, the compiler transforms it into a static method.

---

### Q41. How does Onusly detect if the device has an active internet connection?

**Answer:**
Using `ConnectivityManager` and `NetworkCapabilities` (`hasCapability(NET_CAPABILITY_INTERNET)` and `hasCapability(NET_CAPABILITY_VALIDATED)`), providing reliable network checks on modern Android versions.

---

### Q42. What is the role of `OnSharedPreferenceChangeListener` in `OnuslyAccessibilityService`?

**Answer:**
It listens to changes in the encrypted blocklist preferences. When a background sync job updates the local cache, the listener triggers an immediate in-memory reload of `cachedBlocklist` without needing a service restart.

---

### Q43. How does Onusly prevent memory leaks when using Coroutines in Android Activities?

**Answer:**
By using lifecycle-aware coroutine scopes like `lifecycleScope` or `viewLifecycleOwner.lifecycleScope`. Coroutines launched in these scopes are automatically cancelled when the Activity or Fragment view is destroyed.

---

### Q44. What are Coroutine Exception Handlers?

**Answer:**
A `CoroutineExceptionHandler` is an optional element in a coroutine context that catches uncaught exceptions in root coroutines, preventing app crashes from unhandled asynchronous errors.

---

### Q45. How does Onusly handle Android 14 (API 34) Foreground Service type requirements?

**Answer:**
Android 14 requires every foreground service to declare explicit foreground service types (e.g., `dataSync`, `specialUse`) in `AndroidManifest.xml` and provide permission justifications.

---

### Q46. What is Data Binding vs View Binding in Android?

**Answer:**
- **View Binding**: Generates a binding class for each XML layout containing direct, null-safe references to all views with IDs, eliminating `findViewById`.
- **Data Binding**: Includes View Binding features plus allows binding UI variables directly into XML layouts with two-way observables.

---

### Q47. What is the difference between compileSdk, minSdk, and targetSdk?

**Answer:**
- `compileSdk` (35): The Android SDK version used to compile the app.
- `minSdk` (26 / Android 8.0): The minimum OS version required to install and run the app.
- `targetSdk` (35): Informs the OS that the app has been tested against modern platform behaviors, enabling the latest security and privacy restrictions.

---

### Q48. Why does Onusly keep `isMinifyEnabled = false` in debug builds?

**Answer:**
Minification and R8 obfuscation slow down build times and make debugging stack traces difficult. Keeping it false in debug allows fast compilation and clear line numbers in logcat.

---

### Q49. How does Onusly test offline enforcement in Android unit tests?

**Answer:**
Unit tests simulate network disconnection by shutting down `MockWebServer` or throwing `IOException`, asserting that `BlockerEngine` still evaluates blocked packages correctly against `BlocklistCache`.

---

### Q50. What is the Android back stack and how does Onusly manage it during blocking?

**Answer:**
The back stack tracks user navigation history across activities. When launching `LockActivity`, using flags like `FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS` ensures the blocker overlay does not leave leftover artifacts in the Android multitasking view.

---

## Tech Stack 5: Desktop Systems (Windows/Linux) & Chromium Extension (Manifest V3)

> **Total Questions**: 50 | **Target Domain**: Core Language, Concurrency, Architecture, Edge Cases & Onusly Implementations

### Q1. How is the Windows Desktop Agent architected, and why does it run as a Windows Service under `NT AUTHORITY\LocalService`?

**Answer:**
The Windows agent runs as a native system service managed by the Windows Service Control Manager (SCM):
1. **Continuous Background Execution**: Starts automatically at boot before user login and remains running even when users log off or switch sessions.
2. **Privilege Isolation & Least Privilege**: Instead of running as `NT AUTHORITY\SYSTEM` (which would pose severe privilege escalation risks if compromised), it executes under `NT AUTHORITY\LocalService`. This account has sufficient privileges to bind local sockets (loopback DNS sinkhole) and open named pipes, while lacking access to other user profiles or arbitrary network machines.

---

### Q2. How does Inter-Process Communication (IPC) work between the Windows Desktop Agent and user applications?

**Answer:**
The agent creates a **Windows Named Pipe** at `\\.\pipe\OnuslyAgentIPC`.
The pipe is configured with a restrictive **Discretionary Access Control List (DACL)** granting access only to the local service and the interactive user desktop session. The browser native-messaging host and CLI utilities connect to this named pipe to query agent health, trigger manual blocklist syncs, or retrieve device status.

---

### Q3. How does the Linux Desktop Agent differ from the Windows Agent in terms of IPC and service management?

**Answer:**
On Linux, the agent runs as a **systemd daemon** (`onusly.service`).
Instead of Windows Named Pipes, it communicates via a **UNIX Domain Socket** at `/run/onusly/onusly.sock` with file system permissions restricted to mode `0660` under a dedicated system group. For credential security, Linux uses the kernel keyring (`keyutils`) rather than Windows DPAPI.

---

### Q4. How does Onusly protect stored tokens and credentials on Windows using DPAPI?

**Answer:**
Onusly uses the **Windows Data Protection API (DPAPI)** via `CryptProtectData` and `CryptUnprotectData` system calls.
DPAPI uses keys derived from the user's or machine's cryptographic credentials (bound to the Windows hardware/TPM). Stored tokens in `config.json` cannot be decrypted on another machine or by an unauthorized process without access to the DPAPI context.

---

### Q5. How does Onusly's Loopback DNS Sinkhole work to block distracting domains on Windows?

**Answer:**
The desktop agent listens for UDP/TCP DNS queries on `127.0.0.1:53`.
When DNS sinkhole protection is enabled, the agent configures local network adapters to use `127.0.0.1` as their primary DNS resolver. When a client application or browser resolves a blocked domain (e.g., `reddit.com`), the sinkhole resolver intercepts the query and returns `127.0.0.1` or `NXDOMAIN`, preventing network connections at the operating system socket layer.

---

### Q6. How does Onusly prevent users from tampering with or killing the desktop agent process?

**Answer:**
1. **Service ACLs**: Standard non-administrator user accounts cannot stop or terminate services managed by SCM (`SERVICE_STOP` denied).
2. **Browser Disconnect Lockdown**: The browser extension maintains a continuous Native Messaging IPC connection to the desktop agent. If the agent process is terminated, the pipe breaks; the extension detects the disconnect and triggers an instant fail-closed lockdown.
3. **Watchdog & Auto-Restart**: The service is configured in SCM with `RestartService` recovery actions to immediately respawn if killed.

---

### Q7. What is the difference between Manifest V2 and Manifest V3 in Chrome Extensions?

**Answer:**
- **Manifest V2 (MV2)**: Allowed persistent background pages (running JavaScript continuously in memory) and blocking network requests synchronously via `webRequestBlocking`.
- **Manifest V3 (MV3)**: Replaces persistent background pages with **ephemeral Service Workers** (shut down after 30 seconds of inactivity). Replaces synchronous `webRequestBlocking` with declarative blocking (`declarativeNetRequest`), enhancing browser performance, battery efficiency, and security.

---

### Q8. Explain the Chrome Extension Service Worker lifecycle in Manifest V3.

**Answer:**
An MV3 service worker is completely event-driven:
1. It **spins up** when an event occurs (e.g., web navigation, alarm, message from native host or UI).
2. It processes the event asynchronously.
3. If no further events occur within 30 seconds, Chrome **suspends and terminates** the service worker to conserve system RAM.
4. Any in-memory variables are destroyed; all persistent state must be saved in `chrome.storage.local`.

---

### Q9. How does Onusly maintain WebSocket or long-lived communication in an MV3 Service Worker?

**Answer:**
Because MV3 terminates idle service workers, Onusly uses `chrome.alarms` to schedule periodic wakeups (heartbeats), and connects to the native messaging host. While a native messaging port or active user tab is open, Chrome extends the service worker lifetime.

---

### Q10. Explain the Native Messaging protocol between the Chrome Extension and the Go Native Host.

**Answer:**
Communication between Chrome and the native binary (`native-host.exe`) occurs over standard I/O (`stdin` and `stdout`):
Each message is serialized as UTF-8 JSON preceded by a **32-bit (4-byte) integer in native byte order** specifying the exact length of the message payload:
$$\text{[4-byte length prefix]} + \text{[JSON payload]}$$
Chrome and the Go native host read and write these length-prefixed frames to exchange commands and blocklist updates.

---

### Q11. How is the Native Messaging Host registered on Windows and Linux?

**Answer:**
- **Windows**: Registered in the Windows Registry under `HKCU\Software\Google\Chrome\NativeMessagingHosts\com.onusly.nativehost` pointing to the JSON manifest path.
- **Linux**: Registered by placing the manifest file in `~/.config/google-chrome/NativeMessagingHosts/com.onusly.nativehost.json`.
The manifest defines `name`, `type: "stdio"`, `path` to the binary, and `allowed_origins` with the extension ID.

---

### Q12. What is `declarativeNetRequest` and how does Onusly use it?

**Answer:**
`declarativeNetRequest` lets extensions define rules for blocking or modifying network requests without Chrome having to pass every request to JavaScript.
Rules specify `condition` (urlFilter, domains, resourceTypes) and `action: { type: "block" }`. Chrome evaluates rules directly in C++ native code before issuing network requests, yielding zero latency overhead.

---

### Q13. How does Onusly block YouTube Shorts while allowing educational YouTube videos?

**Answer:**
In `extension/src/youtubeShortsContent.js` and `domainMatcher.js`:
1. YouTube is a Single Page Application (SPA); clicking a video changes the URL via HTML5 History API without triggering a full page reload.
2. A Content Script observes DOM mutations and `yt-navigate-finish` custom events.
3. If the URL matches `youtube.com/shorts/*`, it immediately pauses playback, removes the shorts feed DOM container, and redirects to `blocked.html` or displays an overlay.
4. Standard watch URLs (`youtube.com/watch?v=...`) remain untouched.

---

### Q14. What is the difference between Content Scripts and Background Service Workers in extensions?

**Answer:**
- **Content Scripts**: Injected directly into the DOM of web pages. They can inspect and mutate the page DOM, listen to page events, but cannot access most `chrome.*` APIs (except runtime messaging).
- **Background Service Worker**: Executes in an isolated extension background process. Has access to all `chrome.*` APIs (storage, tabs, declarativeNetRequest, native messaging), but has **no access to web page DOMs**.

---

### Q15. What are Isolated Worlds in browser extensions?

**Answer:**
Chrome executes Content Scripts in an 'Isolated World':
The content script shares the same DOM tree with the host web page, but has completely separate JavaScript execution contexts, prototypes, and global variables (`window`). This prevents malicious web scripts from accessing extension variables or monkey-patching extension methods.

---

### Q16. How does `domainMatcher.js` handle wildcard subdomains (e.g., `*.reddit.com`)?

**Answer:**
In `domainMatcher.js`, target domains are normalized. When evaluating a URL hostname (e.g., `old.reddit.com` or `chat.reddit.com`):
It checks for exact matches or matches where the hostname ends with `.` + target domain (e.g., `.reddit.com`). This ensures subdomains are covered while preventing false positives on sibling domains (e.g., `notreddit.com`).

---

### Q17. How does `chrome.storage.local` differ from `window.localStorage`?

**Answer:**
- `window.localStorage`: Synchronous, blocking, limited to 5 MB per origin, inaccessible to Service Workers in MV3.
- `chrome.storage.local`: Asynchronous, non-blocking, accessible from both Content Scripts and Service Workers, supports complex JavaScript objects, and has a significantly higher storage quota (unlimited with permission).

---

### Q18. What happens when a user attempts to uninstall or disable the extension in enterprise deployments?

**Answer:**
In managed/enterprise environments, Onusly can be deployed via the Windows Registry `ExtensionInstallForcelist`:
`Software\Policies\Google\Chrome\ExtensionInstallForcelist`.
Chrome marks the extension as 'Installed by Enterprise Policy': the user cannot disable, toggle, or uninstall the extension through `chrome://extensions`.

---

### Q19. Explain the Ed25519 cryptographic signature verification in Onusly's update manifests.

**Answer:**
When desktop agents or browser extensions check for software updates via `updates.xml`:
The manifest includes an Ed25519 cryptographic signature of the new binary. The client verifies the signature using the hardcoded public key before downloading or applying the binary, ensuring attackers cannot spoof update feeds or execute remote code.

---

### Q20. What is Anti-Rollback Protection in desktop updates?

**Answer:**
An attacker might attempt to force an agent to downgrade to an older version of Onusly that contains a known vulnerability.
Onusly implements **Anti-Rollback Protection**: client binaries enforce a monotonic semantic version check. Any update manifest containing a version lower than or equal to the currently running version is rejected.

---

### Q21. How does the Windows installer (`installer.exe`) achieve 1-click installation and automated pairing?

**Answer:**
In `desktop/installer/windows/install.ps1` and `main.go`:
1. It copies binaries to `%ProgramFiles%\Onusly`.
2. Creates and configures the Windows Service (`OnuslyDesktopAgent`).
3. Registers the Native Messaging Host registry keys for Chrome and Edge.
4. Reads an embedded pairing token or prompts for a 6-digit PIN to establish authenticated pairing with the backend in a single automated step.

---

### Q22. How does Onusly handle desktop app blocking on Windows (e.g., `discord.exe`, `steam.exe`)?

**Answer:**
In `desktop/agent/internal/apps`, the agent periodically enumerates running processes using Windows APIs (`CreateToolhelp32Snapshot`, `Process32First`, `Process32Next`).
If a process executable matches a blocked target (`discord.exe`), the agent terminates the process handle using `TerminateProcess` or suppresses foreground window focus.

---

### Q23. How does Onusly handle desktop app blocking on Linux?

**Answer:**
On Linux, the agent scans `/proc/[pid]/comm` and `/proc/[pid]/cmdline` to identify running binaries.
When a process matches a blocked application (e.g., `steam` or `slack`), it sends a `SIGTERM` or `SIGKILL` signal via the `kill()` syscall.

---

### Q24. What is `blocked.html` in the extension and how is it styled?

**Answer:**
`blocked.html` is the interstitial page shown when a user tries to access a restricted website.
It displays a clean, calming UI explaining that the site is locked by an active Onusly goal, shows the goal title and assigned approver, and provides a direct button to return to the Onusly dashboard.

---

### Q25. How does the extension communicate with the Onusly web dashboard (`dashboardBridge.js`)?

**Answer:**
When the user visits `app.onusly.com`, `dashboardBridge.js` content script communicates with the dashboard page via `window.postMessage`. It shares extension pairing status and version info, allowing the dashboard to display the active extension shield icon.

---

### Q26. What permissions does Onusly require in `manifest.json` and why?

**Answer:**
- `declarativeNetRequest`: To block network requests without latency.
- `storage`: To persist cached blocklists and user pairing tokens.
- `alarms`: To trigger periodic sync wakeups in the service worker.
- `nativeMessaging`: To communicate with the local desktop agent.
- `host_permissions` (`<all_urls>`): To inspect URLs and enforce domain restrictions.

---

### Q27. What is the difference between `webNavigation.onBeforeNavigate` and `webRequest.onBeforeRequest`?

**Answer:**
- `webNavigation.onBeforeNavigate`: Fires when the browser is about to navigate a tab to a new URL (top-level or iframe).
- `webRequest.onBeforeRequest`: Fires for every individual HTTP resource request (images, scripts, CSS, XHR) made by a page.

---

### Q28. How does Onusly's native host handle graceful pipe disconnection?

**Answer:**
In `desktop/native-host/dial_windows.go`, when the named pipe is closed or the agent stops, reading returns `io.EOF` or `ERROR_BROKEN_PIPE`. The native host detects the disconnection, exits cleanly, and informs the browser extension.

---

### Q29. What is CSP (Content Security Policy) inside a Chrome Extension?

**Answer:**
Chrome Manifest V3 enforces a strict CSP on extension pages: inline scripts (`<script>...</script>`) and `eval()` are strictly forbidden. All JavaScript must be loaded from external `.js` files within the extension bundle.

---

### Q30. How does Onusly support Microsoft Edge alongside Google Chrome?

**Answer:**
Microsoft Edge is built on Chromium and shares identical Manifest V3 extension APIs. Onusly registers Native Messaging Host keys in both Chrome and Edge registry paths (`Software\Microsoft\Edge\NativeMessagingHosts`), allowing seamless multi-browser support.

---

### Q31. What is the role of Mozilla Firefox WebExtensions support in Onusly?

**Answer:**
Firefox uses standard WebExtensions APIs with minor differences (e.g., supporting background scripts via `scripts` in manifest or event pages). Onusly's build script packages cross-browser manifests compatible with both Chromium and Firefox.

---

### Q32. How does Onusly handle DNS cache flushing on Windows when enabling/disabling sinkhole?

**Answer:**
When the agent toggles the DNS sinkhole, Windows may have cached previous DNS resolutions. The agent invokes `DnsFlushResolverCache` from `dnsapi.dll` (or executes `ipconfig /flushdns`) to ensure stale DNS lookups do not bypass the sinkhole.

---

### Q33. Explain how the extension verifies authentication handoff from the web dashboard.

**Answer:**
In `authHandoff.test.js`, when a user logs in on the web dashboard, the dashboard sends a secure message to the extension containing the paired session token. The extension verifies the token structure and stores it in `chrome.storage.local`.

---

### Q34. What is the security risk of `chrome.tabs.executeScript` and why is it deprecated in MV3?

**Answer:**
`executeScript` allowed executing arbitrary string code in page contexts, which could lead to remote code injection vulnerabilities. In MV3, it is replaced by `chrome.scripting.executeScript`, which requires specifying static script files.

---

### Q35. How does Onusly handle extension icon badge updates?

**Answer:**
When shields are active, the extension updates its toolbar icon badge via `chrome.action.setBadgeText({ text: 'ON' })` and sets a bright green/blue color to indicate active distraction protection.

---

### Q36. What is the difference between `HKLM` and `HKCU` in the Windows Registry?

**Answer:**
- `HKLM` (HKEY_LOCAL_MACHINE): Machine-wide settings affecting all users, requires Administrator privileges to modify.
- `HKCU` (HKEY_CURRENT_USER): Settings specific to the currently logged-in user profile, modifiable by standard user processes without elevation.

---

### Q37. How does Onusly manage native messaging manifest file security?

**Answer:**
The native messaging host manifest is written with file system permissions restricted so that non-privileged processes cannot alter the executable path to point to arbitrary malware.

---

### Q38. What is a Service Control Handler in a Go Windows service?

**Answer:**
In `golang.org/x/sys/windows/svc`, the service implements the `Handler` interface (`Execute`). SCM sends control commands (`svc.Stop`, `svc.Pause`, `svc.Shutdown`), which the Go handler translates into context cancellations.

---

### Q39. How are logs structured in the desktop agent?

**Answer:**
Logs are written to `%ProgramData%\Onusly\logs` on Windows and `/var/log/onusly` on Linux, rotating daily to prevent unbounded disk growth.

---

### Q40. What is the role of `syncService.js` in the extension?

**Answer:**
It coordinates background sync intervals, fetches the latest blocklist from the backend API, compares ETags, and updates declarativeNetRequest rules dynamically.

---

### Q41. How does Onusly prevent DNS sinkhole loops if the agent makes outbound API requests?

**Answer:**
The Onusly API domain itself is strictly excluded from the sinkhole list, ensuring the agent can always reach the backend to fetch goal updates and report heartbeats.

---

### Q42. What is the native byte order in Native Messaging IPC?

**Answer:**
Native byte order refers to the endianness of the host CPU (little-endian on x86/ARM). The 4-byte length integer must be encoded in the CPU's native endian format.

---

### Q43. How does Onusly test extension blocking rules in automated CI?

**Answer:**
Using **Puppeteer Core** (`scratch/test_puppeteer.js`), the CI pipeline launches a headless Chromium instance with the unpacked Onusly extension loaded, navigates to blocked websites, and asserts that redirection to `blocked.html` occurs.

---

### Q44. What happens if a user opens an incognito window?

**Answer:**
By default, extensions are disabled in incognito unless explicitly enabled by the user in `chrome://extensions`. Enterprise policies can force extensions to run in incognito mode automatically (`incognito: "spanning"`).

---

### Q45. How does Onusly detect if Chrome has terminated the Native Messaging host?

**Answer:**
The Go native host detects that `os.Stdin` has closed (returns `io.EOF`). When `stdin` closes, the process exits cleanly.

---

### Q46. What is a CRX file?

**Answer:**
A CRX file is a signed ZIP archive containing the Chrome extension files (HTML, JS, CSS, icons, manifest), prefixed with a proprietary header containing the public key and signature.

---

### Q47. How does Onusly distribute enterprise CRX extension updates locally?

**Answer:**
The desktop server hosts a local HTTP endpoint on `127.0.0.1:8765/downloads/extension.crx` and serves an `updates.xml` manifest pointed to by enterprise group policy.

---

### Q48. What is the function of `windows_policy.go`?

**Answer:**
It manages Windows Registry policy keys for Google Chrome and Microsoft Edge, enabling automatic extension force-installation and update URL configuration.

---

### Q49. How does Onusly handle multiple logged-in Windows user sessions?

**Answer:**
The agent monitors active console sessions using `WTSRegisterSessionNotification` (`ConsoleSessionMonitor`) to track when users switch accounts or lock/unlock their workstation.

---

### Q50. What is the ultimate fail-safe if the desktop agent encounters a fatal error?

**Answer:**
It logs the error to the system event log, rolls back any temporary DNS redirection, and allows the Windows Service Control Manager to restart the service cleanly.

---

## Tech Stack 6: Cloud Infrastructure, SQS, Nginx Reverse Proxy & DevOps/SRE Security

> **Total Questions**: 50 | **Target Domain**: Core Language, Concurrency, Architecture, Edge Cases & Onusly Implementations

### Q1. Describe Onusly's production cloud infrastructure topology on AWS.

**Answer:**
Onusly's production architecture is deployed on **Amazon Web Services (AWS)**:
- **Compute**: AWS EC2 instance running hardened Amazon Linux 2023.
- **Database**: Amazon DynamoDB Single-Table schema (Pay-Per-Request).
- **Async Messaging**: Amazon SQS (Simple Queue Service) for decoupling notification events.
- **Reverse Proxy**: Nginx listening on ports 80/443, providing SSL termination, HTTP/2, security headers, and WebSocket upgrades.
- **Application Runtime**: Go REST API & WebSocket server managed by systemd (`onusly.service`), listening strictly on `127.0.0.1:8080` (loopback only).
- **Security**: AWS IAM Least Privilege instance profiles without hardcoded API keys.

---

### Q2. Why does Onusly decouple notification processing using Amazon SQS instead of processing inline in the HTTP request?

**Answer:**
1. **HTTP Latency Reduction**: When an approver submits a decision, sending push notifications to multiple mobile devices via Firebase Cloud Messaging (FCM) or sending emails involves external network round-trips (often 500ms - 2s). Inline processing would block the HTTP response.
2. **Fault Tolerance & Resilience**: If FCM or the push provider experiences an outage, inline calls fail. SQS persists the event; workers can retry with exponential backoff without losing notifications.
3. **Traffic Spike Smoothing**: SQS buffers bursts of goal submissions and approvals, allowing background notification workers to consume events at a steady, controlled rate.

---

### Q3. How does the Background Notification Worker consume messages from SQS?

**Answer:**
The worker runs a continuous loop calling `sqs.ReceiveMessage` with **Long Polling** (`WaitTimeSeconds: 20`):
- Long polling keeps the HTTP connection open with SQS until messages arrive, drastically reducing empty API calls and costs.
- The worker parses the event payload (`PROOF_SUBMITTED`, `GOAL_APPROVED`).
- It dispatches push alerts to registered mobile device tokens via FCM HTTP v1 API.
- Once successfully processed, it deletes the message from SQS using `DeleteMessage`.
- If processing fails, the visibility timeout expires, and the message becomes available for redelivery.

---

### Q4. What is the Visibility Timeout in Amazon SQS?

**Answer:**
When a worker receives a message from SQS, the message is not deleted; it enters a **Visibility Timeout** period (e.g., 30 seconds) during which other workers cannot see or receive it.
If the worker finishes processing and calls `DeleteMessage`, the message is permanently removed. If the worker crashes or fails before the timeout expires, the message becomes visible again for another worker to process, providing **at-least-once delivery**.

---

### Q5. What is a Dead-Letter Queue (DLQ) in SQS, and how does Onusly use it?

**Answer:**
A DLQ isolates 'poison pill' messages (malformed JSON or unhandled exceptions that fail repeatedly).
In SQS, a **Redrive Policy** sets `maxReceiveCount: 5`. If a message fails processing 5 times, SQS moves it to the DLQ. This prevents the worker from getting stuck in an infinite retry loop, and alerts engineers via CloudWatch alarms.

---

### Q6. How is Nginx configured to support both REST endpoints and persistent WebSocket upgrades?

**Answer:**
In `deploy/nginx/onusly.conf`, Nginx maps the HTTP `Upgrade` header:
```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 3600s;
}
```
`proxy_http_version 1.1` and explicit `Upgrade` headers allow HTTP connections to smoothly transition to duplex WebSocket streams, while `proxy_read_timeout 3600s` prevents idle disconnections.

---

### Q7. Why does Onusly enforce loopback binding (`127.0.0.1:8080`) on the Go server in production?

**Answer:**
Binding to `0.0.0.0` exposes the Go server directly on all public network interfaces. An attacker could bypass Nginx's SSL termination, rate limiting, and security headers by sending HTTP requests directly to port 8080.
Binding exclusively to `127.0.0.1` ensures that **only the local Nginx reverse proxy can reach the application**, establishing a Zero-Trust internal network perimeter.

---

### Q8. How does AWS IAM Least Privilege apply to Onusly's EC2 Instance Profile?

**Answer:**
Instead of storing sensitive `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` in environment variables or configuration files, Onusly attaches an **IAM Role to the EC2 instance**.
The role policy (`deploy/aws/ec2_dynamodb_policy.json`) strictly limits actions to:
- DynamoDB: `PutItem`, `GetItem`, `UpdateItem`, `DeleteItem`, `Query`, `TransactWriteItems` on the specific `Onusly` table ARN and its indexes.
- SQS: `SendMessage`, `ReceiveMessage`, `DeleteMessage` on the specific queue ARN.
All other AWS services and resources are denied by default.

---

### Q9. How does Onusly implement Prometheus telemetry and metrics scraping?

**Answer:**
In `internal/metrics`, Onusly initializes a Prometheus registry exposing `GET /metrics`:
- **Counters**: Total HTTP requests by method, route, and status code (`http_requests_total`).
- **Histograms**: Request duration distributions in seconds (`http_request_duration_seconds`).
- **Gauges**: Active WebSocket connections (`websocket_active_connections`) and registered devices.
- **Go Runtime**: Goroutine count, GC pause durations, and heap allocations.
A Prometheus server periodically scrapes this endpoint for monitoring and alerting in Grafana.

---

### Q10. What is the difference between a Counter, Gauge, Histogram, and Summary in Prometheus?

**Answer:**
- **Counter**: A cumulative metric that only increases or resets to zero upon restart (e.g., total requests served).
- **Gauge**: A metric that can arbitrarily go up and down (e.g., current active WebSocket connections, CPU usage).
- **Histogram**: Samples observations (usually request durations or sizes) into configurable buckets and provides cumulative counts and sums, allowing calculation of quantiles (p95, p99) on the Prometheus server.
- **Summary**: Calculates configurable quantiles directly on the client side over a sliding time window.

---

### Q11. Explain how systemd manages Onusly's Go server daemon (`onusly.service`).

**Answer:**
In `deploy/systemd/onusly.service`:
```ini
[Unit]
Description=Onusly Backend API Service
After=network.target

[Service]
Type=simple
User=onusly
Group=onusly
WorkingDirectory=/opt/onusly
ExecStart=/opt/onusly/server
Restart=always
RestartSec=5s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```
Systemd starts the server on boot, runs it under an unprivileged `onusly` user, raises the open file descriptor limit to 65,536 for WebSocket scalability, and restarts the process automatically within 5 seconds if it crashes.

---

### Q12. What is `LimitNOFILE` and why is it crucial for WebSocket servers?

**Answer:**
Every TCP socket and WebSocket connection in Linux consumes a file descriptor. The default Linux user limit is often 1,024.
If a server receives 1,025 concurrent connections, new connections fail with `too many open files`.
Setting `LimitNOFILE=65536` allows the Onusly server to maintain tens of thousands of simultaneous persistent WebSocket connections.

---

### Q13. How is SSL/TLS termination configured with Let's Encrypt and Certbot in Nginx?

**Answer:**
Certbot obtains a free, automated X.509 certificate from the Let's Encrypt Certificate Authority using the ACME protocol (HTTP-01 challenge).
Nginx terminates TLS using modern cipher suites (`TLSv1.2` and `TLSv1.3`), handles the cryptographic handshake, and forwards decrypted HTTP traffic to the Go backend over the internal loopback socket, freeing Go CPU cycles from TLS computations.

---

### Q14. What is HTTP Strict Transport Security (HSTS) and why is it essential?

**Answer:**
HSTS is an HTTP response header: `Strict-Transport-Security: max-age=63072000; includeSubDomains; preload`.
It instructs browsers to strictly interact with the domain exclusively over HTTPS for the next two years. Even if a user types `http://` or clicks an insecure link, the browser automatically rewrites the request to `https://` before sending it, preventing SSL-stripping Man-In-The-Middle (MITM) attacks.

---

### Q15. What is Cross-Origin Resource Sharing (CORS), and how is it secured in Onusly?

**Answer:**
CORS is a browser security mechanism that restricts web pages from making AJAX requests to a different domain than the one that served the page.
In `internal/middleware/cors.go`, Onusly configures allowed origins explicitly (`https://app.onusly.com`). It rejects wildcard origins (`*`) for authenticated endpoints, allows required methods (`GET, POST, PATCH, DELETE`), and validates `Access-Control-Allow-Credentials`.

---

### Q16. How does Firebase Cloud Messaging (FCM) HTTP v1 API work for push notifications?

**Answer:**
The worker authenticates using a Google Service Account JSON key to mint short-lived OAuth 2.0 access tokens.
It sends a `POST` request to `https://fcm.googleapis.com/v1/projects/{project_id}/messages:send` with a structured JSON payload targeting a device registration token. Google's push infrastructure delivers the message to the Android device, waking up the app even in background or low-power modes.

---

### Q17. How does Onusly implement Health Probes (`/health`) for load balancers?

**Answer:**
The Go backend exposes an unauthenticated endpoint `GET /health` returning `200 OK` and `{"status": "ok", "timestamp": "..."}`.
AWS Application Load Balancers (ALB) or external uptime monitors probe this endpoint every 10-30 seconds. If the process freezes or crashes, health checks fail, prompting AWS to mark the instance unhealthy and redirect traffic.

---

### Q18. What is the difference between Blue-Green Deployment and Rolling Deployment?

**Answer:**
- **Rolling Deployment**: Gradually replaces instances of the previous version with the new version across the fleet one by one.
- **Blue-Green Deployment**: Deploys the new version (Green) alongside the existing version (Blue) in a completely separate environment. Once Green passes all smoke tests, traffic is switched instantly at the router/load balancer level, providing zero-downtime cutover and instant rollback if bugs emerge.

---

### Q19. How are database backups managed for DynamoDB in production?

**Answer:**
1. **Point-In-Time Recovery (PITR)**: Provides continuous, automatic per-second recovery for the preceding 35 days.
2. **On-Demand Backups**: Full snapshot backups created prior to major software releases or schema migrations, retained in AWS backup vaults.

---

### Q20. How does AWS Security Group configuration enforce defense-in-depth?

**Answer:**
The EC2 Security Group defines strict inbound firewall rules:
- Port 80 (HTTP): Open to `0.0.0.0/0` (redirects to HTTPS).
- Port 443 (HTTPS): Open to `0.0.0.0/0`.
- Port 22 (SSH): Restricted strictly to the administrator's static bastion IP or accessed via AWS Systems Manager (SSM) Session Manager (zero open SSH ports).
- Port 8080: Completely closed to external traffic.

---

### Q21. What is AWS Systems Manager (SSM) Session Manager and why is it preferred over SSH?

**Answer:**
SSM Session Manager allows managing EC2 instances through a browser shell or AWS CLI without opening port 22 or managing SSH private keys. All shell access is authenticated through AWS IAM, logged in AWS CloudTrail, and session keystrokes are recorded in S3/CloudWatch for auditing.

---

### Q22. How does Onusly manage secret configuration variables in production?

**Answer:**
Production secrets (`JWT_SECRET`, `GOOGLE_CLIENT_ID`, `AWS_REGION`) are never committed to Git.
They are stored in **AWS Secrets Manager** or **AWS Systems Manager Parameter Store** (SecureString with KMS encryption). On boot, the deployment script or instance retrieves and injects these variables into the systemd environment.

---

### Q23. What is the role of GitHub Actions in Onusly's CI/CD pipeline?

**Answer:**
In `.github/workflows/`:
1. **On Pull Request / Push**: Triggers automated workflows that run Go unit tests (`go test -v -race ./...`), Android Gradle unit tests (`./gradlew test`), extension tests, and linting (`golangci-lint`, `eslint`).
2. **On Release Tag**: Compiles cross-platform release binaries (`GOOS=windows`, `GOOS=linux`), packages the extension CRX, generates cryptographic Ed25519 manifests, and publishes releases.

---

### Q24. Explain the difference between synchronous REST calls and asynchronous event publishing in distributed systems.

**Answer:**
- **Synchronous REST**: The client makes a call and blocks waiting for the server to complete all processing and return a response (tight temporal coupling).
- **Asynchronous Event Publishing**: The producer publishes an event to a broker (SQS) and returns immediately. Consumers process the event independently, providing loose coupling, fault isolation, and scalability.

---

### Q25. How does Onusly ensure log rotation to avoid running out of disk space?

**Answer:**
Linux `logrotate` is configured in `/etc/logrotate.d/onusly` and `/etc/logrotate.d/nginx`.
Logs are rotated daily, compressed with gzip (`compress`), and retained for 14-30 days (`rotate 30`), automatically deleting older log archives to protect server storage.

---

### Q26. What is Amazon CloudWatch, and what metrics does it collect for Onusly?

**Answer:**
CloudWatch collects infrastructure telemetry:
- EC2 CPU utilization, network I/O, disk read/write bytes.
- SQS `ApproximateNumberOfMessagesVisible` (queue backlog depth) and `NumberOfMessagesReceived`.
- DynamoDB consumed read/write capacity units and throttled requests.

---

### Q27. How would you set up an alarm for high SQS queue latency?

**Answer:**
Create a CloudWatch Alarm on the SQS metric `ApproximateAgeOfOldestMessage`.
If the oldest message age exceeds a threshold (e.g., >300 seconds), it triggers an Amazon SNS notification that alerts on-call engineers via Slack or PagerDuty, indicating that the notification worker is stalled or crashing.

---

### Q28. What is the difference between TCP and UDP, and where are they used in Onusly?

**Answer:**
- **TCP**: Connection-oriented, reliable, ordered, error-checked stream. Used for Onusly's HTTP REST APIs, WebSocket real-time connections, and DynamoDB/SQS traffic.
- **UDP**: Connectionless, unreliable, low-overhead datagrams. Used by the desktop agent's **DNS Sinkhole** on port 53, where speed and low latency are prioritized for domain name resolutions.

---

### Q29. What is the purpose of Nginx Gzip compression?

**Answer:**
Nginx compresses text-based HTTP responses (HTML, CSS, JS, JSON) using gzip before sending them over the internet (`gzip_types text/plain text/css application/json application/javascript`). This reduces transfer size by up to 70%, decreasing bandwidth costs and improving load times.

---

### Q30. How does Onusly prevent Denial of Service (DoS) attacks on file uploads?

**Answer:**
1. Nginx `client_max_body_size 50M` rejects oversized requests before they reach the backend.
2. Go's `MaxBytesLimiter` caps request body streaming to 50 MB.
3. Per-IP rate limiting prevents automated repeated multi-file POST bursts.

---

### Q31. What is the role of `setup_ec2.sh` in the deployment directory?

**Answer:**
In `deploy/setup_ec2.sh`, Onusly provides an automated provisioning script for Amazon Linux 2023:
- Installs runtime dependencies (Go, Nginx, Git, Certbot).
- Creates system users and directories with proper permissions.
- Installs and enables the `onusly.service` systemd unit.
- Configures Nginx reverse proxy templates.

---

### Q32. What is a 502 Bad Gateway error in Nginx, and what causes it in Onusly?

**Answer:**
A 502 error means Nginx (acting as a reverse proxy) attempted to connect to the backend Go service on `127.0.0.1:8080`, but received an invalid response or connection refusal.
**Common Causes:** The Go server process crashed, failed to start (e.g., config error), or is temporarily unreachable.

---

### Q33. What is a 504 Gateway Timeout error?

**Answer:**
A 504 error occurs when Nginx successfully connects to the Go server, but the backend fails to send an HTTP response within the configured timeout period (`proxy_read_timeout`).

---

### Q34. How does Onusly ensure zero-downtime deployment when updating the backend binary?

**Answer:**
1. Build the new Go binary (`server.new`).
2. Atomically swap the binary file: `mv server.new server`.
3. Reload the systemd service or send a graceful reload signal (`systemctl reload onusly` or `systemctl restart onusly`), allowing existing connections to finish while new requests are immediately routed to the new process.

---

### Q35. What is Semantic Versioning (SemVer), and how does Onusly follow it?

**Answer:**
SemVer uses the format `MAJOR.MINOR.PATCH` (e.g., `v1.0.0-rc1`):
- `MAJOR`: Breaking changes or incompatible API updates.
- `MINOR`: New backwards-compatible functionality.
- `PATCH`: Backwards-compatible bug fixes and security patches.

---

### Q36. What is a CDN (Content Delivery Network), and how could it benefit Onusly?

**Answer:**
A CDN (like AWS CloudFront) caches static assets (frontend HTML, JS bundles, images, icons) on globally distributed edge servers close to users. This reduces latency, offloads origin server bandwidth, and provides DDoS mitigation.

---

### Q37. How does Onusly protect against SQL Injection or NoSQL Injection?

**Answer:**
Because Onusly uses DynamoDB with the AWS SDK v2:
- There is no raw SQL interpreter to inject SQL queries into.
- All database operations use strongly typed SDK methods with parameterized `ExpressionAttributeNames` and `ExpressionAttributeValues`.
User input is treated strictly as literal data attributes and cannot alter query syntax or logic.

---

### Q38. What is a WAF (Web Application Firewall)?

**Answer:**
AWS WAF inspects web traffic before it reaches Nginx or EC2. It blocks malicious traffic matching rules for SQLi, XSS, IP reputation lists, and automated bot scraping.

---

### Q39. How does Onusly handle DNS resolution for its domain name (`onusly.com`)?

**Answer:**
Using **Amazon Route 53**, AWS's highly available DNS web service. Route 53 routes domain queries using A records or Alias records directly to the production EC2 elastic IP or Application Load Balancer.

---

### Q40. What is an Elastic IP (EIP) in AWS?

**Answer:**
An Elastic IP is a static, public IPv4 address allocated to an AWS account that persists independently of EC2 instance lifecycles. If an EC2 instance is replaced or rebooted, the Elastic IP can be instantly reassociated to the new instance without changing public DNS records.

---

### Q41. What is the difference between horizontal and vertical scaling?

**Answer:**
- **Vertical Scaling (Scale Up)**: Adding more CPU, RAM, or storage to an existing server (e.g., upgrading from `t3.medium` to `c6i.2xlarge`). Has physical limits and single points of failure.
- **Horizontal Scaling (Scale Out)**: Adding more server instances behind a load balancer. Provides near-infinite scalability and high availability.

---

### Q42. What is Infrastructure as Code (IaC)?

**Answer:**
IaC manages and provisions cloud infrastructure through machine-readable definition files (such as Terraform, AWS CloudFormation, or AWS CDK) rather than physical hardware configuration or manual web console clicks. It provides version-controlled, reproducible deployments.

---

### Q43. What is the difference between RTO and RPO in disaster recovery?

**Answer:**
- **Recovery Time Objective (RTO)**: The maximum acceptable duration of time that a system can remain down after a disaster before service is restored.
- **Recovery Point Objective (RPO)**: The maximum acceptable age of data that can be lost due to a disaster (e.g., with DynamoDB PITR, RPO is near-zero seconds).

---

### Q44. What is Chaos Engineering, and how does `test_d7_fault_injection.ps1` test resilience?

**Answer:**
Chaos Engineering intentionally injects faults (network loss, process termination, database latency) into a system to verify resilience.
In `desktop/installer/windows/test_d7_fault_injection.ps1`, Onusly runs automated fault injection tests: killing the desktop service, severing internet connections, and verifying that the LKG cache activates and shields remain locked.

---

### Q45. How does Onusly verify that production services are properly configured after deployment?

**Answer:**
In `deploy/verify_production.sh`, an automated verification script runs post-deployment checks:
- Verifies that `127.0.0.1:8080` is listening and not bound to `0.0.0.0`.
- Checks Nginx status and SSL certificate expiration.
- Executes automated health checks (`/health`) and checks Prometheus metrics.

---

### Q46. What is the role of Go's `GOOS` and `GOARCH` environment variables in build automation?

**Answer:**
Go has built-in cross-compilation support:
- `GOOS=windows GOARCH=amd64 go build` compiles Windows 64-bit binaries on any host.
- `GOOS=linux GOARCH=amd64 go build` compiles Linux binaries.
This allows building all production artifacts directly on developer machines or CI servers without native VMs.

---

### Q47. What is the principle of Defense in Depth?

**Answer:**
Defense in depth is an information security strategy that employs multiple layers of defensive controls throughout an IT system. If one security mechanism fails (e.g., browser extension disabled), another mechanism immediately catches the threat (e.g., desktop DNS sinkhole or Android accessibility overlay).

---

### Q48. How does Onusly monitor server disk space and memory usage?

**Answer:**
Using Prometheus Node Exporter or CloudWatch Agent to emit disk usage (`node_filesystem_free_bytes`) and RAM usage, alerting administrators before disks reach 85% capacity.

---

### Q49. What is an SRE Error Budget?

**Answer:**
An Error Budget is the allowable threshold of downtime or unreliability defined by a Service Level Objective (SLO). For a 99.9% availability SLO, the system has an error budget of 0.1% downtime (approx. 43 minutes per month).

---

### Q50. Why is Onusly considered a 'Privacy-First' architecture from a systems and cloud perspective?

**Answer:**
1. **Zero Content Transmission**: Keystrokes, screen recordings, browser histories, and visited URLs are never sent to the cloud backend.
2. **Local Evaluation**: Target distraction evaluation occurs locally on-device (via Android package names, Windows process snapshots, and DNS lookups).
3. **Encrypted at Rest & In Transit**: All stored data uses KMS encryption, and network communication uses TLS 1.3 with strict loopback isolation.

---
