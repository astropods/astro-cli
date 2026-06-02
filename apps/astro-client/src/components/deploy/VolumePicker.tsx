import { useEffect, useId, useRef } from "react";
import { HardDrive } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

const DEFAULT_VOLUME_MOUNT = "/data";

export interface VolumePickerProps {
  volumeMount: string;
  onVolumeMountChange: (value: string) => void;
}

export function VolumePicker({ volumeMount, onVolumeMountChange }: VolumePickerProps) {
  const isSelected = !!volumeMount.trim();
  const switchId = useId();
  // Preserve the last non-empty mount so toggling off and back on restores it.
  const lastMountRef = useRef<string>(volumeMount || DEFAULT_VOLUME_MOUNT);
  useEffect(() => {
    if (volumeMount.trim()) {
      lastMountRef.current = volumeMount;
    }
  }, [volumeMount]);

  const handleToggle = (checked: boolean) => {
    onVolumeMountChange(checked ? lastMountRef.current || DEFAULT_VOLUME_MOUNT : "");
  };

  return (
    <div
      className={cn(
        "overflow-hidden rounded-[10px] border transition-colors",
        isSelected
          ? "border-primary/40 bg-primary/5 dark:border-primary/70"
          : "border-border",
      )}
    >
      <label
        htmlFor={switchId}
        className={cn(
          "flex items-center gap-4 px-4 py-3 cursor-pointer transition-colors",
          !isSelected && "hover:bg-muted/50",
        )}
      >
        <div
          className={cn(
            "flex h-9 w-9 items-center justify-center rounded-sm shrink-0 transition-colors",
            isSelected
              ? "bg-primary/10 text-primary dark:bg-primary/25 dark:text-indigo-300"
              : "bg-muted text-muted-foreground",
          )}
        >
          <HardDrive className="h-5 w-5" strokeWidth={1.5} />
        </div>
        <div className="flex flex-col gap-0.5 flex-1 min-w-0">
          <span className="text-heading-4 text-foreground">Persistent volume</span>
          <span className="text-body-sm text-muted-foreground">
            Attach storage that survives restarts. Required if your agent writes state to disk or uses SQLite.
          </span>
        </div>
        <Switch id={switchId} checked={isSelected} onCheckedChange={handleToggle} />
      </label>
      {isSelected && (
        <div className="space-y-1.5 border-t border-primary/20 bg-surface px-4 py-4">
          <Label size="md" htmlFor="agent-volume-mount">Mount path</Label>
          <Input
            id="agent-volume-mount"
            value={volumeMount}
            onChange={(e) => onVolumeMountChange(e.target.value)}
            placeholder={DEFAULT_VOLUME_MOUNT}
          />
          <p className="text-body-sm text-muted-foreground">
            Absolute path inside the container.
          </p>
        </div>
      )}
    </div>
  );
}
