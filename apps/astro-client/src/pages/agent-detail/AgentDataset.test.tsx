import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { act, screen, cleanup, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { Outlet } from "react-router";
import { toast } from "sonner";
import { server } from "@/test/msw/server";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import { evalKeys } from "@/api/queries/keys";
import type {
  DatasetItemRequest,
  DatasetEvaluationsResponse,
  EvaluationSetResponse,
  EvalDatasetItem,
  EvalDatasetItemsResponse,
  EvalDatasetResponse,
  EvaluationStatusCounts,
  TraceEvaluationResponse,
  ReviewQueueDismissalResponse,
  ReviewQueueItem,
  ReviewQueueResponse,
  TraceDetailResponse,
} from "@/lib/api";
import type { AuthContextType } from "@/lib/auth-context";
import { coachmarkStorageKey } from "@/hooks/use-persistent-coachmark";
import AgentDataset from "./AgentDataset";

const AUTO_EVALUATE_ONBOARDING_KEY = coachmarkStorageKey(
  "llm-judge",
  mockAuthContext.user!.id,
);

beforeEach(() => {
  localStorage.setItem(AUTO_EVALUATE_ONBOARDING_KEY, "true");
});

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
    evaluators: [],
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

function evaluationStatusCounts(
  items: ReviewQueueItem[],
): EvaluationStatusCounts {
  const counts = {
    queued: 0,
    in_progress: 0,
    completed: 0,
    failed: 0,
  };
  for (const item of items) {
    if (item.run) {
      counts[item.run.status] += 1;
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

function mockEvaluationStatus(
  resolve: () => EvaluationStatusCounts | Promise<EvaluationStatusCounts>,
) {
  server.use(
    http.get(
      "/api/v1/deployments/:id/dataset/evaluations/status",
      async () => HttpResponse.json(await resolve()),
    ),
  );
}

function mockRunEvaluations(
  resolve: (
    request: Request,
  ) => DatasetEvaluationsResponse | Promise<DatasetEvaluationsResponse>,
) {
  server.use(
    http.post(
      "/api/v1/deployments/:id/dataset/evaluations",
      async ({ request }) =>
        HttpResponse.json(await resolve(request), { status: 202 }),
    ),
  );
}

function mockDismissReviewQueueTrace(onDismiss?: (traceId: string) => void) {
  server.use(
    http.post(
      "/api/v1/deployments/:id/dataset/review-queue/:traceId/dismiss",
      ({ params }) => {
        const traceId = String(params.traceId);
        onDismiss?.(traceId);
        return HttpResponse.json<ReviewQueueDismissalResponse>({
          eval_dataset_id: "dataset-1",
          trace_id: traceId,
          dismissed: true,
        });
      },
    ),
  );
}

function mockRestoreReviewQueueTrace(onRestore?: (traceId: string) => void) {
  server.use(
    http.delete(
      "/api/v1/deployments/:id/dataset/review-queue/:traceId/dismiss",
      ({ params }) => {
        const traceId = String(params.traceId);
        onRestore?.(traceId);
        return HttpResponse.json<ReviewQueueDismissalResponse>({
          eval_dataset_id: "dataset-1",
          trace_id: traceId,
          dismissed: false,
        });
      },
    ),
  );
}



const REVIEW_QUEUE_PAGE_SIZE = "50";

async function saveToDataset(user: ReturnType<typeof userEvent.setup>) {
  if (!screen.queryByRole("button", { name: "Save" })) {
    await user.click(screen.getByRole("button", { name: "Add to dataset" }));
  }
  await user.click(screen.getByRole("button", { name: "Save" }));
}

const reviewQueueFixtures = new Map<string, ReviewQueueItem>();
const traceEvaluationFixtures = new Map<string, Partial<TraceEvaluationResponse>>();

type QueueItemOverrides = Partial<ReviewQueueItem> &
  Pick<Partial<TraceEvaluationResponse>, "output" | "user_id" | "user_details">;

function queueItem({
  output = "Run ast deploy.",
  user_id,
  user_details,
  ...overrides
}: QueueItemOverrides): ReviewQueueItem {
  const item: ReviewQueueItem = {
    trace_id: "trace_000001",
    timestamp: "2026-06-01T12:00:00Z",
    input: "How do I deploy?",
    run: null,
    ...overrides,
  };
  reviewQueueFixtures.set(item.trace_id, item);
  traceEvaluationFixtures.set(item.trace_id, { output, user_id, user_details });
  return item;
}

function datasetItem(overrides: Partial<EvalDatasetItem>): EvalDatasetItem {
  return {
    id: "item-1",
    input: "input",
    expected_output: "output",
    source_trace_id: "trace-1",
    created_at: "2026-06-01T12:00:00Z",
    evaluation_ref: EVALUATION_REF,
    verified_by_user_id: "user_1",
    evaluator_outputs: [],
    ...overrides,
  };
}

const EVALUATION_REF = "preset/default-evaluation";

const EVALUATION_SET: EvaluationSetResponse = {
  evaluation_ref: EVALUATION_REF,
  evaluators: [
    {
      key: "exposed_pii",
      label: "Exposed PII",
      description: "Flags personal data in the output.",
      type: "llm",
      output: { type: "boolean" },
    },
    {
      key: "user_sentiment",
      label: "User sentiment",
      type: "llm",
      output: { type: "enum", options: ["positive", "negative"] },
    },
  ],
};

function mockAddDatasetItem(onAdd?: (body: DatasetItemRequest) => void) {
  server.use(
    http.post(
      "/api/v1/deployments/:id/dataset/items",
      async ({ request }) => {
        const body = (await request.json()) as DatasetItemRequest;
        onAdd?.(body);
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: body.trace_id,
            evaluation_ref: EVALUATION_REF,
          },
          { status: 201 },
        );
      },
    ),
  );
}

function setupDataset(
  response: EvalDatasetResponse | { status: number },
  items: EvalDatasetItemsResponse = emptyItems(),
  queue: ReviewQueueResponse = reviewQueueResponse([]),
  evaluationStatus: EvaluationStatusCounts = evaluationStatusCounts(queue.items),
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
    http.get(
      "/api/v1/deployments/:id/dataset/review-queue/:trace_id/evaluation",
      ({ params }) => {
        const traceId = String(params.trace_id);
        const item = reviewQueueFixtures.get(traceId);
        return HttpResponse.json({
          trace_id: traceId,
          input: item?.input ?? "",
          output: "",
          evaluation_ref: "preset/default-evaluation",
          run: item?.run ?? null,
          evaluators: [],
          ...traceEvaluationFixtures.get(traceId),
        });
      },
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
            output: traceEvaluationFixtures.get(traceId)?.output,
          },
          observations: [],
          scores: [],
        });
      },
    ),
    http.get("/api/v1/accounts/:account/members", () =>
      HttpResponse.json({ members: [] }),
    ),
    http.get("/api/v1/agents/:account/:name/evaluation-set", () =>
      HttpResponse.json(EVALUATION_SET),
    ),
    http.put(
      "/api/v1/deployments/:id/dataset/items/:traceId/evaluator-outputs",
      async ({ request, params }) => {
        const body = (await request.json()) as {
          values: { key: string; value: unknown }[];
        };
        return HttpResponse.json({
          eval_dataset_id: "dataset-1",
          trace_id: params.traceId,
          evaluation_ref: EVALUATION_REF,
          evaluator_outputs: body.values,
          verified_by_user_id: "user_1",
        });
      },
    ),
    http.delete("/api/v1/deployments/:id/dataset/items/:traceId", ({ params }) =>
      HttpResponse.json({
        eval_dataset_id: "dataset-1",
        trace_id: params.traceId,
        evaluation_ref: EVALUATION_REF,
      }),
    ),
  );
  mockEvaluationStatus(() => evaluationStatus);
}

