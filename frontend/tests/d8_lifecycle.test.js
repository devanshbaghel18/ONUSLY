import { test, describe } from "node:test";
import assert from "node:assert/strict";

describe("D8 Frontend Device Lifecycle & Revocation Suite", () => {
  test("1. Active fleet enforcement calculation strictly excludes REVOKED devices", () => {
    const devices = [
      { deviceId: "dev-1", status: "PAIRED", enforcing: true, issues: [] },
      { deviceId: "dev-2", status: "REVOKED", enforcing: false, issues: ["device_revoked"] },
    ];

    const activeDevices = devices.filter(
      (d) => (d.status || "").toUpperCase() !== "REVOKED"
    );
    assert.equal(activeDevices.length, 1);
    assert.equal(activeDevices[0].deviceId, "dev-1");

    const allEnforcing =
      activeDevices.length > 0 &&
      activeDevices.every((d) => d.enforcing && (!d.issues || d.issues.length === 0));

    assert.equal(allEnforcing, true, "Revoked devices must not degrade active fleet status to Issues Detected");
  });

  test("2. Fleet status correctly identifies issues when an active device has problems", () => {
    const devices = [
      { deviceId: "dev-1", status: "PAIRED", enforcing: false, issues: ["dns_resolver_offline"] },
      { deviceId: "dev-2", status: "REVOKED", enforcing: false, issues: ["device_revoked"] },
    ];

    const activeDevices = devices.filter(
      (d) => (d.status || "").toUpperCase() !== "REVOKED"
    );
    const allEnforcing =
      activeDevices.length > 0 &&
      activeDevices.every((d) => d.enforcing && (!d.issues || d.issues.length === 0));

    assert.equal(allEnforcing, false, "Degraded active devices correctly flag issues");
  });

  test("3. Lifecycle state derivation accurately maps status conditions", () => {
    function deriveLifecycleState(d) {
      const status = (d.status || "").toUpperCase();
      if (status === "REVOKED") return "REVOKED";
      if (status === "AUTHENTICATION_REQUIRED" || status === "AUTH_REQUIRED") return "AUTH_REQUIRED";
      if (!d.enforcing || (d.issues && d.issues.length > 0)) return "DEGRADED";
      const isRecent = d.lastHeartbeatAt && Date.now() - new Date(d.lastHeartbeatAt).getTime() < 5 * 60 * 1000;
      if (!isRecent) return "OFFLINE";
      return "ONLINE";
    }

    const now = new Date().toISOString();
    const staleTime = new Date(Date.now() - 10 * 60 * 1000).toISOString();

    assert.equal(deriveLifecycleState({ status: "REVOKED", enforcing: false }), "REVOKED");
    assert.equal(deriveLifecycleState({ status: "AUTHENTICATION_REQUIRED", enforcing: false }), "AUTH_REQUIRED");
    assert.equal(deriveLifecycleState({ status: "PAIRED", enforcing: false, issues: ["service_degraded"] }), "DEGRADED");
    assert.equal(deriveLifecycleState({ status: "PAIRED", enforcing: true, issues: [], lastHeartbeatAt: staleTime }), "OFFLINE");
    assert.equal(deriveLifecycleState({ status: "PAIRED", enforcing: true, issues: [], lastHeartbeatAt: now }), "ONLINE");
  });
});

