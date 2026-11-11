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
  {
    id: "steam",
    name: "Steam",
    packageName: "com.valvesoftware.android.steam.community",
    packageNames: ["steam", "com.valvesoftware.android.steam.community"],
    domains: ["steampowered.com", "steamcommunity.com"],
  },
  {
    id: "spotify",
    name: "Spotify",
    packageName: "com.spotify.music",
    packageNames: ["spotify", "com.spotify.music"],
    domains: ["spotify.com"],
  },
  {
    id: "slack",
    name: "Slack",
    packageName: "com.Slack",
    packageNames: ["slack", "com.Slack"],
    domains: ["slack.com"],
  },
  {
    id: "telegram",
    name: "Telegram",
    packageName: "org.telegram.messenger",
    packageNames: ["telegram", "org.telegram.messenger"],
    domains: ["t.me", "web.telegram.org"],
  },
  {
    id: "vlc",
    name: "VLC Media Player",
    packageName: "org.videolan.vlc",
    packageNames: ["vlc", "org.videolan.vlc"],
    domains: ["videolan.org"],
  },
  {
    id: "obs",
    name: "OBS Studio",
    packageName: "com.obsproject.Studio",
    packageNames: ["obs", "obs64", "obs32", "com.obsproject.Studio"],
    domains: ["obsproject.com"],
  },
];

export const PROTECTED_SYSTEM_PROCESSES = [
  "csrss", "smss", "wininit", "winlogon", "services", "lsass", "svchost",
  "dwm", "explorer", "taskmgr", "sihost", "fontdrvhost", "searchhost",
  "shellexperiencehost", "startmenuexperiencehost", "ctfmon", "conhost",
  "cmd", "powershell", "pwsh", "wsmprovhost", "dllhost",
  "systemd", "init", "kthreadd", "dbus-daemon", "systemd-journald",
  "systemd-logind", "systemd-resolved", "systemd-udevd", "xorg", "wayland",
  "gnome-shell", "plasmashell", "bash", "sh", "zsh", "login", "sshd", "cron", "polkitd",
  "onusly-agent", "onusly-native-host", "onusly",
];

export function isProtectedProcess(name) {
  if (!name || typeof name !== "string") return false;
  const clean = name.trim().toLowerCase().replace(/\.exe$/i, "").replace(/\.desktop$/i, "");
  const compact = clean.replace(/[-_ ]/g, "");
  return PROTECTED_SYSTEM_PROCESSES.some((p) => {
    const pClean = p.toLowerCase();
    const pCompact = pClean.replace(/[-_ ]/g, "");
    return clean === pClean || compact === pCompact;
  });
}

export function normalizeDesktopAppTarget(raw) {
  if (!raw || typeof raw !== "string") return "";
  let clean = raw.trim().toLowerCase();
  clean = clean.replace(/\.exe$/i, "").replace(/\.desktop$/i, "");
  return clean;
}

export function inferTargetType(target) {
  if (!target || typeof target !== "string") return "desktop";
  const trimmed = target.trim();
  if (trimmed.includes("/") || trimmed.includes("\\") || trimmed.includes(":") || /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/i.test(trimmed)) {
    if (/^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$/.test(trimmed) && (trimmed.startsWith("com.") || trimmed.startsWith("org.") || trimmed.startsWith("tv."))) {
      return "app";
    }
    return "domain";
  }
  return "desktop";
}

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
        type: inferTargetType(item),
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
