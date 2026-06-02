import type { ReactNode } from "react";
import { Download } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { openBadgeShareIntent } from "@/lib/share-utils";

interface ShareBadgeDropdownProps {
  /** Trigger element (typically a Button); passed via `asChild` to the menu trigger. */
  children: ReactNode;
  /** Display name used in the share copy (e.g. "Code Reviewer"). */
  launchName: string;
  /** Public blueprint URL embedded in the share copy. */
  blueprintUrl: string;
  /** Generated SVG markup of the badge — used by the download actions. */
  svg: string;
  /** Slug used as the filename stem when downloading. */
  downloadName: string;
  /** Stable id embedded in the downloaded filename. */
  downloadId: string;
  /** Which side of the trigger to render the menu on. Defaults to "bottom". */
  side?: "top" | "bottom";
}

/** Share + Download menu shared by the post-deploy reveal overlay and the
 *  agent badge modal. Wraps a caller-provided trigger so each surface can
 *  style its own button. */
export function ShareBadgeDropdown({
  children,
  launchName,
  blueprintUrl,
  svg,
  downloadName,
  downloadId,
  side = "bottom",
}: ShareBadgeDropdownProps) {
  const handleDownload = async (format: "svg" | "png") => {
    const mod = await import("astro-trading-card/browser");
    const opts = { name: downloadName, id: downloadId };
    if (format === "svg") {
      await mod.downloadSvg(svg, opts);
    } else {
      await mod.downloadPng(svg, opts);
    }
  };

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent side={side} align="center" sideOffset={6} className="w-fit min-w-0">
        <DropdownMenuItem
          onSelect={() => openBadgeShareIntent("x", { launchName, blueprintUrl })}
          className="gap-2"
        >
          <span className="inline-flex size-4 items-center justify-center rounded-[3px] border border-current text-[10px] font-semibold">
            X
          </span>
          Share on X
        </DropdownMenuItem>
        <DropdownMenuItem
          onSelect={() => openBadgeShareIntent("linkedin", { launchName, blueprintUrl })}
          className="gap-2"
        >
          <span className="inline-flex size-4 items-center justify-center rounded-[3px] border border-current text-[8px] font-bold leading-none">
            in
          </span>
          Share on LinkedIn
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => void handleDownload("png")} className="gap-2">
          <Download className="size-4 text-current" />
          Download PNG
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => void handleDownload("svg")} className="gap-2">
          <Download className="size-4 text-current" />
          Download SVG
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
