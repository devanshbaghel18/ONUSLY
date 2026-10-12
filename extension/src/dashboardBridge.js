/**
 * ONUSLY Dashboard Extension Bridge Content Script
 * Injected into ONUSLY frontend pages on localhost, 127.0.0.1, and onusly.com.
 * Establishes bidirectional communication between the web dashboard and extension background worker.
 */

function getExtRuntime() {
  if (typeof chrome !== "undefined" && chrome.runtime?.sendMessage) {
    return chrome.runtime;
  }
  if (typeof browser !== "undefined" && browser.runtime?.sendMessage) {
    return browser.runtime;
  }
  return null;
}

function announceExtension() {
  const rt = getExtRuntime();
  const extId = rt?.id || "";

  if (typeof document !== "undefined" && document.documentElement) {
    document.documentElement.dataset.onuslyExtensionInstalled = "true";
    document.documentElement.dataset.onuslyExtensionId = extId;
  }

  window.postMessage(
    {
      type: "ONUSLY_EXTENSION_ANNOUNCE",
      extensionId: extId,
      version: rt?.getManifest?.()?.version || "1.0.0",
    },
    "*"
  );
}

// Announce immediately and when DOM is loaded
announceExtension();
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", announceExtension);
}

// Listen for message events from dashboard page
window.addEventListener("message", (event) => {
  if (event.source !== window || !event.data || typeof event.data !== "object") {
    return;
  }

  const { type, requestId, ...payload } = event.data;

  if (
    type === "ONUSLY_PING" ||
    type === "ONUSLY_AUTH_HANDOFF" ||
    type === "ONUSLY_DISCONNECT" ||
    type === "ONUSLY_SYNC"
  ) {
    const rt = getExtRuntime();
    if (!rt) {
      window.postMessage(
        {
          type: `${type}_RESPONSE`,
          requestId,
          extensionId: "",
          error: "Extension runtime not available",
          response: null,
        },
        "*"
      );
      return;
    }

    try {
      rt.sendMessage({ type, ...payload }, (response) => {
        const lastErr = rt.lastError;
        window.postMessage(
          {
            type: `${type}_RESPONSE`,
            requestId,
            extensionId: rt.id,
            error: lastErr ? lastErr.message : null,
            response,
          },
          "*"
        );
      });
    } catch (err) {
      window.postMessage(
        {
          type: `${type}_RESPONSE`,
          requestId,
          extensionId: rt.id,
          error: err.message,
          response: null,
        },
        "*"
      );
    }
  }
});

// Allow extension popup to retrieve active session directly from the page
const rt = getExtRuntime();
if (rt?.onMessage) {
  rt.onMessage.addListener((message, sender, sendResponse) => {
    if (message?.type === "GET_AUTH_FROM_PAGE") {
      try {
        const token =
          localStorage.getItem("token") ||
          localStorage.getItem("onusly_auth_token");
        const rawUser =
          localStorage.getItem("user") ||
          localStorage.getItem("onusly_user");
        let user = null;
        try {
          user = rawUser ? JSON.parse(rawUser) : null;
        } catch {
          user = null;
        }
        sendResponse({ success: !!token, token, user });
      } catch (err) {
        sendResponse({ success: false, error: err.message });
      }
      return true;
    }
  });
}
