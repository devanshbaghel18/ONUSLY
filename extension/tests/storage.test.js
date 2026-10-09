import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { createStorage } from "../src/storage.js";

describe("Storage & Cache Isolation Suite", () => {
  let storage;

  beforeEach(() => {
    storage = createStorage();
  });

  it("stores and retrieves active session", async () => {
    const session = {
      userId: "user_alice",
      email: "alice@example.com",
      name: "Alice",
      token: "jwt_token_123",
      exp: Math.floor(Date.now() / 1000) + 3600,
    };

    await storage.saveSession(session);
    const retrieved = await storage.getSession();
    assert.deepEqual(retrieved, session);
  });

  it("returns null for expired session", async () => {
    const expiredSession = {
      userId: "user_expired",
      email: "expired@example.com",
      token: "jwt_token_expired",
      exp: Math.floor(Date.now() / 1000) - 100, // expired 100s ago
    };

    await storage.saveSession(expiredSession);
    const retrieved = await storage.getSession();
    assert.equal(retrieved, null);
  });

  it("enforces multi-user cache isolation", async () => {
    const aliceData = {
      userId: "user_alice",
      version: "v1-alice",
      targets: [
        { goalId: "g1", goalTitle: "Alice Goal", status: "active", domains: ["youtube.com"] },
      ],
    };

    // Alice saves cache
    const savedAlice = await storage.saveBlocklistCache(aliceData, "user_alice");
    assert.equal(savedAlice, true);

    // Alice can retrieve her cache
    const aliceCache = await storage.getBlocklistCache("user_alice");
    assert.equal(aliceCache.userId, "user_alice");
    assert.equal(aliceCache.version, "v1-alice");

    // Bob tries to read with his userId -> returns null!
    const bobRead = await storage.getBlocklistCache("user_bob");
    assert.equal(bobRead, null);

    // Bob tries to overwrite Alice's cache -> rejected!
    const bobData = {
      userId: "user_bob",
      version: "v1-bob",
      targets: [
        { goalId: "g2", goalTitle: "Bob Goal", status: "active", domains: ["reddit.com"] },
      ],
    };
    const savedBob = await storage.saveBlocklistCache(bobData, "user_bob");
    assert.equal(savedBob, false);

    // Alice's cache remains untainted
    const aliceCacheAfter = await storage.getBlocklistCache("user_alice");
    assert.equal(aliceCacheAfter.userId, "user_alice");
    assert.equal(aliceCacheAfter.version, "v1-alice");
  });

  it("rejects saving cache when response.userId does not match expectedUserId", async () => {
    const mismatchData = {
      userId: "user_server_id",
      version: "v1-1",
      targets: [],
    };

    const saved = await storage.saveBlocklistCache(mismatchData, "user_client_id");
    assert.equal(saved, false);
  });

  it("persists cache indefinitely across calls (offline resilience)", async () => {
    const cacheData = {
      userId: "user_offline",
      version: "v1-offline",
      targets: [{ goalId: "g1", goalTitle: "Offline Goal", status: "active", domains: ["x.com"] }],
    };

    await storage.saveBlocklistCache(cacheData, "user_offline");

    const cached = await storage.getBlocklistCache("user_offline");
    assert.ok(cached.lastSyncTimestamp > 0);
    assert.equal(cached.version, "v1-offline");
  });

  it("updates lastSyncTimestamp on existing cache", async () => {
    const cacheData = {
      userId: "user_sync",
      version: "v1-sync",
      targets: [],
    };
    await storage.saveBlocklistCache(cacheData, "user_sync");

    const before = await storage.getBlocklistCache("user_sync");
    const newTimestamp = Date.now() + 5000;
    await storage.updateLastSyncTimestamp(newTimestamp);

    const after = await storage.getBlocklistCache("user_sync");
    assert.equal(after.lastSyncTimestamp, newTimestamp);
    assert.equal(after.version, "v1-sync");
  });

  it("clears all session and cache data on explicit clearAll", async () => {
    await storage.saveSession({ userId: "u1", token: "tok" });
    await storage.saveBlocklistCache({ userId: "u1", version: "v1" }, "u1");

    await storage.clearAll();

    assert.equal(await storage.getSession(), null);
    assert.equal(await storage.getBlocklistCache("u1"), null);
  });
});
