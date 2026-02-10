import { useState, useEffect, useCallback } from "react";
import { Outlet, useSearchParams } from "react-router-dom";
import { AppSidebar, SidebarInset } from "./AppSidebar";
import { SidebarProvider } from "@/components/ui/sidebar";
import { AuthModal } from "./AuthModal";
import { useAuth } from "../lib/auth";

export function Layout() {
  const [isAuthModalOpen, setIsAuthModalOpen] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const { user, isLoading, isAuthenticated, login, logout, error } = useAuth();

  // Handle error from auth callback
  useEffect(() => {
    const authError = searchParams.get("error");
    const errorDesc = searchParams.get("error_description");

    if (authError) {
      // Clear the error params from URL
      const newParams = new URLSearchParams(searchParams);
      newParams.delete("error");
      newParams.delete("error_description");
      setSearchParams(newParams, { replace: true });

      // Show error to user (you could use a toast notification here)
      console.error("Authentication error:", authError, errorDesc);
    }
  }, [searchParams, setSearchParams]);

  const openAuthModal = useCallback(() => setIsAuthModalOpen(true), []);
  const closeAuthModal = useCallback(() => setIsAuthModalOpen(false), []);

  return (
    <SidebarProvider>
      <AppSidebar
        user={user}
        isLoading={isLoading}
        isAuthenticated={isAuthenticated}
        onSignIn={login}
        onSignOut={logout}
      />
      <SidebarInset>
        {error && (
          <div className="m-6 mb-0 md:m-8 md:mb-0 p-3 bg-red-50 border border-red-200 text-red-700 text-sm rounded">
            {error}
          </div>
        )}
        <Outlet context={{ openAuthModal }} />
      </SidebarInset>
      <AuthModal isOpen={isAuthModalOpen} onClose={closeAuthModal} />
    </SidebarProvider>
  );
}

// Type for useOutletContext
export interface LayoutContext {
  openAuthModal: () => void;
}
