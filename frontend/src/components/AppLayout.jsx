import { useState, useEffect } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { Lock, LogOut, AtSign, Check, Copy, X } from "lucide-react";
import { getUser, clearAuth, updateUserHandle } from "../lib/auth";
import { updateHandle, getMyProfile } from "../lib/api";
import { wsManager } from "../lib/websocket";
import NotificationFeed from "./NotificationFeed";

export default function AppLayout({ children }) {
  const [user, setUser] = useState(() => getUser());
  const location = useLocation();
  const navigate = useNavigate();

  // Scroll direction state for smart auto-hide slide-up
  const [isVisible, setIsVisible] = useState(true);
  const [lastScrollY, setLastScrollY] = useState(0);

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

  useEffect(() => {
    const handleScroll = () => {
      const currentScrollY = window.scrollY;

      if (currentScrollY < 30) {
        setIsVisible(true);
      } else if (currentScrollY > lastScrollY && currentScrollY > 60) {
        setIsVisible(false);
      } else if (currentScrollY < lastScrollY) {
        setIsVisible(true);
      }

      setLastScrollY(currentScrollY);
    };

    window.addEventListener("scroll", handleScroll, { passive: true });
    return () => window.removeEventListener("scroll", handleScroll);
  }, [lastScrollY]);

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
      wsManager.send({
        type: "user.online",
        payload: {
          email: updated.email || "",
          handle: newHandle,
        },
      });
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

  const navLinks = [
    { name: "Dashboard", path: "/dashboard" },
    { name: "My Goals", path: "/goals" },
    { name: "Community & Friends", path: "/community-friends" },
  ];

  const displayHandle = user?.handle ? `@${user.handle}` : "@tag";

  return (
    <div className="relative min-h-screen bg-[#121212] text-[#FFFFFF] antialiased">
      {/* Subtle monochrome background grid */}
      <div
        className="pointer-events-none fixed inset-0 opacity-[0.03]"
        style={{
          backgroundImage: "radial-gradient(#ffffff 1px, transparent 1px)",
          backgroundSize: "8px 8px",
        }}
      />

      {/* Main Container */}
      <div className="relative z-10 flex min-h-screen flex-col">
        {/* Floating Centered Pill Navbar with Auto-Hide Slide-Up */}
        <header
          className={`fixed top-4 left-0 right-0 z-40 mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8 transition-all duration-300 ease-in-out ${
            isVisible
              ? "translate-y-0 opacity-100"
              : "-translate-y-28 opacity-0 pointer-events-none"
          }`}
        >
          <div className="flex items-center justify-between rounded-full border border-[#2D2D2D] bg-[#161616]/95 p-2 shadow-[0_12px_40px_rgba(0,0,0,0.7)] backdrop-blur-xl">
            {/* Left: ONUSLY Brand */}
            <Link
              to="/dashboard"
              className="flex items-center gap-2.5 rounded-full pl-1 pr-3 py-1 transition-opacity hover:opacity-90"
            >
              <div className="flex h-9 w-9 items-center justify-center rounded-full bg-white text-black shadow-sm">
                <Lock size={16} className="text-black stroke-[2.5]" />
              </div>
              <img
                src="https://see.fontimg.com/api/rf5/2nxo/MWIyNjZmNmYzMzQ5NGUyYThiMjRlNTZkNzg0MDE2YWUudHRm/T05VU0xZ/ghang.png?r=fs&h=131&w=1250&fg=FFFFFF&bg=292929&tb=1&s=105"
                alt="ONUSLY"
                className="h-5 w-auto object-contain"
              />
            </Link>

            {/* Center: Navigation Links */}
            <nav className="flex items-center gap-1 sm:gap-2 overflow-x-auto py-0.5">
              {navLinks.map((link) => {
                const isActive =
                  location.pathname === link.path ||
                  (link.path === "/goals" && location.pathname.startsWith("/goals/")) ||
                  (link.path === "/community-friends" && location.pathname === "/communities");

                return (
                  <Link
                    key={link.name}
                    to={link.path}
                    className={`whitespace-nowrap rounded-full px-4 py-2 text-xs font-semibold transition-all ${
                      isActive
                        ? "bg-[#252525] text-white shadow-inner border border-white/20"
                        : "text-[#888888] hover:text-white hover:bg-[#1E1E1E]"
                    }`}
                  >
                    {link.name}
                  </Link>
                );
              })}
            </nav>

            {/* Right: Unique @Handle Badge & User Pill & Sign Out */}
            <div className="flex items-center gap-2 pl-2">
              {/* Clickable @Handle Badge */}
              <button
                type="button"
                onClick={openHandleModal}
                className="hidden sm:flex items-center gap-1.5 rounded-full border border-[#333333] bg-[#202020] px-3 py-1.5 text-[11px] font-bold text-neutral-200 transition-all hover:border-white hover:bg-[#282828]"
                title="Click to customize your ONUSLY Tag / Handle"
              >
                <AtSign size={13} className="text-white" />
                <span className="font-mono">{displayHandle}</span>
              </button>

              {/* In-App Notifications Feed */}
              <NotificationFeed />

              <div
                onClick={openHandleModal}
                className="flex cursor-pointer items-center gap-2 rounded-full bg-white px-3.5 py-1.5 text-xs font-bold text-black shadow-sm transition-transform hover:scale-[1.02]"
                title="Account Profile & Tag Settings"
              >
                {user?.picture ? (
                  <img
                    src={user.picture}
                    alt={user.name || "User"}
                    className="h-5 w-5 rounded-full object-cover"
                    referrerPolicy="no-referrer"
                  />
                ) : (
                  <div className="flex h-5 w-5 items-center justify-center rounded-full bg-black text-[9px] font-bold text-white">
                    {(user?.name?.[0] || "U").toUpperCase()}
                  </div>
                )}
                <span className="max-w-[85px] truncate text-xs font-bold">
                  {user?.name?.split(" ")?.[0] || "Account"}
                </span>
              </div>

              <button
                type="button"
                onClick={handleLogout}
                className="flex h-9 w-9 items-center justify-center rounded-full border border-[#2D2D2D] bg-[#1E1E1E] text-[#888888] transition-colors hover:border-white hover:text-white"
                title="Sign Out"
              >
                <LogOut size={15} />
              </button>
            </div>
          </div>
        </header>

        {/* Page Content with top padding for floating navbar */}
        <main className="flex-1 px-4 pt-24 pb-8 sm:px-6 sm:pt-28 lg:px-8">
          {children}
        </main>
      </div>

      {/* ============================================================
          CUSTOMIZE HANDLE MODAL (100% Privacy Shield)
      ============================================================ */}
      {showHandleModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl">
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
                {copiedHandle ? <Check size={14} className="text-green-400" /> : <Copy size={14} />}
                <span>{copiedHandle ? "Copied" : "Copy"}</span>
              </button>
            </div>

            {/* Privacy Shield Info */}
            <div className="mt-4 rounded-xl border border-[#2B2B2B] bg-[#161616] p-3 text-[11px] text-[#A3A3A3] leading-relaxed">
              <strong className="text-white">100% Privacy Protected:</strong> Other users only see your public handle ({displayHandle}) across chat, friend lists, and goal approvals. Your personal Google email address remains completely private.
            </div>

            {handleError && (
              <div className="mt-4 rounded-xl border border-red-900/50 bg-red-950/40 p-3 text-xs text-red-300">
                {handleError}
              </div>
            )}

            {handleSuccess && (
              <div className="mt-4 rounded-xl border border-green-900/50 bg-green-950/40 p-3 text-xs text-green-300">
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
