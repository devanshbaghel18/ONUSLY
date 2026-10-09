import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { fetchBlocklist } from "../src/apiClient.js";

describe("API Client Suite", () => {
  it("fetches blocklist with 200 SUCCESS and handles ETag", async () => {
    let capturedUrl = "";
    let capturedHeaders = {};

    const mockFetch = async (url, options) => {
      capturedUrl = url;
      capturedHeaders = options.headers;
      return {
        status: 200,
        headers: new Map([["ETag", '"v1-abcdef0123456789"']]),
        async json() {
          return {
            userId: "user_api",
            version: "v1-abcdef0123456789",
            generatedAt: "2026-10-09T12:00:00Z",
            targets: [
              {
                goalId: "g1",
                goalTitle: "No YouTube",
                status: "active",
                domains: ["youtube.com"],
              },
            ],
          };
        },
      };
    };

    const result = await fetchBlocklist(
      "http://localhost:8080",
      "test_jwt_token",
      "v1-old",
      mockFetch
    );

    assert.equal(capturedUrl, "http://localhost:8080/enforcement/blocklist");
    assert.equal(capturedHeaders.Authorization, "Bearer test_jwt_token");
    assert.equal(capturedHeaders["If-None-Match"], "v1-old");

    assert.equal(result.type, "SUCCESS");
    assert.equal(result.version, "v1-abcdef0123456789");
    assert.equal(result.data.targets.length, 1);
  });

  it("handles 304 Not Modified correctly", async () => {
    const mockFetch = async (url, options) => {
      assert.equal(options.headers["If-None-Match"], "v1-current");
      return {
        status: 304,
      };
    };

    const result = await fetchBlocklist(
      "http://localhost:8080",
      "test_jwt_token",
      "v1-current",
      mockFetch
    );

    assert.equal(result.type, "NOT_MODIFIED");
  });

  it("handles 401 Unauthorized errors", async () => {
    const mockFetch = async () => ({
      status: 401,
      async json() {
        return { error: "unauthorized token expired" };
      },
    });

    const result = await fetchBlocklist(
      "http://localhost:8080",
      "expired_jwt",
      null,
      mockFetch
    );

    assert.equal(result.type, "ERROR");
    assert.equal(result.status, 401);
    assert.equal(result.message, "unauthorized token expired");
  });

  it("handles network failure gracefully", async () => {
    const mockFetch = async () => {
      throw new Error("Failed to fetch (offline)");
    };

    const result = await fetchBlocklist(
      "http://localhost:8080",
      "test_jwt",
      null,
      mockFetch
    );

    assert.equal(result.type, "ERROR");
    assert.equal(result.status, -1);
    assert.equal(result.message, "Failed to fetch (offline)");
  });

  it("falls back to unconditional GET if conditional request fails with network error", async () => {
    let callCount = 0;
    const mockFetch = async (url, options) => {
      callCount++;
      if (options.headers["If-None-Match"]) {
        throw new Error("CORS preflight failed for If-None-Match");
      }
      return {
        status: 200,
        headers: new Map([["ETag", '"v1-unconditional"']]),
        async json() {
          return {
            userId: "user_api",
            version: "v1-unconditional",
            targets: [],
          };
        },
      };
    };

    const result = await fetchBlocklist(
      "http://localhost:8080",
      "test_jwt_token",
      "v1-old",
      mockFetch
    );

    assert.equal(callCount, 2);
    assert.equal(result.type, "SUCCESS");
    assert.equal(result.version, "v1-unconditional");
    assert.equal(result.data.targets.length, 0);
  });
});
