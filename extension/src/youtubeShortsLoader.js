/**
 * ONUSLY YouTube Shorts Content Script Loader
 * Dynamically imports the YouTube Shorts ES module content script
 * so that ES module exports work cleanly in Chromium content script context.
 */
(async function () {
  try {
    const rt =
      typeof chrome !== "undefined" && chrome.runtime?.getURL
        ? chrome.runtime
        : typeof browser !== "undefined" && browser.runtime?.getURL
        ? browser.runtime
        : null;

    if (!rt) return;
    const src = rt.getURL("src/youtubeShortsContent.js");
    await import(src);
  } catch (err) {
    console.error("[ONUSLY] Failed to initialize YouTube Shorts content script:", err);
  }
})();
