import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import AppLayout from "../components/AppLayout";
import {
  Target,
  Plus,
  ChevronRight,
  Loader2,
  Unlock,
  X,
} from "lucide-react";
import { getUser } from "../lib/auth";
import { getGoals } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";
import CreateGoalModal from "../components/CreateGoalModal";

export default function Dashboard() {
  const user = getUser();
  const [goals, setGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [unlockBanner, setUnlockBanner] = useState(null);
  const [showCreateModal, setShowCreateModal] = useState(false);

  // Subscribe to real-time WebSocket events
  const { isConnected } = useWebSocket((event) => {
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

      // 3. Re-sync with backend in background
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

        {/* ============================================================
            Your Active Lockouts (Monochrome)
        ============================================================ */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-lg font-bold text-white">Your Active Lockouts</h2>
              <p className="text-xs text-[#A3A3A3]">
                Complete the goal to release target constraints
              </p>
            </div>
            <Link
              to="/goals"
              className="flex items-center gap-1 text-xs font-semibold text-white hover:underline"
            >
              Manage all goals
              <ChevronRight size={14} />
            </Link>
          </div>

          {loading ? (
            <div className="flex h-48 items-center justify-center rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A]">
              <Loader2 size={24} className="animate-spin text-white" />
            </div>
          ) : goals.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-[#333333] bg-[#1A1A1A]/30 p-10 text-center">
              <Target size={32} className="mx-auto text-[#737373] mb-2" />
              <p className="text-sm font-semibold text-white">No active locks right now</p>
              <p className="mt-1 text-xs text-[#A3A3A3]">
                Ready to focus? Start an accountability lock to block distracting sites.
              </p>
              <button
                type="button"
                onClick={() => setShowCreateModal(true)}
                className="mt-4 inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-black transition-all hover:bg-neutral-200"
              >
                <Plus size={14} />
                Set Up First Lock
              </button>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
              {goals.map((g) => {
                const isPendingReview = g.status === "proof_submitted";
                const isCompleted = g.status === "completed";
                const progressPct = isCompleted ? 100 : isPendingReview ? 50 : 0;

                return (
                  <div
                    key={g.id}
                    className="relative flex flex-col justify-between rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 shadow-sm transition-all hover:border-white"
                  >
                    <div>
                      {/* Top status badges */}
                      <div className="flex items-center justify-between text-xs">
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

                        <span className="text-[11px] font-medium text-[#A3A3A3]">
                          Focus Lock Active
                        </span>
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

                      <div className="flex items-center justify-between pt-2">
                        <Link
                          to={`/goals/${g.id}`}
                          className="text-xs font-semibold text-white hover:underline"
                        >
                          Goal Details →
                        </Link>
                        <Link
                          to="/community-friends"
                          className="text-xs font-semibold text-emerald-400 hover:underline"
                        >
                          Share Proof in Chat ↗
                        </Link>
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
      </div>
    </AppLayout>
  );
}
