import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  LogOut,
  Target,
  Plus,
  Trash2,
  Calendar,
  AlertCircle,
  CheckCircle2,
  Loader2,
  X,
} from "lucide-react";
import { getUser, clearAuth } from "../lib/auth";
import { getGoals, createGoal, deleteGoal } from "../lib/api";

export default function Dashboard() {
  const navigate = useNavigate();
  const [user] = useState(() => getUser());
  const [goals, setGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [successNotice, setSuccessNotice] = useState("");

  // Create Goal form state
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newDescription, setNewDescription] = useState("");
  const [newApproverEmail, setNewApproverEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  // Load goals on mount
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
        if (!ignore) {
          setLoading(false);
        }
      });

    return () => {
      ignore = true;
    };
  }, []);

  const handleRefresh = async () => {
    setLoading(true);
    setError("");
    try {
      const data = await getGoals();
      setGoals(data);
    } catch (err) {
      console.error("Failed to load goals:", err);
      setError(err.message || "Failed to load goals");
    } finally {
      setLoading(false);
    }
  };

  // Handle Logout
  const handleLogout = () => {
    clearAuth();
    navigate("/");
  };

  // Handle Create Goal
  const handleCreateGoal = async (e) => {
    e.preventDefault();
    if (!newTitle.trim()) {
      setFormError("Title is required");
      return;
    }

    setSubmitting(true);
    setFormError("");
    try {
    const created = await createGoal({
      title: newTitle,
      description: newDescription,
      approverEmail: newApproverEmail,
    });

    setGoals((prev) => [created, ...prev]);
    setNewTitle("");
    setNewDescription("");
    setNewApproverEmail("");
    setShowCreateModal(false);
    setSuccessNotice("Goal created successfully!");
    setTimeout(() => setSuccessNotice(""), 4000);
  } catch (err) {
    console.error("Failed to create goal:", err);
    setFormError(err.message || "Failed to create goal");
  } finally {
    setSubmitting(false);
  }
};

// Handle Delete Goal
const handleDeleteGoal = async (goalId) => {
  if (!window.confirm("Are you sure you want to delete this goal?")) {
    return;
  }

  try {
    await deleteGoal(goalId);
    setGoals((prev) => prev.filter((g) => g.id !== goalId));
    setSuccessNotice("Goal deleted.");
    setTimeout(() => setSuccessNotice(""), 3000);
  } catch (err) {
    console.error("Failed to delete goal:", err);
    setError(err.message || "Failed to delete goal");
  }
};

const formatDate = (dateString) => {
  if (!dateString) return "";
  try {
    const date = new Date(dateString);
    return date.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    });
  } catch {
    return dateString;
  }
};

