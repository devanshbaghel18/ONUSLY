/**
 * Pure evaluation engine for ONUSLY website blocking.
 */

import { extractHostname, isDomainMatch, isProtectedUrl, isYouTubeShortsUrl } from "./domainMatcher.js";

export const APP_DOMAINS_MAP = {
  "com.google.android.youtube": ["youtube.com", "youtu.be"],
  "youtube": ["youtube.com", "youtu.be"],
  "com.instagram.android": ["instagram.com"],
  "instagram": ["instagram.com"],
  "com.twitter.android": ["x.com", "twitter.com"],
  "twitter": ["x.com", "twitter.com"],
  "x": ["x.com", "twitter.com"],
  "com.zhiliaoapp.musically": ["tiktok.com"],
  "tiktok": ["tiktok.com"],
  "com.reddit.frontpage": ["reddit.com"],
  "reddit": ["reddit.com"],
  "com.netflix.mediaclient": ["netflix.com"],
  "netflix": ["netflix.com"],
  "com.discord": ["discord.com"],
  "discord": ["discord.com"],
  "tv.twitch.android.app": ["twitch.tv"],
  "twitch": ["twitch.tv"],
};

/**
 * Evaluates whether a given URL should be blocked based on the current cached blocklist
 * and built-in distraction rules (e.g. Strict YouTube Shorts mode).
 *
 * @param {string} urlStr - The URL to evaluate.
 * @param {object|null} cachedBlocklist - The currently enforced blocklist cache.
 * @returns {{
 *   isBlocked: boolean,
 *   hostname: string|null,
 *   matchingTargets: Array<object>,
 *   status: string,
 *   instruction: string,
 *   reason?: string
 * }}
 */
export function evaluateUrl(urlStr, cachedBlocklist) {
  if (isProtectedUrl(urlStr)) {
    return {
      isBlocked: false,
      hostname: extractHostname(urlStr),
      matchingTargets: [],
      status: "none",
      instruction: "",
      reason: "protected_url",
    };
  }

  const hostname = extractHostname(urlStr);
  if (!hostname) {
    return {
      isBlocked: false,
      hostname: null,
      matchingTargets: [],
      status: "none",
      instruction: "",
      reason: "invalid_hostname",
    };
  }

  const matchingTargets = [];

  if (
    cachedBlocklist &&
    Array.isArray(cachedBlocklist.targets) &&
    cachedBlocklist.targets.length > 0
  ) {
    for (const target of cachedBlocklist.targets) {
      const status = (target.status || "").toLowerCase().trim();
      // Only goals in 'active' or 'proof_submitted' status enforce restrictions
      if (status !== "active" && status !== "proof_submitted") {
        continue;
      }

      const domains = [...(Array.isArray(target.domains) ? target.domains : [])];
      for (const app of target.apps || []) {
        const mapped = APP_DOMAINS_MAP[app?.toLowerCase()?.trim()];
        if (mapped) {
          domains.push(...mapped);
        }
      }

      const matched = domains.some((domain) => isDomainMatch(hostname, domain));

      if (matched) {
        matchingTargets.push(target);
      }
    }
  }

  // 1. Goal-based blocklist takes precedence if domain is explicitly restricted by user goals
  if (matchingTargets.length > 0) {
    const hasActive = matchingTargets.some(
      (t) => (t.status || "").toLowerCase().trim() === "active"
    );

    const status = hasActive ? "active" : "proof_submitted";
    const instruction = hasActive
      ? "Complete your goal and submit proof."
      : "Proof submitted. Waiting for review.";

    return {
      isBlocked: true,
      hostname,
      matchingTargets,
      status,
      instruction,
      reason: "goal_target",
    };
  }

  // 2. Built-in Strict YouTube Shorts Mode: block direct Shorts navigation while preserving regular YouTube
  if (isYouTubeShortsUrl(urlStr)) {
    return {
      isBlocked: true,
      hostname,
      matchingTargets: [],
      status: "shorts_blocked",
      reason: "youtube_shorts",
      instruction:
        "YouTube Shorts are blocked so you can stay focused. Regular YouTube videos are still available.",
    };
  }

  return {
    isBlocked: false,
    hostname,
    matchingTargets: [],
    status: "none",
    instruction: "",
  };
}

/**
 * Extracts a deduplicated list of all currently blocked domains across active/proof_submitted targets.
 *
 * @param {object|null} cachedBlocklist
 * @returns {Array<string>}
 */
export function getBlockedDomainsList(cachedBlocklist) {
  if (!cachedBlocklist || !Array.isArray(cachedBlocklist.targets)) {
    return [];
  }

  const domainSet = new Set();

  for (const target of cachedBlocklist.targets) {
    const status = (target.status || "").toLowerCase().trim();
    if (status !== "active" && status !== "proof_submitted") {
      continue;
    }

    const hasDomains = Array.isArray(target.domains) && target.domains.length > 0;
    if (hasDomains) {
      for (const d of target.domains) {
        if (d && typeof d === "string") {
          domainSet.add(d.trim().toLowerCase());
        }
      }
    } else if (Array.isArray(target.apps)) {
      for (const app of target.apps) {
        const mapped = APP_DOMAINS_MAP[app?.toLowerCase()?.trim()];
        if (mapped) {
          for (const d of mapped) {
            domainSet.add(d.trim().toLowerCase());
          }
        }
      }
    }
  }

  return Array.from(domainSet).sort();
}
