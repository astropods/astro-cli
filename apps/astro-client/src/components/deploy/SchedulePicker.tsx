import { useState, useCallback, useMemo } from "react";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export interface SchedulePickerProps {
  label: string;
  value: string;
  onChange: (cron: string) => void;
  error?: string;
}

interface Preset {
  label: string;
  cron: string;
}

const PRESETS: Preset[] = [
  { label: "Every 15 minutes", cron: "*/15 * * * *" },
  { label: "Every 30 minutes", cron: "*/30 * * * *" },
  { label: "Hourly", cron: "0 * * * *" },
  { label: "Every 6 hours", cron: "0 */6 * * *" },
  { label: "Daily at midnight", cron: "0 0 * * *" },
  { label: "Weekly on Sunday", cron: "0 0 * * 0" },
];

const CUSTOM_VALUE = "__custom__";

const DAYS_OF_WEEK = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

const parseCronField = (field: string, wildcard: string): string =>
  field === wildcard ? "*" : field;

const describeCron = (cron: string): string => {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return "";
  const [minute, hour, dom, month, dow] = parts;

  const segments: string[] = [];

  if (minute.startsWith("*/") && hour === "*" && dom === "*" && month === "*" && dow === "*") {
    return `Runs every ${minute.slice(2)} minutes`;
  }
  if (hour.startsWith("*/") && dom === "*" && month === "*" && dow === "*") {
    return `Runs every ${hour.slice(2)} hours at minute ${minute}`;
  }

  if (hour !== "*" && minute !== "*") {
    const h = parseInt(hour, 10);
    const m = parseInt(minute, 10);
    if (!isNaN(h) && !isNaN(m)) {
      const ampm = h >= 12 ? "PM" : "AM";
      const h12 = h === 0 ? 12 : h > 12 ? h - 12 : h;
      segments.push(`at ${h12}:${String(m).padStart(2, "0")} ${ampm}`);
    }
  } else if (minute !== "*") {
    segments.push(`at minute ${minute}`);
  }

  if (dow !== "*") {
    const idx = parseInt(dow, 10);
    if (!isNaN(idx) && idx >= 0 && idx <= 6) {
      segments.push(`on ${DAYS_OF_WEEK[idx]}s`);
    } else {
      segments.push(`on day-of-week ${dow}`);
    }
  }

  if (dom !== "*") {
    segments.push(`on day ${dom}`);
  }

  if (month !== "*") {
    const idx = parseInt(month, 10);
    if (!isNaN(idx) && idx >= 1 && idx <= 12) {
      segments.push(`in ${MONTHS[idx - 1]}`);
    } else {
      segments.push(`in month ${month}`);
    }
  }

  if (segments.length === 0) {
    if (hour === "*" && minute === "*") return "Runs every minute";
    return `Runs with schedule: ${cron}`;
  }

  return `Runs ${segments.join(" ")}`;
};

