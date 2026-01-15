import { useState } from "react";
import { Outlet } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { AuthModal } from "./AuthModal";

export function Layout() {
  const [isAuthModalOpen, setIsAuthModalOpen] = useState(false);

  return (
    <div className="flex min-h-screen">
      <Sidebar onSignInClick={() => setIsAuthModalOpen(true)} />
      <main className="flex-1 ml-[220px] p-6 md:p-8">
        <Outlet context={{ openAuthModal: () => setIsAuthModalOpen(true) }} />
      </main>
      <AuthModal
        isOpen={isAuthModalOpen}
        onClose={() => setIsAuthModalOpen(false)}
      />
    </div>
  );
}
