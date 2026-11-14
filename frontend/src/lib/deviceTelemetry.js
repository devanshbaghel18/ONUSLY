/**
 * Multi-device enforcement telemetry utilities
 */

export function pingExtension(timeoutMs = 1200) {
  return new Promise((resolve) => {
    if (typeof window === "undefined" || !window.chrome?.runtime?.sendMessage) {
      resolve({ active: false, reason: "no_chrome_runtime" });
      return;
    }
    const extensionId =
      (typeof localStorage !== "undefined" && localStorage.getItem("onusly_extension_id")) ||
      "jifigaejbinmdhnljobkodcgeaplojmg";

    let resolved = false;
    const timer = setTimeout(() => {
      if (!resolved) {
        resolved = true;
        resolve({ active: false, reason: "timeout" });
      }
    }, timeoutMs);

    try {
      window.chrome.runtime.sendMessage(
        extensionId,
        { type: "ONUSLY_PING" },
        (res) => {
          if (resolved) return;
          resolved = true;
          clearTimeout(timer);
          if (window.chrome?.runtime?.lastError || !res) {
            resolve({ active: false, reason: "disabled_or_unreachable" });
          } else {
            resolve({ active: true, response: res });
          }
        }
      );
    } catch {
      if (!resolved) {
        resolved = true;
        clearTimeout(timer);
        resolve({ active: false, reason: "exception" });
      }
    }
  });
}

export function formatEnforcementSummary(telemetry, localExtensionActive = null) {
  if (!telemetry) return null;

  let rawDevices = Array.isArray(telemetry.devices) ? [...telemetry.devices] : [];

  if (localExtensionActive === false) {
    rawDevices = rawDevices.map((d) =>
      d.clientType === "extension" || (!d.clientType && d.platform === "windows")
        ? { ...d, enforcing: false, healthStatus: "OFFLINE" }
        : d
    );
  }

  let activeCount = 0;
  if (rawDevices.length > 0) {
    activeCount = rawDevices.filter((d) => d.enforcing).length;
  } else {
    activeCount = Number(telemetry.enforcingCount) || 0;
    if (localExtensionActive === false) {
      activeCount = 0;
    }
  }

  const isEnforcing = activeCount > 0;

  return {
    isEnforcing,
    count: activeCount,
    badgeText: isEnforcing
      ? `${activeCount} Device${activeCount > 1 ? "s" : ""} Connected • Active Enforcement`
      : "No Connected Enforcing Devices (Offline)",
    badgeColor: isEnforcing ? "emerald" : "red",
    subtext: isEnforcing
      ? "Process termination and network domain lockout are actively enforced across connected endpoints."
      : "No active desktop daemon or extension heartbeat detected. Local enforcement is inactive.",
    devices: rawDevices,
  };
}