export function SchedulePicker({ label, value, onChange, error }: SchedulePickerProps) {
  const matchedPreset = useMemo(
    () => PRESETS.find((p) => p.cron === value.trim()),
    [value],
  );

  const [mode, setMode] = useState<"preset" | "custom">(() =>
    value && !matchedPreset ? "custom" : "preset",
  );

  const [customFields, setCustomFields] = useState<[string, string, string, string, string]>(() => {
    if (value && !matchedPreset) {
      const parts = value.trim().split(/\s+/);
      if (parts.length === 5) return parts as [string, string, string, string, string];
    }
    return ["*", "*", "*", "*", "*"];
  });

  const handlePresetChange = useCallback(
    (selected: string) => {
      if (selected === CUSTOM_VALUE) {
        setMode("custom");
        const assembled = customFields.join(" ");
        onChange(assembled === "* * * * *" ? "" : assembled);
        return;
      }
      setMode("preset");
      const preset = PRESETS.find((p) => p.cron === selected);
      if (preset) onChange(preset.cron);
    },
    [customFields, onChange],
  );

  const updateCustomField = useCallback(
    (index: number, fieldValue: string) => {
      setCustomFields((prev) => {
        const next = [...prev] as [string, string, string, string, string];
        next[index] = fieldValue;
        onChange(next.join(" "));
        return next;
      });
    },
    [onChange],
  );

  const selectValue = mode === "custom" ? CUSTOM_VALUE : (matchedPreset?.cron ?? "");
  const description = value.trim() ? describeCron(value) : "";

  return (
    <div className="space-y-3">
      <Label size="md">{label}</Label>
      <Select value={selectValue} onValueChange={handlePresetChange}>
        <SelectTrigger className="w-full" aria-invalid={!!error}>
          <SelectValue placeholder="Select a schedule" />
        </SelectTrigger>
        <SelectContent>
          {PRESETS.map((p) => (
            <SelectItem key={p.cron} value={p.cron}>
              {p.label}
            </SelectItem>
          ))}
          <SelectItem value={CUSTOM_VALUE}>Custom schedule</SelectItem>
        </SelectContent>
      </Select>

      {mode === "custom" && (
        <div className="space-y-3 rounded-md border p-4">
          <div className="grid grid-cols-5 gap-3">
            <CronFieldSelect
              label="Minute"
              value={parseCronField(customFields[0], "*")}
              onChange={(v) => updateCustomField(0, v)}
              options={minuteOptions}
            />
            <CronFieldSelect
              label="Hour"
              value={parseCronField(customFields[1], "*")}
              onChange={(v) => updateCustomField(1, v)}
              options={hourOptions}
            />
            <CronFieldSelect
              label="Day"
              value={parseCronField(customFields[2], "*")}
              onChange={(v) => updateCustomField(2, v)}
              options={domOptions}
            />
            <CronFieldSelect
              label="Month"
              value={parseCronField(customFields[3], "*")}
              onChange={(v) => updateCustomField(3, v)}
              options={monthOptions}
            />
            <CronFieldSelect
              label="Weekday"
              value={parseCronField(customFields[4], "*")}
              onChange={(v) => updateCustomField(4, v)}
              options={dowOptions}
            />
          </div>
          <div className="flex items-center gap-3">
            <Label className="shrink-0 text-muted-foreground text-xs">Cron expression</Label>
            <Input
              readOnly
              value={customFields.join(" ")}
              variant="code"
              className="h-8 text-xs"
            />
          </div>
        </div>
      )}

      {description && (
        <p className="text-sm text-muted-foreground">{description}</p>
      )}

      {error && (
        <p className="text-sm text-destructive">{error}</p>
      )}
    </div>
  );
}

interface CronFieldSelectProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
}

const CronFieldSelect = ({ label, value, onChange, options }: CronFieldSelectProps) => (
  <div>
    <Label className="text-xs mb-1 block">{label}</Label>
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 text-xs">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((opt) => (
          <SelectItem key={opt.value} value={opt.value}>
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  </div>
);

const minuteOptions = [
  { value: "*", label: "Every minute" },
  ...Array.from({ length: 60 }, (_, i) => ({
    value: String(i),
    label: String(i).padStart(2, "0"),
  })),
];

const hourOptions = [
  { value: "*", label: "Every hour" },
  ...Array.from({ length: 24 }, (_, i) => ({
    value: String(i),
    label: `${i === 0 ? 12 : i > 12 ? i - 12 : i} ${i >= 12 ? "PM" : "AM"}`,
  })),
];

const domOptions = [
  { value: "*", label: "Every day" },
  ...Array.from({ length: 31 }, (_, i) => ({
    value: String(i + 1),
    label: String(i + 1),
  })),
];

const monthOptions = [
  { value: "*", label: "Every month" },
  ...MONTHS.map((m, i) => ({ value: String(i + 1), label: m.slice(0, 3) })),
];

const dowOptions = [
  { value: "*", label: "Every day" },
  ...DAYS_OF_WEEK.map((d, i) => ({ value: String(i), label: d.slice(0, 3) })),
];
