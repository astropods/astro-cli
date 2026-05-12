import { Input } from "@/components/ui/input";
import { sanitizeAccountName } from "@/hooks/use-account-name";
import { cn } from "@/lib/utils";

interface AccountNameInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoFocus?: boolean;
  isChecking: boolean;
  isAvailable: boolean;
  displayError: string | null;
  onBlur?: () => void;
}

export function AccountNameInput({
  value,
  onChange,
  placeholder = "username",
  autoFocus,
  isChecking,
  isAvailable,
  displayError,
  onBlur,
}: AccountNameInputProps) {
  return (
    <div>
      <div className="relative">
        <Input
          value={value}
          onChange={(e) => onChange(sanitizeAccountName(e.target.value))}
          onBlur={onBlur}
          placeholder={placeholder}
          autoFocus={autoFocus}
          maxLength={39}
          aria-invalid={!!displayError || undefined}
          className={cn(
            "pr-9",
            isAvailable && "border-green-600 focus-visible:border-green-600",
          )}
        />
        <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-lg leading-none">
          {value.length === 0
            ? ""
            : isChecking
              ? "\u2026"
              : isAvailable
                ? "\u2713"
                : displayError
                  ? "\u2717"
                  : ""}
        </span>
      </div>
      <div className="mt-1.5 min-h-5 text-xs">
        {displayError && (
          <p className="text-destructive">{displayError}</p>
        )}
        {isChecking && (
          <p className="text-muted-foreground">Checking availability...</p>
        )}
        {isAvailable && <p className="text-green-600">Available</p>}
      </div>
    </div>
  );
}
