export const APP_CATALOG = [
  {
    id: "youtube",
    name: "YouTube",
    packageName: "com.google.android.youtube",
    domains: ["youtube.com", "youtu.be"],
  },
  {
    id: "youtube_shorts",
    name: "YouTube Shorts",
    packageName: "com.google.android.youtube.shorts",
    domains: ["shorts.youtube.com"],
  },
  {
    id: "instagram",
    name: "Instagram",
    packageName: "com.instagram.android",
    domains: ["instagram.com"],
  },
  {
    id: "twitter",
    name: "X / Twitter",
    packageName: "com.twitter.android",
    domains: ["x.com", "twitter.com"],
  },
  {
    id: "reddit",
    name: "Reddit",
    packageName: "com.reddit.frontpage",
    domains: ["reddit.com"],
  },
  {
    id: "netflix",
    name: "Netflix",
    packageName: "com.netflix.mediaclient",
    domains: ["netflix.com"],
  },
  {
    id: "discord",
    name: "Discord",
    packageName: "com.discord",
    domains: ["discord.com"],
  },
  {
    id: "twitch",
    name: "Twitch",
    packageName: "tv.twitch.android.app",
    domains: ["twitch.tv"],
  },
  {
    id: "tiktok",
    name: "TikTok",
    packageName: "com.zhiliaoapp.musically",
    packageNames: ["com.zhiliaoapp.musically", "com.ss.android.ugc.trill"],
    domains: ["tiktok.com"],
  },
  {
    id: "facebook",
    name: "Facebook",
    packageName: "com.facebook.katana",
    packageNames: ["com.facebook.katana", "com.facebook.orca"],
    domains: ["facebook.com"],
  },
  {
    id: "snapchat",
    name: "Snapchat",
    packageName: "com.snapchat.android",
    domains: ["snapchat.com"],
  },
  {
    id: "linkedin",
    name: "LinkedIn",
    packageName: "com.linkedin.android",
    domains: ["linkedin.com"],
  },
  {
    id: "pinterest",
    name: "Pinterest",
    packageName: "com.pinterest",
    domains: ["pinterest.com"],
  },
];

// Backward-compatibility alias
export const APPS_TO_BLOCK = APP_CATALOG.map((item) => ({
  id: item.id,
  label: item.name,
  packageName: item.packageName,
  domains: item.domains,
}));

export function getFriendlyTargetName(target) {
  if (!target || typeof target !== "string") return "";
  const clean = target.trim().toLowerCase();
  const cleanDomain = clean
    .replace(/^(?:https?:\/\/)?(?:www\.)?/i, "")
    .split("/")[0]
    .split("?")[0];

  const found = APP_CATALOG.find(
    (item) =>
      item.id?.toLowerCase() === clean ||
      item.packageName?.toLowerCase() === clean ||
      (Array.isArray(item.packageNames) &&
        item.packageNames.some((pkg) => pkg.toLowerCase() === clean)) ||
      (Array.isArray(item.domains) &&
        item.domains.some((d) => {
          const dLower = d.toLowerCase();
          const dClean = dLower.replace(/^(?:www\.)?/i, "");
          return (
            dLower === clean ||
            dClean === clean ||
            dClean === cleanDomain ||
            dLower === cleanDomain
          );
        }))
  );
  return found ? found.name : target;
}

/**
 * Deduplicates blocked apps and domains into clean, human-readable display badges.
 * Prevents multiple badges for the same service (e.g. Android package + web domain + secondary domain).
 *
 * @param {Array<string>} [apps]
 * @param {Array<string>} [domains]
 * @param {Array<string>} [fallback]
 * @returns {Array<{ raw: string, label: string }>}
 */
export function getUniqueDisplayTargets(apps = [], domains = [], fallback = []) {
  const primary = [...(apps || []), ...(domains || [])];
  const items = primary.length > 0 ? primary : fallback || [];
  const seenLabels = new Set();
  const results = [];

  for (const item of items) {
    if (!item) continue;
    const label = getFriendlyTargetName(item);
    if (!label) continue;
    const key = label.toLowerCase().trim();
    if (!seenLabels.has(key)) {
      seenLabels.add(key);
      results.push({
        raw: item,
        label,
      });
    }
  }

  return results;
}

export const GOAL_BLOCKED_APPS_KEY = "onusly_goal_blocked_apps";

export function getGoalBlockedApps(goalId) {
  try {
    const raw = localStorage.getItem(GOAL_BLOCKED_APPS_KEY);
    const map = raw ? JSON.parse(raw) : {};
    return map[goalId] || [];
  } catch {
    return [];
  }
}

export function saveGoalBlockedApps(goalId, apps) {
  try {
    const raw = localStorage.getItem(GOAL_BLOCKED_APPS_KEY);
    const map = raw ? JSON.parse(raw) : {};
    map[goalId] = apps;
    localStorage.setItem(GOAL_BLOCKED_APPS_KEY, JSON.stringify(map));
  } catch (err) {
    console.error("Failed to save goal blocked apps:", err);
  }
}
