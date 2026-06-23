import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { Outlet } from "react-router";
import { server } from "@/test/msw/server";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import type {
  EvalDatasetItem,
  EvalDatasetItemsResponse,
  EvalDatasetResponse,
} from "@/lib/api";
import AgentDataset from "./AgentDataset";

afterEach(cleanup);
afterEach(() => server.resetHandlers());

function makeDatasetResponse(
  overrides?: Partial<EvalDatasetResponse>,
): EvalDatasetResponse {
  return {
    dataset_name: "dep-test-deployment",
    item_count: 42,
    good_count: 30,
    bad_count: 12,
    grade: "B",
    next_grade: "A",
    next_grade_progress: 0.6,
    ...overrides,
  };
}

function emptyItems(): EvalDatasetItemsResponse {
  return { items: [], page: 1, limit: 50, total_items: 0, total_pages: 0 };
}

function itemsResponse(items: EvalDatasetItem[]): EvalDatasetItemsResponse {
  return {
    items,
    page: 1,
    limit: 50,
    total_items: items.length,
    total_pages: 1,
  };
}

function datasetItem(overrides: Partial<EvalDatasetItem>): EvalDatasetItem {
  return {
    id: "item-1",
    input: "input",
    expected_output: "output",
    metadata: { verdict: 1 },
    source_trace_id: "trace-1",
    created_at: "2026-06-01T12:00:00Z",
    ...overrides,
  };
}

function setupDataset(
  response: EvalDatasetResponse | { status: number },
  items: EvalDatasetItemsResponse = emptyItems(),
) {
  server.use(
    http.get("/api/v1/deployments/:id/dataset", () => {
      if ("status" in response) {
        return new HttpResponse(null, { status: response.status });
      }
      return HttpResponse.json(response);
    }),
    http.get("/api/v1/deployments/:id/dataset/items", () =>
      HttpResponse.json(items),
    ),
    http.get("/api/v1/accounts/:account/members", () =>
      HttpResponse.json({ members: [] }),
    ),
  );
}

function renderDataset(deploymentId = "dep-test") {
  return renderRoute(
    [
      {
        path: "/:account/agents/:deploymentId",
        Component: () => (
          <Outlet
            context={{
              deployment: { id: deploymentId },
              account: "testuser",
              deploymentId,
            }}
          />
        ),
        children: [{ path: "dataset", Component: AgentDataset }],
      },
    ],
    {
      initialEntries: [`/testuser/agents/${deploymentId}/dataset`],
      auth: mockAuthContext,
    },
  );
}

describe("loading state", () => {
  it("shows a spinner while the request is in flight", () => {
    server.use(
      http.get("/api/v1/deployments/:id/dataset", () => new Promise(() => {})),
    );
    const { container } = renderDataset();
    expect(container.querySelector(".dp-spin")).toBeInTheDocument();
  });
});

describe("error state", () => {
  it("shows unavailable message on API error", async () => {
    setupDataset({ status: 404 });
    renderDataset();
    expect(
      await screen.findByText(/dataset not available/i),
    ).toBeInTheDocument();
  });
});

describe("summary view", () => {
  it("renders the grade letter, dataset name, and counts", async () => {
    setupDataset(makeDatasetResponse());
    renderDataset();
    await waitFor(() => {
      expect(screen.getAllByLabelText(/grade b/i).length).toBeGreaterThan(0);
    });
    expect(screen.getAllByText("dep-test-deployment").length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText(/composition · 42/i)).toBeInTheDocument();
    expect(screen.getByText(/30 good/)).toBeInTheDocument();
    expect(screen.getByText(/12 bad/)).toBeInTheDocument();
  });

  it("shows download button when data is loaded", async () => {
    setupDataset(makeDatasetResponse());
    renderDataset();
    expect(
      await screen.findByRole("link", { name: /download/i }),
    ).toBeInTheDocument();
  });

  it("does not show download button while loading", () => {
    server.use(
      http.get("/api/v1/deployments/:id/dataset", () => new Promise(() => {})),
    );
    renderDataset();
    expect(
      screen.queryByRole("link", { name: /download/i }),
    ).not.toBeInTheDocument();
  });

  it("formats large item counts with locale separators", async () => {
    setupDataset(
      makeDatasetResponse({
        item_count: 12345,
        good_count: 12000,
        bad_count: 345,
      }),
    );
    renderDataset();
    await waitFor(() => {
      expect(screen.getByText(/composition · 12,345/i)).toBeInTheDocument();
    });
  });

  it("renders an em-dash grade when the dataset is empty", async () => {
    setupDataset(
      makeDatasetResponse({
        item_count: 0,
        good_count: 0,
        bad_count: 0,
        grade: "—",
        next_grade: "",
        next_grade_progress: 0,
      }),
    );
    renderDataset();
    expect(await screen.findByText(/no baseline yet/i)).toBeInTheDocument();
  });

  it("uses summary verdict counts in filter chips", async () => {
    setupDataset(
      makeDatasetResponse({ good_count: 30, bad_count: 12 }),
      itemsResponse([
        datasetItem({ id: "good-1", metadata: { verdict: 1 } }),
        datasetItem({ id: "bad-1", metadata: { verdict: -1 } }),
        datasetItem({ id: "bad-2", metadata: { verdict: -1 } }),
      ]),
    );
    renderDataset();
    await waitFor(() => {
      const goodButton = screen
        .getAllByRole("button", { name: /good/i })
        .find((el) => el.tagName === "BUTTON");
      const badButton = screen
        .getAllByRole("button", { name: /bad/i })
        .find((el) => el.tagName === "BUTTON");
      expect(goodButton).toHaveTextContent("30");
      expect(badButton).toHaveTextContent("12");
    });
    expect(screen.getByText(/30 good/)).toBeInTheDocument();
    expect(screen.getByText(/12 bad/)).toBeInTheDocument();
  });

  it("requests server-filtered items with cursor pagination", async () => {
    const seenRequests: Array<{ verdict: string; cursor: string; page: string }> = [];
    let goodRequests = 0;
    setupDataset(makeDatasetResponse({ good_count: 30, bad_count: 12 }));
    server.use(
      http.get("/api/v1/deployments/:id/dataset/items", ({ request }) => {
        const params = new URL(request.url).searchParams;
        const verdict = params.get("verdict") ?? "";
        seenRequests.push({
          verdict,
          cursor: params.get("cursor") ?? "",
          page: params.get("page") ?? "",
        });
        if (verdict === "good") {
          goodRequests += 1;
          return HttpResponse.json({
            ...itemsResponse([
              datasetItem({
                id: `good-${goodRequests}`,
                metadata: { verdict: 1 },
              }),
            ]),
            total_items: 2,
            total_pages: 2,
            next_cursor: goodRequests === 1 ? "cursor-1" : undefined,
          });
        }
        return HttpResponse.json(emptyItems());
      }),
    );

    const user = userEvent.setup();
    renderDataset();
    let goodButton: HTMLElement | undefined;
    await waitFor(() => {
      goodButton = screen
        .getAllByRole("button", { name: /good/i })
        .find((el) => el.tagName === "BUTTON");
      expect(goodButton).toBeTruthy();
    });
    await user.click(goodButton!);

    await waitFor(() => {
      expect(seenRequests).toContainEqual({ verdict: "good", cursor: "", page: "" });
    });

    await user.click(await screen.findByRole("button", { name: /show 1 more/i }));

    await waitFor(() => {
      expect(seenRequests).toContainEqual({ verdict: "good", cursor: "cursor-1", page: "" });
    });
  });
});
