import type { ComponentProps, CSSProperties } from "react";
import { Toaster as SonnerToaster } from "sonner";
import { useResolvedTheme } from "@/lib/theme";

type ToasterProps = ComponentProps<typeof SonnerToaster>;

/** App-wide toast renderer. Wraps sonner, driving its surface colors off our
 *  semantic theme tokens and syncing its light/dark mode to the app's resolved
 *  theme (which is class-based, not OS `prefers-color-scheme`). Callers fire
 *  toasts with sonner's `toast` API; this owns the presentation. */
export function Toaster(props: ToasterProps) {
  const theme = useResolvedTheme();
  return (
    <SonnerToaster
      theme={theme}
      position="bottom-right"
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--foreground)",
          "--normal-border": "var(--border)",
        } as CSSProperties
      }
      toastOptions={{
        classNames: {
          toast: "rounded-lg shadow-lg",
          title: "text-body-sm font-medium",
          description: "text-body-sm text-muted-foreground",
          actionButton: "bg-primary text-primary-foreground",
          cancelButton: "bg-muted text-muted-foreground",
          closeButton: "text-muted-foreground",
          success: "[&_[data-icon]]:text-success",
          error: "[&_[data-icon]]:text-destructive",
          info: "[&_[data-icon]]:text-foreground-accent",
        },
      }}
      {...props}
    />
  );
}
