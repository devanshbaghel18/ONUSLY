import { useState, useEffect } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { Lock, LogOut } from "lucide-react";
import { getUser, clearAuth } from "../lib/auth";

export default function AppLayout({ children }) {
  const user = getUser();
  const location = useLocation();
  const navigate = useNavigate();

  // Scroll direction state for smart auto-hide slide-up
  const [isVisible, setIsVisible] = useState(true);
  const [lastScrollY, setLastScrollY] = useState(0);

  useEffect(() => {
    const handleScroll = () => {
      const currentScrollY = window.scrollY;

      if (currentScrollY < 30) {
        setIsVisible(true);
      } else if (currentScrollY > lastScrollY && currentScrollY > 60) {
        // User scrolling down -> slide up out of view
        setIsVisible(false);
      } else if (currentScrollY < lastScrollY) {
        // User scrolling up -> slide back down into view
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

  const navLinks = [
    { name: "Dashboard", path: "/dashboard" },
    { name: "My Goals", path: "/goals" },
    { name: "Community & Friends", path: "/community-friends" },
    { name: "Submit Proof", path: "/submit-proof" },
  ];

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
            {/* Left: ONUSLY Brand in Place of Hamburger */}
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

            {/* Right: White Rounded Pill with User Account & Sign Out */}
            <div className="flex items-center gap-2 pl-2">
              <div className="flex items-center gap-2 rounded-full bg-white px-4 py-2 text-xs font-bold text-black shadow-sm">
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
                <span className="max-w-[100px] truncate text-xs font-bold">
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
    </div>
  );
}
