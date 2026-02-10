import { ChevronLeftIcon, ChevronRightIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";

export interface HeaderNavProps {
  onBack?: () => void;
  onForward?: () => void;
  canGoBack?: boolean;
  canGoForward?: boolean;
}

export function HeaderNav({
  onBack,
  onForward,
  canGoBack = true,
  canGoForward = true,
}: HeaderNavProps) {
  return (
    <>
      <Button
        variant="ghost"
        size="icon-xs"
        onClick={onBack}
        disabled={!canGoBack}
        aria-label="Go back"
      >
        <ChevronLeftIcon className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        onClick={onForward}
        disabled={!canGoForward}
        aria-label="Go forward"
      >
        <ChevronRightIcon className="size-4" />
      </Button>
    </>
  );
}
