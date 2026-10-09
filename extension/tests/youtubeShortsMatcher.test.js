import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { isYouTubeHost, isYouTubeShortsUrl } from "../src/domainMatcher.js";
import { evaluateUrl } from "../src/blockerEngine.js";

describe("Strict YouTube Shorts Matcher Suite", () => {
  describe("URL and Navigation Decision Rules", () => {
    it("1. /shorts/VIDEO_ID is recognized as a Shorts URL", () => {
      assert.equal(isYouTubeShortsUrl("https://www.youtube.com/shorts/dQw4w9WgXcQ"), true);
      assert.equal(isYouTubeShortsUrl("https://youtube.com/shorts/abc-123_XYZ"), true);
      assert.equal(isYouTubeShortsUrl("https://www.youtube.com/shorts/"), true);
      assert.equal(isYouTubeShortsUrl("https://www.youtube.com/shorts"), true);
      assert.equal(isYouTubeShortsUrl("https://www.youtube.com/shorts/12345?feature=share"), true);
      assert.equal(isYouTubeShortsUrl("/shorts/dQw4w9WgXcQ"), true);
    });

    it("2. Shorts URLs on supported YouTube host variants are recognized", () => {
      // Desktop standard
      assert.equal(isYouTubeShortsUrl("https://youtube.com/shorts/vid1"), true);
      assert.equal(isYouTubeShortsUrl("https://www.youtube.com/shorts/vid2"), true);
      // Mobile web
      assert.equal(isYouTubeShortsUrl("https://m.youtube.com/shorts/vid3"), true);
      // Music / localized subdomains
      assert.equal(isYouTubeShortsUrl("https://music.youtube.com/shorts/vid4"), true);

      // Verify host matcher directly
      assert.equal(isYouTubeHost("youtube.com"), true);
      assert.equal(isYouTubeHost("www.youtube.com"), true);
      assert.equal(isYouTubeHost("m.youtube.com"), true);
      assert.equal(isYouTubeHost("music.youtube.com"), true);
    });

    it("3. /watch?v=VIDEO_ID is allowed by the Shorts-specific rule", () => {
      const normalWatchUrls = [
        "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
        "https://youtube.com/watch?v=3JZ_D3ELwOQ",
        "https://m.youtube.com/watch?v=fJ9rUzIMcZQ&t=42s",
        "https://www.youtube.com/watch?v=shorts_tutorial_video",
      ];

      for (const url of normalWatchUrls) {
        assert.equal(isYouTubeShortsUrl(url), false, `Should not flag ${url} as a Short`);
        const evalResult = evaluateUrl(url, null);
        assert.equal(evalResult.isBlocked, false, `Should allow normal watch URL: ${url}`);
      }
    });

    it("4. Ordinary YouTube playlist URLs remain allowed by the Shorts-specific rule", () => {
      const playlistUrls = [
        "https://www.youtube.com/playlist?list=PLlaN88a748rreM_vcuP77f8O_gG7P2n0Y",
        "https://youtube.com/playlist?list=PLFsQleAWXsj_4yDebiIADdH5FMayBi05",
        "https://www.youtube.com/feed/subscriptions",
        "https://www.youtube.com/results?search_query=educational+lectures",
        "https://www.youtube.com/",
      ];

      for (const url of playlistUrls) {
        assert.equal(isYouTubeShortsUrl(url), false, `Should not flag ${url} as a Short`);
        const evalResult = evaluateUrl(url, null);
        assert.equal(evalResult.isBlocked, false, `Should allow playlist/feed: ${url}`);
      }
    });

    it("5. notyoutube.com is never treated as YouTube", () => {
      const rogueHosts = [
        "notyoutube.com",
        "fakeyoutube.com",
        "youtube.com.attacker.com",
        "evil-youtube.com",
        "myoutube.com",
      ];

      for (const host of rogueHosts) {
        assert.equal(isYouTubeHost(host), false, `Should reject rogue host: ${host}`);
        assert.equal(
          isYouTubeShortsUrl(`https://${host}/shorts/123`),
          false,
          `Should not block shorts on rogue host: ${host}`
        );
      }
    });

    it("6. Unrelated websites are unaffected", () => {
      const externalUrls = [
        "https://en.wikipedia.org/wiki/Shorts",
        "https://www.google.com/search?q=youtube+shorts",
        "https://github.com/onusly/extension",
        "https://reddit.com/r/shorts",
      ];

      for (const url of externalUrls) {
        assert.equal(isYouTubeShortsUrl(url), false);
        const evalResult = evaluateUrl(url, null);
        assert.equal(evalResult.isBlocked, false, `Should allow external site: ${url}`);
      }
    });

    it("7. Direct navigation and SPA navigation use the same enforcement decision", () => {
      const targetShortsUrl = "https://www.youtube.com/shorts/focus_killer";

      // 1. Direct navigation (no cache, standalone)
      const directDecision = evaluateUrl(targetShortsUrl, null);
      assert.equal(directDecision.isBlocked, true);
      assert.equal(directDecision.reason, "youtube_shorts");
      assert.equal(directDecision.status, "shorts_blocked");
      assert.match(directDecision.instruction, /YouTube Shorts are blocked/);

      // 2. SPA navigation with active unrelated goals
      const dummyBlocklist = {
        userId: "user_test",
        targets: [
          { goalId: "g1", domains: ["twitter.com"], status: "active" },
        ],
      };
      const spaDecision = evaluateUrl(targetShortsUrl, dummyBlocklist);
      assert.equal(spaDecision.isBlocked, true);
      assert.equal(spaDecision.reason, "youtube_shorts");
      assert.equal(spaDecision.status, "shorts_blocked");

      // Verify consistency
      assert.equal(directDecision.isBlocked, spaDecision.isBlocked);
      assert.equal(directDecision.reason, spaDecision.reason);
    });

    it("8. Edge Case Diagnostic: /watch?v=... with a Short video ID", () => {
      // Diagnostic verification for Section 5 of specification:
      // When a user plays a Short via /watch?v=VIDEO_ID, URL path matching does not flag it as /shorts/
      const watchUrlOfShort = "https://www.youtube.com/watch?v=dQw4w9WgXcQ";

      assert.equal(
        isYouTubeShortsUrl(watchUrlOfShort),
        false,
        "Watch URLs are deliberately preserved to avoid false positives on educational short clips"
      );

      const result = evaluateUrl(watchUrlOfShort, null);
      assert.equal(
        result.isBlocked,
        false,
        "Allows watch URLs by default so regular and brief educational tutorials are never falsely blocked"
      );
    });
  });
});
