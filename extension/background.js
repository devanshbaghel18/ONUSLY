/**
 * ONUSLY Browser Extension Service Worker (Manifest V3)
 */

import { createStorage } from "./src/storage.js";
import { createSyncService } from "./src/syncService.js";
import { evaluateUrl } from "./src/blockerEngine.js";

const storage = createStorage();
const syncService = createSyncService(storage);

const ALARM_NAME = "onusly_sync_alarm";
const ALLOWED_ORIGIN_PATTERNS = [
  /^http:\/\/localhost:(5173|8080|3000)$/,
  /^http:\/\/127\.0\.0\.1:(5173|8080|3000)$/,
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

/**
 * Evaluates a tab's URL against active restrictions and redirects to blocked.html if blocked.
 * @param {number} tabId
 * @param {string} url
 * @param {object|null} [providedCache]
 */
export async function checkAndBlockTab(tabId, url, providedCache = null) {
  if (!tabId || !url) return;
  try {
    let cache = providedCache;
    if (!cache) {
      const session = await storage.getSession();
      cache = await storage.getBlocklistCache(session?.userId || null);
    }

    const evaluation = evaluateUrl(url, cache);
    if (evaluation.isBlocked) {
      const blockedPageUrl =
        chrome.runtime.getURL("blocked.html") +
        `?domain=${encodeURIComponent(evaluation.hostname || "")}` +
        `&url=${encodeURIComponent(url)}`;

      chrome.tabs.update(tabId, { url: blockedPageUrl });
    }
  } catch (err) {
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
    const cache = await storage.getBlocklistCache(session?.userId || null);
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

// 3. Tab URL change interception (catches dynamic client updates)
if (globalThis.chrome?.tabs?.onUpdated) {
  chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
    const url = changeInfo.url || tab?.url;
    if (url) {
      checkAndBlockTab(tabId, url);
    }
  });
}

// 4. Tab activation interception (when user switches to an already-open tab)
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

      // Handshake / Ping
      if (message.type === "ONUSLY_PING") {
        const session = await storage.getSession();
        const cache = await storage.getBlocklistCache(session?.userId);
        sendResponse({
          status: "ok",
          connected: !!session,
          userId: session?.userId || null,
          version: cache?.version || null,
          targetsCount: cache?.targets?.length || 0,
        });
        return;
      }

      // Authentication Handoff from Dashboard
      if (message.type === "ONUSLY_AUTH_HANDOFF") {
        const { token, user } = message;
        if (!token || typeof token !== "string") {
          sendResponse({ success: false, error: "Missing or invalid token" });
          return;
        }

        const payload = parseJwtPayload(token);
        if (!payload || !payload.sub) {
          sendResponse({ success: false, error: "Invalid JWT token structure" });
          return;
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

        sendResponse({
          success: true,
          userId,
          version: syncResult.version || null,
          targetsCount: syncResult.targetsCount || 0,
        });
        return;
      }

      // Disconnect
      if (message.type === "ONUSLY_DISCONNECT") {
        await storage.clearAll();
        sendResponse({ success: true, message: "Disconnected successfully" });
        return;
      }

      // External Sync Trigger from Dashboard (e.g. after goal approval/unlock/creation)
      if (message.type === "ONUSLY_SYNC") {
        const syncResult = await syncService.sync();
        if (syncResult.success) {
          await enforceOnOpenTabs();
        }

        sendResponse({
          success: syncResult.success,
          version: syncResult.version,
          targetsCount: syncResult.targetsCount,
          error: syncResult.error,
        });
        return;
      }

      sendResponse({ success: false, error: "Unknown message type" });
    })();

    // Return true for asynchronous sendResponse
    return true;
  });
}

// --------------------------------------------------------------------------
// Internal Extension Messaging (Popup & Blocked Page)
// --------------------------------------------------------------------------
if (globalThis.chrome?.runtime?.onMessage) {
  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    (async () => {
      if (!message || typeof message !== "object") return;

      if (message.type === "GET_STATUS") {
        const session = await storage.getSession();
        const cache = await storage.getBlocklistCache(session?.userId);
        const settings = await storage.getSettings();
        sendResponse({
          session,
          cache,
          settings,
        });
        return;
      }

      if (message.type === "SYNC_NOW") {
        const result = await syncService.sync();
        if (result?.success) {
          await enforceOnOpenTabs();
        }
        const session = await storage.getSession();
        const cache = await storage.getBlocklistCache(session?.userId);
        sendResponse({
          result,
          session,
          cache,
        });
        return;
      }

      if (message.type === "DISCONNECT") {
        await storage.clearAll();
        sendResponse({ success: true });
        return;
      }

      if (message.type === "EVALUATE_URL") {
        const session = await storage.getSession();
        const cache = await storage.getBlocklistCache(session?.userId);
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
    syncService.sync().then((res) => {
      if (res?.success) enforceOnOpenTabs();
    }).catch(() => {});
  });
}

if (globalThis.chrome?.runtime?.onStartup) {
  chrome.runtime.onStartup.addListener(() => {
    syncService.sync().then((res) => {
      if (res?.success) enforceOnOpenTabs();
    }).catch(() => {});
  });
}

if (globalThis.chrome?.alarms?.onAlarm) {
  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === ALARM_NAME) {
      syncService.sync().then((res) => {
        if (res?.success) enforceOnOpenTabs();
      }).catch(() => {});
    }
  });
}
