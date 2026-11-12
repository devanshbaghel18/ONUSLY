/**
 * ONUSLY Browser Extension Service Worker (Manifest V3)
 */

import { createStorage } from "./src/storage.js";
import { createSyncService } from "./src/syncService.js";
import { evaluateUrl, isEnforcingStatus } from "./src/blockerEngine.js";

export const storage = createStorage();
export const syncService = createSyncService(storage);

const ALARM_NAME = "onusly_sync_alarm";
const ALLOWED_ORIGIN_PATTERNS = [
  /^http:\/\/localhost:(5173|5174|5175|4173|8080|3000)$/,
  /^http:\/\/127\.0\.0\.1:(5173|5174|5175|4173|8080|3000)$/,
  /^https:\/\/(.*\.)?onusly\.com$/,
];

/**
 * Validates whether the sender of an external message is an authorized ONUSLY origin.
 * @param {chrome.runtime.MessageSender} sender
 * @returns {boolean}
 */
export function isAuthorizedOrigin(sender) {
  if (!sender) return false;
  const origin = sender.origin || (sender.url ? new URL(sender.url).origin : null);
  if (!origin) return false;
  return ALLOWED_ORIGIN_PATTERNS.some((pattern) => pattern.test(origin));
}

/**
 * Parses JWT payload safely without external dependencies.
 * @param {string} token
 * @returns {object|null}
 */
export function parseJwtPayload(token) {
  if (!token || typeof token !== "string") return null;
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  try {
    let base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    while (base64.length % 4) {
      base64 += "=";
    }
    const jsonStr = atob(base64);
    return JSON.parse(jsonStr);
  } catch {
    return null;
  }
}

// --------------------------------------------------------------------------
// Navigation & Open Tab Interception (Browser-Enforced Website Blocking)
// --------------------------------------------------------------------------

// Tracks in-flight redirects per tab to prevent rapid-fire loop storms
const pendingBlockedTabs = new Map(); // tabId -> targetUrl

/**
 * Evaluates a tab's URL against active restrictions and redirects to blocked.html if blocked.
 * @param {number} tabId
 * @param {string} url
 * @param {object|null} [providedCache]
 */
export async function checkAndBlockTab(tabId, url, providedCache = null) {
  if (!tabId || !url) return;

  // Never evaluate or block the extension's own blocked page or internal browser URLs
  if (
    url.includes("blocked.html") ||
    url.startsWith("chrome-extension://") ||
    url.startsWith("moz-extension://") ||
    url.startsWith("edge://") ||
    url.startsWith("about:")
  ) {
    pendingBlockedTabs.delete(tabId);
    return;
  }

  // Prevent duplicate concurrent redirects for the same tab to the same URL
  if (pendingBlockedTabs.has(tabId) && pendingBlockedTabs.get(tabId) === url) {
    return;
  }

  try {
    let cache = providedCache;
    if (!cache) {
      const session = await storage.getSession();
      const isManaged = await storage.getManagedMode();
      cache = await storage.getBlocklistCache(session?.userId || (isManaged ? "managed-user" : null));
    }

    const evaluation = evaluateUrl(url, cache);
    if (evaluation.isBlocked) {
      const blockedPageUrl =
        chrome.runtime.getURL("blocked.html") +
        `?domain=${encodeURIComponent(evaluation.hostname || "")}` +
        `&url=${encodeURIComponent(url)}`;

      pendingBlockedTabs.set(tabId, url);
      chrome.tabs.update(tabId, { url: blockedPageUrl }, () => {
        // Fallback cleanup if callback fires
        if (chrome.runtime.lastError) {
          pendingBlockedTabs.delete(tabId);
        }
      });
    } else {
      pendingBlockedTabs.delete(tabId);
    }
  } catch (err) {
    pendingBlockedTabs.delete(tabId);
    console.error("[ONUSLY] Error evaluating tab navigation:", err);
  }
}

/**
 * Scans all currently open browser tabs and blocks any that match active restricted targets.
 * Ensures tabs open prior to goal creation or sync are immediately enforced without needing page refresh.
 */
