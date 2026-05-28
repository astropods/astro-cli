import { useState, useRef, useMemo } from "react";
import { useCombobox } from "downshift";
import { X, Mail, AlertCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import { inputBase, inputFocusWithin } from "./ui/input";
import { UserAvatar } from "./UserAvatar";
import { useSearchAccounts } from "@/api/queries/accounts";
import { useDebouncedValue } from "@/hooks/use-debounced-value";

export type InviteKind = "account" | "email";

export interface InviteEntry {
  value: string;
  kind: InviteKind;
  valid: boolean;
  /** For account entries — the user's display name (falls back to `value`). */
  displayName?: string;
}

type DropdownItem =
  | { type: "email"; email: string }
  | { type: "account"; name: string; displayName: string };

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
    return {
      value: item.name,
      kind: "account",
      valid: true,
      displayName: item.displayName,
    };
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
  const inputRef = useRef<HTMLInputElement>(null);

  const trimmed = input.trim();
  const looksLikeEmail = isValidEmail(trimmed);
  const shouldSearch = trimmed.length >= 3 && !looksLikeEmail;
  const debouncedSearch = useDebouncedValue(shouldSearch ? trimmed : "", 200);

  const { data: searchData } = useSearchAccounts(debouncedSearch, { type: "personal" });

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
      if (!existingValues.has(r.name) && !exclude?.has(r.name)) {
        items.push({
          type: "account",
          name: r.name,
          displayName: r.display_name || r.name,
        });
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
          "flex min-h-11 flex-wrap items-center gap-1.5 px-2 py-1.5",
          inputBase,
          inputFocusWithin,
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
              "inline-flex items-center gap-1.5 rounded-full border bg-card py-0.5 pr-1 text-body-sm outline-none transition-colors",
              "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
              entry.kind === "account" ? "pl-0.5" : "pl-2",
              entry.valid
                ? "border-border text-foreground"
                : "border-destructive/40 text-destructive",
            )}
          >
            {!entry.valid ? (
              <AlertCircle className="size-3.5 shrink-0" />
            ) : entry.kind === "account" ? (
              <UserAvatar
                handle={entry.value}
                name={entry.displayName || entry.value}
                className="size-5"
              />
            ) : (
              <Mail className="size-3.5 shrink-0 text-muted-foreground" />
            )}
            <span className={cn("truncate", !entry.valid && "line-through")}>
              {entry.kind === "account"
                ? entry.displayName || entry.value
                : entry.value}
            </span>
            <button
              type="button"
              aria-label={`Remove ${entry.value}`}
              className="cursor-pointer rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              onClick={(e) => {
                e.stopPropagation();
                removeEntry(entry);
              }}
            >
              <X className="size-3" />
            </button>
          </span>
        ))}
        <input
          className="min-w-[120px] flex-1 bg-transparent px-1 outline-none placeholder:text-muted-foreground"
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
          "absolute top-full left-0 z-50 mt-1 w-full overflow-hidden rounded-sm border border-border bg-popover text-popover-foreground shadow-md",
          (!isOpen || dropdownItems.length === 0) && "hidden",
        )}
        {...getMenuProps()}
      >
        {isOpen &&
          dropdownItems.map((item, index) => {
            const highlighted = highlightedIndex === index;
            return (
              <li
                key={item.type === "account" ? item.name : item.email}
                className={cn(
                  "flex w-full cursor-default items-center gap-2.5 px-3 py-2 text-body-sm",
                  highlighted ? "bg-accent" : "hover:bg-accent/50",
                )}
                {...getItemProps({ item, index })}
              >
                {item.type === "email" ? (
                  <>
                    <Mail className="size-4 shrink-0 text-muted-foreground" />
                    <span className="truncate">
                      Invite{" "}
                      <span className="font-medium text-foreground">
                        {item.email}
                      </span>{" "}
                      by email
                    </span>
                  </>
                ) : (
                  <>
                    <UserAvatar
                      handle={item.name}
                      name={item.displayName}
                      className="size-7"
                    />
                    <span className="flex min-w-0 flex-col leading-tight">
                      <span className="truncate font-medium text-foreground">
                        {item.displayName}
                      </span>
                      <span className="truncate text-label text-muted-foreground">
                        @{item.name}
                      </span>
                    </span>
                  </>
                )}
              </li>
            );
          })}
      </ul>
    </div>
  );
}
