import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { evaluateUrl } from "../src/blockerEngine.js";
import { createStorage } from "../src/storage.js";
import { createSyncService } from "../src/syncService.js";

describe("Strict Shorts Mode Regression Suite", () => {
  it("1. Goal explicitly blocking youtube.com takes precedence over Shorts mode", () => {
    const goalCache = {
      userId: "user_focus",
      version: "v1-goal-yt",
      targets: [
        {
          goalId: "goal_study",
          goalTitle: "No YouTube At All",
          status: "active",
          domains: ["youtube.com"],
        },
      ],
    };

    // Both /watch and /shorts must be blocked by the goal
    const watchResult = evaluateUrl("https://www.youtube.com/watch?v=123", goalCache);
    assert.equal(watchResult.isBlocked, true);
    assert.equal(watchResult.reason, "goal_target");
    assert.equal(watchResult.matchingTargets[0].goalTitle, "No YouTube At All");

    const shortsResult = evaluateUrl("https://www.youtube.com/shorts/123", goalCache);
    assert.equal(shortsResult.isBlocked, true);
    assert.equal(shortsResult.reason, "goal_target");
    assert.equal(shortsResult.matchingTargets[0].goalTitle, "No YouTube At All");
  });

  it("2. When a goal blocking youtube.com is completed, /watch unblocks while /shorts stays blocked", () => {
    const completedCache = {
      userId: "user_focus",
      version: "v1-goal-done",
      targets: [
        {
          goalId: "goal_study",
          goalTitle: "No YouTube At All",
          status: "completed", // completed goal is excluded from enforcement
          domains: ["youtube.com"],
        },
      ],
    };

    // /watch is unblocked!
    const watchResult = evaluateUrl("https://www.youtube.com/watch?v=123", completedCache);
    assert.equal(watchResult.isBlocked, false);

    // /shorts is still blocked by built-in Shorts enforcement!
    const shortsResult = evaluateUrl("https://www.youtube.com/shorts/123", completedCache);
    assert.equal(shortsResult.isBlocked, true);
    assert.equal(shortsResult.reason, "youtube_shorts");
  });

  it("3. Unrelated goal domain blocking works alongside Shorts mode", () => {
    const redditCache = {
      userId: "user_focus",
      version: "v1-reddit",
      targets: [
        {
          goalId: "goal_reddit",
          goalTitle: "Deep Focus",
          status: "active",
          domains: ["reddit.com"],
        },
      ],
    };

    // Reddit is blocked by goal
    const redditResult = evaluateUrl("https://reddit.com/r/all", redditCache);
    assert.equal(redditResult.isBlocked, true);
    assert.equal(redditResult.reason, "goal_target");

    // YouTube watch is allowed
    const ytWatchResult = evaluateUrl("https://www.youtube.com/watch?v=python_course", redditCache);
    assert.equal(ytWatchResult.isBlocked, false);

    // YouTube Shorts is blocked
    const ytShortsResult = evaluateUrl("https://www.youtube.com/shorts/distraction", redditCache);
    assert.equal(ytShortsResult.isBlocked, true);
    assert.equal(ytShortsResult.reason, "youtube_shorts");
  });

  it("4. Authentication and cache isolation remains intact with Shorts mode", async () => {
    const storage = createStorage();
    await storage.saveSession({ userId: "alice", token: "jwt_alice" });
    await storage.saveBlocklistCache(
      {
        userId: "alice",
        version: "v1-alice",
        targets: [{ goalId: "g1", domains: ["tiktok.com"], status: "active" }],
      },
      "alice"
    );

    // Alice cache is isolated
    const session = await storage.getSession();
    const cache = await storage.getBlocklistCache(session.userId);
    assert.equal(cache.userId, "alice");

    // Bob cannot read Alice's cache
    const bobCache = await storage.getBlocklistCache("bob");
    assert.equal(bobCache, null);

    // Shorts mode works independently for both
    assert.equal(evaluateUrl("https://youtube.com/shorts/test", cache).isBlocked, true);
    assert.equal(evaluateUrl("https://youtube.com/shorts/test", bobCache).isBlocked, true);
    assert.equal(evaluateUrl("https://youtube.com/watch?v=test", cache).isBlocked, false);
  });

  it("5. Offline enforcement continues to preserve cache and block Shorts", async () => {
    const storage = createStorage();
    await storage.saveSession({ userId: "offline_user", token: "jwt_offline" });
    await storage.saveBlocklistCache(
      {
        userId: "offline_user",
        version: "v1-cached",
        targets: [{ goalId: "g1", domains: ["instagram.com"], status: "active" }],
      },
      "offline_user"
    );

    // Mock network failure
    const mockFailingFetch = async () => {
      throw new Error("Offline / DNS failure");
    };
    const sync = createSyncService(storage, mockFailingFetch);
    const syncRes = await sync.sync();

    assert.equal(syncRes.success, false);
    // Cached targets remain intact
    const cachedAfter = await storage.getBlocklistCache("offline_user");
    assert.equal(cachedAfter.targets.length, 1);
    assert.equal(evaluateUrl("https://instagram.com/", cachedAfter).isBlocked, true);

    // Shorts continue to be blocked
    assert.equal(evaluateUrl("https://www.youtube.com/shorts/offline", cachedAfter).isBlocked, true);
  });
});
