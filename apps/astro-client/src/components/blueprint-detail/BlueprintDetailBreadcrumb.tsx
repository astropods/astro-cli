import { Check } from "lucide-react";
import { HeartIcon as HeartOutline, ShareIcon } from "@heroicons/react/24/outline";
import { HeartIcon as HeartSolid } from "@heroicons/react/24/solid";
import { Button } from "@/components/ui/button";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { UserAvatar } from "@/components/UserAvatar";
import { useToggleHeart } from "@/api/queries/hearts";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { blueprintsAccountPath } from "@/lib/routes";

function HeartButton({
  iconOnly,
  hearted,
  heartCount,
  onToggle,
}: {
  iconOnly?: boolean;
  hearted: boolean;
  heartCount: number;
  onToggle: () => void;
}) {
  return (
    <Button
      variant="outline"
      size={iconOnly ? "icon" : "sm"}
      aria-label="Heart"
      onClick={onToggle}
    >
      {hearted ? (
        <HeartSolid className="h-3.5 w-3.5 text-red-500" />
      ) : (
        <HeartOutline className="h-3.5 w-3.5" />
      )}
      {!iconOnly && (
        <>
          Hearts
          <span className="border-l pl-2 ml-1 text-xs tabular-nums">{heartCount}</span>
        </>
      )}
    </Button>
  );
}

function ShareButton({
  iconOnly,
  copied,
  onShare,
}: {
  iconOnly?: boolean;
  copied: boolean;
  onShare: () => void;
}) {
  return (
    <Button
      variant="outline"
      size={iconOnly ? "icon" : "sm"}
      onClick={onShare}
    >
      {copied ? (
        <>
          <Check className="h-3.5 w-3.5 text-green-500" />
          {!iconOnly && "Link copied"}
        </>
      ) : (
        <>
          <ShareIcon className="h-3.5 w-3.5" />
          {!iconOnly && "Share"}
        </>
      )}
    </Button>
  );
}

export interface BlueprintDetailBreadcrumbProps {
  account: string;
  blueprintName: string;
  hearted?: boolean;
  heartCount?: number;
}

export function BlueprintDetailBreadcrumb({
  account,
  blueprintName,
  hearted = false,
  heartCount = 0,
}: BlueprintDetailBreadcrumbProps) {
  const { copy, copied } = useCopyToClipboard(2000);
  const toggleHeart = useToggleHeart(account, blueprintName);

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

    await copy(url);
  };

  return (
    <PageBreadcrumb
      items={[
        { label: "Blueprints", to: "/blueprints" },
        { label: account, to: blueprintsAccountPath(account) },
        { label: blueprintName },
      ]}
      mobileItems={[
        {
          label: (
            <span className="inline-flex items-center gap-2">
              <UserAvatar handle={account} name={account} className="size-5" />
              {account}
            </span>
          ),
          to: blueprintsAccountPath(account),
        },
      ]}
      actions={
        <>
          <HeartButton hearted={hearted} heartCount={heartCount} onToggle={() => toggleHeart.mutate()} />
          <ShareButton copied={copied} onShare={handleShare} />
        </>
      }
      mobileActions={
        <>
          <HeartButton iconOnly hearted={hearted} heartCount={heartCount} onToggle={() => toggleHeart.mutate()} />
          <ShareButton iconOnly copied={copied} onShare={handleShare} />
        </>
      }
    />
  );
}
