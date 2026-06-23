export interface ParsedContent {
  /** Parsed JSON (object/array/primitive). Null when not JSON. */
  json: unknown;
  /** Plain text fallback when not JSON. */
  text: string;
  /** Plain-text representation for the copy button (always populated). */
  copyText: string;
  isJson: boolean;
  isEmpty: boolean;
}

export function safeStringify(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

/**
 * Decide whether the content is JSON or plain text.
 * Objects / arrays passed in directly are treated as JSON.
 * Strings get a JSON.parse attempt when they look JSON-shaped.
 */
export function parseContent(value: unknown): ParsedContent {
  if (value == null) {
    return { json: null, text: "", copyText: "", isJson: false, isEmpty: true };
  }

  if (typeof value === "object") {
    return {
      json: value,
      text: "",
      copyText: safeStringify(value),
      isJson: true,
      isEmpty: false,
    };
  }

  const str = String(value);
  if (!str) {
    return { json: null, text: "", copyText: "", isJson: false, isEmpty: true };
  }

  const trimmed = str.trim();
  if (
    (trimmed.startsWith("{") && trimmed.endsWith("}")) ||
    (trimmed.startsWith("[") && trimmed.endsWith("]"))
  ) {
    try {
      const parsed = JSON.parse(trimmed);
      return {
        json: parsed,
        text: "",
        copyText: safeStringify(parsed),
        isJson: true,
        isEmpty: false,
      };
    } catch {
      // fall through to plain text
    }
  }

  return { json: null, text: str, copyText: str, isJson: false, isEmpty: false };
}
