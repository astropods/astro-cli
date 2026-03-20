const CRON_FIELD_RANGES: [number, number][] = [
  [0, 59],
  [0, 23],
  [1, 31],
  [1, 12],
  [0, 6],
];

export const isValidCronField = (field: string, min: number, max: number): boolean => {
  if (field === "*") return true;
  if (field.startsWith("*/")) {
    const step = parseInt(field.slice(2), 10);
    return !isNaN(step) && step >= 1 && step <= max;
  }
  const num = parseInt(field, 10);
  return !isNaN(num) && num >= min && num <= max;
};

export const isValidCron = (cron: string): boolean => {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return false;
  return parts.every((field, i) =>
    isValidCronField(field, CRON_FIELD_RANGES[i][0], CRON_FIELD_RANGES[i][1]),
  );
};
