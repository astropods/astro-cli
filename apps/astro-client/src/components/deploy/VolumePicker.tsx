import { useEffect, useRef } from "react";
import { Check, HardDrive } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

const DEFAULT_VOLUME_MOUNT = "/data";

export interface VolumePickerProps {
  volumeMount: string;
  onVolumeMountChange: (value: string) => void;
}

export function VolumePicker({ volumeMount, onVolumeMountChange }: VolumePickerProps) {
  const isSelected = !!volumeMount.trim();
  // Remember the last non-empty mount so toggling off and back on restores
  // the user's choice instead of resetting to the default.
  const lastMountRef = useRef<string>(volumeMount || DEFAULT_VOLUME_MOUNT);
  useEffect(() => {
    if (volumeMount.trim()) {
      lastMountRef.current = volumeMount;
    }
  }, [volumeMount]);

  const toggle = () => {
    if (isSelected) {
      onVolumeMountChange("");
    } else {
      onVolumeMountChange(lastMountRef.current || DEFAULT_VOLUME_MOUNT);
    }
  };

  return (
    <div
      className={cn(
        "rounded-[6px] border transition-[border-color,background-color]",
        isSelected
          ? "border-primary/40 bg-primary/5"
          : "border-border bg-transparent",
      )}
    >
      <button
        type="button"
        aria-pressed={isSelected}
        onClick={toggle}
        className={cn(
          "w-full flex items-center gap-4 py-3 px-3 rounded-[6px] border-none bg-transparent text-left cursor-pointer transition-colors",
          !isSelected && "hover:bg-slate-200/50",
        )}
      >
        <div
          className={cn(
            "flex h-9 w-9 items-center justify-center rounded-sm shrink-0 transition-colors",
            isSelected ? "bg-primary/10 text-primary" : "bg-slate-200 text-muted-foreground",
          )}
        >
          <HardDrive className="h-5 w-5" strokeWidth={1.5} />
        </div>
        <div className="flex flex-col gap-0.5 flex-1 min-w-0">
          <span className="text-[13px] font-medium text-foreground">Persistent volume</span>
          <span className="text-[11px] text-muted-foreground">
            Attach storage that survives restarts. Required if your agent writes state to disk or uses SQLite.
          </span>
        </div>
        <div
          className={cn(
            "w-5 h-5 rounded border-2 flex items-center justify-center shrink-0 transition-colors",
            isSelected ? "border-primary bg-primary" : "border-input bg-background",
          )}
        >
          {isSelected && <Check size={14} strokeWidth={3} className="text-primary-foreground" />}
        </div>
      </button>
      {isSelected && (
        <div className="border-t border-primary/20 bg-surface px-6 py-4 rounded-b-[6px]">
          <Label size="md" htmlFor="agent-volume-mount">Mount path</Label>
          <Input
            id="agent-volume-mount"
            value={volumeMount}
            onChange={(e) => onVolumeMountChange(e.target.value)}
            placeholder={DEFAULT_VOLUME_MOUNT}
          />
          <p className="mt-1 text-body-sm text-muted-foreground">
            Absolute path inside the container.
          </p>
        </div>
      )}
    </div>
  );
}
