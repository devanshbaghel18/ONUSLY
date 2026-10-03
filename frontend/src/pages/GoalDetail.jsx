import { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import {
  ArrowLeft,
  Target,
  Users,
  UserCheck,
  Edit3,
  Check,
  X,
  AlertCircle,
  CheckCircle2,
  Calendar,
  Clock,
  Trash2,
  Loader2,
  ShieldCheck,
  Sparkles,
  Mail,
  Send,
  HelpCircle,
} from "lucide-react";
import { getUser, clearAuth } from "../lib/auth";
import { getGoal, updateGoal, deleteGoal } from "../lib/api";

import AppLayout from "../components/AppLayout";

export default function GoalDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [user] = useState(() => getUser());

  const [goal, setGoal] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [successNotice, setSuccessNotice] = useState("");

  // Edit Goal Title/Description state
  const [isEditingGoal, setIsEditingGoal] = useState(false);
  const [editTitle, setEditTitle] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [savingGoal, setSavingGoal] = useState(false);
  const [goalFormError, setGoalFormError] = useState("");

  // Accountability Option State
  // "community" | "friend" | "none"
  const [selectedApprovalType, setSelectedApprovalType] = useState("none");
  const [friendEmail, setFriendEmail] = useState("");
  const [savingAccountability, setSavingAccountability] = useState(false);
  const [accountabilityNotice, setAccountabilityNotice] = useState("");
  const [accountabilityError, setAccountabilityError] = useState("");

  // Load Goal
  useEffect(() => {
    let ignore = false;

    setLoading(true);
    setError("");

    getGoal(id)
      .then((data) => {
        if (!ignore && data) {
          setGoal(data);
          setEditTitle(data.title || "");
          setEditDescription(data.description || "");
          setSelectedApprovalType(data.approvalType || "none");
          setFriendEmail(data.approverEmail || "");
        }
      })
      .catch((err) => {
        if (!ignore) {
          console.error("Failed to load goal details:", err);
          setError(err.message || "Failed to load goal");
        }
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });

    return () => {
      ignore = true;
    };
  }, [id]);

  // Handle Save Goal Edit (Title / Description)
  const handleSaveGoalInfo = async (e) => {
    e.preventDefault();
    if (!editTitle.trim()) {
      setGoalFormError("Title cannot be empty");
      return;
    }

    setSavingGoal(true);
    setGoalFormError("");
    try {
      const updated = await updateGoal(id, {
        title: editTitle,
        description: editDescription,
      });

      setGoal(updated);
      setIsEditingGoal(false);
      setSuccessNotice("Goal updated successfully!");
      setTimeout(() => setSuccessNotice(""), 3500);
    } catch (err) {
      console.error("Failed to update goal:", err);
      setGoalFormError(err.message || "Failed to update goal");
    } finally {
      setSavingGoal(false);
    }
  };

  // Handle Cancel Goal Edit
  const handleCancelGoalEdit = () => {
    if (goal) {
      setEditTitle(goal.title || "");
      setEditDescription(goal.description || "");
    }
    setGoalFormError("");
    setIsEditingGoal(false);
  };

  // Handle Save Accountability Method
  const handleSaveAccountability = async () => {
    setAccountabilityError("");
    setAccountabilityNotice("");

    if (selectedApprovalType === "friend") {
      const trimmedEmail = friendEmail.trim();
      if (!trimmedEmail) {
        setAccountabilityError("Please enter your friend's email address");
        return;
      }
      if (!trimmedEmail.includes("@") || !trimmedEmail.includes(".")) {
        setAccountabilityError("Please enter a valid email address");
        return;
      }
      if (user?.email && trimmedEmail.toLowerCase() === user.email.toLowerCase()) {
        setAccountabilityError("You cannot assign yourself as your accountability partner");
        return;
      }
    }

    setSavingAccountability(true);
    try {
      const updated = await updateGoal(id, {
        approvalType: selectedApprovalType,
        approverEmail: selectedApprovalType === "friend" ? friendEmail : "",
      });

      setGoal(updated);
      setSelectedApprovalType(updated.approvalType || "none");
      setFriendEmail(updated.approverEmail || "");
      setAccountabilityNotice("Accountability setting saved successfully!");
      setTimeout(() => setAccountabilityNotice(""), 4000);
    } catch (err) {
      console.error("Failed to update accountability:", err);
      setAccountabilityError(err.message || "Failed to update accountability setting");
    } finally {
      setSavingAccountability(false);
    }
  };

  // Handle Delete Goal
  const handleDelete = async () => {
    if (!window.confirm("Are you sure you want to permanently delete this goal?")) {
      return;
    }

    try {
      await deleteGoal(id);
      navigate("/goals");
    } catch (err) {
      console.error("Failed to delete goal:", err);
      setError(err.message || "Failed to delete goal");
    }
  };

  const formatDate = (dateString) => {
    if (!dateString) return "N/A";
    try {
      const date = new Date(dateString);
      return date.toLocaleDateString("en-US", {
        month: "short",
        day: "numeric",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return dateString;
    }
  };

  return (
    <AppLayout>
      <div className="mx-auto max-w-5xl space-y-6">
        {/* Navigation Bar back button */}
        <div className="flex items-center justify-between border-b border-[#777777]/30 pb-4">
          <Link
            to="/goals"
            className="group flex items-center gap-2 rounded-xl border border-[#777777] bg-[#3A3A3A] px-3.5 py-2 text-xs font-semibold text-[#B5B5B5] transition hover:border-white hover:text-white"
          >
            <ArrowLeft size={15} className="transition-transform group-hover:-translate-x-0.5" />
            <span>Back to Goals</span>
          </Link>
          <span className="text-xs font-semibold text-[#8E8E8E]">Goal ID: {id}</span>
        </div>
          {/* Notifications */}
          {successNotice && (
            <div className="mb-6 flex items-center gap-3 rounded-2xl border border-emerald-500/40 bg-emerald-950/30 px-5 py-3.5 text-sm text-emerald-300 shadow-lg">
              <CheckCircle2 size={18} className="shrink-0" />
              <span>{successNotice}</span>
            </div>
          )}

          {error && (
            <div className="mb-6 flex items-center justify-between rounded-2xl border border-rose-500/40 bg-rose-950/30 px-5 py-3.5 text-sm text-rose-300 shadow-lg">
              <div className="flex items-center gap-3">
                <AlertCircle size={18} className="shrink-0" />
                <span>{error}</span>
              </div>
              <Link to="/dashboard" className="underline hover:text-white">
                Back to Dashboard
              </Link>
            </div>
          )}

          {loading ? (
            <div className="space-y-6 py-12">
              <div className="h-48 animate-pulse rounded-[28px] border border-[#777777]/30 bg-[#3A3A3A]/40" />
              <div className="h-64 animate-pulse rounded-[28px] border border-[#777777]/30 bg-[#3A3A3A]/40" />
            </div>
          ) : !goal ? (
            <div className="flex flex-col items-center justify-center rounded-[28px] border border-dashed border-[#777777] bg-[#3A3A3A]/40 px-6 py-20 text-center">
              <Target size={40} className="text-[#B5B5B5] mb-4" />
              <h2 className="text-xl font-bold text-white">Goal not found</h2>
              <p className="mt-2 text-sm text-[#B5B5B5]">The goal you are looking for may have been deleted.</p>
              <Link
                to="/dashboard"
                className="mt-6 rounded-xl bg-white px-5 py-2.5 text-sm font-bold text-[#292929] transition hover:bg-[#B5B5B5]"
              >
                Return to Dashboard
              </Link>
            </div>
          ) : (
            <div className="space-y-8">
              {/* =====================================================
                  1. GOAL OVERVIEW & EDIT CARD
              ===================================================== */}
              <div className="relative overflow-hidden rounded-[28px] border border-[#777777]/70 bg-[#3A3A3A] p-6 shadow-[0_20px_50px_rgba(0,0,0,0.4)] sm:p-8">
                {/* Status & Action Bar */}
                <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#777777]/30 pb-5">
                  <div className="flex items-center gap-3">
                    <span className="flex items-center gap-1.5 rounded-full border border-emerald-500/40 bg-emerald-500/10 px-3 py-1 text-xs font-semibold text-emerald-300 uppercase tracking-wider">
                      <span className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
                      {goal.status || "active"}
                    </span>

                    <span className="hidden sm:inline-flex items-center gap-1.5 text-xs text-[#B5B5B5]">
                      <Calendar size={13} />
                      Created {formatDate(goal.createdAt)}
                    </span>
                  </div>

                  <div className="flex items-center gap-2">
                    {!isEditingGoal && (
                      <button
                        type="button"
                        onClick={() => setIsEditingGoal(true)}
                        className="flex items-center gap-2 rounded-xl border border-[#777777] bg-[#292929] px-3.5 py-2 text-xs font-semibold text-white transition hover:border-white hover:bg-[#4A4A4A]"
                      >
                        <Edit3 size={14} className="text-[#B5B5B5]" />
                        <span>Edit Goal</span>
                      </button>
                    )}

                    <button
                      type="button"
                      onClick={handleDelete}
                      className="flex items-center gap-2 rounded-xl border border-rose-500/30 bg-[#292929] px-3 py-2 text-xs font-semibold text-rose-400 transition hover:bg-rose-500/20 hover:border-rose-400"
                      title="Delete Goal"
                    >
                      <Trash2 size={14} />
                      <span className="hidden sm:inline">Delete</span>
                    </button>
                  </div>
                </div>

                {/* View / Edit Mode */}
                {!isEditingGoal ? (
                  <div className="pt-6">
                    <h1 className="text-2xl font-bold tracking-tight text-[#FFFFFF] sm:text-3xl">
                      {goal.title}
                    </h1>

                    <div className="mt-4 rounded-2xl border border-[#777777]/40 bg-[#292929]/60 p-5">
                      <h4 className="text-[11px] font-semibold uppercase tracking-wider text-[#B5B5B5] mb-2">
                        Description & Context
                      </h4>
                      {goal.description ? (
                        <p className="whitespace-pre-wrap text-sm leading-relaxed text-[#E0E0E0]">
                          {goal.description}
                        </p>
                      ) : (
                        <p className="text-sm italic text-[#777777]">
                          No description provided. Click "Edit Goal" to add context, stakes, or metrics.
                        </p>
                      )}
                    </div>

                    {goal.updatedAt && (
                      <div className="mt-4 flex items-center gap-2 text-[11px] text-[#777777]">
                        <Clock size={12} />
                        <span>Last modified {formatDate(goal.updatedAt)}</span>
                      </div>
                    )}
                  </div>
                ) : (
                  /* Edit Form */
                  <form onSubmit={handleSaveGoalInfo} className="space-y-5 pt-6">
                    {goalFormError && (
                      <div className="rounded-xl border border-rose-500/40 bg-rose-950/30 px-4 py-2.5 text-xs text-rose-300">
                        {goalFormError}
                      </div>
                    )}

                    <div>
                      <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#FFFFFF]">
                        Goal Title <span className="text-rose-400">*</span>
                      </label>
                      <input
                        type="text"
                        value={editTitle}
                        onChange={(e) => setEditTitle(e.target.value)}
                        placeholder="e.g. Master Go Single-Table DynamoDB"
                        required
                        className="w-full rounded-xl border border-[#777777] bg-[#4A4A4A] px-4 py-3 text-sm text-[#FFFFFF] placeholder:text-[#B5B5B5] outline-none transition focus:border-white focus:ring-2 focus:ring-white/10"
                      />
                    </div>

                    <div>
                      <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#FFFFFF]">
                        Description & Stakes
                      </label>
                      <textarea
                        value={editDescription}
                        onChange={(e) => setEditDescription(e.target.value)}
                        placeholder="Provide the purpose, milestones, or consequences..."
                        rows={4}
                        className="w-full resize-none rounded-xl border border-[#777777] bg-[#4A4A4A] px-4 py-3 text-sm text-[#FFFFFF] placeholder:text-[#B5B5B5] outline-none transition focus:border-white focus:ring-2 focus:ring-white/10"
                      />
                    </div>

                    <div className="flex items-center justify-end gap-3 pt-2">
                      <button
                        type="button"
                        onClick={handleCancelGoalEdit}
                        className="rounded-xl border border-[#777777] bg-[#292929] px-4 py-2 text-xs font-semibold text-[#B5B5B5] transition hover:text-white"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        disabled={savingGoal}
                        className="flex items-center gap-2 rounded-xl bg-white px-5 py-2 text-xs font-bold text-[#292929] transition hover:bg-[#B5B5B5] disabled:opacity-50"
                      >
                        {savingGoal ? (
                          <>
                            <Loader2 size={14} className="animate-spin" />
                            Saving...
                          </>
                        ) : (
                          <>
                            <Check size={14} />
                            Save Changes
                          </>
                        )}
                      </button>
                    </div>
                  </form>
                )}
              </div>

              {/* =====================================================
                  2. ACCOUNTABILITY & VERIFICATION STRATEGY
              ===================================================== */}
              <div className="rounded-[28px] border border-[#777777]/70 bg-[#3A3A3A] p-6 shadow-[0_20px_50px_rgba(0,0,0,0.4)] sm:p-8">
                <div className="mb-6 flex flex-col justify-between gap-2 sm:flex-row sm:items-center">
                  <div>
                    <div className="flex items-center gap-2.5">
                      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
                        <ShieldCheck size={18} />
                      </div>
                      <h2 className="text-xl font-bold tracking-tight text-white">
                        Accountability & Approval Method
                      </h2>
                    </div>
                    <p className="mt-1 text-xs text-[#B5B5B5]">
                      Choose who must verify your proof submissions before this goal is marked complete.
                    </p>
                  </div>

                  {goal.approvalType && (
                    <span className="self-start sm:self-auto rounded-full border border-blue-500/30 bg-blue-500/10 px-3 py-1 text-xs font-medium text-blue-300">
                      Current:{" "}
                      <strong className="capitalize text-white">
                        {goal.approvalType === "community" ? "Community Verification" : "Friend Partner"}
                      </strong>
                    </span>
                  )}
                </div>

                {accountabilityNotice && (
                  <div className="mb-6 flex items-center gap-3 rounded-xl border border-emerald-500/40 bg-emerald-950/30 px-4 py-3 text-xs text-emerald-300">
                    <CheckCircle2 size={16} className="shrink-0" />
                    <span>{accountabilityNotice}</span>
                  </div>
                )}

                {accountabilityError && (
                  <div className="mb-6 flex items-center gap-3 rounded-xl border border-rose-500/40 bg-rose-950/30 px-4 py-3 text-xs text-rose-300">
                    <AlertCircle size={16} className="shrink-0" />
                    <span>{accountabilityError}</span>
                  </div>
                )}

                {/* Option Cards Grid */}
                <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
                  {/* OPTION 1: COMMUNITY */}
                  <div
                    onClick={() => setSelectedApprovalType("community")}
                    className={`group relative flex cursor-pointer flex-col justify-between rounded-2xl border p-6 transition-all duration-200 ${
                      selectedApprovalType === "community"
                        ? "border-white bg-[#262626] shadow-sm ring-1 ring-white/30"
                        : "border-[#333333] bg-[#1A1A1A] hover:border-[#555555]"
                    }`}
                  >
                    <div>
                      <div className="flex items-center justify-between">
                        <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-[#444444] bg-[#222222] text-white">
                          <Users size={22} className="text-white" />
                        </div>
                        <div
                          className={`flex h-6 w-6 items-center justify-center rounded-full border transition-all ${
                            selectedApprovalType === "community"
                              ? "border-white bg-white text-black"
                              : "border-[#444444] bg-transparent"
                          }`}
                        >
                          {selectedApprovalType === "community" && <Check size={14} className="stroke-[3]" />}
                        </div>
                      </div>

                      <h3 className="mt-4 text-base font-bold text-white">
                        Approval via Community
                      </h3>
                      <span className="text-[11px] font-semibold uppercase tracking-wider text-[#A3A3A3]">
                        Decentralized Peer Review
                      </span>

                      <p className="mt-2.5 text-xs leading-relaxed text-[#A3A3A3]">
                        Submit your proof to your community quorum. Verified fellow members review your evidence and reach a collective verdict.
                      </p>
                    </div>
                  </div>

                  {/* OPTION 2: FRIEND / MENTOR */}
                  <div
                    onClick={() => setSelectedApprovalType("friend")}
                    className={`group relative flex cursor-pointer flex-col justify-between rounded-2xl border p-6 transition-all duration-200 ${
                      selectedApprovalType === "friend"
                        ? "border-white bg-[#262626] shadow-sm ring-1 ring-white/30"
                        : "border-[#333333] bg-[#1A1A1A] hover:border-[#555555]"
                    }`}
                  >
                    <div>
                      <div className="flex items-center justify-between">
                        <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-[#444444] bg-[#222222] text-white">
                          <UserCheck size={22} className="text-white" />
                        </div>
                        <div
                          className={`flex h-6 w-6 items-center justify-center rounded-full border transition-all ${
                            selectedApprovalType === "friend"
                              ? "border-white bg-white text-black"
                              : "border-[#444444] bg-transparent"
                          }`}
                        >
                          {selectedApprovalType === "friend" && <Check size={14} className="stroke-[3]" />}
                        </div>
                      </div>

                      <h3 className="mt-4 text-base font-bold text-white">
                        Approval via Friend
                      </h3>
                      <span className="text-[11px] font-semibold uppercase tracking-wider text-[#A3A3A3]">
                        Direct 1-on-1 Accountability Partner
                      </span>

                      <p className="mt-2.5 text-xs leading-relaxed text-[#A3A3A3]">
                        Assign a trusted friend or partner. They have the authority to inspect your proof submissions and decide the outcome.
                      </p>
                    </div>
                  </div>
                </div>

                {/* Friend Email Input (Animated display when "friend" is selected) */}
                {selectedApprovalType === "friend" && (
                  <div className="mt-6 rounded-2xl border border-[#333333] bg-[#1A1A1A] p-5">
                    <label className="mb-2 flex items-center justify-between text-xs font-semibold uppercase tracking-wider text-white">
                      <span className="flex items-center gap-1.5">
                        <Mail size={14} className="text-white" />
                        Accountability Partner's Email <span className="text-neutral-400">*</span>
                      </span>
                      <span className="text-[11px] font-normal lowercase text-[#B5B5B5]">
                        Friend must sign in to ONUSLY to review
                      </span>
                    </label>
                    <input
                      type="email"
                      value={friendEmail}
                      onChange={(e) => setFriendEmail(e.target.value)}
                      placeholder="e.g. partner@example.com"
                      className="w-full rounded-xl border border-[#777777] bg-[#4A4A4A] px-4 py-3 text-sm text-[#FFFFFF] placeholder:text-[#B5B5B5] outline-none transition focus:border-purple-400 focus:ring-2 focus:ring-purple-400/20"
                    />
                    <p className="mt-2 text-[11px] text-[#B5B5B5]">
                      When you submit proof for this goal, your partner will receive the notification to review your evidence.
                    </p>
                  </div>
                )}

                {/* Save Accountability Button */}
                <div className="mt-6 flex flex-wrap items-center justify-between gap-4 border-t border-[#777777]/30 pt-5">
                  <div className="flex items-center gap-2 text-xs text-[#B5B5B5]">
                    <HelpCircle size={14} />
                    <span>You can switch between Community and Friend accountability at any time.</span>
                  </div>

                  <button
                    type="button"
                    onClick={handleSaveAccountability}
                    disabled={savingAccountability}
                    className="flex items-center gap-2 rounded-xl bg-white px-6 py-3 text-xs font-bold text-[#292929] shadow-md transition hover:bg-[#B5B5B5] active:translate-y-0.5 disabled:opacity-50"
                  >
                    {savingAccountability ? (
                      <>
                        <Loader2 size={15} className="animate-spin" />
                        Saving Accountability...
                      </>
                    ) : (
                      <>
                        <ShieldCheck size={15} />
                        Save Accountability Setting
                      </>
                    )}
                  </button>
                </div>
              </div>

              {/* =====================================================
                  3. PROOF & VERIFICATION TIMELINE PREVIEW
              ===================================================== */}
              <div className="rounded-[28px] border border-[#777777]/70 bg-[#3A3A3A] p-6 shadow-[0_20px_50px_rgba(0,0,0,0.4)] sm:p-8">
                <div className="flex items-center justify-between border-b border-[#777777]/30 pb-4">
                  <div className="flex items-center gap-2.5">
                    <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-[#292929] border border-[#777777] text-white">
                      <Send size={16} />
                    </div>
                    <div>
                      <h3 className="text-lg font-bold text-white">Proof & Verification Pipeline</h3>
                      <p className="text-xs text-[#B5B5B5]">
                        Submit daily or milestone proof to fulfill your commitment.
                      </p>
                    </div>
                  </div>
                </div>

                <div className="mt-6 flex flex-col items-center justify-center rounded-2xl border border-dashed border-[#777777]/60 bg-[#292929]/50 px-6 py-10 text-center">
                  <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-[#3A3A3A] text-[#B5B5B5] border border-[#777777]">
                    <Target size={24} />
                  </div>
                  <h4 className="text-sm font-semibold text-white">No proof submitted yet</h4>
                  <p className="mt-1 max-w-sm text-xs text-[#B5B5B5]">
                    Once you make progress, submit your evidence here. It will be verified by your chosen accountability method (
                    <strong className="text-white">
                      {goal.approvalType === "community"
                        ? "Community"
                        : goal.approvalType === "friend"
                        ? `Friend (${goal.approverEmail || "Partner"})`
                        : "Not yet configured"}
                    </strong>
                    ).
                  </p>
                </div>
              </div>
            </div>
          )}
      </div>
    </AppLayout>
  );
}

