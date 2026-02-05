import { NavLink } from "react-router-dom";
import {
  Home,
  Users,
  Briefcase,
  Sparkles,
  LogIn,
  LogOut,
  HelpCircle,
  Loader2,
  Wrench,
} from "lucide-react";
import { useAuth, getUserDisplayName, getUserInitials } from "../lib/auth";

export function Sidebar() {
  const { user, isLoading, isAuthenticated, login, logout } = useAuth();

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `flex items-center gap-2 px-3 py-2 text-sm text-gray-700 no-underline ${isActive ? "bg-gray-100" : "hover:bg-gray-50"}`;

  return (
    <aside className="fixed top-0 left-0 w-[220px] h-screen bg-white border-r border-gray-300 flex flex-col p-4">
      <div className="pb-4 border-b border-gray-300 mb-4">
        <div className="flex items-center gap-2">
          <Sparkles size={20} />
          <span className="text-lg font-bold">Astro</span>
        </div>
      </div>

      <nav className="flex flex-col gap-1 flex-1">
        <NavLink to="/" className={linkClass} end>
          <Home size={18} />
          <span>Home</span>
        </NavLink>

        <NavLink to="/hire" className={linkClass}>
          <Briefcase size={18} />
          <span>Hire agents</span>
        </NavLink>

        <NavLink to="/agents" className={linkClass}>
          <Users size={18} />
          <span>Your agents</span>
        </NavLink>

        <NavLink to="/operator" className={linkClass}>
          <Wrench size={18} />
          <span>Operator</span>
        </NavLink>

        <NavLink
          to="/hire?start=true"
          className="flex items-center gap-2 px-3 py-2 text-sm text-gray-700 no-underline border border-gray-300 mt-3 hover:bg-gray-50"
        >
          <Sparkles size={18} />
          <span>Find me agents</span>
        </NavLink>
      </nav>

      <div className="border-t border-gray-300 pt-3 flex flex-col gap-1">
        {isLoading ? (
          <div className="flex items-center gap-2 px-3 py-2 text-sm text-gray-500">
            <Loader2 size={18} className="animate-spin" />
            <span>Loading...</span>
          </div>
        ) : isAuthenticated && user ? (
          <>
            {/* User info */}
            <div className="flex items-center gap-2 px-3 py-2 mb-1">
              {user.profile_picture_url ? (
                <img
                  src={user.profile_picture_url}
                  alt={getUserDisplayName(user)}
                  className="w-8 h-8 rounded-full object-cover"
                />
              ) : (
                <div className="w-8 h-8 rounded-full bg-gray-800 text-white flex items-center justify-center text-xs font-medium">
                  {getUserInitials(user)}
                </div>
              )}
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium text-gray-900 truncate">
                  {getUserDisplayName(user)}
                </div>
                <div className="text-xs text-gray-500 truncate">
                  {user.email}
                </div>
              </div>
            </div>

            {/* Sign out button */}
            <button
              onClick={logout}
              className="flex items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 w-full text-left bg-transparent border-none cursor-pointer"
            >
              <LogOut size={18} />
              <span>Sign Out</span>
            </button>
          </>
        ) : (
          <button
            onClick={login}
            className="flex items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 w-full text-left bg-transparent border-none cursor-pointer"
          >
            <LogIn size={18} />
            <span>Sign In</span>
          </button>
        )}

        <NavLink to="/support" className={linkClass}>
          <HelpCircle size={18} />
          <span>Get support</span>
        </NavLink>
      </div>
    </aside>
  );
}
