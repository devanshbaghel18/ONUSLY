import { Link, useLocation, useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  Target,
  Users,
  LogOut,
  X,
  Lock,
} from "lucide-react";
import { getUser, clearAuth } from "../lib/auth";

export default function Sidebar({ isOpen, onClose }) {
  const location = useLocation();
  const navigate = useNavigate();
  const user = getUser();

  const handleLogout = () => {
    clearAuth();
    navigate("/");
  };

  const navItems = [
    {
      name: "Dashboard",
      path: "/dashboard",
      icon: LayoutDashboard,
    },
    {
      name: "My Goals",
      path: "/goals",
      icon: Target,
    },
    {
      name: "Community & Friends",
      path: "/community-friends",
      icon: Users,
    },
  ];

  return (
    <>
      {/* Drawer Backdrop */}
      {isOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/80 backdrop-blur-sm transition-opacity"
          onClick={onClose}
        />
      )}

      {/* Hamburger Drawer */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 flex w-72 flex-col justify-between border-r border-[#333333] bg-[#141414] p-5 shadow-2xl transition-transform duration-300 ease-in-out ${
          isOpen ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        {/* Top Header */}
        <div>
          <div className="flex items-center justify-between border-b border-[#2B2B2B] pb-5">
            <Link
              to="/dashboard"
              className="flex items-center gap-2.5"
              onClick={onClose}
            >
              <div className="flex h-8 w-8 items-center justify-center rounded-lg border border-[#404040] bg-[#222222]">
                <Lock size={16} className="text-white" />
              </div>
              <img
                src="https://see.fontimg.com/api/rf5/2nxo/MWIyNjZmNmYzMzQ5NGUyYThiMjRlNTZkNzg0MDE2YWUudHRm/T05VU0xZ/ghang.png?r=fs&h=131&w=1250&fg=FFFFFF&bg=292929&tb=1&s=105"
                alt="ONUSLY"
                className="h-5 w-auto object-contain"
              />
            </Link>
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-[#333333] bg-[#1F1F1F] p-1.5 text-[#A3A3A3] transition-colors hover:border-white hover:text-white"
            >
              <X size={18} />
            </button>
          </div>

          {/* Navigation Links */}
          <nav className="mt-5 space-y-1.5">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive =
                location.pathname === item.path ||
                (item.path === "/goals" && location.pathname.startsWith("/goals/")) ||
                (item.path === "/community-friends" && location.pathname === "/communities");

              return (
                <Link
                  key={item.name}
                  to={item.path}
                  onClick={onClose}
                  className={`flex items-center gap-3 rounded-xl px-3.5 py-3 text-sm font-medium transition-all ${
                    isActive
                      ? "border border-white bg-white text-black font-semibold shadow-sm"
                      : "border border-transparent text-[#A3A3A3] hover:border-[#333333] hover:bg-[#1E1E1E] hover:text-white"
                  }`}
                >
                  <Icon
                    size={18}
                    className={isActive ? "text-black" : "text-[#737373]"}
                  />
                  <span>{item.name}</span>
                </Link>
              );
            })}
          </nav>
        </div>

        {/* Bottom User Profile Section (No Yellow box, No Email display) */}
        <div className="border-t border-[#2B2B2B] pt-4">
          <div className="flex items-center justify-between rounded-xl border border-[#333333] bg-[#1A1A1A] p-3">
            <div className="flex items-center gap-2.5 min-w-0">
              {user?.picture ? (
                <img
                  src={user.picture}
                  alt={user.name || "User"}
                  className="h-8 w-8 rounded-full border border-[#444444] object-cover"
                  referrerPolicy="no-referrer"
                />
              ) : (
                <div className="flex h-8 w-8 items-center justify-center rounded-full border border-[#444444] bg-[#2E2E2E] text-xs font-bold text-white">
                  {(user?.name?.[0] || "U").toUpperCase()}
                </div>
              )}
              <div className="min-w-0 flex-1 text-left">
                <p className="truncate text-xs font-semibold text-white">
                  {user?.name || "Account"}
                </p>
                <p className="truncate font-mono text-[11px] text-[#A3A3A3]">
                  {user?.handle ? `@${user.handle}` : "@tag"}
                </p>
              </div>
            </div>

            <button
              type="button"
              onClick={handleLogout}
              className="rounded-lg border border-[#333333] bg-[#222222] p-2 text-[#A3A3A3] transition-colors hover:border-white hover:text-white"
              title="Sign Out"
            >
              <LogOut size={15} />
            </button>
          </div>
        </div>
      </aside>
    </>
  );
}
