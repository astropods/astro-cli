import { useState, useEffect, useCallback } from "react";
import { Outlet, useSearchParams } from "react-router";
import { AuthModal } from "./AuthModal";
import { AppHeader } from "./AppHeader";
import { useAuth } from "../lib/auth";

export default function Layout() {
  const [isAuthModalOpen, setIsAuthModalOpen] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const { error } = useAuth();

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
    <>
      <AppHeader />
      {error && (
        <div className="m-6 mb-0 md:m-8 md:mb-0 p-3 bg-red-50 border border-red-200 text-red-700 text-sm rounded">
          {error}
        </div>
      )}
      <Outlet context={{ openAuthModal }} />
      <AuthModal isOpen={isAuthModalOpen} onClose={closeAuthModal} />
    </>
  );
}

// Type for useOutletContext
export interface LayoutContext {
  openAuthModal: () => void;
}