export async function enforceOnOpenTabs() {
  if (!globalThis.chrome?.tabs?.query) return;

  try {
    const session = await storage.getSession();
    const isManaged = await storage.getManagedMode();
    const cache = await storage.getBlocklistCache(session?.userId || (isManaged ? "managed-user" : null));
    if (!cache || !cache.targets || cache.targets.length === 0) return;

    const tabs = await chrome.tabs.query({});
    for (const tab of tabs) {
      if (tab.id && tab.url) {
        await checkAndBlockTab(tab.id, tab.url, cache);
      }
    }
  } catch (err) {
    console.error("[ONUSLY] Error enforcing on open tabs:", err);
  }
}

// 1. Full-page navigation interception
if (globalThis.chrome?.webNavigation?.onBeforeNavigate) {
  chrome.webNavigation.onBeforeNavigate.addListener((details) => {
    if (details.frameId !== 0) return;
    checkAndBlockTab(details.tabId, details.url);
  });
}

// 2. SPA in-page history navigation interception (e.g. YouTube clicking next video, pushState)
if (globalThis.chrome?.webNavigation?.onHistoryStateUpdated) {
  chrome.webNavigation.onHistoryStateUpdated.addListener((details) => {
    if (details.frameId !== 0) return;
    checkAndBlockTab(details.tabId, details.url);
  });
}

// 3. Tab URL change interception (catches explicit URL navigations only)
if (globalThis.chrome?.tabs?.onUpdated) {
  chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
    const currentUrl = changeInfo.url || tab?.url;
    if (currentUrl && currentUrl.includes("blocked.html")) {
      pendingBlockedTabs.delete(tabId);
      return;
    }
    if (changeInfo.status === "complete") {
      pendingBlockedTabs.delete(tabId);
    }

    // CRITICAL: Only evaluate when changeInfo explicitly contains a new URL.
    // Falling back to tab.url on status/favicon updates causes infinite navigation loop storms.
    if (changeInfo.url) {
      checkAndBlockTab(tabId, changeInfo.url);
    }
  });
}

// 4. Tab cleanup on close
if (globalThis.chrome?.tabs?.onRemoved) {
  chrome.tabs.onRemoved.addListener((tabId) => {
    pendingBlockedTabs.delete(tabId);
  });
}

// 5. Tab activation interception (when user switches to an already-open tab)
if (globalThis.chrome?.tabs?.onActivated) {
  chrome.tabs.onActivated.addListener(async (activeInfo) => {
    try {
      const tab = await chrome.tabs.get(activeInfo.tabId);
      if (tab?.url) {
        await checkAndBlockTab(tab.id, tab.url);
      }
    } catch {
      // Tab may have closed
    }
  });
}

// --------------------------------------------------------------------------
// Authentication Handoff via externally_connectable
// --------------------------------------------------------------------------
// Core Message Processing Handlers
// --------------------------------------------------------------------------
export async function processAuthHandoff(message) {
  const { token, user, baseUrl } = message || {};
  if (!token || typeof token !== "string") {
    return { success: false, error: "Missing or invalid token" };
  }

  const payload = parseJwtPayload(token);
  if (!payload || !payload.sub) {
    return { success: false, error: "Invalid JWT token structure" };
  }

  if (baseUrl && typeof baseUrl === "string") {
    await storage.saveSettings({ baseUrl: baseUrl.trim().replace(/\/+$/, "") });
  }

  const userId = payload.sub;
  const currentSession = await storage.getSession();

  // Safe account switching: if switching from User A to User B, clear old cache first
  if (currentSession && currentSession.userId !== userId) {
    await storage.clearAll();
  }

  const session = {
    userId,
    email: payload.email || user?.email || "",
    name: payload.name || user?.name || "User",
    token,
    exp: payload.exp || null,
  };

  await storage.saveSession(session);

  // Immediately trigger blocklist sync and enforce on open tabs
  const syncResult = await syncService.sync();
  if (syncResult.success) {
    await enforceOnOpenTabs();
  }

  return {
    success: true,
    userId,
    version: syncResult.version || null,
    targetsCount: syncResult.targetsCount || 0,
  };
}

