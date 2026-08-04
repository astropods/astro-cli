import { useId, useMemo } from "react";
import { BuildingOffice2Icon } from "@heroicons/react/24/outline";
import { UserAvatar } from "@/components/UserAvatar";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { comparePersonalFirst } from "@/lib/account-order";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

interface CreateInAccountPickerProps {
  value: string;
  onChange: (account: string) => void;
  disabled?: boolean;
  className?: string;
}

export function CreateInAccountPicker({ value, onChange, disabled, className }: CreateInAccountPickerProps) {
  const { accounts } = useAuth();
  const triggerId = useId();
  const sorted = useMemo(() => [...accounts].sort(comparePersonalFirst), [accounts]);

  return (
    <div className={cn("space-y-1.5", className)}>
      <Label htmlFor={triggerId} size="md">Create in</Label>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={triggerId} className="[&>span]:flex [&>span]:items-center">
          <SelectValue placeholder="Select account" />
        </SelectTrigger>
        <SelectContent>
          {sorted.map((account) => (
            <SelectItem key={account.id} value={account.name}>
              <span className="inline-flex items-center gap-2">
                {account.type === "personal" ? (
                  <UserAvatar
                    handle={account.name}
                    name={account.display_name || account.name}
                    avatarUrl={account.avatar_url}
                    className="size-[18px] shrink-0"
                  />
                ) : (
                  <span className="flex size-[18px] shrink-0 items-center justify-center rounded-md bg-accent">
                    <BuildingOffice2Icon className="size-2.5 text-muted-foreground" />
                  </span>
                )}
                {account.display_name || account.name}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
