import { useState, useCallback, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { CheckCircle2 } from "lucide-react";
import { useOpenAPISchema } from "@/api/openmeter";
import {
  extractSchema,
  validateAgainstSchema,
  formatErrors,
  getSchemaDefaults,
  SCHEMA_REFS,
} from "@/lib/schemas";
import { SchemaForm } from "@/components/schema-form";

type SchemaRefKey = keyof typeof SCHEMA_REFS;

interface SchemaFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  schemaRef: SchemaRefKey;
  onSubmit: (body: unknown) => void;
  isPending: boolean;
  submitLabel?: string;
  hiddenFields?: string[];
  defaults?: Record<string, unknown>;
  maxWidth?: string;
  error?: string;
}

export function SchemaFormDialog({
  open,
  onOpenChange,
  title,
  description: desc,
  schemaRef,
  onSubmit,
  isPending,
  submitLabel = "Create",
  hiddenFields = [],
  defaults = {},
  maxWidth = "sm:max-w-md",
  error: externalError,
}: SchemaFormDialogProps) {
  const { data: spec, isLoading: schemaLoading } = useOpenAPISchema();
  const schema = spec ? extractSchema(spec, SCHEMA_REFS[schemaRef]) : null;

  const [value, setValue] = useState<Record<string, unknown>>({});
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);

  // Reset state when dialog opens/closes or schema loads
  useEffect(() => {
    if (open && schema) {
      const schemaDefaults = getSchemaDefaults(schema);
      const merged = { ...schemaDefaults, ...defaults };
      setValue(merged);
      setRawJson(JSON.stringify(cleanValue(merged), null, 2));
      setMode("pretty");
      setValidated(false);
      setErrors([]);
    }
  }, [open, !!schema]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleChange = (newValue: Record<string, unknown>) => {
    setValue(newValue);
    setValidated(false);
    setErrors([]);
  };

  const handleTabChange = useCallback(
    (tab: string) => {
      if (tab === "json") {
        setRawJson(JSON.stringify(cleanValue(value), null, 2));
      } else {
        try {
          setValue(JSON.parse(rawJson));
        } catch {
          // keep current value
        }
      }
      setMode(tab);
      setValidated(false);
      setErrors([]);
    },
    [value, rawJson]
  );

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") {
      try {
        return JSON.parse(rawJson);
      } catch {
        setErrors(["Invalid JSON"]);
        return null;
      }
    }
    return cleanValue(value);
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body || !schema) return;
    const { valid, errors: valErrors } = validateAgainstSchema(schema, body);
    if (valid) {
      setValidated(true);
      setErrors([]);
    } else {
      setValidated(false);
      setErrors(formatErrors(valErrors));
    }
  };

  const handleSubmit = () => {
    const body = getBody();
    if (body) onSubmit(body);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={maxWidth} showCloseButton={false}>
        {schemaLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : (
          <Tabs value={mode} onValueChange={handleTabChange}>
            <div className="flex items-start justify-between">
              <DialogHeader>
                <DialogTitle>{title}</DialogTitle>
                <DialogDescription>{desc}</DialogDescription>
              </DialogHeader>
              <TabsList className="h-6 p-0.5">
                <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">
                  Pretty
                </TabsTrigger>
                <TabsTrigger value="json" className="h-5 px-2 text-[10px]">
                  JSON
                </TabsTrigger>
              </TabsList>
            </div>
            <TabsContent value="pretty" className="mt-3">
              {schema && (
                <SchemaForm
                  schema={schema}
                  value={value}
                  onChange={handleChange}
                  hiddenFields={hiddenFields}
                  defaults={defaults}
                />
              )}
            </TabsContent>
            <TabsContent value="json" className="mt-3">
              <Textarea
                value={rawJson}
                onChange={(e) => {
                  setRawJson(e.target.value);
                  setValidated(false);
                  setErrors([]);
                }}
                className="min-h-48 font-mono text-xs text-[11px]"
              />
            </TabsContent>
          </Tabs>
        )}
        {errors.length > 0 && (
          <ul className="text-xs text-destructive space-y-0.5">
            {errors.map((e, i) => (
              <li key={i}>{e}</li>
            ))}
          </ul>
        )}
        {externalError && <p className="text-xs text-destructive">{externalError}</p>}
        {validated && errors.length === 0 && (
          <p className="flex items-center gap-1 text-xs text-green-600">
            <CheckCircle2 className="size-3" /> Validation passed
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          {validated ? (
            <Button size="sm" onClick={handleSubmit} disabled={isPending}>
              {submitLabel}
            </Button>
          ) : (
            <Button
              size="sm"
              variant="secondary"
              onClick={handleValidate}
              disabled={schemaLoading}
            >
              {schemaLoading ? "Loading..." : "Validate"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Remove undefined and empty-string optional values from the output */
function cleanValue(obj: Record<string, unknown>): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(obj)) {
    if (val === undefined || val === "") continue;
    result[key] = val;
  }
  return result;
}
