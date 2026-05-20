import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  BLUEPRINT_PAGE_SIZE_COOKIE,
  parseCookieBlueprintPageSize,
  persistBlueprintPageSize,
  readStoredBlueprintPageSize,
} from './blueprint-page-size-preference';

function clearCookie(name: string) {
  document.cookie = `${name}=;path=/;max-age=0`;
}

describe('parseCookieBlueprintPageSize', () => {
  it('defaults to 50 when the cookie is missing', () => {
    expect(parseCookieBlueprintPageSize(null)).toBe(50);
  });

  it('reads a valid cookie value', () => {
    expect(parseCookieBlueprintPageSize(`${BLUEPRINT_PAGE_SIZE_COOKIE}=20`)).toBe(20);
  });

  it('ignores invalid cookie values', () => {
    expect(parseCookieBlueprintPageSize(`${BLUEPRINT_PAGE_SIZE_COOKIE}=99`)).toBe(50);
  });
});

describe('persistBlueprintPageSize', () => {
  beforeEach(() => {
    localStorage.clear();
    clearCookie(BLUEPRINT_PAGE_SIZE_COOKIE);
  });

  afterEach(() => {
    localStorage.clear();
    clearCookie(BLUEPRINT_PAGE_SIZE_COOKIE);
  });

  it('writes localStorage and cookie together', () => {
    persistBlueprintPageSize(20);
    expect(readStoredBlueprintPageSize()).toBe(20);
    expect(parseCookieBlueprintPageSize(document.cookie)).toBe(20);
  });
});
