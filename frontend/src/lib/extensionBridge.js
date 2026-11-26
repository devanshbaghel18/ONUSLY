export const DEFAULT_EXTENSION_ID = "jifigaejbinmdhnljobkodcgeaplojmg";

/**
 * Resolves the active ONUSLY extension ID announced by dashboardBridge.js or saved locally.
 */
export function getActiveExtensionId() {
  if (typeof document !== "undefined") {
    const datasetId = document.documentElement?.dataset?.onuslyExtensionId;
    if (datasetId) return datasetId;
  }
  if (typeof localStorage !== "undefined") {
    const saved = localStorage.getItem("onusly_extension_id");
    if (saved) return saved;
  }
  return DEFAULT_EXTENSION_ID;
}

/**
 * Sends a typed request to the ONUSLY browser extension using chrome.runtime.sendMessage
 * or the dashboardBridge.js window.postMessage relay. Resolves to null on timeout/unavailable.
 */
export function sendExtensionBridgeMessage(type, payload = {}, timeoutMs = 2500) {
  if (typeof window === "undefined") {
    return Promise.resolve(null);
  }

  const extId = getActiveExtensionId();
  const requestId = `req_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;

  return new Promise((resolve) => {
    let settled = false;
    const finish = (val) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      window.removeEventListener("message", onBridgeMessage);
      resolve(val);
    };

    const timer = setTimeout(() => finish(null), timeoutMs);

    const onBridgeMessage = (event) => {
      if (event.source !== window || !event.data || typeof event.data !== "object") return;
      if (event.data.type === `${type}_RESPONSE` && event.data.requestId === requestId) {
        if (event.data.error && !event.data.response) {
          finish({ success: false, error: event.data.error });
          return;
        }
        finish(event.data.response || null);
      }
    };

    window.addEventListener("message", onBridgeMessage);

    if (window.chrome?.runtime?.sendMessage) {
      try {
        window.chrome.runtime.sendMessage(extId, { type, ...payload }, (resp) => {
          if (!window.chrome.runtime.lastError && resp) {
            finish(resp);
          }
        });
      } catch {
        // Fall back to window.postMessage bridge
      }
    }

    window.postMessage({ type, requestId, ...payload }, "*");
  });
}
