import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  setPersistentStorageSnapshot,
  subscribePersistentStorage,
} from "@/lib/persistent-storage";
import { usePersistentSearchParams } from "./use-persistent-search-params";

const ACCOUNT_PARAMS = ["account"] as const;
const SEARCH_PARAMS = ["search"] as const;
const INSIGHTS_PARAMS = [
  "account",
  "range",
  "view",
  "hide_sources",
] as const;
const STORAGE_KEY = "astro:page-filters:agents";

describe("usePersistentSearchParams", () => {
  afterEach(cleanup);

  it("preserves params owned by another instance on the same route", async () => {
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter initialEntries={["/agents/?account=acme&search=old"]}>
        {children}
      </MemoryRouter>
    );
    const { result } = renderHook(
      () => {
        usePersistentSearchParams("agents", ACCOUNT_PARAMS);
        return usePersistentSearchParams("agents", SEARCH_PARAMS);
      },
      { wrapper },
    );

    await waitFor(() => {
      const stored = new URLSearchParams(localStorage.getItem(STORAGE_KEY) ?? "");
      expect(stored.get("account")).toBe("acme");
      expect(stored.get("search")).toBe("old");
    });

    const onStorageChange = vi.fn();
    const unsubscribe = subscribePersistentStorage(
      STORAGE_KEY,
      onStorageChange,
    );
    act(() => result.current[1]({ search: "new" }));

    let stored = new URLSearchParams(localStorage.getItem(STORAGE_KEY) ?? "");
    expect(result.current[0].get("search")).toBe("new");
    expect(stored.get("account")).toBe("acme");
    expect(stored.get("search")).toBe("new");
    expect(onStorageChange).toHaveBeenCalledTimes(1);

    onStorageChange.mockClear();
    act(() =>
      result.current[1]((current) => {
        current.set("search", "functional");
        return current;
      }),
    );

    stored = new URLSearchParams(localStorage.getItem(STORAGE_KEY) ?? "");
    expect(result.current[0].get("search")).toBe("functional");
    expect(stored.get("account")).toBe("acme");
    expect(stored.get("search")).toBe("functional");
    expect(onStorageChange).toHaveBeenCalledTimes(1);
    unsubscribe();
  });

  it("restores omitted filters without persisting URL search text", async () => {
    const storageKey = "astro:page-filters:insights";
    setPersistentStorageSnapshot(
      storageKey,
      "range=30d&view=users&hide_sources=agents",
    );
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter initialEntries={["/insights?account=acme&q=typed"]}>
        {children}
      </MemoryRouter>
    );

    const { result } = renderHook(
      () => usePersistentSearchParams("insights", INSIGHTS_PARAMS),
      { wrapper },
    );

    await waitFor(() => {
      const stored = new URLSearchParams(
        localStorage.getItem(storageKey) ?? "",
      );
      expect(Object.fromEntries(stored)).toEqual({
        range: "30d",
        view: "users",
        hide_sources: "agents",
        account: "acme",
      });
      expect(Object.fromEntries(result.current[0])).toEqual({
        account: "acme",
        q: "typed",
        range: "30d",
        view: "users",
        hide_sources: "agents",
      });
    });
  });
});
