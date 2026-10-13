/**
 * ONUSLY YouTube Shorts Content Script Loader
 * Dynamically imports the YouTube Shorts ES module content script
 * so that ES module exports work cleanly in Chromium content script context.
 */
(async function () {
  try {
    const src = chrome.runtime.getURL("src/youtubeShortsContent.js");
    await import(src);
  } catch (err) {
    console.error("[ONUSLY] Failed to initialize YouTube Shorts content script:", err);
  }
})();