export async function processPing() {
  const session = await storage.getSession();
  const cache = await storage.getBlocklistCache(session?.userId);
  const hasActiveTargets =
    cache?.targets &&
    cache.targets.some((t) => isEnforcingStatus(t.status));

  return {
    status: "ok",
    connected: !!session,
    userId: session?.userId || null,
    version: cache?.version || null,
    targetsCount: cache?.targets?.length || 0,
    locked: !!hasActiveTargets,
  };
}

export async function processDisconnect() {
  const session = await storage.getSession();
  const cache = await storage.getBlocklistCache(session?.userId || null);
  const hasActiveTargets =
    cache?.targets &&
    cache.targets.some((t) => isEnforcingStatus(t.status));

  if (hasActiveTargets) {
    return {
      success: false,
      error: "Cannot disconnect extension while goals are in progress.",
    };
  }

  await storage.clearAll();
  return { success: true, message: "Disconnected successfully" };
}

export async function processSync(action = "get-blocklist") {
  // First attempt managed synchronization via Desktop Agent
  const agentSync = await syncFromDesktopAgent(action);
  if (agentSync?.success) {
    return agentSync;
  }

  const syncResult = await syncService.sync();
  if (syncResult.success) {
    await enforceOnOpenTabs();
  }

  return {
    success: syncResult.success,
    version: syncResult.version,
    targetsCount: syncResult.targetsCount,
    error: syncResult.error,
  };
}

/**
 * Queries the central ONUSLY Desktop Agent over Native Messaging.
 * Compatible with Chromium (callbacks) and Firefox (Promises).
 *
 * @param {string} [action="get-blocklist"] - "get-blocklist" or "sync" (on-demand cloud fetch)
 */
export async function syncFromDesktopAgent(action = "get-blocklist") {
  if (globalThis.browser?.runtime?.sendNativeMessage) {
    try {
      const response = await globalThis.browser.runtime.sendNativeMessage(
        "com.onusly.desktop_agent",
        { action }
      );
      if (response && response.success && response.blocklist) {
        const blocklist = response.blocklist;
        await storage.setManagedMode(true);
        await storage.saveBlocklistCache(blocklist, blocklist.userId || "managed-user");
        await enforceOnOpenTabs();
        return {
          success: true,
          managed: true,
          version: blocklist.version,
          targetsCount: (blocklist.targets || []).length,
        };
      }
      return { success: false, error: response?.error || "Agent returned invalid response" };
    } catch (err) {
      await storage.setManagedMode(false);
      return { success: false, error: err.message };
    }
  }

  if (globalThis.chrome?.runtime?.sendNativeMessage) {
    return new Promise((resolve) => {
      try {
        chrome.runtime.sendNativeMessage(
          "com.onusly.desktop_agent",
          { action },
          async (response) => {
            if (chrome.runtime.lastError) {
              await storage.setManagedMode(false);
              resolve({ success: false, error: chrome.runtime.lastError.message });
              return;
            }

            if (response && response.success && response.blocklist) {
              const blocklist = response.blocklist;
              await storage.setManagedMode(true);
              await storage.saveBlocklistCache(blocklist, blocklist.userId || "managed-user");
              await enforceOnOpenTabs();
              resolve({
                success: true,
                managed: true,
                version: blocklist.version,
                targetsCount: (blocklist.targets || []).length,
              });
              return;
            }

            resolve({ success: false, error: response?.error || "Agent returned invalid response" });
          }
        );
      } catch (err) {
        resolve({ success: false, error: err.message });
      }
    });
  }

  return { success: false, error: "Native messaging unsupported" };
}

