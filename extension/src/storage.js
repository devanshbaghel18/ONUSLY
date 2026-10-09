/**
 * Storage and cache isolation layer for ONUSLY extension.
 * Persists session and blocklist cache via chrome.storage.local (or provided adapter).
 */

const KEY_SESSION = "onusly_session";
const KEY_CACHE = "onusly_cache";
const KEY_SETTINGS = "onusly_settings";

/**
 * Creates a storage manager instance.
 * @param {object} [customAdapter] - Optional custom storage adapter (e.g. for testing).
 */
export function createStorage(customAdapter = null) {
  // In-memory fallback if chrome.storage is not available
  const memoryStore = new Map();

  const adapter = customAdapter || (globalThis.chrome?.storage?.local ? {
    async get(keys) {
      return new Promise((resolve) => {
        globalThis.chrome.storage.local.get(keys, resolve);
      });
    },
    async set(items) {
      return new Promise((resolve) => {
        globalThis.chrome.storage.local.set(items, resolve);
      });
    },
    async remove(keys) {
      return new Promise((resolve) => {
        globalThis.chrome.storage.local.remove(keys, resolve);
      });
    },
    async clear() {
      return new Promise((resolve) => {
        globalThis.chrome.storage.local.clear(resolve);
      });
    },
  } : {
    async get(keys) {
      const result = {};
      const keyList = Array.isArray(keys) ? keys : typeof keys === "string" ? [keys] : Object.keys(keys || {});
      for (const k of keyList) {
        if (memoryStore.has(k)) {
          result[k] = memoryStore.get(k);
        }
      }
      return result;
    },
    async set(items) {
      for (const [k, v] of Object.entries(items)) {
        memoryStore.set(k, v);
      }
    },
    async remove(keys) {
      const keyList = Array.isArray(keys) ? keys : [keys];
      for (const k of keyList) {
        memoryStore.delete(k);
      }
    },
    async clear() {
      memoryStore.clear();
    },
  });

  return {
    /**
     * Retrieves the current authenticated user session.
     */
    async getSession() {
      const data = await adapter.get([KEY_SESSION]);
      const session = data[KEY_SESSION] || null;
      if (!session || !session.token || !session.userId) {
        return null;
      }
      // Check expiration if exp timestamp is present
      if (session.exp && Date.now() >= session.exp * 1000) {
        return null;
      }
      return session;
    },

    /**
     * Saves a new authenticated user session.
     */
    async saveSession(session) {
      if (!session || !session.userId || !session.token) {
        throw new Error("Invalid session data: userId and token are required");
      }
      await adapter.set({ [KEY_SESSION]: session });
    },

    /**
     * Clears user session.
     */
    async clearSession() {
      await adapter.remove([KEY_SESSION]);
    },

    /**
     * Retrieves the cached blocklist.
     * Enforces user isolation: if expectedUserId is provided, ensures cached userId matches.
     *
     * @param {string|null} [expectedUserId]
     * @returns {Promise<object|null>}
     */
    async getBlocklistCache(expectedUserId = null) {
      const data = await adapter.get([KEY_CACHE]);
      const cached = data[KEY_CACHE] || null;
      if (!cached || !cached.userId) {
        return null;
      }

      // User isolation check: never apply User A's cache to User B
      if (expectedUserId && cached.userId !== expectedUserId) {
        return null;
      }

      return cached;
    },

    /**
     * Saves a blocklist response to cache for a specific user.
     * Strict isolation rules:
     * - response.userId must match expectedUserId.
     * - An existing cache belonging to a different user cannot be overwritten without clearing.
     *
     * @param {object} response - The blocklist API response payload.
     * @param {string} expectedUserId - The authenticated user's ID.
     * @returns {Promise<boolean>}
     */
    async saveBlocklistCache(response, expectedUserId) {
      if (!response || !response.userId || !expectedUserId) {
        return false;
      }

      if (response.userId !== expectedUserId) {
        return false;
      }

      const existingData = await adapter.get([KEY_CACHE]);
      const existing = existingData[KEY_CACHE] || null;

      if (existing && existing.userId && existing.userId !== expectedUserId) {
        // Multi-user isolation: do not overwrite User A's non-empty cache with User B's data
        return false;
      }

      const cacheEntry = {
        userId: response.userId,
        version: response.version || "",
        generatedAt: response.generatedAt || new Date().toISOString(),
        lastSyncTimestamp: Date.now(),
        targets: Array.isArray(response.targets) ? response.targets : [],
      };

      await adapter.set({ [KEY_CACHE]: cacheEntry });
      return true;
    },

    /**
     * Updates the last sync timestamp on existing cache (e.g. for 304 Not Modified).
     * @param {number} timestamp
     */
    async updateLastSyncTimestamp(timestamp = Date.now()) {
      const data = await adapter.get([KEY_CACHE]);
      const existing = data[KEY_CACHE];
      if (existing) {
        existing.lastSyncTimestamp = timestamp;
        await adapter.set({ [KEY_CACHE]: existing });
      }
    },

    /**
     * Clears both session and cache (used on explicit disconnect/logout).
     */
    async clearAll() {
      await adapter.remove([KEY_SESSION, KEY_CACHE]);
    },

    /**
     * Gets API settings (e.g. baseUrl).
     */
    async getSettings() {
      const data = await adapter.get([KEY_SETTINGS]);
      return (
        data[KEY_SETTINGS] || {
          baseUrl: "http://localhost:8080",
          dashboardUrl: "http://localhost:5173",
        }
      );
    },

    /**
     * Updates API settings.
     */
    async saveSettings(settings) {
      const current = await this.getSettings();
      await adapter.set({ [KEY_SETTINGS]: { ...current, ...settings } });
    },
  };
}
