/**
 * Utility to notify the ONUSLY Chrome extension to sync its blocklist.
 * Dispatches via both content-script window bridge and chrome.runtime.
 * Fails silently if extension is not installed or unreachable.
 */
export function notifyExtensionSync() {
  if (typeof window === "undefined") return;

  const token =
    localStorage.getItem("token") ||
    localStorage.getItem("onusly_auth_token");
  const rawUser =
    localStorage.getItem("user") ||
    localStorage.getItem("onusly_user");
  let user;
  try {
    user = rawUser ? JSON.parse(rawUser) : null;
  } catch {
    user = null;
  }

  const extensionId =
    localStorage.getItem("onusly_extension_id") ||
    "gkichkblphmdbojdjnonljlegnmjcnhg";

  // 1. If authenticated, dispatch credentials so extension is guaranteed connected
  if (token) {
    try {
      window.postMessage(
        {
          type: "ONUSLY_AUTH_HANDOFF",
          token,
          user,
          baseUrl: "http://localhost:8080",
        },
        "*"
      );
    } catch {
      // Fail silently
    }

    if (window.chrome?.runtime?.sendMessage) {
      try {
        window.chrome.runtime.sendMessage(
          extensionId,
          {
            type: "ONUSLY_AUTH_HANDOFF",
            token,
            user,
            baseUrl: "http://localhost:8080",
          },
          () => {
            void window.chrome.runtime.lastError;
          }
        );
      } catch {
        // Fail silently
      }
    }
  }

  // 2. Dispatch sync via DOM / window bridge for content script
  try {
    window.postMessage({ type: "ONUSLY_SYNC" }, "*");
  } catch {
    // Fail silently
  }

  // 3. Dispatch sync via direct chrome.runtime.sendMessage
  if (window.chrome?.runtime?.sendMessage) {
    try {
      window.chrome.runtime.sendMessage(
        extensionId,
        { type: "ONUSLY_SYNC" },
        () => {
          if (window.chrome.runtime.lastError) {
            // Suppress lastError if extension is inactive
          }
        }
      );
    } catch {
      // Fail silently
    }
  }
}
