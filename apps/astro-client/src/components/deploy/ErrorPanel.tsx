import type { ReactNode } from "react";
import { AlertCircle } from "lucide-react";

export interface ErrorPanelProps {
  /** Heading shown next to the icon */
  title?: string;
  /** Error message body */
  children: ReactNode;
}

export function ErrorPanel({ title, children }: ErrorPanelProps) {
  return (
    <div className="rounded-[6px] bg-red-100 p-4">
      {title && (
        <div className="flex items-center gap-1.5 mb-2">
          <AlertCircle size={16} className="text-red-700" />
          <span className="text-sm font-medium text-red-700">{title}</span>
        </div>
      )}
      <p className="text-sm text-red-700 whitespace-pre-wrap">{children}</p>
    </div>
  );
}
