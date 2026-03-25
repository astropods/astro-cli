import { useState } from "react";
import { Check, ChevronDown, ChevronUp } from "lucide-react";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { SidebarSection } from "./SidebarSection";

const VISIBLE_COUNT = 4;

export interface CapabilitiesListProps {
  capabilities: string[];
}

export function CapabilitiesList({ capabilities }: CapabilitiesListProps) {
  const [open, setOpen] = useState(false);

  if (capabilities.length === 0) return null;

  const visible = capabilities.slice(0, VISIBLE_COUNT);
  const hidden = capabilities.slice(VISIBLE_COUNT);

  return (
    <SidebarSection title="Capabilities">
      <div className="flex flex-col gap-2 text-[13px] leading-[1.5] text-foreground">
        {visible.map((c) => (
          <div key={c} className="flex items-start gap-2">
            <Check className="h-3.5 w-3.5 shrink-0 text-emerald-500 mt-0.5" />
            <span>{c}</span>
          </div>
        ))}

        {hidden.length > 0 && (
          <Collapsible open={open} onOpenChange={setOpen}>
            <CollapsibleContent>
              <div className="flex flex-col gap-2">
                {hidden.map((c) => (
                  <div key={c} className="flex items-start gap-2">
                    <Check className="h-3.5 w-3.5 shrink-0 text-emerald-500 mt-0.5" />
                    <span>{c}</span>
                  </div>
                ))}
              </div>
            </CollapsibleContent>
            <CollapsibleTrigger asChild>
              <button className="mt-1 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors">
                {open ? (
                  <>Show less <ChevronUp className="h-3 w-3" /></>
                ) : (
                  <>Show more <ChevronDown className="h-3 w-3" /></>
                )}
              </button>
            </CollapsibleTrigger>
          </Collapsible>
        )}
      </div>
    </SidebarSection>
  );
}
