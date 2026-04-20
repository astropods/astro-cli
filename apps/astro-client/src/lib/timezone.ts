import { useState, useCallback, useEffect } from "react";

const TIMEZONE_STORAGE_KEY = "astro:log-timezone";

export function loadTimezone(): string {
  try {
    return localStorage.getItem(TIMEZONE_STORAGE_KEY) ?? "UTC";
  } catch {
    return "UTC";
  }
}

export function useLogTimezone() {
  const [timezone, setTimezoneState] = useState<string>(loadTimezone);

  const setTimezone = useCallback((tz: string) => {
    try { localStorage.setItem(TIMEZONE_STORAGE_KEY, tz); } catch { /* private browsing */ }
    setTimezoneState(tz);
  }, []);

  // Sync across tabs.
  useEffect(() => {
    function onStorage(e: StorageEvent) {
      if (e.key === TIMEZONE_STORAGE_KEY) setTimezoneState(loadTimezone());
    }
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  return { timezone, setTimezone };
}

// ── Timezone option list ───────────────────────────────────────────────────────

export interface TimezoneOption {
  value: string;
  label: string;
  offsetMinutes: number;
}

function parseOffsetMinutes(shortOffset: string): number {
  // shortOffset is like "GMT", "GMT+5", "GMT+5:30", "GMT-7"
  const m = shortOffset.match(/GMT([+-])(\d+):?(\d+)?/);
  if (!m) return 0;
  const sign = m[1] === "+" ? 1 : -1;
  return sign * (parseInt(m[2], 10) * 60 + parseInt(m[3] ?? "0", 10));
}

function formatOffsetLabel(shortOffset: string): string {
  // Normalize to "(GMT±HH:MM)" — e.g. "GMT+5:30" -> "(GMT+05:30)"
  if (shortOffset === "GMT") return "(GMT+00:00)";
  const m = shortOffset.match(/GMT([+-])(\d+):?(\d+)?/);
  if (!m) return `(${shortOffset})`;
  const sign = m[1];
  const h = m[2].padStart(2, "0");
  const min = (m[3] ?? "0").padStart(2, "0");
  return `(GMT${sign}${h}:${min})`;
}

function buildTimezoneOptions(): TimezoneOption[] {
  const now = new Date();
  const names: string[] = (Intl as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.("timeZone") ?? ["UTC"];
  const options: TimezoneOption[] = names.map((tz) => {
    const parts = new Intl.DateTimeFormat("en", {
      timeZone: tz,
      timeZoneName: "shortOffset",
    }).formatToParts(now);
    const shortOffset = parts.find((p) => p.type === "timeZoneName")?.value ?? "GMT";
    const offsetMinutes = parseOffsetMinutes(shortOffset);
    const label = `${formatOffsetLabel(shortOffset)} ${tz}`;
    return { value: tz, label, offsetMinutes };
  });
  options.sort((a, b) => a.offsetMinutes - b.offsetMinutes || a.value.localeCompare(b.value));
  return options;
}

export const TIMEZONE_OPTIONS: TimezoneOption[] = buildTimezoneOptions();
