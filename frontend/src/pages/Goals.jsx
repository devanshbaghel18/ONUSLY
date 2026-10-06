import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import AppLayout from "../components/AppLayout";
import {
  Target,
  Lock,
  Unlock,
  Users,
  UserCheck,
  Shield,
  Trash2,
  ChevronRight,
  Loader2,
  CheckCircle2,
  AlertCircle,
} from "lucide-react";
import { getGoals, createGoal, updateGoal, deleteGoal } from "../lib/api";
import { getStoredCommunities } from "../lib/communities";
import { getUser } from "../lib/auth";
import { useWebSocket } from "../hooks/useWebSocket";

const APPS_TO_BLOCK = [
  { id: "youtube", label: "YouTube" },
  { id: "twitter", label: "Twitter" },
  { id: "instagram", label: "Instagram" },
  { id: "reddit", label: "Reddit" },
  { id: "tiktok", label: "TikTok" },
  { id: "netflix", label: "Netflix" },
  { id: "linkedin", label: "LinkedIn" },
];

export default function Goals() {
  const currentUser = getUser();
  const [goals, setGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [successNotice, setSuccessNotice] = useState("");

  // Lockout creation form state
  const [taskDescription, setTaskDescription] = useState("");
  const [taskDetails, setTaskDetails] = useState("");
  const [selectedApps, setSelectedApps] = useState([]);

  // Protocol: 'none' (Honor System) | 'community' (Quorum Community) | 'friend' (Dedicated Partner)
  const [protocol, setProtocol] = useState("community");
  const [selectedCommunityName, setSelectedCommunityName] = useState("");
  const [friendEmail, setFriendEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  const communities = getStoredCommunities();

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

  const toggleApp = (appId) => {
    setSelectedApps((prev) =>
      prev.includes(appId)
        ? prev.filter((id) => id !== appId)
        : [...prev, appId]
    );
  };

  const handleStartLock = async (e) => {
    e.preventDefault();
    setFormError("");
    setSuccessNotice("");

    if (!taskDescription.trim()) {
      setFormError("Task description is required");
      return;
    }

    if (protocol === "friend") {
      const email = friendEmail.trim();
      if (!email) {
        setFormError("Please enter your friend's email address");
        return;
      }
      if (!email.includes("@") || !email.includes(".")) {
        setFormError("Please enter a valid email address");
        return;
      }
      if (currentUser?.email && email.toLowerCase() === currentUser.email.toLowerCase()) {
        setFormError("You cannot assign yourself as your accountability partner");
        return;
      }
    }

    setSubmitting(true);
    try {
      const created = await createGoal({
        title: taskDescription.trim(),
        description: taskDetails.trim(),
      });

      const updated = await updateGoal(created.id, {
        approvalType: protocol,
        approverEmail: protocol === "friend" ? friendEmail.trim() : "",
      });

      setGoals((prev) => [updated || created, ...prev]);
      setTaskDescription("");
      setTaskDetails("");
      setFriendEmail("");
      setSuccessNotice("Accountability Lock activated successfully! Sites are locked until verified.");
      setTimeout(() => setSuccessNotice(""), 4000);
    } catch (err) {
      console.error("Failed to create accountability lock:", err);
      setFormError(err.message || "Failed to activate lock");
    } finally {
      setSubmitting(false);
    }
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
              Your Lockout Goals
            </h1>
            <p className="mt-1 text-sm text-[#A3A3A3]">
              Verify and manage active constraints, distraction blocking, and peer approvals.
            </p>
          </div>

          <div className="flex items-center gap-3">
            <span className="rounded-xl border border-[#333333] bg-[#1A1A1A] px-3.5 py-1.5 text-xs font-semibold text-white">
              {goals.length} Total Goals
            </span>
          </div>
        </div>

        {/* Notices */}
        {successNotice && (
          <div className="flex items-center gap-2 rounded-2xl border border-white bg-white/10 p-4 text-xs font-semibold text-white">
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

        {/* 2-Column Split: Lockout Goals List (Left) & New Accountability Lock (Right) */}
        <div className="grid grid-cols-1 gap-8 lg:grid-cols-12">
          {/* Left: Your Goals List */}
          <div className="space-y-4 lg:col-span-7">
            <div className="flex items-center justify-between pb-2">
              <h2 className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
                Active Constraints & Goals ({goals.length})
              </h2>
            </div>

            {loading ? (
              <div className="flex h-64 items-center justify-center rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A]">
                <Loader2 size={24} className="animate-spin text-white" />
              </div>
            ) : goals.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-[#333333] bg-[#1A1A1A]/30 p-12 text-center">
                <Target size={36} className="mx-auto text-[#737373] mb-3" />
                <h3 className="text-base font-bold text-white">No active goals yet</h3>
                <p className="mt-1 text-xs text-[#A3A3A3]">
                  Create your first accountability lock using the panel on the right.
                </p>
              </div>
            ) : (
              <div className="space-y-4">
                {goals.map((g) => {
                  const isCompleted = g.status === "completed";
                  const isProofSubmitted = g.status === "proof_submitted";

                  return (
                    <Link
                      key={g.id}
                      to={`/goals/${g.id}`}
                      className="group relative block rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-5 transition-all hover:border-white"
                    >
                      <div className="flex items-start justify-between gap-4">
                        <div className="space-y-1">
                          <h3 className="text-base font-bold text-white group-hover:text-neutral-200">
                            {g.title}
                          </h3>
                          {g.description && (
                            <p className="text-xs text-[#A3A3A3] line-clamp-2">
                              {g.description}
                            </p>
                          )}
                        </div>

                        {/* Status badge */}
                        <div className="flex flex-col items-end gap-2">
                          {isCompleted ? (
                            <span className="flex items-center gap-1 rounded-full border border-white/20 bg-white/10 px-3 py-0.5 text-[11px] font-bold text-white">
                              <Unlock size={12} />
                              Unlocked
                            </span>
                          ) : isProofSubmitted ? (
                            <span className="flex items-center gap-1 rounded-full border border-[#444444] bg-[#262626] px-3 py-0.5 text-[11px] font-bold text-white">
                              Pending Review
                            </span>
                          ) : (
                            <span className="flex items-center gap-1 rounded-full border border-[#444444] bg-[#222222] px-3 py-0.5 text-[11px] font-bold text-[#D4D4D4]">
                              <Lock size={12} />
                              Active Lock
                            </span>
                          )}

                          <button
                            type="button"
                            onClick={(e) => handleDelete(g.id, e)}
                            className="rounded-lg p-1 text-[#737373] opacity-0 transition-all hover:text-white group-hover:opacity-100"
                            title="Delete Goal"
                          >
                            <Trash2 size={14} />
                          </button>
                        </div>
                      </div>

                      {/* Verification protocol indicator */}
                      <div className="mt-4 flex items-center justify-between border-t border-[#2B2B2B] pt-3 text-[11px]">
                        <div className="flex items-center gap-2">
                          {g.approvalType === "community" ? (
                            <span className="flex items-center gap-1 text-white">
                              <Users size={13} />
                              Community
                            </span>
                          ) : g.approvalType === "friend" ? (
                            <span className="flex items-center gap-1 text-white">
                              <UserCheck size={13} />
                              Friend: {g.approverEmail || "Assigned"}
                            </span>
                          ) : (
                            <span className="flex items-center gap-1 text-[#737373]">
                              <Shield size={13} />
                              Self-Honor
                            </span>
                          )}
                        </div>

                        <span className="flex items-center gap-1 font-semibold text-[#A3A3A3] group-hover:text-white">
                          Configure / Details
                          <ChevronRight size={13} />
                        </span>
                      </div>
                    </Link>
                  );
                })}
              </div>
            )}
          </div>

          {/* Right: New Accountability Lock Panel */}
          <div className="lg:col-span-5">
            <div className="sticky top-20 rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 shadow-xl sm:p-7">
              <div className="border-b border-[#2A2A2A] pb-4">
                <h3 className="text-base font-bold text-white">
                  New Accountability Lock
                </h3>
                <p className="mt-0.5 text-xs text-[#A3A3A3]">
                  Enforce strict focus by tying app blockers to peer verification.
                </p>
              </div>

              {formError && (
                <div className="mt-4 rounded-xl border border-[#444444] bg-[#242424] p-3 text-xs text-white">
                  {formError}
                </div>
              )}

              <form onSubmit={handleStartLock} className="mt-5 space-y-5">
                {/* Task Description */}
                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wider text-[#A3A3A3] mb-1.5">
                    Describe your task
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="e.g. Finish building relational database schemas..."
                    value={taskDescription}
                    onChange={(e) => setTaskDescription(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#121212] px-4 py-3 text-sm text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                {/* Additional task details */}
                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wider text-[#A3A3A3] mb-1.5">
                    Task Notes / Proof Criteria
                  </label>
                  <textarea
                    rows={2}
                    placeholder="Proof expectations or commit link targets..."
                    value={taskDetails}
                    onChange={(e) => setTaskDetails(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#121212] px-4 py-2 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                {/* Select distracting apps to block */}
                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wider text-[#A3A3A3] mb-2">
                    Select distracting apps to block
                  </label>
                  <div className="flex flex-wrap gap-2">
                    {APPS_TO_BLOCK.map((app) => {
                      const isSelected = selectedApps.includes(app.id);
                      return (
                        <button
                          key={app.id}
                          type="button"
                          onClick={() => toggleApp(app.id)}
                          className={`rounded-xl border px-3.5 py-1.5 text-xs font-medium transition-all ${
                            isSelected
                              ? "border-white bg-white text-black font-bold"
                              : "border-[#333333] bg-[#121212] text-[#A3A3A3] hover:text-white"
                          }`}
                        >
                          {app.label}
                        </button>
                      );
                    })}
                  </div>
                </div>

                {/* Choose verification protocol */}
                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wider text-[#A3A3A3] mb-2">
                    Choose verification protocol
                  </label>
                  <div className="space-y-2">
                    {/* 1. Honor System */}
                    <div
                      onClick={() => setProtocol("none")}
                      className={`cursor-pointer rounded-xl border p-3.5 transition-all ${
                        protocol === "none"
                          ? "border-white bg-[#262626]"
                          : "border-[#333333] bg-[#121212] hover:border-[#555555]"
                      }`}
                    >
                      <span className="text-xs font-bold text-white">
                        Honor System (Self)
                      </span>
                      <p className="mt-1 text-[11px] text-[#A3A3A3]">
                        Release constraint yourself when done.
                      </p>
                    </div>

                    {/* 2. Community */}
                    <div
                      onClick={() => setProtocol("community")}
                      className={`cursor-pointer rounded-xl border p-3.5 transition-all ${
                        protocol === "community"
                          ? "border-white bg-[#262626]"
                          : "border-[#333333] bg-[#121212] hover:border-[#555555]"
                      }`}
                    >
                      <div className="flex items-center gap-1.5">
                        <Users size={14} className="text-white" />
                        <span className="text-xs font-bold text-white">
                          Community Group
                        </span>
                      </div>
                      <p className="mt-1 text-[11px] text-[#A3A3A3]">
                        Release requires member approvals from friends in your community.
                      </p>

                      {protocol === "community" && (
                        <div className="mt-3 pt-3 border-t border-[#333333]">
                          {communities.length > 0 ? (
                            <select
                              value={selectedCommunityName}
                              onChange={(e) => setSelectedCommunityName(e.target.value)}
                              className="w-full rounded-lg border border-[#444444] bg-[#1A1A1A] px-3 py-2 text-xs text-white focus:outline-none"
                            >
                              <option value="">Select a community...</option>
                              {communities.map((c) => (
                                <option key={c.id} value={c.name}>
                                  {c.name} ({c.membersCount} members)
                                </option>
                              ))}
                            </select>
                          ) : (
                            <div className="rounded-lg bg-[#1E1E1E] p-2 text-[11px] text-[#A3A3A3]">
                              No communities created yet.{" "}
                              <Link to="/community-friends" className="text-white font-semibold underline">
                                Create a community
                              </Link>{" "}
                              to assign your goal.
                            </div>
                          )}
                        </div>
                      )}
                    </div>

                    {/* 3. Dedicated Partner */}
                    <div
                      onClick={() => setProtocol("friend")}
                      className={`cursor-pointer rounded-xl border p-3.5 transition-all ${
                        protocol === "friend"
                          ? "border-white bg-[#262626]"
                          : "border-[#333333] bg-[#121212] hover:border-[#555555]"
                      }`}
                    >
                      <div className="flex items-center gap-1.5">
                        <UserCheck size={14} className="text-white" />
                        <span className="text-xs font-bold text-white">
                          Dedicated Partner (Friend)
                        </span>
                      </div>
                      <p className="mt-1 text-[11px] text-[#A3A3A3]">
                        A specific friend or partner must review and approve your upload.
                      </p>

                      {protocol === "friend" && (
                        <div className="mt-3 pt-3 border-t border-[#333333]">
                          <input
                            type="email"
                            required={protocol === "friend"}
                            placeholder="e.g. partner@example.com"
                            value={friendEmail}
                            onChange={(e) => setFriendEmail(e.target.value)}
                            className="w-full rounded-lg border border-[#444444] bg-[#1A1A1A] px-3 py-2 text-xs text-white placeholder-[#737373] focus:outline-none"
                          />
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                {/* Submit button */}
                <button
                  type="submit"
                  disabled={submitting}
                  className="w-full flex items-center justify-center gap-2 rounded-xl bg-white py-3.5 text-xs font-bold text-black transition-all hover:bg-neutral-200 disabled:opacity-40"
                >
                  {submitting ? (
                    <>
                      <Loader2 size={16} className="animate-spin" />
                      Activating Lock...
                    </>
                  ) : (
                    <>
                      <Lock size={15} />
                      Start Accountability Lock
                    </>
                  )}
                </button>
              </form>
            </div>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
