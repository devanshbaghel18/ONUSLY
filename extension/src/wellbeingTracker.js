/**
 * Privacy-first Digital Wellbeing tracker for the ONUSLY browser extension.
 * Tracks only normalized hostnames (no paths, queries, or page titles),
 * counting website opens, active foreground browse duration, and blocked attempts.
 */

const MAX_RETENTION_DAYS = 14;
const MAX_SINGLE_FLUSH_SECONDS = 1800; // Cap single interval at 30m in case system suspended
const VISIT_DEBOUNCE_MS = 5000;
const KEY_ACTIVE_STATE = "onusly_wellbeing_active";

function addDurationAcrossMidnight(map, ensureBucketFn, domain, startMs, elapsedMs) {
  let cursor = startMs;
  let remainingMs = Math.min(elapsedMs, MAX_SINGLE_FLUSH_SECONDS * 1000);
  while (remainingMs > 0) {
    const curDate = new Date(cursor);
    const nextMidnight = new Date(curDate.getFullYear(), curDate.getMonth(), curDate.getDate() + 1).getTime();
    const sliceMs = Math.min(remainingMs, Math.max(1, nextMidnight - cursor));
    const sliceSec = Math.floor(sliceMs / 1000);
    const dateKey = getLocalDateKey(cursor);
    if (sliceSec > 0) {
      const bucket = ensureBucketFn(map, dateKey, domain);
      bucket.seconds += sliceSec;
    }
    cursor += sliceMs;
    remainingMs -= sliceMs;
  }
}

export function extractTrackableDomain(rawUrl) {
  if (!rawUrl || typeof rawUrl !== "string") return null;
  const trimmed = rawUrl.trim();
  if (!trimmed) return null;

  try {
    const parsed = new URL(trimmed);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return null;
    }
    const host = parsed.hostname.toLowerCase().replace(/^www\./, "").replace(/\.+$/, "");
    if (!host || host === "localhost" || host === "127.0.0.1" || host === "::1") {
      return null;
    }
    return host;
  } catch {
    return null;
  }
}

