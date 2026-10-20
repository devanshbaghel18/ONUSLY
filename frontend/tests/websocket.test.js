import { describe, it, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { WebSocketManager, ConnectionState } from "../src/lib/websocket.js";

// Mock WebSocket implementation for unit testing connection lifecycle
class MockWebSocket {
  constructor(url) {
    this.url = url;
    this.readyState = 0; // CONNECTING
    this.sentFrames = [];
    this.closed = false;
    this.closeCode = null;
    this.closeReason = null;
    this.onopen = null;
    this.onmessage = null;
    this.onclose = null;
    this.onerror = null;
  }

  send(data) {
    if (this.readyState !== 1) {
      throw new Error("Cannot send on closed socket");
    }
    this.sentFrames.push(data);
  }

  close(code = 1000, reason = "") {
    this.readyState = 3; // CLOSED
    this.closed = true;
    this.closeCode = code;
    this.closeReason = reason;
    if (this.onclose) {
      this.onclose({ code, reason });
    }
  }

  triggerOpen() {
    this.readyState = 1; // OPEN
    if (this.onopen) {
      this.onopen();
    }
  }

  triggerMessage(data) {
    if (this.onmessage) {
      const payload = typeof data === "string" ? data : JSON.stringify(data);
      this.onmessage({ data: payload });
    }
  }

  triggerClose(code = 1000, reason = "") {
    this.readyState = 3;
    this.closed = true;
    if (this.onclose) {
      this.onclose({ code, reason });
    }
  }

  triggerError(err) {
    if (this.onerror) {
      this.onerror(err);
    }
  }
}

describe("WebSocketManager Authentication & Connection State Suite", () => {
  let createdSockets = [];
  let createdManagers = [];
  let mockToken = "valid.jwt.token";
  let mockUser = { id: "user-1", email: "alice@example.com", handle: "alice" };
  let isAuthed = true;

  function createTestManager(overrides = {}) {
    const mgr = new WebSocketManager({
      createSocket: (url) => {
        const sock = new MockWebSocket(url);
        createdSockets.push(sock);
        return sock;
      },
      getWsBaseUrl: () => "ws://localhost:8080",
      getToken: () => mockToken,
      getUser: () => mockUser,
      isAuthenticated: () => isAuthed,
      ...overrides,
    });
    createdManagers.push(mgr);
    return mgr;
  }

  beforeEach(() => {
    createdSockets = [];
    createdManagers = [];
    mockToken = "valid.jwt.token";
    mockUser = { id: "user-1", email: "alice@example.com", handle: "alice" };
    isAuthed = true;
  });

  afterEach(() => {
    for (const mgr of createdManagers) {
      mgr.disconnect();
    }
    createdManagers = [];
    createdSockets = [];
  });

  it("1. No application messages are sent before authentication succeeds", () => {
    const ws = createTestManager();
    ws.connect();

    assert.equal(ws.state, ConnectionState.CONNECTING);
    assert.equal(ws.isConnected, false);
    const sock = createdSockets[0];

    // Attempt to send an application message while connecting
    const queued1 = ws.send({ type: "chat.message", text: "Hello before auth" });
    assert.equal(queued1, false);

    // Socket opens
    sock.triggerOpen();
    assert.equal(ws.state, ConnectionState.AUTHENTICATING);

    // Attempt to send an application message while authenticating
    const queued2 = ws.send({ type: "community.join_room", payload: { communityId: "comm-1" } });
    assert.equal(queued2, false);

    // Check frames sent to the server so far: ONLY the auth frame, NO application messages!
    assert.equal(sock.sentFrames.length, 1);
    const firstFrame = JSON.parse(sock.sentFrames[0]);
    assert.equal(firstFrame.type, "auth");
    assert.equal(firstFrame.token, mockToken);
  });

  it("2. Authentication frame is sent first and URL has no token query parameter", () => {
    const ws = createTestManager();
    ws.connect();

    const sock = createdSockets[0];
    assert.ok(sock.url.endsWith("/ws"));
    assert.equal(sock.url.includes("?token="), false, "Token must NOT be in URL query parameters");

    sock.triggerOpen();
    assert.equal(sock.sentFrames.length, 1);
    const frame = JSON.parse(sock.sentFrames[0]);
    assert.equal(frame.type, "auth");
    assert.equal(frame.token, "valid.jwt.token");
  });

  it("3. Pending room joins and application messages are sent only after auth.success", () => {
    const ws = createTestManager();
    ws.connect();
    const sock = createdSockets[0];

    ws.send({ type: "community.join_room", payload: { communityId: "comm-100" } });
    ws.send({ type: "chat.message", payload: { text: "Verified member" } });

    sock.triggerOpen();
    // Only auth frame sent so far
    assert.equal(sock.sentFrames.length, 1);

    // Server sends auth.success
    sock.triggerMessage({
      type: "auth.success",
      payload: { userId: "user-1", handle: "alice" },
    });

    assert.equal(ws.state, ConnectionState.AUTHENTICATED);
    assert.equal(ws.isConnected, true);

    // Now all pending messages must be flushed in order:
    // Frame 0: auth frame
    // Frame 1: user.online announcement
    // Frame 2: community.join_room
    // Frame 3: chat.message
    assert.equal(sock.sentFrames.length, 4);

    const f1 = JSON.parse(sock.sentFrames[1]);
    assert.equal(f1.type, "user.online");
    assert.equal(f1.payload.handle, "alice");

    const f2 = JSON.parse(sock.sentFrames[2]);
    assert.equal(f2.type, "community.join_room");
    assert.equal(f2.payload.communityId, "comm-100");

    const f3 = JSON.parse(sock.sentFrames[3]);
    assert.equal(f3.type, "chat.message");
    assert.equal(f3.payload.text, "Verified member");
  });

  it("4. Authentication failure prevents queued messages from being sent", () => {
    const ws = createTestManager();
    ws.connect();
    const sock = createdSockets[0];

    ws.send({ type: "chat.message", text: "Secret message" });
    sock.triggerOpen();
    assert.equal(sock.sentFrames.length, 1); // auth frame only

    // Server sends auth.error
    sock.triggerMessage({
      type: "auth.error",
      payload: { message: "token expired" },
    });

    assert.equal(ws.state, ConnectionState.DISCONNECTED);
    assert.equal(ws.isConnected, false);
    assert.equal(sock.closed, true);
    assert.equal(sock.closeCode, 4403);

    // No queued message must ever be sent
    assert.equal(sock.sentFrames.length, 1);
    assert.equal(ws.pendingQueue.length, 0);
  });

  it("5. Connection closure during authentication clears pending operations", () => {
    const ws = createTestManager();
    ws.connect();
    const sock = createdSockets[0];

    ws.send({ type: "chat.message", text: "Pending during handshake" });
    sock.triggerOpen();
    assert.equal(ws.pendingQueue.length, 1);

    // Connection drops before auth.success
    sock.triggerClose(1006, "Connection dropped");

    assert.equal(ws.state, ConnectionState.DISCONNECTED);
    assert.equal(ws.isConnected, false);
    // Pending operations must be discarded to avoid leaking into unknown state
    assert.equal(ws.pendingQueue.length, 0);
  });

  it("6. Reconnection authenticates again before sending application messages", () => {
    const ws = createTestManager();
    ws.connect();
    const sock1 = createdSockets[0];
    sock1.triggerOpen();
    sock1.triggerMessage({ type: "auth.success", payload: { userId: "user-1" } });
    assert.equal(ws.state, ConnectionState.AUTHENTICATED);

    // Disconnect
    sock1.triggerClose(1000, "Normal closure");
    assert.equal(ws.state, ConnectionState.DISCONNECTED);

    // Queue a new message while disconnected
    ws.send({ type: "chat.message", text: "Sent during reconnect" });

    // Connect again (simulating reconnect)
    ws.connect();
    assert.equal(createdSockets.length, 2);
    const sock2 = createdSockets[1];
    assert.equal(ws.state, ConnectionState.CONNECTING);

    // Socket 2 opens -> must send auth frame first
    sock2.triggerOpen();
    assert.equal(ws.state, ConnectionState.AUTHENTICATING);
    assert.equal(sock2.sentFrames.length, 1);
    assert.equal(JSON.parse(sock2.sentFrames[0]).type, "auth");

    // Only after auth.success on socket 2 are messages sent
    sock2.triggerMessage({ type: "auth.success", payload: { userId: "user-1" } });
    assert.equal(ws.state, ConnectionState.AUTHENTICATED);
    assert.equal(sock2.sentFrames.length, 3); // auth, user.online, chat.message
  });

  it("7. A stale socket event cannot authenticate or flush messages for a newer socket", () => {
    const ws = createTestManager();
    ws.connect();
    const sock1 = createdSockets[0];

    // Explicit disconnect increments generation and cancels socket 1
    ws.disconnect();
    assert.equal(ws.state, ConnectionState.DISCONNECTED);

    // Reconnect creates socket 2
    ws.connect();
    const sock2 = createdSockets[1];
    assert.equal(ws.state, ConnectionState.CONNECTING);

    // Socket 1 fires late open and late auth.success
    sock1.triggerOpen();
    sock1.triggerMessage({ type: "auth.success", payload: { userId: "user-1" } });

    // ws must NOT transition to AUTHENTICATED on sock1's stale event
    assert.equal(ws.state, ConnectionState.CONNECTING);
    assert.equal(ws.socket, sock2);

    // Socket 2 opens and authenticates properly
    sock2.triggerOpen();
    assert.equal(ws.state, ConnectionState.AUTHENTICATING);
    sock2.triggerMessage({ type: "auth.success", payload: { userId: "user-1" } });
    assert.equal(ws.state, ConnectionState.AUTHENTICATED);
  });

  it("8. Logout or account switching prevents old-session messages from being sent", () => {
    const ws = createTestManager();
    ws.connect();
    const sock1 = createdSockets[0];

    // Alice queues a message
    ws.send({ type: "chat.message", text: "Alice private message" });
    assert.equal(sock1.sentFrames.length, 0);

    // Alice logs out
    ws.disconnect();
    assert.equal(ws.pendingQueue.length, 0);

    // Bob logs in
    mockUser = { id: "user-2", email: "bob@example.com", handle: "bob" };
    mockToken = "bob.jwt.token";

    ws.connect();
    const sock2 = createdSockets[1];
    sock2.triggerOpen();
    sock2.triggerMessage({ type: "auth.success", payload: { userId: "user-2" } });

    // Bob's connection must NOT send Alice's private message
    const frames = sock2.sentFrames.map((f) => JSON.parse(f));
    assert.ok(frames.every((f) => f?.text !== "Alice private message"));
  });

  it("9. Authentication success is handled exactly once", () => {
    const ws = createTestManager();
    let statusCallCount = 0;
    ws.onStatusChange((status) => {
      if (status) statusCallCount++;
    });

    ws.connect();
    const sock = createdSockets[0];
    sock.triggerOpen();

    ws.send({ type: "chat.message", text: "Once only" });

    // First auth.success
    sock.triggerMessage({ type: "auth.success", payload: { userId: "user-1" } });
    assert.equal(statusCallCount, 1);
    const sentCountFirst = sock.sentFrames.length;

    // Duplicate auth.success from server (e.g. spurious event)
    sock.triggerMessage({ type: "auth.success", payload: { userId: "user-1" } });
    assert.equal(statusCallCount, 1, "Status listeners should not be triggered again");
    assert.equal(sock.sentFrames.length, sentCountFirst, "Queue should not be flushed again");
  });

  it("10. All supported WebSocket clients follow the same protocol", () => {
    const ws = createTestManager();

    // Verify initial state
    assert.equal(ws.state, ConnectionState.DISCONNECTED);
    assert.equal(ws.isConnected, false);

    // Verify unauthenticated send attempts trigger connection but queue safely
    ws.send({ type: "ping" });
    assert.equal(createdSockets.length, 1);
    const sock = createdSockets[0];

    // Query string has no token
    const url = new URL(sock.url);
    assert.equal(url.searchParams.get("token"), null);

    // Verify state progression: CONNECTING -> AUTHENTICATING -> AUTHENTICATED
    sock.triggerOpen();
    assert.equal(ws.state, ConnectionState.AUTHENTICATING);

    sock.triggerMessage({ type: "auth.success", payload: { userId: "user-1" } });
    assert.equal(ws.state, ConnectionState.AUTHENTICATED);
    assert.equal(ws.isConnected, true);

    // Verify direct send while AUTHENTICATED sends immediately
    const directSent = ws.send({ type: "direct.msg" });
    assert.equal(directSent, true);
  });
});
