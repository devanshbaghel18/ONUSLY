import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { isAuthorizedOrigin, parseJwtPayload } from "../background.js";
import { createStorage } from "../src/storage.js";

describe("Authentication Handoff Suite", () => {
  describe("isAuthorizedOrigin", () => {
    it("allows local dev server origins", () => {
      assert.equal(isAuthorizedOrigin({ origin: "http://localhost:5173" }), true);
      assert.equal(isAuthorizedOrigin({ origin: "http://localhost:8080" }), true);
      assert.equal(isAuthorizedOrigin({ url: "http://localhost:5173/dashboard" }), true);
      assert.equal(isAuthorizedOrigin({ origin: "http://127.0.0.1:5173" }), true);
    });

    it("allows official ONUSLY production and subdomain origins", () => {
      assert.equal(isAuthorizedOrigin({ origin: "https://onusly.com" }), true);
      assert.equal(isAuthorizedOrigin({ origin: "https://app.onusly.com" }), true);
      assert.equal(isAuthorizedOrigin({ origin: "https://dashboard.onusly.com" }), true);
    });

    it("strictly rejects untrusted or third-party origins", () => {
      assert.equal(isAuthorizedOrigin({ origin: "https://malicious.com" }), false);
      assert.equal(isAuthorizedOrigin({ origin: "http://localhost:3001" }), false);
      assert.equal(isAuthorizedOrigin({ origin: "https://fake-onusly.com" }), false);
      assert.equal(isAuthorizedOrigin({ origin: "https://onusly.com.attacker.com" }), false);
      assert.equal(isAuthorizedOrigin(null), false);
      assert.equal(isAuthorizedOrigin({}), false);
    });
  });

  describe("parseJwtPayload", () => {
    it("parses valid JWT claims", () => {
      // payload: {"sub":"user_12345","email":"test@onusly.com","name":"Test User","exp":1893456000}
      const rawPayload = JSON.stringify({
        sub: "user_12345",
        email: "test@onusly.com",
        name: "Test User",
        exp: 1893456000,
      });
      const encodedPayload = Buffer.from(rawPayload).toString("base64url");
      const fakeToken = `header.${encodedPayload}.signature`;

      const parsed = parseJwtPayload(fakeToken);
      assert.equal(parsed.sub, "user_12345");
      assert.equal(parsed.email, "test@onusly.com");
      assert.equal(parsed.name, "Test User");
      assert.equal(parsed.exp, 1893456000);
    });

    it("returns null for malformed tokens", () => {
      assert.equal(parseJwtPayload("not-a-jwt"), null);
      assert.equal(parseJwtPayload("header.part2"), null);
      assert.equal(parseJwtPayload(""), null);
      assert.equal(parseJwtPayload(null), null);
    });
  });

  describe("Account switching safety", () => {
    it("ensures switching accounts clears old user session and cache", async () => {
      const storage = createStorage();

      // Alice is logged in and has cache
      await storage.saveSession({ userId: "alice", token: "tok_alice" });
      await storage.saveBlocklistCache(
        {
          userId: "alice",
          version: "v1-alice",
          targets: [{ goalId: "g1", domains: ["youtube.com"], status: "active" }],
        },
        "alice"
      );

      assert.equal((await storage.getSession()).userId, "alice");
      assert.notEqual(await storage.getBlocklistCache("alice"), null);

      // Simulate Bob logging in: detection of user change clears old data
      const currentSession = await storage.getSession();
      const newUserId = "bob";

      if (currentSession && currentSession.userId !== newUserId) {
        await storage.clearAll();
      }

      await storage.saveSession({ userId: "bob", token: "tok_bob" });
      await storage.saveBlocklistCache(
        {
          userId: "bob",
          version: "v1-bob",
          targets: [{ goalId: "g2", domains: ["x.com"], status: "active" }],
        },
        "bob"
      );

      // Verify Alice's cache is gone and Bob's cache is isolated
      assert.equal((await storage.getSession()).userId, "bob");
      assert.equal(await storage.getBlocklistCache("alice"), null);
      const bobCache = await storage.getBlocklistCache("bob");
      assert.equal(bobCache.version, "v1-bob");
      assert.equal(bobCache.targets[0].domains[0], "x.com");
    });
  });
});
