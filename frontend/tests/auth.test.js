import { test, describe } from "node:test";
import assert from "node:assert/strict";

// Mock browser globals before importing auth module
const storage = new Map();
globalThis.localStorage = {
  getItem: (k) => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: (k) => storage.delete(k),
};
globalThis.fetch = async () => ({ ok: true });

const { isAuthenticated, setAuth } = await import("../src/lib/auth.js");

function toBase64Url(str) {
  return btoa(str).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function makeJwt(payload) {
  const header = toBase64Url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const body = toBase64Url(JSON.stringify(payload));
  return `${header}.${body}.sig`;
}

describe("SECURITY-003 Frontend isAuthenticated Fail-Closed Suite", () => {
  test("malformed non-JWT token returns false and clears storage", () => {
    storage.clear();
    localStorage.setItem("token", "not-a-valid-jwt");
    assert.equal(isAuthenticated(), false);
    assert.equal(localStorage.getItem("token"), null);
  });

  test("expired JWT returns false and clears storage", () => {
    storage.clear();
    setAuth(makeJwt({ sub: "user-1", exp: Math.floor(Date.now() / 1000) - 60 }), { id: "user-1" });
    assert.equal(isAuthenticated(), false);
    assert.equal(localStorage.getItem("token"), null);
  });

  test("valid unexpired JWT returns true", () => {
    storage.clear();
    setAuth(makeJwt({ sub: "user-1", exp: Math.floor(Date.now() / 1000) + 3600 }), { id: "user-1" });
    assert.equal(isAuthenticated(), true);
  });
});
