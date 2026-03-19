import { useState } from "react";
import { Check } from "lucide-react";
import { HeartIcon as HeartOutline, ShareIcon } from "@heroicons/react/24/outline";
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
            size="icon"
            onClick={() => setHearted((h) => !h)}
          >
            {hearted ? (
              <HeartSolid className="h-3.5 w-3.5 text-red-500" />
            ) : (
              <HeartOutline className="h-3.5 w-3.5" />
            )}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleShare}
          >
            {copied ? (
              <>
                <Check className="h-3.5 w-3.5 text-green-500" />
                Copied
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
