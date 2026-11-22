import { useState, useEffect, useRef, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import {
  Bell,
  CheckCheck,
  AlertCircle,
  FileText,
  Unlock,
  X,
} from "lucide-react";
import {
  getNotifications,
  markNotificationRead,
  markAllNotificationsRead,
} from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";

function formatRelativeTime(dateString) {
  if (!dateString) return "";
  const date = new Date(dateString);
  if (isNaN(date.getTime())) return "";
  const now = new Date();
  const diffSec = Math.floor((now - date) / 1000);

  if (diffSec < 45) return "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString([], { month: "short", day: "numeric" });
}

export default function NotificationFeed() {
  const navigate = useNavigate();
  const [isOpen, setIsOpen] = useState(false);
  const [notifications, setNotifications] = useState([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [markingAll, setMarkingAll] = useState(false);
  const containerRef = useRef(null);

  const fetchNotifications = useCallback(() => {
    getNotifications()
      .then((res) => {
        if (res && Array.isArray(res.notifications)) {
          setNotifications(res.notifications);
          setUnreadCount(typeof res.unreadCount === "number" ? res.unreadCount : 0);
        }
      })
      .catch((err) => {
        console.warn("[NotificationFeed] Failed to load notifications:", err);
      });
  }, []);

  // Initial fetch on mount
  useEffect(() => {
    fetchNotifications();
  }, [fetchNotifications]);

  // Live real-time sync via existing Slice A WebSocket
  useWebSocket((event) => {
    if (
      event?.type === "goal.unlocked" ||
      event?.type === "proof.submitted" ||
      event?.type === "notification.new"
    ) {
      console.log("[NotificationFeed] Real-time event received, refreshing feed...");
      fetchNotifications();
    }
  });

  // Close dropdown on outside click or Escape key
  useEffect(() => {
    const handleOutside = (e) => {
      if (containerRef.current && !containerRef.current.contains(e.target)) {
        setIsOpen(false);
      }
    };
    const handleKeyDown = (e) => {
      if (e.key === "Escape") {
        setIsOpen(false);
      }
    };

    if (isOpen) {
      document.addEventListener("mousedown", handleOutside);
      document.addEventListener("keydown", handleKeyDown);
    }
    return () => {
      document.removeEventListener("mousedown", handleOutside);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [isOpen]);

  const handleToggle = () => {
    setIsOpen((prev) => !prev);
    if (!isOpen) {
      fetchNotifications();
    }
  };

  const handleItemClick = async (notif) => {
    if (!notif.read) {
      // Optimistically mark read in local state
      setNotifications((prev) =>
        prev.map((n) => (n.id === notif.id ? { ...n, read: true } : n))
      );
      setUnreadCount((c) => Math.max(0, c - 1));
      try {
        await markNotificationRead(notif.id);
      } catch (err) {
        console.error("Failed to mark read:", err);
      }
    }

    setIsOpen(false);
    if (notif.goalId) {
      if (notif.type === "proof_submitted") {
        navigate("/community-friends", {
          state: {
            view: "friends",
            targetId: notif.ownerId || "",
          },
        });
      } else {
        const ownerParam = notif.ownerId ? `?ownerId=${encodeURIComponent(notif.ownerId)}` : "";
        navigate(`/goals/${notif.goalId}${ownerParam}`);
      }
    }
  };

  const handleMarkAllRead = async () => {
    if (unreadCount === 0 || markingAll) return;
    setMarkingAll(true);
    setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
    setUnreadCount(0);

    try {
      await markAllNotificationsRead();
    } catch (err) {
      console.error("Failed to mark all read:", err);
      fetchNotifications();
    } finally {
      setMarkingAll(false);
    }
  };

  return (
    <div className="relative" ref={containerRef}>
      {/* Bell Icon Trigger */}
      <button
        type="button"
        onClick={handleToggle}
        aria-label="Notifications"
        aria-expanded={isOpen}
        className={`relative flex h-9 w-9 sm:h-10 sm:w-10 lg:h-11 lg:w-11 items-center justify-center rounded border transition-all ${
          isOpen
            ? "border-[#ff7843] bg-[#1c2430] text-[#ff7843] shadow-inner"
            : "border-[#26313d] bg-[#12161c] text-neutral-300 hover:border-[#384654] hover:text-white"
        }`}
        title="Notifications"
      >
        <Bell size={17} />
        {unreadCount > 0 && (
          <span className="absolute -top-1 -right-1 flex h-4 min-w-[16px] items-center justify-center rounded bg-[#ff7843] px-1 font-mono text-[9px] font-bold text-black shadow-[0_0_8px_rgba(255,120,67,0.6)] animate-pulse">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        )}
      </button>

      {/* Notification Dropdown Panel */}
      {isOpen && (
        <div className="absolute right-0 top-12 sm:top-14 z-50 w-80 sm:w-96 rounded-lg border border-[#2b3542] bg-[#12161c] p-0 shadow-[0_25px_60px_rgba(0,0,0,0.95)] backdrop-blur-2xl animate-scale-in">
          {/* Header */}
          <div className="flex items-center justify-between border-b border-[#26313d] px-4 py-3 bg-[#0e1216]">
            <div className="flex items-center gap-2">
              <span className="font-mono text-[10px] font-bold uppercase tracking-wider text-neutral-300">
                Notifications
              </span>
              {unreadCount > 0 && (
                <span className="rounded border border-[#ff7843]/40 bg-[#ff7843]/15 px-2 py-0.5 font-mono text-[9px] font-bold text-[#ff9769]">
                  {unreadCount} new
                </span>
              )}
            </div>

            <div className="flex items-center gap-2">
              {unreadCount > 0 && (
                <button
                  type="button"
                  onClick={handleMarkAllRead}
                  disabled={markingAll}
                  className="flex items-center gap-1 rounded border border-[#26313d] bg-[#161c23] px-2 py-1 font-mono text-[10px] font-semibold text-neutral-300 hover:text-white hover:border-[#384654] transition-colors"
                  title="Mark all as read"
                >
                  <CheckCheck size={12} />
                  <span>Mark all read</span>
                </button>
              )}
              <button
                type="button"
                onClick={() => setIsOpen(false)}
                className="text-neutral-500 hover:text-neutral-300 p-1 rounded hover:bg-[#1c2430]"
                aria-label="Close"
              >
                <X size={14} />
              </button>
            </div>
          </div>

          {/* List */}
          <div className="max-h-[380px] overflow-y-auto divide-y divide-[#222933] custom-scrollbar">
            {notifications.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-10 px-4 text-center">
                <div className="flex h-9 w-9 items-center justify-center rounded border border-[#26313d] bg-[#161c23] text-neutral-500 mb-2">
                  <Bell size={16} />
                </div>
                <p className="font-mono text-xs font-bold text-neutral-300 uppercase tracking-wider">All caught up!</p>
                <p className="text-[11px] text-neutral-500 mt-1 max-w-[220px]">
                  You&apos;ll be notified when proof is submitted or approved.
                </p>
              </div>
            ) : (
              notifications.map((n) => {
                const isProof = n.type === "proof_submitted";
                const isApproved =
                  n.type === "approval_decided" &&
                  (n.title?.toLowerCase().includes("approved") ||
                    n.message?.toLowerCase().includes("approved"));

                return (
                  <div
                    key={n.id}
                    onClick={() => handleItemClick(n)}
                    className={`group relative flex items-start gap-3 p-3.5 transition-colors cursor-pointer hover:bg-[#161c23] ${
                      !n.read ? "bg-[#182028]/80 border-l-2 border-[#ff7843]" : "bg-transparent opacity-85"
                    }`}
                  >
                    {/* Icon */}
                    <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded border border-[#2b3542] bg-[#10141a] shadow-sm">
                      {isApproved ? (
                        <Unlock size={13} className="text-[#10b981]" />
                      ) : isProof ? (
                        <FileText size={13} className="text-[#ff7843]" />
                      ) : (
                        <AlertCircle size={13} className="text-[#ff7843]" />
                      )}
                    </div>

                    {/* Content */}
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-2">
                        <p
                          className={`text-xs font-bold truncate ${
                            !n.read ? "text-white" : "text-neutral-300"
                          }`}
                        >
                          {n.title || (isProof ? "New Proof" : "Decision Update")}
                        </p>
                        <span className="shrink-0 font-mono text-[9px] text-neutral-500">
                          {formatRelativeTime(n.createdAt)}
                        </span>
                      </div>
                      <p className="mt-0.5 text-xs text-neutral-400 line-clamp-2 leading-relaxed">
                        {n.message}
                      </p>
                    </div>

                    {/* Unread indicator dot */}
                    {!n.read && (
                      <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[#ff7843] shadow-[0_0_6px_rgba(255,120,67,0.8)]" />
                    )}
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
}
