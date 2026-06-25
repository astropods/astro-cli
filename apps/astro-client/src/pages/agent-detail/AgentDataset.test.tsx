import { describe, it, expect, afterEach, vi } from "vitest";
import { screen, cleanup, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { Outlet } from "react-router";
import { server } from "@/test/msw/server";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import type {
  DatasetJudgmentRequest,
  EvalDatasetItem,
  EvalDatasetItemsResponse,
  EvalDatasetResponse,
  ReviewQueueItem,
  ReviewQueueResponse,
} from "@/lib/api";
import AgentDataset from "./AgentDataset";

afterEach(cleanup);
afterEach(() => server.resetHandlers());
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

function reviewQueueResponse(items: ReviewQueueItem[]): ReviewQueueResponse {
  return {
    items,
    end_time: "2026-06-01T12:00:00Z",
  };
}

function queueItem(overrides: Partial<ReviewQueueItem>): ReviewQueueItem {
  return {
    trace_id: "trace_000001",
    timestamp: "2026-06-01T12:00:00Z",
    input: "How do I deploy?",
    output: "Run ast deploy.",
    sentiment: "positive",
    ...overrides,
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
  queue: ReviewQueueResponse = reviewQueueResponse([]),
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
    expect(screen.getByText("Likely positive")).toBeInTheDocument();
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

  it("navigates between queue traces from the detail header", async () => {
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
    expect(screen.getByRole("button", { name: /^previous$/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /^next$/i })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: /^next$/i }));
    expect(screen.getByText("Second response")).toBeInTheDocument();
    expect(screen.getByText("Likely negative")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^previous$/i })).toBeEnabled();
    expect(screen.getByRole("button", { name: /^next$/i })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: /^previous$/i }));
    expect(screen.getByText("First response")).toBeInTheDocument();
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

    await screen.findByText("No signal");
    expect(screen.queryByRole("button", { name: /^pretty$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^raw$/i })).not.toBeInTheDocument();
    expect(
      Array.from(document.querySelectorAll("pre")).some((pre) =>
        pre.textContent?.includes("agent **answer**"),
      ),
    ).toBe(false);
    expect(screen.getByText("No signal")).toBeInTheDocument();
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

  it.each([
    ["Good", "good"],
    ["Bad", "bad"],
    ["Neutral", "unknown"],
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

  it.each([
    ["g", "good"],
    ["b", "bad"],
    ["n", "unknown"],
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

    await waitFor(() => {
      expect(screen.queryByText("First prompt")).not.toBeInTheDocument();
      expect(screen.getByText("Second response")).toBeInTheDocument();
    });
  });

  it("shows a quick undo after judging and restores the trace", async () => {
    const trace = queueItem({
      trace_id: "trace_111111",
      input: "Undoable prompt",
      output: "Undoable response",
    });
    let queueItems = [trace];
    let slowNextQueueResponse = false;
    let deletedTraceId = "";

    setupDataset(makeDatasetResponse(), emptyItems());
    server.use(
      http.get("/api/v1/deployments/:id/dataset/review-queue", async () => {
        if (slowNextQueueResponse) {
          await new Promise((resolve) => setTimeout(resolve, 250));
        }
        return HttpResponse.json(reviewQueueResponse(queueItems));
      }),
      http.post("/api/v1/deployments/:id/dataset/judgments", async ({ request }) => {
        const posted = (await request.json()) as DatasetJudgmentRequest;
        queueItems = [];
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
          slowNextQueueResponse = true;
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

    expect(await screen.findByText("Undoable response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Good" }));

    expect(await screen.findByText("Marked as good")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^undo$/i }));

    await waitFor(() => {
      expect(deletedTraceId).toBe("trace_111111");
    });
    expect(await screen.findByText("Undoable response")).toBeInTheDocument();
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

  it("changes a judged dataset item to neutral and removes it from the dataset", async () => {
    const item = datasetItem({
      id: "dataset-item-neutral",
      input: "Neutral prompt",
      expected_output: "Neutral response",
      source_trace_id: "trace-neutral",
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
          items = [];
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

    expect(await screen.findByText("Neutral prompt")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^trace actions$/i }));
    await user.click(
      await screen.findByRole("menuitem", { name: /^neutral$/i }),
    );

    await waitFor(() => {
      expect(patched).toEqual({
        traceId: "trace-neutral",
        body: { verdict: "unknown" },
      });
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
