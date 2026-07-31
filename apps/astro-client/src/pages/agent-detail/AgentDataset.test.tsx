import { describe, it, expect, afterEach, vi } from "vitest";
import { act, screen, cleanup, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { Outlet } from "react-router";
import { server } from "@/test/msw/server";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import { evalKeys } from "@/api/queries/keys";
import type {
  DatasetJudgmentRequest,
  EvalDatasetItem,
  EvalDatasetItemsResponse,
  EvalDatasetResponse,
  PredictionStatusCounts,
  ReviewQueueItem,
  ReviewQueueResponse,
  TraceDetailResponse,
} from "@/lib/api";
import AgentDataset from "./AgentDataset";

afterEach(cleanup);
afterEach(() => {
  reviewQueueFixtures.clear();
  server.resetHandlers();
});
afterEach(() => vi.restoreAllMocks());

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
    cases_to_next_grade: 100,
    criteria_counts: [],
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

function reviewQueueResponse(
  items: ReviewQueueItem[],
  overrides: Partial<ReviewQueueResponse> = {},
): ReviewQueueResponse {
  return {
    items,
    ...overrides,
  };
}

function predictionStatusCounts(
  items: ReviewQueueItem[],
): PredictionStatusCounts {
  const counts = {
    queued: 0,
    in_progress: 0,
    completed: 0,
    failed: 0,
  };
  for (const item of items) {
    if (item.prediction) {
      counts.completed += 1;
    } else if (item.prediction_status !== "not_requested") {
      counts[item.prediction_status] += 1;
    }
  }
  return counts;
}

function mockReviewQueueRequest(
  resolve: (url: URL) => ReviewQueueResponse | Promise<ReviewQueueResponse>,
) {
  server.use(
    http.get(
      "/api/v1/deployments/:id/dataset/review-queue",
      async ({ request }) =>
        HttpResponse.json(await resolve(new URL(request.url))),
    ),
  );
}

function mockCreateJudgment(
  onCreate?: (judgment: DatasetJudgmentRequest) => void,
) {
  server.use(
    http.post(
      "/api/v1/deployments/:id/dataset/judgments",
      async ({ request }) => {
        const judgment = (await request.json()) as DatasetJudgmentRequest;
        onCreate?.(judgment);
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: judgment.trace_id,
            verdict: judgment.verdict,
          },
          { status: 201 },
        );
      },
    ),
  );
}

const REVIEW_QUEUE_PAGE_SIZE = "50";

const reviewQueueFixtures = new Map<string, ReviewQueueItem>();

function queueItem(overrides: Partial<ReviewQueueItem>): ReviewQueueItem {
  const item: ReviewQueueItem = {
    trace_id: "trace_000001",
    timestamp: "2026-06-01T12:00:00Z",
    input: "How do I deploy?",
    output: "Run ast deploy.",
    sentiment: "positive",
    prediction_status: "not_requested",
    prediction_error: null,
    prediction: null,
    ...overrides,
  };
  reviewQueueFixtures.set(item.trace_id, item);
  return item;
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
  queue: ReviewQueueResponse = reviewQueueResponse([]),
  predictionStatus: PredictionStatusCounts = predictionStatusCounts(queue.items),
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
    http.get("/api/v1/deployments/:id/dataset/review-queue", () =>
      HttpResponse.json(queue),
    ),
    http.get("/api/v1/deployments/:id/dataset/predictions/status", () =>
      HttpResponse.json(predictionStatus),
    ),
    http.get(
      "/api/v1/deployments/:id/observability/traces/:traceId",
      ({ params }) => {
        const traceId = String(params.traceId);
        const item = reviewQueueFixtures.get(traceId);
        if (!item) {
          return HttpResponse.json({ error: "not found" }, { status: 404 });
        }
        return HttpResponse.json<TraceDetailResponse>({
          trace: {
            trace_id: item.trace_id,
            name: item.trace_id,
            timestamp: item.timestamp,
            latency_ms: 0,
            total_cost: 0,
            input: item.input,
            output: item.output,
          },
          observations: [],
          scores: [],
        });
      },
    ),
    http.get("/api/v1/accounts/:account/members", () =>
      HttpResponse.json({ members: [] }),
    ),
  );
}

