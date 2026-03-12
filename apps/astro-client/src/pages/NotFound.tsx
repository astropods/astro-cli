import { Link } from "react-router";

export default function NotFound() {
  return (
    <div className="flex items-center justify-center flex-1">
      <div className="text-center">
        <h1 className="text-7xl font-extrabold mb-2">404</h1>
        <p className="text-xl font-semibold mb-2">Oops! Page not found</p>
        <p className="text-stone-600 text-sm mb-6">
          The page you're looking for doesn't exist or has been moved.
        </p>
        <Link
          to="/"
          className="inline-block px-4 py-2 bg-stone-800 text-white border border-stone-800 text-sm no-underline"
        >
          Return to Home
        </Link>
      </div>
    </div>
  );
}
