import { useState, useEffect } from "react";
import { Link, useNavigate } from "react-router-dom";
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
  Share2,
  Users,
  X,
  Send,
  Pin,
  RotateCcw,
} from "lucide-react";
import { getGoals, deleteGoal, submitProof, updateGoal, repeatGoal } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";
import { getStoredFriends } from "../lib/friendsChat";
import { getStoredCommunities } from "../lib/communities";
import CreateGoalModal from "../components/CreateGoalModal";
import { getGoalBlockedApps, getUniqueDisplayTargets } from "../lib/blockedApps";
import { getUser } from "../lib/auth";
import { sendChatMessage } from "../lib/friendsChat";
import { sendCommunityMessage } from "../lib/communities";
import { notifyExtensionSync } from "../lib/extensionSync";
import {
  getPinnedGoalIds,
  togglePinGoal,
  transferPinGoal,
  MAX_PINNED_GOALS,
} from "../lib/pinnedGoals";

function generateMsgId(prefix = "msg") {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
}

export default function Goals() {
  const navigate = useNavigate();
  const [goals, setGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [successNotice, setSuccessNotice] = useState("");
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [pinnedIds, setPinnedIds] = useState(() => getPinnedGoalIds());
  const [repeatingGoalId, setRepeatingGoalId] = useState(null);

  const handleTogglePin = (goalId) => {
    const res = togglePinGoal(goalId);
    setPinnedIds(res.pinnedIds);
    if (res.error) {
      setError(res.error);
      setTimeout(() => setError(""), 4500);
    } else if (res.pinned) {
      setSuccessNotice(`📌 Goal pinned to dashboard (${res.pinnedIds.length}/${MAX_PINNED_GOALS})`);
      setTimeout(() => setSuccessNotice(""), 3000);
    } else {
      setSuccessNotice(`Goal unpinned from dashboard (${res.pinnedIds.length}/${MAX_PINNED_GOALS})`);
      setTimeout(() => setSuccessNotice(""), 2500);
    }
  };

  const handleRepeatGoal = async (g) => {
    if (!g) return;

    if (g.status !== "completed") {
      setError(`⚠️ "${g.title}" is currently active! Complete or unlock the current lock before repeating.`);
      setTimeout(() => setError(""), 5000);
      return;
    }

    setRepeatingGoalId(g.id);
    try {
      const newGoal = await repeatGoal(g);

      if (pinnedIds.includes(g.id)) {
        const updatedPinned = transferPinGoal(g.id, newGoal.id);
        setPinnedIds(updatedPinned);
      }

      setGoals((prev) => [newGoal, ...prev]);
      notifyExtensionSync();
      setSuccessNotice(`🔁 Routine Repeated: New lock started for "${newGoal.title}"! Apps & sites are blocked.`);
      setTimeout(() => setSuccessNotice(""), 6000);
    } catch (err) {
      console.error("Failed to repeat routine:", err);
      setError(err.message || "Failed to repeat routine");
      setTimeout(() => setError(""), 5000);
    } finally {
      setRepeatingGoalId(null);
    }
  };

  // Share for verification modal
  const [shareGoal, setShareGoal] = useState(null); // goal to share
  const [shareTarget, setShareTarget] = useState("friend"); // 'friend' | 'community'
  const [selectedFriendId, setSelectedFriendId] = useState("");
  const [selectedCommunityId, setSelectedCommunityId] = useState("");
  const [sharingProof, setSharingProof] = useState(false);
  const [shareError, setShareError] = useState("");

  const friends = getStoredFriends();
  const communities = getStoredCommunities();
  const currentUser = getUser();

  const { send } = useWebSocket((event) => {
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
          setError(err.message || "Failed to load goals. Make sure the backend server is running.");
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

  // Open share modal for a goal
  const openShareModal = (goal, e) => {
    e.preventDefault();
    e.stopPropagation();
    setShareGoal(goal);
    setShareTarget(friends.length > 0 ? "friend" : "community");
    setSelectedFriendId(friends.length > 0 ? friends[0].id : "");
    setSelectedCommunityId(communities.length > 0 ? communities[0].id : "");
    setShareError("");
  };

  // Share verification link to chat
  const handleShareVerification = async () => {
    if (!shareGoal) return;
    setSharingProof(true);
    setShareError("");

    const senderEmail = currentUser?.email || "";
    const senderHandle = currentUser?.handle || "";
    const senderName = currentUser?.name || (senderHandle ? `@${senderHandle}` : "You");
    const goalTitle = shareGoal.title;
    const goalId = shareGoal.id;

    try {
      // Update backend goal configuration while active so approver is recognized
      try {
        if (shareGoal.status === "active") {
          await updateGoal(goalId, {
            approvalType: shareTarget === "community" ? "community" : "friend",
            approverEmail: shareTarget === "friend" ? (friends.find((f) => f.id === selectedFriendId)?.email || friends.find((f) => f.id === selectedFriendId)?.handle) : undefined,
          });
        }
      } catch (updErr) {
        console.warn("Backend goal approver update skipped:", updErr.message);
      }

      // Submit proof on behalf of owner so goal transitions to proof_submitted and is ready for friend to approve
      if (shareGoal.status === "active") {
        try {
          await submitProof(goalId, {
            proofType: "text",
            textExplanation: `Verification request: "${goalTitle}"`,
          });
        } catch (proofErr) {
          console.warn("Auto-submit proof on share:", proofErr.message);
        }
      }

      const targetId = shareTarget === "community" ? selectedCommunityId : selectedFriendId;

      if (shareTarget === "friend" && selectedFriendId) {
        const friend = friends.find((f) => f.id === selectedFriendId);
        if (!friend) throw new Error("Friend not found");

        const msgId = generateMsgId("msg");

        // Send via WebSocket
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
            ownerId: currentUser?.id || "",
          },
        });

        // Save locally
        sendChatMessage(
          selectedFriendId,
          {
            id: msgId,
            text: `Please verify my goal: "${goalTitle}"`,
            isProof: true,
            goalId,
            goalTitle,
            ownerId: currentUser?.id || "",
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
            ownerId: currentUser?.id || "",
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
          ownerId: currentUser?.id || "",
        });

        setSuccessNotice(`Verification request shared in ${comm.name}! Opening chat...`);
      }

      setShareGoal(null);

      // Navigate to chat thread so user can immediately drop photos/proof
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

  return (
    <AppLayout>
      <div className="mx-auto w-full max-w-[1720px] space-y-7">
        {/* Top Header */}
        <div className="flex flex-col justify-between gap-4 border-b border-white/[0.08] pb-6 sm:flex-row sm:items-center">
          <div>
            <h1 className="text-3xl font-black tracking-tight text-white sm:text-4xl">
              Lockout Goals
            </h1>
            <p className="mt-1 text-xs sm:text-sm text-[#A1A1AA] leading-relaxed">
              Active focus tasks and distraction constraints. Click + to start a new lock.
            </p>
          </div>

          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={() => setShowCreateModal(true)}
              className="btn-glow inline-flex items-center gap-2 rounded-xl px-5 py-2.5 text-xs font-bold uppercase tracking-wider text-black shadow-lg"
              title="Create new accountability lock"
            >
              <Plus size={16} strokeWidth={3} />
              <span>New Lock</span>
            </button>
          </div>
        </div>

        {/* Notices */}
        {successNotice && (
          <div className="flex items-center gap-2.5 rounded-2xl border border-white/20 bg-white/[0.06] backdrop-blur-md p-4 text-xs font-semibold text-white animate-fade-in shadow-lg">
            <CheckCircle2 size={16} className="shrink-0 text-white" />
            <span>{successNotice}</span>
          </div>
        )}
        {error && (
          <div className="flex items-center gap-2.5 rounded-2xl border border-white/10 bg-white/[0.04] backdrop-blur-md p-4 text-xs font-semibold text-neutral-300 animate-fade-in shadow-lg">
            <AlertCircle size={16} className="shrink-0 text-neutral-400" />
            <span>{error}</span>
          </div>
        )}

        {/* Goals List (Full Width Clean View) */}
        <div className="space-y-4">
          <div className="flex items-center justify-between pb-1">
            <h2 className="text-xs font-bold uppercase tracking-wider text-[#A1A1AA]">
              Your Created Goals ({goals.length})
            </h2>

            <span className="text-[11px] text-neutral-400">
              Click "Share" on a goal to send verification to chat
            </span>
          </div>

          {loading ? (
            <div className="flex h-64 items-center justify-center rounded-2xl border border-white/10 bg-[#141418]/60 backdrop-blur-xl">
              <Loader2 size={26} className="animate-spin text-white" />
            </div>
          ) : goals.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-white/10 bg-[#141418]/40 backdrop-blur-xl p-14 text-center">
              <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-white/10 bg-white/5 text-[#A1A1AA] mb-4">
                <Target size={26} />
              </div>
              <h3 className="text-lg font-bold text-white">No active goals yet</h3>
              <p className="mt-1.5 text-xs text-[#A1A1AA] max-w-sm mx-auto leading-relaxed">
                Ready to focus? Click below to create your accountability lock and block distracting apps.
              </p>
              <button
                type="button"
                onClick={() => setShowCreateModal(true)}
                className="btn-glow mt-5 inline-flex items-center gap-2 rounded-xl px-5 py-2.5 text-xs font-bold text-black"
              >
                <Plus size={15} strokeWidth={3} />
                <span>Create Your First Lock</span>
              </button>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
              {goals.map((g) => {
                const isCompleted = g.status === "completed";
                const isProofSubmitted = g.status === "proof_submitted";
                const isPinned = pinnedIds.includes(g.id);
                const displayTargets = getUniqueDisplayTargets(
                  g.blockedApps,
                  g.blockedDomains,
                  getGoalBlockedApps(g.id)
                );

                return (
                  <div
                    key={g.id}
                    className={`card-glass group relative flex flex-col justify-between rounded-2xl p-6 transition-all duration-300 shadow-md ${
                      isPinned
                        ? "border-white/30 shadow-[0_0_30px_rgba(255,255,255,0.04)]"
                        : "border-white/[0.08]"
                    }`}
                  >
                    <div>
                      {/* Top Row: Title + Status Badge + Pin */}
                      <div className="flex items-start justify-between gap-3">
                        <Link
                          to={`/goals/${g.id}`}
                          className="text-base font-bold text-white hover:text-neutral-100 transition line-clamp-1 flex-1"
                        >
                          {g.title}
                        </Link>

                        <div className="flex items-center gap-1.5 shrink-0">
                          {isCompleted ? (
                            <span className="inline-flex items-center gap-1 rounded-full border border-white/25 bg-white/10 px-2.5 py-0.5 text-[10px] font-bold text-white glow-pill-white">
                              <Unlock size={11} />
                              Unlocked
                            </span>
                          ) : isProofSubmitted ? (
                            <span className="inline-flex items-center gap-1 rounded-full border border-white/15 bg-white/5 px-2.5 py-0.5 text-[10px] font-bold text-neutral-300 glow-pill-white">
                              Pending
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1 rounded-full border border-white/15 bg-white/5 px-2.5 py-0.5 text-[10px] font-bold text-neutral-300">
                              <Lock size={11} className="text-neutral-400" />
                              Active
                            </span>
                          )}

                          {/* Pin Toggle Button */}
                          <button
                            type="button"
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              handleTogglePin(g.id);
                            }}
                            className={`btn-premium flex items-center gap-1 rounded-lg border px-2 py-0.5 text-[10px] font-semibold transition ${
                              isPinned
                                ? "border-white bg-white text-black font-bold shadow-sm"
                                : "border-white/10 bg-white/5 text-[#A1A1AA] hover:text-white hover:border-white/20 hover:bg-white/10"
                            }`}
                            title={
                              isPinned
                                ? "Unpin from dashboard"
                                : `Pin to dashboard (${pinnedIds.length}/${MAX_PINNED_GOALS})`
                            }
                          >
                            <Pin size={11} className={isPinned ? "fill-black" : ""} />
                            <span>{isPinned ? "Pinned" : "Pin"}</span>
                          </button>

                          <button
                            type="button"
                            onClick={(e) => handleDelete(g.id, e)}
                            className="rounded-lg p-1 text-neutral-500 opacity-0 transition-all hover:text-white hover:bg-white/5 group-hover:opacity-100"
                            title="Delete Goal"
                          >
                            <Trash2 size={13} />
                          </button>
                        </div>
                      </div>

                      {/* Description */}
                      {g.description ? (
                        <p className="mt-2 text-xs text-[#A1A1AA] line-clamp-2 leading-relaxed">
                          {g.description}
                        </p>
                      ) : (
                        <p className="mt-2 text-xs italic text-neutral-500">
                          No description provided.
                        </p>
                      )}

                      {/* Blocked Apps & Domains list */}
                      <div className="mt-4 flex flex-wrap items-center gap-1.5">
                        {displayTargets.length > 0 ? (
                          displayTargets.map((target) => (
                            <span
                              key={target.label}
                              className="inline-flex items-center gap-1 rounded-md border border-white/10 bg-white/[0.04] px-2 py-0.5 text-[10px] font-medium text-neutral-300 transition-colors hover:border-white/20"
                            >
                              <Lock size={9} className="text-neutral-400" />
                              {target.label}
                            </span>
                          ))
                        ) : (
                          <span className="text-[10px] text-neutral-400 italic">
                            Full Focus Shield Active
                          </span>
                        )}
                      </div>
                    </div>

                    {/* Card Footer Actions */}
                    <div className="mt-5 flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.08] pt-3 text-[11px]">
                      <div className="flex items-center gap-2">
                        <button
                          type="button"
                          onClick={(e) => openShareModal(g, e)}
                          className="btn-premium inline-flex items-center gap-1.5 rounded-lg border border-white/15 bg-white/5 px-2.5 py-1 font-semibold text-white hover:bg-white/10 hover:border-white/30 transition"
                          title="Share for verification with friend or community"
                        >
                          <Share2 size={12} className="text-white" />
                          <span>Share</span>
                        </button>

                        {/* Recurring Goal Button */}
                        <button
                          type="button"
                          disabled={repeatingGoalId === g.id || g.status !== "completed"}
                          onClick={(e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            handleRepeatGoal(g);
                          }}
                          className={`btn-premium inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 font-semibold transition ${
                            g.status === "completed"
                              ? "border-white/15 bg-white/5 text-neutral-200 hover:text-white hover:border-white/30 hover:bg-white/10"
                              : "border-white/5 bg-white/[0.02] text-neutral-600 cursor-not-allowed opacity-50"
                          }`}
                          title={
                            g.status === "completed"
                              ? "Start another cycle of this lock"
                              : "Routine is currently active"
                          }
                        >
                          {repeatingGoalId === g.id ? (
                            <Loader2 size={12} className="animate-spin text-white" />
                          ) : (
                            <RotateCcw size={12} className={g.status === "completed" ? "text-white" : "text-neutral-600"} />
                          )}
                          <span>Repeat Routine</span>
                        </button>
                      </div>

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

        {/* ============================================================
            SHARE FOR VERIFICATION MODAL
        ============================================================ */}
        {shareGoal && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center modal-backdrop p-4"
            onClick={() => setShareGoal(null)}
          >
            <div
              className="w-full max-w-md rounded-2xl border border-white/10 bg-[#181818] p-6 shadow-2xl animate-scale-in"
              onClick={(e) => e.stopPropagation()}
            >
              {/* Header */}
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                <div className="flex items-center gap-2.5">
                  <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-white/10 border border-white/20 text-white">
                    <Share2 size={18} />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-white">
                      Share for Verification
                    </h3>
                    <p className="text-[11px] text-[#A3A3A3]">
                      Send "{shareGoal.title}" to be verified
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setShareGoal(null)}
                  className="rounded-lg p-1.5 text-[#737373] hover:text-white hover:bg-[#252525] transition"
                >
                  <X size={18} />
                </button>
              </div>

              {shareError && (
                <div className="mt-4 rounded-xl border border-white/10 bg-white/[0.04] p-3 text-xs text-neutral-300">
                  {shareError}
                </div>
              )}

              <div className="mt-5 space-y-5">
                {/* Target selector */}
                <div>
                  <label className="block text-xs font-bold uppercase tracking-wider text-white mb-2">
                    Send To
                  </label>
                  <div className="flex items-center rounded-xl border border-[#333333] bg-[#1A1A1A] p-1 text-xs font-semibold">
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
                                <CheckCircle2 size={16} className="text-white flex-shrink-0" />
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
                                <CheckCircle2 size={16} className="text-white flex-shrink-0" />
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
                    onClick={() => setShareGoal(null)}
                    disabled={sharingProof}
                    className="btn-premium flex-1 rounded-xl border border-[#3A3A3A] bg-[#222222] py-3 text-xs font-semibold text-white hover:border-white transition"
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
                    className="btn-premium flex-1 flex items-center justify-center gap-2 rounded-xl bg-white py-3 text-xs font-bold text-black transition-all hover:bg-neutral-100 disabled:opacity-40"
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
