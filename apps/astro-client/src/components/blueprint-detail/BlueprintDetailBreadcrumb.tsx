import { Check, Copy } from "lucide-react";
import { HeartIcon as HeartOutline, ShareIcon } from "@heroicons/react/24/outline";
import { HeartIcon as HeartSolid } from "@heroicons/react/24/solid";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { UserAvatar } from "@/components/UserAvatar";
import { useToggleHeart } from "@/api/queries/hearts";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { blueprintsAccountPath } from "@/lib/routes";
import { getLinkedInShareHref, getXShareHref } from "@/lib/share-utils";
import { XIcon } from "@/components/ui/svgs/xIcon";
import { LinkedInIcon } from "@/components/ui/svgs/linkedinIcon";

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

function ShareDropdown({
  iconOnly,
  copied,
  shareUrl,
  blueprintName,
  onCopy,
}: {
  iconOnly?: boolean;
  copied: boolean;
  shareUrl: string;
  blueprintName: string;
  onCopy: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size={iconOnly ? "icon" : "sm"}>
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
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onCopy}>
          <Copy className="h-3.5 w-3.5" />
          Copy link
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <a href={getXShareHref(shareUrl, blueprintName)} target="_blank" rel="noopener noreferrer">
            <XIcon className="h-[11px] w-[11px] shrink-0" />
            Share on X
          </a>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <a href={getLinkedInShareHref(shareUrl, blueprintName)} target="_blank" rel="noopener noreferrer">
            <LinkedInIcon className="h-[13px] w-[13px] shrink-0" />
            Share on LinkedIn
          </a>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export interface BlueprintDetailBreadcrumbProps {
  account: string;
  blueprintName: string;
  hearted?: boolean;
  heartCount?: number;
  shareUrl?: string;
}

export function BlueprintDetailBreadcrumb({
  account,
  blueprintName,
  hearted = false,
  heartCount = 0,
  shareUrl = "",
}: BlueprintDetailBreadcrumbProps) {
  const { copy, copied } = useCopyToClipboard(2000);
  const toggleHeart = useToggleHeart(account, blueprintName);

  const handleCopy = async () => {
    await copy(shareUrl || window.location.href);
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
          <ShareDropdown copied={copied} shareUrl={shareUrl || window.location.href} blueprintName={blueprintName} onCopy={handleCopy} />
        </>
      }
      mobileActions={
        <>
          <HeartButton iconOnly hearted={hearted} heartCount={heartCount} onToggle={() => toggleHeart.mutate()} />
          <ShareDropdown iconOnly copied={copied} shareUrl={shareUrl || window.location.href} blueprintName={blueprintName} onCopy={handleCopy} />
        </>
      }
    />
  );
}
