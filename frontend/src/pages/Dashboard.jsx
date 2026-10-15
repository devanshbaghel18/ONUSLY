import { useState, useEffect, useMemo } from "react";
import { Link } from "react-router-dom";
import AppLayout from "../components/AppLayout";
import {
  Target,
  Plus,
  ChevronRight,
  Loader2,
  Unlock,
  X,
  CheckCircle2,
  Pin,
  RotateCcw,
  Clock,
  Sparkles,
} from "lucide-react";
import { getUser } from "../lib/auth";
import { getGoals, repeatGoal } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";
import CreateGoalModal from "../components/CreateGoalModal";
import ShareProofModal from "../components/ShareProofModal";
import ConnectExtension from "../components/ConnectExtension";
import { notifyExtensionSync } from "../lib/extensionSync";
import {
  getPinnedGoalIds,
  togglePinGoal,
  transferPinGoal,
  MAX_PINNED_GOALS,
} from "../lib/pinnedGoals";

export default function Dashboard() {
  const user = getUser();
  const [goals, setGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [unlockBanner, setUnlockBanner] = useState(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [shareGoal, setShareGoal] = useState(null);

  // Subscribe to real-time WebSocket events
  const { isConnected, send } = useWebSocket((event) => {
    if (event?.type === "goal.unlocked") {
      const payload = event.payload || {};
      console.log("[Dashboard] Real-time unlock event received:", payload);

      // 1. Instantly flip the goal status in local state for zero-latency UI update
      setGoals((prev) =>
        prev.map((g) =>
          g.id === payload.goalId ? { ...g, status: "completed" } : g
        )
      );

      // 2. Display the live celebration banner
      setUnlockBanner({
        goalId: payload.goalId,
        title: payload.title || "Your Goal",
        time: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
      });

      // 3. Immediately trigger extension sync to unblock websites
      notifyExtensionSync();

      // 4. Re-sync with backend in background
      getGoals()
        .then((data) => {
          setGoals(data);
          setError("");
        })
        .catch((err) => console.error("Background goals refresh failed:", err));
    }
  });

  useEffect(() => {
    let ignore = false;
    getGoals()
      .then((data) => {
        if (!ignore) {
          setGoals(data);
          setError("");
        }
      })
      .catch((err) => {
        if (!ignore) {
          console.error("Failed to load goals:", err);
          setError(err.message || "Failed to load goals");
        }
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });

    return () => {
      ignore = true;
    };
  }, []);

  const activeGoals = goals.filter((g) => g.status === "active");
  const pendingApprovals = goals.filter((g) => g.status === "proof_submitted");
  const completedGoals = goals.filter((g) => g.status === "completed");
  const reputationScore = completedGoals.length * 100;

  // Pinned Goals & Recurring Routine State (Max 6 Pinned Goals)
  const [pinnedIds, setPinnedIds] = useState(() => getPinnedGoalIds());
  const [pinNotice, setPinNotice] = useState("");
  const [repeatNotice, setRepeatNotice] = useState("");
  const [repeatingGoalId, setRepeatingGoalId] = useState(null);
  const [dashboardFilter, setDashboardFilter] = useState("all_active"); // "all_active" | "pinned"

  const handleTogglePin = (goalId) => {
    const res = togglePinGoal(goalId);
    setPinnedIds(res.pinnedIds);
    if (res.error) {
      setPinNotice(res.error);
      setTimeout(() => setPinNotice(""), 4500);
    } else if (res.pinned) {
      setPinNotice(`📌 Goal pinned to dashboard (${res.pinnedIds.length}/${MAX_PINNED_GOALS})`);
      setTimeout(() => setPinNotice(""), 3000);
    } else {
      setPinNotice(`Goal unpinned from dashboard (${res.pinnedIds.length}/${MAX_PINNED_GOALS})`);
      setTimeout(() => setPinNotice(""), 2500);
    }
  };

  const handleRepeatGoal = async (g) => {
    if (!g) return;

    // Guard: Prevent creating duplicate active locks for a routine already running
    if (g.status !== "completed") {
      setRepeatNotice(`⚠️ "${g.title}" is currently active! Complete or unlock the current lock before repeating.`);
      setTimeout(() => setRepeatNotice(""), 5000);
      return;
    }

    setRepeatingGoalId(g.id);
    try {
      const newGoal = await repeatGoal(g);

      // If the repeated routine was pinned, automatically transfer the pin to the new active cycle
      if (pinnedIds.includes(g.id)) {
        const updatedPinned = transferPinGoal(g.id, newGoal.id);
        setPinnedIds(updatedPinned);
      }

      setGoals((prev) => [newGoal, ...prev]);
      notifyExtensionSync();
      setRepeatNotice(`🔁 Routine Repeated: New active lock started for "${newGoal.title}"! Target apps & sites are now blocked.`);
      setTimeout(() => setRepeatNotice(""), 6000);
    } catch (err) {
      console.error("Failed to repeat routine:", err);
      setError(err.message || "Failed to repeat routine");
      setTimeout(() => setError(""), 5000);
    } finally {
      setRepeatingGoalId(null);
    }
  };

  // Only show active goals and pinned templates on the dashboard, hiding old completed non-pinned goals
  const activeLockouts = useMemo(() => {
    return goals.filter((g) => g.status === "active" || g.status === "proof_submitted");
  }, [goals]);

  const pinnedGoals = useMemo(() => {
    const set = new Set(pinnedIds);
    return goals.filter((g) => set.has(g.id));
  }, [goals, pinnedIds]);

  const dashboardGoals = useMemo(() => {
    const pinnedSet = new Set(pinnedIds);
    const candidateList =
      dashboardFilter === "pinned"
        ? goals.filter((g) => pinnedSet.has(g.id))
        : [
            ...goals.filter((g) => pinnedSet.has(g.id)),
            ...activeLockouts.filter((g) => !pinnedSet.has(g.id)),
          ];

    // Deduplicate: If multiple instances share the same routine title,
    // display only ONE card on the dashboard (preferring active/pending over completed).
    const uniqueMap = new Map();
    for (const g of candidateList) {
      const key = (g.title || "").trim().toLowerCase();
      if (!uniqueMap.has(key)) {
        uniqueMap.set(key, g);
      } else {
        const existing = uniqueMap.get(key);
        if (existing.status === "completed" && g.status !== "completed") {
          uniqueMap.set(key, g);
        }
      }
    }

    return Array.from(uniqueMap.values());
  }, [dashboardFilter, pinnedIds, goals, activeLockouts]);

  return (
    <AppLayout>
      <div className="mx-auto w-full max-w-[1720px] space-y-8">
        {/* Welcome Header */}
        <div className="flex flex-col justify-between gap-5 border-b border-white/[0.08] pb-7 sm:flex-row sm:items-center">
          <div>
            <div className="flex items-center gap-3 flex-wrap">
              <h1 className="text-3xl font-black tracking-tight text-white sm:text-4xl">
                Welcome back,{" "}
                <span className="bg-gradient-to-r from-white via-neutral-100 to-neutral-400 bg-clip-text text-transparent">
                  {user?.name?.split(" ")?.[0] || "User"}
                </span>
              </h1>
              <div
                className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-bold tracking-wide transition-all ${
                  isConnected
                    ? "border-white/30 bg-white/10 text-white glow-pill-white"
                    : "border-neutral-700 bg-neutral-900 text-neutral-500"
                }`}
                title={isConnected ? "Real-Time WebSocket Connected" : "Connecting to real-time gateway..."}
              >
                <span className="relative flex h-2 w-2">
                  {isConnected && (
                    <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-white opacity-60" />
                  )}
                  <span
                    className={`relative inline-flex h-2 w-2 rounded-full ${
                      isConnected ? "bg-white" : "bg-neutral-600"
                    }`}
                  />
                </span>
                <span>{isConnected ? "Live Sync Active" : "Connecting..."}</span>
              </div>
            </div>
            <p className="mt-1.5 text-sm text-[#A1A1AA] leading-relaxed">
              Your goals are locked and loaded. Complete tasks to release constraints.
            </p>
          </div>

          <div className="flex items-center gap-3 shrink-0">
            <button
              type="button"
              onClick={() => setShowCreateModal(true)}
              className="btn-glow inline-flex items-center gap-2.5 rounded-xl px-5 py-3 text-xs font-black uppercase tracking-wider text-black shadow-lg"
            >
              <Plus size={16} strokeWidth={3} />
              <span>New Accountability Lock</span>
            </button>
          </div>
        </div>

        {/* Real-Time Celebratory Unlock Banner */}
        {unlockBanner && (
          <div className="relative overflow-hidden rounded-2xl border border-white/20 bg-white/[0.06] backdrop-blur-xl p-5 shadow-[0_12px_35px_rgba(0,0,0,0.6),0_0_24px_rgba(255,255,255,0.06)] animate-scale-in">
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-center gap-3.5">
                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-white/20 bg-white/10 text-white shadow-inner">
                  <Unlock size={22} className="animate-pulse" />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="rounded-full bg-white px-2 py-0.5 text-[10px] font-black uppercase tracking-wider text-black">
                      Unlocked Live
                    </span>
                    <span className="text-xs text-neutral-500">{unlockBanner.time}</span>
                  </div>
                  <h3 className="mt-1 text-base font-bold text-white">
                    Goal Unlocked: <span className="text-white font-extrabold">{unlockBanner.title}</span>
                  </h3>
                  <p className="text-xs text-neutral-400 mt-0.5">
                    Your accountability partner approved your proof. All restrictions and application blocks have been released!
                  </p>
                </div>
              </div>

              <button
                type="button"
                onClick={() => setUnlockBanner(null)}
                className="rounded-lg p-1 text-neutral-500 hover:text-white transition hover:bg-white/5"
                title="Dismiss"
              >
                <X size={16} />
              </button>
            </div>
          </div>
        )}

        {error && (
          <div className="rounded-2xl border border-[#444444] bg-[#1E1E1E] p-4 text-xs font-medium text-white animate-fade-in">
            {error}
          </div>
        )}

        {/* 4 Summary Stat Cards (World-Class Metric Cards) */}
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {/* Card 1: Active Goals */}
          <div className="card-stat rounded-2xl p-5 shadow-sm group relative overflow-hidden">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-bold uppercase tracking-wider text-neutral-500 transition-colors group-hover:text-white">Active Goals</span>
              <div className="flex h-8 w-8 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-neutral-300 transition-transform group-hover:scale-110">
                <Target size={16} />
              </div>
            </div>
            <div className="my-2">
              <div className="text-4xl font-black tracking-tight sm:text-5xl stat-metric">
                {loading ? "..." : activeGoals.length}
              </div>
            </div>
            <div className="flex items-center">
              <span className="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/5 px-2.5 py-0.5 text-[11px] font-bold text-neutral-300 glow-pill-white">
                <span className="h-1.5 w-1.5 rounded-full bg-white animate-pulse" />
                {activeGoals.length} Active Lockout{activeGoals.length === 1 ? "" : "s"}
              </span>
            </div>
          </div>

          {/* Card 2: Pending Approvals */}
          <div className="card-stat rounded-2xl p-5 shadow-sm group relative overflow-hidden">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-bold uppercase tracking-wider text-[#A1A1AA] transition-colors group-hover:text-white">Pending Review</span>
              <div className="flex h-8 w-8 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-neutral-300 transition-transform group-hover:scale-110">
                <Clock size={16} />
              </div>
            </div>
            <div className="my-2">
              <div className="text-4xl font-black tracking-tight sm:text-5xl stat-metric">
                {loading ? "..." : pendingApprovals.length}
              </div>
            </div>
            <div className="flex items-center">
              <span className="inline-flex items-center gap-1.5 rounded-full border border-neutral-700 bg-neutral-800/90 px-2.5 py-0.5 text-[11px] font-medium text-neutral-300">
                <span className="h-1.5 w-1.5 rounded-full bg-neutral-400" />
                {pendingApprovals.length > 0 ? "Under Review" : "No Pending Proofs"}
              </span>
            </div>
          </div>

          {/* Card 3: Goals Completed */}
          <div className="card-stat rounded-2xl p-5 shadow-sm group relative overflow-hidden">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-bold uppercase tracking-wider text-neutral-500 transition-colors group-hover:text-white">Goals Completed</span>
              <div className="flex h-8 w-8 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-neutral-300 transition-transform group-hover:scale-110">
                <CheckCircle2 size={16} />
              </div>
            </div>
            <div className="my-2">
              <div className="text-4xl font-black tracking-tight sm:text-5xl stat-metric">
                {loading ? "..." : completedGoals.length}
              </div>
            </div>
            <div className="flex items-center">
              <span className="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/5 px-2.5 py-0.5 text-[11px] font-bold text-neutral-300 glow-pill-white">
                <span className="h-1.5 w-1.5 rounded-full bg-white" />
                {completedGoals.length} Finished Lockout{completedGoals.length === 1 ? "" : "s"}
              </span>
            </div>
          </div>

          {/* Card 4: Reputation Points */}
          <div className="card-stat rounded-2xl p-5 shadow-sm group relative overflow-hidden">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-bold uppercase tracking-wider text-[#A1A1AA] transition-colors group-hover:text-white">Reputation Points</span>
              <div className="flex h-8 w-8 items-center justify-center rounded-xl border border-white/20 bg-white/10 text-white transition-transform group-hover:scale-110">
                <Sparkles size={16} />
              </div>
            </div>
            <div className="my-2">
              <div className="text-4xl font-black tracking-tight sm:text-5xl stat-metric">
                {reputationScore}
              </div>
            </div>
            <div className="flex items-center">
              <span className="inline-flex items-center gap-1.5 rounded-full border border-white/20 bg-white/10 px-2.5 py-0.5 text-[11px] font-bold text-white glow-pill-white">
                <span className="h-1.5 w-1.5 rounded-full bg-white" />
                {completedGoals.length > 0 ? "Verified Consistency" : "New Member"}
              </span>
            </div>
          </div>
        </div>

        {/* Browser Extension Enforcement Status */}
        <ConnectExtension />

        {/* ============================================================
            Your Active & Pinned Lockouts (Dashboard View)
        ============================================================ */}
        <div className="space-y-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <div className="flex items-center gap-2.5">
                <h2 className="text-xl font-black tracking-tight text-white">Your Focus Lockouts</h2>
                {pinnedIds.length > 0 && (
                  <span className="rounded-full border border-white/20 bg-white/10 px-2.5 py-0.5 text-[10px] font-bold text-white tracking-wide glow-pill-white">
                    📌 {pinnedIds.length}/{MAX_PINNED_GOALS} Pinned
                  </span>
                )}
              </div>
              <p className="text-xs text-[#A1A1AA] mt-1">
                Active accountability locks and pinned routines. (Pin up to 6 priority goals to keep them permanently here).
              </p>
            </div>

            <div className="flex items-center gap-3 flex-wrap">
              {/* Filter Tabs */}
              <div className="flex items-center rounded-xl border border-white/10 bg-[#16161A] p-1 text-xs font-semibold shadow-inner">
                <button
                  type="button"
                  onClick={() => setDashboardFilter("all_active")}
                  className={`rounded-lg px-3.5 py-1.5 transition-all duration-200 ${
                    dashboardFilter === "all_active"
                      ? "bg-white text-black font-extrabold shadow-md"
                      : "text-[#A1A1AA] hover:text-white"
                  }`}
                >
                  Active ({activeLockouts.length})
                </button>
                <button
                  type="button"
                  onClick={() => setDashboardFilter("pinned")}
                  className={`flex items-center gap-1.5 rounded-lg px-3.5 py-1.5 transition-all duration-200 ${
                    dashboardFilter === "pinned"
                      ? "bg-white text-black font-extrabold shadow-md"
                      : "text-[#A1A1AA] hover:text-white"
                  }`}
                >
                  <Pin size={12} className={dashboardFilter === "pinned" ? "fill-black" : ""} />
                  <span>Pinned ({pinnedGoals.length}/{MAX_PINNED_GOALS})</span>
                </button>
              </div>

              <Link
                to="/goals"
                className="btn-premium flex items-center gap-1.5 rounded-xl border border-white/10 bg-white/5 px-3.5 py-2 text-xs font-semibold text-white hover:bg-white/10 transition shrink-0"
              >
                <span>Manage all goals</span>
                <ChevronRight size={13} />
              </Link>
            </div>
          </div>

          {/* Feedback Notices */}
          {pinNotice && (
            <div className="flex items-center justify-between rounded-xl border border-white/20 bg-[#16161A] p-3.5 text-xs font-semibold text-white animate-fade-in shadow-lg">
              <div className="flex items-center gap-2.5">
                <Pin size={14} className="fill-white shrink-0" />
                <span>{pinNotice}</span>
              </div>
              <button
                type="button"
                onClick={() => setPinNotice("")}
                className="text-neutral-400 hover:text-white transition p-1"
              >
                <X size={14} />
              </button>
            </div>
          )}

          {repeatNotice && (
            <div className="flex items-center justify-between rounded-xl border border-white/20 bg-[#16161A] p-3.5 text-xs font-semibold text-white animate-fade-in shadow-lg">
              <div className="flex items-center gap-2.5">
                <RotateCcw size={14} className="text-white shrink-0 animate-spin" style={{ animationIterationCount: 1 }} />
                <span>{repeatNotice}</span>
              </div>
              <button
                type="button"
                onClick={() => setRepeatNotice("")}
                className="text-neutral-400 hover:text-white transition p-1"
              >
                <X size={14} />
              </button>
            </div>
          )}

          {loading ? (
            <div className="flex h-56 items-center justify-center rounded-2xl border border-white/10 bg-[#141418]/60 backdrop-blur-xl">
              <Loader2 size={26} className="animate-spin text-white" />
            </div>
          ) : dashboardGoals.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-white/10 bg-[#141418]/40 backdrop-blur-xl p-12 text-center">
              {dashboardFilter === "pinned" ? (
                <>
                  <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl border border-white/10 bg-white/5 text-[#A1A1AA] mb-3">
                    <Pin size={22} />
                  </div>
                  <p className="text-base font-bold text-white">No pinned goals yet</p>
                  <p className="mt-1.5 text-xs text-[#A1A1AA] max-w-md mx-auto leading-relaxed">
                    Click the "Pin" button on any priority goal to keep up to 6 routines permanently visible on your dashboard.
                  </p>
                  <button
                    type="button"
                    onClick={() => setDashboardFilter("all_active")}
                    className="btn-glow mt-5 inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-xs font-bold text-black"
                  >
                    View Active Goals
                  </button>
                </>
              ) : (
                <>
                  <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl border border-white/10 bg-white/5 text-[#A1A1AA] mb-3">
                    <Target size={22} />
                  </div>
                  <p className="text-base font-bold text-white">No active locks right now</p>
                  <p className="mt-1.5 text-xs text-[#A1A1AA] max-w-md mx-auto leading-relaxed">
                    All goals are completed! Start an accountability lock or repeat a routine from past goals.
                  </p>
                  <div className="mt-5 flex items-center justify-center gap-3">
                    <button
                      type="button"
                      onClick={() => setShowCreateModal(true)}
                      className="btn-glow inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-xs font-bold text-black"
                    >
                      <Plus size={14} strokeWidth={3} />
                      Set Up New Lock
                    </button>
                    <Link
                      to="/goals"
                      className="btn-premium inline-flex items-center gap-1 rounded-xl border border-white/10 bg-white/5 px-4 py-2.5 text-xs font-semibold text-white hover:bg-white/10 transition"
                    >
                      View Past Goals
                    </Link>
                  </div>
                </>
              )}
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
              {dashboardGoals.map((g) => {
                const isPendingReview = g.status === "proof_submitted";
                const isCompleted = g.status === "completed";
                const isPinned = pinnedIds.includes(g.id);
                const progressPct = isCompleted ? 100 : isPendingReview ? 50 : 0;

                return (
                  <div
                    key={g.id}
                    className={`card-glass relative flex flex-col justify-between rounded-2xl p-6 shadow-md transition-all duration-300 group ${
                      isPinned
                        ? "border-white/30 shadow-[0_0_30px_rgba(255,255,255,0.04)]"
                        : "border-white/[0.08]"
                    }`}
                  >
                    <div>
                      {/* Top status badges & Pin Toggle Button */}
                      <div className="flex items-center justify-between text-xs">
                        <div className="flex items-center gap-2">
                          {isCompleted ? (
                            <span className="inline-flex items-center gap-1.5 rounded-full border border-white/25 bg-white/10 px-2.5 py-0.5 text-[10px] font-bold text-white glow-pill-white">
                              <CheckCircle2 size={11} className="text-white" />
                              Unlocked & Verified
                            </span>
                          ) : isPendingReview ? (
                            <span className="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/5 px-2.5 py-0.5 text-[10px] font-bold text-neutral-300 glow-pill-white">
                              <Clock size={11} className="text-neutral-400" />
                              Pending Verification
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/5 px-2.5 py-0.5 text-[10px] font-bold text-neutral-300">
                              <span className="h-1.5 w-1.5 rounded-full bg-white animate-pulse" />
                              Active Lockout
                            </span>
                          )}

                          {isPinned && (
                            <span className="flex items-center gap-1 rounded-full border border-white/20 bg-white/10 px-2.5 py-0.5 text-[10px] font-bold text-white">
                              <Pin size={10} className="fill-white" />
                              <span>Pinned Routine</span>
                            </span>
                          )}
                        </div>

                        {/* Pin Button (Max 6) */}
                        <button
                          type="button"
                          onClick={() => handleTogglePin(g.id)}
                          className={`btn-premium flex items-center gap-1 rounded-lg border px-2.5 py-1 text-[11px] font-semibold transition ${
                            isPinned
                              ? "border-white bg-white text-black font-bold shadow-sm"
                              : "border-white/10 bg-white/5 text-[#A1A1AA] hover:text-white hover:border-white/20 hover:bg-white/10"
                          }`}
                          title={
                            isPinned
                              ? "Unpin from dashboard"
                              : `Pin to dashboard (${pinnedIds.length}/${MAX_PINNED_GOALS})`
                          }
                        >
                          <Pin size={12} className={isPinned ? "fill-black" : ""} />
                          <span>{isPinned ? "Pinned" : "Pin"}</span>
                        </button>
                      </div>

                      {/* Title & Notes */}
                      <Link
                        to={`/goals/${g.id}`}
                        className="group/title mt-4 flex items-center justify-between text-base font-bold text-white hover:text-neutral-100 transition-colors"
                      >
                        <span className="line-clamp-1">{g.title}</span>
                        <ChevronRight size={16} className="text-neutral-500 group-hover/title:text-white group-hover/title:translate-x-1 transition-all shrink-0 ml-2" />
                      </Link>
                      {g.description && (
                        <p className="mt-1 text-xs text-[#A1A1AA] line-clamp-2 leading-relaxed">
                          {g.description}
                        </p>
                      )}
                    </div>

                    {/* Progress Bar & Actions */}
                    <div className="mt-6 space-y-3">
                      <div className="flex items-center justify-between text-[11px] font-semibold">
                        <span className="text-[#A1A1AA]">Status</span>
                        <span className="text-white font-bold">
                          {isCompleted ? "Completed & Released" : isPendingReview ? "Proof Submitted" : "In Progress"}
                        </span>
                      </div>
                      <div className="h-2 w-full overflow-hidden rounded-full bg-[#121214] border border-white/10 p-[1px]">
                        <div
                          className={`h-full rounded-full transition-all duration-500 ease-out ${
                            isCompleted
                              ? "bg-white shadow-[0_0_12px_rgba(255,255,255,0.3)]"
                              : isPendingReview
                              ? "bg-gradient-to-r from-neutral-400 to-neutral-300"
                              : "bg-gradient-to-r from-neutral-600 to-white"
                          }`}
                          style={{ width: `${progressPct}%` }}
                        />
                      </div>

                      <div className="flex flex-wrap items-center justify-between gap-2 pt-3 border-t border-white/[0.08]">
                        <div className="flex items-center gap-3">
                          <Link
                            to={`/goals/${g.id}`}
                            className="text-xs font-semibold text-white hover:underline transition-all"
                          >
                            Goal Details →
                          </Link>

                          {/* Recurring Goals Button: "Repeat Routine" */}
                          <button
                            type="button"
                            disabled={repeatingGoalId === g.id || !isCompleted}
                            onClick={() => handleRepeatGoal(g)}
                            className={`btn-premium inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-[11px] font-semibold transition ${
                              isCompleted
                                ? "border-white/15 bg-white/5 text-white hover:border-white/30 hover:bg-white/10"
                                : "border-white/5 bg-white/[0.02] text-neutral-600 cursor-not-allowed opacity-50"
                            }`}
                            title={
                              isCompleted
                                ? "Start another cycle of this lock without re-filling the form"
                                : "Routine is currently active"
                            }
                          >
                            {repeatingGoalId === g.id ? (
                              <Loader2 size={12} className="animate-spin text-white" />
                            ) : (
                              <RotateCcw size={12} className={isCompleted ? "text-white" : "text-neutral-600"} />
                            )}
                            <span>Repeat Routine</span>
                          </button>
                        </div>

                        {isCompleted ? (
                          <span className="text-xs font-semibold text-white flex items-center gap-1">
                            <CheckCircle2 size={13} />
                            Approved
                          </span>
                        ) : (
                          <button
                            type="button"
                            onClick={() => setShareGoal(g)}
                            className="btn-premium inline-flex items-center gap-1 rounded-lg border border-white/15 bg-white/5 px-2.5 py-1 text-xs font-semibold text-white hover:bg-white/10 hover:border-white/30 transition"
                            title="Share proof for verification in chat"
                          >
                            <span>Share Proof in Chat</span>
                            <span aria-hidden="true">↗</span>
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Modal Popup for Creating New Goal Lock */}
        <CreateGoalModal
          isOpen={showCreateModal}
          onClose={() => setShowCreateModal(false)}
          onGoalCreated={(newGoal) => {
            setGoals((prev) => [newGoal, ...prev]);
          }}
        />

        {/* Modal Popup for Sharing Goal Proof / Verification Request */}
        <ShareProofModal
          isOpen={Boolean(shareGoal)}
          onClose={() => setShareGoal(null)}
          goal={shareGoal}
          onSuccess={(updatedGoal) => {
            setGoals((prev) =>
              prev.map((g) =>
                g.id === updatedGoal.id ? { ...g, ...updatedGoal } : g
              )
            );
          }}
          send={send}
        />
      </div>
    </AppLayout>
  );
}
