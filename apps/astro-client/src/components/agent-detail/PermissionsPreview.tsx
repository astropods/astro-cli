import { useState } from "react";
import { ShieldCheck, ChevronDown, ChevronUp } from "lucide-react";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { SidebarSection } from "./SidebarSection";

export interface PermissionsPreviewProps {
  permissions: string[];
}

const VISIBLE_COUNT = 3;

function PermissionItem({ text }: { text: string }) {
  return (
    <div className="flex items-center gap-2">
      <ShieldCheck className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <span>{text}</span>
    </div>
  );
}

export function PermissionsPreview({ permissions }: PermissionsPreviewProps) {
  const [open, setOpen] = useState(false);

  if (permissions.length === 0) return null;

  const visible = permissions.slice(0, VISIBLE_COUNT);
  const hidden = permissions.slice(VISIBLE_COUNT);

  return (
    <SidebarSection title="Permissions">
      <div className="flex flex-col gap-2 text-[13px] leading-[1.5] text-foreground">
        {visible.map((p) => (
          <PermissionItem key={p} text={p} />
        ))}

        {hidden.length > 0 && (
          <Collapsible open={open} onOpenChange={setOpen}>
            <CollapsibleContent>
              <div className="flex flex-col gap-2">
                {hidden.map((p) => (
                  <PermissionItem key={p} text={p} />
                ))}
              </div>
            </CollapsibleContent>
            <CollapsibleTrigger asChild>
              <button className="mt-1 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors">
                {open ? (
                  <>
                    Show less <ChevronUp className="h-3 w-3" />
                  </>
                ) : (
                  <>
                    Show {hidden.length} more <ChevronDown className="h-3 w-3" />
                  </>
                )}
              </button>
            </CollapsibleTrigger>
          </Collapsible>
        )}
      </div>
    </SidebarSection>
  );
}
