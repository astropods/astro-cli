import { FolderOpen, Search, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { inputBase, inputFocusWithin } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type SubpathPickerProps = {
  value: string;
  onChange: (value: string) => void;
};

export function SubpathPicker({ value, onChange }: SubpathPickerProps) {
  return (
    <div className="space-y-1.5">
      <Label size="md" className="flex items-center gap-1.5">
        <FolderOpen className="size-3.5" />
        Subdirectory
        <span className="font-normal text-muted-foreground">(optional)</span>
      </Label>

      <div className={cn(inputBase, inputFocusWithin, "flex h-9 items-center gap-2 px-3")}>
        <input
          type="text"
          className="flex-1 min-w-0 bg-transparent border-none outline-none text-sm placeholder:text-muted-foreground"
          placeholder="services/my-agent"
          value={value}
          onChange={e => onChange(e.target.value)}
        />
      </div>

      <p className="text-xs text-muted-foreground">
        Only trigger builds when files inside this path change.
      </p>
    </div>
  );
}
