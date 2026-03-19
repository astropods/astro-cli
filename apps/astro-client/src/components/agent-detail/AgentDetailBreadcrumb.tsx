import { useState } from "react";
import { Check } from "lucide-react";
import { HeartIcon as HeartOutline, ShareIcon } from "@heroicons/react/24/outline";
import { HeartIcon as HeartSolid } from "@heroicons/react/24/solid";
import { Button } from "@/components/ui/button";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { useToggleHeart } from "@/api/queries/hearts";

export interface AgentDetailBreadcrumbProps {
  account: string;
  agentName: string;
  hearted?: boolean;
  heartCount?: number;
}

export function AgentDetailBreadcrumb({
  account,
  agentName,
  hearted: initialHearted = false,
  heartCount: initialHeartCount = 0,
}: AgentDetailBreadcrumbProps) {
  const [copied, setCopied] = useState(false);
  const toggleHeart = useToggleHeart(account, agentName);

  const handleShare = async () => {
    const url = window.location.href;
    const isMobile = /Android|iPhone|iPad|iPod/i.test(navigator.userAgent);

    if (isMobile && navigator.share) {
      try {
        await navigator.share({ url });
        return;
      } catch {
        // User cancelled or share failed — fall through to clipboard
      }
    }

    await navigator.clipboard.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <PageBreadcrumb
      items={[
        { label: "Browse Agents", to: "/browse" },
        {
          label: (
            <>
              {account} <span className="text-muted-foreground">/</span> {agentName}
            </>
          ),
        },
      ]}
      actions={
        <>
          <Button
            variant="outline"
            size="sm"
            aria-label="Heart"
            onClick={() => toggleHeart.mutate()}
          >
            {initialHearted ? (
              <HeartSolid className="h-3.5 w-3.5 text-red-500" />
            ) : (
              <HeartOutline className="h-3.5 w-3.5" />
            )}
            Likes
            <span className="border-l pl-2 ml-1 text-xs tabular-nums">{initialHeartCount}</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleShare}
          >
            {copied ? (
              <>
                <Check className="h-3.5 w-3.5 text-green-500" />
                Link copied
              </>
            ) : (
              <>
                <ShareIcon className="h-3.5 w-3.5" />
                Share
              </>
            )}
          </Button>
        </>
      }
    />
  );
}
