import { Link } from "react-router";
import { ChevronRight, Heart, Share2 } from "lucide-react";
import { Button } from "@/components/ui/button";

export interface AgentDetailBreadcrumbProps {
  account: string;
  agentName: string;
}

export function AgentDetailBreadcrumb({
  account,
  agentName,
}: AgentDetailBreadcrumbProps) {
  const handleShare = () => {
    navigator.clipboard.writeText(window.location.href);
  };

  return (
    <div className="sticky top-0 z-10 flex items-center justify-between px-6 py-3 bg-white border-b border-border">
      <div className="flex items-center gap-2 text-sm text-stone-500">
        <Link
          to="/hire"
          className="hover:text-foreground transition-colors"
        >
          Browse Agents
        </Link>
        <ChevronRight className="h-3.5 w-3.5 text-stone-400" />
        <span className="text-foreground font-medium">
          {account} <span className="text-stone-400">/</span> {agentName}
        </span>
      </div>
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon-sm"
          disabled
          className="text-stone-400"
        >
          <Heart className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={handleShare}
        >
          <Share2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
