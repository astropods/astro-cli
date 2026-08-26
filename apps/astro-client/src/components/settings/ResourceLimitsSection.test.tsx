import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ResourceLimitsSection } from "./ResourceLimitsSection";

const mockAccountUsage = vi.fn();
const mockQuotaRequests = vi.fn();

vi.mock("@/api/queries", () => ({
  useAccountUsage: () => mockAccountUsage(),
  useQuotaIncreaseRequests: () => mockQuotaRequests(),
}));

function renderSection(props: Partial<{ canRequestIncrease: boolean }> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ResourceLimitsSection account="acme" canRequestIncrease {...props} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockAccountUsage.mockReturnValue({
    data: { meters: { agent_deployments: { usage: 3, quota: 10 } } },
    isLoading: false,
    isError: false,
  });
  mockQuotaRequests.mockReturnValue({ data: undefined, isLoading: false, isError: false });
});

describe("ResourceLimitsSection meters", () => {
  it("renders the meter grid", () => {
    renderSection();
    expect(screen.getByText("Deployments")).toBeInTheDocument();
    expect(screen.getByText("3 / 10")).toBeInTheDocument();
  });

  it("shows an empty state when the account has no meters", () => {
    mockAccountUsage.mockReturnValue({ data: { meters: {} }, isLoading: false, isError: false });
    renderSection();

    expect(screen.getByText("No usage data available.")).toBeInTheDocument();
  });

  it("shows a retry state on a meters query that's never loaded, distinct from an empty account", async () => {
    const refetch = vi.fn();
    mockAccountUsage.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch });
    renderSection();

    expect(screen.getByText("Couldn't load this.")).toBeInTheDocument();
    expect(screen.queryByText("No usage data available.")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("keeps the request-increase button disabled while meters can't be read", () => {
    mockAccountUsage.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: vi.fn() });
    renderSection();

    expect(screen.getByRole("button", { name: "Request increase" })).toBeDisabled();
  });

  it("keeps showing loaded meters through a background refetch failure", () => {
    mockAccountUsage.mockReturnValue({
      data: { meters: { agent_deployments: { usage: 3, quota: 10 } } },
      isLoading: false,
      isError: true,
      isLoadingError: false,
    });
    renderSection();

    expect(screen.getByText("Deployments")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this.")).not.toBeInTheDocument();
  });
});

describe("ResourceLimitsSection quota requests", () => {
  it("hides the whole section when there are no requests", () => {
    renderSection();

    expect(screen.queryByText("Quota increase requests")).not.toBeInTheDocument();
  });

  it("shows a retry state on a requests query that's never loaded", async () => {
    const refetch = vi.fn();
    mockQuotaRequests.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch });
    renderSection();

    expect(screen.getByText("Couldn't load this.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("keeps showing loaded requests through a background refetch failure", () => {
    mockQuotaRequests.mockReturnValue({
      data: {
        requests: [
          {
            id: "r1",
            feature_key: "blueprints",
            reason: "Need more room to prototype",
            requested_amount: 20,
            status: "pending",
            created_at: "2026-08-01T00:00:00Z",
          },
        ],
      },
      isLoading: false,
      isError: true,
      isLoadingError: false,
    });
    renderSection();

    expect(screen.getByText("Blueprints")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this.")).not.toBeInTheDocument();
  });

  it("renders requested rows once they load", () => {
    mockQuotaRequests.mockReturnValue({
      data: {
        requests: [
          {
            id: "r1",
            feature_key: "blueprints",
            reason: "Need more room to prototype",
            requested_amount: 20,
            status: "pending",
            // Noon UTC, not midnight: a request is an instant the viewer
            // reads in their own timezone (unlike a UTC-midnight billing
            // boundary), and noon keeps the calendar day stable across any
            // timezone this suite might run in.
            created_at: "2026-08-01T12:00:00Z",
          },
        ],
      },
      isLoading: false,
      isError: false,
    });
    renderSection();

    expect(screen.getByText("Blueprints")).toBeInTheDocument();
    expect(screen.getByText("Need more room to prototype")).toBeInTheDocument();
    expect(screen.getByText("pending")).toBeInTheDocument();
    expect(screen.getByText("Aug 1, 2026")).toBeInTheDocument();
  });
});
