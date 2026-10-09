import { useState, useEffect } from "react";
import { useParams, useNavigate, useSearchParams, Link } from "react-router-dom";
import {
  ArrowLeft,
  Target,
  Users,
  Edit3,
  Check,
  AlertCircle,
  CheckCircle2,
  Calendar,
  Clock,
  Trash2,
  Loader2,
  ShieldCheck,
  Send,
  ExternalLink,
  ThumbsUp,
  ThumbsDown,
  FileText,
  Share2,
  Lock,
  MessageSquare,
  X,
} from "lucide-react";
import { getUser } from "../lib/auth";
import { getGoal, updateGoal, deleteGoal, getProofs, decideApproval, submitProof } from "../lib/api";
import { getStoredFriends, sendChatMessage } from "../lib/friendsChat";
import { getStoredCommunities, sendCommunityMessage } from "../lib/communities";
import { useWebSocket } from "../hooks/useWebSocket";
import { notifyExtensionSync } from "../lib/extensionSync";

import AppLayout from "../components/AppLayout";
import { getFriendlyTargetName } from "../lib/blockedApps";

function generateMsgId(prefix = "msg") {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
}

const GOAL_BLOCKED_APPS_KEY = "onusly_goal_blocked_apps";

function getGoalBlockedApps(goalId) {
  try {
    const raw = localStorage.getItem(GOAL_BLOCKED_APPS_KEY);
    const map = raw ? JSON.parse(raw) : {};
    return map[goalId] || [];
  } catch {
    return [];
  }
}

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

  // Proofs & Decision State
  const [proofs, setProofs] = useState([]);
  const [loadingProofs, setLoadingProofs] = useState(true);
  const [decidingProofId, setDecidingProofId] = useState(null);
  const [decisionComment, setDecisionComment] = useState("");
  const [decisionError, setDecisionError] = useState("");
  const [decisionSuccess, setDecisionSuccess] = useState("");

  // Subscribe to real-time WebSocket events for instant unlock sync
  const { isConnected, send } = useWebSocket((event) => {
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

  // Share for Verification Modal State
  const [showShareModal, setShowShareModal] = useState(false);
  const [shareTarget, setShareTarget] = useState("friend");
  const [selectedFriendId, setSelectedFriendId] = useState("");
  const [selectedCommunityId, setSelectedCommunityId] = useState("");
  const [sharingProof, setSharingProof] = useState(false);
  const [shareError, setShareError] = useState("");
  const friends = getStoredFriends();
  const communities = getStoredCommunities();

  const openShareModal = () => {
    setShowShareModal(true);
    setShareTarget(friends.length > 0 ? "friend" : "community");
    setSelectedFriendId(friends.length > 0 ? friends[0].id : "");
    setSelectedCommunityId(communities.length > 0 ? communities[0].id : "");
    setShareError("");
  };

  const handleShareVerification = async () => {
    if (!goal) return;
    setSharingProof(true);
    setShareError("");

    const senderEmail = user?.email || "";
    const senderHandle = user?.handle || "";
    const senderName = user?.name || (senderHandle ? `@${senderHandle}` : "You");
    const goalTitle = goal.title;
    const goalId = goal.id;

    try {
      try {
        await submitProof(goalId, {
          proofType: "text",
          textExplanation: `Verification request shared for: ${goalTitle}`,
        });
      } catch (proofErr) {
        console.warn("Backend proof submission skipped:", proofErr.message);
      }

      try {
        await updateGoal(goalId, {
          approvalType: shareTarget === "community" ? "community" : "friend",
          approverEmail: shareTarget === "friend" ? (friends.find((f) => f.id === selectedFriendId)?.email || friends.find((f) => f.id === selectedFriendId)?.handle) : undefined,
        });

        // Submit proof on behalf of owner so goal transitions to proof_submitted and is ready for friend to approve
        try {
          await submitProof(goalId, {
            proofType: "text",
            textExplanation: `Verification request: "${goalTitle}"`,
          });
        } catch (proofErr) {
          console.warn("Auto-submit proof on share:", proofErr.message);
        }
      } catch (updErr) {
        console.warn("Backend goal approver update skipped:", updErr.message);
      }

      const targetId = shareTarget === "community" ? selectedCommunityId : selectedFriendId;

      if (shareTarget === "friend" && selectedFriendId) {
        const friend = friends.find((f) => f.id === selectedFriendId);
        if (!friend) throw new Error("Friend not found");

        const msgId = generateMsgId("msg");

        send({
          type: "chat.message",
          payload: {
            id: msgId,
            recipientEmail: friend.email || "",
            recipientHandle: friend.handle || "",
            senderEmail,
            senderHandle,
            senderName,
            text: `Please verify my goal: "${goalTitle}"`,
            isProof: true,
            goalId,
            goalTitle,
            ownerId: user?.id || "",
          },
        });

        sendChatMessage(
          selectedFriendId,
          {
            id: msgId,
            text: `Please verify my goal: "${goalTitle}"`,
            isProof: true,
            goalId,
            goalTitle,
            ownerId: user?.id || "",
          },
          "me"
        );

        setSuccessNotice(`Verification request sent to ${friend.name}! Opening chat...`);
      } else if (shareTarget === "community" && selectedCommunityId) {
        const comm = communities.find((c) => c.id === selectedCommunityId);
        if (!comm) throw new Error("Community not found");

        const msgId = generateMsgId("comm-msg");

        send({
          type: "community.message",
          payload: {
            id: msgId,
            communityId: comm.id,
            communityName: comm.name || "Community",
            communityCode: comm.code || "",
            senderName,
            senderEmail,
            senderHandle,
            text: `Please verify my goal: "${goalTitle}"`,
            isProof: true,
            goalId,
            goalTitle,
            ownerId: user?.id || "",
          },
        });

        sendCommunityMessage(comm.id, {
          id: msgId,
          senderName: "You",
          senderHandle,
          text: `Please verify my goal: "${goalTitle}"`,
          isProof: true,
          goalId,
          goalTitle,
          ownerId: user?.id || "",
        });

        setSuccessNotice(`Verification request shared in ${comm.name}! Opening chat...`);
      }

      setShowShareModal(false);

      setTimeout(() => {
        navigate("/community-friends", {
          state: {
            view: shareTarget === "community" ? "communities" : "friends",
            targetId,
          },
        });
      }, 600);
    } catch (err) {
      console.error("Failed to share verification:", err);
      setShareError(err.message || "Failed to share");
    } finally {
      setSharingProof(false);
    }
  };

  // Edit Goal Title/Description state
  const [isEditingGoal, setIsEditingGoal] = useState(false);
  const [editTitle, setEditTitle] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [savingGoal, setSavingGoal] = useState(false);
  const [goalFormError, setGoalFormError] = useState("");

  const [copiedReviewLink, setCopiedReviewLink] = useState(false);

  // Load Goal & Proofs
  useEffect(() => {
    let ignore = false;

    getGoal(id, ownerIdParam)
      .then((data) => {
        if (!ignore && data) {
          setGoal(data);
          setEditTitle(data.title || "");
          setEditDescription(data.description || "");
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

      // Trigger extension blocklist update immediately
      notifyExtensionSync();

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
                    {goal.status !== "completed" && (
                      <button
                        type="button"
                        onClick={openShareModal}
                        className="flex items-center gap-2 rounded-xl bg-white px-3.5 py-2 text-xs font-bold text-black hover:bg-neutral-200 transition shadow-sm"
                      >
                        <Share2 size={14} />
                        <span>Share for Verification</span>
                      </button>
                    )}

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
                  2. DISTRACTION APPS & IN-CHAT VERIFICATION STRATEGY
              ===================================================== */}
              <div className="rounded-[28px] border border-[#777777]/50 bg-[#262626] p-6 shadow-xl sm:p-8">
                <div className="mb-6 flex flex-col justify-between gap-2 sm:flex-row sm:items-center">
                  <div>
                    <div className="flex items-center gap-2.5">
                      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
                        <Lock size={18} />
                      </div>
                      <h2 className="text-xl font-bold tracking-tight text-white">
                        Distraction Shield & App Blockers
                      </h2>
                    </div>
                    <p className="mt-1 text-xs text-[#B5B5B5]">
                      Apps locked while this goal constraint is active.
                    </p>
                  </div>

                  <span className="self-start sm:self-auto rounded-full border border-neutral-700 bg-neutral-800 px-3 py-1 text-xs font-medium text-neutral-300">
                    Status: <strong className="text-white">{goal.status === "completed" ? "Unlocked" : "Active Lock"}</strong>
                  </span>
                </div>

                {/* Blocked Apps & Domains Display */}
                <div className="rounded-2xl border border-[#333333] bg-[#1A1A1A] p-5">
                  <h4 className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3] mb-3">
                    Target Apps & Domains Blocked For This Goal
                  </h4>
                  {((goal?.blockedApps?.length > 0 || goal?.blockedDomains?.length > 0) || getGoalBlockedApps(id).length > 0) ? (
                    <div className="flex flex-wrap gap-2">
                      {[
                        ...(goal?.blockedApps || []),
                        ...(goal?.blockedDomains || []),
                        ...(goal?.blockedApps?.length || goal?.blockedDomains?.length ? [] : getGoalBlockedApps(id)),
                      ].map((target) => (
                        <span
                          key={target}
                          className="inline-flex items-center gap-1.5 rounded-xl border border-red-900/40 bg-red-950/20 px-3 py-1.5 text-xs font-semibold text-red-300"
                        >
                          <Lock size={12} className="text-red-400" />
                          <span>{getFriendlyTargetName(target)}</span>
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="text-xs text-[#737373] italic">
                      Standard Distraction Lock Active (Focus mode enabled).
                    </p>
                  )}
                </div>

                {/* How to Unlock via Chat Card */}
                <div className="mt-5 rounded-2xl border border-emerald-900/40 bg-gradient-to-r from-emerald-950/30 to-[#1e2e26]/30 p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <MessageSquare size={16} className="text-emerald-400" />
                      <h4 className="text-sm font-bold text-white">
                        Verify & Unlock via Chat
                      </h4>
                    </div>
                    <p className="text-xs text-[#A3A3A3] leading-relaxed max-w-lg">
                      Submit proof directly in Community or Friend chat using the <strong className="text-white">+</strong> button. Once verified, this goal unlocks automatically in real time.
                    </p>
                  </div>

                  {goal.status !== "completed" ? (
                    <button
                      type="button"
                      onClick={openShareModal}
                      className="inline-flex items-center gap-2 rounded-xl bg-white px-5 py-2.5 text-xs font-bold text-black hover:bg-neutral-200 transition shrink-0"
                    >
                      <Share2 size={13} />
                      <span>Share for Verification in Chat</span>
                    </button>
                  ) : (
                    <Link
                      to="/community-friends"
                      className="inline-flex items-center gap-2 rounded-xl bg-[#292929] border border-[#444] px-5 py-2.5 text-xs font-semibold text-white hover:border-white transition shrink-0"
                    >
                      <span>Open Community & Friends Chat</span>
                      <ExternalLink size={13} />
                    </Link>
                  )}
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
                          ? "Community"
                          : goal.approvalType === "friend"
                          ? `Friend Partner (${goal.approverEmail || "Partner"})`
                          : "Honor System"}
                      </strong>.
                    </p>
                    {goal.status === "active" && (
                      <Link
                        to="/community-friends"
                        className="mt-4 inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-[#292929] hover:bg-[#B5B5B5] transition"
                      >
                        <Send size={13} />
                        Share Proof in Community / Friend Chat
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

        {/* ============================================================
            SHARE FOR VERIFICATION MODAL
        ============================================================ */}
        {showShareModal && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/85 p-4 backdrop-blur-md"
            onClick={() => setShowShareModal(false)}
          >
            <div
              className="w-full max-w-md rounded-2xl border border-[#333333] bg-[#181818] p-6 shadow-2xl"
              onClick={(e) => e.stopPropagation()}
            >
              {/* Header */}
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                <div className="flex items-center gap-2.5">
                  <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
                    <Share2 size={18} />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-white">
                      Share for Verification
                    </h3>
                    <p className="text-[11px] text-[#A3A3A3]">
                      Send "{goal?.title}" to be verified in chat
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setShowShareModal(false)}
                  className="rounded-lg p-1.5 text-[#737373] hover:text-white hover:bg-[#252525] transition"
                >
                  <X size={18} />
                </button>
              </div>

              {shareError && (
                <div className="mt-4 rounded-xl border border-red-900 bg-red-950/40 p-3 text-xs text-red-300">
                  {shareError}
                </div>
              )}

              <div className="mt-5 space-y-4">
                {/* Target Type Selector: Friend vs Community */}
                <div>
                  <label className="block text-xs font-bold text-white mb-2">
                    Share with
                  </label>
                  <div className="flex rounded-xl border border-[#333333] bg-[#121212] p-1 text-xs">
                    <button
                      type="button"
                      onClick={() => setShareTarget("friend")}
                      className={`flex-1 flex items-center justify-center gap-1.5 rounded-lg px-3 py-2 transition-all ${
                        shareTarget === "friend"
                          ? "bg-white text-black font-bold shadow-sm"
                          : "text-[#A3A3A3] hover:text-white"
                      }`}
                    >
                      <MessageSquare size={14} />
                      Friend
                    </button>
                    <button
                      type="button"
                      onClick={() => setShareTarget("community")}
                      className={`flex-1 flex items-center justify-center gap-1.5 rounded-lg px-3 py-2 transition-all ${
                        shareTarget === "community"
                          ? "bg-white text-black font-bold shadow-sm"
                          : "text-[#A3A3A3] hover:text-white"
                      }`}
                    >
                      <Users size={14} />
                      Community
                    </button>
                  </div>
                </div>

                {/* Friend / Community selector */}
                {shareTarget === "friend" ? (
                  <div>
                    <label className="block text-xs font-bold text-white mb-1.5">
                      Choose Friend
                    </label>
                    {friends.length === 0 ? (
                      <div className="rounded-xl border border-[#333333] bg-[#141414] p-3 text-xs text-[#A3A3A3]">
                        No friends added yet.{" "}
                        <Link to="/community-friends" className="text-white underline font-semibold">
                          Add a friend first
                        </Link>
                      </div>
                    ) : (
                      <div className="max-h-48 overflow-y-auto rounded-xl border border-[#333333] bg-[#141414] p-1.5 space-y-0.5">
                        {friends.map((f) => {
                          const isSelected = f.id === selectedFriendId;
                          return (
                            <button
                              key={f.id}
                              type="button"
                              onClick={() => setSelectedFriendId(f.id)}
                              className={`flex w-full items-center gap-3 rounded-lg p-2.5 text-left transition ${
                                isSelected ? "bg-[#252525] border border-white/20" : "hover:bg-[#1E1E1E]"
                              }`}
                            >
                              <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-[#444444] bg-[#2E2E2E] text-[10px] font-bold text-white overflow-hidden">
                                {f.picture ? (
                                  <img src={f.picture} alt={f.name} className="h-full w-full object-cover" referrerPolicy="no-referrer" />
                                ) : (
                                  f.name[0]?.toUpperCase() || "F"
                                )}
                              </div>
                              <div className="flex-1 min-w-0">
                                <p className="text-xs font-semibold text-white truncate">{f.name}</p>
                                {f.handle && (
                                  <p className="font-mono text-[10px] text-[#737373]">@{f.handle.replace(/^@/, "")}</p>
                                )}
                              </div>
                              {isSelected && (
                                <CheckCircle2 size={16} className="text-emerald-400 flex-shrink-0" />
                              )}
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </div>
                ) : (
                  <div>
                    <label className="block text-xs font-bold text-white mb-1.5">
                      Choose Community
                    </label>
                    {communities.length === 0 ? (
                      <div className="rounded-xl border border-[#333333] bg-[#141414] p-3 text-xs text-[#A3A3A3]">
                        No communities joined yet.{" "}
                        <Link to="/community-friends" className="text-white underline font-semibold">
                          Create or join one
                        </Link>
                      </div>
                    ) : (
                      <div className="max-h-48 overflow-y-auto rounded-xl border border-[#333333] bg-[#141414] p-1.5 space-y-0.5">
                        {communities.map((c) => {
                          const isSelected = c.id === selectedCommunityId;
                          return (
                            <button
                              key={c.id}
                              type="button"
                              onClick={() => setSelectedCommunityId(c.id)}
                              className={`flex w-full items-center gap-3 rounded-lg p-2.5 text-left transition ${
                                isSelected ? "bg-[#252525] border border-white/20" : "hover:bg-[#1E1E1E]"
                              }`}
                            >
                              <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-[#444444] bg-[#2E2E2E] text-white">
                                <Users size={14} />
                              </div>
                              <div className="flex-1 min-w-0">
                                <p className="text-xs font-semibold text-white truncate">{c.name}</p>
                                <p className="text-[10px] text-[#737373]">{c.membersCount} Members</p>
                              </div>
                              {isSelected && (
                                <CheckCircle2 size={16} className="text-emerald-400 flex-shrink-0" />
                              )}
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </div>
                )}

                {/* Action buttons */}
                <div className="pt-2 flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => setShowShareModal(false)}
                    disabled={sharingProof}
                    className="flex-1 rounded-xl border border-[#3A3A3A] bg-[#222222] py-3 text-xs font-semibold text-white hover:border-white transition"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={handleShareVerification}
                    disabled={
                      sharingProof ||
                      (shareTarget === "friend" && (!selectedFriendId || friends.length === 0)) ||
                      (shareTarget === "community" && (!selectedCommunityId || communities.length === 0))
                    }
                    className="flex-1 flex items-center justify-center gap-2 rounded-xl bg-white py-3 text-xs font-bold text-black transition-all hover:bg-neutral-200 disabled:opacity-40"
                  >
                    {sharingProof ? (
                      <>
                        <Loader2 size={15} className="animate-spin" />
                        <span>Sending...</span>
                      </>
                    ) : (
                      <>
                        <Send size={14} />
                        <span>Share in Chat</span>
                      </>
                    )}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

