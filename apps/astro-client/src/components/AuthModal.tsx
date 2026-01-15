import { X } from "lucide-react";

interface AuthModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function AuthModal({ isOpen, onClose }: AuthModalProps) {
  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={onClose}
    >
      <div
        className="bg-white border border-gray-300 p-6 w-full max-w-[380px] relative"
        onClick={(e) => e.stopPropagation()}
      >
        <button
          className="absolute top-3 right-3 bg-transparent border-none cursor-pointer p-1"
          onClick={onClose}
        >
          <X size={20} />
        </button>

        <div className="text-center mb-5">
          <h2 className="text-xl font-semibold mb-1">Welcome to Astro</h2>
          <p className="text-gray-600 text-sm">Create your account to get started</p>
        </div>

        <form className="flex flex-col" onSubmit={(e) => e.preventDefault()}>
          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="fullName" className="text-sm text-gray-600">
              Full Name
            </label>
            <input
              type="text"
              id="fullName"
              placeholder="Enter your full name"
              className="px-3 py-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="email" className="text-sm text-gray-600">
              Email
            </label>
            <input
              type="email"
              id="email"
              placeholder="Enter your email"
              className="px-3 py-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <div className="flex flex-col gap-1 mb-4">
            <label htmlFor="password" className="text-sm text-gray-600">
              Password
            </label>
            <input
              type="password"
              id="password"
              placeholder="Create a password"
              className="px-3 py-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
            />
          </div>

          <button
            type="submit"
            className="w-full px-4 py-2 bg-gray-800 text-white border border-gray-800 text-sm cursor-pointer hover:bg-gray-700"
          >
            Create Account
          </button>

          <p className="text-center text-xs text-gray-500 mt-3">
            By signing up, you agree to our{" "}
            <a href="/terms" className="text-gray-600 underline">
              Terms of Service
            </a>{" "}
            and{" "}
            <a href="/privacy" className="text-gray-600 underline">
              Privacy Policy
            </a>
            .
          </p>
        </form>
      </div>
    </div>
  );
}
