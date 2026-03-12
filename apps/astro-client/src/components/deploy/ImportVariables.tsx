import { useState, useRef, useCallback, useEffect } from "react";
import { Import, Check, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { parseVariables } from "./parse-env";

export interface ImportResult {
  matched: string[];
  skipped: string[];
}

interface ImportVariablesProps {
  onImport: (values: Record<string, string>) => ImportResult;
}

const ALLOWED_FILE_PATTERN = /(\.(env|json|txt)(\.?\w*)$)|(^\.env)/i;
const MAX_FILE_SIZE = 256 * 1024; // 256 KB

export function ImportVariables({ onImport }: ImportVariablesProps) {
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [parseError, setParseError] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const autoCloseTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearAutoClose = useCallback(() => {
    if (autoCloseTimer.current) {
      clearTimeout(autoCloseTimer.current);
      autoCloseTimer.current = null;
    }
  }, []);

  useEffect(() => clearAutoClose, [clearAutoClose]);

  const reset = useCallback(() => {
    clearAutoClose();
    setResult(null);
    setParseError(null);
    setIsDragging(false);
  }, [clearAutoClose]);

  const handleClose = useCallback(() => {
    setOpen(false);
    reset();
  }, [reset]);

  const processFile = useCallback(
    (file: File) => {
      if (!ALLOWED_FILE_PATTERN.test(file.name)) {
        setParseError(`"${file.name}" is not a supported file type. Use a .env, .json, or .txt file.`);
        return;
      }

      if (file.size > MAX_FILE_SIZE) {
        setParseError("File is too large. Maximum size is 256 KB.");
        return;
      }

      setParseError(null);

      const reader = new FileReader();
      reader.onload = (event) => {
        const text = event.target?.result;
        if (typeof text !== "string") return;

        const parsed = parseVariables(text);
        if (Object.keys(parsed).length === 0) {
          setParseError(`No variables found in "${file.name}". Expected KEY=VALUE lines or a JSON object.`);
          return;
        }

        const importResult = onImport(parsed);
        setResult(importResult);
        clearAutoClose();
        autoCloseTimer.current = setTimeout(() => handleClose(), importResult.matched.length > 0 ? 3000 : 5000);
      };
      reader.readAsText(file);
    },
    [onImport, handleClose],
  );

  const handleFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) processFile(file);
      e.target.value = "";
    },
    [processFile],
  );

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(false);

      const file = e.dataTransfer.files?.[0];
      if (file) processFile(file);
    },
    [processFile],
  );

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) handleClose(); else { reset(); setOpen(true); } }}>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="gap-1.5 text-xs"
        onClick={() => { reset(); setOpen(true); }}
      >
        <Import className="size-3.5" />
        Import
      </Button>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>Import variables</DialogTitle>
          <DialogDescription>
            Upload a .env, .json, or .txt file to auto-fill matching variables.
          </DialogDescription>
        </DialogHeader>

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
            <div
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
              onClick={() => fileInputRef.current?.click()}
              className={`flex flex-col items-center justify-center gap-2 rounded-[6px] border-2 border-dashed p-8 cursor-pointer transition-colors ${
                isDragging
                  ? "border-primary bg-primary/5"
                  : "border-border hover:border-muted-foreground"
              }`}
            >
              <Upload className="size-5 text-muted-foreground" />
              <div className="text-center">
                <p className="text-sm text-foreground">
                  Drop a file here or <span className="text-primary underline">browse</span>
                </p>
                <p className="text-xs text-muted-foreground mt-1">
                  Supports .env, .json, and .txt files
                </p>
              </div>
              <input
                ref={fileInputRef}
                type="file"
                className="hidden"
                onChange={handleFileChange}
              />
            </div>
            {parseError && (
              <p className="text-xs text-destructive mt-2">{parseError}</p>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
