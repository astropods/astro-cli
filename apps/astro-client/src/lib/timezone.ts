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

export interface TimezoneOption {
  value: string;
  label: string;
}

function buildTimezoneOptions(): TimezoneOption[] {
  const now = new Date();
  const names: string[] = (Intl as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.("timeZone") ?? ["UTC"];
  return names
    .map((tz) => {
      const parts = new Intl.DateTimeFormat("en", { timeZone: tz, timeZoneName: "longOffset" }).formatToParts(now);
      const offset = parts.find((p) => p.type === "timeZoneName")?.value ?? "GMT+00:00";
      const normalized = offset === "GMT" ? "GMT+00:00" : offset;
      const m = normalized.match(/([+-])(\d{2}):(\d{2})/);
      const sortKey = m ? (m[1] === "+" ? 1 : -1) * (parseInt(m[2], 10) * 60 + parseInt(m[3], 10)) : 0;
      return { value: tz, label: `(${normalized}) ${tz}`, sortKey };
    })
    .sort((a, b) => a.sortKey - b.sortKey || a.value.localeCompare(b.value))
    .map(({ value, label }) => ({ value, label }));
}

export const TIMEZONE_OPTIONS: TimezoneOption[] = buildTimezoneOptions();
