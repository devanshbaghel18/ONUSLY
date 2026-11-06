import test from "node:test";
import assert from "node:assert/strict";
import { createStorage } from "../src/storage.js";
import { createSyncService } from "../src/syncService.js";

function createMemoryAdapter() {
  const store = new Map();
  return {
    async get(keys) {
      const res = {};
      const keyList = Array.isArray(keys) ? keys : [keys];
      for (const k of keyList) {
        if (store.has(k)) res[k] = store.get(k);
      }
      return res;
    },
    async set(items) {
      for (const [k, v] of Object.entries(items)) {
        store.set(k, v);
      }
    },
    async remove(keys) {
      const keyList = Array.isArray(keys) ? keys : [keys];
      for (const k of keyList) {
        store.delete(k);
      }
    },
    async clear() {
      store.clear();
    },
  };
}

test("Extension Heartbeat & Device Telemetry Suite", async (t) => {
  await t.test("generates and persists unique device UUID", async () => {
    const storage = createStorage(createMemoryAdapter());
    const id1 = await storage.getDeviceId();
    assert.match(id1, /^[0-9a-f-]{36}$/i);

    const id2 = await storage.getDeviceId();
    assert.equal(id1, id2, "Device ID should be persistent");
  });

  await t.test("sends heartbeat with correct telemetry payload on sync", async () => {
    const storage = createStorage(createMemoryAdapter());
    await storage.saveSession({
      userId: "user-ext-test",
      email: "test@onusly.com",
      token: "valid-jwt-token",
    });

    let heartbeatSent = false;
    let heartbeatBody = null;

    const mockFetch = async (url, options) => {
      if (url.includes("/enforcement/blocklist")) {
        return {
          status: 200,
          headers: new Headers({ ETag: '"v1-mock123"' }),
          json: async () => ({
            userId: "user-ext-test",
            version: "v1-mock123",
            targets: [{ goalId: "g1", domains: ["twitter.com"], status: "active" }],
          }),
        };
      }
      if (url.includes("/enforcement/heartbeat")) {
        heartbeatSent = true;
        heartbeatBody = JSON.parse(options.body);
        return {
          status: 200,
          ok: true,
          json: async () => ({ status: "ok" }),
        };
      }
      throw new Error(`Unexpected URL: ${url}`);
    };

    const syncService = createSyncService(storage, mockFetch);
    const res = await syncService.sync();
    assert.equal(res.success, true);

    // Give microtask queue moment for non-blocking sendHeartbeat
    await new Promise((r) => setTimeout(r, 50));

    assert.equal(heartbeatSent, true, "Heartbeat should have been sent to backend");
    assert.equal(heartbeatBody.clientType, "extension");
    assert.match(heartbeatBody.deviceId, /^[0-9a-f-]{36}$/i);
    assert.equal(heartbeatBody.enforcing, true);
    assert.equal(heartbeatBody.blocklistVersion, "v1-mock123");
  });

  await t.test("reports service_degraded issue when blocklist sync encounters network error", async () => {
    const storage = createStorage(createMemoryAdapter());
    await storage.saveSession({
      userId: "user-ext-test",
      email: "test@onusly.com",
      token: "valid-jwt-token",
    });

    let reportedIssues = null;
    const mockFetch = async (url, options) => {
      if (url.includes("/enforcement/blocklist")) {
        throw new Error("Network down");
      }
      if (url.includes("/enforcement/heartbeat")) {
        const body = JSON.parse(options.body);
        reportedIssues = body.issues;
        return { status: 200, ok: true, json: async () => ({}) };
      }
      throw new Error(`Unexpected URL: ${url}`);
    };

    const syncService = createSyncService(storage, mockFetch);
    const res = await syncService.sync();
    assert.equal(res.success, false);

    await new Promise((r) => setTimeout(r, 50));
    assert.deepEqual(reportedIssues, ["service_degraded"]);
  });
});
