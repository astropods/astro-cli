import { useState } from "react";
import { Check, Share2 } from "lucide-react";
import { HeartIcon as HeartOutline } from "@heroicons/react/24/outline";
import { HeartIcon as HeartSolid } from "@heroicons/react/24/solid";
import { Button } from "@/components/ui/button";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";

export interface AgentDetailBreadcrumbProps {
  account: string;
  agentName: string;
}

export function AgentDetailBreadcrumb({
  account,
  agentName,
}: AgentDetailBreadcrumbProps) {
  const [copied, setCopied] = useState(false);
  const [hearted, setHearted] = useState(false);

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
            onClick={() => setHearted((h) => !h)}
            className="h-8 rounded-md px-3 text-[13px] font-semibold text-muted-foreground"
          >
            {hearted ? (
              <HeartSolid className="h-3.5 w-3.5 text-red-500" />
            ) : (
              <HeartOutline className="h-3.5 w-3.5" />
            )}
            Heart
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleShare}
            className="h-8 rounded-md px-3 text-[13px] font-semibold text-muted-foreground"
          >
            {copied ? (
              <>
                <Check className="h-3.5 w-3.5 text-green-500" />
                Copied
              </>
            ) : (
              <>
                <Share2 className="h-3.5 w-3.5" />
                Share
              </>
            )}
          </Button>
        </>
      }
    />
  );
}
