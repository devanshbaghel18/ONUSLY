import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  APP_DOMAINS_MAP,
  evaluateUrl,
  getBlockedDomainsList,
  isTargetShortsRestricted,
} from "../src/blockerEngine.js";

describe("App Domain Expansion and Path-Level Isolation Suite", () => {
  it("blocks https://instagram.com and https://www.instagram.com for app-only target (com.instagram.android)", () => {
    const cache = {
      targets: [
        {
          goalId: "goal_ig",
          status: "active",
          apps: ["com.instagram.android"],
          domains: [],
        },
      ],
    };

    const mainResult = evaluateUrl("https://instagram.com", cache);
    assert.equal(mainResult.isBlocked, true);
    assert.equal(mainResult.reason, "goal_target");

    const wwwResult = evaluateUrl("https://www.instagram.com/p/abc123", cache);
    assert.equal(wwwResult.isBlocked, true);
    assert.equal(wwwResult.reason, "goal_target");
  });

  it("blocks subdomains for app-only targets (e.g. m.instagram.com)", () => {
    const cache = {
      targets: [
        {
          goalId: "goal_ig",
          status: "active",
          apps: ["com.instagram.android"],
        },
      ],
    };

    const mobileResult = evaluateUrl("https://m.instagram.com/explore", cache);
    assert.equal(mobileResult.isBlocked, true);
    assert.equal(mobileResult.reason, "goal_target");
  });

  it("strictly rejects false positives (e.g. notinstagram.com is NOT blocked)", () => {
    const cache = {
      targets: [
        {
          goalId: "goal_ig",
          status: "active",
          apps: ["com.instagram.android"],
        },
      ],
    };

    const falseResult1 = evaluateUrl("https://notinstagram.com", cache);
    assert.equal(falseResult1.isBlocked, false);

    const falseResult2 = evaluateUrl("https://instagram.com.attacker.com", cache);
    assert.equal(falseResult2.isBlocked, false);

    const falseResult3 = evaluateUrl("https://myinstagram.com", cache);
    assert.equal(falseResult3.isBlocked, false);
  });

  it("handles partial domain mappings: a target with domains: ['reddit.com'] AND apps: ['com.instagram.android'] includes BOTH reddit.com and instagram.com in getBlockedDomainsList", () => {
    const cache = {
      targets: [
        {
          goalId: "goal_combo",
          status: "active",
          domains: ["reddit.com"],
          apps: ["com.instagram.android"],
        },
      ],
    };

    const list = getBlockedDomainsList(cache);
    assert.deepEqual(list, ["instagram.com", "reddit.com"]);

    // Both domains evaluate to blocked in evaluateUrl
    const redditEval = evaluateUrl("https://reddit.com/r/technology", cache);
    assert.equal(redditEval.isBlocked, true);
    assert.equal(redditEval.reason, "goal_target");

    const igEval = evaluateUrl("https://instagram.com/explore", cache);
    assert.equal(igEval.isBlocked, true);
    assert.equal(igEval.reason, "goal_target");
  });

  it("ensures YouTube Shorts path-only rule blocks /shorts but allows /watch and normal videos", () => {
    // 1. Target with apps: ["com.google.android.youtube.shorts"]
    const appShortsCache = {
      targets: [
        {
          goalId: "goal_shorts_app",
          status: "active",
          apps: ["com.google.android.youtube.shorts"],
        },
      ],
    };

    assert.equal(isTargetShortsRestricted(appShortsCache.targets[0]), true);

    const appShortsEval = evaluateUrl("https://www.youtube.com/shorts/test_vid_1", appShortsCache);
    assert.equal(appShortsEval.isBlocked, true);
    assert.equal(appShortsEval.reason, "youtube_shorts");

    const appWatchEval = evaluateUrl("https://www.youtube.com/watch?v=regular_vid_1", appShortsCache);
    assert.equal(appWatchEval.isBlocked, false);
    assert.equal(appWatchEval.isShortsRestricted, true);

    // 2. Target with domains: ["shorts.youtube.com"]
    const domShortsCache = {
      targets: [
        {
          goalId: "goal_shorts_dom",
          status: "active",
          domains: ["shorts.youtube.com"],
        },
      ],
    };

    assert.equal(isTargetShortsRestricted(domShortsCache.targets[0]), true);

    const domShortsEval = evaluateUrl("https://www.youtube.com/shorts/test_vid_2", domShortsCache);
    assert.equal(domShortsEval.isBlocked, true);
    assert.equal(domShortsEval.reason, "youtube_shorts");

    const domWatchEval = evaluateUrl("https://www.youtube.com/watch?v=regular_vid_2", domShortsCache);
    assert.equal(domWatchEval.isBlocked, false);
    assert.equal(domWatchEval.isShortsRestricted, true);

    // Direct navigation to shorts.youtube.com is blocked
    const directShortsHostEval = evaluateUrl("https://shorts.youtube.com", domShortsCache);
    assert.equal(directShortsHostEval.isBlocked, true);
  });

  it("ensures standard YouTube target blocks all of YouTube (watch, feed, and shorts)", () => {
    const fullYtCache = {
      targets: [
        {
          goalId: "goal_yt_full",
          status: "active",
          apps: ["com.google.android.youtube"],
        },
      ],
    };

    assert.equal(isTargetShortsRestricted(fullYtCache.targets[0]), true);

    const watchEval = evaluateUrl("https://www.youtube.com/watch?v=xyz", fullYtCache);
    assert.equal(watchEval.isBlocked, true);
    assert.equal(watchEval.reason, "goal_target");

    const shortsEval = evaluateUrl("https://www.youtube.com/shorts/xyz", fullYtCache);
    assert.equal(shortsEval.isBlocked, true);
    assert.equal(shortsEval.reason, "goal_target");

    const youtuBeEval = evaluateUrl("https://youtu.be/xyz", fullYtCache);
    assert.equal(youtuBeEval.isBlocked, true);
    assert.equal(youtuBeEval.reason, "goal_target");
  });

  it("custom unmapped domains remain domain-only and do not map to arbitrary apps", () => {
    const customCache = {
      targets: [
        {
          goalId: "goal_custom",
          status: "active",
          domains: ["news.ycombinator.com"],
          apps: [],
        },
      ],
    };

    const list = getBlockedDomainsList(customCache);
    assert.deepEqual(list, ["news.ycombinator.com"]);

    assert.equal(evaluateUrl("https://news.ycombinator.com/item?id=123", customCache).isBlocked, true);
    assert.equal(evaluateUrl("https://ycombinator.com", customCache).isBlocked, false);
  });

  it("verifies canonical catalog mappings in APP_DOMAINS_MAP", () => {
    assert.deepEqual(APP_DOMAINS_MAP["com.instagram.android"], ["instagram.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.google.android.youtube"], ["youtube.com", "youtu.be"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.google.android.youtube.shorts"], ["shorts.youtube.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.twitter.android"], ["x.com", "twitter.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.reddit.frontpage"], ["reddit.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.netflix.mediaclient"], ["netflix.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.discord"], ["discord.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["tv.twitch.android.app"], ["twitch.tv"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.zhiliaoapp.musically"], ["tiktok.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.ss.android.ugc.trill"], ["tiktok.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.facebook.katana"], ["facebook.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.facebook.orca"], ["facebook.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.snapchat.android"], ["snapchat.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.linkedin.android"], ["linkedin.com"]);
    assert.deepEqual(APP_DOMAINS_MAP["com.pinterest"], ["pinterest.com"]);
  });

  it("enforces canonical app expansion across TikTok, Facebook, and Twitter", () => {
    const multiAppCache = {
      targets: [
        {
          goalId: "g_social",
          status: "active",
          apps: [
            "com.twitter.android",
            "com.zhiliaoapp.musically",
            "com.facebook.katana",
          ],
        },
      ],
    };

    const list = getBlockedDomainsList(multiAppCache);
    assert.deepEqual(list, ["facebook.com", "tiktok.com", "twitter.com", "x.com"]);

    assert.equal(evaluateUrl("https://x.com/home", multiAppCache).isBlocked, true);
    assert.equal(evaluateUrl("https://twitter.com/explore", multiAppCache).isBlocked, true);
    assert.equal(evaluateUrl("https://www.tiktok.com/@creator", multiAppCache).isBlocked, true);
    assert.equal(evaluateUrl("https://m.facebook.com", multiAppCache).isBlocked, true);
  });
});
