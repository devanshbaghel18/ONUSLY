import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { evaluateUrl, getBlockedDomainsList } from "../src/blockerEngine.js";

describe("Blocker Engine Suite", () => {
  const sampleCache = {
    userId: "user_123",
    version: "v1-abcdef0123456789",
    generatedAt: "2026-10-09T12:00:00Z",
    targets: [
      {
        goalId: "goal_focus",
        goalTitle: "Deep Work Sprint",
        status: "active",
        apps: ["com.google.android.youtube"],
        domains: ["youtube.com", "x.com"],
      },
      {
        goalId: "goal_study",
        goalTitle: "Reading & Research",
        status: "proof_submitted",
        apps: ["com.instagram.android"],
        domains: ["instagram.com", "reddit.com"],
      },
      {
        goalId: "goal_done",
        goalTitle: "Completed Workout",
        status: "completed",
        apps: ["com.netflix.mediaclient"],
        domains: ["netflix.com"],
      },
    ],
  };

  it("blocks domains for active goals", () => {
    const result = evaluateUrl("https://youtube.com/watch?v=123", sampleCache);
    assert.equal(result.isBlocked, true);
    assert.equal(result.status, "active");
    assert.equal(result.instruction, "Complete your goal and submit proof.");
    assert.equal(result.matchingTargets.length, 1);
    assert.equal(result.matchingTargets[0].goalTitle, "Deep Work Sprint");
  });

  it("blocks subdomains of active targets", () => {
    const result = evaluateUrl("https://www.youtube.com/", sampleCache);
    assert.equal(result.isBlocked, true);

    const mResult = evaluateUrl("https://m.x.com/home", sampleCache);
    assert.equal(mResult.isBlocked, true);
  });

  it("blocks domains for proof_submitted goals", () => {
    const result = evaluateUrl("https://www.instagram.com/explore", sampleCache);
    assert.equal(result.isBlocked, true);
    assert.equal(result.status, "proof_submitted");
    assert.equal(result.instruction, "Proof submitted. Waiting for review.");
    assert.equal(result.matchingTargets[0].goalTitle, "Reading & Research");
  });

  it("does NOT block completed goals", () => {
    // netflix.com is on goal_done which is completed
    const result = evaluateUrl("https://www.netflix.com/browse", sampleCache);
    assert.equal(result.isBlocked, false);
    assert.equal(result.matchingTargets.length, 0);
  });

  it("does NOT block unrelated domains", () => {
    const result = evaluateUrl("https://github.com/features", sampleCache);
    assert.equal(result.isBlocked, false);
  });

  it("handles multiple goals sharing the same domain", () => {
    const multiGoalCache = {
      userId: "user_multi",
      version: "v1-multi",
      targets: [
        {
          goalId: "g1",
          goalTitle: "Goal One",
          status: "proof_submitted",
          domains: ["reddit.com"],
        },
        {
          goalId: "g2",
          goalTitle: "Goal Two",
          status: "active",
          domains: ["reddit.com"],
        },
      ],
    };

    const result = evaluateUrl("https://reddit.com/r/programming", multiGoalCache);
    assert.equal(result.isBlocked, true);
    assert.equal(result.matchingTargets.length, 2);
    // Since at least one goal is active, the overall instruction is active
    assert.equal(result.status, "active");
    assert.equal(result.instruction, "Complete your goal and submit proof.");
  });

  it("shows proof_submitted instruction only when all sharing goals are in review", () => {
    const allReviewCache = {
      userId: "user_review",
      version: "v1-review",
      targets: [
        {
          goalId: "g1",
          goalTitle: "Goal One",
          status: "proof_submitted",
          domains: ["reddit.com"],
        },
        {
          goalId: "g2",
          goalTitle: "Goal Two",
          status: "proof_submitted",
          domains: ["reddit.com"],
        },
      ],
    };

    const result = evaluateUrl("https://reddit.com/", allReviewCache);
    assert.equal(result.isBlocked, true);
    assert.equal(result.status, "proof_submitted");
    assert.equal(result.instruction, "Proof submitted. Waiting for review.");
  });

  it("never blocks protected pages (dashboard / auth / extension)", () => {
    // Even if somehow added to the blocklist
    const rogueCache = {
      userId: "user_rogue",
      targets: [
        {
          goalId: "rogue",
          goalTitle: "Rogue",
          status: "active",
          domains: ["localhost", "onusly.com"],
        },
      ],
    };

    assert.equal(evaluateUrl("http://localhost:5173/dashboard", rogueCache).isBlocked, false);
    assert.equal(evaluateUrl("https://app.onusly.com/login", rogueCache).isBlocked, false);
    assert.equal(evaluateUrl("chrome-extension://abcdef/blocked.html", rogueCache).isBlocked, false);
  });

  it("fails open when cache is null or empty", () => {
    assert.equal(evaluateUrl("https://youtube.com/", null).isBlocked, false);
    assert.equal(evaluateUrl("https://youtube.com/", { targets: [] }).isBlocked, false);
  });

  it("getBlockedDomainsList aggregates unique active/proof_submitted domains", () => {
    const list = getBlockedDomainsList(sampleCache);
    assert.deepEqual(list, ["instagram.com", "reddit.com", "x.com", "youtube.com"]);
  });

  it("evaluates open tabs URLs and identifies blocked ones for redirection", () => {
    const tabs = [
      { id: 1, url: "https://www.youtube.com/watch?v=123" },
      { id: 2, url: "https://github.com/onusly" },
      { id: 3, url: "https://m.x.com/home" },
      { id: 4, url: "http://localhost:5173/dashboard" },
    ];

    const blockedTabs = tabs.filter((t) => evaluateUrl(t.url, sampleCache).isBlocked);
    assert.equal(blockedTabs.length, 2);
    assert.deepEqual(
      blockedTabs.map((t) => t.id),
      [1, 3]
    );
  });
});
