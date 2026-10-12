/**
 * ONUSLY Instant Page Blocker Content Script
 * Executes at document_start on target websites to immediately evaluate
 * against active goal locks and redirect to blocked.html with zero delay.
 */

(function () {
  const rt =
    typeof chrome !== "undefined" && chrome.runtime?.sendMessage
      ? chrome.runtime
      : typeof browser !== "undefined" && browser.runtime?.sendMessage
      ? browser.runtime
      : null;

  if (!rt) return;

  try {
    rt.sendMessage(
      { type: "EVALUATE_URL", url: window.location.href },
      (res) => {
        if (res?.isBlocked) {
          const blockedPageUrl =
            rt.getURL("blocked.html") +
            `?domain=${encodeURIComponent(res.hostname || "")}` +
            `&url=${encodeURIComponent(window.location.href)}`;

          window.location.replace(blockedPageUrl);
        }
      }
    );
  } catch {
    // Fail gracefully
  }
})();
