import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { evaluateUrl, getBlockedDomainsList, isEnforcingStatus, isTargetShortsRestricted } from "../src/blockerEngine.js";

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

  it("blocking YouTube blocks both normal YouTube and YouTube Shorts", () => {
    const ytCache = {
      targets: [
        {
          goalId: "g_yt",
          status: "active",
          apps: ["com.google.android.youtube"],
          domains: ["youtube.com", "youtu.be"],
        },
      ],
    };

    const watchEval = evaluateUrl("https://www.youtube.com/watch?v=abc", ytCache);
    assert.equal(watchEval.isBlocked, true);
    assert.equal(watchEval.reason, "goal_target");

    const shortsEval = evaluateUrl("https://www.youtube.com/shorts/xyz", ytCache);
    assert.equal(shortsEval.isBlocked, true);
    assert.equal(shortsEval.reason, "goal_target");
  });

  it("blocking YouTube Shorts ONLY blocks YouTube Shorts and allows normal YouTube", () => {
    const shortsCache = {
      targets: [
        {
          goalId: "g_shorts",
          status: "active",
          apps: ["com.google.android.youtube.shorts"],
          domains: ["shorts.youtube.com"],
        },
      ],
    };

    const watchEval = evaluateUrl("https://www.youtube.com/watch?v=abc", shortsCache);
    assert.equal(watchEval.isBlocked, false);
    assert.equal(watchEval.isShortsRestricted, true);

    const shortsEval = evaluateUrl("https://www.youtube.com/shorts/xyz", shortsCache);
    assert.equal(shortsEval.isBlocked, true);
    assert.equal(shortsEval.reason, "youtube_shorts");
  });

  it("does not block YouTube or YouTube Shorts when no goal selects them", () => {
    const emptyCache = { targets: [] };
    assert.equal(evaluateUrl("https://www.youtube.com/watch?v=abc", emptyCache).isBlocked, false);
    assert.equal(evaluateUrl("https://www.youtube.com/shorts/xyz", emptyCache).isBlocked, false);

    const unrelatedCache = {
      targets: [
        {
          goalId: "g_reddit",
          status: "active",
          apps: ["com.reddit.frontpage"],
          domains: ["reddit.com"],
        },
      ],
    };
    assert.equal(evaluateUrl("https://www.youtube.com/watch?v=abc", unrelatedCache).isBlocked, false);
    assert.equal(evaluateUrl("https://www.youtube.com/shorts/xyz", unrelatedCache).isBlocked, false);
  });

  describe("P0 Client Enforcement Invariant - Enforcing Statuses & Normalization", () => {
    it("Active goal with matching domain -> Block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Active Goal", status: "active", domains: ["twitter.com"] },
        ],
      };
      const res = evaluateUrl("https://twitter.com/home", cache);
      assert.equal(res.isBlocked, true);
      assert.equal(res.status, "active");
      assert.equal(res.instruction, "Complete your goal and submit proof.");
    });

    it("Proof-submitted goal with matching domain -> Block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Review Goal", status: "proof_submitted", domains: ["twitter.com"] },
        ],
      };
      const res = evaluateUrl("https://twitter.com/home", cache);
      assert.equal(res.isBlocked, true);
      assert.equal(res.status, "proof_submitted");
      assert.equal(res.instruction, "Proof submitted. Waiting for review.");
    });

    it("Rejected goal with matching domain -> Block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Rejected Goal", status: "rejected", domains: ["twitter.com"] },
        ],
      };
      const res = evaluateUrl("https://twitter.com/home", cache);
      assert.equal(res.isBlocked, true);
      assert.equal(res.status, "rejected");
      assert.equal(res.instruction, "Proof was rejected. Complete your goal before resubmission.");
      assert.equal(res.matchingTargets.length, 1);
    });

    it("Completed goal with matching domain -> Allow", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Completed Goal", status: "completed", domains: ["twitter.com"] },
        ],
      };
      const res = evaluateUrl("https://twitter.com/home", cache);
      assert.equal(res.isBlocked, false);
      assert.equal(res.status, "none");
      assert.equal(res.instruction, "");
      assert.equal(res.matchingTargets.length, 0);
    });

    it("Rejected goal with matching application -> Block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Rejected App", status: "rejected", apps: ["com.twitter.android"] },
        ],
      };
      const res = evaluateUrl("https://x.com/explore", cache);
      assert.equal(res.isBlocked, true);
      assert.equal(res.status, "rejected");
      assert.equal(res.instruction, "Proof was rejected. Complete your goal before resubmission.");
    });

    it("Rejected goal with YouTube Shorts restriction -> Block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Shorts Goal", status: "rejected", apps: ["youtube_shorts"] },
        ],
      };
      assert.equal(isTargetShortsRestricted(cache.targets[0]), true);
      const res = evaluateUrl("https://www.youtube.com/shorts/xyz123", cache);
      assert.equal(res.isBlocked, true);
      assert.equal(res.reason, "youtube_shorts");
      assert.equal(res.status, "rejected");
      assert.equal(res.instruction, "Proof was rejected. Complete your goal before resubmission.");

      // Regular video is allowed
      const resWatch = evaluateUrl("https://www.youtube.com/watch?v=regular", cache);
      assert.equal(resWatch.isBlocked, false);
    });

    it("Rejected goal with no targets -> No target to block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Empty Targets", status: "rejected", apps: [], domains: [] },
        ],
      };
      const res = evaluateUrl("https://twitter.com/home", cache);
      assert.equal(res.isBlocked, false);
      assert.equal(res.matchingTargets.length, 0);
    });

    it("Two matching goals: active + rejected -> Block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Goal Active", status: "active", domains: ["reddit.com"] },
          { goalId: "g2", goalTitle: "Goal Rejected", status: "rejected", domains: ["reddit.com"] },
        ],
      };
      const res = evaluateUrl("https://reddit.com/r/all", cache);
      assert.equal(res.isBlocked, true);
      assert.equal(res.matchingTargets.length, 2);
      assert.equal(res.status, "active");
      assert.equal(res.instruction, "Complete your goal and submit proof.");
    });

    it("Two matching goals: completed + rejected -> Block", () => {
      const cache = {
        targets: [
          { goalId: "g1", goalTitle: "Goal Done", status: "completed", domains: ["reddit.com"] },
          { goalId: "g2", goalTitle: "Goal Rejected", status: "rejected", domains: ["reddit.com"] },
        ],
      };
      const res = evaluateUrl("https://reddit.com/r/all", cache);
      assert.equal(res.isBlocked, true);
      assert.equal(res.matchingTargets.length, 1);
      assert.equal(res.status, "rejected");
      assert.equal(res.instruction, "Proof was rejected. Complete your goal before resubmission.");
    });

    it("Unknown status -> Preserve existing safe handling; does not classify as enforcing", () => {
      const unknownStatuses = ["unknown", "archived", "pending", "paused", "deleted", "", null, undefined];
      for (const st of unknownStatuses) {
        const cache = {
          targets: [
            { goalId: "g1", goalTitle: "Unknown Status Goal", status: st, domains: ["reddit.com"] },
          ],
        };
        const res = evaluateUrl("https://reddit.com/r/all", cache);
        assert.equal(res.isBlocked, false, `Status ${st} must not block`);
        assert.equal(res.matchingTargets.length, 0);
        assert.equal(isEnforcingStatus(st), false);
      }
    });

    it("Whitespace and case normalization (' REJECTED ')", () => {
      const variations = [" REJECTED ", "Rejected", "rejected ", "  proof_submitted  ", "PROOF_SUBMITTED", " Active "];
      for (const st of variations) {
        assert.equal(isEnforcingStatus(st), true, `Status '${st}' must be enforcing`);
        const cache = {
          targets: [
            { goalId: "g_var", goalTitle: "Var Goal", status: st, domains: ["example-blocked.com"] },
          ],
        };
        const res = evaluateUrl("https://example-blocked.com/", cache);
        assert.equal(res.isBlocked, true, `Status '${st}' must block target`);
      }
    });

    it("getBlockedDomainsList includes domains from rejected goals", () => {
      const cache = {
        targets: [
          { goalId: "g1", status: "active", domains: ["domain-a.com"] },
          { goalId: "g2", status: "proof_submitted", domains: ["domain-b.com"] },
          { goalId: "g3", status: "rejected", domains: ["domain-c.com"] },
          { goalId: "g4", status: "completed", domains: ["domain-d.com"] },
          { goalId: "g5", status: "unknown", domains: ["domain-e.com"] },
        ],
      };
      const domains = getBlockedDomainsList(cache);
      assert.deepEqual(domains, ["domain-a.com", "domain-b.com", "domain-c.com"]);
    });
  });
});