export function getLocalDateKey(nowMs = Date.now()) {
  const d = new Date(nowMs);
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function createWellbeingTracker({ storage, nowFn = () => Date.now() }) {
  let activeDomain = null;
  let activeSinceMs = null;
  let isIdle = false;
  let activeLoaded = false;
  let dailyMapCache = null;
  let lastSyncedDates = {};
  const recentVisits = new Map(); // `${tabId}:${domain}` -> timestamp
  let queue = Promise.resolve();

  function enqueue(taskFn) {
    const run = queue.then(taskFn, taskFn);
    queue = run.catch(() => {});
    return run;
  }

  async function restoreActiveState() {
    if (activeLoaded) return;
    activeLoaded = true;
    try {
      if (globalThis.chrome?.storage?.local) {
        const res = await new Promise((resolve) => {
          chrome.storage.local.get([KEY_ACTIVE_STATE], (d) => resolve(d || {}));
        });
        const saved = res[KEY_ACTIVE_STATE];
        if (saved && saved.activeDomain) {
          activeDomain = saved.activeDomain;
          activeSinceMs = typeof saved.activeSinceMs === "number" ? saved.activeSinceMs : null;
          isIdle = Boolean(saved.isIdle);
        }
      }
    } catch {
      // Fallback safely when running in unit tests without chrome.storage.local
    }
  }

  async function persistActiveState() {
    try {
      if (globalThis.chrome?.storage?.local) {
        await new Promise((resolve) => {
          chrome.storage.local.set(
            { [KEY_ACTIVE_STATE]: { activeDomain, activeSinceMs, isIdle } },
            () => resolve()
          );
        });
      }
    } catch {
      // Ignore in non-browser test environments
    }
  }

  async function loadMap() {
    if (!dailyMapCache) {
      dailyMapCache = (await storage.getWellbeingData()) || {};
    }
    return dailyMapCache;
  }

  function pruneOldDays(map, currentDateKey) {
    const keys = Object.keys(map).sort();
    if (keys.length <= MAX_RETENTION_DAYS) return;
    const cutoff = keys.length - MAX_RETENTION_DAYS;
    for (let i = 0; i < cutoff; i++) {
      if (keys[i] !== currentDateKey) {
        delete map[keys[i]];
      }
    }
  }

  function ensureDomainBucket(map, dateKey, domain) {
    if (!map[dateKey]) {
      map[dateKey] = {};
    }
    if (!map[dateKey][domain]) {
      map[dateKey][domain] = { domain, seconds: 0, opens: 0, blockedAttempts: 0 };
    }
    return map[dateKey][domain];
  }

  async function flushElapsedInternal() {
    await restoreActiveState();
    const now = nowFn();
    if (isIdle || !activeDomain || activeSinceMs === null) {
      if (!isIdle && activeDomain) {
        activeSinceMs = now;
        await persistActiveState();
      }
      return;
    }

    const elapsedMs = now - activeSinceMs;
    const wholeSec = Math.floor(elapsedMs / 1000);
    if (wholeSec <= 0) return;

    const effectiveMs = Math.min(wholeSec * 1000, MAX_SINGLE_FLUSH_SECONDS * 1000);
    const startMs = now - effectiveMs;
    // Preserve sub-second remainder only when interval wasn't clamped by suspension cap
    activeSinceMs = elapsedMs <= MAX_SINGLE_FLUSH_SECONDS * 1000
      ? activeSinceMs + wholeSec * 1000
      : now;

    const todayKey = getLocalDateKey(now);
    const map = await loadMap();
    addDurationAcrossMidnight(map, ensureDomainBucket, activeDomain, startMs, effectiveMs);
    pruneOldDays(map, todayKey);
    await storage.saveWellbeingData(map);
    await persistActiveState();
  }

  async function flushElapsed() {
    return enqueue(() => flushElapsedInternal());
  }

  return {
    extractTrackableDomain,

    async recordActiveFocus(rawUrl) {
      return enqueue(async () => {
        await flushElapsedInternal();
        const domain = extractTrackableDomain(rawUrl);
        activeDomain = domain;
        activeSinceMs = domain && !isIdle ? nowFn() : null;
        await persistActiveState();
      });
    },

    async clearActiveFocus() {
      return enqueue(async () => {
        await flushElapsedInternal();
        activeDomain = null;
        activeSinceMs = null;
        await persistActiveState();
      });
    },

    async recordVisit(rawUrl, tabId = 0) {
      const domain = extractTrackableDomain(rawUrl);
      if (!domain) return false;

      const now = nowFn();
      const dedupeKey = `${tabId}:${domain}`;
      const prevAt = recentVisits.get(dedupeKey) || 0;
      if (now - prevAt < VISIT_DEBOUNCE_MS) {
        return false;
      }
      recentVisits.set(dedupeKey, now);

      return enqueue(async () => {
        const dateKey = getLocalDateKey(now);
        const map = await loadMap();
        const bucket = ensureDomainBucket(map, dateKey, domain);
        bucket.opens += 1;
        pruneOldDays(map, dateKey);
        await storage.saveWellbeingData(map);
        return true;
      });
    },

    async recordBlockedAttempt(domainOrUrl) {
      const domain = extractTrackableDomain(domainOrUrl) ||
        (typeof domainOrUrl === "string" ? domainOrUrl.trim().toLowerCase().replace(/^www\./, "") : "");
      if (!domain) return false;

      return enqueue(async () => {
        const dateKey = getLocalDateKey(nowFn());
        const map = await loadMap();
        const bucket = ensureDomainBucket(map, dateKey, domain);
        bucket.blockedAttempts += 1;
        pruneOldDays(map, dateKey);
        await storage.saveWellbeingData(map);
        return true;
      });
    },

    async setIdleState(state) {
      const nextIdle = state === "idle" || state === "locked";
      return enqueue(async () => {
        await restoreActiveState();
        if (nextIdle && !isIdle) {
          await flushElapsedInternal();
          isIdle = true;
          activeSinceMs = null;
          await persistActiveState();
        } else if (!nextIdle && isIdle) {
          isIdle = false;
          if (activeDomain) {
            activeSinceMs = nowFn();
          }
          await persistActiveState();
        }
      });
    },

    flushElapsed,

    async getTodaySummary(limit = 5) {
      return enqueue(async () => {
        await flushElapsedInternal();
        const todayKey = getLocalDateKey(nowFn());
        const map = await loadMap();
        const buckets = Object.values(map[todayKey] || {});
        const totalSeconds = buckets.reduce((s, d) => s + (d.seconds || 0), 0);
        const totalOpens = buckets.reduce((s, d) => s + (d.opens || 0), 0);
        const totalBlocked = buckets.reduce((s, d) => s + (d.blockedAttempts || 0), 0);
        const topWebsites = [...buckets]
          .filter((d) => (d.seconds || 0) > 0 || (d.opens || 0) > 0 || (d.blockedAttempts || 0) > 0)
          .sort((a, b) => (b.seconds - a.seconds) || (b.opens - a.opens))
          .slice(0, limit);
        return { date: todayKey, totalSeconds, totalOpens, totalBlocked, topWebsites };
      });
    },

    async buildIngestPayload(deviceId, browserName = "chrome", dateKey = getLocalDateKey(nowFn())) {
      await flushElapsed();
      const map = await loadMap();
      const dayBuckets = map[dateKey] || {};
      const domains = Object.values(dayBuckets);
      const totalScreenSeconds = domains.reduce((acc, d) => acc + (d.seconds || 0), 0);

      return {
        date: dateKey,
        deviceId,
        clientType: "extension",
        platform: browserName,
        totalScreenSeconds,
        domains,
        apps: [],
        tzOffsetMinutes: -new Date(nowFn()).getTimezoneOffset(),
        sentAt: new Date(nowFn()).toISOString(),
      };
    },

    async syncToBackend(sendWellbeingFn, browserName = "chrome") {
      try {
        const session = await storage.getSession();
        if (!session || !session.token || !session.userId) return false;

        const settings = await storage.getSettings();
        if (!settings?.baseUrl) return false;

        const deviceId = await storage.getDeviceId();
        const now = nowFn();
        const todayKey = getLocalDateKey(now);
        const yesterdayKey = getLocalDateKey(now - 24 * 3600 * 1000);

        // Flush yesterday's pre-midnight remainder once after rollover
        const map = await loadMap();
        if (yesterdayKey !== todayKey && map[yesterdayKey] && Object.keys(map[yesterdayKey]).length > 0 && lastSyncedDates[yesterdayKey] !== "finalized") {
          const yPayload = await this.buildIngestPayload(deviceId, browserName, yesterdayKey);
          if (yPayload.domains.length > 0) {
            const okY = await sendWellbeingFn(settings.baseUrl, session.token, yPayload);
            if (okY) lastSyncedDates[yesterdayKey] = "finalized";
          }
        }

        const payload = await this.buildIngestPayload(deviceId, browserName, todayKey);
        if (payload.domains.length === 0) return false;

        return await sendWellbeingFn(settings.baseUrl, session.token, payload);
      } catch {
        return false;
      }
    },
  };
}
