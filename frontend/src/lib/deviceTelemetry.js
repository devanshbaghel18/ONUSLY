/**
 * Multi-device enforcement telemetry utilities
 */

export function formatEnforcementSummary(telemetry) {
  if (!telemetry) return null;
  const count = Number(telemetry.enforcingCount) || 0;
  const isEnforcing = count > 0;
  return {
    isEnforcing,
    count,
    badgeText: isEnforcing
      ? `${count} Device${count > 1 ? "s" : ""} Connected • Active Enforcement`
      : "No Connected Enforcing Devices (Offline)",
    badgeColor: isEnforcing ? "emerald" : "red",
    subtext: isEnforcing
      ? "Process termination and network domain lockout are actively enforced across connected endpoints."
      : "No active desktop daemon or extension heartbeat detected. Local enforcement may be inactive.",
    devices: Array.isArray(telemetry.devices) ? telemetry.devices : [],
  };
}
