import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import AppLayout from "../components/AppLayout";
import {
  Target,
  Lock,
  Unlock,
  Trash2,
  ChevronRight,
  Loader2,
  CheckCircle2,
  AlertCircle,
  Plus,
  MessageSquare,
} from "lucide-react";
import { getGoals, deleteGoal } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";
import CreateGoalModal, {
  APPS_TO_BLOCK,
  getGoalBlockedApps,
} from "../components/CreateGoalModal";

export default function Goals() {
  const [goals, setGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [successNotice, setSuccessNotice] = useState("");
  const [showCreateModal, setShowCreateModal] = useState(false);

  useWebSocket((event) => {
    if (event?.type === "goal.unlocked") {
      const payload = event.payload || {};
      setGoals((prev) =>
        prev.map((g) => (g.id === payload.goalId ? { ...g, status: "completed" } : g))
      );
      setSuccessNotice(`🎉 Real-time: "${payload.title || "Goal"}" was approved and unlocked!`);
      setTimeout(() => setSuccessNotice(""), 6000);
      getGoals().then((data) => setGoals(data)).catch(() => {});
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

  const handleGoalCreated = (newGoal) => {
    setGoals((prev) => [newGoal, ...prev]);
    setSuccessNotice(`Lock activated for "${newGoal.title}"! Target apps are now blocked.`);
    setTimeout(() => setSuccessNotice(""), 4500);
  };

  const handleDelete = async (goalId, e) => {
    e.preventDefault();
    e.stopPropagation();
    if (!window.confirm("Permanently delete this goal?")) return;

    try {
      await deleteGoal(goalId);
      setGoals((prev) => prev.filter((g) => g.id !== goalId));
      setSuccessNotice("Goal removed.");
      setTimeout(() => setSuccessNotice(""), 3000);
    } catch (err) {
      setError(err.message || "Failed to delete goal");
    }
  };

  return (
    <AppLayout>
      <div className="mx-auto max-w-7xl space-y-6">
        {/* Top Header */}
        <div className="flex flex-col justify-between gap-4 border-b border-[#2A2A2A] pb-6 sm:flex-row sm:items-center">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
              Lockout Goals
            </h1>
            <p className="mt-1 text-xs sm:text-sm text-[#A3A3A3]">
              Active focus tasks and distraction constraints. Click + to start a new lock.
            </p>
          </div>

          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={() => setShowCreateModal(true)}
              className="flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-black hover:bg-neutral-200 transition shadow-sm"
              title="Create new accountability lock"
            >
              <Plus size={16} />
              <span>New Lock</span>
            </button>
          </div>
        </div>

        {/* Notices */}
        {successNotice && (
          <div className="flex items-center gap-2 rounded-2xl border border-emerald-500/50 bg-emerald-950/40 p-4 text-xs font-semibold text-emerald-300">
            <CheckCircle2 size={16} className="flex-shrink-0" />
            <span>{successNotice}</span>
          </div>
        )}
        {error && (
          <div className="flex items-center gap-2 rounded-2xl border border-[#444444] bg-[#1E1E1E] p-4 text-xs font-semibold text-white">
            <AlertCircle size={16} className="flex-shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {/* Goals List (Full Width Clean View) */}
        <div>
          <div className="flex items-center justify-between pb-3">
            <h2 className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
              Your Created Goals ({goals.length})
            </h2>

            <span className="text-[11px] text-[#737373]">
              Submit proof in chat using <strong>+</strong> to release apps
            </span>
          </div>

          {loading ? (
            <div className="flex h-64 items-center justify-center rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A]">
              <Loader2 size={24} className="animate-spin text-white" />
            </div>
          ) : goals.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-[#333333] bg-[#1A1A1A]/30 p-12 text-center">
              <Target size={40} className="mx-auto text-[#737373] mb-3" />
              <h3 className="text-base font-bold text-white">No active goals yet</h3>
              <p className="mt-1 text-xs text-[#A3A3A3] max-w-sm mx-auto">
                Ready to focus? Click below to create your accountability lock and block distracting apps.
              </p>
              <button
                type="button"
                onClick={() => setShowCreateModal(true)}
                className="mt-5 inline-flex items-center gap-2 rounded-xl bg-white px-5 py-2.5 text-xs font-bold text-black hover:bg-neutral-200 transition"
              >
                <Plus size={15} />
                <span>Create Your First Lock</span>
              </button>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
              {goals.map((g) => {
                const isCompleted = g.status === "completed";
                const isProofSubmitted = g.status === "proof_submitted";
                const blockedAppIds = getGoalBlockedApps(g.id);

                return (
                  <div
                    key={g.id}
                    className="group relative flex flex-col justify-between rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-5 transition-all hover:border-[#444444] shadow-sm"
                  >
                    <div>
                      {/* Top Row: Title + Status Badge */}
                      <div className="flex items-start justify-between gap-3">
                        <Link
                          to={`/goals/${g.id}`}
                          className="text-base font-bold text-white hover:underline transition line-clamp-1 flex-1"
                        >
                          {g.title}
                        </Link>

                        <div className="flex items-center gap-1.5 flex-shrink-0">
                          {isCompleted ? (
                            <span className="flex items-center gap-1 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-[10px] font-bold text-emerald-400">
                              <Unlock size={11} />
                              Unlocked
                            </span>
                          ) : isProofSubmitted ? (
                            <span className="flex items-center gap-1 rounded-full border border-[#444444] bg-[#262626] px-2.5 py-0.5 text-[10px] font-bold text-white">
                              Pending
                            </span>
                          ) : (
                            <span className="flex items-center gap-1 rounded-full border border-[#444444] bg-[#222222] px-2.5 py-0.5 text-[10px] font-bold text-[#D4D4D4]">
                              <Lock size={11} className="text-amber-400" />
                              Active
                            </span>
                          )}

                          <button
                            type="button"
                            onClick={(e) => handleDelete(g.id, e)}
                            className="rounded-lg p-1 text-[#737373] opacity-0 transition-all hover:text-red-400 group-hover:opacity-100"
                            title="Delete Goal"
                          >
                            <Trash2 size={13} />
                          </button>
                        </div>
                      </div>

                      {/* Description */}
                      {g.description ? (
                        <p className="mt-2 text-xs text-[#A3A3A3] line-clamp-2 leading-relaxed">
                          {g.description}
                        </p>
                      ) : (
                        <p className="mt-2 text-xs italic text-[#666666]">
                          No description provided.
                        </p>
                      )}

                      {/* Blocked Apps list */}
                      <div className="mt-3.5 flex flex-wrap items-center gap-1.5">
                        {blockedAppIds.length > 0 ? (
                          blockedAppIds.map((appId) => {
                            const appInfo = APPS_TO_BLOCK.find((a) => a.id === appId);
                            return (
                              <span
                                key={appId}
                                className="inline-flex items-center gap-1 rounded-md border border-[#333333] bg-[#141414] px-2 py-0.5 text-[10px] font-medium text-neutral-300"
                              >
                                <Lock size={9} className="text-red-400" />
                                {appInfo ? appInfo.label : appId}
                              </span>
                            );
                          })
                        ) : (
                          <span className="text-[10px] text-[#737373] italic">
                            Full Focus Shield Active
                          </span>
                        )}
                      </div>
                    </div>

                    {/* Card Footer Actions */}
                    <div className="mt-5 flex items-center justify-between border-t border-[#2B2B2B] pt-3 text-[11px]">
                      <Link
                        to="/community-friends"
                        className="inline-flex items-center gap-1.5 rounded-lg border border-[#3A3A3A] bg-[#222222] px-2.5 py-1 font-semibold text-white hover:border-white transition"
                        title="Open chat to share proof with the + button"
                      >
                        <MessageSquare size={12} className="text-emerald-400" />
                        <span>Share Proof</span>
                      </Link>

                      <Link
                        to={`/goals/${g.id}`}
                        className="flex items-center gap-1 font-semibold text-[#A3A3A3] hover:text-white transition"
                      >
                        <span>Details</span>
                        <ChevronRight size={13} />
                      </Link>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Modal Window for Creating New Goal */}
        <CreateGoalModal
          isOpen={showCreateModal}
          onClose={() => setShowCreateModal(false)}
          onGoalCreated={handleGoalCreated}
        />
      </div>
    </AppLayout>
  );
}
