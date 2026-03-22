import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertCircle, RotateCcw } from "lucide-react";

interface Props {
  children: ReactNode;
  /** Optional fallback to render instead of the default error UI */
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

/**
 * Catches render errors in any descendant component and displays a
 * recovery UI instead of crashing the whole page.
 *
 * In development, the full component stack is shown to help debug.
 * In production, a user-friendly message is shown with a retry button.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null, errorInfo: null };

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    this.setState({ errorInfo });
    console.error("[ErrorBoundary]", error, errorInfo);
  }

  private handleRetry = () => {
    this.setState({ hasError: false, error: null, errorInfo: null });
  };

  render() {
    if (!this.state.hasError) return this.props.children;
    if (this.props.fallback) return this.props.fallback;

    const { error, errorInfo } = this.state;
    const isDev = import.meta.env.DEV;

    // Attempt to produce a friendlier message for common React errors
    const message = friendlyMessage(error) ?? error?.message ?? "Unknown error";

    return (
      <div className="m-6 md:m-8 flex flex-col items-center justify-center gap-4 py-16">
        <div className="flex items-center gap-2 text-red-700">
          <AlertCircle size={20} />
          <h2 className="text-lg font-semibold">Something went wrong</h2>
        </div>

        <p className="max-w-lg text-center text-sm text-stone-600">{message}</p>

        <button
          type="button"
          onClick={this.handleRetry}
          className="mt-2 flex items-center gap-1.5 rounded-md bg-stone-900 px-4 py-2 text-sm font-medium text-white hover:bg-stone-800 transition-colors"
        >
          <RotateCcw size={14} />
          Try again
        </button>

        {isDev && errorInfo && (
          <details className="mt-4 w-full max-w-2xl rounded-md border border-stone-200 bg-stone-50 p-4 text-xs">
            <summary className="cursor-pointer font-medium text-stone-700">
              Component stack (dev only)
            </summary>
            <pre className="mt-2 overflow-x-auto whitespace-pre-wrap text-stone-500">
              {error?.stack}
              {"\n\nComponent Stack:"}
              {errorInfo.componentStack}
            </pre>
          </details>
        )}
      </div>
    );
  }
}

/** Map known minified React errors to human-readable messages. */
function friendlyMessage(error: Error | null): string | null {
  if (!error) return null;
  const msg = error.message;

  // React error #31: Objects are not valid as a React child
  if (msg.includes("Minified React error #31") || msg.includes("Objects are not valid as a React child")) {
    return "A component tried to render a data object instead of text. This usually means a variable holding an object (like an API response) was placed directly in JSX instead of accessing a string property on it.";
  }

  // React error #130: Element type is invalid
  if (msg.includes("Minified React error #130") || msg.includes("Element type is invalid")) {
    return "A component import resolved to undefined. Check that the component is exported correctly and the import path is right.";
  }

  // React error #152: Nothing was returned from render
  if (msg.includes("Minified React error #152") || msg.includes("Nothing was returned from render")) {
    return "A component returned undefined instead of JSX. Make sure every branch of the component returns valid JSX or null.";
  }

  // React error #185: Maximum update depth exceeded
  if (msg.includes("Minified React error #185") || msg.includes("Maximum update depth exceeded")) {
    return "Infinite re-render loop detected. A state update is triggering itself repeatedly — check useEffect dependencies and event handlers.";
  }

  return null;
}
