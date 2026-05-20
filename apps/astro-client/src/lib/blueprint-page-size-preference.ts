import { readCookieValue } from '@/lib/active-account';
import {
  BLUEPRINT_LIST_DEFAULT_PAGE_SIZE,
  BLUEPRINT_PAGE_SIZE_OPTIONS,
  type BlueprintPageSize,
} from '@/lib/blueprint-list-params';

export const BLUEPRINT_PAGE_SIZE_STORAGE_KEY = 'astro:blueprints:page-size';
export const BLUEPRINT_PAGE_SIZE_COOKIE = 'astro-blueprints-page-size';
const COOKIE_MAX_AGE = 31536000;

function parsePageSize(raw: string | null | undefined): BlueprintPageSize | null {
  if (!raw) return null;
  const parsed = Number(raw);
  if (BLUEPRINT_PAGE_SIZE_OPTIONS.includes(parsed as BlueprintPageSize)) {
    return parsed as BlueprintPageSize;
  }
  return null;
}

export function parseCookieBlueprintPageSize(
  cookieHeader: string | null | undefined,
): BlueprintPageSize {
  const fromCookie = parsePageSize(readCookieValue(cookieHeader, BLUEPRINT_PAGE_SIZE_COOKIE));
  return fromCookie ?? BLUEPRINT_LIST_DEFAULT_PAGE_SIZE;
}

export function readStoredBlueprintPageSize(): BlueprintPageSize {
  try {
    const fromStorage = parsePageSize(localStorage.getItem(BLUEPRINT_PAGE_SIZE_STORAGE_KEY));
    if (fromStorage) return fromStorage;
  } catch {
    // localStorage unavailable
  }
  return BLUEPRINT_LIST_DEFAULT_PAGE_SIZE;
}

export function persistBlueprintPageSize(size: BlueprintPageSize) {
  try {
    localStorage.setItem(BLUEPRINT_PAGE_SIZE_STORAGE_KEY, String(size));
  } catch {
    // ignore
  }
  if (typeof document !== 'undefined') {
    document.cookie = `${BLUEPRINT_PAGE_SIZE_COOKIE}=${size};path=/;max-age=${COOKIE_MAX_AGE};SameSite=Lax`;
  }
}
