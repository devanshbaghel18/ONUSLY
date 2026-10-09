/**
 * Domain matching and protected URL utilities for ONUSLY extension.
 */

/**
 * Normalizes a domain or host string (trims, converts to lowercase, strips trailing dot and ports).
 * @param {string} domain
 * @returns {string}
 */
export function normalizeDomain(domain) {
  if (!domain || typeof domain !== "string") return "";
  let clean = domain.trim().toLowerCase();
  // Strip scheme if inadvertently included
  clean = clean.replace(/^[a-z]+:\/\//i, "");
  // Strip path/query if present
  clean = clean.split("/")[0].split("?")[0].split("#")[0];
  // Strip port
  clean = clean.split(":")[0];
  // Strip trailing dot
  clean = clean.replace(/\.+$/, "");
  return clean;
}

/**
 * Checks if a given hostname matches a target blocked domain.
 * Supports exact domain matches and any subdomains (e.g., 'm.youtube.com' matches 'youtube.com').
 * Strictly prevents false positives (e.g., 'notyoutube.com' does NOT match 'youtube.com').
 *
 * @param {string} hostname - The hostname of the page being visited.
 * @param {string} targetDomain - The target domain from the blocklist.
 * @returns {boolean}
 */
export function isDomainMatch(hostname, targetDomain) {
  const host = normalizeDomain(hostname);
  const target = normalizeDomain(targetDomain);

  if (!host || !target) return false;

  // Exact match: 'youtube.com' === 'youtube.com'
  if (host === target) return true;

  // Subdomain match: 'www.youtube.com' ends with '.youtube.com'
  if (host.endsWith("." + target)) return true;

  return false;
}

/**
 * Safely extracts the normalized hostname from a URL string.
 * @param {string} urlStr
 * @returns {string|null}
 */
export function extractHostname(urlStr) {
  if (!urlStr || typeof urlStr !== "string") return null;
  try {
    const url = new URL(urlStr);
    return normalizeDomain(url.hostname);
  } catch {
    return null;
  }
}

/**
 * Determines whether a URL is protected from being blocked.
 * ONUSLY dashboard, authentication pages, internal browser schemes,
 * and extension pages must NEVER be blocked.
 *
 * @param {string} urlStr
 * @returns {boolean}
 */
export function isProtectedUrl(urlStr) {
  if (!urlStr || typeof urlStr !== "string") return true;

  try {
    const url = new URL(urlStr);

    // Only HTTP and HTTPS URLs are subject to blocking
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      return true;
    }

    const host = normalizeDomain(url.hostname);
    if (!host) return true;

    // Localhost, loopback, and dev environments
    if (host === "localhost" || host === "127.0.0.1" || host === "10.0.2.2") {
      return true;
    }

    // ONUSLY production and staging domains
    if (host === "onusly.com" || host.endsWith(".onusly.com")) {
      return true;
    }

    return false;
  } catch {
    return true;
  }
}

/**
 * Checks whether a hostname is a valid YouTube host (e.g. 'youtube.com', 'www.youtube.com', 'm.youtube.com').
 * Prevents false positives such as 'notyoutube.com' or 'youtube.com.attacker.com'.
 *
 * @param {string} hostname
 * @returns {boolean}
 */
export function isYouTubeHost(hostname) {
  if (!hostname || typeof hostname !== "string") return false;
  return isDomainMatch(hostname, "youtube.com");
}

/**
 * Checks whether a URL is a YouTube Shorts URL.
 * Recognizes '/shorts', '/shorts/', '/shorts/VIDEO_ID' across all valid YouTube host variants.
 * Ensures ordinary '/watch?v=...', playlists, and non-Shorts paths are never matched.
 *
 * @param {string} urlStr
 * @returns {boolean}
 */
export function isYouTubeShortsUrl(urlStr) {
  if (!urlStr || typeof urlStr !== "string") return false;

  try {
    const url = new URL(urlStr, "https://www.youtube.com");
    const host = normalizeDomain(url.hostname);

    if (!isYouTubeHost(host)) {
      return false;
    }

    const path = url.pathname.toLowerCase();
    return path === "/shorts" || path === "/shorts/" || path.startsWith("/shorts/");
  } catch {
    return false;
  }
}
