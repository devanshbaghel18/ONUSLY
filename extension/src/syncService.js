/**
 * Synchronization coordinator for ONUSLY blocklist.
 */

import { fetchBlocklist, sendHeartbeat as apiSendHeartbeat } from "./apiClient.js";

export function detectBrowser() {
  if (typeof navigator === "undefined") return "chrome";
  const ua = (navigator.userAgent || "").toLowerCase();
  if (ua.includes("edg/")) return "edge";
  if (ua.includes("brave") || Boolean(navigator.brave)) return "brave";
  if (ua.includes("firefox")) return "firefox";
  return "chrome";
}

export function detectPlatform() {
  if (typeof navigator === "undefined") return "linux";
  const ua = (navigator.userAgent || "").toLowerCase();
  if (ua.includes("win")) return "windows";
  if (ua.includes("mac")) return "macos";
  if (ua.includes("android")) return "android";
  return "linux";
}

/**
 * Creates a sync service instance bound to a storage instance.
 *
 * @param {object} storage - Storage manager created by createStorage().
 * @param {Function} [fetchFn] - Optional fetch implementation for testing.
 */
export function createSyncService(storage, fetchFn = globalThis.fetch) {
  return {
    /**
     * Synchronizes the blocklist from the backend API.
     * Respects ETag/304 caching and maintains offline cache persistence.
     *
     * @returns {Promise<{
     *   success: boolean,
     *   type: "UPDATED"|"NOT_MODIFIED"|"ERROR"|"UNAUTHENTICATED",
     *   version?: string,
     *   targetsCount?: number,
     *   error?: string
     * }>}
     */
    async sync() {
      const session = await storage.getSession();
      if (!session || !session.token || !session.userId) {
        return {
          success: false,
          type: "UNAUTHENTICATED",
          error: "No active user session",
        };
      }

      const settings = await storage.getSettings();
      const existingCache = await storage.getBlocklistCache(session.userId);
      const ifNoneMatch = existingCache?.version || null;

      const result = await fetchBlocklist(
        settings.baseUrl,
        session.token,
        ifNoneMatch,
        fetchFn
      );

      if (result.type === "SUCCESS") {
        const { data, version } = result;

        // Multi-user safety check: verify server returned data for the authenticated user
        if (data.userId !== session.userId) {
          return {
            success: false,
            type: "ERROR",
            error: "User ID mismatch in API response",
          };
        }

        const saved = await storage.saveBlocklistCache(data, session.userId);
        if (!saved) {
          return {
            success: false,
            type: "ERROR",
            error: "Failed to persist blocklist cache",
          };
        }

        this.sendHeartbeat().catch(() => {});
        return {
          success: true,
          type: "UPDATED",
          version,
          targetsCount: (data.targets || []).length,
        };
      }

      if (result.type === "NOT_MODIFIED") {
        // Cache is still fresh on the server — update sync timestamp
        await storage.updateLastSyncTimestamp();
        this.sendHeartbeat().catch(() => {});
        return {
          success: true,
          type: "NOT_MODIFIED",
          version: existingCache?.version,
          targetsCount: (existingCache?.targets || []).length,
        };
      }

      // Offline / Network / Server Error:
      // CRITICAL REQUIREMENT: Do NOT clear cached restrictions when network request fails.
      this.sendHeartbeat(["service_degraded"]).catch(() => {});
      return {
        success: false,
        type: "ERROR",
        error: result.message || "Sync failed",
        version: existingCache?.version,
        targetsCount: (existingCache?.targets || []).length,
      };
    },

    /**
     * Reports client telemetry and enforcement health to the backend.
     * @param {string[]} [issues]
     * @returns {Promise<boolean>}
     */
    async sendHeartbeat(issues = []) {
      const session = await storage.getSession();
      if (!session || !session.token || !session.userId) {
        return false;
      }
      const settings = await storage.getSettings();
      const deviceId = await storage.getDeviceId();
      const cache = await storage.getBlocklistCache(session.userId);

      const payload = {
        deviceId,
        clientType: "extension",
        platform: detectPlatform(),
        browser: detectBrowser(),
        clientVersion: "1.0.0",
        enforcing: true,
        blocklistVersion: cache?.version || "",
        issues,
        sentAt: new Date().toISOString(),
      };

      return await apiSendHeartbeat(settings.baseUrl, session.token, payload, fetchFn);
    },
  };
}
