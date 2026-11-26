import { useState, useEffect, useMemo, useRef, useCallback } from "react";
import {
  Activity,
  Clock,
  Globe,
  ShieldAlert,
  Smartphone,
  Monitor,
  RefreshCw,
  Loader2,
  TrendingUp,
  TrendingDown,
  Timer,
  Moon,
  Trash2,
  Lock,
  Plus,
} from "lucide-react";
import AppLayout from "../components/AppLayout";
import CreateGoalModal from "../components/CreateGoalModal";
import {
  getWellbeingSummary,
  getWellbeingRules,
  upsertWellbeingLimit,
  deleteWellbeingLimit,
  saveWellbeingSchedule,
} from "../lib/api";
import { notifyExtensionSync } from "../lib/extensionSync";
import { useWebSocket } from "../hooks/useWebSocket";
import { formatDuration, formatDelta, formatShortDay } from "../lib/wellbeingFormat";

const RANGE_OPTIONS = [
  { id: "today", label: "Today" },
  { id: "yesterday", label: "Yesterday" },
  { id: "7d", label: "Past 7 Days" },
  { id: "all", label: "All Time" },
];

export default function DigitalWellbeing() {
  const [range, setRange] = useState("today");
  const [deviceId, setDeviceId] = useState("");
  const [summary, setSummary] = useState(null);
  const [rules, setRules] = useState({ limits: [], schedule: null });
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");

  // Quick Daily Limit form/modal state
  const [limitTarget, setLimitTarget] = useState(null); // { targetType, target, displayName }
  const [limitMinutes, setLimitMinutes] = useState(30);
  const [savingLimit, setSavingLimit] = useState(false);
  const [limitModalError, setLimitModalError] = useState("");
  const [limitAlertBanner, setLimitAlertBanner] = useState("");

  // Quick Goal Lock modal state
  const [goalPrefill, setGoalPrefill] = useState(null);

  // Wind-down Focus Schedule form state
  const [schedEnabled, setSchedEnabled] = useState(false);
  const [schedName, setSchedName] = useState("Wind-Down Focus Hours");
  const [schedStart, setSchedStart] = useState("23:00");
  const [schedEnd, setSchedEnd] = useState("07:00");
  const [schedDomainsText, setSchedDomainsText] = useState("");
  const [schedAppsText, setSchedAppsText] = useState("");
  const [savingSched, setSavingSched] = useState(false);
  const [schedSavedNotice, setSchedSavedNotice] = useState("");

  const scheduleDirtyRef = useRef(false);
  const requestSeqRef = useRef(0);
  const prefillSeqRef = useRef(0);

  const loadAll = useCallback(async (selectedRange = range, selectedDevice = deviceId, silent = false) => {
    const seq = ++requestSeqRef.current;
    if (!silent) setLoading(true);
    else setRefreshing(true);

    try {
      const [sumData, rulesData] = await Promise.all([
        getWellbeingSummary({ range: selectedRange, deviceId: selectedDevice }),
        getWellbeingRules().catch(() => ({ limits: [], schedule: null })),
      ]);
      if (seq !== requestSeqRef.current) return;

      setSummary(sumData);
      setRules(rulesData || { limits: [], schedule: null });

      if (rulesData?.schedule && !scheduleDirtyRef.current) {
        const s = rulesData.schedule;
        setSchedEnabled(Boolean(s.enabled));
        if (s.name) setSchedName(s.name);
        if (s.startTime) setSchedStart(s.startTime);
        if (s.endTime) setSchedEnd(s.endTime);
        if (Array.isArray(s.domains) && s.domains.length > 0) {
          setSchedDomainsText(s.domains.join(", "));
        }
        if (Array.isArray(s.apps) && s.apps.length > 0) {
          setSchedAppsText(s.apps.join(", "));
        }
      }
      setError("");
    } catch (err) {
      if (seq !== requestSeqRef.current) return;
      setError(err.message || "Failed to load Digital Wellbeing summary");
    } finally {
      if (seq === requestSeqRef.current) {
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, [range, deviceId]);

  useEffect(() => {
    const timer = setTimeout(() => {
      void loadAll(range, deviceId, false);
    }, 0);
    return () => clearTimeout(timer);
  }, [range, deviceId, loadAll]);

  useWebSocket((event) => {
    if (event?.type === "wellbeing.limit_reached") {
      const exceeded = event?.payload?.exceeded || [];
      const names = exceeded.map((r) => r.displayName || r.target).filter(Boolean).join(", ");
      setLimitAlertBanner(
        names
          ? `Daily limit reached for ${names} — blocking rules activated across connected devices.`
          : "Daily time limit reached — blocking rules activated across connected devices."
      );
      setTimeout(() => setLimitAlertBanner(""), 8000);
      notifyExtensionSync();
      loadAll(range, deviceId, true);
    }
  });

  const limitsByKey = useMemo(() => {
    const map = new Map();
    for (const r of rules?.limits || []) {
      map.set(`${r.targetType}:${r.target}`, r);
    }
    return map;
  }, [rules]);

  const maxDailySeconds = useMemo(() => {
    const series = summary?.dailySeries || [];
    const maxVal = series.reduce((acc, p) => Math.max(acc, p.totalSeconds || 0), 0);
    return Math.max(maxVal, 60);
  }, [summary]);

  const topActivityLabel = useMemo(() => {
    const topApp = summary?.topApps?.[0];
    const topSite = summary?.topWebsites?.[0];
    if (!topApp && !topSite) return { name: "No activity yet", meta: "—" };
    if (topApp && (!topSite || topApp.seconds >= topSite.seconds)) {
      return {
        name: topApp.displayName || topApp.appId,
        meta: `${formatDuration(topApp.seconds)} • ${topApp.opens} opens`,
      };
    }
    return {
      name: topSite.domain,
      meta: `${formatDuration(topSite.seconds)} • ${topSite.opens} opens`,
    };
  }, [summary]);

  const handleSaveLimit = async (e) => {
    e.preventDefault();
    if (!limitTarget?.target) return;
    setSavingLimit(true);
    setLimitModalError("");
    try {
      await upsertWellbeingLimit({
        targetType: limitTarget.targetType,
        target: limitTarget.target,
        displayName: limitTarget.displayName || limitTarget.target,
        dailyLimitMinutes: Number(limitMinutes) || 30,
        enabled: true,
      });
      notifyExtensionSync();
      setLimitTarget(null);
      await loadAll(range, deviceId, true);
    } catch (err) {
      setLimitModalError(err.message || "Failed to save daily limit");
    } finally {
      setSavingLimit(false);
    }
  };

  const handleDeleteLimit = async (ruleId) => {
    try {
      await deleteWellbeingLimit(ruleId);
      notifyExtensionSync();
      await loadAll(range, deviceId, true);
    } catch (err) {
      setError(err.message || "Failed to remove limit");
    }
  };

  const handleSaveSchedule = async (e) => {
    e.preventDefault();
    setSavingSched(true);
    setSchedSavedNotice("");
    try {
      const domains = schedDomainsText
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      const apps = schedAppsText
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);

      await saveWellbeingSchedule({
        enabled: schedEnabled,
        name: schedName,
        startTime: schedStart,
        endTime: schedEnd,
        domains,
        apps,
      });
      scheduleDirtyRef.current = false;
      notifyExtensionSync();
      setSchedSavedNotice("Focus schedule saved & synced to devices.");
      setTimeout(() => setSchedSavedNotice(""), 4000);
      await loadAll(range, deviceId, true);
    } catch (err) {
      setError(err.message || "Failed to save focus schedule");
    } finally {
      setSavingSched(false);
    }
  };

  const openGoalLockForDomain = (domain) => {
    prefillSeqRef.current += 1;
    setGoalPrefill({
      id: `wb-dom-${prefillSeqRef.current}`,
      type: "domain",
      value: domain,
    });
  };

  const openGoalLockForApp = (app) => {
    const lower = (app.appId || "").toLowerCase();
    const isAndroidPkg = /^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$/.test(lower);
    prefillSeqRef.current += 1;
    setGoalPrefill({
      id: `wb-app-${prefillSeqRef.current}`,
      type: isAndroidPkg ? "app" : "desktop",
      value: app.appId,
    });
  };

  const topWebsites = summary?.topWebsites || [];
  const topApps = summary?.topApps || [];
  const dailySeries = summary?.dailySeries || [];
  const devices = summary?.devices || [];
  const activeLimits = rules?.limits || [];
  const scheduleActiveNow = Boolean(rules?.schedule?.activeNow);

  const maxSiteSec = Math.max(...topWebsites.map((w) => w.seconds || 0), 1);
  const maxAppSec = Math.max(...topApps.map((a) => a.seconds || 0), 1);

  return (
    <AppLayout>
      <div className="mx-auto w-full max-w-[1720px] space-y-8">
        {/* Header & Range Controls */}
        <div className="flex flex-col justify-between gap-5 border-b border-white/[0.08] pb-7 lg:flex-row lg:items-center">
          <div>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/15 bg-white/5 text-white">
                <Activity size={20} />
              </div>
              <h1 className="text-3xl font-black tracking-tight text-white">
                Digital Wellbeing
              </h1>
            </div>
            <p className="mt-2 text-sm text-[#A1A1AA]">
              Track daily screen time, website open counts, active browsing duration, daily quotas, and wind-down schedules across Android, Desktop, and Browser.
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            {/* Time Range Pills */}
            <div className="inline-flex rounded-xl border border-white/10 bg-white/[0.03] p-1">
              {RANGE_OPTIONS.map((opt) => {
                const active = range === opt.id;
                return (
                  <button
                    key={opt.id}
                    type="button"
                    onClick={() => setRange(opt.id)}
                    className={`rounded-lg px-3.5 py-2 text-xs font-bold transition ${
                      active
                        ? "bg-white text-black shadow-sm"
                        : "text-neutral-400 hover:text-white"
                    }`}
                  >
                    {opt.label}
                  </button>
                );
              })}
            </div>

            {devices.length > 0 && (
              <select
                value={deviceId}
                onChange={(e) => setDeviceId(e.target.value)}
                className="rounded-xl border border-white/10 bg-[#141419] px-3 py-2 text-xs font-semibold text-white outline-none focus:border-white/30"
              >
                <option value="">All Devices</option>
                {devices.map((d) => (
                  <option key={d.deviceId} value={d.deviceId}>
                    {d.platform || d.clientType} ({d.deviceId.slice(0, 8)})
                  </option>
                ))}
              </select>
            )}

            <button
              type="button"
              onClick={() => loadAll(range, deviceId, true)}
              disabled={refreshing}
              className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/5 px-3.5 py-2 text-xs font-bold text-white transition hover:border-white/25 hover:bg-white/10 disabled:opacity-50"
            >
              <RefreshCw size={14} className={refreshing ? "animate-spin" : ""} />
              <span>Refresh</span>
            </button>
          </div>
        </div>

        {limitAlertBanner && (
          <div className="rounded-xl border border-amber-500/40 bg-amber-500/15 px-4 py-3 text-sm font-semibold text-amber-200">
            {limitAlertBanner}
          </div>
        )}

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">
            {error}
          </div>
        )}

        {loading && !summary ? (
          <div className="flex h-64 items-center justify-center rounded-2xl border border-white/[0.08] bg-white/[0.02]">
            <Loader2 size={28} className="animate-spin text-white" />
          </div>
        ) : (
          <>
            {/* Top KPI Grid */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5">
                <div className="flex items-center justify-between text-xs font-bold uppercase tracking-wider text-neutral-400">
                  <span>Screen Time</span>
                  <Clock size={16} className="text-neutral-300" />
                </div>
                <div className="mt-3 text-3xl font-black text-white">
                  {formatDuration(summary?.totalScreenSeconds)}
                </div>
                <div className="mt-2 flex items-center gap-1.5 text-xs text-neutral-400">
                  {(range === "today" || range === "yesterday") && summary?.comparisonDeltaSeconds !== 0 ? (
                    <>
                      {summary.comparisonDeltaSeconds > 0 ? (
                        <TrendingUp size={13} className="text-amber-400" />
                      ) : (
                        <TrendingDown size={13} className="text-emerald-400" />
                      )}
                      <span>{formatDelta(summary.comparisonDeltaSeconds)}</span>
                    </>
                  ) : (
                    <span>
                      Apps {formatDuration(summary?.totalAppSeconds)} • Web {formatDuration(summary?.totalWebSeconds)}
                    </span>
                  )}
                </div>
              </div>

              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5">
                <div className="flex items-center justify-between text-xs font-bold uppercase tracking-wider text-neutral-400">
                  <span>Websites Opened</span>
                  <Globe size={16} className="text-indigo-300" />
                </div>
                <div className="mt-3 text-3xl font-black text-white">
                  {(summary?.totalWebsiteOpens || 0).toLocaleString()}
                </div>
                <div className="mt-2 text-xs text-neutral-400">
                  Across {topWebsites.length} unique domain{topWebsites.length === 1 ? "" : "s"}
                </div>
              </div>

              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5">
                <div className="flex items-center justify-between text-xs font-bold uppercase tracking-wider text-neutral-400">
                  <span>Distractions Blocked</span>
                  <ShieldAlert size={16} className="text-emerald-400" />
                </div>
                <div className="mt-3 text-3xl font-black text-emerald-400">
                  {(summary?.totalBlockedAttempts || 0).toLocaleString()}
                </div>
                <div className="mt-2 text-xs text-neutral-400">
                  Intercepted by Mobile DNS &amp; Extension
                </div>
              </div>

              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5">
                <div className="flex items-center justify-between text-xs font-bold uppercase tracking-wider text-neutral-400">
                  <span>Top Activity</span>
                  <Activity size={16} className="text-amber-300" />
                </div>
                <div className="mt-3 truncate text-2xl font-black text-white" title={topActivityLabel.name}>
                  {topActivityLabel.name}
                </div>
                <div className="mt-2 text-xs text-neutral-400">
                  {topActivityLabel.meta}
                </div>
              </div>
            </div>

            {/* Past 7 Days Daily Screen Time Trend */}
            <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-6">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h2 className="text-base font-bold text-white">Past 7 Days Screen Time</h2>
                  <p className="text-xs text-neutral-400">
                    Daily total usage across connected devices (hover bars for breakdown)
                  </p>
                </div>
                <div className="flex items-center gap-4 text-xs text-neutral-400">
                  <span className="inline-flex items-center gap-1.5">
                    <span className="h-2.5 w-2.5 rounded-sm bg-indigo-400" /> Apps
                  </span>
                  <span className="inline-flex items-center gap-1.5">
                    <span className="h-2.5 w-2.5 rounded-sm bg-emerald-400" /> Web
                  </span>
                </div>
              </div>

              <div className="mt-6 grid grid-cols-7 items-end gap-3 h-44 pt-4">
                {dailySeries.map((day, idx) => {
                  const pct = Math.min(100, Math.round(((day.totalSeconds || 0) / maxDailySeconds) * 100));
                  const appShare = day.totalSeconds > 0 ? Math.round(((day.appSeconds || 0) / Math.max(day.appSeconds + day.webSeconds, 1)) * 100) : 50;
                  const isToday = idx === dailySeries.length - 1;
                  return (
                    <div
                      key={day.date}
                      className="group flex h-full flex-col items-center justify-end gap-2"
                      title={`${day.date}: ${formatDuration(day.totalSeconds)} • ${day.websiteOpens} website opens`}
                    >
                      <span className="text-[11px] font-semibold text-neutral-400 group-hover:text-white transition">
                        {formatDuration(day.totalSeconds)}
                      </span>
                      <div className="relative flex w-full max-w-[44px] flex-1 items-end rounded-lg bg-white/[0.03] overflow-hidden">
                        <div
                          className="flex w-full flex-col justify-end overflow-hidden rounded-lg transition-all duration-300"
                          style={{ height: `${Math.max(day.totalSeconds > 0 ? 6 : 2, pct)}%` }}
                        >
                          <div
                            className="w-full bg-indigo-400/90"
                            style={{ height: `${appShare}%` }}
                          />
                          <div
                            className="w-full bg-emerald-400/90"
                            style={{ height: `${100 - appShare}%` }}
                          />
                        </div>
                      </div>
                      <span className={`text-xs font-bold ${isToday ? "text-white" : "text-neutral-500"}`}>
                        {isToday ? "Today" : formatShortDay(day.date)}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Daily Time Limits & Wind-Down Focus Schedule */}
            <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
              {/* Daily Time Quotas */}
              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-6">
                <div className="flex items-center justify-between border-b border-white/[0.06] pb-4">
                  <div className="flex items-center gap-2.5">
                    <Timer size={18} className="text-amber-300" />
                    <h2 className="text-base font-bold text-white">Daily Time Limits (Auto-Block Quotas)</h2>
                  </div>
                  <button
                    type="button"
                    onClick={() => {
                      setLimitTarget({ targetType: "domain", target: "", displayName: "" });
                      setLimitMinutes(30);
                    }}
                    className="inline-flex items-center gap-1 rounded-lg border border-white/15 bg-white/5 px-2.5 py-1.5 text-xs font-bold text-white hover:bg-white/10 transition"
                  >
                    <Plus size={13} />
                    <span>Add Limit</span>
                  </button>
                </div>

                {activeLimits.length === 0 ? (
                  <p className="py-8 text-center text-sm text-neutral-500">
                    No daily limits configured yet. Click &ldquo;Set Limit&rdquo; on any website or app below to auto-block it once its daily quota is reached.
                  </p>
                ) : (
                  <div className="mt-4 space-y-3">
                    {activeLimits.map((rule) => {
                      const quotaSec = rule.dailyLimitMinutes * 60;
                      const pct = Math.min(100, Math.round(((rule.usedSecondsToday || 0) / Math.max(quotaSec, 1)) * 100));
                      return (
                        <div key={rule.id} className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-3.5">
                          <div className="flex items-center justify-between gap-2">
                            <div className="min-w-0">
                              <div className="flex items-center gap-2">
                                <span className="truncate text-sm font-bold text-white">
                                  {rule.displayName || rule.target}
                                </span>
                                {rule.exceeded && (
                                  <span className="rounded-md border border-red-500/40 bg-red-500/15 px-2 py-0.5 text-[10px] font-black uppercase tracking-wider text-red-300">
                                    Quota Reached • Blocked
                                  </span>
                                )}
                              </div>
                              <div className="text-xs text-neutral-400">
                                {formatDuration(rule.usedSecondsToday)} / {rule.dailyLimitMinutes}m today ({rule.targetType})
                              </div>
                            </div>
                            <button
                              type="button"
                              onClick={() => handleDeleteLimit(rule.id)}
                              className="rounded-lg p-1.5 text-neutral-500 hover:bg-white/5 hover:text-red-300 transition"
                              title="Remove daily limit"
                            >
                              <Trash2 size={14} />
                            </button>
                          </div>
                          <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-white/[0.06]">
                            <div
                              className={`h-full rounded-full ${rule.exceeded ? "bg-red-400" : "bg-amber-400"}`}
                              style={{ width: `${pct}%` }}
                            />
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              {/* Wind-Down Focus Schedule */}
              <form
                onSubmit={handleSaveSchedule}
                className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-6"
              >
                <div className="flex items-center justify-between border-b border-white/[0.06] pb-4">
                  <div className="flex items-center gap-2.5">
                    <Moon size={18} className="text-indigo-300" />
                    <h2 className="text-base font-bold text-white">Wind-Down Focus Schedule</h2>
                  </div>
                  <div className="flex items-center gap-2.5">
                    {scheduleActiveNow && (
                      <span className="rounded-full border border-emerald-500/40 bg-emerald-500/15 px-2.5 py-0.5 text-[10px] font-black uppercase text-emerald-300">
                        Active Now
                      </span>
                    )}
                    <label className="inline-flex cursor-pointer items-center gap-2 text-xs font-bold text-neutral-300">
                      <input
                        type="checkbox"
                        checked={schedEnabled}
                        onChange={(e) => { scheduleDirtyRef.current = true; setSchedEnabled(e.target.checked); }}
                        className="h-4 w-4 accent-white"
                      />
                      <span>Enabled</span>
                    </label>
                  </div>
                </div>

                <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div>
                    <label className="block text-[11px] font-bold uppercase text-neutral-400">
                      Schedule Name
                    </label>
                    <input
                      type="text"
                      value={schedName}
                      onChange={(e) => { scheduleDirtyRef.current = true; setSchedName(e.target.value); }}
                      className="mt-1 w-full rounded-xl border border-white/10 bg-[#141419] px-3 py-2 text-xs text-white outline-none focus:border-white/30"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-bold uppercase text-neutral-400">
                      Start Time
                    </label>
                    <input
                      type="time"
                      value={schedStart}
                      onChange={(e) => { scheduleDirtyRef.current = true; setSchedStart(e.target.value); }}
                      className="mt-1 w-full rounded-xl border border-white/10 bg-[#141419] px-3 py-2 text-xs text-white outline-none focus:border-white/30"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-bold uppercase text-neutral-400">
                      End Time
                    </label>
                    <input
                      type="time"
                      value={schedEnd}
                      onChange={(e) => { scheduleDirtyRef.current = true; setSchedEnd(e.target.value); }}
                      className="mt-1 w-full rounded-xl border border-white/10 bg-[#141419] px-3 py-2 text-xs text-white outline-none focus:border-white/30"
                    />
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div>
                    <label className="block text-[11px] font-bold uppercase text-neutral-400">
                      Quiet Domains (comma-separated)
                    </label>
                    <input
                      type="text"
                      value={schedDomainsText}
                      onChange={(e) => { scheduleDirtyRef.current = true; setSchedDomainsText(e.target.value); }}
                      placeholder="instagram.com, youtube.com, reddit.com"
                      className="mt-1 w-full rounded-xl border border-white/10 bg-[#141419] px-3 py-2 text-xs text-white outline-none focus:border-white/30"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-bold uppercase text-neutral-400">
                      Quiet Apps (optional)
                    </label>
                    <input
                      type="text"
                      value={schedAppsText}
                      onChange={(e) => { scheduleDirtyRef.current = true; setSchedAppsText(e.target.value); }}
                      placeholder="com.instagram.android, discord.exe"
                      className="mt-1 w-full rounded-xl border border-white/10 bg-[#141419] px-3 py-2 text-xs text-white outline-none focus:border-white/30"
                    />
                  </div>
                </div>

                <div className="mt-4 flex items-center justify-between gap-3">
                  <span className="text-xs text-emerald-300">{schedSavedNotice}</span>
                  <button
                    type="submit"
                    disabled={savingSched}
                    className="inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-black text-black hover:bg-neutral-200 transition disabled:opacity-50"
                  >
                    {savingSched && <Loader2 size={13} className="animate-spin" />}
                    <span>Save Schedule</span>
                  </button>
                </div>
              </form>
            </div>

            {/* Websites & Apps Breakdown Split */}
            <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
              {/* Websites Breakdown */}
              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-6">
                <div className="flex items-center justify-between border-b border-white/[0.06] pb-4">
                  <div className="flex items-center gap-2.5">
                    <Globe size={18} className="text-emerald-400" />
                    <h2 className="text-base font-bold text-white">Websites Opened &amp; Time Browsed</h2>
                  </div>
                  <span className="text-xs font-semibold text-neutral-400">
                    {topWebsites.length} sites
                  </span>
                </div>

                {topWebsites.length === 0 ? (
                  <p className="py-10 text-center text-sm text-neutral-500">
                    No website usage recorded for this timeframe yet. Connect the browser extension or enable Mobile DNS Shield.
                  </p>
                ) : (
                  <div className="mt-4 space-y-3.5 max-h-[460px] overflow-y-auto pr-1">
                    {topWebsites.map((site) => {
                      const barPct = Math.max(4, Math.round(((site.seconds || 0) / maxSiteSec) * 100));
                      const activeRule = limitsByKey.get(`domain:${site.domain}`);
                      return (
                        <div key={site.domain} className="rounded-xl border border-white/[0.05] bg-white/[0.015] p-3.5">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <div className="flex items-center gap-2 min-w-0">
                              <span className="truncate font-semibold text-sm text-white">{site.domain}</span>
                              {activeRule && (
                                <span
                                  className={`rounded-md px-2 py-0.5 text-[10px] font-bold ${
                                    activeRule.exceeded
                                      ? "bg-red-500/20 text-red-300"
                                      : "bg-amber-500/15 text-amber-300"
                                  }`}
                                >
                                  Limit: {activeRule.dailyLimitMinutes}m
                                </span>
                              )}
                            </div>
                            <div className="flex items-center gap-2 shrink-0 text-xs">
                              <span className="rounded-md bg-white/5 px-2 py-0.5 font-bold text-neutral-200">
                                {site.opens || 0} opens
                              </span>
                              <span className="font-mono font-bold text-emerald-300">
                                {formatDuration(site.seconds)}
                              </span>
                              <button
                                type="button"
                                onClick={() => {
                                  setLimitTarget({
                                    targetType: "domain",
                                    target: site.domain,
                                    displayName: site.domain,
                                  });
                                  setLimitMinutes(activeRule?.dailyLimitMinutes || 30);
                                }}
                                className="rounded-lg border border-white/10 bg-white/5 px-2 py-1 text-[11px] font-semibold text-neutral-300 hover:border-white/25 hover:text-white transition"
                              >
                                Set Limit
                              </button>
                              <button
                                type="button"
                                onClick={() => openGoalLockForDomain(site.domain)}
                                className="inline-flex items-center gap-1 rounded-lg border border-white/10 bg-white/5 px-2 py-1 text-[11px] font-semibold text-neutral-300 hover:border-white/25 hover:text-white transition"
                                title="Create accountability lock for this website"
                              >
                                <Lock size={11} />
                                <span>Block</span>
                              </button>
                            </div>
                          </div>
                          <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-white/[0.06]">
                            <div
                              className="h-full rounded-full bg-emerald-400/80"
                              style={{ width: `${barPct}%` }}
                            />
                          </div>
                          {site.blockedAttempts > 0 && (
                            <div className="mt-1.5 text-[11px] font-medium text-amber-300">
                              Blocked {site.blockedAttempts} time{site.blockedAttempts === 1 ? "" : "s"}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              {/* Applications Breakdown */}
              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-6">
                <div className="flex items-center justify-between border-b border-white/[0.06] pb-4">
                  <div className="flex items-center gap-2.5">
                    <Smartphone size={18} className="text-indigo-400" />
                    <h2 className="text-base font-bold text-white">Apps Usage &amp; Screen Time</h2>
                  </div>
                  <span className="text-xs font-semibold text-neutral-400">
                    {topApps.length} apps
                  </span>
                </div>

                {topApps.length === 0 ? (
                  <p className="py-10 text-center text-sm text-neutral-500">
                    No app screen time recorded for this timeframe yet. Enable Usage Access in Android or run the Desktop Agent.
                  </p>
                ) : (
                  <div className="mt-4 space-y-3.5 max-h-[460px] overflow-y-auto pr-1">
                    {topApps.map((app) => {
                      const barPct = Math.max(4, Math.round(((app.seconds || 0) / maxAppSec) * 100));
                      const activeRule = limitsByKey.get(`app:${(app.appId || "").toLowerCase()}`);
                      return (
                        <div key={app.appId} className="rounded-xl border border-white/[0.05] bg-white/[0.015] p-3.5">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <div className="min-w-0">
                              <div className="flex items-center gap-2">
                                <span className="truncate font-semibold text-sm text-white">
                                  {app.displayName || app.appId}
                                </span>
                                {activeRule && (
                                  <span
                                    className={`rounded-md px-2 py-0.5 text-[10px] font-bold ${
                                      activeRule.exceeded
                                        ? "bg-red-500/20 text-red-300"
                                        : "bg-amber-500/15 text-amber-300"
                                    }`}
                                  >
                                    Limit: {activeRule.dailyLimitMinutes}m
                                  </span>
                                )}
                              </div>
                              <div className="truncate font-mono text-[11px] text-neutral-500">
                                {app.appId}
                              </div>
                            </div>
                            <div className="flex items-center gap-2 shrink-0 text-xs">
                              <span className="rounded-md bg-white/5 px-2 py-0.5 font-bold text-neutral-200">
                                {app.opens || 0} opens
                              </span>
                              <span className="font-mono font-bold text-indigo-300">
                                {formatDuration(app.seconds)}
                              </span>
                              <button
                                type="button"
                                onClick={() => {
                                  setLimitTarget({
                                    targetType: "app",
                                    target: app.appId,
                                    displayName: app.displayName || app.appId,
                                  });
                                  setLimitMinutes(activeRule?.dailyLimitMinutes || 30);
                                }}
                                className="rounded-lg border border-white/10 bg-white/5 px-2 py-1 text-[11px] font-semibold text-neutral-300 hover:border-white/25 hover:text-white transition"
                              >
                                Set Limit
                              </button>
                              <button
                                type="button"
                                onClick={() => openGoalLockForApp(app)}
                                className="inline-flex items-center gap-1 rounded-lg border border-white/10 bg-white/5 px-2 py-1 text-[11px] font-semibold text-neutral-300 hover:border-white/25 hover:text-white transition"
                                title="Create accountability lock for this app"
                              >
                                <Lock size={11} />
                                <span>Block</span>
                              </button>
                            </div>
                          </div>
                          <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-white/[0.06]">
                            <div
                              className="h-full rounded-full bg-indigo-400/80"
                              style={{ width: `${barPct}%` }}
                            />
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>

            {/* Connected Devices Breakdown */}
            {devices.length > 0 && (
              <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-6">
                <h2 className="text-sm font-bold uppercase tracking-wider text-neutral-400">
                  Usage By Device
                </h2>
                <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {devices.map((d) => (
                    <div
                      key={d.deviceId}
                      className="flex items-center justify-between rounded-xl border border-white/[0.06] bg-white/[0.02] p-4"
                    >
                      <div className="flex items-center gap-3">
                        {d.clientType === "android" ? (
                          <Smartphone size={18} className="text-indigo-300" />
                        ) : d.clientType === "extension" ? (
                          <Globe size={18} className="text-emerald-300" />
                        ) : (
                          <Monitor size={18} className="text-amber-300" />
                        )}
                        <div>
                          <div className="text-sm font-bold capitalize text-white">
                            {d.platform || d.clientType}
                          </div>
                          <div className="font-mono text-[11px] text-neutral-500">
                            {d.deviceId.slice(0, 12)}
                          </div>
                        </div>
                      </div>
                      <div className="text-right">
                        <div className="text-sm font-black text-white">
                          {formatDuration(d.totalScreenSeconds)}
                        </div>
                        <div className="text-[11px] text-neutral-400">
                          {d.websiteOpens} opens • {d.blockedAttempts} blocked
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </>
        )}

        {/* Set Daily Limit Modal */}
        {limitTarget && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm">
            <form
              onSubmit={handleSaveLimit}
              className="w-full max-w-md rounded-2xl border border-white/15 bg-[#121217] p-6 shadow-2xl"
            >
              <h3 className="text-lg font-black text-white">Set Daily Time Limit</h3>
              <p className="mt-1 text-xs text-neutral-400">
                Automatically blocks this target across connected devices once daily usage reaches the limit.
              </p>

              {limitModalError && (
                <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-200">
                  {limitModalError}
                </div>
              )}

              <div className="mt-4 space-y-3">
                <div>
                  <label className="block text-xs font-bold text-neutral-300">Type</label>
                  <select
                    value={limitTarget.targetType}
                    onChange={(e) => setLimitTarget({ ...limitTarget, targetType: e.target.value })}
                    className="mt-1 w-full rounded-xl border border-white/10 bg-[#1B1B22] px-3 py-2 text-sm text-white"
                  >
                    <option value="domain">Website Domain</option>
                    <option value="app">Application Package / Exe</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-bold text-neutral-300">
                    {limitTarget.targetType === "domain" ? "Domain (e.g. instagram.com)" : "App ID (e.g. com.instagram.android)"}
                  </label>
                  <input
                    type="text"
                    required
                    value={limitTarget.target}
                    onChange={(e) => setLimitTarget({ ...limitTarget, target: e.target.value, displayName: e.target.value })}
                    placeholder={limitTarget.targetType === "domain" ? "instagram.com" : "com.instagram.android"}
                    className="mt-1 w-full rounded-xl border border-white/10 bg-[#1B1B22] px-3 py-2 text-sm text-white outline-none focus:border-white/30"
                  />
                </div>

                <div>
                  <label className="block text-xs font-bold text-neutral-300">Daily Limit (Minutes)</label>
                  <input
                    type="number"
                    min={1}
                    max={1440}
                    required
                    value={limitMinutes}
                    onChange={(e) => setLimitMinutes(e.target.value)}
                    className="mt-1 w-full rounded-xl border border-white/10 bg-[#1B1B22] px-3 py-2 text-sm text-white outline-none focus:border-white/30"
                  />
                </div>
              </div>

              <div className="mt-5 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => { setLimitModalError(""); setLimitTarget(null); }}
                  className="rounded-xl border border-white/10 px-4 py-2 text-xs font-bold text-neutral-300 hover:bg-white/5"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={savingLimit}
                  className="inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-black text-black hover:bg-neutral-200 disabled:opacity-50"
                >
                  {savingLimit && <Loader2 size={13} className="animate-spin" />}
                  <span>Save Limit</span>
                </button>
              </div>
            </form>
          </div>
        )}

        {/* Quick-Block Accountability Goal Modal */}
        <CreateGoalModal
          isOpen={Boolean(goalPrefill)}
          onClose={() => setGoalPrefill(null)}
          initialCustomTarget={goalPrefill}
          onGoalCreated={() => setGoalPrefill(null)}
        />
      </div>
    </AppLayout>
  );
}
