import { useState, useEffect } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { MessageSquare, X, ArrowRight } from "lucide-react";
import { useWebSocket } from "../hooks/useWebSocket";
import { receiveChatMessage, getStoredFriends } from "../lib/friendsChat";

export default function GlobalChatNotifier() {
  const navigate = useNavigate();
  const location = useLocation();
  const [activeToast, setActiveToast] = useState(null);

  const { isConnected } = useWebSocket((event) => {
    if (event?.type === "chat.message") {
      const payload = event.payload || {};
      const timeFormatted = payload.time
        ? new Date(payload.time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
        : new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

      // 1. Save and update local chat store
      const { friend } = receiveChatMessage({
        messageId: payload.id,
        friendEmail: payload.senderEmail,
        text: payload.text,
        time: timeFormatted,
        sender: "friend",
      });

      // 2. If user is NOT currently on the chat page, show the WhatsApp-style popup toast
      const isChatPage = location.pathname === "/community-friends" || location.pathname === "/communities";
      if (!isChatPage) {
        const senderName = friend?.name || (payload.senderEmail ? payload.senderEmail.split("@")[0] : "Friend");
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
      <div className="rounded-2xl border border-white/20 bg-neutral-900/95 p-4 shadow-2xl backdrop-blur-md">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3 min-w-0">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-emerald-500/30 bg-emerald-950/60 text-emerald-400">
              <MessageSquare size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h4 className="truncate text-xs font-bold text-white">
                  {activeToast.senderName}
                </h4>
                {activeToast.isOffline && (
                  <span className="rounded-full bg-blue-950/80 border border-blue-500/30 px-1.5 py-0.2 text-[9px] font-semibold text-blue-300">
                    Offline Delivered
                  </span>
                )}
                <span className="text-[10px] text-neutral-400">{activeToast.time}</span>
              </div>
              <p className="mt-1 line-clamp-2 text-xs text-neutral-300 leading-snug">
                "{activeToast.text}"
              </p>
              <button
                type="button"
                onClick={() => {
                  setActiveToast(null);
                  navigate("/community-friends");
                }}
                className="mt-2.5 inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-400 hover:text-emerald-300 transition"
              >
                <span>Open Chat</span>
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
