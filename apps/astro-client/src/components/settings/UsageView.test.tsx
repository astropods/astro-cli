import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { BillingSpend } from "@/lib/api";
import { buildSpendResponse } from "@/api/queries/billing.fixtures";
import { periodDayKeys, UsageView } from "./UsageView";

const mockAccountUsage = vi.fn();
const mockBillingUsage = vi.fn();
const mockBillingDailySpend = vi.fn();
const mockQuotaRequests = vi.fn();
const mockSpend = vi.fn();

vi.mock("@/api/queries", () => ({
  useAccountUsage: () => mockAccountUsage(),
  useBillingUsage: (...args: unknown[]) => mockBillingUsage(...args),
  useBillingDailySpend: (...args: unknown[]) => mockBillingDailySpend(...args),
  useQuotaIncreaseRequests: () => mockQuotaRequests(),
}));
vi.mock("@/api/queries/billing", () => ({
  useBillingSpend: () => mockSpend(),
}));

function spendResponse(partial: Partial<BillingSpend> = {}) {
  return {
    data: buildSpendResponse({
      currency: "USD",
      current_period_end: "2026-09-11T00:00:00Z",
      current_spend: 45.02,
      usage_spend: 45.02,
      limit: { amount: 6000, in_alarm: false },
      ...partial,
    }),
    isLoading: false,
  };
}

function renderView(props: Partial<{ canRequestIncrease: boolean }> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <UsageView account="acme" {...props} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockAccountUsage.mockReturnValue({
    data: { meters: { agent_deployments: { usage: 3, quota: 10 }, blueprints: { usage: 8, quota: 5 } } },
    isLoading: false,
  });
  mockBillingUsage.mockReturnValue({ data: { available: true, data: [] }, isLoading: false, refetch: vi.fn() });
  mockBillingDailySpend.mockReturnValue({ data: { available: true, data: [] }, isLoading: false, refetch: vi.fn() });
  mockQuotaRequests.mockReturnValue({ data: undefined, isLoading: false });
  mockSpend.mockReturnValue(spendResponse());
});

describe("UsageView header", () => {
  it("shows the total spend against the converted spend limit", () => {
    renderView();
    // With no daily breakdown to split, the total shows once and neither
    // stream box fabricates a share of it by guessing.
    expect(screen.getByText("$45.02")).toBeInTheDocument();
    expect(screen.getAllByText("$0.00")).toHaveLength(2);
    expect(screen.getByText("of $60.00 spend limit")).toBeInTheDocument();
  });

  it("splits each stream from the daily breakdown's own rated by_product", () => {
    mockBillingDailySpend.mockReturnValue({
      data: {
        available: true,
        data: [
          {
            day: "2026-08-01T00:00:00Z",
            amount: 45.02,
            by_product: { "LLM Usage": 12.41, "Compute Units": 32.61 },
          },
        ],
      },
      isLoading: false,
      refetch: vi.fn(),
    });
    renderView();

    expect(screen.getByText("$12.41")).toBeInTheDocument();
    expect(screen.getByText("$32.61")).toBeInTheDocument();
  });
});

describe("UsageView spend breakdown", () => {
  // Nothing renders until the provider sends a grouping, so the page never
  // shows a breakdown that reads as "you spent nothing".
  it("shows no breakdown while the provider sends no grouping", () => {
    renderView();
    // "Models" also labels the header's split, so the table is the heading.
    expect(screen.queryByRole("heading", { name: "Models" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Agents" })).not.toBeInTheDocument();
  });

  it("renders a models table once the provider sends a grouped breakdown", () => {
    mockBillingUsage.mockReturnValue({
      data: {
        available: true,
        data: [
          {
            billable_metric_name: "LLM Usage",
            start_timestamp: "2026-08-01T00:00:00Z",
            value: 12.41,
            groups: { "gpt-4o": 8.1, "claude-sonnet-5": 4.31 },
          },
        ],
      },
      isLoading: false,
    });
    renderView();

    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    expect(screen.getByText("$8.10")).toBeInTheDocument();
  });
});

describe("UsageView period window", () => {
  it("reads the period straight from the server when it reports a start", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_period_start: "2026-08-20T00:00:00Z", current_period_end: "2026-09-11T00:00:00Z" }),
    );
    renderView();

    expect(mockBillingUsage).toHaveBeenCalledWith(
      "acme",
      { from: "2026-08-20T00:00:00.000Z", to: "2026-09-11T00:00:00.000Z" },
      { enabled: true },
    );
    // The header's split and the daily chart describe the same period, so
    // both queries have to share the exact same window.
    expect(mockBillingDailySpend).toHaveBeenCalledWith(
      "acme",
      { from: "2026-08-20T00:00:00.000Z", to: "2026-09-11T00:00:00.000Z" },
      { enabled: true },
    );
  });

  it("approximates the start by stepping back a month when the server omits it", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_period_start: undefined, current_period_end: "2026-09-11T00:00:00Z" }),
    );
    renderView();

    expect(mockBillingUsage).toHaveBeenCalledWith(
      "acme",
      { from: "2026-08-11T00:00:00.000Z", to: "2026-09-11T00:00:00.000Z" },
      { enabled: true },
    );
  });
});

