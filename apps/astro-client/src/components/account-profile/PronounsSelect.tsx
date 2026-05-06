import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

interface PronounsSelectProps {
  value: string;
  onValueChange: (value: string) => void;
  className?: string;
}

const PRONOUNS = ["he/him", "she/her", "they/them", "he/they", "she/they", "any/all", "prefer not to say"];

export function PronounsSelect({ value, onValueChange, className }: PronounsSelectProps) {
  return (
    <Select value={value} onValueChange={onValueChange}>
      <SelectTrigger className={className}>
        <SelectValue placeholder="Select pronouns" />
      </SelectTrigger>
      <SelectContent>
        {PRONOUNS.map((p) => (
          <SelectItem key={p} value={p}>{p}</SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
