import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { createStorage } from "../src/storage.js";
import { createSyncService } from "../src/syncService.js";

describe("Sync Service Suite", () => {
  let storage;

  beforeEach(async () => {
    storage = createStorage();
    await storage.saveSession({
      userId: "user_alice",
      token: "valid_jwt_token",
      email: "alice@example.com",
    });
  });

  it("updates cache on successful 200 sync", async () => {
    const mockFetch = async () => ({
      status: 200,
      async json() {
        return {
          userId: "user_alice",
          version: "v1-newversion",
          generatedAt: "2026-10-09T12:00:00Z",
          targets: [
            { goalId: "g1", goalTitle: "Goal 1", status: "active", domains: ["youtube.com"] },
          ],
        };
      },
    });

    const syncService = createSyncService(storage, mockFetch);
    const result = await syncService.sync();

    assert.equal(result.success, true);
    assert.equal(result.type, "UPDATED");
    assert.equal(result.version, "v1-newversion");
    assert.equal(result.targetsCount, 1);

    const cached = await storage.getBlocklistCache("user_alice");
    assert.equal(cached.version, "v1-newversion");
    assert.equal(cached.targets[0].domains[0], "youtube.com");
  });

  it("preserves cache on 304 Not Modified sync", async () => {
    // Prime cache first
    await storage.saveBlocklistCache(
      {
        userId: "user_alice",
        version: "v1-initial",
        targets: [{ goalId: "g1", goalTitle: "Goal 1", status: "active", domains: ["x.com"] }],
      },
      "user_alice"
    );

    const mockFetch = async (url, options) => {
      assert.equal(options.headers["If-None-Match"], "v1-initial");
      return { status: 304 };
    };

    const syncService = createSyncService(storage, mockFetch);
    const result = await syncService.sync();

    assert.equal(result.success, true);
    assert.equal(result.type, "NOT_MODIFIED");
    assert.equal(result.version, "v1-initial");
    assert.equal(result.targetsCount, 1);

    const cached = await storage.getBlocklistCache("user_alice");
    assert.equal(cached.targets[0].domains[0], "x.com");
  });

  it("CRITICAL: maintains offline cache when network sync fails", async () => {
    // Prime cache with an active block
    await storage.saveBlocklistCache(
      {
        userId: "user_alice",
        version: "v1-offline-safe",
        targets: [
          { goalId: "g1", goalTitle: "Keep Blocked", status: "active", domains: ["youtube.com"] },
        ],
      },
      "user_alice"
    );

    // Network request throws offline error
    const failingFetch = async () => {
      throw new Error("Network offline");
    };

    const syncService = createSyncService(storage, failingFetch);
    const result = await syncService.sync();

    assert.equal(result.success, false);
    assert.equal(result.type, "ERROR");

    // Existing cache MUST STILL BE INTACT
    const cached = await storage.getBlocklistCache("user_alice");
    assert.notEqual(cached, null);
    assert.equal(cached.version, "v1-offline-safe");
    assert.equal(cached.targets.length, 1);
    assert.equal(cached.targets[0].domains[0], "youtube.com");
  });

  it("fails early when unauthenticated", async () => {
    await storage.clearSession();

    const syncService = createSyncService(storage);
    const result = await syncService.sync();

    assert.equal(result.success, false);
    assert.equal(result.type, "UNAUTHENTICATED");
  });
});
