import Brand from "./Brand";
import { useState, useEffect } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { LogOut, AtSign, Check, Copy, X, Menu, Volume2, VolumeX } from "lucide-react";
import { getUser, setAuth, clearAuth, updateUserHandle } from "../lib/auth";
import { updateHandle, getMyProfile } from "../lib/api";
import { notifyExtensionSync } from "../lib/extensionSync";
import { wsManager } from "../lib/websocket";
import { isSoundEnabled, toggleSound } from "../lib/sound";
import NotificationFeed from "./NotificationFeed";
import Sidebar from "./Sidebar";

export default function AppLayout({ children }) {
  const [user, setUser] = useState(() => getUser());
  const location = useLocation();
  const navigate = useNavigate();

  // Mobile drawer open state
  const [isSidebarOpen, setIsSidebarOpen] = useState(false);

  // Handle customization modal state
  const [showHandleModal, setShowHandleModal] = useState(false);
  const [customHandle, setCustomHandle] = useState("");
  const [handleSaving, setHandleSaving] = useState(false);
  const [handleError, setHandleError] = useState("");
  const [handleSuccess, setHandleSuccess] = useState("");
  const [copiedHandle, setCopiedHandle] = useState(false);

  // Sync latest user profile and handle on mount
  useEffect(() => {
    getMyProfile()
      .then((profile) => {
        if (profile?.handle) {
          const updated = updateUserHandle(profile.handle);
          setUser(updated);
        }
      })
      .catch(() => {});
  }, []);

  const handleLogout = () => {
    clearAuth();
    navigate("/");
  };

  const openHandleModal = () => {
    setCustomHandle(user?.handle || "");
    setHandleError("");
    setHandleSuccess("");
    setShowHandleModal(true);
  };

  const handleCopyHandle = (e) => {
    e?.stopPropagation();
    const tag = user?.handle ? `@${user.handle}` : "@user";
    navigator.clipboard.writeText(tag);
    setCopiedHandle(true);
    setTimeout(() => setCopiedHandle(false), 2000);
  };

  const handleSaveCustomHandle = async (e) => {
    e.preventDefault();
    const clean = customHandle.trim().replace(/^@/, "").toLowerCase();
    if (clean.length < 3 || clean.length > 20) {
      setHandleError("Handle must be between 3 and 20 characters");
      return;
    }

    setHandleSaving(true);
    setHandleError("");
    setHandleSuccess("");

    try {
      const res = await updateHandle(clean);
      const newHandle = res.handle || clean;
      const updated = updateUserHandle(newHandle);
      setUser(updated);
      if (res.token) {
        setAuth(res.token, updated);
        wsManager.disconnect();
        wsManager.connect();
      } else {
        wsManager.send({
          type: "user.online",
          payload: {
            email: updated.email || "",
            handle: newHandle,
          },
        });
      }
      setHandleSuccess("Handle updated successfully!");
      setTimeout(() => {
        setShowHandleModal(false);
      }, 1200);
    } catch (err) {
      setHandleError(err.message || "Failed to update handle");
    } finally {
      setHandleSaving(false);
    }
  };

  const [pendingApprovalsCount, setPendingApprovalsCount] = useState(0);
  const [pendingFriendReqsCount, setPendingFriendReqsCount] = useState(0);
  const [soundOn, setSoundOn] = useState(() => isSoundEnabled());

  const handleToggleSound = () => {
    const nextState = toggleSound();
    setSoundOn(nextState);
  };

  useEffect(() => {
    // Notify extension on every page navigation
    notifyExtensionSync();

    let mounted = true;
    const fetchPending = async () => {
      try {
        const { getApproverInbox, getFriendRequests } = await import("../lib/api");
        const [inboxRes, reqsRes] = await Promise.all([
          getApproverInbox({ status: "pending" }).catch(() => null),
          getFriendRequests().catch(() => null),
        ]);
        if (mounted && inboxRes?.total !== undefined) {
          setPendingApprovalsCount(inboxRes.total);
        }
        if (mounted && Array.isArray(reqsRes?.incoming)) {
          setPendingFriendReqsCount(reqsRes.incoming.length);
        }
      } catch {
        // Ignore transient pending count fetch errors
      }
    };

    fetchPending();

    const handleUpdate = () => fetchPending();
    window.addEventListener("onusly:inbox-updated", handleUpdate);

    // Subscribe to real-time WebSocket events for instant navbar badge synchronization
    const unsub = wsManager.subscribe((event) => {
      if (
        event?.type === "proof.submitted" ||
        event?.type === "goal.unlocked" ||
        event?.type === "goal.rejected" ||
        event?.type === "chat.message.approved" ||
        event?.type === "friend.request" ||
        event?.type === "friend.accepted" ||
        event?.type === "notification.new"
      ) {
        fetchPending();
      }
    });

    return () => {
      mounted = false;
      window.removeEventListener("onusly:inbox-updated", handleUpdate);
      if (typeof unsub === "function") unsub();
    };
  }, [location.pathname]);

  const navLinks = [
    { name: "Lock desk", path: "/dashboard" },
    { name: "My goals", path: "/goals" },
    { name: "Digital Wellbeing", path: "/wellbeing" },
    {
      name: "Review inbox",
      path: "/approvals",
      badge: pendingApprovalsCount,
    },
    {
      name: "Conversations",
      path: "/community-friends",
      badge: pendingFriendReqsCount,
    },
  ];

  const displayHandle = user?.handle
    ? (user.handle.startsWith("@") ? user.handle : `@${user.handle}`)
    : (user?.name ? `@${user.name.toLowerCase().replace(/\s+/g, "")}` : "@user");

  const isChatPage =
    location.pathname === "/community-friends" ||
    location.pathname === "/communities";

  return (
    <div className={`lock-workspace relative ${isChatPage ? "h-screen overflow-hidden" : "min-h-screen"} bg-[#0D0D0E] text-[#FFFFFF] antialiased selection:bg-white/20 selection:text-white`}>
      {/* Mobile Navigation Drawer */}
      <Sidebar isOpen={isSidebarOpen} onClose={() => setIsSidebarOpen(false)} />

      {/* Main Container */}
      <div className={`relative z-10 flex ${isChatPage ? "h-screen overflow-hidden" : "min-h-screen"} flex-col`}>
        <header className="workspace-nav">
          <div className="workspace-nav-inner">
            {/* Left: Mobile Menu Trigger + ONUSLY Brand */}
            <div className="flex items-center gap-2 sm:gap-3 shrink-0">
              {/* Mobile Hamburger Drawer Button */}
              <button
                type="button"
                onClick={() => setIsSidebarOpen(true)}
                className="flex h-9 w-9 sm:h-10 sm:w-10 items-center justify-center rounded-full border border-[#2D2D2D] bg-[#1E1E1E] text-neutral-300 transition-colors hover:border-white hover:text-white xl:hidden"
                aria-label="Open navigation menu"
                aria-expanded={isSidebarOpen}
              >
                <Menu size={18} />
              </button>

              <Link
                to="/dashboard"
                className="flex items-center pl-1 sm:pl-2 pr-2 sm:pr-4 py-1 transition-all hover:opacity-90 hover:scale-[1.02] shrink-0"
              >
                <Brand />
              </Link>
            </div>

            {/* Center: Navigation Links (Desktop / Tablet only, completely hidden on mobile, NO scrollbar slider) */}
            <nav className="hidden xl:flex items-center gap-2 lg:gap-3 overflow-hidden no-scrollbar py-0.5">
              {navLinks.map((link) => {
                const isActive =
                  location.pathname === link.path ||
                  (link.path === "/goals" && location.pathname.startsWith("/goals/")) ||
                  (link.path === "/community-friends" && location.pathname === "/communities");

                return (
                  <Link
                    aria-current={isActive ? "page" : undefined}
                    key={link.name}
                    to={link.path}
                    className={`whitespace-nowrap rounded-md px-3.5 sm:px-4 lg:px-5 py-2 text-xs font-semibold transition-all duration-150 flex items-center gap-2 ${
                      isActive
                        ? "bg-[#1f2732] text-white border border-[#384656] shadow-sm"
                        : "text-neutral-400 hover:text-white hover:bg-white/5"
                    }`}
                  >
                    <span>{link.name}</span>
                    {link.badge > 0 && (
                      <span className="flex h-4 min-w-[18px] items-center justify-center rounded bg-amber-500/20 px-1 text-[10px] font-mono font-bold text-amber-300 border border-amber-500/30">
                        {link.badge}
                      </span>
                    )}
                  </Link>
                );
              })}
            </nav>

            {/* Right: Notifications, User Name Badge & Sign Out */}
            <div className="flex items-center gap-2 sm:gap-3 shrink-0">
              {/* Sound Mute/Unmute Toggle Button */}
              <button
                type="button"
                onClick={handleToggleSound}
                className={`btn-premium flex h-8 w-8 sm:h-9 sm:w-9 items-center justify-center rounded border transition-colors shrink-0 ${
                  soundOn
                    ? "border-[#28323e] bg-[#141920] text-neutral-300 hover:border-[#445468] hover:text-white"
                    : "border-[#28323e] bg-[#141920] text-neutral-500 hover:border-[#445468] hover:text-neutral-300"
                }`}
                title={soundOn ? "Mute tactile lock sounds" : "Enable tactile lock sounds"}
                aria-label={soundOn ? "Mute sounds" : "Enable sounds"}
              >
                {soundOn ? <Volume2 size={15} /> : <VolumeX size={15} />}
              </button>

              {/* In-App Notifications Feed */}
              <NotificationFeed />

              {/* Clickable Username Pill */}
              <button
                type="button"
                onClick={openHandleModal}
                className="btn-premium flex items-center gap-1.5 rounded border border-[#2e3947] bg-[#141920] px-3 sm:px-4 py-2 text-xs font-semibold text-neutral-200 transition-all hover:border-[#445468] hover:bg-[#1a212b] shadow-sm max-w-[130px] sm:max-w-[200px] md:max-w-none"
                title="Click to customize your ONUSLY Tag / Handle"
              >
                <AtSign size={13} className="text-neutral-400 shrink-0" />
                <span className="font-mono tracking-tight truncate">{displayHandle}</span>
              </button>

              {/* Sign Out Button */}
              <button
                type="button"
                onClick={handleLogout}
                className="btn-premium flex h-8 w-8 sm:h-9 sm:w-9 items-center justify-center rounded border border-[#28323e] bg-[#141920] text-neutral-400 transition-colors hover:border-[#445468] hover:text-white shrink-0"
                title="Sign Out"
              >
                <LogOut size={15} />
              </button>
            </div>
          </div>
        </header>

        {/* Page Content with adjusted top padding for the taller floating navbar */}
        <main
          className={`workspace-content flex-1 px-3 sm:px-6 lg:px-8 flex flex-col min-h-0 ${
            isChatPage
              ? "pb-2 sm:pb-3 overflow-hidden"
              : "pb-12"
          }`}
        >
          {children}
        </main>
      </div>

      {/* ============================================================
          CUSTOMIZE HANDLE MODAL (100% Privacy Shield)
      ============================================================ */}
      {showHandleModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center modal-backdrop p-4">
          <div className="w-full max-w-md rounded-2xl border border-white/10 bg-[#1A1A1A] p-6 shadow-2xl animate-scale-in">
            <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
              <div>
                <h3 className="text-base font-bold text-white">
                  Your Public ONUSLY Tag
                </h3>
                <p className="mt-0.5 text-xs text-[#A3A3A3]">
                  Share this tag with friends to connect without revealing your Gmail.
                </p>
              </div>
              <button
                type="button"
                onClick={() => setShowHandleModal(false)}
                className="rounded-lg p-1 text-[#737373] hover:text-white"
              >
                <X size={18} />
              </button>
            </div>

            {/* Current Handle Tag Badge with Copy */}
            <div className="mt-5 flex items-center justify-between rounded-xl border border-[#333333] bg-[#121212] p-3.5">
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-white text-black font-bold text-xs">
                  @
                </div>
                <div>
                  <span className="text-[10px] uppercase font-bold tracking-wider text-[#737373]">
                    Active Tag
                  </span>
                  <p className="font-mono text-sm font-bold text-white">
                    {displayHandle}
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={handleCopyHandle}
                className="flex items-center gap-1.5 rounded-lg border border-[#3A3A3A] bg-[#222222] px-3 py-1.5 text-xs font-semibold text-white transition hover:border-white"
              >
                {copiedHandle ? <Check size={14} className="text-white" /> : <Copy size={14} />}
                <span>{copiedHandle ? "Copied" : "Copy"}</span>
              </button>
            </div>

            {handleError && (
              <div className="mt-4 rounded-xl border border-white/20 bg-white/10 p-3 text-xs text-white">
                {handleError}
              </div>
            )}

            {handleSuccess && (
              <div className="mt-4 rounded-xl border border-white/20 bg-white/10 p-3 text-xs text-white">
                {handleSuccess}
              </div>
            )}

            {/* Customize Form */}
            <form onSubmit={handleSaveCustomHandle} className="mt-4 space-y-4">
              <div>
                <label className="block text-xs font-semibold text-[#A3A3A3] mb-1">
                  Customize Handle
                </label>
                <div className="relative">
                  <span className="absolute left-3.5 top-2.5 font-mono text-xs text-[#737373]">
                    @
                  </span>
                  <input
                    type="text"
                    required
                    value={customHandle}
                    onChange={(e) => setCustomHandle(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ""))}
                    placeholder="e.g. himanshu_dev"
                    className="w-full rounded-xl border border-[#333333] bg-[#121212] pl-8 pr-3.5 py-2.5 text-xs font-mono text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>
                <p className="mt-1 text-[10px] text-[#737373]">
                  3-20 characters. Lowercase letters, numbers, and underscores only.
                </p>
              </div>

              <div className="flex justify-end gap-2 pt-3 border-t border-[#2A2A2A]">
                <button
                  type="button"
                  onClick={() => setShowHandleModal(false)}
                  className="rounded-xl border border-[#333333] px-4 py-2 text-xs font-semibold text-[#A3A3A3] hover:text-white"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={handleSaving}
                  className="rounded-xl bg-white px-5 py-2 text-xs font-bold text-black hover:bg-neutral-200 disabled:opacity-50"
                >
                  {handleSaving ? "Saving..." : "Save Custom Handle"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