function renderDataset({
  deploymentId = "dep-test",
  tab = "dataset",
}: {
  deploymentId?: string;
  tab?: "queue" | "dataset" | null;
} = {}) {
  const query = tab ? `?tab=${tab}` : "";
  return renderRoute(
    [
      {
        path: "/:account/agents/:deploymentId",
        Component: () => (
          <Outlet
            context={{
              deployment: {
                id: deploymentId,
                name: "cruise-line",
                display_name: "Cruise Line",
              },
              account: "testuser",
              deploymentId,
            }}
          />
        ),
        children: [{ path: "dataset", Component: AgentDataset }],
      },
    ],
    {
      initialEntries: [`/testuser/agents/${deploymentId}/dataset${query}`],
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

describe("review queue view", () => {
  it("shows an empty queue message when there are no traces to review", async () => {
    setupDataset(makeDatasetResponse(), emptyItems(), reviewQueueResponse([]));

    renderDataset({ tab: null });

    expect(
      await screen.findByText("No traces waiting for review."),
    ).toBeInTheDocument();
    expect(screen.getByText("You're all caught up")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /load more items/i }),
    ).not.toBeInTheDocument();
  });

  it("asks the server to enqueue the most recent unjudged traces", async () => {
    let postedBody: string | null = null;

    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([queueItem({})]),
    );
    server.use(
      http.post(
        "/api/v1/deployments/:id/dataset/predictions",
        async ({ request }) => {
          postedBody = await request.text();
          return HttpResponse.json(
            {
              enqueued_trace_ids: ["trace-1"],
              failed_trace_ids: [],
            },
            { status: 202 },
          );
        },
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await user.hover(
      await screen.findByRole("button", { name: "Run AI Judge" }),
    );

    const tooltip = await screen.findByRole("tooltip");
    expect(
      within(tooltip).getByRole("heading", {
        name: "Automatically judge traces",
      }),
    ).toBeInTheDocument();
    expect(
      within(tooltip).getByText(
        "The judge will score up to 50 of the most recent unjudged traces. You can confirm each verdict in the queue.",
      ),
    ).toBeInTheDocument();
    expect(
      within(tooltip).getByText("Estimated ~500 credits"),
    ).toBeInTheDocument();
    expect(
      within(tooltip).getByRole("link", { name: "View usage" }),
    ).toHaveAttribute("href", "/settings/billing");

    await user.click(screen.getByRole("button", { name: "Run AI Judge" }));

    await waitFor(() => {
      expect(postedBody).toBe("");
    });
  });

  it("disables the judge button while predictions are active", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace-1",
          prediction_status: "queued",
        }),
        queueItem({
          trace_id: "trace-2",
          prediction_status: "in_progress",
        }),
        queueItem({
          trace_id: "trace-3",
          prediction_status: "not_requested",
        }),
      ]),
    );

    renderDataset({ tab: null });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Judging 2 items" }),
      ).toHaveAttribute("aria-disabled", "true");
    });
  });

  it("disables the judge button when the loaded queue has nothing to judge", async () => {
    setupDataset(makeDatasetResponse(), emptyItems(), reviewQueueResponse([]));

    renderDataset({ tab: null });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Run AI Judge" }),
      ).toHaveAttribute("aria-disabled", "true");
    });
  });

  it("keeps the judge button enabled for a prediction-filtered queue", async () => {
    setupDataset(makeDatasetResponse(), emptyItems(), reviewQueueResponse([]));
    mockReviewQueueRequest((url) =>
      reviewQueueResponse(
        url.searchParams.get("prediction") === "good"
          ? [
              queueItem({
                trace_id: "trace-predicted-good",
                input: "Predicted good prompt",
                output: "Predicted good response",
                prediction_status: "completed",
                prediction: {
                  verdict_score: 0.8,
                  confidence: 82,
                  explanation: "The response addressed the request.",
                  judge_version: "1",
                  criteria: [],
                },
              }),
            ]
          : [],
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await user.click(
      await screen.findByRole("combobox", { name: "Filter review queue" }),
    );
    await user.click(screen.getByRole("option", { name: "Good" }));

    expect(await screen.findByText("Predicted good response")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Run AI Judge" }),
    ).toHaveAttribute("aria-disabled", "false");
  });

  it("refreshes the selected queue while prediction status is active", async () => {
    let predictionStatus: PredictionStatusCounts = {
      queued: 0,
      in_progress: 0,
      completed: 0,
      failed: 0,
    };
    let queueRequestCount = 0;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([]),
      predictionStatus,
    );
    server.use(
      http.get("/api/v1/deployments/:id/dataset/predictions/status", () =>
        HttpResponse.json(predictionStatus),
      ),
    );
    mockReviewQueueRequest((url) => {
      queueRequestCount += 1;
      const prediction = url.searchParams.get("prediction");
      return reviewQueueResponse([
        queueItem({
          trace_id: `trace-${prediction ?? "all"}`,
          output: `${prediction ?? "all"} response`,
          prediction_status: "in_progress",
        }),
      ]);
    });

    const user = userEvent.setup();
    const { queryClient } = renderDataset({ tab: null });

    expect(
      await screen.findByRole("button", { name: "Run AI Judge" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(queueRequestCount).toBe(1));

    predictionStatus = {
      queued: 0,
      in_progress: 1,
      completed: 0,
      failed: 0,
    };
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: evalKeys.predictionStatus("dep-test"),
      });
    });

    const judgingButton = await screen.findByRole("button", {
      name: "Judging 1 item",
    });
    expect(judgingButton).toHaveAttribute("aria-disabled", "true");
    expect(
      judgingButton.querySelector(".dp-judging-gavel"),
    ).toBeInTheDocument();
    await waitFor(() => expect(queueRequestCount).toBe(2));

    await user.click(
      screen.getByRole("combobox", { name: "Filter review queue" }),
    );
    await user.click(screen.getByRole("option", { name: "Good" }));
    expect(
      screen.getByRole("button", { name: "Judging 1 item" }),
    ).toHaveAttribute("aria-disabled", "true");
    await waitFor(() => expect(queueRequestCount).toBe(3));

    predictionStatus = {
      queued: 0,
      in_progress: 0,
      completed: 1,
      failed: 0,
    };
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: evalKeys.predictionStatus("dep-test"),
      });
    });

    expect(
      await screen.findByRole("button", { name: "Run AI Judge" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(queueRequestCount).toBe(4));
  });

  it("shows queue errors independently from the dataset summary", async () => {
    setupDataset(makeDatasetResponse(), emptyItems(), reviewQueueResponse([]));
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", () =>
        HttpResponse.json({ error: "nope" }, { status: 500 }),
      ),
    );

    renderDataset({ tab: null });

    expect(await screen.findByText("Queue unavailable")).toBeInTheDocument();
    expect(screen.getByText("Failed to load the queue.")).toBeInTheDocument();
  });

  it.each([
    {
      grade: "A",
      label: "Strong coverage",
      tooltip:
        "You've labeled a representative sample. Keep going to capture edge cases and strengthen future evals.",
      nextGrade: "",
      nextGradeProgress: 1,
    },
    {
      grade: "B",
      label: "Good coverage",
      tooltip:
        "You've labeled a solid sample of traces. Keep going to capture edge cases and push toward an A.",
      nextGrade: "A",
      nextGradeProgress: 0.6,
    },
    {
      grade: "C",
      label: "Enough coverage",
      tooltip:
        "You've labeled enough traces to get started. Keep going to improve coverage and reliability.",
      nextGrade: "B",
      nextGradeProgress: 0.4,
    },
  ])(
    "shows $label in the queue header for a $grade grade",
    async ({ grade, label, tooltip, nextGrade, nextGradeProgress }) => {
      setupDataset(
        makeDatasetResponse({
          item_count: 48,
          good_count: 38,
          bad_count: 10,
          grade,
          next_grade: nextGrade,
          next_grade_progress: nextGradeProgress,
        }),
        emptyItems(),
        reviewQueueResponse([
          queueItem({
            trace_id: "trace_111111",
            input: "Ready prompt",
            output: "Ready response",
          }),
        ]),
      );

      const user = userEvent.setup();
      renderDataset({ tab: null });

      expect(await screen.findByText("Ready response")).toBeInTheDocument();
      await user.hover(screen.getByText(label));
      const tooltipText = await screen.findAllByText(tooltip);
      expect(tooltipText.length).toBeGreaterThan(0);
      expect(document.querySelector('[data-slot="tooltip-content"]')).toHaveAttribute(
        "data-side",
        "top",
      );
    },
  );

  it("hides the baseline cue before the dataset reaches a C grade", async () => {
    setupDataset(
      makeDatasetResponse({
        item_count: 100,
        good_count: 58,
        bad_count: 42,
        grade: "D",
        next_grade: "C",
        next_grade_progress: 0.7,
      }),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Needs more prompt",
          output: "Needs more response",
        }),
      ]),
    );

    renderDataset({ tab: null });

    expect(await screen.findByText("Needs more response")).toBeInTheDocument();
    expect(screen.queryByText("Enough coverage")).not.toBeInTheDocument();
    expect(screen.queryByText("Good coverage")).not.toBeInTheDocument();
    expect(screen.queryByText("Strong coverage")).not.toBeInTheDocument();
  });

  it("defaults the clean dataset URL to the review queue", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "First prompt",
          output: "First response",
          sentiment: "positive",
        }),
      ]),
    );

    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    expect(screen.getAllByText("First prompt").length).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: /view trace_111111/i }),
    ).toBeInTheDocument();
  });

  it("switches from the default queue tab to the dataset tab", async () => {
    setupDataset(
      makeDatasetResponse({
        item_count: 0,
        good_count: 0,
        bad_count: 0,
        grade: "—",
        next_grade: "",
        next_grade_progress: 0,
      }),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Queued prompt",
          output: "Queued response",
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Queued response")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^dataset/i }));

    expect(await screen.findByText("No items yet.")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText("Queued response")).not.toBeInTheDocument();
    });
  });

  it("navigates between traces from the detail header", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "First prompt",
          output: "First response",
        }),
        queueItem({
          trace_id: "trace_222222",
          input: "Second prompt",
          output: "Second response",
          sentiment: "negative",
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 1 of 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous trace" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Next trace" }));
    expect(screen.getByText("Second response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 2 of 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next trace" })).toBeDisabled();
  });

  it("passes the selected prediction filter to the review queue", async () => {
    const requestedFilters: Array<string | null> = [];
    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest((url) => {
      const prediction = url.searchParams.get("prediction");
      requestedFilters.push(prediction);
      return reviewQueueResponse([
        queueItem({
          trace_id: `trace_${prediction ?? "all"}`,
          input: `${prediction ?? "all"} prompt`,
          output: `${prediction ?? "all"} response`,
        }),
      ]);
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("all response")).toBeInTheDocument();

    for (const [label, prediction] of [
      ["Good", "good"],
      ["Bad", "bad"],
      ["Not sure", "unknown"],
      ["Not judged", "none"],
    ] as const) {
      await user.click(
        screen.getByRole("combobox", { name: "Filter review queue" }),
      );
      await user.click(screen.getByRole("option", { name: label }));
      expect(
        await screen.findByText(`${prediction} response`),
      ).toBeInTheDocument();
    }

    expect(requestedFilters).toEqual([null, "good", "bad", "unknown", "none"]);
  });

  it("loads additional queue pages with cursors", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "First paged prompt",
      output: "First paged response",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Second paged prompt",
      output: "Second paged response",
      sentiment: "negative",
    });
    const third = queueItem({
      trace_id: "trace_333333",
      input: "Third paged prompt",
      output: "Third paged response",
    });
    const requests: Array<{
      cursor: string | null;
      limit: string | null;
    }> = [];

    setupDataset(makeDatasetResponse(), emptyItems());
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", ({ request }) => {
        const url = new URL(request.url);
        const cursor = url.searchParams.get("cursor");
        requests.push({
          cursor,
          limit: url.searchParams.get("limit"),
        });

        if (cursor === "cursor-2") {
          return HttpResponse.json(reviewQueueResponse([third]));
        }

        if (cursor === "cursor-1") {
          return HttpResponse.json(
            reviewQueueResponse([second], {
              next_cursor: "cursor-2",
            }),
          );
        }

        return HttpResponse.json(
          reviewQueueResponse([first], {
            next_cursor: "cursor-1",
          }),
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First paged response")).toBeInTheDocument();
    await user.click(
      screen.getAllByRole("button", { name: /load more items/i })[0],
    );
    await user.click(
      await screen.findByRole("button", { name: /second paged prompt/i }),
    );

    expect(screen.getByText("Second paged response")).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: /load more items/i }),
    );
    await user.click(
      await screen.findByRole("button", { name: /third paged prompt/i }),
    );

    expect(screen.getByText("Third paged response")).toBeInTheDocument();
    expect(requests).toContainEqual({
      cursor: null,
      limit: REVIEW_QUEUE_PAGE_SIZE,
    });
    expect(requests).toContainEqual({
      cursor: "cursor-1",
      limit: REVIEW_QUEUE_PAGE_SIZE,
    });
    expect(requests).toContainEqual({
      cursor: "cursor-2",
      limit: REVIEW_QUEUE_PAGE_SIZE,
    });
  });

  it("automatically loads past empty visible queue pages", async () => {
    const visible = queueItem({
      trace_id: "trace_222222",
      input: "Auto loaded prompt",
      output: "Auto loaded response",
    });
    const requests: Array<{
      cursor: string | null;
      limit: string | null;
    }> = [];

    setupDataset(makeDatasetResponse(), emptyItems());
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", ({ request }) => {
        const url = new URL(request.url);
        const cursor = url.searchParams.get("cursor");
        requests.push({
          cursor,
          limit: url.searchParams.get("limit"),
        });

        return HttpResponse.json(
          cursor === "cursor-1"
            ? reviewQueueResponse([visible])
            : reviewQueueResponse([], {
                next_cursor: "cursor-1",
              }),
        );
      }),
    );

    renderDataset({ tab: null });

    expect(await screen.findByText("Auto loaded response")).toBeInTheDocument();
    expect(requests).toContainEqual({
      cursor: null,
      limit: REVIEW_QUEUE_PAGE_SIZE,
    });
    expect(requests).toContainEqual({
      cursor: "cursor-1",
      limit: REVIEW_QUEUE_PAGE_SIZE,
    });
  });

  it("offers to load more from the queue list when the current page has no visible traces", async () => {
    let requestCount = 0;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
    );
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", () => {
        requestCount += 1;
        return HttpResponse.json(
          reviewQueueResponse([], { next_cursor: `cursor-${requestCount}` }),
        );
      }),
    );

    renderDataset({ tab: null });

    expect(await screen.findAllByText("Ready for more traces")).toHaveLength(2);
    await waitFor(() => {
      expect(requestCount).toBe(4);
    });
    expect(
      screen.queryByText("No traces waiting for review."),
    ).not.toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: /load more items/i }).length,
    ).toBeGreaterThan(1);
  });

  it("renders queue detail in pretty mode without a raw toggle", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "plain input",
          output: "agent **answer**",
          sentiment: "",
        }),
      ]),
    );

    renderDataset({ tab: null });

    await screen.findByText("answer");
    expect(screen.queryByRole("button", { name: /^pretty$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^raw$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Previous" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("Trace 1 of 1")).toBeInTheDocument();
    expect(screen.getAllByText("plain input").length).toBeGreaterThan(0);
    expect(screen.getByText("answer")).toBeInTheDocument();
    expect(
      Array.from(document.querySelectorAll("pre")).some((pre) =>
        pre.textContent?.includes("agent **answer**"),
      ),
    ).toBe(false);
    expect(screen.getByText("answer").closest(".dp-scroll")).toHaveClass(
      "dp-scroll",
      "overflow-y-auto",
    );
    expect(
      screen.getByRole("button", { name: /resize user content/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /resize cruise line content/i }),
    ).toBeInTheDocument();
  });

  it("lets the dataset page own review detail scrolling", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "plain input",
          output: "agent answer",
          sentiment: "",
        }),
      ]),
    );

    renderDataset({ tab: null });

    await screen.findByText("agent answer");
    const detail = document.querySelector("[data-review-queue-detail]");
    expect(detail).toBeInTheDocument();
    expect(detail).not.toHaveClass("dp-scroll", "overflow-y-auto");
    expect(document.querySelector("[data-review-queue-trace-scroll]")).toBeNull();
  });

  it("opens the full trace panel from the review queue detail header", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Panel prompt",
          output: "Panel response",
          sentiment: "",
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("Panel prompt")).toBeInTheDocument();
    expect(within(panel).getByText("Panel response")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /view trace_111111/i }),
    ).not.toBeInTheDocument();

    await user.click(within(panel).getByRole("button", { name: /close trace/i }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: /trace details/i })).not.toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: /view trace_111111/i }),
    ).toBeInTheDocument();
  });

  it("updates the open trace panel when selecting another review queue trace", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "First panel prompt",
          output: "First panel response",
          sentiment: "",
        }),
        queueItem({
          trace_id: "trace_222222",
          input: "Second panel prompt",
          output: "Second panel response",
          sentiment: "negative",
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("First panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("First panel prompt")).toBeInTheDocument();
    expect(within(panel).getByText("First panel response")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /second panel prompt/i }));

    await waitFor(() => {
      expect(within(panel).getByText("Second panel prompt")).toBeInTheDocument();
      expect(within(panel).getByText("Second panel response")).toBeInTheDocument();
      expect(within(panel).queryByText("First panel prompt")).not.toBeInTheDocument();
    });
    expect(
      screen.queryByRole("button", { name: /view trace_222222/i }),
    ).not.toBeInTheDocument();
  });

  it("advances the open trace panel after judging the selected queue trace", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "First judged panel prompt",
      output: "First judged panel response",
      sentiment: "",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Second judged panel prompt",
      output: "Second judged panel response",
      sentiment: "negative",
    });
    let queueItems = [first, second];

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockCreateJudgment(() => {
      queueItems = [second];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("First judged panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("First judged panel prompt")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Good" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(within(panel).getByText("Second judged panel prompt")).toBeInTheDocument();
      expect(within(panel).getByText("Second judged panel response")).toBeInTheDocument();
      expect(within(panel).queryByText("First judged panel prompt")).not.toBeInTheDocument();
    });
  });

  it("closes the open trace panel after judging the final queue trace", async () => {
    const only = queueItem({
      trace_id: "trace_111111",
      input: "Final panel prompt",
      output: "Final panel response",
      sentiment: "",
    });
    let queueItems = [only];

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockCreateJudgment(() => {
      queueItems = [];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Final panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));
    expect(await screen.findByRole("dialog", { name: /trace details/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Good" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: /trace details/i })).not.toBeInTheDocument();
    });
    expect(await screen.findByText("You're all caught up")).toBeInTheDocument();
  });

  it("updates the open trace panel when cached queue data removes the selected trace", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "Refetched first panel prompt",
      output: "Refetched first panel response",
      sentiment: "",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Refetched second panel prompt",
      output: "Refetched second panel response",
      sentiment: "negative",
    });

    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([first, second]),
    );

    const user = userEvent.setup();
    const { queryClient } = renderDataset({ tab: null });

    await screen.findByText("Refetched first panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("Refetched first panel prompt")).toBeInTheDocument();

    act(() => {
      queryClient.setQueryData(evalKeys.reviewQueue("dep-test"), {
        pages: [reviewQueueResponse([second])],
        pageParams: [undefined],
      });
    });

    await waitFor(() => {
      expect(within(panel).getByText("Refetched second panel prompt")).toBeInTheDocument();
      expect(within(panel).getByText("Refetched second panel response")).toBeInTheDocument();
      expect(within(panel).queryByText("Refetched first panel prompt")).not.toBeInTheDocument();
    });
  });

  it("keeps the open trace panel pinned when cached queue data reorders", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "Pinned first panel prompt",
      output: "Pinned first panel response",
      sentiment: "",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Reordered second panel prompt",
      output: "Reordered second panel response",
      sentiment: "negative",
    });

    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([first, second]),
    );

    const user = userEvent.setup();
    const { queryClient } = renderDataset({ tab: null });

    await screen.findByText("Pinned first panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("Pinned first panel prompt")).toBeInTheDocument();

    act(() => {
      queryClient.setQueryData(evalKeys.reviewQueue("dep-test"), {
        pages: [reviewQueueResponse([second, first])],
        pageParams: [undefined],
      });
    });

    await waitFor(() => {
      expect(within(panel).getByText("Pinned first panel prompt")).toBeInTheDocument();
      expect(within(panel).getByText("Pinned first panel response")).toBeInTheDocument();
      expect(within(panel).queryByText("Reordered second panel prompt")).not.toBeInTheDocument();
    });
  });

  it.each([
    ["Good", "good"],
    ["Bad", "bad"],
    ["Not sure", "unknown"],
  ] as const)("posts %s as %s", async (label, verdict) => {
    let posted: DatasetJudgmentRequest | null = null;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: `${label} prompt`,
          output: `${label} response`,
        }),
      ]),
    );
    server.use(
      http.post("/api/v1/deployments/:id/dataset/judgments", async ({ request }) => {
        posted = (await request.json()) as DatasetJudgmentRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            verdict: posted.verdict,
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText(`${label} response`);
    await user.click(screen.getByRole("button", { name: label }));

    await waitFor(() => {
      expect(posted).toEqual({
        trace_id: "trace_111111",
        verdict,
      });
    });
  });

  it("uses the selected prediction as the agree verdict", async () => {
    let posted: DatasetJudgmentRequest | null = null;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_predicted",
          input: "Predicted prompt",
          output: "Predicted response",
          prediction_status: "completed",
          prediction: {
            verdict_score: -0.8,
            confidence: 79,
            explanation: "The response did not address the request.",
            judge_version: "1",
            criteria: [
              { dimension_key: "accuracy", dimension_value: -0.9 },
              { dimension_key: "completeness", dimension_value: 0.6 },
              { dimension_key: "instruction_following", dimension_value: 0 },
              { dimension_key: "scope_clarity", dimension_value: -0.4 },
              { dimension_key: "tone", dimension_value: 0.2 },
            ],
          },
        }),
      ]),
    );
    server.use(
      http.post(
        "/api/v1/deployments/:id/dataset/judgments",
        async ({ request }) => {
          posted = (await request.json()) as DatasetJudgmentRequest;
          return HttpResponse.json(
            {
              eval_dataset_id: "dataset-1",
              trace_id: posted.trace_id,
              verdict: posted.verdict,
            },
            { status: 201 },
          );
        },
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Predicted response");
    await user.click(
      screen.getByRole("button", { name: "Agree with judge" }),
    );

    expect(
      await screen.findByRole("button", { name: /hallucination/i }),
    ).toHaveAttribute("data-active");
    expect(
      screen.getByRole("button", { name: /unclear or poorly scoped/i }),
    ).not.toHaveAttribute("data-active");
    expect(
      screen.getByRole("button", { name: /incomplete/i }),
    ).not.toHaveAttribute("data-active");

    await waitFor(() => {
      expect(posted).toEqual({
        trace_id: "trace_predicted",
        verdict: "bad",
      });
    });
  });

  it.each([
    ["g", "good"],
    ["b", "bad"],
    ["s", "unknown"],
  ] as const)("posts %s keyboard shortcut as %s", async (shortcut, verdict) => {
    let posted: DatasetJudgmentRequest | null = null;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: `${shortcut} prompt`,
          output: `${shortcut} response`,
        }),
      ]),
    );
    server.use(
      http.post("/api/v1/deployments/:id/dataset/judgments", async ({ request }) => {
        posted = (await request.json()) as DatasetJudgmentRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            verdict: posted.verdict,
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText(`${shortcut} response`);
    await user.keyboard(shortcut);

    await waitFor(() => {
      expect(posted).toEqual({
        trace_id: "trace_111111",
        verdict,
      });
    });
  });

  it("does not post keyboard shortcuts from editable fields", async () => {
    let posted = false;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Editable prompt",
          output: "Editable response",
        }),
      ]),
    );
    server.use(
      http.post("/api/v1/deployments/:id/dataset/judgments", () => {
        posted = true;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: "trace_111111",
            verdict: "good",
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Editable response");

    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();

    try {
      await user.keyboard("g");
      expect(input).toHaveValue("g");
      expect(posted).toBe(false);
    } finally {
      input.remove();
    }
  });

  it("runs the verdict animation from the matching button on keyboard shortcuts", async () => {
    const hadAnimate = "animate" in HTMLElement.prototype;
    const originalAnimate = HTMLElement.prototype.animate;
    const animation = {
      addEventListener: vi.fn(),
    } as unknown as Animation;
    const animate = vi.fn<HTMLElement["animate"]>(() => animation);
    Object.defineProperty(HTMLElement.prototype, "animate", {
      configurable: true,
      value: animate,
    });

    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Animated prompt",
          output: "Animated response",
        }),
      ]),
    );
    server.use(
      http.post("/api/v1/deployments/:id/dataset/judgments", async ({ request }) => {
        const posted = (await request.json()) as DatasetJudgmentRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            verdict: posted.verdict,
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Animated response");
    animate.mockClear();

    try {
      await user.keyboard("g");

      await waitFor(() => {
        expect(animate).toHaveBeenCalled();
      });
      expect(animate.mock.calls[0]?.[0]).toEqual(
        expect.arrayContaining([expect.objectContaining({ offset: 0 })]),
      );
    } finally {
      if (hadAnimate) {
        Object.defineProperty(HTMLElement.prototype, "animate", {
          configurable: true,
          value: originalAnimate,
        });
      } else {
        delete (HTMLElement.prototype as { animate?: HTMLElement["animate"] })
          .animate;
      }
    }
  });

  it("keeps a trace in the queue and shows a retry message when judgment fails", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Retry prompt",
          output: "Retry response",
        }),
      ]),
    );
    server.use(
      http.post("/api/v1/deployments/:id/dataset/judgments", () =>
        HttpResponse.json({ error: "failed" }, { status: 500 }),
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Retry response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Good" }));

    expect(
      await screen.findByText("Could not save verdict. Try again."),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Retry prompt").length).toBeGreaterThan(0);
  });

  it("removes a judged trace from the queue and selects the next trace", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "First prompt",
      output: "First response",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Second prompt",
      output: "Second response",
    });
    let queueItems = [first, second];

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockCreateJudgment(() => {
      queueItems = [second];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Good" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByText("First prompt")).not.toBeInTheDocument();
      expect(screen.getByText("Second response")).toBeInTheDocument();
    });
  });

  it("submits the criteria selected before clicking Save", async () => {
    const trace = queueItem({
      trace_id: "trace_111111",
      input: "Criteria prompt",
      output: "Criteria response",
    });
    let queueItems = [trace];
    let criteriaBody: {
      criteria: { dimension_key: string; value: number }[];
    } | null = null;

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockCreateJudgment(() => {
      queueItems = [];
    });
    server.use(
      http.put(
        "/api/v1/deployments/:id/dataset/judgments/:traceId/criteria",
        async ({ request, params }) => {
          criteriaBody = (await request.json()) as {
            criteria: { dimension_key: string; value: number }[];
          };
          return HttpResponse.json({
            eval_dataset_id: "dataset-1",
            trace_id: params.traceId,
            verdict: "good",
            criteria: criteriaBody.criteria,
          });
        },
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Criteria response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Good" }));
    await user.click(screen.getByRole("button", { name: "Correct info" }));
    await user.click(screen.getByRole("button", { name: "Followed instruction" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(criteriaBody).toEqual({
        criteria: [
          { dimension_key: "accuracy", value: 1 },
          { dimension_key: "instruction_following", value: 1 },
        ],
      });
    });
  });

  it("automatically loads more when the loaded queue page is exhausted", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "Only loaded prompt",
      output: "Only loaded response",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Fresh page prompt",
      output: "Fresh page response",
    });

    setupDataset(makeDatasetResponse(), emptyItems());
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", ({ request }) => {
        const url = new URL(request.url);
        return HttpResponse.json(
          url.searchParams.get("cursor") === "cursor-1"
            ? reviewQueueResponse([second])
            : reviewQueueResponse([first], { next_cursor: "cursor-1" }),
        );
      }),
      http.post("/api/v1/deployments/:id/dataset/judgments", async ({ request }) => {
        const posted = (await request.json()) as DatasetJudgmentRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            verdict: posted.verdict,
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Only loaded response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Good" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Fresh page response")).toBeInTheDocument();
    expect(screen.queryByText("Ready for more traces")).not.toBeInTheDocument();
    expect(screen.queryByText("You're all caught up")).not.toBeInTheDocument();
  });

  it("shows a quick undo after judging and restores the trace", async () => {
    const firstPageTrace = queueItem({
      trace_id: "trace_111111",
      input: "First page prompt",
      output: "First page response",
      timestamp: "2026-06-01T13:00:00Z",
      sentiment: "",
    });
    const secondPageTrace = queueItem({
      trace_id: "trace_222222",
      input: "Undoable prompt",
      output: "Undoable response",
      timestamp: "2026-06-01T12:00:00Z",
      sentiment: "positive",
    });
    let deletedTraceId = "";
    let queueFetchCount = 0;

    setupDataset(makeDatasetResponse(), emptyItems());
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", ({ request }) => {
        queueFetchCount += 1;
        const url = new URL(request.url);
        return HttpResponse.json(
          url.searchParams.get("cursor") === "cursor-1"
            ? reviewQueueResponse([secondPageTrace])
            : reviewQueueResponse([firstPageTrace], {
                next_cursor: "cursor-1",
              }),
        );
      }),
      http.post("/api/v1/deployments/:id/dataset/judgments", async ({ request }) => {
        const posted = (await request.json()) as DatasetJudgmentRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            verdict: posted.verdict,
          },
          { status: 201 },
        );
      }),
      http.delete(
        "/api/v1/deployments/:id/dataset/judgments/:traceId",
        ({ params }) => {
          deletedTraceId = String(params.traceId);
          return HttpResponse.json({
            eval_dataset_id: "dataset-1",
            trace_id: deletedTraceId,
            verdict: "good",
          });
        },
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First page response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /load more items/i }));
    await user.click(
      await screen.findByRole("button", { name: /undoable prompt/i }),
    );

    expect(screen.getByText("Undoable response")).toBeInTheDocument();
    expect(queueFetchCount).toBe(2);
    await user.click(screen.getByRole("button", { name: "Good" }));

    expect(await screen.findByText("Marked as good")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^undo$/i }));

    await waitFor(() => {
      expect(deletedTraceId).toBe("trace_222222");
    });
    expect(await screen.findByText("Undoable response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 2 of 2")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^previous$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^next$/i })).not.toBeInTheDocument();
    expect(queueFetchCount).toBe(2);
  });

  it("clears quick undo when selecting another queue trace", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "First prompt",
      output: "First response",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Second prompt",
      output: "Second response",
    });
    let queueItems = [first, second];

    setupDataset(makeDatasetResponse(), emptyItems());
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", () =>
        HttpResponse.json(reviewQueueResponse(queueItems)),
      ),
      http.post("/api/v1/deployments/:id/dataset/judgments", async ({ request }) => {
        const posted = (await request.json()) as DatasetJudgmentRequest;
        queueItems = [second];
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            verdict: posted.verdict,
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Good" }));
    expect(await screen.findByText("Marked as good")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /second prompt/i }));

    await waitFor(() => {
      expect(screen.queryByText("Marked as good")).not.toBeInTheDocument();
    });
  });
});

