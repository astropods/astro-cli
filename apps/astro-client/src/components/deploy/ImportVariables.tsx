import { useState, useRef, useCallback } from "react";
import { Import, Check, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { parseVariables } from "./parse-env";

export interface ImportResult {
  matched: string[];
  skipped: string[];
}

interface ImportVariablesTriggerProps {
  open: boolean;
  onToggle: () => void;
}

/** Button that goes in the FormSection header `action` slot. */
export function ImportVariablesTrigger({ open, onToggle }: ImportVariablesTriggerProps) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="gap-1.5 text-xs"
      onClick={onToggle}
    >
      {open ? (
        <>
          <X className="size-3.5" />
          Close
        </>
      ) : (
        <>
          <Import className="size-3.5" />
          Import
        </>
      )}
    </Button>
  );
}

interface ImportVariablesContentProps {
  onImport: (values: Record<string, string>) => ImportResult;
  onClose: () => void;
}

const ALLOWED_FILE_PATTERN = /(\.(env|json|txt)(\.?\w*)$)|(^\.env)/i;
const MAX_FILE_SIZE = 256 * 1024; // 256 KB

/** Expandable content that renders inside FormSection children, above the fields. */
export function ImportVariablesContent({ onImport, onClose }: ImportVariablesContentProps) {
  const [pasteValue, setPasteValue] = useState("");
  const [result, setResult] = useState<ImportResult | null>(null);
  const [parseError, setParseError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleApply = useCallback(() => {
    const parsed = parseVariables(pasteValue);
    if (Object.keys(parsed).length === 0) {
      setParseError("No variables found. Paste KEY=VALUE lines (.env format) or a JSON object.");
      return;
    }
    setParseError(null);
    const importResult = onImport(parsed);
    setResult(importResult);
    setPasteValue("");

    setTimeout(() => onClose(), importResult.matched.length > 0 ? 3000 : 5000);
  }, [pasteValue, onImport, onClose]);

  const handleFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file) return;

      if (!ALLOWED_FILE_PATTERN.test(file.name)) {
        setParseError(`"${file.name}" is not a supported file type. Use a .env, .json, or .txt file.`);
        e.target.value = "";
        return;
      }

      if (file.size > MAX_FILE_SIZE) {
        setParseError("File is too large. Maximum size is 256 KB.");
        e.target.value = "";
        return;
      }

      const reader = new FileReader();
      reader.onload = (event) => {
        const text = event.target?.result;
        if (typeof text === "string") {
          setPasteValue(text);
          setResult(null);
          setParseError(null);
        }
      };
      reader.readAsText(file);
      e.target.value = "";
    },
    [],
  );

  return (
    <div className="mb-5">
      {result ? (
        <div className="rounded-[6px] bg-stone-100 p-4 dark:bg-stone-800">
          <div className="flex items-center gap-1.5">
            <Check size={16} className="text-green-700 dark:text-green-400 shrink-0" />
            <span className="text-sm font-medium text-foreground">
              Filled {result.matched.length} variable{result.matched.length !== 1 ? "s" : ""}
            </span>
          </div>
          {result.matched.length > 0 && (
            <ul className="mt-2 space-y-0.5 pl-6">
              {result.matched.map((key) => (
                <li key={key} className="text-xs text-muted-foreground font-mono">{key}</li>
              ))}
            </ul>
          )}
          {result.skipped.length > 0 && (
            <div className="mt-3">
              <p className="text-xs text-muted-foreground">
                Skipped {result.skipped.length} unrecognized key{result.skipped.length !== 1 ? "s" : ""}:
              </p>
              <ul className="mt-1 space-y-0.5 pl-6">
                {result.skipped.map((key) => (
                  <li key={key} className="text-xs text-muted-foreground font-mono">{key}</li>
                ))}
              </ul>
            </div>
          )}
          {result.matched.length === 0 && result.skipped.length === 0 && (
            <p className="text-xs text-muted-foreground mt-1">
              No keys matched the expected variables. Check that your key names match exactly.
            </p>
          )}
        </div>
      ) : (
        <>
          <Textarea
            value={pasteValue}
            onChange={(e) => {
              setPasteValue(e.target.value);
              setParseError(null);
            }}
            placeholder={"OPENAI_API_KEY=sk-...\nQDRANT_API_KEY=...\n\nor paste a JSON object"}
            rows={5}
            aria-invalid={!!parseError}
          />
          <div className="flex items-center justify-between mt-3">
            <div className="flex items-center gap-2">
              <p className="text-xs text-muted-foreground">
                Supports KEY=VALUE (.env) and JSON formats
              </p>
              <input
                ref={fileInputRef}
                type="file"
                className="hidden"
                onChange={handleFileChange}
              />
              <button
                type="button"
                className="text-xs text-muted-foreground hover:text-foreground transition-colors underline"
                onClick={() => fileInputRef.current?.click()}
              >
                or load a file
              </button>
            </div>
            <div className="flex items-center gap-2">
              <Button type="button" variant="ghost" size="sm" onClick={onClose}>
                Cancel
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={!pasteValue.trim()}
                onClick={handleApply}
              >
                Apply
              </Button>
            </div>
          </div>
          {parseError && (
            <p className="text-xs text-destructive mt-2">{parseError}</p>
          )}
        </>
      )}
    </div>
  );
}
