import { Link } from "react-router";
import { Star } from "lucide-react";
import { ArrowDownTrayIcon } from "@heroicons/react/24/outline";
import { AgentIdentity } from "./AgentIdentity";
import { PrivacyBadge } from "@/components/PrivacyBadge";

export interface AgentCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  visibility?: string;
  variant?: "default" | "oftenUsedTogether";
  rating?: number;
  installs?: number;
}

export function AgentCard({
  slug,
  account,
  name,
  description,
  visibility,
  variant = "default",
  rating,
  installs,
}: AgentCardProps) {
  const formattedInstalls = installs != null
    ? new Intl.NumberFormat("en-US").format(installs)
    : null;

  if (variant === "oftenUsedTogether") {
    return (
      <Link
        to={`/${slug}`}
        className="group flex items-start gap-3 overflow-hidden rounded-md border border-border-strong bg-stone-200 p-4 transition-all duration-150 hover:bg-stone-300 hover:border-teal-500 hover:shadow-md dark:bg-muted/30 dark:hover:border-teal-400 [@media(max-height:1000px)]:p-2.5"
      >
        <AgentIdentity
          account={account}
          name={name}
          size={36}
          className="size-9 shrink-0 rounded-sm overflow-hidden"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h3 className="truncate text-heading-4 text-foreground transition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
            {name}
          </h3>
          <p className="font-mono text-mono-sm text-muted-foreground">
            @{account}
          </p>
          {(rating != null || installs != null) && (
            <p className="inline-flex items-center gap-1.5 text-mono-sm font-mono text-faint-foreground">
              {rating != null && (
                <>
                  <Star className="h-3 w-3 fill-current text-yellow-500" />
                  <span>{rating.toFixed(1)}</span>
                </>
              )}
              {rating != null && installs != null && <span className="text-faint-foreground">·</span>}
              {installs != null && (
                <span className="inline-flex items-center gap-1">
                  <ArrowDownTrayIcon className="h-3.5 w-3.5" />
                  <span>{formattedInstalls}</span>
                </span>
              )}
            </p>
          )}
        </div>
      </Link>
    );
  }

  return (
    <Link
      to={`/${slug}`}
      className="group flex flex-col overflow-hidden rounded-md border border-stone-400 bg-stone-50 transition-all duration-150 hover:bg-stone-25 hover:border-teal-500 hover:shadow-md dark:bg-teal-900/30 dark:hover:border-teal-400"
    >
      <div className="flex flex-1 items-start gap-3 p-4 pb-3">
        <AgentIdentity
          account={account}
          name={name}
          size={36}
          className="size-9 shrink-0 rounded-sm overflow-hidden"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h3 className="flex flex-wrap items-center gap-1.5 text-heading-4 text-foregroundtransition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
            <span className="truncate">{name}</span>
            {visibility === "private" && (
              <PrivacyBadge onClick={(e) => e.preventDefault()} />
            )}
          </h3>
          <p className="line-clamp-3 text-body-sm text-muted-foreground">
            {description}
          </p>
        </div>
      </div>
      <div className="flex items-center justify-between border-t border-border px-4 py-2.5">
        <div className="flex items-center gap-1.5">
          <ArrowDownTrayIcon className="h-3.5 w-3.5 text-faint-foreground" />
          <span className="text-mono-sm font-mono text-faint-foreground">
            {formattedInstalls ?? "1.2K"}
          </span>
        </div>
        <span className="text-mono-sm font-mono text-faint-foreground">
          {account}
        </span>
      </div>
    </Link>
  );
}
