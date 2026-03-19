import { cn } from "@/lib/utils";
import { PRESET_AVATARS } from "@/lib/presetAvatars";

export interface PresetAvatarPickerProps {
  value: string | null;
  onChange: (id: string) => void;
  className?: string;
}

export function PresetAvatarPicker({
  value,
  onChange,
  className,
}: PresetAvatarPickerProps) {
  return (
    <div
      className={cn("grid grid-cols-5 gap-2", className)}
      role="radiogroup"
      aria-label="Choose a profile avatar"
    >
      {PRESET_AVATARS.map((avatar) => {
        const selected = value === avatar.id;
        return (
          <button
            key={avatar.id}
            type="button"
            role="radio"
            aria-checked={selected}
            aria-label={avatar.label}
            onClick={() => onChange(avatar.id)}
            className={cn(
              "relative rounded-full overflow-hidden transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
              selected
                ? "ring-2 ring-primary ring-offset-2 ring-offset-background"
                : "ring-1 ring-transparent hover:ring-border-strong",
            )}
          >
            <img
              src={avatar.src}
              alt={avatar.label}
              className="w-full aspect-square object-cover"
              draggable={false}
            />
          </button>
        );
      })}
    </div>
  );
}
