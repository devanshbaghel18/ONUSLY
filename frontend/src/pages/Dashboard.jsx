import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import AppLayout from "../components/AppLayout";
import {
  Target,
  Plus,
  ChevronRight,
  Loader2,
} from "lucide-react";
import { getUser } from "../lib/auth";
import { getGoals } from "../lib/api";

export default function Dashboard() {
  const user = getUser();
  const [goals, setGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

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
            <h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
              Welcome back, {user?.name?.split(" ")?.[0] || "User"}
            </h1>
            <p className="mt-1 text-sm text-[#A3A3A3]">
              Your goals are locked and loaded. Complete tasks to release constraints.
            </p>
          </div>

          <div className="flex items-center gap-3">
            <Link
              to="/goals"
              className="flex items-center gap-2 rounded-xl bg-white px-5 py-2.5 text-xs font-bold text-black transition-all hover:bg-neutral-200"
            >
              <Plus size={16} />
              New Accountability Lock
            </Link>
          </div>
        </div>

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
              <Link
                to="/goals"
                className="mt-4 inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-black transition-all hover:bg-neutral-200"
              >
                <Plus size={14} />
                Set Up First Lock
              </Link>
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
                          {g.approvalType === "community"
                            ? "Community Quorum"
                            : g.approvalType === "friend"
                            ? "Single Friend"
                            : "Honor System"}
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
                          className="text-xs font-semibold text-white underline hover:text-neutral-300"
                        >
                          Configure Verification →
                        </Link>
                        {g.approvalType === "friend" && g.approverEmail && (
                          <span className="text-[10px] text-[#737373]">
                            Partner: {g.approverEmail}
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

      </div>
    </AppLayout>
  );
}
