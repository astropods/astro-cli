import { Brain, Globe, LockKeyhole } from "lucide-react";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { GitHubIcon } from "@/components/ui/svgs/githubIcon";

type LinkConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  avatarPreviewUrl: string | null;
  slug: string;
  name: string;
  selectedOrg: string;
  repoBase: string | null;
  selectedBranch: string;
  subpath?: string;
  visibility: "public" | "private";
  isCreatingBlueprint: boolean;
  onConfirm: () => void;
};

export function LinkConfirmDialog({
  open,
  onOpenChange,
  avatarPreviewUrl,
  slug,
  name,
  selectedOrg,
  repoBase,
  selectedBranch,
  subpath,
  visibility,
  isCreatingBlueprint,
  onConfirm,
}: LinkConfirmDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">

        {/* Connection visual */}
        <div className="flex items-center justify-center px-6 pt-2 pb-1">
          {/* Blueprint avatar */}
          <div className="dp-float size-14 rounded-2xl overflow-hidden border border-border shadow-md shrink-0">
            {avatarPreviewUrl ? (
              <img src={avatarPreviewUrl} alt={slug} className="size-full object-cover" />
            ) : slug ? (
              <BlueprintIdentity account={selectedOrg} name={slug} size={56} className="size-full" />
            ) : (
              <div className="size-full bg-muted" />
            )}
          </div>

          {/* Dashed connector */}
          <div className="flex-1 mx-2 border-t-2 border-dashed border-border" />

          {/* Brain node */}
          <div className="size-10 rounded-full bg-primary flex items-center justify-center shrink-0 shadow-sm">
            <Brain className="size-4 text-primary-foreground" />
          </div>

          {/* Dashed connector */}
          <div className="flex-1 mx-2 border-t-2 border-dashed border-border" />

          {/* GitHub */}
          <div className="size-14 rounded-full bg-[#1b1f23] flex items-center justify-center border border-border shrink-0 shadow-md">
            <GitHubIcon className="size-7 text-white" />
          </div>
        </div>

        <DialogHeader className="text-center items-center gap-0.5">
          <DialogTitle className="text-sm">
            Linking<span aria-hidden="true"><span className="dp-dot-1">.</span><span className="dp-dot-2">.</span><span className="dp-dot-3">.</span></span>
          </DialogTitle>
          <DialogDescription className="text-xs break-all">
            <span className="font-semibold text-foreground">{name || slug}</span>
            {" "}to{" "}
            <span className="font-mono">{repoBase}</span>
          </DialogDescription>
        </DialogHeader>

        {/* Details box */}
        <div className="rounded-lg border border-border bg-muted/30 divide-y divide-border text-sm">
          <div className="flex items-center justify-between px-3 py-2">
            <span className="text-muted-foreground text-xs">Visibility</span>
            <span className="flex items-center gap-1.5 text-xs font-medium text-foreground">
              {visibility === "private"
                ? <><LockKeyhole className="size-3 text-muted-foreground" />Private</>
                : <><Globe className="size-3 text-muted-foreground" />Public</>
              }
            </span>
          </div>
          <div className="flex items-center justify-between px-3 py-2">
            <span className="text-muted-foreground text-xs">Organization</span>
            <span className="text-xs font-medium text-foreground font-mono">{selectedOrg}</span>
          </div>
          <div className="flex items-center justify-between px-3 py-2">
            <span className="text-muted-foreground text-xs">Branch</span>
            <span className="text-xs font-medium text-foreground font-mono">{selectedBranch}</span>
          </div>
          {subpath && (
            <div className="flex items-center justify-between px-3 py-2">
              <span className="text-muted-foreground text-xs">Subdirectory</span>
              <span className="text-xs font-medium text-foreground font-mono">{subpath}</span>
            </div>
          )}
        </div>

        <p className="text-xs text-muted-foreground leading-relaxed">
          Once linked, every push to this repository will trigger an automatic build, keeping deployed agents in sync with your agent code.
        </p>

        <DialogFooter className="border-t border-border pt-4 sm:justify-between">
          <DialogClose asChild>
            <Button variant="outline" size="sm">Back</Button>
          </DialogClose>
          <Button size="sm" onClick={onConfirm} disabled={isCreatingBlueprint}>
            Create blueprint
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
