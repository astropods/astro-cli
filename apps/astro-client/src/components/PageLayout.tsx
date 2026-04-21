import type { CSSProperties, ReactNode } from "react";
import { cn } from "@/lib/utils";

interface PageContainerProps {
  children: ReactNode;
  className?: string;
  /**
   * Classes applied to the outer full-bleed wrapper (the one that gets
   * `flex-1 bg-muted`). Use this for page-level background treatments
   * like the Agents empty-state gradient.
   */
  outerClassName?: string;
  /**
   * Inline style on the outer wrapper. Reserved for runtime-computed
   * values Tailwind can't express (e.g. `color-mix()` gradients).
   */
  style?: CSSProperties;
}

export function PageContainer({ children, className, outerClassName, style }: PageContainerProps) {
  return (
    <div className={cn("flex-1 bg-muted", outerClassName)} style={style}>
      <div
        className={cn(
          "@container w-full mx-auto max-w-[1500px] px-6 pb-6 pt-6 md:px-8 md:pb-8 md:pt-8",
          className,
        )}
      >
        {children}
      </div>
    </div>
  );
}

interface PageHeaderProps {
  title: string;
  description?: string;
  /** Rendered inline to the right of the title (e.g. scope switcher). */
  adornment?: ReactNode;
  /** Rendered on the right of the header row (e.g. primary CTA). */
  action?: ReactNode;
  className?: string;
}

export function PageHeader({ title, description, adornment, action, className }: PageHeaderProps) {
  return (
    <div className={cn("mb-6 flex flex-wrap items-start justify-between gap-3", className)}>
      <div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <h1 className="text-heading-1 text-foreground">{title}</h1>
          {adornment}
        </div>
        {description && (
          <p className="mt-1 text-[13px] text-muted-foreground">{description}</p>
        )}
      </div>
      {action}
    </div>
  );
}
