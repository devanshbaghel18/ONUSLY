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

  it("2. When a goal blocking YouTube Shorts is active, /watch is allowed while /shorts is blocked", () => {
    const shortsCache = {
      userId: "user_focus",
      version: "v1-goal-shorts",
      targets: [
        {
          goalId: "goal_study",
          goalTitle: "Block Only Shorts",
          status: "active",
          apps: ["com.google.android.youtube.shorts"],
          domains: ["shorts.youtube.com"],
        },
      ],
    };

    // /watch is allowed!
    const watchResult = evaluateUrl("https://www.youtube.com/watch?v=123", shortsCache);
    assert.equal(watchResult.isBlocked, false);
    assert.equal(watchResult.isShortsRestricted, true);

    // /shorts is blocked by goal Shorts restriction!
    const shortsResult = evaluateUrl("https://www.youtube.com/shorts/123", shortsCache);
    assert.equal(shortsResult.isBlocked, true);
    assert.equal(shortsResult.reason, "youtube_shorts");

    // When the goal is completed, /shorts is unblocked
    const completedCache = {
      userId: "user_focus",
      version: "v1-goal-done",
      targets: [
        {
          goalId: "goal_study",
          goalTitle: "Block Only Shorts",
          status: "completed",
          apps: ["com.google.android.youtube.shorts"],
          domains: ["shorts.youtube.com"],
        },
      ],
    };

    const completedShorts = evaluateUrl("https://www.youtube.com/shorts/123", completedCache);
    assert.equal(completedShorts.isBlocked, false);
    assert.equal(completedShorts.isShortsRestricted, false);
  });

  it("3. Unrelated goal domain blocking does NOT block YouTube Shorts", () => {
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

    // YouTube Shorts is NOT blocked when no goal targets it
    const ytShortsResult = evaluateUrl("https://www.youtube.com/shorts/distraction", redditCache);
    assert.equal(ytShortsResult.isBlocked, false);
    assert.equal(ytShortsResult.isShortsRestricted, false);
  });

  it("4. Authentication and cache isolation remains intact with Shorts targeting", async () => {
    const storage = createStorage();
    await storage.saveSession({ userId: "alice", token: "jwt_alice" });
    await storage.saveBlocklistCache(
      {
        userId: "alice",
        version: "v1-alice",
        targets: [{ goalId: "g1", domains: ["shorts.youtube.com"], apps: ["youtube_shorts"], status: "active" }],
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

    // Alice has Shorts blocked
    assert.equal(evaluateUrl("https://youtube.com/shorts/test", cache).isBlocked, true);
    // Bob has NO active goals, so Shorts is NOT blocked
    assert.equal(evaluateUrl("https://youtube.com/shorts/test", bobCache).isBlocked, false);
    // Alice's regular /watch is allowed
    assert.equal(evaluateUrl("https://youtube.com/watch?v=test", cache).isBlocked, false);
  });

  it("5. Offline enforcement continues to preserve cache and block Shorts when targeted", async () => {
    const storage = createStorage();
    await storage.saveSession({ userId: "offline_user", token: "jwt_offline" });
    await storage.saveBlocklistCache(
      {
        userId: "offline_user",
        version: "v1-cached",
        targets: [{ goalId: "g1", domains: ["shorts.youtube.com"], status: "active" }],
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

    // Shorts continues to be blocked from offline cache
    assert.equal(evaluateUrl("https://www.youtube.com/shorts/offline", cachedAfter).isBlocked, true);
    // Regular watch remains unblocked
    assert.equal(evaluateUrl("https://www.youtube.com/watch?v=offline", cachedAfter).isBlocked, false);
  });

  it("6. Unblocked Shorts URL preserves originalUrl and does not redirect to normal YouTube home", () => {
    // Helper replicating unblocked destination calculation from blocked.js
    function computeUnblockedDestination(originalUrl, targetDomain, dashboardUrl, isBlocked) {
      if (isBlocked) return null;
      let destination = dashboardUrl;
      if (originalUrl && !originalUrl.startsWith("chrome-extension://")) {
        destination = originalUrl;
      } else if (targetDomain && !targetDomain.includes(" ") && targetDomain !== "Restricted Website") {
        destination = `https://${targetDomain}/`;
      }
      return destination;
    }

    const shortsUrl = "https://www.youtube.com/shorts/my_short_id";
    const normalYt = "https://www.youtube.com/";
    const emptyCache = { userId: "user1", targets: [] };
    const evalRes = evaluateUrl(shortsUrl, emptyCache);

    assert.equal(evalRes.isBlocked, false);
    const dest = computeUnblockedDestination(shortsUrl, "www.youtube.com", "http://localhost:5173", evalRes.isBlocked);
    assert.equal(dest, shortsUrl, "Must restore the original Shorts URL rather than redirecting to normal YouTube");
    assert.notEqual(dest, normalYt, "Must not redirect to normal YouTube homepage");
  });
});