describe("dataset view", () => {
  it("renders the grade letter, dataset name, and counts", async () => {
    setupDataset(makeDatasetResponse());
    renderDataset();
    await waitFor(() => {
      expect(screen.getAllByLabelText(/grade b/i).length).toBeGreaterThan(0);
    });
    const datasetName = screen.getAllByText("dep-test-deployment")[0];
    expect(datasetName).toBeInTheDocument();
    expect(datasetName).toHaveClass("font-mono", "text-mono-xs");
    expect(screen.getByText("Baseline grade")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
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
      expect(screen.getByText(/12,345/)).toBeInTheDocument();
    });
  });

  it("undoes a judged dataset item and refreshes the table", async () => {
    const hadAnimate = "animate" in HTMLElement.prototype;
    const originalAnimate = HTMLElement.prototype.animate;
    const animation = {
      addEventListener: vi.fn((event: string, listener: EventListener) => {
        if (event === "finish") {
          listener(new Event("finish"));
        }
      }),
    } as unknown as Animation;
    const animate = vi.fn<HTMLElement["animate"]>(() => animation);
    Object.defineProperty(HTMLElement.prototype, "animate", {
      configurable: true,
      value: animate,
    });

    const item = datasetItem({
      id: "dataset-item-undo",
      input: "Undo prompt",
      expected_output: "Undo response",
      source_trace_id: "trace-undo",
      metadata: { verdict: 1 },
    });
    let items = [item];
    let deletedTraceId = "";

    setupDataset(
      makeDatasetResponse({ item_count: 1, good_count: 1, bad_count: 0 }),
      itemsResponse(items),
    );
    server.use(
      http.get("/api/v1/deployments/:id/dataset/items", () =>
        HttpResponse.json(itemsResponse(items)),
      ),
      http.get("/api/v1/deployments/:id/dataset/review-queue", () =>
        HttpResponse.json(
          reviewQueueResponse(
            deletedTraceId
              ? [
                  queueItem({
                    trace_id: deletedTraceId,
                    input: "Undo prompt",
                    output: "Undo response",
                    prediction_status: "completed",
                    prediction: {
                      verdict_score: -0.8,
                      confidence: 82,
                      explanation: "The response missed the requested result.",
                      judge_version: "1",
                      criteria: [],
                    },
                  }),
                ]
              : [],
          ),
        ),
      ),
      http.delete(
        "/api/v1/deployments/:id/dataset/judgments/:traceId",
        ({ params }) => {
          deletedTraceId = String(params.traceId);
          items = [];
          return HttpResponse.json({
            eval_dataset_id: "dataset-1",
            trace_id: deletedTraceId,
            verdict: "good",
          });
        },
      ),
    );

    const user = userEvent.setup();
    try {
      renderDataset();

      expect(await screen.findByText("Undo prompt")).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: /^trace actions$/i }));
      await user.click(
        await screen.findByRole("menuitem", {
          name: /remove from dataset/i,
        }),
      );

      await waitFor(() => {
        expect(deletedTraceId).toBe("trace-undo");
      });
      await waitFor(() => {
        expect(animate).toHaveBeenCalled();
        expect(screen.queryByText("Undo prompt")).not.toBeInTheDocument();
      });

      await user.click(screen.getByRole("button", { name: /^review queue$/i }));

      expect(await screen.findByText("Undo response")).toBeInTheDocument();
      expect(
        await screen.findByRole("button", { name: /undo prompt bad/i }),
      ).toBeInTheDocument();
    } finally {
      if (hadAnimate) {
        Object.defineProperty(HTMLElement.prototype, "animate", {
          configurable: true,
          value: originalAnimate,
        });
      } else {
        delete (HTMLElement.prototype as { animate?: HTMLElement["animate"] })
          .animate;
      }
    }
  });

  it("changes a judged dataset item verdict in place", async () => {
    const item = datasetItem({
      id: "dataset-item-change",
      input: "Change prompt",
      expected_output: "Change response",
      source_trace_id: "trace-change",
      metadata: { verdict: 1 },
    });
    let items = [item];
    let patched:
      | { traceId: string; body: Pick<DatasetJudgmentRequest, "verdict"> }
      | null = null;

    setupDataset(
      makeDatasetResponse({ item_count: 1, good_count: 1, bad_count: 0 }),
      itemsResponse(items),
    );
    server.use(
      http.get("/api/v1/deployments/:id/dataset/items", () =>
        HttpResponse.json(itemsResponse(items)),
      ),
      http.patch(
        "/api/v1/deployments/:id/dataset/judgments/:traceId",
        async ({ params, request }) => {
          const traceId = String(params.traceId);
          const body = (await request.json()) as Pick<
            DatasetJudgmentRequest,
            "verdict"
          >;
          patched = { traceId, body };
          items = [
            {
              ...item,
              metadata: { ...item.metadata, verdict: -1 },
            },
          ];
          return HttpResponse.json({
            eval_dataset_id: "dataset-1",
            trace_id: traceId,
            verdict: body.verdict,
          });
        },
      ),
    );

    const user = userEvent.setup();
    renderDataset();

    expect(await screen.findByText("Change prompt")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^trace actions$/i }));
    await user.click(await screen.findByRole("menuitem", { name: /^bad$/i }));

    await waitFor(() => {
      expect(patched).toEqual({
        traceId: "trace-change",
        body: { verdict: "bad" },
      });
    });
    await waitFor(() => {
      const row = screen.getByText("Change prompt").closest("tr");
      expect(row).not.toBeNull();
      expect(within(row!).getByText("Bad")).toBeInTheDocument();
    });
  });

  it("removes a judged dataset item from the dataset", async () => {
    const item = datasetItem({
      id: "dataset-item-neutral",
      input: "Neutral prompt",
      expected_output: "Neutral response",
      source_trace_id: "trace-neutral",
      metadata: { verdict: 1 },
    });
    let items = [item];
    let deletedTraceId: string | null = null;

    setupDataset(
      makeDatasetResponse({ item_count: 1, good_count: 1, bad_count: 0 }),
      itemsResponse(items),
    );
    server.use(
      http.get("/api/v1/deployments/:id/dataset/items", () =>
        HttpResponse.json(itemsResponse(items)),
      ),
      http.delete(
        "/api/v1/deployments/:id/dataset/judgments/:traceId",
        ({ params }) => {
          const traceId = String(params.traceId);
          deletedTraceId = traceId;
          items = [];
          return HttpResponse.json({
            eval_dataset_id: "dataset-1",
            trace_id: traceId,
            verdict: "good",
          });
        },
      ),
    );

    const user = userEvent.setup();
    renderDataset();

    expect(await screen.findByText("Neutral prompt")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^trace actions$/i }));
    await user.click(
      await screen.findByRole("menuitem", { name: /remove from dataset/i }),
    );

    await waitFor(() => {
      expect(deletedTraceId).toBe("trace-neutral");
    });
    await waitFor(() => {
      expect(screen.queryByText("Neutral prompt")).not.toBeInTheDocument();
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
    expect((await screen.findAllByText(/start grading/i)).length).toBeGreaterThan(0);
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
