import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import type { KnowledgeStore } from "@/lib/api";
import { server } from "@/test/msw/server";
import { renderRoute } from "@/test/test-utils";
import KnowledgeStores from "./KnowledgeStores";

function knowledgeStore(
  overrides: Partial<KnowledgeStore> = {},
): KnowledgeStore {
  return {
    id: "store-1",
    arn: "arn:astro:knowledge:store-1",
    name: "review-memory",
    provider: "postgres",
    mode: "managed",
    status: "ready",
    created_at: "2026-08-03T00:00:00Z",
    updated_at: "2026-08-03T00:00:00Z",
    ...overrides,
  };
}

function renderKnowledgeStores(stores: KnowledgeStore[]) {
  server.use(
    http.get("/api/v1/me/knowledge", () =>
      HttpResponse.json({
        stores,
        page: { limit: 50 },
        scope: { accounts: ["testuser"], all: true },
      }),
    ),
  );
  return renderRoute(
    [
      {
        path: "/knowledge",
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: KnowledgeStores as any,
      },
    ],
    { initialEntries: ["/knowledge"] },
  );
}

afterEach(cleanup);

describe("KnowledgeStores account ownership guard", () => {
  it("keeps a row without an account visible but non-interactive", async () => {
    renderKnowledgeStores([knowledgeStore()]);

    const row = (await screen.findByText("review-memory")).closest("tr");
    expect(row).not.toBeNull();
    if (!row) throw new Error("knowledge store row was not rendered");
    expect(row).not.toHaveAttribute("data-interactive");
    expect(within(row).getByText("Unavailable")).toBeInTheDocument();
    expect(within(row).queryByRole("button")).not.toBeInTheDocument();
  });

  it("keeps navigation and delete controls when the account is present", async () => {
    renderKnowledgeStores([knowledgeStore({ account: "testuser" })]);

    const row = (await screen.findByText("review-memory")).closest("tr");
    expect(row).not.toBeNull();
    if (!row) throw new Error("knowledge store row was not rendered");
    expect(row).toHaveAttribute("data-interactive", "true");
    expect(within(row).getByRole("button")).toBeInTheDocument();
  });
});

describe("KnowledgeStores search", () => {
  it("passes the debounced term to the full-scope server query", async () => {
    const user = userEvent.setup();
    const searches: string[] = [];
    server.use(
      http.get("/api/v1/me/knowledge", ({ request }) => {
        const url = new URL(request.url);
        searches.push(url.search);
        const q = url.searchParams.get("q");
        return HttpResponse.json({
          stores: q === "billing"
            ? [knowledgeStore({ name: "billing-memory", account: "testuser" })]
            : [knowledgeStore({ account: "testuser" })],
          page: { limit: 50 },
          scope: { accounts: ["testuser"], all: true },
        });
      }),
    );
    renderRoute(
      [{
        path: "/knowledge",
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: KnowledgeStores as any,
      }],
      { initialEntries: ["/knowledge"] },
    );

    await screen.findByText("review-memory");
    await user.type(screen.getByPlaceholderText("Search knowledge stores…"), "billing");

    await waitFor(() => expect(screen.getByText("billing-memory")).toBeInTheDocument());
    expect(searches.some((search) => new URLSearchParams(search).get("q") === "billing")).toBe(true);
  });
});
