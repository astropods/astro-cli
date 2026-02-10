import { useState, useEffect, useCallback } from "react";
import { Outlet, useSearchParams, useNavigate } from "react-router-dom";
import { AppSidebar, SidebarInset } from "./AppSidebar";
import { SidebarProvider } from "@/components/ui/sidebar";
import { AuthModal } from "./AuthModal";
import { BreadcrumbHeader } from "./BreadcrumbHeader";
import { useAuth } from "../lib/auth";
import { useBreadcrumbs } from "@/hooks/use-breadcrumbs";

export function Layout() {
  const [isAuthModalOpen, setIsAuthModalOpen] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const { user, isLoading, isAuthenticated, login, logout, error } = useAuth();
  const navigate = useNavigate();
  const breadcrumbs = useBreadcrumbs();

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
        <BreadcrumbHeader
          breadcrumbs={breadcrumbs}
          onBack={() => navigate(-1)}
          onForward={() => navigate(1)}
        />
        {error && (
          <div className="m-6 mb-0 md:m-8 md:mb-0 p-3 bg-red-50 border border-red-200 text-red-700 text-sm rounded">
            {error}
          </div>
        )}
        <div className="flex-1 p-6 md:p-8">
          <Outlet context={{ openAuthModal }} />
        </div>
      </SidebarInset>
      <AuthModal isOpen={isAuthModalOpen} onClose={closeAuthModal} />
    </SidebarProvider>
  );
}

// Type for useOutletContext
export interface LayoutContext {
  openAuthModal: () => void;
}
