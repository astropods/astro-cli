export function formatEvaluatorValue(value: unknown): string {
  if (typeof value === "boolean") {
    return value ? "True" : "False";
  }
  if (value === null || value === undefined || value === "") {
    return "—";
  }
  if (typeof value === "number") {
    return String(value);
  }
  const text = String(value).replace(/_/g, " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}
