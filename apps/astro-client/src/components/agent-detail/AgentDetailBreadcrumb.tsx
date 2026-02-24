import { useState } from "react";
import { Check, Heart, Share2 } from "lucide-react";
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
        { label: "Browse Agents", to: "/hire" },
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
            variant="ghost"
            size="icon-sm"
            disabled
            className="text-tertiary-foreground"
          >
            <Heart className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={handleShare}
          >
            {copied ? (
              <Check className="h-4 w-4 text-green-500" />
            ) : (
              <Share2 className="h-4 w-4" />
            )}
          </Button>
        </>
      }
    />
  );
}
