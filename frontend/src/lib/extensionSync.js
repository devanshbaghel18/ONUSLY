/**
 * Utility to notify the ONUSLY Chrome extension to sync its blocklist.
 * Fails silently if extension is not installed or unreachable.
 */
export function notifyExtensionSync() {
  if (typeof window === "undefined" || !window.chrome?.runtime?.sendMessage) {
    return;
  }

  const extensionId =
    localStorage.getItem("onusly_extension_id") ||
    "gkichkblphmdbojdjnonljlegnmjcnhg";

  try {
    window.chrome.runtime.sendMessage(
      extensionId,
      { type: "ONUSLY_SYNC" },
      () => {
        // Suppress lastError if extension is inactive or not present
        if (window.chrome.runtime.lastError) {
          // Expected when extension is not installed
        }
      }
    );
  } catch {
    // Fail silently
  }
}
