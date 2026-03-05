import { useState, useRef, useEffect, useMemo } from "react";
import { useCombobox } from "downshift";
import { X, Mail, User, AlertCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import { useSearchAccounts } from "@/api/queries/accounts";

export type InviteKind = "account" | "email";

export interface InviteEntry {
  value: string;
  kind: InviteKind;
  valid: boolean;
}

type DropdownItem =
  | { type: "email"; email: string }
  | { type: "account"; name: string };

interface InviteInputProps {
  entries: InviteEntry[];
  onChange: (entries: InviteEntry[]) => void;
  exclude?: Set<string>;
  placeholder?: string;
  className?: string;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function isValidEmail(value: string): boolean {
  return EMAIL_RE.test(value);
}

function itemToString(item: DropdownItem | null): string {
  if (!item) return "";
  return item.type === "email" ? item.email : item.name;
}

function toEntry(item: DropdownItem): InviteEntry {
  if (item.type === "account") {
    return { value: item.name, kind: "account", valid: true };
  }
  return { value: item.email, kind: "email", valid: isValidEmail(item.email) };
}

export function InviteInput({
  entries,
  onChange,
  exclude,
  placeholder = "Search by username or enter an email",
  className,
}: InviteInputProps) {
  const [input, setInput] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const trimmed = input.trim();
  const looksLikeEmail = isValidEmail(trimmed);
  const shouldSearch = trimmed.length >= 3 && !looksLikeEmail;

  // Debounce search input
  useEffect(() => {
    const id = setTimeout(
      () => setDebouncedSearch(shouldSearch ? trimmed : ""),
      shouldSearch ? 200 : 0,
    );
    return () => clearTimeout(id);
  }, [trimmed, shouldSearch]);

  const { data: searchData } = useSearchAccounts(debouncedSearch);

  const existingValues = useMemo(
    () => new Set(entries.map((e) => e.value)),
    [entries],
  );

  const dropdownItems = useMemo(() => {
    const items: DropdownItem[] = [];
    if (looksLikeEmail && !existingValues.has(trimmed)) {
      items.push({ type: "email", email: trimmed });
    }
    for (const r of searchData?.results ?? []) {
      if (r.type !== "personal") continue;
      if (!existingValues.has(r.name) && !exclude?.has(r.name)) {
        items.push({ type: "account", name: r.name });
      }
    }
    return items;
  }, [looksLikeEmail, trimmed, existingValues, searchData?.results, exclude]);

  const removeEntry = (entry: InviteEntry) =>
    onChange(entries.filter((e) => e.value !== entry.value));

  // --- Combobox (search dropdown) ---
  const {
    isOpen,
    getInputProps,
    getMenuProps,
    getItemProps,
    highlightedIndex,
  } = useCombobox<DropdownItem>({
    items: dropdownItems,
    itemToString,
    defaultHighlightedIndex: 0,
    selectedItem: null,
    inputValue: input,
    onInputValueChange: ({ inputValue: newValue }) => {
      setInput(newValue ?? "");
    },
    onSelectedItemChange: ({ selectedItem: newItem }) => {
      if (!newItem) return;
      const entry = toEntry(newItem);
      if (!entries.some((e) => e.value === entry.value)) {
        onChange([...entries, entry]);
      }
    },
    stateReducer: (_state, actionAndChanges) => {
      const { changes, type } = actionAndChanges;
      switch (type) {
        case useCombobox.stateChangeTypes.InputKeyDownEnter:
        case useCombobox.stateChangeTypes.ItemClick:
          return {
            ...changes,
            inputValue: "",
            highlightedIndex: 0,
          };
        case useCombobox.stateChangeTypes.InputBlur:
          return {
            ...changes,
            selectedItem: null,
          };
        case useCombobox.stateChangeTypes.ControlledPropUpdatedSelectedItem:
          return {
            ...changes,
            inputValue: _state.inputValue,
          };
        default:
          return changes;
      }
    },
  });

  // --- Paste handler (domain-specific, not in downshift) ---
  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const pasted = e.clipboardData.getData("text");
    if (
      pasted.includes(",") ||
      pasted.includes(";") ||
      pasted.includes(" ")
    ) {
      e.preventDefault();
      const parts = pasted
        .split(/[,;\s]+/)
        .map((s) => s.trim())
        .filter(Boolean);
      const newEntries: InviteEntry[] = [];
      const seen = new Set(existingValues);
      for (const part of parts) {
        if (seen.has(part)) continue;
        if (exclude?.has(part)) continue;
        if (isValidEmail(part)) {
          newEntries.push({ value: part, kind: "email", valid: true });
        } else {
          newEntries.push({ value: part, kind: "account", valid: true });
        }
        seen.add(part);
      }
      if (newEntries.length > 0) onChange([...entries, ...newEntries]);
      setInput("");
    }
  };

  return (
    <div className="relative">
      <div
        className={cn(
          "border-input flex min-h-9 flex-wrap items-center gap-1.5 rounded border bg-transparent px-2 py-1.5 text-sm transition-[color,box-shadow]",
          "focus-within:border-teal-700 focus-within:ring-teal-700 focus-within:ring-[2px] focus-within:ring-offset-2",
          className,
        )}
        onClick={() => inputRef.current?.focus()}
      >
        {entries.map((entry) => (
          <span
            key={entry.value}
            tabIndex={-1}
            onKeyDown={(e) => {
              if (e.key === "ArrowLeft") {
                (e.currentTarget.previousElementSibling as HTMLElement)?.focus();
              } else if (e.key === "ArrowRight") {
                (e.currentTarget.nextElementSibling as HTMLElement)?.focus();
              } else if (e.key === "Backspace" || e.key === "Delete") {
                e.preventDefault();
                const target =
                  e.currentTarget.previousElementSibling ??
                  e.currentTarget.nextElementSibling;
                removeEntry(entry);
                (target as HTMLElement)?.focus();
              }
            }}
            className={cn(
              "inline-flex items-center gap-1 rounded-full bg-teal-700 py-0.5 pl-2 pr-1 text-xs font-medium text-teal-50 outline-none dark:bg-teal-300 dark:text-teal-950",
              !entry.valid && "opacity-60",
              "focus:ring-2 focus:ring-teal-700 focus:ring-offset-1",
            )}
          >
            {!entry.valid ? (
              <AlertCircle className="size-3 shrink-0" />
            ) : entry.kind === "account" ? (
              <User className="size-3 shrink-0" />
            ) : (
              <Mail className="size-3 shrink-0" />
            )}
            <span className={cn(!entry.valid && "line-through")}>{entry.value}</span>
            <span
              className="hover:bg-teal-600 dark:hover:bg-teal-200 cursor-pointer rounded-full p-0.5 transition-colors"
              onClick={(e) => {
                e.stopPropagation();
                removeEntry(entry);
              }}
            >
              <X className="size-3" />
            </span>
          </span>
        ))}
        <input
          className="min-w-[120px] flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
          placeholder={entries.length === 0 ? placeholder : undefined}
          // eslint-disable-next-line react-hooks/refs -- downshift requires ref passed via getInputProps
          {...getInputProps({
            ref: inputRef,
            onPaste: handlePaste,
            onKeyDown: (e: React.KeyboardEvent<HTMLInputElement>) => {
              if (
                e.key === "ArrowLeft" &&
                entries.length > 0 &&
                inputRef.current?.selectionStart === 0
              ) {
                (inputRef.current.previousElementSibling as HTMLElement)?.focus();
              }
              if (e.key === "Backspace" && input === "" && entries.length > 0) {
                removeEntry(entries[entries.length - 1]);
              }
            },
          })}
        />
      </div>

      <ul
        className={cn(
          "border-input bg-popover text-popover-foreground absolute top-full left-0 z-50 mt-1 w-full overflow-hidden rounded border shadow-md",
          (!isOpen || dropdownItems.length === 0) && "hidden",
        )}
        {...getMenuProps()}
      >
        {isOpen &&
          dropdownItems.map((item, index) => (
            <li
              key={item.type === "account" ? item.name : item.email}
              className={cn(
                "flex w-full cursor-default items-center gap-2 px-3 py-2 text-sm",
                highlightedIndex === index
                  ? "bg-stone-200 dark:bg-stone-700"
                  : "hover:bg-accent/50",
              )}
              {...getItemProps({ item, index })}
            >
              {item.type === "email" ? (
                <>
                  <Mail className="text-muted-foreground size-4 shrink-0" />
                  <span>
                    Invite{" "}
                    <span className="font-medium">{item.email}</span> by
                    email
                  </span>
                </>
              ) : (
                <>
                  <User className="text-muted-foreground size-4 shrink-0" />
                  <span className="font-medium">{item.name}</span>
                </>
              )}
            </li>
          ))}
      </ul>
    </div>
  );
}
