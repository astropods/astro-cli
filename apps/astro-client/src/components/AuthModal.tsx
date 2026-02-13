import { X, Sparkles, Shield } from "lucide-react";
import { useAuth } from "../lib/auth";

interface AuthModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function AuthModal({ isOpen, onClose }: AuthModalProps) {
  const { login, isLoading } = useAuth();

  if (!isOpen) return null;

  const handleSignIn = () => {
    login();
  };

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={onClose}
    >
      <div
        className="bg-white border border-stone-300 p-6 w-full max-w-[380px] relative"
        onClick={(e) => e.stopPropagation()}
      >
        <button
          className="absolute top-3 right-3 bg-transparent border-none cursor-pointer p-1 hover:bg-stone-100 rounded"
          onClick={onClose}
          aria-label="Close"
        >
          <X size={20} />
        </button>

        <div className="text-center mb-6">
          <div className="inline-flex items-center justify-center w-12 h-12 bg-stone-100 rounded-full mb-4">
            <Sparkles size={24} className="text-stone-800" />
          </div>
          <h2 className="text-xl font-semibold mb-2">Welcome to Astro</h2>
          <p className="text-stone-600 text-sm">
            Sign in to deploy and manage your AI agents
          </p>
        </div>

        <div className="flex flex-col gap-4">
          <button
            onClick={handleSignIn}
            disabled={isLoading}
            className="w-full px-4 py-3 bg-stone-800 text-white border border-stone-800 text-sm cursor-pointer hover:bg-stone-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
          >
            {isLoading ? (
              "Redirecting..."
            ) : (
              <>
                <Shield size={18} />
                Continue with WorkOS
              </>
            )}
          </button>

          <div className="flex items-center gap-3">
            <div className="flex-1 h-px bg-stone-200" />
            <span className="text-xs text-stone-500">Secure authentication</span>
            <div className="flex-1 h-px bg-stone-200" />
          </div>

          <p className="text-center text-xs text-stone-500">
            By signing in, you agree to our{" "}
            <a href="/terms" className="text-stone-600 underline hover:text-stone-800">
              Terms of Service
            </a>{" "}
            and{" "}
            <a href="/privacy" className="text-stone-600 underline hover:text-stone-800">
              Privacy Policy
            </a>
            .
          </p>
        </div>
      </div>
    </div>
  );
}
