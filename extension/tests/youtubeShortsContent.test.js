import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import {
  filterYouTubeShorts,
  isShortsCard,
  isShortsShelf,
  isRegularVideoCard,
  resetFilterStats,
  getFilterStats,
  injectShortsBlockingStyles,
} from "../src/youtubeShortsContent.js";

/**
 * Lightweight mock DOM Element to test YouTube content filtering in Node.js.
 */
class MockElement {
  constructor(tagName, attributes = {}, textContent = "") {
    this.tagName = tagName.toUpperCase();
    this.attributes = { ...attributes };
    this.textContent = textContent;
    this.children = [];
    this.parent = null;
    this.style = {
      display: "",
      setProperty: (prop, val) => {
        this.style[prop] = val;
      },
    };
  }

  get id() {
    return this.getAttribute("id") || "";
  }

  set id(val) {
    this.setAttribute("id", val);
  }

  getAttribute(name) {
    return this.attributes[name] ?? null;
  }

  setAttribute(name, value) {
    this.attributes[name] = String(value);
  }

  hasAttribute(name) {
    return name in this.attributes;
  }

  appendChild(child) {
    child.parent = this;
    this.children.push(child);
    return child;
  }

  matches(selector) {
    const sel = selector.trim();
    if (!sel) return false;

    // Check tag (if present at start)
    const tagMatch = sel.match(/^[a-zA-Z0-9_-]+/);
    if (tagMatch) {
      if (this.tagName.toLowerCase() !== tagMatch[0].toLowerCase()) {
        return false;
      }
    }

    // Check #id
    const idMatch = sel.match(/#([a-zA-Z0-9_-]+)/);
    if (idMatch) {
      if (this.getAttribute("id") !== idMatch[1]) {
        return false;
      }
    }

    // Check .class
    const classMatches = sel.matchAll(/\.([a-zA-Z0-9_-]+)/g);
    for (const cm of classMatches) {
      const classAttr = this.getAttribute("class") || "";
      if (!classAttr.split(/\s+/).includes(cm[1])) {
        return false;
      }
    }

    // Check [attr op= val]
    const attrMatches = sel.matchAll(/\[([a-zA-Z0-9_-]+)(?:([*^$]?=)(?:["']?)(.*?)(?:["']?))?\]/g);
    for (const am of attrMatches) {
      const attrName = am[1];
      const op = am[2];
      const expectedVal = am[3];

      if (!this.hasAttribute(attrName)) {
        return false;
      }

      if (op) {
        const actualVal = this.getAttribute(attrName) || "";
        if (op === "=" && actualVal !== expectedVal) return false;
        if (op === "*=" && !actualVal.includes(expectedVal)) return false;
        if (op === "^=" && !actualVal.startsWith(expectedVal)) return false;
        if (op === "$=" && !actualVal.endsWith(expectedVal)) return false;
      }
    }

    return true;
  }

  closest(selector) {
    let curr = this;
    const selectors = selector.split(",").map((s) => s.trim());
    while (curr) {
      for (const sel of selectors) {
        if (curr.matches(sel)) {
          return curr;
        }
      }
      curr = curr.parent;
    }
    return null;
  }

  querySelector(selector) {
    const list = this.querySelectorAll(selector);
    return list.length > 0 ? list[0] : null;
  }

  querySelectorAll(selector) {
    const results = [];
    const selectors = selector.split(",").map((s) => s.trim());

    const traverse = (node) => {
      for (const child of node.children) {
        for (const sel of selectors) {
          if (child.matches(sel)) {
            results.push(child);
            break;
          }
        }
        traverse(child);
      }
    };

    traverse(this);
    return results;
  }
}

describe("Strict YouTube Shorts Feed & Content Script Suite", () => {
  beforeEach(() => {
    resetFilterStats();
  });

  it("1. A Shorts card is detected and hidden", () => {
    const root = new MockElement("div");

    // Create a Shorts card (ytd-rich-item-renderer)
    const shortsCard = new MockElement("ytd-rich-item-renderer");
    const thumbnailAnchor = new MockElement("a", {
      id: "thumbnail",
      href: "/shorts/video_xyz",
    });
    shortsCard.appendChild(thumbnailAnchor);
    root.appendChild(shortsCard);

    assert.equal(isShortsCard(shortsCard), true);

    filterYouTubeShorts(root);

    assert.equal(shortsCard.style.display, "none");
    assert.equal(shortsCard.getAttribute("data-onusly-shorts-hidden"), "true");
    assert.equal(getFilterStats().hiddenCards, 1);
  });

  it("2. A Shorts shelf is removed or hidden as a whole", () => {
    const root = new MockElement("div");

    // Create an entire Shorts shelf row (ytd-reel-shelf-renderer inside rich section)
    const sectionWrapper = new MockElement("ytd-rich-section-renderer");
    const shelf = new MockElement("ytd-reel-shelf-renderer");
    const title = new MockElement("span", { id: "title" }, "Shorts");
    shelf.appendChild(title);
    sectionWrapper.appendChild(shelf);
    root.appendChild(sectionWrapper);

    assert.equal(isShortsShelf(shelf), true);

    filterYouTubeShorts(root);

    assert.equal(shelf.style.display, "none");
    assert.equal(shelf.getAttribute("data-onusly-shorts-hidden"), "true");
    assert.equal(sectionWrapper.style.display, "none", "Parent section wrapper must also be hidden");
    assert.equal(getFilterStats().hiddenShelves, 1);
  });

  it("3. A regular video card remains visible", () => {
    const root = new MockElement("div");

    // Regular video card pointing to /watch?v=...
    const regularCard = new MockElement("ytd-rich-item-renderer");
    const thumbnail = new MockElement("a", {
      id: "thumbnail",
      href: "/watch?v=educational_lecture_123",
    });
    const title = new MockElement("a", {
      id: "video-title-link",
      href: "/watch?v=educational_lecture_123",
    }, "Algorithms and Data Structures");

    regularCard.appendChild(thumbnail);
    regularCard.appendChild(title);
    root.appendChild(regularCard);

    assert.equal(isRegularVideoCard(regularCard), true);
    assert.equal(isShortsCard(regularCard), false);

    filterYouTubeShorts(root);

    assert.equal(regularCard.style.display, "", "Regular video must NOT be hidden");
    assert.equal(regularCard.getAttribute("data-onusly-shorts-hidden"), null);
    assert.equal(getFilterStats().hiddenCards, 0);
  });

  it("4. A regular video card containing unrelated text mentioning Shorts remains visible", () => {
    const root = new MockElement("div");

    // Regular video card with description mentioning 'shorts' or 'bermuda shorts'
    const regularCard = new MockElement("ytd-video-renderer");
    const thumbnail = new MockElement("a", {
      id: "thumbnail",
      href: "/watch?v=fashion_history_456",
    });
    const title = new MockElement("a", {
      id: "video-title",
      href: "/watch?v=fashion_history_456",
    }, "History of Summer Shorts and Clothing");

    const description = new MockElement("span", {
      id: "description-text",
    }, "This documentary explains how shorts evolved over time.");

    regularCard.appendChild(thumbnail);
    regularCard.appendChild(title);
    regularCard.appendChild(description);
    root.appendChild(regularCard);

    assert.equal(isRegularVideoCard(regularCard), true);
    assert.equal(isShortsCard(regularCard), false);

    filterYouTubeShorts(root);

    assert.equal(regularCard.style.display, "", "Must not hide regular video mentioning shorts in text");
    assert.equal(getFilterStats().hiddenCards, 0);
  });

  it("5. Dynamically inserted Shorts cards are hidden", () => {
    const root = new MockElement("div");

    // Initial regular video
    const video1 = new MockElement("ytd-rich-item-renderer");
    video1.appendChild(new MockElement("a", { id: "thumbnail", href: "/watch?v=vid1" }));
    root.appendChild(video1);

    filterYouTubeShorts(root);
    assert.equal(getFilterStats().hiddenCards, 0);

    // Simulate infinite scroll: new Short dynamically inserted into feed
    const dynamicShort = new MockElement("ytd-rich-item-renderer");
    dynamicShort.appendChild(new MockElement("a", { id: "thumbnail", href: "/shorts/scroll_short_789" }));
    root.appendChild(dynamicShort);

    // Rescan after mutation
    filterYouTubeShorts(root);

    assert.equal(dynamicShort.style.display, "none");
    assert.equal(dynamicShort.getAttribute("data-onusly-shorts-hidden"), "true");
    assert.equal(getFilterStats().hiddenCards, 1);
  });

  it("6. Repeated observer callbacks do not cause an infinite loop or duplicate work", () => {
    const root = new MockElement("div");

    const shelf = new MockElement("ytd-reel-shelf-renderer");
    root.appendChild(shelf);

    // First scan
    filterYouTubeShorts(root);
    assert.equal(getFilterStats().hiddenShelves, 1);

    // Second scan (idempotency check)
    filterYouTubeShorts(root);
    assert.equal(
      getFilterStats().hiddenShelves,
      1,
      "Second scan must not double count or re-hide already hidden elements"
    );

    // Third scan
    filterYouTubeShorts(root);
    assert.equal(getFilterStats().hiddenShelves, 1);
  });

  it("7. Reprocessing an already-processed feed is safe", () => {
    const root = new MockElement("div");

    const shortsCard = new MockElement("ytd-compact-video-renderer");
    shortsCard.appendChild(new MockElement("a", { href: "/shorts/related_short_1" }));

    const regularCard = new MockElement("ytd-compact-video-renderer");
    regularCard.appendChild(new MockElement("a", { id: "thumbnail", href: "/watch?v=related_vid_2" }));

    root.appendChild(shortsCard);
    root.appendChild(regularCard);

    // Process feed 5 times consecutively
    for (let i = 0; i < 5; i++) {
      filterYouTubeShorts(root);
    }

    assert.equal(shortsCard.style.display, "none");
    assert.equal(regularCard.style.display, "");
    assert.equal(getFilterStats().hiddenCards, 1);
  });

  it("8. Navigation between YouTube sections triggers the required rescan", () => {
    const pageContainer = new MockElement("div");

    // User is on /feed/subscriptions: 1 Shorts shelf
    const subsShelf = new MockElement("ytd-reel-shelf-renderer");
    pageContainer.appendChild(subsShelf);

    filterYouTubeShorts(pageContainer);
    assert.equal(subsShelf.style.display, "none");
    assert.equal(getFilterStats().hiddenShelves, 1);

    // User navigates via SPA to /results?search_query=python:
    // YouTube clears pageContainer and loads search results with Shorts cards
    pageContainer.children = []; // clear
    const searchResultShort = new MockElement("ytd-video-renderer");
    searchResultShort.appendChild(new MockElement("a", { href: "/shorts/search_short" }));
    pageContainer.appendChild(searchResultShort);

    // SPA navigation event triggers filterYouTubeShorts
    filterYouTubeShorts(pageContainer);

    assert.equal(searchResultShort.style.display, "none");
    assert.equal(getFilterStats().hiddenCards, 1);
  });

  it("9. Injects CSS rules into document head", () => {
    const fakeHead = new MockElement("head");
    const fakeDoc = {
      head: fakeHead,
      getElementById: (id) => fakeHead.children.find((c) => c.getAttribute("id") === id) || null,
      createElement: (tag) => new MockElement(tag),
    };

    injectShortsBlockingStyles(fakeDoc);
    assert.equal(fakeHead.children.length, 1);
    assert.equal(fakeHead.children[0].getAttribute("id"), "onusly-shorts-style");

    // Second injection is no-op
    injectShortsBlockingStyles(fakeDoc);
    assert.equal(fakeHead.children.length, 1);
  });
});
