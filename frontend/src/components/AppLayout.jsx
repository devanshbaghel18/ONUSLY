import { useState, useEffect } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { LogOut, AtSign, Check, Copy, X, Menu } from "lucide-react";
import { getUser, clearAuth, updateUserHandle } from "../lib/auth";
import { updateHandle, getMyProfile } from "../lib/api";
import { wsManager } from "../lib/websocket";
import NotificationFeed from "./NotificationFeed";
import Sidebar from "./Sidebar";

export default function AppLayout({ children }) {
  const [user, setUser] = useState(() => getUser());
  const location = useLocation();
  const navigate = useNavigate();

  // Mobile drawer open state
  const [isSidebarOpen, setIsSidebarOpen] = useState(false);

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

  const displayHandle = user?.handle
    ? (user.handle.startsWith("@") ? user.handle : `@${user.handle}`)
    : (user?.name ? `@${user.name.toLowerCase().replace(/\s+/g, "")}` : "@user");

  const isChatPage =
    location.pathname === "/community-friends" ||
    location.pathname === "/communities";

  return (
    <div className={`relative ${isChatPage ? "h-screen overflow-hidden" : "min-h-screen"} bg-[#0D0D0E] text-[#FFFFFF] antialiased selection:bg-white/20 selection:text-white`}>
      {/* Ambient Top Glow Mesh */}
      <div className="ambient-mesh" />

      {/* Subtle monochrome micro dot grid */}
      <div
        className="pointer-events-none fixed inset-0 opacity-[0.035]"
        style={{
          backgroundImage: "radial-gradient(#ffffff 1px, transparent 1px)",
          backgroundSize: "24px 24px",
        }}
      />

      {/* Mobile Navigation Drawer */}
      <Sidebar isOpen={isSidebarOpen} onClose={() => setIsSidebarOpen(false)} />

      {/* Main Container */}
      <div className={`relative z-10 flex ${isChatPage ? "h-screen overflow-hidden" : "min-h-screen"} flex-col`}>
        {/* Floating Centered Pill Navbar with Auto-Hide Slide-Up */}
        <header
          className={`fixed top-3 sm:top-5 left-0 right-0 z-40 mx-auto w-[98%] 2xl:w-[96%] max-w-[1720px] px-1 sm:px-4 transition-all duration-300 ease-in-out ${
            isVisible
              ? "translate-y-0 opacity-100"
              : "-translate-y-28 opacity-0 pointer-events-none"
          }`}
        >
          <div className="flex items-center justify-between rounded-full border border-white/[0.12] bg-[#141416]/85 px-4 sm:px-8 lg:px-10 py-2.5 sm:py-3 lg:py-3.5 shadow-[0_20px_50px_rgba(0,0,0,0.85),inset_0_1px_0_0_rgba(255,255,255,0.1)] backdrop-blur-2xl transition-all">
            {/* Left: Mobile Menu Trigger + ONUSLY Brand */}
            <div className="flex items-center gap-2 sm:gap-3 shrink-0">
              {/* Mobile Hamburger Drawer Button */}
              <button
                type="button"
                onClick={() => setIsSidebarOpen(true)}
                className="flex h-9 w-9 sm:h-10 sm:w-10 items-center justify-center rounded-full border border-[#2D2D2D] bg-[#1E1E1E] text-neutral-300 transition-colors hover:border-white hover:text-white md:hidden"
                aria-label="Open Navigation Menu"
              >
                <Menu size={18} />
              </button>

              <Link
                to="/dashboard"
                className="flex items-center pl-1 sm:pl-2 pr-2 sm:pr-4 py-1 transition-all hover:opacity-90 hover:scale-[1.02] shrink-0"
              >
                <img
                  src="https://see.fontimg.com/api/rf5/2nxo/MWIyNjZmNmYzMzQ5NGUyYThiMjRlNTZkNzg0MDE2YWUudHRm/T05VU0xZ/ghang.png?r=fs&h=131&w=1250&fg=FFFFFF&bg=292929&tb=1&s=105"
                  alt="ONUSLY"
                  className="h-6 sm:h-7 md:h-8 lg:h-9 w-auto object-contain"
                />
              </Link>
            </div>

            {/* Center: Navigation Links (Desktop / Tablet only, completely hidden on mobile, NO scrollbar slider) */}
            <nav className="hidden md:flex items-center gap-2 lg:gap-3 overflow-hidden no-scrollbar py-0.5">
              {navLinks.map((link) => {
                const isActive =
                  location.pathname === link.path ||
                  (link.path === "/goals" && location.pathname.startsWith("/goals/")) ||
                  (link.path === "/community-friends" && location.pathname === "/communities");

                return (
                  <Link
                    key={link.name}
                    to={link.path}
                    className={`whitespace-nowrap rounded-full px-4 sm:px-5 lg:px-6 py-2.5 sm:py-3 text-xs sm:text-sm font-semibold transition-all duration-200 ${
                      isActive
                        ? "bg-white/10 text-white shadow-[0_0_15px_rgba(255,255,255,0.06)] border border-white/20"
                        : "text-[#888888] hover:text-white hover:bg-white/5"
                    }`}
                  >
                    {link.name}
                  </Link>
                );
              })}
            </nav>

            {/* Right: Notifications, User Name Badge & Sign Out */}
            <div className="flex items-center gap-2 sm:gap-3 lg:gap-4 shrink-0">
              {/* In-App Notifications Feed */}
              <NotificationFeed />

              {/* Clickable Username Pill (responsive on mobile, no overflow, no white background) */}
              <button
                type="button"
                onClick={openHandleModal}
                className="btn-premium flex items-center gap-1.5 sm:gap-2 rounded-full border border-[#333333] bg-[#1E1E1E] px-3 sm:px-5 lg:px-6 py-2 sm:py-2.5 lg:py-3 text-xs sm:text-sm font-semibold text-neutral-200 transition-all hover:border-white/50 hover:bg-[#252525] shadow-sm max-w-[130px] sm:max-w-[200px] md:max-w-none"
                title="Click to customize your ONUSLY Tag / Handle"
              >
                <AtSign size={14} className="text-neutral-300 shrink-0" />
                <span className="font-mono tracking-tight truncate">{displayHandle}</span>
              </button>

              {/* Sign Out Button */}
              <button
                type="button"
                onClick={handleLogout}
                className="btn-premium flex h-9 w-9 sm:h-10 sm:w-10 lg:h-11 lg:w-11 items-center justify-center rounded-full border border-[#2D2D2D] bg-[#1E1E1E] text-[#888888] transition-colors hover:border-white/40 hover:text-white shrink-0"
                title="Sign Out"
              >
                <LogOut size={17} />
              </button>
            </div>
          </div>
        </header>

        {/* Page Content with adjusted top padding for the taller floating navbar */}
        <main
          className={`flex-1 px-3 sm:px-6 lg:px-8 animate-fade-in flex flex-col min-h-0 ${
            isChatPage
              ? "pt-20 sm:pt-24 pb-2 sm:pb-3 overflow-hidden"
              : "pt-28 sm:pt-32 md:pt-36 pb-12"
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
