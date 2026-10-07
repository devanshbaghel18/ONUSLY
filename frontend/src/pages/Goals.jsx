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
} from "lucide-react";
import { getGoals, deleteGoal, submitProof } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";
import { getStoredFriends } from "../lib/friendsChat";
import { getStoredCommunities } from "../lib/communities";
import CreateGoalModal from "../components/CreateGoalModal";
import { APPS_TO_BLOCK, getGoalBlockedApps } from "../lib/blockedApps";
import { getUser } from "../lib/auth";
import { sendChatMessage } from "../lib/friendsChat";
import { sendCommunityMessage } from "../lib/communities";

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
      // Try to submit proof to backend (soft fail if backend is offline)
      try {
        await submitProof(goalId, {
          proofType: "text",
          textExplanation: `Verification request shared for: ${goalTitle}`,
        });
      } catch (proofErr) {
        console.warn("Backend proof submission skipped:", proofErr.message);
      }

      // Update backend goal configuration so approver is recognized
      try {
        const { updateGoal, submitProof } = await import("../lib/api");
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
              Click "Share" on a goal to send verification to chat
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
                      <button
                        type="button"
                        onClick={(e) => openShareModal(g, e)}
                        className="inline-flex items-center gap-1.5 rounded-lg border border-[#3A3A3A] bg-[#222222] px-2.5 py-1 font-semibold text-white hover:border-white transition"
                        title="Share for verification with friend or community"
                      >
                        <Share2 size={12} className="text-emerald-400" />
                        <span>Share for Verification</span>
                      </button>

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
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/85 p-4 backdrop-blur-md"
            onClick={() => setShareGoal(null)}
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
                <div className="mt-4 rounded-xl border border-red-900 bg-red-950/40 p-3 text-xs text-red-300">
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
                    onClick={() => setShareGoal(null)}
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
