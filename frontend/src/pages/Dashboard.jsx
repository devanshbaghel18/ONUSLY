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
      <div className="mx-auto max-w-7xl space-y-8">
        {/* Welcome Header */}
        <div className="flex flex-col justify-between gap-4 border-b border-[#2A2A2A] pb-6 sm:flex-row sm:items-center">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
                Welcome back, {user?.name?.split(" ")?.[0] || "User"}
              </h1>
              <div
                className={`flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[10px] font-semibold tracking-wide transition-colors ${
                  isConnected
                    ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300"
                    : "border-neutral-700 bg-neutral-900 text-neutral-400"
                }`}
                title={isConnected ? "Real-Time WebSocket Connected" : "Connecting to real-time gateway..."}
              >
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    isConnected ? "bg-emerald-400 animate-pulse" : "bg-neutral-500"
                  }`}
                />
                <span>{isConnected ? "Live Sync Active" : "Connecting..."}</span>
              </div>
            </div>
            <p className="mt-1 text-sm text-[#A3A3A3]">
              Your goals are locked and loaded. Complete tasks to release constraints.
            </p>
          </div>

          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => setShowCreateModal(true)}
              className="flex items-center gap-2 rounded-xl bg-white px-5 py-2.5 text-xs font-bold text-black transition-all hover:bg-neutral-200 shadow-sm"
            >
              <Plus size={16} />
              New Accountability Lock
            </button>
          </div>
        </div>

        {/* Real-Time Celebratory Unlock Banner */}
        {unlockBanner && (
          <div className="relative overflow-hidden rounded-2xl border border-emerald-500/50 bg-gradient-to-r from-emerald-950/80 via-[#1A2E1E] to-[#121212] p-5 shadow-[0_10px_35px_rgba(16,185,129,0.25)] transition-all">
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-center gap-3.5">
                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-emerald-400/40 bg-emerald-500/20 text-emerald-300 shadow-inner">
                  <Unlock size={22} className="animate-pulse" />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="rounded-full bg-emerald-400 px-2 py-0.5 text-[10px] font-black uppercase tracking-wider text-black">
                      Unlocked Live
                    </span>
                    <span className="text-xs text-emerald-300/80">{unlockBanner.time}</span>
                  </div>
                  <h3 className="mt-1 text-base font-bold text-white">
                    Goal Unlocked: <span className="text-emerald-300 font-extrabold">{unlockBanner.title}</span>
                  </h3>
                  <p className="text-xs text-neutral-300 mt-0.5">
                    Your accountability partner approved your proof. All restrictions and application blocks have been released!
                  </p>
                </div>
              </div>

              <button
                type="button"
                onClick={() => setUnlockBanner(null)}
                className="rounded-lg p-1 text-neutral-400 hover:text-white transition hover:bg-neutral-800"
                title="Dismiss"
              >
                <X size={16} />
              </button>
            </div>
          </div>
        )}

        {error && (
          <div className="rounded-2xl border border-[#444444] bg-[#1E1E1E] p-4 text-xs font-medium text-white">
            {error}
          </div>
        )}

        {/* 4 Summary Stat Cards (Monochrome) */}
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {/* Card 1: Active Goals */}
          <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-5 shadow-sm">
            <span className="text-xs font-semibold text-[#A3A3A3]">Active Goals</span>
            <div className="mt-3 flex items-baseline justify-between">
              <span className="text-3xl font-extrabold text-white">
                {loading ? "..." : activeGoals.length}
              </span>
              <span className="rounded-full border border-[#444444] bg-[#242424] px-2.5 py-0.5 text-[11px] font-semibold text-white">
                {activeGoals.length} Active
              </span>
            </div>
          </div>

          {/* Card 2: Pending Approvals */}
          <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-5 shadow-sm">
            <span className="text-xs font-semibold text-[#A3A3A3]">Pending Approvals</span>
            <div className="mt-3 flex items-baseline justify-between">
              <span className="text-3xl font-extrabold text-white">
                {loading ? "..." : pendingApprovals.length}
              </span>
              <span className="rounded-full border border-[#444444] bg-[#242424] px-2.5 py-0.5 text-[11px] font-semibold text-white">
                {pendingApprovals.length > 0 ? "Under Review" : "0 Pending"}
              </span>
            </div>
          </div>

          {/* Card 3: Goals Completed */}
          <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-5 shadow-sm">
            <span className="text-xs font-semibold text-[#A3A3A3]">Goals Completed</span>
            <div className="mt-3 flex items-baseline justify-between">
              <span className="text-3xl font-extrabold text-white">
                {loading ? "..." : completedGoals.length}
              </span>
              <span className="rounded-full border border-white/20 bg-white/10 px-2.5 py-0.5 text-[11px] font-semibold text-white">
                {completedGoals.length} Finished
              </span>
            </div>
          </div>

          {/* Card 4: Reputation Points */}
          <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-5 shadow-sm">
            <span className="text-xs font-semibold text-[#A3A3A3]">Reputation Points</span>
            <div className="mt-3 flex items-baseline justify-between">
              <span className="text-3xl font-extrabold text-white">{reputationScore}</span>
              <span className="rounded-full border border-[#444444] bg-[#242424] px-2.5 py-0.5 text-[11px] font-semibold text-white">
                {completedGoals.length > 0 ? "Verified" : "New Member"}
              </span>
            </div>
          </div>
        </div>

        {/* Browser Extension Enforcement Status */}
        <ConnectExtension />

        {/* ============================================================
            Your Active & Pinned Lockouts (Dashboard View)
        ============================================================ */}
        <div className="space-y-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <div className="flex items-center gap-2.5">
                <h2 className="text-lg font-bold text-white">Your Focus Lockouts</h2>
                {pinnedIds.length > 0 && (
                  <span className="rounded-full border border-amber-500/40 bg-amber-500/10 px-2.5 py-0.5 text-[10px] font-bold text-amber-400">
                    📌 {pinnedIds.length}/{MAX_PINNED_GOALS} Pinned
                  </span>
                )}
              </div>
              <p className="text-xs text-[#A3A3A3] mt-0.5">
                Active accountability locks and pinned routines. (Pin up to 6 priority goals to keep them here).
              </p>
            </div>

            <div className="flex items-center gap-3">
              {/* Filter Tabs */}
              <div className="flex items-center rounded-xl border border-[#333333] bg-[#1A1A1A] p-1 text-xs font-semibold">
                <button
                  type="button"
                  onClick={() => setDashboardFilter("all_active")}
                  className={`rounded-lg px-3 py-1.5 transition ${
                    dashboardFilter === "all_active"
                      ? "bg-white text-black font-bold shadow-sm"
                      : "text-[#A3A3A3] hover:text-white"
                  }`}
                >
                  Active ({activeLockouts.length})
                </button>
                <button
                  type="button"
                  onClick={() => setDashboardFilter("pinned")}
                  className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 transition ${
                    dashboardFilter === "pinned"
                      ? "bg-white text-black font-bold shadow-sm"
                      : "text-[#A3A3A3] hover:text-white"
                  }`}
                >
                  <Pin size={12} className={dashboardFilter === "pinned" ? "fill-black" : ""} />
                  <span>Pinned ({pinnedGoals.length}/{MAX_PINNED_GOALS})</span>
                </button>
              </div>

              <Link
                to="/goals"
                className="flex items-center gap-1 text-xs font-semibold text-white hover:underline shrink-0"
              >
                Manage all goals
                <ChevronRight size={14} />
              </Link>
            </div>
          </div>

          {/* Feedback Notices */}
          {pinNotice && (
            <div className="flex items-center justify-between rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs font-semibold text-amber-300">
              <div className="flex items-center gap-2">
                <Pin size={14} className="fill-amber-400 shrink-0" />
                <span>{pinNotice}</span>
              </div>
              <button
                type="button"
                onClick={() => setPinNotice("")}
                className="text-amber-400 hover:text-white transition"
              >
                <X size={14} />
              </button>
            </div>
          )}

          {repeatNotice && (
            <div className="flex items-center justify-between rounded-xl border border-emerald-500/40 bg-emerald-950/60 p-3 text-xs font-semibold text-emerald-300">
              <div className="flex items-center gap-2">
                <RotateCcw size={14} className="text-emerald-400 shrink-0 animate-spin" style={{ animationIterationCount: 1 }} />
                <span>{repeatNotice}</span>
              </div>
              <button
                type="button"
                onClick={() => setRepeatNotice("")}
                className="text-emerald-400 hover:text-white transition"
              >
                <X size={14} />
              </button>
            </div>
          )}

          {loading ? (
            <div className="flex h-48 items-center justify-center rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A]">
              <Loader2 size={24} className="animate-spin text-white" />
            </div>
          ) : dashboardGoals.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-[#333333] bg-[#1A1A1A]/30 p-10 text-center">
              {dashboardFilter === "pinned" ? (
                <>
                  <Pin size={32} className="mx-auto text-[#737373] mb-2" />
                  <p className="text-sm font-semibold text-white">No pinned goals yet</p>
                  <p className="mt-1 text-xs text-[#A3A3A3]">
                    Click the "Pin" button on any goal to keep up to 6 favorite routines permanently on your dashboard.
                  </p>
                  <button
                    type="button"
                    onClick={() => setDashboardFilter("all_active")}
                    className="mt-4 inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-black transition-all hover:bg-neutral-200"
                  >
                    View Active Goals
                  </button>
                </>
              ) : (
                <>
                  <Target size={32} className="mx-auto text-[#737373] mb-2" />
                  <p className="text-sm font-semibold text-white">No active locks right now</p>
                  <p className="mt-1 text-xs text-[#A3A3A3]">
                    All goals are completed! Start an accountability lock or repeat a routine from past goals.
                  </p>
                  <div className="mt-4 flex items-center justify-center gap-3">
                    <button
                      type="button"
                      onClick={() => setShowCreateModal(true)}
                      className="inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-black transition-all hover:bg-neutral-200"
                    >
                      <Plus size={14} />
                      Set Up New Lock
                    </button>
                    <Link
                      to="/goals"
                      className="inline-flex items-center gap-1 rounded-xl border border-[#444444] bg-[#222222] px-4 py-2 text-xs font-semibold text-white hover:border-white transition"
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
                    className={`relative flex flex-col justify-between rounded-2xl border p-6 shadow-sm transition-all hover:border-white ${
                      isPinned
                        ? "border-amber-500/40 bg-[#1A1814]"
                        : "border-[#2B2B2B] bg-[#1A1A1A]"
                    }`}
                  >
                    <div>
                      {/* Top status badges & Pin Toggle Button */}
                      <div className="flex items-center justify-between text-xs">
                        <div className="flex items-center gap-2">
                          {isCompleted ? (
                            <span className="rounded-md border border-white/20 bg-white/10 px-2 py-0.5 text-[10px] font-bold text-white">
                              Unlocked
                            </span>
                          ) : isPendingReview ? (
                            <span className="rounded-md border border-[#444444] bg-[#262626] px-2 py-0.5 text-[10px] font-bold text-white">
                              Pending Review
                            </span>
                          ) : (
                            <span className="rounded-md border border-[#444444] bg-[#222222] px-2 py-0.5 text-[10px] font-bold text-[#D4D4D4]">
                              Active Lockout
                            </span>
                          )}

                          {isPinned && (
                            <span className="flex items-center gap-1 rounded-md border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold text-amber-400">
                              <Pin size={10} className="fill-amber-400" />
                              <span>Pinned</span>
                            </span>
                          )}
                        </div>

                        {/* Pin Button (Max 6) */}
                        <button
                          type="button"
                          onClick={() => handleTogglePin(g.id)}
                          className={`flex items-center gap-1 rounded-lg border px-2.5 py-1 text-[11px] font-semibold transition ${
                            isPinned
                              ? "border-amber-500/50 bg-amber-500/15 text-amber-400 hover:bg-amber-500/25"
                              : "border-[#333333] bg-[#222222] text-[#A3A3A3] hover:text-white hover:border-[#555555]"
                          }`}
                          title={
                            isPinned
                              ? "Unpin from dashboard"
                              : `Pin to dashboard (${pinnedIds.length}/${MAX_PINNED_GOALS})`
                          }
                        >
                          <Pin size={12} className={isPinned ? "fill-amber-400" : ""} />
                          <span>{isPinned ? "Pinned" : "Pin"}</span>
                        </button>
                      </div>

                      {/* Title & Notes */}
                      <h3 className="mt-4 text-base font-bold text-white">
                        {g.title}
                      </h3>
                      {g.description && (
                        <p className="mt-1 text-xs text-[#A3A3A3] line-clamp-2">
                          {g.description}
                        </p>
                      )}
                    </div>

                    {/* Progress Bar & Actions */}
                    <div className="mt-6 space-y-3">
                      <div className="flex items-center justify-between text-[11px] font-semibold">
                        <span className="text-[#737373]">Status</span>
                        <span className="text-white">
                          {isCompleted ? "Completed" : isPendingReview ? "Proof Submitted" : "In Progress"}
                        </span>
                      </div>
                      <div className="h-2 w-full overflow-hidden rounded-full bg-[#121212]">
                        <div
                          className="h-full rounded-full bg-white transition-all duration-500"
                          style={{ width: `${progressPct}%` }}
                        />
                      </div>

                      <div className="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-[#262626]">
                        <div className="flex items-center gap-3">
                          <Link
                            to={`/goals/${g.id}`}
                            className="text-xs font-semibold text-white hover:underline"
                          >
                            Goal Details →
                          </Link>

                          {/* Recurring Goals Button: "Repeat Routine" */}
                          <button
                            type="button"
                            disabled={repeatingGoalId === g.id || !isCompleted}
                            onClick={() => handleRepeatGoal(g)}
                            className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-[11px] font-semibold transition ${
                              isCompleted
                                ? "border-[#3A3A3A] bg-[#222222] text-neutral-200 hover:text-white hover:border-white"
                                : "border-[#262626] bg-[#1A1A1A] text-[#666666] cursor-not-allowed opacity-60"
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
                              <RotateCcw size={12} className={isCompleted ? "text-emerald-400" : "text-[#666666]"} />
                            )}
                            <span>Repeat Routine</span>
                          </button>
                        </div>

                        {isCompleted ? (
                          <span className="text-xs font-semibold text-emerald-400 flex items-center gap-1">
                            <CheckCircle2 size={13} />
                            Approved
                          </span>
                        ) : (
                          <button
                            type="button"
                            onClick={() => setShareGoal(g)}
                            className="text-xs font-semibold text-emerald-400 hover:underline flex items-center gap-1 transition"
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
