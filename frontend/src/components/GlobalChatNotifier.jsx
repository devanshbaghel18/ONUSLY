import { useState, useEffect } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { MessageSquare, X, ArrowRight, ShieldCheck } from "lucide-react";
import { useWebSocket } from "../hooks/useWebSocket";
import { receiveChatMessage } from "../lib/friendsChat";
import { notifyExtensionSync } from "../lib/extensionSync";

export default function GlobalChatNotifier() {
  const navigate = useNavigate();
  const location = useLocation();
  const [activeToast, setActiveToast] = useState(null);

  useWebSocket((event) => {
    // 1. Goal approval notification event
    if (event?.type === "chat.message.approved" || event?.type === "goal.unlocked") {
      const payload = event.payload || {};
      const timeFormatted = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
      const approverName = payload.approverName || "Accountability Partner";
      const goalTitle = payload.goalTitle || payload.title || "Goal";

      // Trigger extension blocklist refresh immediately so focus lock is released
      notifyExtensionSync();

      setActiveToast({
        id: `toast-approval-${Date.now()}`,
        senderName: approverName,
        text: `Verified and approved "${goalTitle}"! Distraction locks are now lifted.`,
        time: timeFormatted,
        isApproval: true,
      });
      return;
    }

    // 2. Incoming chat message event
    if (event?.type === "chat.message") {
      const payload = event.payload || {};
      const timeFormatted = payload.time
        ? new Date(payload.time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
        : new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

      // If user is currently on the chat page, let Communities.jsx handle it directly
      const isChatPage = location.pathname === "/community-friends" || location.pathname === "/communities";
      if (!isChatPage) {
        // Save and update local chat store with full payload
        const { friend } = receiveChatMessage({
          messageId: payload.id,
          friendEmail: payload.senderEmail,
          friendHandle: payload.senderHandle,
          friendName: payload.senderName,
          text: payload.text,
          time: timeFormatted,
          sender: "friend",
          isProof: payload.isProof,
          goalId: payload.goalId,
          goalTitle: payload.goalTitle,
          ownerId: payload.ownerId,
          approved: payload.approved,
          images: payload.images,
          files: payload.files,
          externalLink: payload.externalLink,
        });

        const senderName = friend?.name || (payload.senderName || (payload.senderEmail ? payload.senderEmail.split("@")[0] : "Friend"));
        setActiveToast({
          id: payload.id || `toast-${Date.now()}`,
          senderName,
          senderEmail: payload.senderEmail,
          text: payload.text,
          time: timeFormatted,
          isOffline: Boolean(payload.isOffline),
        });
      }
    }
  });

  // Auto-dismiss after 6 seconds
  useEffect(() => {
    if (!activeToast) return;
    const timer = setTimeout(() => {
      setActiveToast(null);
    }, 6000);
    return () => clearTimeout(timer);
  }, [activeToast]);

  if (!activeToast) return null;

  return (
    <div className="fixed top-5 right-5 z-50 max-w-sm w-full animate-in fade-in slide-in-from-top-4 duration-300">
      <div className="rounded-2xl border border-white/20 bg-[#161618]/95 p-4 shadow-2xl backdrop-blur-md">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3 min-w-0">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-white/20 bg-white/10 text-white shadow-inner">
              {activeToast.isApproval ? <ShieldCheck size={20} /> : <MessageSquare size={18} />}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h4 className="truncate text-xs font-bold text-white">
                  {activeToast.senderName}
                </h4>
                {activeToast.isApproval && (
                  <span className="rounded-full bg-white/10 border border-white/20 px-2 py-0.5 text-[9px] font-bold text-white">
                    Goal Approved
                  </span>
                )}
                {activeToast.isOffline && (
                  <span className="rounded-full bg-white/10 border border-white/20 px-2 py-0.5 text-[9px] font-semibold text-neutral-300">
                    Offline Delivered
                  </span>
                )}
                <span className="text-[10px] text-neutral-400">{activeToast.time}</span>
              </div>
              <p className="mt-1 line-clamp-2 text-xs text-neutral-300 leading-snug">
                {activeToast.isApproval ? activeToast.text : `"${activeToast.text}"`}
              </p>
              <button
                type="button"
                onClick={() => {
                  setActiveToast(null);
                  if (activeToast.isApproval) {
                    navigate("/goals");
                  } else {
                    navigate("/community-friends");
                  }
                }}
                className="mt-2.5 inline-flex items-center gap-1 text-[11px] font-bold text-white hover:text-neutral-300 transition"
              >
                <span>{activeToast.isApproval ? "View Goals" : "Open Chat"}</span>
                <ArrowRight size={12} />
              </button>
            </div>
          </div>

          <button
            type="button"
            onClick={() => setActiveToast(null)}
            className="rounded-lg p-1 text-neutral-400 hover:bg-neutral-800 hover:text-white transition"
          >
            <X size={14} />
          </button>
        </div>
      </div>
    </div>
  );
}
