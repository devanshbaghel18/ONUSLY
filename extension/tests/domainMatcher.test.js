import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  isDomainMatch,
  normalizeDomain,
  extractHostname,
  isProtectedUrl,
} from "../src/domainMatcher.js";

describe("Domain Matcher Suite", () => {
  describe("normalizeDomain", () => {
    it("trims whitespace and converts to lowercase", () => {
      assert.equal(normalizeDomain("  YouTube.COM  "), "youtube.com");
    });

    it("strips scheme, path, query and ports", () => {
      assert.equal(normalizeDomain("https://reddit.com/r/all?sort=top"), "reddit.com");
      assert.equal(normalizeDomain("http://example.com:8080/path"), "example.com");
      assert.equal(normalizeDomain("sub.domain.com:3000"), "sub.domain.com");
    });

    it("strips trailing dots", () => {
      assert.equal(normalizeDomain("instagram.com..."), "instagram.com");
    });
  });

  describe("isDomainMatch", () => {
    it("matches exact target domains", () => {
      assert.equal(isDomainMatch("youtube.com", "youtube.com"), true);
      assert.equal(isDomainMatch("YOUTUBE.COM", "youtube.com"), true);
      assert.equal(isDomainMatch("reddit.com", "reddit.com"), true);
    });

    it("matches subdomains of the target domain", () => {
      assert.equal(isDomainMatch("www.youtube.com", "youtube.com"), true);
      assert.equal(isDomainMatch("m.youtube.com", "youtube.com"), true);
      assert.equal(isDomainMatch("music.youtube.com", "youtube.com"), true);
      assert.equal(isDomainMatch("deep.sub.reddit.com", "reddit.com"), true);
    });

    it("prevents false-positive matches on similar or prefix/suffix domains", () => {
      // notyoutube.com contains youtube.com but is a completely different domain!
      assert.equal(isDomainMatch("notyoutube.com", "youtube.com"), false);
      assert.equal(isDomainMatch("myoutube.com", "youtube.com"), false);
      assert.equal(isDomainMatch("youtube.co", "youtube.com"), false);
      assert.equal(isDomainMatch("fake-reddit.com", "reddit.com"), false);
      assert.equal(isDomainMatch("reddit.com.attacker.com", "reddit.com"), false);
    });

    it("returns false for empty or null inputs", () => {
      assert.equal(isDomainMatch("", "youtube.com"), false);
      assert.equal(isDomainMatch("youtube.com", ""), false);
      assert.equal(isDomainMatch(null, null), false);
    });
  });

  describe("extractHostname", () => {
    it("extracts hostname from valid URLs", () => {
      assert.equal(extractHostname("https://www.instagram.com/p/123"), "www.instagram.com");
      assert.equal(extractHostname("http://x.com:8080/explore"), "x.com");
    });

    it("returns null for malformed URLs", () => {
      assert.equal(extractHostname("not-a-url"), null);
      assert.equal(extractHostname(""), null);
      assert.equal(extractHostname(null), null);
    });
  });

  describe("isProtectedUrl", () => {
    it("protects internal browser schemes", () => {
      assert.equal(isProtectedUrl("chrome://settings"), true);
      assert.equal(isProtectedUrl("chrome-extension://abcdef/blocked.html"), true);
      assert.equal(isProtectedUrl("about:blank"), true);
      assert.equal(isProtectedUrl("file:///home/user/doc.txt"), true);
    });

    it("protects localhost and development servers", () => {
      assert.equal(isProtectedUrl("http://localhost:5173/"), true);
      assert.equal(isProtectedUrl("http://localhost:8080/health"), true);
      assert.equal(isProtectedUrl("http://127.0.0.1:5173/dashboard"), true);
      assert.equal(isProtectedUrl("http://10.0.2.2:8080/"), true);
    });

    it("protects ONUSLY official domains", () => {
      assert.equal(isProtectedUrl("https://onusly.com/"), true);
      assert.equal(isProtectedUrl("https://app.onusly.com/dashboard"), true);
      assert.equal(isProtectedUrl("https://api.onusly.com/auth/google"), true);
    });

    it("does not protect external websites", () => {
      assert.equal(isProtectedUrl("https://youtube.com/"), false);
      assert.equal(isProtectedUrl("https://www.instagram.com/"), false);
      assert.equal(isProtectedUrl("https://reddit.com/"), false);
    });
  });
});
