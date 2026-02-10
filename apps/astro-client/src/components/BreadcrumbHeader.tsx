import { Fragment } from "react";
import { Link } from "react-router-dom";
import {
  ChevronRightIcon,
  ShareIcon,
  XMarkIcon,
} from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { Separator } from "@/components/ui/separator";
import { Button } from "@/components/ui/button";
import { ContentHeader, type BreadcrumbItem } from "./ContentHeader";
import { HeaderNav, type HeaderNavProps } from "./HeaderNav";

export interface BreadcrumbHeaderProps extends HeaderNavProps {
  breadcrumbs?: BreadcrumbItem[];
  onShare?: () => void;
  onClose?: () => void;
  className?: string;
}

export function BreadcrumbHeader({
  breadcrumbs = [],
  onBack,
  onForward,
  onShare,
  onClose,
  canGoBack = true,
  canGoForward = true,
  className,
}: BreadcrumbHeaderProps) {
  return (
    <ContentHeader className={className}>
      {/* Breadcrumbs */}
      <nav className="flex flex-1 items-center gap-1.5 min-w-0 text-sm">
        {breadcrumbs.map((item, index) => {
          const isLast = index === breadcrumbs.length - 1;
          return (
            <Fragment key={index}>
              {index > 0 && (
                <ChevronRightIcon className="size-3.5 shrink-0 text-muted-foreground" />
              )}
              {isLast || !item.to ? (
                <span
                  className={cn(
                    "truncate",
                    isLast
                      ? "font-medium text-foreground"
                      : "text-muted-foreground",
                  )}
                >
                  {item.label}
                </span>
              ) : (
                <Link
                  to={item.to}
                  className="truncate text-muted-foreground hover:text-foreground transition-colors"
                >
                  {item.label}
                </Link>
              )}
            </Fragment>
          );
        })}
      </nav>

      {/* Navigation */}
      {(onBack || onForward) && (
        <HeaderNav
          onBack={onBack}
          onForward={onForward}
          canGoBack={canGoBack}
          canGoForward={canGoForward}
        />
      )}

      {/* Actions */}
      {(onShare || onClose) && (
        <>
          <Separator orientation="vertical" className="h-5" />
          {onShare && (
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onShare}
              aria-label="Share"
            >
              <ShareIcon className="size-4" />
            </Button>
          )}
          {onClose && (
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onClose}
              aria-label="Close"
            >
              <XMarkIcon className="size-4" />
            </Button>
          )}
        </>
      )}
    </ContentHeader>
  );
}
