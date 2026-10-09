/**
 * ONUSLY — Strict YouTube Shorts Mode Content Script
 *
 * Runs on youtube.com surfaces to detect and hide:
 * 1. Shorts shelves (ytd-reel-shelf-renderer, ytm-reel-shelf-renderer, rich shelves)
 * 2. Individual Shorts video cards across Home, Search, Subscriptions, and Related sections
 * 3. Shorts navigation entry points (sidebar guides, chips, pivot bars)
 *
 * Crucially preserves:
 * - Regular /watch?v=... videos and playlists
 * - Educational channels, search results, and recommendations
 *
 * Designed to be efficient, idempotent, and resilient against infinite observer loops.
 */

// Diagnostic metrics for verification and tests
export const filterStats = {
  hiddenShelves: 0,
  hiddenCards: 0,
  hiddenNavEntries: 0,
  mutationRuns: 0,
};

export function resetFilterStats() {
  filterStats.hiddenShelves = 0;
  filterStats.hiddenCards = 0;
  filterStats.hiddenNavEntries = 0;
  filterStats.mutationRuns = 0;
}

export function getFilterStats() {
  return { ...filterStats };
}

/**
 * CSS rules injected at document_start for instantaneous, zero-layout-shift hiding.
 */
export const SHORTS_CSS = `
/* Hide direct Shorts shelf renderers across desktop & mobile */
ytd-reel-shelf-renderer,
ytm-reel-shelf-renderer,
ytd-rich-shelf-renderer[is-shorts],
ytd-reel-item-renderer,
ytm-reel-item-renderer,

/* Hide sidebar and mini-guide Shorts navigation links */
ytd-guide-entry-renderer:has(a[href*="/shorts"]),
ytd-mini-guide-entry-renderer:has(a[href*="/shorts"]),
ytm-pivot-bar-item-renderer:has(a[href*="/shorts"]),

/* Hide Shorts filter chips */
yt-chip-cloud-chip-renderer:has(yt-formatted-string[title="Shorts"]),
yt-chip-cloud-chip-renderer:has(yt-formatted-string[title="shorts"]),

/* Elements dynamically marked by ONUSLY content script */
[data-onusly-shorts-hidden="true"] {
  display: none !important;
}
`;

/**
 * Injects CSS rules into the document.
 * @param {Document} [doc]
 */
export function injectShortsBlockingStyles(doc = globalThis.document) {
  if (!doc || !doc.head) return;
  const styleId = "onusly-shorts-style";
  if (doc.getElementById(styleId)) return;

  const style = doc.createElement("style");
  style.id = styleId;
  style.textContent = SHORTS_CSS;
  doc.head.appendChild(style);
}

/**
 * Checks whether an element is or is part of an ordinary /watch video card.
 * Used to avoid false positives (e.g. video cards containing text or descriptions mentioning "Shorts").
 *
 * @param {Element} element
 * @returns {boolean}
 */
export function isRegularVideoCard(element) {
  if (!element || typeof element.querySelector !== "function") return false;

  // Check for primary watch video indicators
  const hasWatchThumbnail = element.querySelector(
    'a#thumbnail[href*="/watch"], a.ytd-thumbnail[href*="/watch"]'
  );
  if (hasWatchThumbnail) return true;

  const hasWatchTitle = element.querySelector(
    'a#video-title-link[href*="/watch"], a#video-title[href*="/watch"], a[id*="video-title"][href*="/watch"]'
  );
  if (hasWatchTitle) return true;

  // Check for playlist indicators
  const hasPlaylistLink = element.querySelector(
    'a[href*="/playlist?list="], a#thumbnail[href*="/playlist"]'
  );
  if (hasPlaylistLink) return true;

  return false;
}

/**
 * Determines whether a given DOM element represents a Shorts shelf.
 *
 * @param {Element} element
 * @returns {boolean}
 */
export function isShortsShelf(element) {
  if (!element || typeof element.matches !== "function") return false;

  const tagName = element.tagName ? element.tagName.toLowerCase() : "";

  // 1. Direct shelf tags
  if (tagName === "ytd-reel-shelf-renderer" || tagName === "ytm-reel-shelf-renderer") {
    return true;
  }

  // 2. Rich shelf with is-shorts attribute
  if (tagName === "ytd-rich-shelf-renderer" && element.hasAttribute("is-shorts")) {
    return true;
  }

  // 3. Rich shelf or standard shelf with Shorts header/links
  if (tagName === "ytd-rich-shelf-renderer" || tagName === "ytd-shelf-renderer") {
    // Check title text
    const titleEl = element.querySelector("#title, #title-text, .title, h2");
    if (titleEl && /shorts/i.test(titleEl.textContent || "")) {
      return true;
    }

    // Check if it hosts reel items or has an anchor linking to /shorts
    if (
      element.querySelector("ytd-reel-item-renderer, ytm-reel-item-renderer") ||
      element.querySelector('a[href*="/shorts"]')
    ) {
      return true;
    }
  }

  return false;
}

/**
 * Determines whether a given DOM element represents an individual Shorts video card.
 *
 * @param {Element} element
 * @returns {boolean}
 */
export function isShortsCard(element) {
  if (!element || typeof element.querySelector !== "function") return false;

  // If this card is clearly a regular video card (has a /watch primary link), do NOT treat as Shorts
  if (isRegularVideoCard(element)) {
    return false;
  }

  // Check if thumbnail or primary link is a /shorts/ URL
  const shortsLink = element.querySelector(
    'a[href*="/shorts/"], a[href^="/shorts/"], a#thumbnail[href*="/shorts/"]'
  );

  return !!shortsLink;
}

/**
 * Hides an element cleanly and marks it with ONUSLY tracking attributes.
 *
 * @param {HTMLElement} element
 * @param {"shelf"|"card"|"nav"} type
 */
