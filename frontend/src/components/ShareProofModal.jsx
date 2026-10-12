import { useState, useEffect } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  Share2,
  X,
  MessageSquare,
  Users,
  CheckCircle2,
  Loader2,
  Send,
} from "lucide-react";
import { getUser } from "../lib/auth";
import { updateGoal, submitProof } from "../lib/api";
import { getStoredFriends, sendChatMessage } from "../lib/friendsChat";
import { getStoredCommunities, sendCommunityMessage } from "../lib/communities";
import { wsManager } from "../lib/websocket";

function generateMsgId(prefix = "msg") {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).substring(2, 7)}`;
}

export default function ShareProofModal({
  isOpen,
  onClose,
  goal,
  onSuccess,
  send,
}) {
  const navigate = useNavigate();
  const friends = getStoredFriends();
  const communities = getStoredCommunities();
  const [shareTarget, setShareTarget] = useState("friend");
  const [selectedFriendId, setSelectedFriendId] = useState("");
  const [selectedCommunityId, setSelectedCommunityId] = useState("");
  const [sharingProof, setSharingProof] = useState(false);
  const [shareError, setShareError] = useState("");

  // Reset or select default target when opening
  useEffect(() => {
    if (isOpen) {
      const currentFriends = getStoredFriends();
      const currentComms = getStoredCommunities();
      const timer = setTimeout(() => {
        setShareTarget(currentFriends.length > 0 ? "friend" : "community");
        setSelectedFriendId(currentFriends.length > 0 ? currentFriends[0].id : "");
        setSelectedCommunityId(currentComms.length > 0 ? currentComms[0].id : "");
        setShareError("");
      }, 0);
      return () => clearTimeout(timer);
    }
  }, [isOpen]);

  // Handle escape key
  useEffect(() => {
    const handleKeyDown = (e) => {
      if (e.key === "Escape" && isOpen && !sharingProof) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, sharingProof, onClose]);

  if (!isOpen || !goal) return null;

  const handleShareVerification = async () => {
    setSharingProof(true);
    setShareError("");

    const currentUser = getUser();
    const senderEmail = currentUser?.email || "";
    const senderHandle = currentUser?.handle || "";
    const senderName =
      currentUser?.name || (senderHandle ? `@${senderHandle}` : "You");
    const goalTitle = goal.title;
    const goalId = goal.id;

    // Dispatch function: prefer prop `send` or fall back to `wsManager.send`
    const sendWS = (msg) => {
      if (typeof send === "function") {
        send(msg);
      } else {
        wsManager.send(msg);
      }
    };

    try {
      let targetId = "";
      let resolvedApproverEmail = undefined;

      if (shareTarget === "friend") {
        const friend = friends.find((f) => f.id === selectedFriendId);
        if (!friend) {
          throw new Error("Please select a friend to send the verification request to.");
        }
        targetId = friend.id;
        resolvedApproverEmail =
          friend.email || (friend.handle ? `@${friend.handle}` : undefined);

        // 1. Update backend goal configuration while active so approver is recognized
        if (goal.status === "active" && resolvedApproverEmail) {
          try {
            await updateGoal(goalId, {
              approvalType: "friend",
              approverEmail: resolvedApproverEmail,
            });
          } catch (updErr) {
            console.warn("Backend goal approver update skipped:", updErr.message);
          }
        }

        // 2. Submit proof on behalf of owner so goal transitions to proof_submitted
        if (goal.status === "active") {
          try {
            await submitProof(goalId, {
              proofType: "text",
              textExplanation: `Verification request: "${goalTitle}"`,
            });
          } catch (proofErr) {
            console.warn("Auto-submit proof on share skipped:", proofErr.message);
          }
        }

        const msgId = generateMsgId("msg");

        // 3. Send real-time chat message via WebSocket
        sendWS({
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

        // 4. Save message in local chat store
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
      } else {
        const comm = communities.find((c) => c.id === selectedCommunityId);
        if (!comm) {
          throw new Error("Please select a community to share the verification request in.");
        }
        targetId = comm.id;

        // 1. Update backend goal configuration while active
        if (goal.status === "active") {
          try {
            await updateGoal(goalId, {
              approvalType: "community",
            });
          } catch (updErr) {
            console.warn("Backend goal approver update skipped:", updErr.message);
          }

          try {
            await submitProof(goalId, {
              proofType: "text",
              textExplanation: `Verification request: "${goalTitle}"`,
            });
          } catch (proofErr) {
            console.warn("Auto-submit proof on share skipped:", proofErr.message);
          }
        }

        const msgId = generateMsgId("comm-msg");

        sendWS({
          type: "community.message",
          payload: {
            id: msgId,
            communityId: comm.id,
            communityName: comm.name || "Community",
            communityCode: comm.code || "",
            senderName: currentUser?.name || (currentUser?.handle ? `@${currentUser.handle}` : "You"),
            senderEmail: currentUser?.email || "",
            senderHandle: currentUser?.handle || "",
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
          senderHandle: currentUser?.handle || "",
          text: `Please verify my goal: "${goalTitle}"`,
          isProof: true,
          goalId,
          goalTitle,
          ownerId: currentUser?.id || "",
        });
      }

      if (onSuccess) {
        onSuccess({
          ...goal,
          status: "proof_submitted",
          approvalType: shareTarget,
          approverEmail: resolvedApproverEmail,
        });
      }

      onClose();

      // Navigate to chat thread so user can immediately view and converse with partner
      setTimeout(() => {
        navigate("/community-friends", {
          state: {
            view: shareTarget === "community" ? "communities" : "friends",
            targetId,
          },
        });
      }, 300);
    } catch (err) {
      console.error("Failed to share verification:", err);
      setShareError(err.message || "Failed to share verification request");
    } finally {
      setSharingProof(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="share-modal-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/85 p-4 backdrop-blur-md"
      onClick={onClose}
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
              <h3 id="share-modal-title" className="text-sm font-bold text-white">
                Share for Verification
              </h3>
              <p className="text-[11px] text-[#A3A3A3] truncate max-w-[260px]">
                Send "{goal.title}" to be verified in chat
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
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
                  <Link
                    to="/community-friends"
                    onClick={onClose}
                    className="text-white underline font-semibold"
                  >
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
                          isSelected
                            ? "bg-[#252525] border border-white/20"
                            : "hover:bg-[#1E1E1E]"
                        }`}
                      >
                        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-[#444444] bg-[#2E2E2E] text-[10px] font-bold text-white overflow-hidden">
                          {f.picture ? (
                            <img
                              src={f.picture}
                              alt={f.name}
                              className="h-full w-full object-cover"
                              referrerPolicy="no-referrer"
                            />
                          ) : (
                            f.name?.[0]?.toUpperCase() || "F"
                          )}
                        </div>
                        <div className="flex-1 min-w-0">
                          <p className="text-xs font-semibold text-white truncate">
                            {f.name}
                          </p>
                          {f.handle && (
                            <p className="font-mono text-[10px] text-[#737373]">
                              @{f.handle.replace(/^@/, "")}
                            </p>
                          )}
                        </div>
                        {isSelected && (
                          <CheckCircle2
                            size={16}
                            className="text-emerald-400 flex-shrink-0"
                          />
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
                  <Link
                    to="/community-friends"
                    onClick={onClose}
                    className="text-white underline font-semibold"
                  >
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
                          isSelected
                            ? "bg-[#252525] border border-white/20"
                            : "hover:bg-[#1E1E1E]"
                        }`}
                      >
                        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-[#444444] bg-[#2E2E2E] text-white">
                          <Users size={14} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <p className="text-xs font-semibold text-white truncate">
                            {c.name}
                          </p>
                          <p className="text-[10px] text-[#737373]">
                            {c.membersCount || 0} Members
                          </p>
                        </div>
                        {isSelected && (
                          <CheckCircle2
                            size={16}
                            className="text-emerald-400 flex-shrink-0"
                          />
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
              onClick={onClose}
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
                (shareTarget === "friend" &&
                  (!selectedFriendId || friends.length === 0)) ||
                (shareTarget === "community" &&
                  (!selectedCommunityId || communities.length === 0))
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
  );
}