// --------------------------------------------------------------------------
// Authentication Handoff via externally_connectable
// --------------------------------------------------------------------------
if (globalThis.chrome?.runtime?.onMessageExternal) {
  chrome.runtime.onMessageExternal.addListener((message, sender, sendResponse) => {
    (async () => {
      // 1. Origin verification
      if (!isAuthorizedOrigin(sender)) {
        console.warn("[ONUSLY] Rejected external message from unauthorized origin:", sender?.origin);
        sendResponse({ success: false, error: "Unauthorized sender origin" });
        return;
      }

      if (!message || typeof message !== "object") {
        sendResponse({ success: false, error: "Invalid message payload" });
        return;
      }

      if (message.type === "ONUSLY_PING") {
        sendResponse(await processPing());
        return;
      }

      if (message.type === "ONUSLY_AUTH_HANDOFF") {
        sendResponse(await processAuthHandoff(message));
        return;
      }

      if (message.type === "ONUSLY_DISCONNECT") {
        sendResponse(await processDisconnect());
        return;
      }

      if (message.type === "ONUSLY_SYNC") {
        sendResponse(await processSync());
        return;
      }

      sendResponse({ success: false, error: "Unknown message type" });
    })();

    // Return true for asynchronous sendResponse
    return true;
  });
}

// --------------------------------------------------------------------------
// Internal Extension Messaging (Bridge, Popup & Blocked Page)
// --------------------------------------------------------------------------
if (globalThis.chrome?.runtime?.onMessage) {
  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    (async () => {
      if (!message || typeof message !== "object") return;

      // Handle bridge requests from ONUSLY dashboard content script
      if (message.type === "ONUSLY_PING") {
        sendResponse(await processPing());
        return;
      }

      if (message.type === "ONUSLY_AUTH_HANDOFF") {
        sendResponse(await processAuthHandoff(message));
        return;
      }

      if (message.type === "ONUSLY_DISCONNECT") {
        sendResponse(await processDisconnect());
        return;
      }

      if (message.type === "ONUSLY_SYNC") {
        sendResponse(await processSync());
        return;
      }

      if (message.type === "GET_STATUS") {
        const session = await storage.getSession();
        const isManaged = await storage.getManagedMode();
        const cache = await storage.getBlocklistCache(session?.userId || (isManaged ? "managed-user" : null));
        const settings = await storage.getSettings();
        sendResponse({
          session,
          cache,
          settings,
          isManaged,
        });
        return;
      }

      if (message.type === "SYNC_NOW") {
        const result = await processSync("sync");
        if (result?.success) {
          await enforceOnOpenTabs();
        }
        const session = await storage.getSession();
        const isManaged = await storage.getManagedMode();
        const cache = await storage.getBlocklistCache(session?.userId || (isManaged ? "managed-user" : null));
        sendResponse({
          result,
          session,
          cache,
          isManaged,
        });
        return;
      }

      if (message.type === "DISCONNECT") {
        const result = await processDisconnect();
        sendResponse(result);
        return;
      }

      if (message.type === "EVALUATE_URL") {
        const session = await storage.getSession();
        const isManaged = await storage.getManagedMode();
        const cache = await storage.getBlocklistCache(session?.userId || (isManaged ? "managed-user" : null));
        const evalResult = evaluateUrl(message.url, cache);
        sendResponse(evalResult);
        return;
      }
    })();

    return true;
  });
}

// --------------------------------------------------------------------------
// Lifecycle & Alarms (Periodic Sync & Startup)
// --------------------------------------------------------------------------
if (globalThis.chrome?.runtime?.onInstalled) {
  chrome.runtime.onInstalled.addListener(() => {
    chrome.alarms.create(ALARM_NAME, { periodInMinutes: 1 });
    processSync().then((res) => {
      if (res?.success) enforceOnOpenTabs();
    }).catch(() => {});
  });
}

if (globalThis.chrome?.runtime?.onStartup) {
  chrome.runtime.onStartup.addListener(() => {
    processSync().then((res) => {
      if (res?.success) enforceOnOpenTabs();
    }).catch(() => {});
  });
}

if (globalThis.chrome?.alarms?.onAlarm) {
  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === ALARM_NAME) {
      processSync().then((res) => {
        if (res?.success) enforceOnOpenTabs();
      }).catch(() => {});
    }
  });
}

// Execute initial synchronization on script load
processSync().then((res) => {
  if (res?.success) enforceOnOpenTabs();
}).catch(() => {});
