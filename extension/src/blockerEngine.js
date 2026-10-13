/**
 * Pure evaluation engine for ONUSLY website blocking.
 */

import { extractHostname, isDomainMatch, isProtectedUrl, isYouTubeShortsUrl } from "./domainMatcher.js";

export const APP_DOMAINS_MAP = {
  "com.google.android.youtube": ["youtube.com", "youtu.be"],
  "youtube": ["youtube.com", "youtu.be"],
  "com.google.android.youtube.shorts": ["shorts.youtube.com"],
  "youtube_shorts": ["shorts.youtube.com"],
  "youtube-shorts": ["shorts.youtube.com"],
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
 * Checks whether an active target explicitly restricts YouTube Shorts (or full YouTube).
 *
 * @param {object} target
 * @returns {boolean}
 */
export function isTargetShortsRestricted(target) {
  if (!target) return false;
  const status = (target.status || "").toLowerCase().trim();
  if (status !== "active" && status !== "proof_submitted") {
    return false;
  }

  const domains = Array.isArray(target.domains) ? target.domains : [];
  const apps = Array.isArray(target.apps) ? target.apps : [];

  // 1. Directly restricts YouTube Shorts
  const hasShortsDomain = domains.some((d) => {
    const clean = (d || "").toLowerCase().trim();
    return (
      clean === "shorts.youtube.com" ||
      clean === "youtube.com/shorts" ||
      clean === "m.youtube.com/shorts"
    );
  });
  if (hasShortsDomain) return true;

  const hasShortsApp = apps.some((a) => {
    const clean = (a || "").toLowerCase().trim();
    return (
      clean === "com.google.android.youtube.shorts" ||
      clean === "youtube_shorts" ||
      clean === "youtube-shorts"
    );
  });
  if (hasShortsApp) return true;

  // 2. Full YouTube restriction also restricts Shorts
  const hasFullYouTubeDomain = domains.some((d) => {
    const clean = (d || "").toLowerCase().trim();
    return isDomainMatch(clean, "youtube.com") || clean === "youtu.be";
  });
  if (hasFullYouTubeDomain) return true;

  const hasFullYouTubeApp = apps.some((a) => {
    const clean = (a || "").toLowerCase().trim();
    return clean === "com.google.android.youtube" || clean === "youtube";
  });
  if (hasFullYouTubeApp) return true;

  return false;
}

/**
 * Returns all currently active or proof_submitted targets that restrict YouTube Shorts or full YouTube.
 *
 * @param {object|null} cachedBlocklist
 * @returns {Array<object>}
 */
export function getActiveShortsTargets(cachedBlocklist) {
  if (!cachedBlocklist || !Array.isArray(cachedBlocklist.targets)) {
    return [];
  }
  return cachedBlocklist.targets.filter(isTargetShortsRestricted);
}

/**
 * Checks whether YouTube Shorts is currently restricted by any active goal.
 *
 * @param {object|null} cachedBlocklist
 * @returns {boolean}
 */
export function isShortsRestricted(cachedBlocklist) {
  return getActiveShortsTargets(cachedBlocklist).length > 0;
}

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
 *   reason?: string,
 *   isShortsRestricted?: boolean
 * }}
 */
export function evaluateUrl(urlStr, cachedBlocklist) {
  const shortsRestricted = isShortsRestricted(cachedBlocklist);

  if (isProtectedUrl(urlStr)) {
    return {
      isBlocked: false,
      hostname: extractHostname(urlStr),
      matchingTargets: [],
      status: "none",
      instruction: "",
      reason: "protected_url",
      isShortsRestricted: shortsRestricted,
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
      isShortsRestricted: shortsRestricted,
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
      isShortsRestricted: shortsRestricted,
    };
  }

  // 2. Strict YouTube Shorts Mode: ONLY block if Shorts is restricted by an active goal!
  if (isYouTubeShortsUrl(urlStr)) {
    const shortsTargets = getActiveShortsTargets(cachedBlocklist);
    if (shortsTargets.length > 0) {
      const hasActive = shortsTargets.some(
        (t) => (t.status || "").toLowerCase().trim() === "active"
      );
      const status = hasActive ? "active" : "proof_submitted";
      const instruction = hasActive
        ? "YouTube Shorts are blocked so you can stay focused. Regular YouTube videos are still available."
        : "Proof submitted. Waiting for review.";

      return {
        isBlocked: true,
        hostname,
        matchingTargets: shortsTargets,
        status,
        reason: "youtube_shorts",
        instruction,
        isShortsRestricted: true,
      };
    }
  }

  return {
    isBlocked: false,
    hostname,
    matchingTargets: [],
    status: "none",
    instruction: "",
    isShortsRestricted: shortsRestricted,
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
