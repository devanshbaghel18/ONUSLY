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

  // Never block the blocked page itself
  if (window.location.href.includes("blocked.html")) return;

  let hasRedirected = false;

  const performBlock = (res) => {
    if (hasRedirected) return;
    hasRedirected = true;

    // Immediately stop execution and freeze any active media to prevent buffering glitches
    try {
      window.stop();
    } catch {}

    try {
      document.querySelectorAll("video, audio").forEach((media) => {
        try {
          media.pause();
          media.muted = true;
          media.src = "";
          media.load();
        } catch {}
      });
    } catch {}

    const blockedPageUrl =
      rt.getURL("blocked.html") +
      `?domain=${encodeURIComponent(res.hostname || "")}` +
      `&url=${encodeURIComponent(window.location.href)}`;

    window.location.replace(blockedPageUrl);
  };

  try {
    rt.sendMessage(
      { type: "EVALUATE_URL", url: window.location.href },
      (res) => {
        if (res?.isBlocked) {
          performBlock(res);
        }
      }
    );
  } catch {
    // Fail gracefully
  }
})();
