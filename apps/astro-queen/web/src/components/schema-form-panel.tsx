import { useState, useCallback, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
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

interface SchemaFormPanelProps {
  title: string;
  description: string;
  schemaRef: SchemaRefKey;
  onSubmit: (body: unknown) => void;
  isPending: boolean;
  submitLabel?: string;
  hiddenFields?: string[];
  defaults?: Record<string, unknown>;
  error?: string;
}

export function SchemaFormPanel({
  title,
  description: desc,
  schemaRef,
  onSubmit,
  isPending,
  submitLabel = "Create",
  hiddenFields = [],
  defaults = {},
  error: externalError,
}: SchemaFormPanelProps) {
  const { data: spec, isLoading: schemaLoading } = useOpenAPISchema();
  const schema = spec ? extractSchema(spec, SCHEMA_REFS[schemaRef]) : null;

  const [value, setValue] = useState<Record<string, unknown>>({});
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const [resetKey, setResetKey] = useState(0);

  useEffect(() => {
    if (schema) {
      const schemaDefaults = getSchemaDefaults(schema);
      const merged = { ...schemaDefaults, ...defaults };
      setValue(merged);
      setRawJson(JSON.stringify(cleanValue(merged), null, 2));
      setMode("pretty");
      setValidated(false);
      setErrors([]);
    }
  }, [!!schema, resetKey]); // eslint-disable-line react-hooks/exhaustive-deps

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
        try { setValue(JSON.parse(rawJson)); } catch {}
      }
      setMode(tab);
      setValidated(false);
      setErrors([]);
    },
    [value, rawJson]
  );

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") {
      try { return JSON.parse(rawJson); }
      catch { setErrors(["Invalid JSON"]); return null; }
    }
    return cleanValue(value);
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body || !schema) return;
    const { valid, errors: valErrors } = validateAgainstSchema(schema, body);
    if (valid) { setValidated(true); setErrors([]); }
    else { setValidated(false); setErrors(formatErrors(valErrors)); }
  };

  const handleSubmit = () => {
    const body = getBody();
    if (body) onSubmit(body);
  };

  const handleReset = () => setResetKey((k) => k + 1);

  return (
    <div className="w-72 shrink-0 rounded-lg glass p-3 space-y-2 self-start sticky top-4">
      {schemaLoading ? (
        <Skeleton className="h-48 w-full" />
      ) : (
        <Tabs value={mode} onValueChange={handleTabChange}>
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <h3 className="text-[11px] font-semibold truncate">{title}</h3>
              <p className="text-[9px] text-muted-foreground">{desc}</p>
            </div>
            <TabsList className="h-6 p-0.5 shrink-0">
              <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">Pretty</TabsTrigger>
              <TabsTrigger value="json" className="h-5 px-2 text-[10px]">JSON</TabsTrigger>
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
              onChange={(e) => { setRawJson(e.target.value); setValidated(false); setErrors([]); }}
              className="min-h-32 font-mono text-[10px]"
            />
          </TabsContent>
        </Tabs>
      )}
      {errors.length > 0 && (
        <ul className="text-[10px] text-destructive space-y-0.5">
          {errors.map((e, i) => <li key={i}>{e}</li>)}
        </ul>
      )}
      {externalError && <p className="text-[10px] text-destructive">{externalError}</p>}
      {validated && errors.length === 0 && (
        <p className="flex items-center gap-1 text-[10px] text-green-600">
          <CheckCircle2 className="size-3" /> Validation passed
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="outline" size="xs" onClick={handleReset}>Reset</Button>
        {validated ? (
          <Button size="xs" onClick={handleSubmit} disabled={isPending}>{submitLabel}</Button>
        ) : (
          <Button size="xs" variant="secondary" onClick={handleValidate} disabled={schemaLoading}>
            {schemaLoading ? "Loading..." : "Validate"}
          </Button>
        )}
      </div>
    </div>
  );
}

function cleanValue(obj: Record<string, unknown>): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(obj)) {
    if (val === undefined || val === "") continue;
    result[key] = val;
  }
  return result;
}
