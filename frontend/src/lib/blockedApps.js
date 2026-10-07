export const APPS_TO_BLOCK = [
  { id: "youtube", label: "YouTube" },
  { id: "twitter", label: "Twitter / X" },
  { id: "instagram", label: "Instagram" },
  { id: "reddit", label: "Reddit" },
  { id: "tiktok", label: "TikTok" },
  { id: "netflix", label: "Netflix" },
  { id: "linkedin", label: "LinkedIn" },
  { id: "discord", label: "Discord" },
  { id: "twitch", label: "Twitch" },
];

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
