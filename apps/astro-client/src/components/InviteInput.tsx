import { useState, useRef, useCallback, type KeyboardEvent } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export interface InviteEntry {
  value: string;
  valid: boolean;
}

interface InviteInputProps {
  entries: InviteEntry[];
  onChange: (entries: InviteEntry[]) => void;
  placeholder?: string;
  className?: string;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function isValidEmail(value: string): boolean {
  return EMAIL_RE.test(value);
}

function parseInput(raw: string): InviteEntry[] {
  return raw
    .split(/[,;\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((value) => ({ value, valid: isValidEmail(value) }));
}

export function InviteInput({
  entries,
  onChange,
  placeholder = "name@example.com",
  className,
}: InviteInputProps) {
  const [input, setInput] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const addEntries = useCallback(
    (raw: string) => {
      const parsed = parseInput(raw);
      if (parsed.length === 0) return;
      const existing = new Set(entries.map((e) => e.value));
      const deduped = parsed.filter((e) => !existing.has(e.value));
      if (deduped.length > 0) {
        onChange([...entries, ...deduped]);
      }
    },
    [entries, onChange]
  );

  const removeEntry = useCallback(
    (value: string) => {
      onChange(entries.filter((e) => e.value !== value));
    },
    [entries, onChange]
  );

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    const trimmed = input.trim();

    if (e.key === "Enter" || e.key === "," || e.key === "Tab") {
      if (trimmed) {
        e.preventDefault();
        addEntries(trimmed);
        setInput("");
      } else if (e.key === "Enter") {
        // Let form submit if input is empty
      }
    }

    if (e.key === "Backspace" && !input && entries.length > 0) {
      removeEntry(entries[entries.length - 1].value);
    }
  };

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const pasted = e.clipboardData.getData("text");
    if (pasted.includes(",") || pasted.includes(";") || pasted.includes(" ")) {
      e.preventDefault();
      addEntries(pasted);
      setInput("");
    }
  };

  const handleBlur = () => {
    const trimmed = input.trim();
    if (trimmed) {
      addEntries(trimmed);
      setInput("");
    }
  };

  return (
    <div
      className={cn(
        "border-input flex min-h-9 flex-wrap items-center gap-1.5 rounded border bg-transparent px-2 py-1.5 text-sm transition-[color,box-shadow]",
        "focus-within:border-teal-700 focus-within:ring-teal-700 focus-within:ring-[2px] focus-within:ring-offset-2",
        className
      )}
      onClick={() => inputRef.current?.focus()}
    >
      {entries.map((entry) => (
        <span
          key={entry.value}
          className={cn(
            "inline-flex items-center gap-1 rounded-full py-0.5 pl-2.5 pr-1 text-xs font-medium",
            entry.valid
              ? "bg-stone-100 text-stone-700 dark:bg-stone-800 dark:text-stone-300"
              : "bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300"
          )}
        >
          {entry.value}
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              removeEntry(entry.value);
            }}
            className="hover:bg-stone-200 dark:hover:bg-stone-700 rounded-full p-0.5 transition-colors"
          >
            <X className="size-3" />
          </button>
        </span>
      ))}
      <input
        ref={inputRef}
        type="text"
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onPaste={handlePaste}
        onBlur={handleBlur}
        placeholder={entries.length === 0 ? placeholder : undefined}
        className="min-w-[120px] flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
      />
    </div>
  );
}