return (
  <div className="relative min-h-screen overflow-hidden bg-[#292929] text-[#FFFFFF]">
    {/* Smoky ambient backgrounds */}
    <div className="pointer-events-none absolute -left-40 -top-40 h-[600px] w-[600px] rounded-full bg-[#616161]/20 blur-[120px]" />
    <div className="pointer-events-none absolute -right-40 top-[20%] h-[600px] w-[600px] rounded-full bg-[#777777]/10 blur-[130px]" />
    <div className="pointer-events-none absolute bottom-[-250px] left-[25%] h-[600px] w-[600px] rounded-full bg-[#3A3A3A] blur-[120px]" />

    {/* Subtle texture grid */}
    <div
      className="pointer-events-none absolute inset-0 opacity-[0.045]"
      style={{
        backgroundImage: "radial-gradient(#ffffff 0.7px, transparent 0.7px)",
        backgroundSize: "5px 5px",
      }}
    />

    <div className="relative z-10 flex min-h-screen flex-col">
      {/* =====================================================
          TOP NAVIGATION BAR
      ===================================================== */}
      <header className="border-b border-[#777777]/50 bg-[#3A3A3A]/70 backdrop-blur-md">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-4 sm:px-6 lg:px-8">
          {/* Logo / Brand */}
          <div className="flex items-center gap-3">
            <a href="/dashboard" className="block">
              <img
                src="https://see.fontimg.com/api/rf5/2nxo/MWIyNjZmNmYzMzQ5NGUyYThiMjRlNTZkNzg0MDE2YWUudHRm/T05VU0xZ/ghang.png?r=fs&h=131&w=1250&fg=FFFFFF&bg=292929&tb=1&s=105"
                alt="ONUSLY"
                className="h-7 w-auto object-contain drop-shadow-lg"
              />
            </a>
            <span className="hidden rounded-full border border-[#777777] bg-[#292929] px-2.5 py-0.5 text-[11px] font-semibold tracking-wider text-[#B5B5B5] uppercase sm:inline-block">
              Dashboard
            </span>
          </div>

          {/* User Profile & Logout */}
          <div className="flex items-center gap-3 sm:gap-4">
            {user && (
              <div className="flex items-center gap-3 rounded-full border border-[#777777]/60 bg-[#292929]/80 py-1.5 pl-2 pr-3.5 shadow-sm">
                {user.picture ? (
                  <img
                    src={user.picture}
                    alt={user.name || "User"}
                    className="h-8 w-8 rounded-full object-cover border border-[#777777]"
                    referrerPolicy="no-referrer"
                  />
                ) : (
                  <div className="flex h-8 w-8 items-center justify-center rounded-full bg-[#616161] text-xs font-bold text-white">
                    {(user.name?.[0] || user.email?.[0] || "U").toUpperCase()}
                  </div>
                )}
                <div className="hidden flex-col text-left sm:flex justify-center h-full">
                  <span className="text-sm font-semibold text-[#FFFFFF] leading-tight max-w-[140px] truncate">
                    {user.name || "User"}
                  </span>
                </div>
              </div>
            )}

              <button
                type="button"
                onClick={handleLogout}
                className="group flex items-center gap-2 rounded-xl border border-[#777777] bg-[#4A4A4A] px-3.5 py-2 text-xs font-semibold text-[#FFFFFF] transition-all hover:border-[#B5B5B5] hover:bg-[#616161] active:translate-y-0.5"
                title="Sign out of ONUSLY"
              >
                <LogOut size={15} className="text-[#B5B5B5] transition-colors group-hover:text-white" />
                <span className="hidden sm:inline">Sign Out</span>
              </button>
            </div>
          </div>
        </header>

        {/* =====================================================
            MAIN CONTENT AREA
        ===================================================== */}
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-8 sm:px-6 lg:px-8">
          {/* Welcome Banner */}
          <div className="mb-8 rounded-[24px] border border-[#777777]/70 bg-[#3A3A3A] p-6 shadow-[0_15px_40px_rgba(0,0,0,0.35),inset_0_1px_0_rgba(255,255,255,0.06)] sm:p-8">
            <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
              <div>
                <h1 className="text-2xl font-bold tracking-tight text-[#FFFFFF] sm:text-3xl">
                  Welcome back, {user?.name?.split(" ")?.[0] || "Goal Achiever"} 👋
                </h1>
                <p className="mt-1 text-sm text-[#B5B5B5]">
                  Stay disciplined. Hold yourself accountable and conquer your targets.
                </p>
              </div>

              <button
                type="button"
                onClick={() => setShowCreateModal(true)}
                className="flex items-center justify-center gap-2 rounded-xl bg-[#FFFFFF] px-5 py-3 text-sm font-bold text-[#292929] shadow-[0_4px_15px_rgba(0,0,0,0.3)] transition-all duration-200 hover:-translate-y-0.5 hover:bg-[#B5B5B5] active:translate-y-0"
              >
                <Plus size={18} />
                New Goal
              </button>
            </div>

            {/* Quick Stat Chips */}
            <div className="mt-6 flex flex-wrap items-center gap-3 border-t border-[#777777]/40 pt-6 text-xs text-[#B5B5B5]">
              <span className="flex items-center gap-2 rounded-lg bg-[#292929] px-3.5 py-1.5 font-medium">
                <Target size={14} className="text-[#FFFFFF]" />
                Total Goals:{" "}
                <strong className="text-white">{loading ? "..." : goals.length}</strong>
              </span>
              <span className="flex items-center gap-2 rounded-lg bg-[#292929] px-3.5 py-1.5 font-medium">
                <CheckCircle2 size={14} className="text-emerald-400" />
                Active:{" "}
                <strong className="text-white">
                  {loading ? "..." : goals.filter((g) => g.status === "active").length}
                </strong>
              </span>
            </div>
          </div>

          {/* Feedback Notices */}
          {successNotice && (
            <div className="mb-6 flex items-center gap-3 rounded-xl border border-emerald-500/40 bg-emerald-950/30 px-4 py-3 text-sm text-emerald-300">
              <CheckCircle2 size={18} className="shrink-0" />
              <span>{successNotice}</span>
            </div>
          )}

          {error && (
            <div className="mb-6 flex items-center justify-between rounded-xl border border-rose-500/40 bg-rose-950/30 px-4 py-3 text-sm text-rose-300">
              <div className="flex items-center gap-3">
                <AlertCircle size={18} className="shrink-0" />
                <span>{error}</span>
              </div>
              <button
                type="button"
                onClick={handleRefresh}
                className="underline hover:text-white"
              >
                Retry
              </button>
            </div>
          )}

          {/* Goals Header */}
          <div className="mb-5 flex items-center justify-between">
            <h2 className="text-xl font-bold tracking-tight text-[#FFFFFF]">Your Goals</h2>
            {!loading && goals.length > 0 && (
              <span className="text-xs text-[#B5B5B5]">
                Showing {goals.length} {goals.length === 1 ? "goal" : "goals"}
              </span>
            )}
          </div>

          {/* Goals Content States */}
          {loading ? (
            /* Loading State */
            <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
              {[1, 2, 3].map((i) => (
                <div
                  key={i}
                  className="h-44 animate-pulse rounded-2xl border border-[#777777]/30 bg-[#3A3A3A]/50 p-6"
                />
              ))}
            </div>
          ) : goals.length === 0 ? (
            /* Empty State */
            <div className="flex flex-col items-center justify-center rounded-[24px] border border-dashed border-[#777777] bg-[#3A3A3A]/40 px-6 py-16 text-center">
              <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl border border-[#777777] bg-[#292929] text-[#B5B5B5]">
                <Target size={32} />
              </div>
              <h3 className="text-lg font-bold text-[#FFFFFF]">No goals created yet</h3>
              <p className="mt-1 max-w-md text-sm text-[#B5B5B5]">
                Set clear objectives and hold yourself to them. Create your first goal to get started!
              </p>
              <button
                type="button"
                onClick={() => setShowCreateModal(true)}
                className="mt-6 flex items-center gap-2 rounded-xl bg-[#FFFFFF] px-5 py-3 text-sm font-bold text-[#292929] shadow-md transition hover:bg-[#B5B5B5]"
              >
                <Plus size={18} />
                Create First Goal
              </button>
            </div>
          ) : (
            /* Goals Grid */
            <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
              {goals.map((goal) => (
                <div
                  key={goal.id}
                  className="group flex flex-col justify-between rounded-2xl border border-[#777777] bg-[#3A3A3A] p-6 shadow-md transition-all duration-200 hover:-translate-y-1 hover:border-[#B5B5B5] hover:shadow-[0_15px_30px_rgba(0,0,0,0.35)]"
                >
                  <div>
                    {/* Status & Actions Header */}
                    <div className="flex items-center justify-between gap-2">
                      <span className="rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-[11px] font-semibold text-emerald-300 uppercase tracking-wider">
                        {goal.status || "active"}
                      </span>
                      <button
                        type="button"
                        onClick={() => handleDeleteGoal(goal.id)}
                        className="rounded-lg p-1.5 text-[#B5B5B5] opacity-80 transition hover:bg-rose-500/20 hover:text-rose-400 group-hover:opacity-100"
                        title="Delete Goal"
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>

                    {/* Goal Title */}
                    <h3 className="mt-3 text-lg font-bold tracking-tight text-[#FFFFFF] line-clamp-2">
                      {goal.title}
                    </h3>

                    {/* Goal Description */}
                    {goal.description && (
                      <p className="mt-2 text-sm text-[#B5B5B5] line-clamp-3">
                        {goal.description}
                      </p>
                    )}
                  </div>

                  {/* Goal Footer / Date */}
                  <div className="mt-5 flex items-center gap-2 border-t border-[#777777]/30 pt-4 text-xs text-[#B5B5B5]">
                    <Calendar size={13} />
                    <span>Created {formatDate(goal.createdAt)}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </main>

        {/* Footer */}
        <footer className="mt-auto border-t border-[#777777]/30 py-6 text-center text-[10px] tracking-[0.25em] text-[#777777] uppercase">
          ONUSLY · FOCUS · CONSISTENCY · GROWTH
        </footer>
      </div>

      {/* =====================================================
          CREATE GOAL MODAL
      ===================================================== */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
          <div className="relative w-full max-w-lg rounded-[28px] border border-[#777777] bg-[#3A3A3A] p-7 shadow-[0_25px_70px_rgba(0,0,0,0.6)] sm:p-8">
            <button
              type="button"
              onClick={() => setShowCreateModal(false)}
              className="absolute right-5 top-5 rounded-lg p-2 text-[#B5B5B5] transition hover:bg-[#4A4A4A] hover:text-white"
            >
              <X size={20} />
            </button>

            <div className="mb-6 flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-[#777777] bg-[#292929] text-white">
                <Target size={20} />
              </div>
              <div>
                <h3 className="text-xl font-bold text-white">Create a New Goal</h3>
                <p className="text-xs text-[#B5B5B5]">Define what you intend to accomplish.</p>
              </div>
            </div>

            {formError && (
              <div className="mb-5 rounded-xl border border-rose-500/40 bg-rose-950/30 px-4 py-2.5 text-xs text-rose-300">
                {formError}
              </div>
            )}

            <form onSubmit={handleCreateGoal} className="space-y-4">
              <div>
                <label className="mb-1.5 block text-xs font-semibold tracking-wider text-[#FFFFFF] uppercase">
                  Goal Title <span className="text-rose-400">*</span>
                </label>
                <input
                  type="text"
                  value={newTitle}
                  onChange={(e) => setNewTitle(e.target.value)}
                  placeholder="e.g. Run 5km daily for 30 days"
                  required
                  autoFocus
                  className="w-full rounded-xl border border-[#777777] bg-[#4A4A4A] px-4 py-3 text-sm text-[#FFFFFF] placeholder:text-[#B5B5B5] outline-none transition focus:border-white focus:ring-2 focus:ring-white/10"
                />
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-semibold tracking-wider text-[#FFFFFF] uppercase">
                  Description <span className="text-[#B5B5B5] font-normal">(Optional)</span>
                </label>
                <textarea
                  value={newDescription}
                  onChange={(e) => setNewDescription(e.target.value)}
                  placeholder="Provide context, metrics, or personal stakes..."
                  rows={3}
                  className="w-full resize-none rounded-xl border border-[#777777] bg-[#4A4A4A] px-4 py-3 text-sm text-[#FFFFFF] placeholder:text-[#B5B5B5] outline-none transition focus:border-white focus:ring-2 focus:ring-white/10"
                />
              </div>

              <div className="mt-6 flex items-center justify-end gap-3 pt-3">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="rounded-xl border border-[#777777] bg-[#292929] px-4 py-2.5 text-sm font-semibold text-[#B5B5B5] transition hover:text-white"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="flex items-center gap-2 rounded-xl bg-white px-5 py-2.5 text-sm font-bold text-[#292929] transition hover:bg-[#B5B5B5] disabled:opacity-50"
                >
                  {submitting ? (
                    <>
                      <Loader2 size={16} className="animate-spin" />
                      Creating...
                    </>
                  ) : (
                    "Create Goal"
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
