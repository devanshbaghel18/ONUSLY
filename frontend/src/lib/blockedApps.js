export const APP_CATALOG = [
  {
    id: "youtube",
    name: "YouTube",
    packageName: "com.google.android.youtube",
    domains: ["youtube.com", "youtu.be"],
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
    id: "tiktok",
    name: "TikTok",
    packageName: "com.zhiliaoapp.musically",
    domains: ["tiktok.com"],
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
];

// Backward-compatibility alias
export const APPS_TO_BLOCK = APP_CATALOG.map((item) => ({
  id: item.id,
  label: item.name,
  packageName: item.packageName,
  domains: item.domains,
}));

export function getFriendlyTargetName(target) {
  if (!target) return "";
  const found = APP_CATALOG.find(
    (item) =>
      item.packageName === target ||
      item.domains.includes(target) ||
      item.id === target
  );
  return found ? found.name : target;
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