export function hideShortsElement(element, type = "card") {
  if (!element || element.getAttribute("data-onusly-shorts-hidden") === "true") {
    return;
  }

  element.style.setProperty("display", "none", "important");
  element.setAttribute("data-onusly-shorts-hidden", "true");

  if (type === "shelf") {
    filterStats.hiddenShelves++;
    // If the shelf is housed inside a grid section row wrapper, hide that row wrapper too to prevent blank spaces
    const parentSection = element.closest("ytd-rich-section-renderer");
    if (parentSection && parentSection !== element) {
      parentSection.style.setProperty("display", "none", "important");
      parentSection.setAttribute("data-onusly-shorts-hidden", "true");
    }
  } else if (type === "card") {
    filterStats.hiddenCards++;
  } else if (type === "nav") {
    filterStats.hiddenNavEntries++;
  }
}

/**
 * Scans a DOM container and hides all Shorts shelves, cards, and navigation entries.
 * Idempotent: skips elements already marked with data-onusly-shorts-hidden.
 *
 * @param {Element|Document} [root]
 * @returns {{ hiddenShelves: number, hiddenCards: number, hiddenNavEntries: number }}
 */
export function filterYouTubeShorts(root = globalThis.document) {
  if (!root || typeof root.querySelectorAll !== "function") {
    return getFilterStats();
  }

  // 1. Process Shorts Shelves
  const shelfCandidates = root.querySelectorAll(
    'ytd-reel-shelf-renderer, ytm-reel-shelf-renderer, ytd-rich-shelf-renderer, ytd-shelf-renderer'
  );
  for (const shelf of shelfCandidates) {
    if (isShortsShelf(shelf)) {
      hideShortsElement(shelf, "shelf");
    }
  }

  // 2. Process Individual Shorts Video Cards in Grid / Search / Recommendations
  const shortsAnchors = root.querySelectorAll('a[href*="/shorts/"], a[href^="/shorts/"]');
  for (const anchor of shortsAnchors) {
    // Find the enclosing card container
    const card = anchor.closest(
      'ytd-rich-item-renderer, ytd-video-renderer, ytd-compact-video-renderer, ytd-grid-video-renderer, ytd-reel-item-renderer, ytm-video-with-context-renderer, ytm-compact-video-renderer'
    );

    if (card) {
      // Check if it's not a false positive on a regular video card
      if (isShortsCard(card)) {
        hideShortsElement(card, "card");
      }
    } else {
      // If anchor is in a shelf or container without standard card wrapper
      const shelf = anchor.closest('ytd-reel-shelf-renderer, ytd-rich-shelf-renderer, ytd-shelf-renderer, ytd-rich-section-renderer');
      if (shelf) {
        hideShortsElement(shelf, "shelf");
      }
    }
  }

  // 3. Process Sidebar & Mini-Guide Shorts navigation entries
  const navShortsAnchors = root.querySelectorAll('ytd-guide-entry-renderer a[href*="/shorts"], ytd-mini-guide-entry-renderer a[href*="/shorts"], ytm-pivot-bar-item-renderer a[href*="/shorts"]');
  for (const navAnchor of navShortsAnchors) {
    const navEntry = navAnchor.closest('ytd-guide-entry-renderer, ytd-mini-guide-entry-renderer, ytm-pivot-bar-item-renderer');
    if (navEntry) {
      hideShortsElement(navEntry, "nav");
    }
  }

  // 4. Process Shorts Chips in cloud filter
  const chips = root.querySelectorAll('yt-chip-cloud-chip-renderer');
  for (const chip of chips) {
    if (/shorts/i.test(chip.textContent || "")) {
      hideShortsElement(chip, "nav");
    }
  }

  return getFilterStats();
}

/**
 * Initializes the MutationObserver to handle infinite scrolling and dynamic updates without infinite loops.
 *
 * @param {Document} [doc]
 * @returns {MutationObserver|null}
 */
export function createShortsObserver(doc = globalThis.document) {
  if (!doc || typeof doc.addEventListener !== "function") return null;

  let scheduled = false;

  const scheduleFilter = () => {
    if (scheduled) return;
    scheduled = true;

    const run = () => {
      scheduled = false;
      filterStats.mutationRuns++;
      filterYouTubeShorts(doc);
    };

    if (typeof globalThis.requestAnimationFrame === "function") {
      globalThis.requestAnimationFrame(run);
    } else {
      setTimeout(run, 0);
    }
  };

  const observer = new MutationObserver((mutations) => {
    let relevantMutation = false;

    for (const mutation of mutations) {
      // Ignore mutations created by our own attribute changes to prevent any infinite loops
      if (
        mutation.type === "attributes" &&
        mutation.attributeName &&
        mutation.attributeName.startsWith("data-onusly-")
      ) {
        continue;
      }

      if (mutation.type === "childList" && mutation.addedNodes.length > 0) {
        relevantMutation = true;
        break;
      }
    }

    if (relevantMutation) {
      scheduleFilter();
    }
  });

  const target = doc.documentElement || doc.body;
  if (target) {
    observer.observe(target, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["href", "is-shorts"],
    });
  }

  // Handle YouTube SPA navigation events
  globalThis.addEventListener?.("yt-navigate-finish", scheduleFilter);
  globalThis.addEventListener?.("yt-page-data-updated", scheduleFilter);
  globalThis.addEventListener?.("popstate", scheduleFilter);

  return observer;
}

// Auto-run in browser content script context
if (typeof document !== "undefined" && typeof window !== "undefined") {
  injectShortsBlockingStyles(document);

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      filterYouTubeShorts(document);
      createShortsObserver(document);
    });
  } else {
    filterYouTubeShorts(document);
    createShortsObserver(document);
  }
}
