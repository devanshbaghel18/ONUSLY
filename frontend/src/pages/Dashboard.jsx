import LockMechanism from "../components/LockMechanism";
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
} from "lucide-react";
import { getUser } from "../lib/auth";
import { getGoals, repeatGoal } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";
import CreateGoalModal from "../components/CreateGoalModal";
import SubmitProofModal from "../components/approvals/SubmitProofModal";
import ConnectExtension from "../components/ConnectExtension";
import DeviceCoverageSection from "../components/DeviceCoverageSection";
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
      setPinNotice(`Goal pinned to dashboard (${res.pinnedIds.length}/${MAX_PINNED_GOALS})`);
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
      setRepeatNotice(`"${g.title}" is currently active. Complete or unlock the current lock before repeating.`);
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
      setRepeatNotice(`Routine Repeated: New active lock started for "${newGoal.title}". Target apps & sites are now blocked.`);
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

  const featuredLock = [unlockBanner && goals.find(goal => goal.id === unlockBanner.goalId), ...pendingApprovals, ...goals]
    .find(goal => goal && (goal.blockedApps?.length || goal.blockedDomains?.length || goal.applicationTargets?.length));
  const featuredStatus = {
    active: "Commitment active",
    proof_submitted: "Proof under review",
    rejected: "Revision requested",
    completed: "Commitment complete",
  }[featuredLock?.status];
  return (
    <AppLayout>
      <div className="mx-auto w-full max-w-[1720px] space-y-8">
        <section className="desk-header">
          <div className="desk-copy"><span className="instrument-label"><i /> {user?.name ? `${user.name.split(" ")[0]}’s workspace` : "Your workspace"}</span>
            <h1>Your lock<br />desk<span>.</span></h1>
            <button className="signal-button" onClick={() => setShowCreateModal(true)}><Plus size={18} /> Create a lock</button>
            <dl className="desk-counts">{[["Active goals", activeGoals.length], ["In review", pendingApprovals.length], ["Completed", completedGoals.length]].map(([label,count]) => <div key={label}><dt>{label}</dt><dd>{loading || error ? "—" : count}</dd></div>)}</dl>
          </div>
          <div className="desk-object">
            <LockMechanism released={featuredLock?.status === "completed"} reviewing={featuredLock?.status === "proof_submitted"} interactive />
            {featuredLock ? <Link className="desk-object-caption" to={`/goals/${featuredLock.id}`}>
              <span className="instrument-label">{featuredStatus}</span><strong>{featuredLock.title}</strong><ChevronRight size={17} />
            </Link> : <p className="desk-object-caption">Lock preview · Drag to rotate</p>}
          </div>
        </section>

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

        {/* ============================================================
            Your Active & Pinned Lockouts (Dashboard View)
        ============================================================ */}
        <div className="space-y-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <div className="flex items-center gap-2.5">
                <h2 className="text-xl font-black tracking-tight text-white">Your Focus Lockouts</h2>
                {pinnedIds.length > 0 && (
                  <span className="inline-flex items-center gap-1 rounded border border-[#2b3542] bg-[#161c23] px-2 py-0.5 text-[10px] font-mono font-bold text-neutral-300">
                    <Pin size={10} className="text-[#ff7843]" />
                    <span>{pinnedIds.length}/{MAX_PINNED_GOALS} PINNED</span>
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
                  className={`rounded-lg px-3.5 py-1.5 transition-all duration-200 ${dashboardFilter === "all_active"
                    ? "bg-white text-black font-extrabold shadow-md"
                    : "text-[#A1A1AA] hover:text-white"
                    }`}
                >
                  Active ({activeLockouts.length})
                </button>
                <button
                  type="button"
                  onClick={() => setDashboardFilter("pinned")}
                  className={`flex items-center gap-1.5 rounded-lg px-3.5 py-1.5 transition-all duration-200 ${dashboardFilter === "pinned"
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
          ) : error ? null : dashboardGoals.length === 0 ? (
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
                    Start an accountability lock or repeat a routine from past goals.
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
                    data-lock-state={g.status}
                    className={`lock-record card-glass relative flex flex-col justify-between rounded-2xl p-6 shadow-md transition-all duration-300 group ${isPinned
                      ? "border-white/30 shadow-[0_0_30px_rgba(255,255,255,0.04)]"
                      : "border-white/[0.08]"
                      }`}
                  >
                    <div>
                      {/* Top status badges & Pin Toggle Button */}
                      <div className="flex items-center justify-between text-xs">
                        <div className="flex items-center gap-2">
                          {isCompleted ? (
                            <span className="inline-flex items-center gap-1.5 rounded border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-[10px] font-mono font-bold tracking-wider text-emerald-400 uppercase">
                              <CheckCircle2 size={11} className="text-emerald-400" />
                              Unlocked & Verified
                            </span>
                          ) : isPendingReview ? (
                            <span className="inline-flex items-center gap-1.5 rounded border border-amber-500/30 bg-amber-500/10 px-2.5 py-0.5 text-[10px] font-mono font-bold tracking-wider text-amber-300 uppercase">
                              <span className="h-1.5 w-1.5 rounded-full bg-amber-400 animate-pulse" />
                              Witness Review
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1.5 rounded border border-[#ff7843]/30 bg-[#ff7843]/10 px-2.5 py-0.5 text-[10px] font-mono font-bold tracking-wider text-[#ff9769] uppercase">
                              <span className="h-1.5 w-1.5 rounded-full bg-[#ff7843] animate-pulse" />
                              Lock Engaged
                            </span>
                          )}

                          {isPinned && (
                            <span className="flex items-center gap-1 rounded border border-[#384654] bg-[#1a2128] px-2 py-0.5 text-[10px] font-mono text-neutral-300">
                              <Pin size={10} className="fill-neutral-300" />
                              <span>PINNED</span>
                            </span>
                          )}
                        </div>

                        {/* Pin Button (Max 6) */}
                        <button
                          type="button"
                          onClick={() => handleTogglePin(g.id)}
                          className={`btn-premium flex items-center gap-1 rounded border px-2.5 py-1 text-[11px] font-semibold transition ${isPinned
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

                    {/* Analog Segmented Mechanical Gauge */}
                    <div className="mt-6 space-y-2.5">
                      <div className="flex items-center justify-between text-[11px] font-mono tracking-wider uppercase text-neutral-400">
                        <span className="flex items-center gap-1.5">
                          <span className="text-neutral-500">PHASE //</span>
                          <span className={isCompleted ? "text-emerald-400 font-bold" : isPendingReview ? "text-amber-300 font-bold" : "text-white font-bold"}>
                            {isCompleted ? "RELEASED" : isPendingReview ? "WITNESS REVIEW" : "LOCKED"}
                          </span>
                        </span>
                        <span className="text-neutral-400 font-bold">
                          {isCompleted ? "100%" : isPendingReview ? "50%" : "0%"}
                        </span>
                      </div>
                      <div className="machined-gauge" data-state={g.status}>
                        <div className="machined-gauge-tick" data-active="true" />
                        <div className="machined-gauge-tick" data-active={isPendingReview || isCompleted ? "true" : "false"} />
                        <div className="machined-gauge-tick" data-active={isCompleted ? "true" : "false"} />
                        <div className="machined-gauge-tick" data-active={isCompleted ? "true" : "false"} />
                      </div>

                      <div className="flex flex-wrap items-center justify-between gap-2 pt-3 border-t border-white/[0.08]">
                        <div className="flex items-center gap-3">
                          <Link
                            to={`/goals/${g.id}`}
                            className="text-xs font-semibold text-neutral-300 hover:text-white transition-all"
                          >
                            Goal Details →
                          </Link>

                          {/* Recurring Goals Button: "Repeat Routine" */}
                          <button
                            type="button"
                            disabled={repeatingGoalId === g.id || !isCompleted}
                            onClick={() => handleRepeatGoal(g)}
                            className={`btn-premium inline-flex items-center gap-1.5 rounded border px-2.5 py-1 text-[11px] font-semibold transition ${isCompleted
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
                          <span className="text-xs font-mono font-semibold text-emerald-400 flex items-center gap-1">
                            <CheckCircle2 size={13} />
                            RELEASED
                          </span>
                        ) : g.status === "proof_submitted" ? (
                          <span className="inline-flex items-center gap-1.5 rounded border border-amber-500/30 bg-amber-500/10 px-2.5 py-1 text-xs font-mono font-semibold text-amber-300">
                            <span className="h-1.5 w-1.5 rounded-full bg-amber-400 animate-pulse" />
                            IN REVIEW
                          </span>
                        ) : (
                          <button
                            type="button"
                            onClick={() => setShareGoal(g)}
                            className="inline-flex items-center gap-1.5 rounded border border-[#ff7843]/40 bg-[#ff7843] px-3.5 py-1 text-xs font-bold text-black hover:bg-[#ff956b] transition shadow-sm"
                            title="Submit proof for verification"
                          >
                            <span>Submit Proof</span>
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

        <section className="coverage-section"><header><h2>Device coverage</h2><span>{isConnected ? "Live sync connected" : "Connecting to live sync"}</span></header><ConnectExtension /><DeviceCoverageSection /></section>

        {/* Modal Popup for Creating New Goal Lock */}
        <CreateGoalModal
          isOpen={showCreateModal}
          onClose={() => setShowCreateModal(false)}
          onGoalCreated={(newGoal) => {
            setGoals((prev) => [newGoal, ...prev]);
          }}
        />

        {/* Modal Popup for Submitting Goal Proof / Verification Request */}
        <SubmitProofModal
          isOpen={Boolean(shareGoal)}
          onClose={() => setShareGoal(null)}
          goal={shareGoal}
          onSuccess={() => {
            setGoals((prev) =>
              prev.map((g) =>
                g.id === shareGoal.id ? { ...g, status: "proof_submitted" } : g
              )
            );
            setShareGoal(null);
          }}
          send={send}
        />
      </div>
    </AppLayout>
  );
}
