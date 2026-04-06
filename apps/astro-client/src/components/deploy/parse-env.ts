export interface ParsedEnvLine {
  name: string;
  value: string;
  valid: boolean;
}

/**
 * Parse .env-formatted text into per-line results.
 * Handles comments, blank lines, quoted values, and inline comments.
 * Lines without `=` or with empty keys are marked invalid.
 */
export function parseEnvLines(text: string): ParsedEnvLine[] {
  const lines: ParsedEnvLine[] = [];

  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;

    const eqIndex = line.indexOf("=");
    if (eqIndex === -1) {
      lines.push({ name: line, value: "", valid: false });
      continue;
    }

    const key = line.slice(0, eqIndex).trim();
    let value = line.slice(eqIndex + 1).trim();

    if (!key) {
      lines.push({ name: "", value, valid: false });
      continue;
    }

    // Track and strip surrounding quotes (double or single)
    const wasQuoted =
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"));
    if (wasQuoted) {
      value = value.slice(1, -1);
    }

    // Strip inline comments (only for values that were originally unquoted)
    const commentIndex = value.indexOf(" #");
    if (commentIndex !== -1 && !wasQuoted) {
      value = value.slice(0, commentIndex).trim();
    }

    lines.push({ name: key, value, valid: true });
  }

  return lines;
}

/**
 * Parse .env-formatted text into a flat key-value map.
 * Handles comments, blank lines, and quoted values.
 */
export function parseEnvText(text: string): Record<string, string> {
  const result: Record<string, string> = {};
  for (const { name, value, valid } of parseEnvLines(text)) {
    if (valid) result[name] = value;
  }
  return result;
}

/**
 * Auto-detect format (JSON or .env) and parse into a flat key-value map.
 * JSON values that aren't strings are stringified.
 */
export function parseVariables(text: string): Record<string, string> {
  const trimmed = text.trim();
  if (!trimmed) return {};

  // Try JSON first
  if (trimmed.startsWith("{")) {
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
        const result: Record<string, string> = {};
        for (const [key, value] of Object.entries(parsed)) {
          result[key] = typeof value === "string" ? value : JSON.stringify(value);
        }
        return result;
      }
    } catch {
      // Fall through to .env parser
    }
  }

  return parseEnvText(trimmed);
}