describe("UsageView without a reported period", () => {
  // The header reads server totals, so it survives; a daily series cannot be
  // dated without a window and says so rather than guessing one.
  it("keeps the header and explains the missing daily series", () => {
    mockSpend.mockReturnValue(spendResponse({ current_period_end: undefined, limit: undefined }));
    renderView();

    // No window means no daily breakdown to split, so the total shows once
    // and neither stream box fabricates a share of it.
    expect(screen.getByText("$45.02")).toBeInTheDocument();
    expect(screen.getAllByText("$0.00")).toHaveLength(2);
    expect(
      screen.getByText("Daily usage isn't available until the provider reports a billing period."),
    ).toBeInTheDocument();
  });

  it("shows one unavailable notice, not one per section, when billing is off", () => {
    mockSpend.mockReturnValue({ data: { available: false }, isLoading: false });
    renderView();

    expect(
      screen.getAllByText(
        "Billing isn't available for this account yet. Data appears here once billing is enabled.",
      ),
    ).toHaveLength(1);
  });
});

describe("UsageView when the spend query has never loaded successfully", () => {
  it("shows a retry state instead of the unavailable message", () => {
    mockSpend.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: vi.fn() });
    renderView();

    expect(screen.getByText("Couldn't load this.")).toBeInTheDocument();
    expect(
      screen.queryByText("Billing isn't available for this account yet. Data appears here once billing is enabled."),
    ).not.toBeInTheDocument();
  });

  it("retries the query on click", async () => {
    const refetch = vi.fn();
    mockSpend.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch });
    renderView();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

describe("UsageView when the daily spend chart's queries have never loaded successfully", () => {
  it("keeps the header and shows a retry state for the chart alone, on a usage-rows failure", () => {
    mockBillingUsage.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: vi.fn() });
    renderView();

    expect(screen.getByText("of $60.00 spend limit")).toBeInTheDocument();
    expect(screen.getByText("Couldn't load daily usage.")).toBeInTheDocument();
  });

  it("keeps the header and shows a retry state for the chart alone, on a daily-spend failure", () => {
    mockBillingDailySpend.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: vi.fn() });
    renderView();

    expect(screen.getByText("of $60.00 spend limit")).toBeInTheDocument();
    expect(screen.getByText("Couldn't load daily usage.")).toBeInTheDocument();
  });

  it("retries both queries on click", async () => {
    const refetchUsage = vi.fn();
    const refetchDailySpend = vi.fn();
    mockBillingUsage.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: refetchUsage });
    mockBillingDailySpend.mockReturnValue({
      data: undefined,
      isLoading: false,
      isLoadingError: true,
      refetch: refetchDailySpend,
    });
    renderView();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(refetchUsage).toHaveBeenCalledTimes(1);
    expect(refetchDailySpend).toHaveBeenCalledTimes(1);
  });
});

describe("UsageView when a background refetch fails after loading", () => {
  it("keeps rendering loaded spend instead of an error banner", () => {
    mockSpend.mockReturnValue({ ...spendResponse(), isError: true, isLoadingError: false });
    renderView();

    expect(screen.getByText("of $60.00 spend limit")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this.")).not.toBeInTheDocument();
  });

  it("keeps rendering loaded daily usage instead of an error banner, on a usage-rows refetch failure", () => {
    mockBillingUsage.mockReturnValue({
      data: { available: true, data: [] },
      isLoading: false,
      isError: true,
      isLoadingError: false,
    });
    renderView();

    expect(screen.getByText("No usage recorded for this period.")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load daily usage.")).not.toBeInTheDocument();
  });

  it("keeps rendering loaded daily usage instead of an error banner, on a daily-spend refetch failure", () => {
    mockBillingDailySpend.mockReturnValue({
      data: { available: true, data: [] },
      isLoading: false,
      isError: true,
      isLoadingError: false,
    });
    renderView();

    expect(screen.getByText("No usage recorded for this period.")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load daily usage.")).not.toBeInTheDocument();
  });
});

describe("periodDayKeys", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("clamps to today instead of walking into an open period's future days", () => {
    vi.useFakeTimers();
    vi.setSystemTime("2026-08-25T15:00:00Z");

    const keys = periodDayKeys({ from: "2026-08-11T00:00:00Z", to: "2026-09-11T00:00:00Z" });

    // Aug 25 is today (included, via startOfTomorrow); Aug 26 through the
    // period's real end on Sep 11 haven't happened yet.
    expect(keys[keys.length - 1]).toBe("2026-08-25");
    expect(keys).not.toContain("2026-08-26");
    expect(keys).not.toContain("2026-09-11");
  });

  it("walks the full period once it has already closed", () => {
    vi.useFakeTimers();
    vi.setSystemTime("2026-09-20T00:00:00Z");

    const keys = periodDayKeys({ from: "2026-08-11T00:00:00Z", to: "2026-09-11T00:00:00Z" });

    expect(keys[0]).toBe("2026-08-11");
    expect(keys[keys.length - 1]).toBe("2026-09-10");
  });
});

describe("UsageView limits", () => {
  it("shows the resource meters and gates the request-increase button", () => {
    renderView({ canRequestIncrease: false });
    expect(screen.getByText("Deployments")).toBeInTheDocument();
    expect(screen.getByText("3 / 10")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Request increase" })).not.toBeInTheDocument();
  });

  it("shows the request-increase button when allowed", () => {
    renderView({ canRequestIncrease: true });
    expect(screen.getByRole("button", { name: "Request increase" })).toBeInTheDocument();
  });
});