function renderDataset({
  deploymentId = "dep-test",
  tab = "dataset",
  auth = mockAuthContext,
}: {
  deploymentId?: string;
  tab?: "queue" | "dataset" | null;
  auth?: AuthContextType;
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
      auth,
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
  it("introduces auto-evaluation once per user before enabling its hover popup", async () => {
    localStorage.removeItem(AUTO_EVALUATE_ONBOARDING_KEY);
    const queue = reviewQueueResponse([queueItem({})]);
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      queue,
    );
    let resolveQueue!: (value: ReviewQueueResponse) => void;
    const pendingQueue = new Promise<ReviewQueueResponse>((resolve) => {
      resolveQueue = resolve;
    });
    mockReviewQueueRequest(() => pendingQueue);

    const user = userEvent.setup();
    const { unmount } = renderDataset({ tab: null });
    const evaluateButton = await screen.findByRole("button", {
      name: "Run AI Evaluator",
    });

    expect(evaluateButton).toBeDisabled();
    expect(
      screen.queryByRole("heading", { name: "Save time with auto-evaluation" }),
    ).not.toBeInTheDocument();
    await user.hover(evaluateButton.parentElement!);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    resolveQueue(queue);
    expect(
      await screen.findByRole("heading", {
        name: "Save time with auto-evaluation",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Score every trace in one pass, then use the results while deciding which traces belong in the dataset.",
      ),
    ).toBeInTheDocument();

    await user.hover(evaluateButton);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Got it" }));
    expect(localStorage.getItem(AUTO_EVALUATE_ONBOARDING_KEY)).toBe("true");
    expect(
      screen.queryByRole("heading", { name: "Save time with auto-evaluation" }),
    ).not.toBeInTheDocument();

    const hoverEvaluateButton = screen.getByRole("button", {
      name: "Run AI Evaluator",
    });
    await user.hover(hoverEvaluateButton);
    expect(await screen.findByRole("tooltip")).toBeInTheDocument();

    unmount();
    const { unmount: unmountReturningUser } = renderDataset({ tab: null });
    expect(
      screen.queryByRole("heading", { name: "Save time with auto-evaluation" }),
    ).not.toBeInTheDocument();

    unmountReturningUser();
    renderDataset({
      tab: null,
      auth: {
        ...mockAuthContext,
        user: { ...mockAuthContext.user!, id: "user-2" },
      },
    });
    expect(
      await screen.findByRole("heading", {
        name: "Save time with auto-evaluation",
      }),
    ).toBeInTheDocument();
  });

  it("dismisses onboarding when the user runs the evaluator", async () => {
    localStorage.removeItem(AUTO_EVALUATE_ONBOARDING_KEY);
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([queueItem({})]),
    );
    mockRunEvaluations(() => ({
      enqueued_trace_ids: ["trace-1"],
      failed_trace_ids: [],
    }));

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(
      await screen.findByRole("heading", {
        name: "Save time with auto-evaluation",
      }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Run AI Evaluator" }));

    expect(localStorage.getItem(AUTO_EVALUATE_ONBOARDING_KEY)).toBe("true");
    expect(
      screen.queryByRole("heading", { name: "Save time with auto-evaluation" }),
    ).not.toBeInTheDocument();
  });

  it("shows an empty queue message when there are no traces to review", async () => {
    setupDataset(makeDatasetResponse(), emptyItems(), reviewQueueResponse([]));

    const { container } = renderDataset({ tab: null });

    expect(
      await screen.findByText("No traces waiting for review."),
    ).toBeInTheDocument();
    expect(screen.getByText("You're all caught up")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /load more items/i }),
    ).not.toBeInTheDocument();
    expect(
      container.querySelector("[data-review-queue-controls]"),
    ).not.toBeInTheDocument();
  });

  it("asks the server to enqueue the most recent unevaluated traces", async () => {
    let postedBody: string | null = null;

    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([queueItem({})]),
    );
    mockRunEvaluations(async (request) => {
      postedBody = await request.text();
      return {
        enqueued_trace_ids: ["trace-1"],
        failed_trace_ids: [],
      };
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await user.hover(
      await screen.findByRole("button", { name: "Run AI Evaluator" }),
    );

    const tooltip = await screen.findByRole("tooltip");
    expect(
      within(tooltip).getByRole("heading", {
        name: "Automatically evaluate traces",
      }),
    ).toBeInTheDocument();
    expect(
      within(tooltip).getByText(
        "The evaluators will assess up to 50 of the most recent unevaluated traces. Use the results while deciding which traces belong in the dataset.",
      ),
    ).toBeInTheDocument();
    expect(
      within(tooltip).getByText("Estimated ~500 credits"),
    ).toBeInTheDocument();
    expect(
      within(tooltip).getByRole("link", { name: "View usage" }),
    ).toHaveAttribute("href", "/settings/billing");

    await user.click(screen.getByRole("button", { name: "Run AI Evaluator" }));

    await waitFor(() => {
      expect(postedBody).toBe("");
    });
  });

  it.each([
    {
      completedCount: 2,
      title: "Assessment complete",
      description: "Traces scored by the evaluator are ready to review.",
      label: "successful",
    },
    {
      completedCount: 1,
      title: "Some traces couldn’t be evaluated",
      description: "Retry them on the next run or review the traces manually.",
      label: "partially failed",
    },
    {
      completedCount: 0,
      title: "Assessment failed",
      description:
        "Evaluations could not be generated. Retry them on the next run.",
      label: "failed",
    },
  ])(
    "shows a completion toast after a $label evaluation run settles",
    async ({ completedCount, title, description }) => {
      const queueItems = [
        queueItem({
          trace_id: "trace-1",
          run: null,
        }),
      ];
      let evaluationStatus: EvaluationStatusCounts = {
        queued: 0,
        in_progress: 0,
        completed: 5,
        failed: 0,
      };
      setupDataset(
        makeDatasetResponse(),
        emptyItems(),
        reviewQueueResponse(queueItems),
        evaluationStatus,
      );
      mockEvaluationStatus(() => evaluationStatus);
      mockRunEvaluations(() => {
        evaluationStatus = {
          queued: 2,
          in_progress: 0,
          completed: 5,
          failed: 0,
        };
        return {
          enqueued_trace_ids: ["trace-1", "trace-2"],
          failed_trace_ids: [],
        };
      });
      const successToastSpy = vi.spyOn(toast, "success");
      const warningToastSpy = vi.spyOn(toast, "warning");
      const errorToastSpy = vi.spyOn(toast, "error");
      const user = userEvent.setup();
      const { queryClient } = renderDataset({ tab: null });

      await user.click(
        await screen.findByRole("button", { name: "Run AI Evaluator" }),
      );
      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: "Evaluating 2 items" }),
        ).toBeInTheDocument(),
      );
      expect(successToastSpy).not.toHaveBeenCalled();
      expect(warningToastSpy).not.toHaveBeenCalled();
      expect(errorToastSpy).not.toHaveBeenCalled();

      evaluationStatus = {
        queued: 0,
        in_progress: 0,
        completed: 5 + completedCount,
        failed: 2 - completedCount,
      };
      await act(async () => {
        await queryClient.invalidateQueries({
          queryKey: evalKeys.evaluationStatus("dep-test"),
        });
      });

      const expectedToastSpy =
        completedCount === 0
          ? errorToastSpy
          : completedCount < 2
            ? warningToastSpy
            : successToastSpy;
      await waitFor(() =>
        expect(expectedToastSpy).toHaveBeenCalledWith(title, {
          closeButton: true,
          description,
        }),
      );
      expect(
        successToastSpy.mock.calls.length +
          warningToastSpy.mock.calls.length +
          errorToastSpy.mock.calls.length,
      ).toBe(1);
    },
  );

  it("disables the evaluate button while predictions are active", async () => {
    localStorage.removeItem(AUTO_EVALUATE_ONBOARDING_KEY);
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace-1",
          run: { status: "queued", error: null },
        }),
        queueItem({
          trace_id: "trace-2",
          run: { status: "in_progress", error: null },
        }),
        queueItem({
          trace_id: "trace-3",
          run: null,
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    const evaluateButton = await screen.findByRole("button", {
      name: "Evaluating 2 items",
    });
    await waitFor(() => expect(evaluateButton).toBeDisabled());
    expect(
      screen.queryByRole("heading", { name: "Save time with auto-evaluation" }),
    ).not.toBeInTheDocument();
    expect(localStorage.getItem(AUTO_EVALUATE_ONBOARDING_KEY)).toBeNull();
    await user.hover(evaluateButton.parentElement!);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });

  it("shows the no-result message for a failed evaluation", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace-failed",
          run: { status: "failed", error: "evaluator request failed" },
        }),
      ]),
    );

    renderDataset({ tab: null });

    expect(await screen.findByLabelText("Couldn’t evaluate")).toBeInTheDocument();
    expect(
      await screen.findByText(
        /The evaluator couldn’t score this trace\. Label it manually below\./,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("No results")).toBeInTheDocument();
  });

  it("shows an enqueued evaluation as evaluating immediately", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace-enqueued",
        }),
      ]),
    );
    mockRunEvaluations(() => ({
      enqueued_trace_ids: ["trace-enqueued"],
      failed_trace_ids: [],
    }));

    const user = userEvent.setup();
    renderDataset({ tab: null });

    const evaluateButton = await screen.findByRole("button", { name: "Run AI Evaluator" });
    expect(screen.queryByLabelText("Evaluating")).not.toBeInTheDocument();
    await user.click(evaluateButton);

    expect(await screen.findByLabelText("Evaluating")).toBeInTheDocument();
  });

  it("disables the evaluate button when the loaded queue has nothing to evaluate", async () => {
    localStorage.removeItem(AUTO_EVALUATE_ONBOARDING_KEY);
    setupDataset(makeDatasetResponse(), emptyItems(), reviewQueueResponse([]));

    const user = userEvent.setup();
    renderDataset({ tab: null });

    const evaluateButton = await screen.findByRole("button", {
      name: "Run AI Evaluator",
    });
    await waitFor(() => expect(evaluateButton).toBeDisabled());
    expect(
      screen.queryByRole("heading", { name: "Save time with auto-evaluation" }),
    ).not.toBeInTheDocument();
    expect(localStorage.getItem(AUTO_EVALUATE_ONBOARDING_KEY)).toBeNull();

    await user.hover(evaluateButton.parentElement!);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Every trace is already evaluated.",
    );
  });

  it("keeps the evaluate button disabled after filtering a resolved queue", async () => {
    setupDataset(makeDatasetResponse(), emptyItems(), reviewQueueResponse([]));
    mockReviewQueueRequest((url) =>
      reviewQueueResponse(
        url.searchParams.get("evaluation") === "evaluated"
          ? [
              queueItem({
                trace_id: "trace-predicted-good",
                input: "Predicted good prompt",
                output: "Predicted good response",
                run: { status: "completed", error: null },
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
    await user.click(screen.getByRole("option", { name: "Evaluated" }));

    expect(await screen.findByText("Predicted good response")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Run AI Evaluator" }),
    ).toBeDisabled();
  });

  it("enables the evaluate button when the Not evaluated filter finds an unevaluated trace", async () => {
    localStorage.removeItem(AUTO_EVALUATE_ONBOARDING_KEY);
    const predictedItem = queueItem({
      trace_id: "trace-predicted-good",
      output: "Predicted response",
      run: { status: "completed", error: null },
    });
    const unevaluatedItem = queueItem({
      trace_id: "trace-not-evaluated",
      output: "Unevaluated response",
    });
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([predictedItem]),
    );
    mockReviewQueueRequest((url) =>
      reviewQueueResponse(
        url.searchParams.get("evaluation") === "not_evaluated"
          ? [unevaluatedItem]
          : [predictedItem],
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    const evaluateButton = await screen.findByRole("button", {
      name: "Run AI Evaluator",
    });
    await waitFor(() => expect(evaluateButton).toBeDisabled());
    expect(
      screen.queryByRole("heading", { name: "Save time with auto-evaluation" }),
    ).not.toBeInTheDocument();
    expect(localStorage.getItem(AUTO_EVALUATE_ONBOARDING_KEY)).toBeNull();

    await user.click(
      screen.getByRole("combobox", { name: "Filter review queue" }),
    );
    await user.click(screen.getByRole("option", { name: "Not evaluated" }));

    expect(await screen.findByText("Unevaluated response")).toBeInTheDocument();
    expect(evaluateButton).toBeEnabled();
    expect(
      await screen.findByRole("heading", {
        name: "Save time with auto-evaluation",
      }),
    ).toBeInTheDocument();
  });

  it("disables the evaluate button when every queue item has a prediction", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace-predicted-good",
          run: { status: "completed", error: null },
        }),
      ]),
    );

    renderDataset({ tab: null });

    const evaluateButton = await screen.findByRole("button", {
      name: "Run AI Evaluator",
    });
    await waitFor(() => expect(evaluateButton).toBeDisabled());
  });

  it("refreshes the selected queue while prediction status is active", async () => {
    let evaluationStatus: EvaluationStatusCounts = {
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
      evaluationStatus,
    );
    mockEvaluationStatus(() => evaluationStatus);
    mockReviewQueueRequest((url) => {
      queueRequestCount += 1;
      const prediction = url.searchParams.get("evaluation");
      return reviewQueueResponse([
        queueItem({
          trace_id: `trace-${prediction ?? "all"}`,
          output: `${prediction ?? "all"} response`,
          run: { status: "in_progress", error: null },
        }),
      ]);
    });

    const user = userEvent.setup();
    const { queryClient } = renderDataset({ tab: null });

    expect(
      await screen.findByRole("button", { name: "Run AI Evaluator" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(queueRequestCount).toBe(1));

    evaluationStatus = {
      queued: 0,
      in_progress: 1,
      completed: 0,
      failed: 0,
    };
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: evalKeys.evaluationStatus("dep-test"),
      });
    });

    const evaluatingButton = await screen.findByRole("button", {
      name: "Evaluating 1 item",
    });
    expect(evaluatingButton).toBeDisabled();
    expect(
      evaluatingButton.querySelector(".dp-evaluating-gavel"),
    ).toBeInTheDocument();
    await waitFor(() => expect(queueRequestCount).toBe(2));

    await user.click(
      screen.getByRole("combobox", { name: "Filter review queue" }),
    );
    await user.click(screen.getByRole("option", { name: "Evaluated" }));
    expect(
      screen.getByRole("button", { name: "Evaluating 1 item" }),
    ).toBeDisabled();
    await waitFor(() => expect(queueRequestCount).toBe(3));

    evaluationStatus = {
      queued: 0,
      in_progress: 0,
      completed: 1,
      failed: 0,
    };
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: evalKeys.evaluationStatus("dep-test"),
      });
    });

    expect(
      await screen.findByRole("button", { name: "Run AI Evaluator" }),
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

  it("defaults the clean dataset URL to the review queue", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "First prompt",
          output: "First response",
        }),
      ]),
    );

    renderDataset({ tab: null });

    expect(
      await screen.findByText("Review traces from the last 30 days."),
    ).toBeInTheDocument();
    expect(await screen.findByText("First response")).toBeInTheDocument();
    expect(screen.getAllByText("First prompt").length).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: /view trace_111111/i }),
    ).toBeInTheDocument();
  });

  it("switches from the default queue tab to the dataset tab", async () => {
    setupDataset(
      makeDatasetResponse({ item_count: 0 }),
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

  it("navigates between traces from the detail header and arrow keys", async () => {
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
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 1 of 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous trace" })).toBeDisabled();

    const queue = screen.getByRole("listbox", { name: "Review queue" });
    const firstQueueItem = screen.getByRole("option", {
      name: /First prompt/,
    });
    await user.click(firstQueueItem);
    expect(queue).toHaveFocus();

    await user.keyboard("{ArrowDown}");
    expect(screen.getByText("Second response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 2 of 2")).toBeInTheDocument();
    expect(firstQueueItem).toHaveAttribute("aria-selected", "false");
    expect(
      screen.getByRole("option", { name: /Second prompt/ }),
    ).toHaveAttribute("aria-selected", "true");
    expect(queue).toHaveFocus();

    const boundaryArrow = new KeyboardEvent("keydown", {
      key: "ArrowDown",
      bubbles: true,
      cancelable: true,
    });
    queue.dispatchEvent(boundaryArrow);
    expect(boundaryArrow.defaultPrevented).toBe(true);

    await user.keyboard("{ArrowUp}");
    expect(screen.getByText("First response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 1 of 2")).toBeInTheDocument();

    const nextTrace = screen.getByRole("button", { name: "Next trace" });
    await user.click(nextTrace);
    expect(screen.getByText("Second response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 2 of 2")).toBeInTheDocument();
    expect(nextTrace).toBeDisabled();
  });

  it("keeps navigating by arrow key after the header buttons take focus", async () => {
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
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    const nextTrace = screen.getByRole("button", { name: "Next trace" });
    await user.click(nextTrace);
    expect(nextTrace).toHaveFocus();
    expect(screen.getByLabelText("Trace 2 of 2")).toBeInTheDocument();

    await user.keyboard("{ArrowUp}");

    expect(screen.getByText("First response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 1 of 2")).toBeInTheDocument();
    expect(
      screen.getByRole("listbox", { name: "Review queue" }),
    ).toHaveFocus();
  });

  it("hands ArrowDown from the review queue tab to queue navigation", async () => {
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
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: "dataset" });

    const reviewQueueTab = await screen.findByRole("button", {
      name: /^review queue$/i,
    });
    await user.click(reviewQueueTab);
    expect(reviewQueueTab).toHaveFocus();
    expect(await screen.findByText("First response")).toBeInTheDocument();

    await user.keyboard("{ArrowDown}");

    expect(screen.getByText("Second response")).toBeInTheDocument();
    expect(
      screen.getByRole("listbox", { name: "Review queue" }),
    ).toHaveFocus();
  });

  it("hands boundary arrows to the queue without scrolling the page", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Only prompt",
          output: "Only response",
        }),
      ]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: "dataset" });

    const reviewQueueTab = await screen.findByRole("button", {
      name: /^review queue$/i,
    });
    await user.click(reviewQueueTab);
    expect(await screen.findByText("Only response")).toBeInTheDocument();
    const queue = screen.getByRole("listbox", { name: "Review queue" });
    const focusQueue = vi.spyOn(queue, "focus");

    const boundaryArrow = new KeyboardEvent("keydown", {
      key: "ArrowDown",
      bubbles: true,
      cancelable: true,
    });
    reviewQueueTab.dispatchEvent(boundaryArrow);

    expect(boundaryArrow.defaultPrevented).toBe(true);
    expect(queue).toHaveFocus();
    expect(focusQueue).toHaveBeenCalledWith({ preventScroll: true });
    expect(screen.getByText("Only response")).toBeInTheDocument();
  });

  it("passes the selected evaluation filter to the review queue", async () => {
    const requestedFilters: Array<string | null> = [];
    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest((url) => {
      const evaluation = url.searchParams.get("evaluation");
      requestedFilters.push(evaluation);
      return reviewQueueResponse([
        queueItem({
          trace_id: `trace_${evaluation ?? "all"}`,
          input: `${evaluation ?? "all"} prompt`,
          output: `${evaluation ?? "all"} response`,
        }),
      ]);
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("all response")).toBeInTheDocument();

    for (const [label, evaluation] of [
      ["Evaluated", "evaluated"],
      ["Not evaluated", "not_evaluated"],
    ] as const) {
      await user.click(
        screen.getByRole("combobox", { name: "Filter review queue" }),
      );
      await user.click(screen.getByRole("option", { name: label }));
      expect(
        await screen.findByText(`${evaluation} response`),
      ).toBeInTheDocument();
    }

    expect(requestedFilters).toEqual([null, "evaluated", "not_evaluated"]);
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
      await screen.findByRole("option", { name: /second paged prompt/i }),
    );

    expect(screen.getByText("Second paged response")).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: /load more items/i }),
    );
    await user.click(
      await screen.findByRole("option", { name: /third paged prompt/i }),
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

  it("renders the trace user's avatar in the review queue input", async () => {
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          user_id: "user_01HXX_bob",
          user_details: {
            kind: "astro",
            display_name: "Bob Smith",
            username: "bob",
          },
        }),
      ]),
    );

    renderDataset({ tab: null });

    expect(
      await screen.findByRole("img", { name: "Bob Smith" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Bob Smith")).toBeInTheDocument();
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
        }),
        queueItem({
          trace_id: "trace_222222",
          input: "Second panel prompt",
          output: "Second panel response",
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

    await user.click(screen.getByRole("option", { name: /second panel prompt/i }));

    await waitFor(() => {
      expect(within(panel).getByText("Second panel prompt")).toBeInTheDocument();
      expect(within(panel).getByText("Second panel response")).toBeInTheDocument();
      expect(within(panel).queryByText("First panel prompt")).not.toBeInTheDocument();
    });
    expect(
      screen.queryByRole("button", { name: /view trace_222222/i }),
    ).not.toBeInTheDocument();
  });

  it("advances the open trace panel after adding the selected queue trace", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "First judged panel prompt",
      output: "First judged panel response",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Second judged panel prompt",
      output: "Second judged panel response",
    });
    let queueItems = [first, second];

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockAddDatasetItem(() => {
      queueItems = [second];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("First judged panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("First judged panel prompt")).toBeInTheDocument();

    await saveToDataset(user);

    await waitFor(() => {
      expect(within(panel).getByText("Second judged panel prompt")).toBeInTheDocument();
      expect(within(panel).getByText("Second judged panel response")).toBeInTheDocument();
      expect(within(panel).queryByText("First judged panel prompt")).not.toBeInTheDocument();
    });
  });

  it("closes the open trace panel after adding the final queue trace", async () => {
    const only = queueItem({
      trace_id: "trace_111111",
      input: "Final panel prompt",
      output: "Final panel response",
    });
    let queueItems = [only];

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockAddDatasetItem(() => {
      queueItems = [];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Final panel response");
    await user.click(screen.getByRole("button", { name: /view trace_111111/i }));
    expect(await screen.findByRole("dialog", { name: /trace details/i })).toBeInTheDocument();

    await saveToDataset(user);

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
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Refetched second panel prompt",
      output: "Refetched second panel response",
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
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Reordered second panel prompt",
      output: "Reordered second panel response",
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

  it("adds the selected trace as a dataset item", async () => {
    let posted: DatasetItemRequest | null = null;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_111111",
          input: "Membership prompt",
          output: "Membership response",
        }),
      ]),
    );
    mockAddDatasetItem((body) => {
      posted = body;
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Membership response");
    await saveToDataset(user);

    await waitFor(() => {
      expect(posted).toEqual({
        trace_id: "trace_111111",
        evaluator_outputs: [],
      });
    });
  });

  it("carries an evaluated trace's own values into the dataset", async () => {
    let posted: DatasetItemRequest | null = null;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_predicted",
          input: "Predicted prompt",
          output: "Predicted response",
          run: { id: "run-1", status: "completed", error: null },
        }),
      ]),
    );
    traceEvaluationFixtures.set("trace_predicted", {
      output: "Predicted response",
      run: { id: "run-1", status: "completed", error: null },
      evaluators: [
        {
          key: "exposed_pii",
          label: "Exposed PII",
          output: { type: "boolean" },
          status: "completed",
          value: false,
          confidence: 0.9,
          explanation: "No personal data appeared.",
          error: null,
        },
      ],
    });
    mockAddDatasetItem((body) => {
      posted = body;
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText("Predicted response");

    expect(
      await screen.findByRole("combobox", { name: "Exposed PII" }),
    ).toHaveTextContent("False");

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(posted).toEqual({
        trace_id: "trace_predicted",
        evaluation_run_id: "run-1",
        evaluator_outputs: [{ key: "exposed_pii", value: false }],
      });
    });
  });

  it("does not map Enter to a legacy prediction verdict", async () => {
    let posted: DatasetItemRequest | null = null;
    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([
        queueItem({
          trace_id: "trace_predicted",
          input: "Predicted prompt",
          output: "Predicted response",
          run: { status: "completed", error: null },
        }),
      ]),
    );
    server.use(
      http.post(
        "/api/v1/deployments/:id/dataset/items",
        async ({ request }) => {
          posted = (await request.json()) as DatasetItemRequest;
          return HttpResponse.json(
            {
              eval_dataset_id: "dataset-1",
              trace_id: posted.trace_id,
              evaluation_ref: EVALUATION_REF,
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
      screen.getByRole("option", { name: /Predicted prompt/ }),
    );
    await user.keyboard("{Enter}");

    expect(posted).toBeNull();
  });

  it.each(["g", "b", "s"] as const)(
    "does not post the removed %s verdict shortcut",
    async (shortcut) => {
    let posted: DatasetItemRequest | null = null;
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
      http.post("/api/v1/deployments/:id/dataset/items", async ({ request }) => {
        posted = (await request.json()) as DatasetItemRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            evaluation_ref: EVALUATION_REF,
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    await screen.findByText(`${shortcut} response`);
    await user.keyboard(shortcut);

      expect(posted).toBeNull();
    },
  );

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
      http.post("/api/v1/deployments/:id/dataset/items", () => {
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

  it("animates an added trace toward the dataset with a neutral plus cue", async () => {
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
      http.post("/api/v1/deployments/:id/dataset/items", async ({ request }) => {
        const posted = (await request.json()) as DatasetItemRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            evaluation_ref: EVALUATION_REF,
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
      await saveToDataset(user);

      expect(animate).toHaveBeenCalled();
      const cue = document.querySelector<HTMLElement>(
        '[data-eval-flight-cue="add"]',
      );
      expect(cue).not.toBeNull();
      expect(cue?.querySelector("path")).toHaveAttribute(
        "d",
        "M12 5v14M5 12h14",
      );
      cue?.remove();
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
      http.post("/api/v1/deployments/:id/dataset/items", () =>
        HttpResponse.json({ error: "failed" }, { status: 500 }),
      ),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Retry response")).toBeInTheDocument();
    await saveToDataset(user);

    expect(
      await screen.findByText("Could not add to the dataset. Try again."),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Retry prompt").length).toBeGreaterThan(0);
  });

  it("removes an added trace from the queue and selects the next trace", async () => {
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
    mockAddDatasetItem(() => {
      queueItems = [second];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    await saveToDataset(user);

    await waitFor(() => {
      expect(screen.queryByText("First prompt")).not.toBeInTheDocument();
      expect(screen.getByText("Second response")).toBeInTheDocument();
    });
  });

  it("admits a trace the reviewer left unlabelled", async () => {
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
    let posted: DatasetItemRequest | null = null;

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockAddDatasetItem((body) => {
      posted = body;
      queueItems = [second];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    await saveToDataset(user);

    await waitFor(() => {
      expect(posted).toEqual({
        trace_id: "trace_111111",
        evaluator_outputs: [],
      });
      expect(screen.queryByText("First prompt")).not.toBeInTheDocument();
      expect(screen.getByText("Second response")).toBeInTheDocument();
    });
  });

  it("submits the values the reviewer set before saving", async () => {
    const trace = queueItem({
      trace_id: "trace_111111",
      input: "Criteria prompt",
      output: "Criteria response",
    });
    let queueItems = [trace];
    let posted: DatasetItemRequest | null = null;

    setupDataset(makeDatasetResponse(), emptyItems());
    mockReviewQueueRequest(() => reviewQueueResponse(queueItems));
    mockAddDatasetItem((body) => {
      posted = body;
      queueItems = [];
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Criteria response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add to dataset" }));

    await user.click(screen.getByRole("combobox", { name: "Exposed PII" }));
    await user.click(screen.getByRole("option", { name: "True" }));
    await user.click(screen.getByRole("combobox", { name: "User sentiment" }));
    await user.click(screen.getByRole("option", { name: "Negative" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(posted).toEqual({
        trace_id: "trace_111111",
        evaluator_outputs: [
          { key: "exposed_pii", value: true },
          { key: "user_sentiment", value: "negative" },
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
      http.post("/api/v1/deployments/:id/dataset/items", async ({ request }) => {
        const posted = (await request.json()) as DatasetItemRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            evaluation_ref: EVALUATION_REF,
          },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Only loaded response")).toBeInTheDocument();
    await saveToDataset(user);

    expect(await screen.findByText("Fresh page response")).toBeInTheDocument();
    expect(screen.queryByText("Ready for more traces")).not.toBeInTheDocument();
    expect(screen.queryByText("You're all caught up")).not.toBeInTheDocument();
  });

  it("shows a quick undo after adding and restores the trace", async () => {
    const firstPageTrace = queueItem({
      trace_id: "trace_111111",
      input: "First page prompt",
      output: "First page response",
      timestamp: "2026-06-01T13:00:00Z",
    });
    const secondPageTrace = queueItem({
      trace_id: "trace_222222",
      input: "Undoable prompt",
      output: "Undoable response",
      timestamp: "2026-06-01T12:00:00Z",
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
      http.post("/api/v1/deployments/:id/dataset/items", async ({ request }) => {
        const posted = (await request.json()) as DatasetItemRequest;
        return HttpResponse.json(
          {
            eval_dataset_id: "dataset-1",
            trace_id: posted.trace_id,
            evaluation_ref: EVALUATION_REF,
          },
          { status: 201 },
        );
      }),
      http.delete(
        "/api/v1/deployments/:id/dataset/items/:traceId",
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
      await screen.findByRole("option", { name: /undoable prompt/i }),
    );

    expect(screen.getByText("Undoable response")).toBeInTheDocument();
    expect(queueFetchCount).toBe(2);
    await saveToDataset(user);

    expect(await screen.findByText("Added to dataset")).toBeInTheDocument();
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

  it("removes a trace from the queue and advances the selection", async () => {
    const first = queueItem({
      trace_id: "trace_111111",
      input: "Dismissable prompt",
      output: "Dismissable response",
      timestamp: "2026-06-01T13:00:00Z",
    });
    const second = queueItem({
      trace_id: "trace_222222",
      input: "Second prompt",
      output: "Second response",
      timestamp: "2026-06-01T12:00:00Z",
    });
    let dismissedTraceId = "";

    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([first, second]),
    );
    mockDismissReviewQueueTrace((traceId) => {
      dismissedTraceId = traceId;
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("Dismissable response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => {
      expect(dismissedTraceId).toBe("trace_111111");
    });
    expect(await screen.findByText("Second response")).toBeInTheDocument();
    expect(screen.queryByText("Dismissable response")).not.toBeInTheDocument();
    expect(
      await screen.findByText("Removed from review queue"),
    ).toBeInTheDocument();
  });

  it("restores a removed trace to its original position without refetching", async () => {
    const firstPageTrace = queueItem({
      trace_id: "trace_111111",
      input: "First page prompt",
      output: "First page response",
      timestamp: "2026-06-01T13:00:00Z",
    });
    const secondPageTrace = queueItem({
      trace_id: "trace_222222",
      input: "Undoable prompt",
      output: "Undoable response",
      timestamp: "2026-06-01T12:00:00Z",
    });
    let restoredTraceId = "";
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
    );
    mockDismissReviewQueueTrace();
    mockRestoreReviewQueueTrace((traceId) => {
      restoredTraceId = traceId;
    });

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First page response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /load more items/i }));
    await user.click(
      await screen.findByRole("option", { name: /undoable prompt/i }),
    );

    expect(screen.getByText("Undoable response")).toBeInTheDocument();
    expect(queueFetchCount).toBe(2);
    await user.click(screen.getByRole("button", { name: "Remove" }));

    expect(
      await screen.findByText("Removed from review queue"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^undo$/i }));

    await waitFor(() => {
      expect(restoredTraceId).toBe("trace_222222");
    });
    expect(await screen.findByText("Undoable response")).toBeInTheDocument();
    expect(screen.getByLabelText("Trace 2 of 2")).toBeInTheDocument();
    expect(queueFetchCount).toBe(2);
  });

  it("drops the open panel when another queue item is selected", async () => {
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

    setupDataset(
      makeDatasetResponse(),
      emptyItems(),
      reviewQueueResponse([first, second]),
    );

    const user = userEvent.setup();
    renderDataset({ tab: null });

    expect(await screen.findByText("First response")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add to dataset" }));
    expect(
      await screen.findByRole("button", { name: "Save" }),
    ).toBeInTheDocument();

    const secondQueueItem = screen.getByRole("option", {
      name: /second prompt/i,
    });
    expect(secondQueueItem).toHaveAttribute("aria-selected", "false");
    await user.click(secondQueueItem);

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    });
    expect(screen.getByText("Second response")).toBeInTheDocument();
    expect(secondQueueItem).toHaveAttribute("aria-selected", "true");
  });
});

describe("dataset view", () => {
  it("renders each evaluator's value distribution", async () => {
    setupDataset(
      makeDatasetResponse({
        evaluators: [
          {
            key: "exposed_pii",
            label: "Exposed PII",
            distribution: [
              { value: false, count: 30 },
              { value: true, count: 12 },
            ],
          },
        ],
      }),
    );
    const user = userEvent.setup();
    renderDataset();

    expect(
      await screen.findByRole("heading", { name: "Dataset overview" }),
    ).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();

    await user.click(await screen.findByRole("button", { name: /Exposed PII/ }));

    expect(screen.getByText("False")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
    expect(screen.getByText("True")).toBeInTheDocument();
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
      makeDatasetResponse({ item_count: 12345 }),
    );
    renderDataset();
    await waitFor(() => {
      expect(screen.getByText(/12,345/)).toBeInTheDocument();
    });
  });

  it("undoes an added dataset item and refreshes the table", async () => {
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
    });
    let items = [item];
    let deletedTraceId = "";

    setupDataset(
      makeDatasetResponse({ item_count: 1 }),
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
                    run: { status: "completed", error: null },
                  }),
                ]
              : [],
          ),
        ),
      ),
      http.delete(
        "/api/v1/deployments/:id/dataset/items/:traceId",
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
        await screen.findByRole("option", { name: /undo prompt evaluated/i }),
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

  it("locks editing when the evaluation set cannot be loaded", async () => {
    const item = datasetItem({
      id: "dataset-item-unset",
      input: "Unset prompt",
      source_trace_id: "trace-unset",
      evaluator_outputs: [
        { key: "exposed_pii", label: "Exposed PII", value: false },
      ],
    });
    setupDataset(
      makeDatasetResponse({ item_count: 1 }),
      itemsResponse([item]),
    );
    server.use(
      http.get("/api/v1/agents/:account/:name/evaluation-set", () =>
        HttpResponse.json({ error: "boom" }, { status: 500 }),
      ),
    );

    const user = userEvent.setup();
    renderDataset();

    expect(await screen.findByText("Unset prompt")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^trace actions$/i }));

    await waitFor(() => {
      expect(
        screen.getByRole("menuitem", { name: /edit evaluations/i }),
      ).toHaveAttribute("data-disabled");
    });
    expect(
      screen.getByRole("menuitem", { name: /remove from dataset/i }),
    ).not.toHaveAttribute("data-disabled");
  });

  it("updates a dataset item's evaluator values in place", async () => {
    const item = datasetItem({
      id: "dataset-item-change",
      input: "Change prompt",
      expected_output: "Change response",
      source_trace_id: "trace-change",
      evaluator_outputs: [
        { key: "exposed_pii", label: "Exposed PII", value: false },
      ],
    });
    let items = [item];
    let updated:
      | {
          traceId: string;
          body: { values: { key: string; value: unknown }[] };
        }
      | null = null;

    setupDataset(
      makeDatasetResponse({ item_count: 1 }),
      itemsResponse(items),
    );
    server.use(
      http.get("/api/v1/deployments/:id/dataset/items", () =>
        HttpResponse.json(itemsResponse(items)),
      ),
      http.put(
        "/api/v1/deployments/:id/dataset/items/:traceId/evaluator-outputs",
        async ({ params, request }) => {
          const traceId = String(params.traceId);
          const body = (await request.json()) as {
            values: { key: string; value: unknown }[];
          };
          updated = { traceId, body };
          items = [
            {
              ...item,
              evaluator_outputs: body.values.map((value) => ({
                ...value,
                label: "Exposed PII",
              })),
            },
          ];
          return HttpResponse.json({
            eval_dataset_id: "dataset-1",
            trace_id: traceId,
            evaluation_ref: EVALUATION_REF,
            evaluator_outputs: body.values,
            verified_by_user_id: "user_1",
          });
        },
      ),
    );

    const user = userEvent.setup();
    renderDataset();

    expect(await screen.findByText("Change prompt")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^trace actions$/i }));
    await user.click(
      await screen.findByRole("menuitem", { name: /edit evaluations/i }),
    );
    await user.click(screen.getByRole("combobox", { name: "Exposed PII" }));
    await user.click(screen.getByRole("option", { name: "True" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(updated).toEqual({
        traceId: "trace-change",
        body: { values: [{ key: "exposed_pii", value: true }] },
      });
    });
    await waitFor(() => {
      const row = screen.getByText("Change prompt").closest("tr");
      expect(row).not.toBeNull();
      expect(within(row!).getByText(/Exposed PII: True/)).toBeInTheDocument();
    });
  });

  it("removes a dataset item from the dataset", async () => {
    const item = datasetItem({
      id: "dataset-item-neutral",
      input: "Neutral prompt",
      expected_output: "Neutral response",
      source_trace_id: "trace-neutral",
    });
    let items = [item];
    let deletedTraceId: string | null = null;

    setupDataset(
      makeDatasetResponse({ item_count: 1 }),
      itemsResponse(items),
    );
    server.use(
      http.get("/api/v1/deployments/:id/dataset/items", () =>
        HttpResponse.json(itemsResponse(items)),
      ),
      http.delete(
        "/api/v1/deployments/:id/dataset/items/:traceId",
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

  it("renders a neutral overview when the dataset is empty", async () => {
    setupDataset(makeDatasetResponse({ item_count: 0 }));
    renderDataset();

    expect(
      await screen.findByText("No evaluator values recorded yet."),
    ).toBeInTheDocument();
  });

  it("requests items with page pagination", async () => {
    const seenPages: string[] = [];
    setupDataset(makeDatasetResponse({ item_count: 2 }));
    server.use(
      http.get("/api/v1/deployments/:id/dataset/items", ({ request }) => {
        const params = new URL(request.url).searchParams;
        const page = params.get("page") ?? "";
        seenPages.push(page);
        return HttpResponse.json({
          ...itemsResponse([datasetItem({ id: `item-${page}` })]),
          page: Number(page),
          total_items: 2,
          total_pages: 2,
        });
      }),
    );

    const user = userEvent.setup();
    renderDataset();

    await waitFor(() => {
      expect(seenPages).toContain("1");
    });

    await user.click(await screen.findByRole("button", { name: /show 1 more/i }));

    await waitFor(() => {
      expect(seenPages).toContain("2");
    });
  });
});
