import { useState, useEffect } from "react";
import { useParams, useNavigate, useSearchParams, Link } from "react-router-dom";
import {
  ArrowLeft,
  Target,
  Users,
  UserCheck,
  Edit3,
  Check,
  AlertCircle,
  CheckCircle2,
  Calendar,
  Clock,
  Trash2,
  Loader2,
  ShieldCheck,
  Mail,
  Send,
  HelpCircle,
  ExternalLink,
  ThumbsUp,
  ThumbsDown,
  FileText,
  Share2,
  AtSign,
} from "lucide-react";
import { getUser } from "../lib/auth";
import { getGoal, updateGoal, deleteGoal, getProofs, decideApproval } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";

import AppLayout from "../components/AppLayout";

export default function GoalDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const ownerIdParam = searchParams.get("ownerId") || "";
  const [user] = useState(() => getUser());

  const [goal, setGoal] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [successNotice, setSuccessNotice] = useState("");
  const [unlockBanner, setUnlockBanner] = useState(null);

  // Subscribe to real-time WebSocket events for instant unlock sync
  const { isConnected } = useWebSocket((event) => {
    if (event?.type === "goal.unlocked") {
      const payload = event.payload || {};
      if (String(payload.goalId) === String(id)) {
        console.log("[GoalDetail] Real-time goal unlock received:", payload);

        // 1. Instantly flip the goal status to "completed" in local state
        setGoal((prev) => (prev ? { ...prev, status: "completed" } : prev));

        // 2. Mark any pending proofs as approved
        setProofs((prev) =>
          prev.map((p) =>
            p.status === "pending" || p.status === "proof_submitted" ? { ...p, status: "approved" } : p
          )
        );

        // 3. Display celebratory real-time unlock banner
        setUnlockBanner({
          goalId: payload.goalId,
          title: payload.title || goal?.title || "Your Goal",
          time: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
        });

        // 4. Background re-fetch to ensure full synchronization
        getGoal(id, ownerIdParam)
          .then((fresh) => {
            if (fresh) setGoal(fresh);
          })
          .catch(() => {});
        getProofs(id, ownerIdParam)
          .then((fresh) => {
            if (fresh) setProofs(fresh);
          })
          .catch(() => {});
      }
    }
  });

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
  const [copiedReviewLink, setCopiedReviewLink] = useState(false);

  // Proofs & Decision State
  const [proofs, setProofs] = useState([]);
  const [loadingProofs, setLoadingProofs] = useState(true);
  const [decidingProofId, setDecidingProofId] = useState(null);
  const [decisionComment, setDecisionComment] = useState("");
  const [decisionError, setDecisionError] = useState("");
  const [decisionSuccess, setDecisionSuccess] = useState("");

  // Load Goal & Proofs
  useEffect(() => {
    let ignore = false;

    getGoal(id, ownerIdParam)
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

    getProofs(id, ownerIdParam)
      .then((data) => {
        if (!ignore) setProofs(data);
      })
      .catch((err) => {
        console.error("Failed to load proofs:", err);
      })
      .finally(() => {
        if (!ignore) setLoadingProofs(false);
      });

    return () => {
      ignore = true;
    };
  }, [id, ownerIdParam]);

  // Handle Approver Decision (Approve or Reject)
  const handleDecideApproval = async (proofId, status) => {
    if (!goal) return;
    setDecisionError("");
    setDecisionSuccess("");
    setDecidingProofId(proofId);

    try {
      await decideApproval(id, proofId, {
        ownerId: goal.ownerId,
        status,
        comment: decisionComment,
      });

      setDecisionSuccess(
        status === "approved"
          ? "🎉 Proof approved successfully! The goal is unlocked and constraints released."
          : "Proof rejected. The goal has returned to active status."
      );

      // Refresh goal and proofs to reflect new status
      const updatedGoal = await getGoal(id, ownerIdParam || goal.ownerId);
      setGoal(updatedGoal);
      const updatedProofs = await getProofs(id, ownerIdParam || goal.ownerId);
      setProofs(updatedProofs);
      setDecisionComment("");
    } catch (err) {
      console.error("Failed to decide approval:", err);
      setDecisionError(err.message || "Failed to submit decision");
    } finally {
      setDecidingProofId(null);
    }
  };

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
      const trimmedVal = friendEmail.trim();
      if (!trimmedVal) {
        setAccountabilityError("Please enter your partner's ONUSLY handle (e.g. @robert_01)");
        return;
      }
      const cleanHandle = trimmedVal.replace(/^@/, "").toLowerCase();
      if (cleanHandle.length < 3) {
        setAccountabilityError("Handle must be at least 3 characters");
        return;
      }
      if (
        (user?.handle && cleanHandle === user.handle.toLowerCase().replace(/^@/, "")) ||
        (user?.email && trimmedVal.toLowerCase() === user.email.toLowerCase())
      ) {
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
          <div className="flex items-center gap-2.5">
            {goal?.ownerId && (
              <button
                type="button"
                onClick={() => {
                  const link = `${window.location.origin}/goals/${id}?ownerId=${encodeURIComponent(goal.ownerId)}`;
                  navigator.clipboard.writeText(link);
                  setCopiedReviewLink(true);
                  setTimeout(() => setCopiedReviewLink(false), 2500);
                }}
                className="flex items-center gap-1.5 rounded-xl border border-[#777777] bg-[#292929] px-3 py-1 text-[11px] font-semibold text-white hover:border-white transition"
                title="Copy direct verification link for your assigned approver"
              >
                <Share2 size={12} className="text-emerald-400" />
                <span>{copiedReviewLink ? "Link Copied! ✓" : "Share with Approver"}</span>
              </button>
            )}
            <span
              className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium transition ${
                isConnected
                  ? "border border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
                  : "border border-neutral-700 bg-neutral-800 text-neutral-400"
              }`}
              title={isConnected ? "Real-Time WebSocket Connected" : "Connecting to real-time gateway..."}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${isConnected ? "bg-emerald-400 animate-pulse" : "bg-neutral-500"}`} />
              <span>{isConnected ? "Live Sync Active" : "Connecting..."}</span>
            </span>
            <span className="text-xs font-semibold text-[#8E8E8E]">Goal ID: {id}</span>
          </div>
        </div>

        {/* Real-time Unlock Celebration Banner */}
        {unlockBanner && (
          <div className="relative mb-6 overflow-hidden rounded-2xl border border-emerald-500/50 bg-gradient-to-r from-emerald-950/80 via-[#22382e] to-emerald-950/80 p-5 text-white shadow-[0_10px_40px_rgba(16,185,129,0.25)] animate-in fade-in slide-in-from-top-4 duration-500">
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-3.5">
                <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-emerald-500/20 border border-emerald-400/40 text-emerald-300">
                  <ShieldCheck size={24} />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-bold uppercase tracking-wider text-emerald-400">
                      Goal Completed & Unlocked Live!
                    </span>
                    <span className="text-[10px] text-emerald-300/70">{unlockBanner.time}</span>
                  </div>
                  <p className="mt-0.5 text-sm font-semibold text-white">
                    "{unlockBanner.title}" has been reviewed, approved, and unlocked in real time!
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setUnlockBanner(null)}
                className="rounded-lg p-1 text-emerald-400 hover:bg-emerald-900/50 hover:text-white transition"
              >
                ✕
              </button>
            </div>
          </div>
        )}
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
                    <span className="self-start sm:self-auto rounded-full border border-neutral-700 bg-neutral-800 px-3 py-1 text-xs font-medium text-neutral-300">
                      Current:{" "}
                      <strong className="text-white">
                        {goal.approvalType === "community"
                          ? "Community Quorum"
                          : `Friend: ${goal.approverEmail ? (goal.approverEmail.startsWith("@") ? goal.approverEmail : `@${goal.approverEmail}`) : "Assigned Partner"}`}
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

                {/* Friend Handle / Tag Input (Animated display when "friend" is selected) */}
                {selectedApprovalType === "friend" && (
                  <div className="mt-6 rounded-2xl border border-[#333333] bg-[#1A1A1A] p-5">
                    <label className="mb-2 flex items-center justify-between text-xs font-semibold uppercase tracking-wider text-white">
                      <span className="flex items-center gap-1.5">
                        <AtSign size={14} className="text-white" />
                        Partner's ONUSLY Tag / Handle <span className="text-neutral-400">*</span>
                      </span>
                      <span className="text-[11px] font-normal lowercase text-[#A3A3A3]">
                        Enter @handle (100% Privacy Shield)
                      </span>
                    </label>
                    <div className="relative">
                      <span className="absolute left-3.5 top-3 font-mono text-sm text-[#737373]">
                        @
                      </span>
                      <input
                        type="text"
                        value={friendEmail.replace(/^@/, "")}
                        onChange={(e) => setFriendEmail(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ""))}
                        placeholder="e.g. robert_01"
                        className="w-full rounded-xl border border-[#333333] bg-[#121212] pl-8 pr-4 py-3 font-mono text-sm text-white placeholder-[#737373] outline-none transition focus:border-white focus:ring-1 focus:ring-white/20"
                      />
                    </div>
                    <p className="mt-2 text-[11px] text-[#A3A3A3]">
                      Enter your partner's public handle. Your personal Google email and their email stay 100% private.
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
                <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#777777]/30 pb-4">
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

                  {goal?.approverEmail && (
                    <button
                      type="button"
                      onClick={() => {
                        const link = `${window.location.origin}/goals/${id}${goal?.ownerId ? `?ownerId=${goal.ownerId}` : ""}`;
                        navigator.clipboard.writeText(link);
                        setCopiedReviewLink(true);
                        setTimeout(() => setCopiedReviewLink(false), 2500);
                      }}
                      className="inline-flex items-center gap-1.5 rounded-xl border border-[#777777] bg-[#292929] px-3.5 py-2 text-xs font-semibold text-white hover:bg-[#3A3A3A] transition shadow-sm"
                    >
                      {copiedReviewLink ? (
                        <>
                          <Check size={14} className="text-emerald-400" />
                          <span className="text-emerald-400">Review Link Copied!</span>
                        </>
                      ) : (
                        <>
                          <ExternalLink size={14} />
                          <span>Share Review Link</span>
                        </>
                      )}
                    </button>
                  )}
                </div>

                {decisionSuccess && (
                  <div className="mt-4 flex items-center gap-3 rounded-xl border border-emerald-500/40 bg-emerald-950/30 px-4 py-3 text-xs text-emerald-300">
                    <CheckCircle2 size={16} className="shrink-0" />
                    <span>{decisionSuccess}</span>
                  </div>
                )}

                {decisionError && (
                  <div className="mt-4 flex items-center gap-3 rounded-xl border border-rose-500/40 bg-rose-950/30 px-4 py-3 text-xs text-rose-300">
                    <AlertCircle size={16} className="shrink-0" />
                    <span>{decisionError}</span>
                  </div>
                )}

                {loadingProofs ? (
                  <div className="mt-6 flex h-32 items-center justify-center rounded-2xl border border-[#777777]/30 bg-[#292929]/40">
                    <Loader2 size={20} className="animate-spin text-white" />
                  </div>
                ) : proofs.length === 0 ? (
                  <div className="mt-6 flex flex-col items-center justify-center rounded-2xl border border-dashed border-[#777777]/60 bg-[#292929]/50 px-6 py-10 text-center">
                    <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-[#3A3A3A] text-[#B5B5B5] border border-[#777777]">
                      <Target size={24} />
                    </div>
                    <h4 className="text-sm font-semibold text-white">No proof submitted yet</h4>
                    <p className="mt-1 max-w-sm text-xs text-[#B5B5B5]">
                      Once progress is made, submit evidence to release constraints. Verification is assigned to{" "}
                      <strong className="text-white">
                        {goal.approvalType === "community"
                          ? "Community Quorum"
                          : goal.approvalType === "friend"
                          ? `Friend Partner (${goal.approverEmail || "Partner"})`
                          : "Honor System"}
                      </strong>.
                    </p>
                    {goal.status === "active" && (
                      <Link
                        to="/submit-proof"
                        className="mt-4 inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-[#292929] hover:bg-[#B5B5B5] transition"
                      >
                        <Send size={13} />
                        Submit Verification Evidence
                      </Link>
                    )}
                  </div>
                ) : (
                  <div className="mt-6 space-y-4">
                    {proofs.map((p, idx) => {
                      const isApproverUser =
                        Boolean(user?.email && goal?.approverEmail && user.email.toLowerCase() === goal.approverEmail.toLowerCase()) ||
                        Boolean(user?.id && goal?.approverId && user.id === goal.approverId);
                      const canDecide = isApproverUser && goal.status === "proof_submitted";
                      const isDeciding = decidingProofId === p.id;

                      return (
                        <div
                          key={p.id}
                          className="rounded-2xl border border-[#777777]/50 bg-[#292929] p-5 shadow-sm space-y-4"
                        >
                          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[#777777]/20 pb-3">
                            <div className="flex items-center gap-2">
                              <span className="rounded-lg bg-[#3A3A3A] px-2.5 py-1 text-[11px] font-bold text-white uppercase tracking-wider">
                                Proof #{proofs.length - idx}
                              </span>
                              <span className="rounded-md border border-neutral-600 bg-neutral-800 px-2 py-0.5 text-[10px] font-semibold text-neutral-300 uppercase">
                                {p.proofType || "evidence"}
                              </span>
                            </div>
                            <span className="text-[11px] text-[#A3A3A3]">
                              Submitted {formatDate(p.submittedAt)}
                            </span>
                          </div>

                          {p.textExplanation && (
                            <div className="rounded-xl border border-[#444444] bg-[#1E1E1E] p-3.5">
                              <span className="text-[10px] font-semibold uppercase tracking-wider text-[#A3A3A3] block mb-1">
                                Explanation
                              </span>
                              <p className="text-xs text-white leading-relaxed whitespace-pre-wrap">
                                {p.textExplanation}
                              </p>
                            </div>
                          )}

                          {p.externalLink && (
                            <div className="flex items-center gap-2 text-xs">
                              <ExternalLink size={14} className="text-blue-400 shrink-0" />
                              <span className="text-[#A3A3A3]">Verification Link:</span>
                              <a
                                href={p.externalLink.startsWith("http") ? p.externalLink : `https://${p.externalLink}`}
                                target="_blank"
                                rel="noreferrer"
                                className="text-blue-400 underline hover:text-blue-300 truncate"
                              >
                                {p.externalLink}
                              </a>
                            </div>
                          )}

                          {p.photoUrl && (
                            <div className="flex items-center gap-2 text-xs text-[#A3A3A3]">
                              <FileText size={14} className="text-purple-400 shrink-0" />
                              <span>Attached: <strong className="text-white">{p.photoUrl}</strong></span>
                            </div>
                          )}

                          {/* Approver Action Panel */}
                          {canDecide ? (
                            <div className="mt-4 rounded-xl border border-white/20 bg-gradient-to-b from-neutral-900 to-[#1F1F1F] p-4 space-y-3">
                              <div className="flex items-center justify-between">
                                <span className="text-xs font-bold text-white flex items-center gap-1.5">
                                  <ShieldCheck size={15} className="text-emerald-400" />
                                  Accountability Partner Decision
                                </span>
                                <span className="text-[10px] text-amber-300 bg-amber-950/60 border border-amber-500/30 px-2 py-0.5 rounded-full">
                                  Action Required
                                </span>
                              </div>

                              <input
                                type="text"
                                value={decisionComment}
                                onChange={(e) => setDecisionComment(e.target.value)}
                                placeholder="Optional feedback or congratulatory comment..."
                                className="w-full rounded-xl border border-[#555555] bg-[#2A2A2A] px-3.5 py-2 text-xs text-white placeholder:text-neutral-400 outline-none focus:border-white"
                              />

                              <div className="flex flex-wrap items-center gap-3 pt-1">
                                <button
                                  type="button"
                                  disabled={isDeciding}
                                  onClick={() => handleDecideApproval(p.id, "approved")}
                                  className="flex items-center gap-2 rounded-xl bg-emerald-500 hover:bg-emerald-400 text-black px-4 py-2 text-xs font-bold transition shadow-md disabled:opacity-50"
                                >
                                  {isDeciding ? (
                                    <Loader2 size={13} className="animate-spin" />
                                  ) : (
                                    <ThumbsUp size={13} />
                                  )}
                                  Approve & Unlock Goal
                                </button>

                                <button
                                  type="button"
                                  disabled={isDeciding}
                                  onClick={() => handleDecideApproval(p.id, "rejected")}
                                  className="flex items-center gap-2 rounded-xl border border-rose-500/50 bg-rose-950/20 hover:bg-rose-950/40 text-rose-300 px-4 py-2 text-xs font-bold transition disabled:opacity-50"
                                >
                                  {isDeciding ? (
                                    <Loader2 size={13} className="animate-spin" />
                                  ) : (
                                    <ThumbsDown size={13} />
                                  )}
                                  Reject Proof
                                </button>
                              </div>
                            </div>
                          ) : goal.status === "completed" ? (
                            <div className="flex items-center gap-2 text-xs text-emerald-400 font-semibold bg-emerald-950/20 border border-emerald-500/20 rounded-xl p-2.5">
                              <CheckCircle2 size={15} />
                              <span>Approved and unlocked! Constraints have been released.</span>
                            </div>
                          ) : (
                            <div className="flex items-center gap-2 text-xs text-neutral-400 bg-neutral-900/60 border border-neutral-700/30 rounded-xl p-2.5">
                              <Clock size={14} className="text-amber-400" />
                              <span>
                                Waiting for review by partner{" "}
                                <strong className="text-neutral-200">({goal.approverEmail || "Assigned Approver"})</strong>.
                              </span>
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          )}
      </div>
    </AppLayout>
  );
}

