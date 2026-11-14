import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { formatEnforcementSummary } from "../src/lib/deviceTelemetry.js";

describe("Sub-part 6.4 — Live Multi-Device Telemetry & Dashboard Integration Suite", () => {
  it("1. Returns null when telemetry is null or undefined", () => {
    assert.equal(formatEnforcementSummary(null), null);
    assert.equal(formatEnforcementSummary(undefined), null);
  });

  it("2. Formats active enforcement badge correctly when 1 or more devices are enforcing", () => {
    const singleDevice = formatEnforcementSummary({
      goalId: "goal-1",
      aggregateStatus: "enforcing",
      devicesCount: 1,
      enforcingCount: 1,
      devices: [
        {
          deviceId: "dev-win-1",
          platform: "windows",
          clientType: "desktop_agent",
          clientVersion: "1.0.0",
          enforcing: true,
        },
      ],
    });

    assert.equal(singleDevice.isEnforcing, true);
    assert.equal(singleDevice.count, 1);
    assert.equal(singleDevice.badgeText, "1 Device Connected • Active Enforcement");
    assert.equal(singleDevice.badgeColor, "emerald");
    assert.equal(singleDevice.devices.length, 1);

    const multiDevice = formatEnforcementSummary({
      goalId: "goal-2",
      aggregateStatus: "enforcing",
      devicesCount: 3,
      enforcingCount: 2,
    });
    assert.equal(multiDevice.isEnforcing, true);
    assert.equal(multiDevice.count, 2);
    assert.equal(multiDevice.badgeText, "2 Devices Connected • Active Enforcement");
  });

  it("3. Formats offline/no devices badge correctly when enforcingCount is 0", () => {
    const offline = formatEnforcementSummary({
      goalId: "goal-3",
      aggregateStatus: "no_devices",
      devicesCount: 0,
      enforcingCount: 0,
    });

    assert.equal(offline.isEnforcing, false);
    assert.equal(offline.count, 0);
    assert.equal(offline.badgeText, "No Connected Enforcing Devices (Offline)");
    assert.equal(offline.badgeColor, "red");
  });

  it("4. Approver privacy: safely handles telemetry when devices array is omitted", () => {
    const approverTelemetry = formatEnforcementSummary({
      goalId: "goal-4",
      aggregateStatus: "enforcing",
      devicesCount: 1,
      enforcingCount: 1,
      // devices omitted by backend for privacy
    });

    assert.equal(approverTelemetry.isEnforcing, true);
    assert.equal(approverTelemetry.badgeText, "1 Device Connected • Active Enforcement");
    assert.deepEqual(approverTelemetry.devices, []);
  });

  it("5. Immediately flips to RED (Offline) when local extension is disabled", () => {
    const disabledTelemetry = formatEnforcementSummary(
      {
        goalId: "goal-5",
        aggregateStatus: "enforcing",
        devicesCount: 1,
        enforcingCount: 1,
        devices: [
          {
            deviceId: "dev-ext-1",
            platform: "windows",
            clientType: "extension",
            clientVersion: "1.0.0",
            enforcing: true,
          },
        ],
      },
      false // localExtensionActive = false (disabled in chrome://extensions)
    );

    assert.equal(disabledTelemetry.isEnforcing, false);
    assert.equal(disabledTelemetry.count, 0);
    assert.equal(disabledTelemetry.badgeText, "No Connected Enforcing Devices (Offline)");
    assert.equal(disabledTelemetry.badgeColor, "red");
    assert.equal(disabledTelemetry.devices[0].enforcing, false);
  });
});
