import test from "node:test";
import assert from "node:assert/strict";
import { createStorage } from "../src/storage.js";
import { createWellbeingTracker, extractTrackableDomain } from "../src/wellbeingTracker.js";

test("extractTrackableDomain strips paths, queries, www, and internal URLs", () => {
  assert.equal(extractTrackableDomain("https://www.instagram.com/reels/abc?x=1"), "instagram.com");
  assert.equal(extractTrackableDomain("http://docs.github.com:8080/en"), "docs.github.com");
  assert.equal(extractTrackableDomain("chrome://extensions"), null);
  assert.equal(extractTrackableDomain("chrome-extension://abcdef/blocked.html"), null);
  assert.equal(extractTrackableDomain("http://localhost:5173/dashboard"), null);
});

test("wellbeingTracker counts website opens, browse duration, idle pause, and blocked attempts", async () => {
  let fakeNow = new Date("2026-11-26T10:00:00").getTime();
  const storage = createStorage();
  const tracker = createWellbeingTracker({ storage, nowFn: () => fakeNow });

  // Open github.com in tab 1 and spend 120 seconds
  assert.equal(await tracker.recordVisit("https://github.com/trending", 1), true);
  await tracker.recordActiveFocus("https://github.com/trending");
  fakeNow += 2_000;

  // Duplicate reload within 5s debounce should NOT count as second open
  assert.equal(await tracker.recordVisit("https://github.com/trending", 1), false);
  fakeNow += 118_000;

  // Switch focus to youtube.com for 45 seconds
  assert.equal(await tracker.recordVisit("https://www.youtube.com/watch?v=123", 2), true);
  await tracker.recordActiveFocus("https://www.youtube.com/watch?v=123");
  fakeNow += 45_000;

  // User goes idle -> 45s on youtube flushed, next 300s idle ignored
  await tracker.setIdleState("idle");
  fakeNow += 300_000;
  await tracker.setIdleState("active");

  // Spend 15 more seconds on youtube after returning
  fakeNow += 15_000;
  await tracker.flushElapsed();

  // Blocked attempt recorded for instagram.com
  await tracker.recordBlockedAttempt("https://www.instagram.com/");

  const payload = await tracker.buildIngestPayload("ext-dev-1", "chrome", "2026-11-26");
  assert.equal(payload.clientType, "extension");
  assert.equal(payload.totalScreenSeconds, 180);

  const gh = payload.domains.find((d) => d.domain === "github.com");
  const yt = payload.domains.find((d) => d.domain === "youtube.com");
  const ig = payload.domains.find((d) => d.domain === "instagram.com");

  assert.deepEqual(gh, { domain: "github.com", seconds: 120, opens: 1, blockedAttempts: 0 });
  assert.deepEqual(yt, { domain: "youtube.com", seconds: 60, opens: 1, blockedAttempts: 0 });
  assert.deepEqual(ig, { domain: "instagram.com", seconds: 0, opens: 0, blockedAttempts: 1 });
});

test("wellbeingTracker splits elapsed duration across midnight and builds popup today summary", async () => {
  let fakeNow = new Date("2026-11-26T23:59:30").getTime();
  const storage = createStorage();
  const tracker = createWellbeingTracker({ storage, nowFn: () => fakeNow });

  await tracker.recordVisit("https://reddit.com/r/programming", 3);
  await tracker.recordActiveFocus("https://reddit.com/r/programming");
  fakeNow += 90_000; // 30s on Nov 26 + 60s on Nov 27

  const summary = await tracker.getTodaySummary(5);
  assert.equal(summary.date, "2026-11-27");
  assert.equal(summary.totalSeconds, 60);
  assert.equal(summary.topWebsites[0].domain, "reddit.com");

  const nov26 = await tracker.buildIngestPayload("ext-dev-1", "edge", "2026-11-26");
  const nov27 = await tracker.buildIngestPayload("ext-dev-1", "edge", "2026-11-27");
  assert.equal(nov26.platform, "edge");
  assert.equal(nov26.totalScreenSeconds, 30);
  assert.equal(nov27.totalScreenSeconds, 60);
});
