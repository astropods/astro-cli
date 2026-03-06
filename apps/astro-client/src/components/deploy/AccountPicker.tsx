import { useMemo } from "react";
import type { Account } from "@/lib/api";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export interface AccountPickerProps {
  accounts: Account[];
  selected: string;
  onChange: (name: string) => void;
}

export function AccountPicker({
  accounts,
  selected,
  onChange,
}: AccountPickerProps) {
  const { personal, orgs } = useMemo(() => {
    const personal: Account[] = [];
    const orgs: Account[] = [];
    for (const a of accounts) {
      if (a.type === "organization") orgs.push(a);
      else personal.push(a);
    }
    orgs.sort((a, b) => a.name.localeCompare(b.name));
    return { personal, orgs };
  }, [accounts]);

  return (
    <Select value={selected} onValueChange={onChange}>
      <SelectTrigger className="w-full">
        <SelectValue placeholder="Select an account" />
      </SelectTrigger>
      <SelectContent>
        {personal.length > 0 && (
          <SelectGroup>
            <SelectLabel>Personal</SelectLabel>
            {personal.map((a) => (
              <SelectItem key={a.id} value={a.name}>
                {a.name}
              </SelectItem>
            ))}
          </SelectGroup>
        )}
        {orgs.length > 0 && (
          <SelectGroup>
            <SelectLabel>Organizations</SelectLabel>
            {orgs.map((a) => (
              <SelectItem key={a.id} value={a.name}>
                {a.name}
              </SelectItem>
            ))}
          </SelectGroup>
        )}
      </SelectContent>
    </Select>
  );
}
