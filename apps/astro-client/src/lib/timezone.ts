import { useState, useCallback } from "react";
import { useAuth } from "./auth";

function storageKey(userId: string): string {
  return `astro:log-timezone:${userId}`;
}

export function useLogTimezone() {
  const { personalAccount } = useAuth();
  const userId = personalAccount!.id;
  const [timezone, setTimezoneState] = useState<string>(() => {
    try {
      return localStorage.getItem(storageKey(userId)) ?? "UTC";
    } catch {
      return "UTC";
    }
  });

  const setTimezone = useCallback((tz: string) => {
    try { localStorage.setItem(storageKey(userId), tz); } catch { /* private browsing */ }
    setTimezoneState(tz);
  }, [userId]);

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
