import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FreeTrialModalHost, NO_GRANT_MAX_RETRIES, NO_GRANT_RETRY_MS } from "./FreeTrialModalHost";

const PENDING_KEY = "astro:free-trial-modal:pending:user-1";

const mockBalances = vi.fn();
vi.mock("@/api/queries/billing", () => ({
  useBillingBalances: (account: string) => mockBalances(account),
}));

const mockBlueprints = vi.fn();
vi.mock("@/api/queries/blueprints", () => ({
  useAccountBlueprints: (account: string) => mockBlueprints(account),
}));

vi.mock("@/hooks/use-active-account", () => ({
  useActiveAccount: () => ({ activeAccount: "acme" }),
}));

vi.mock("@/lib/auth", () => ({
  useAuth: () => ({ user: { id: "user-1" } }),
}));

let searchParams = new URLSearchParams();
const setSearchParams = vi.fn((next: URLSearchParams) => {
  searchParams = next;
});
const mockNavigate = vi.fn();
vi.mock("react-router", () => ({
  useSearchParams: () => [searchParams, setSearchParams],
  useNavigate: () => mockNavigate,
}));

function grant(granted: number) {
  return {
    available: true,
    data: {
      credits: [
        {
          name: "Signup credit",
          balance: granted,
          access_schedule: {
            credit_type: { name: "USD credits" },
            schedule_items: [{ amount: granted, ending_before: "2027-01-01" }],
          },
        },
      ],
    },
  };
}

beforeEach(() => {
  mockBalances.mockReset();
  mockBlueprints.mockReset();
  mockBlueprints.mockReturnValue({ data: { agents: [{ name: "agent-1" }] } });
  setSearchParams.mockClear();
  mockNavigate.mockClear();
  searchParams = new URLSearchParams();
});

afterEach(() => {
  localStorage.removeItem(PENDING_KEY);
  vi.useRealTimers();
});

describe("FreeTrialModalHost", () => {
  // Mounts on every page, so it must not query for sessions that never open.
  it("renders nothing and queries nothing when not pending and no QA override", () => {
    mockBalances.mockReturnValue({ data: grant(20), isLoading: false });
    const { container } = render(<FreeTrialModalHost />);
    expect(container).toBeEmptyDOMElement();
    expect(mockBalances).not.toHaveBeenCalled();
  });

  it("opens once the pending flag is set", () => {
    localStorage.setItem(PENDING_KEY, "true");
    mockBalances.mockReturnValue({ data: grant(20), isLoading: false });
    render(<FreeTrialModalHost />);
    expect(screen.getByRole("button", { name: /deploy an agent/i })).toBeInTheDocument();
  });

  it("clears the pending flag once closed", async () => {
    const user = userEvent.setup();
    localStorage.setItem(PENDING_KEY, "true");
    mockBalances.mockReturnValue({ data: grant(20), isLoading: false });
    render(<FreeTrialModalHost />);

    await user.click(screen.getByRole("button", { name: /close/i }));

    expect(localStorage.getItem(PENDING_KEY)).toBeNull();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  // A CTA that only closed the card would dead-end the one conversion moment.
  it("navigates to the account's blueprints on the CTA when it has any, and clears pending", async () => {
    const user = userEvent.setup();
    localStorage.setItem(PENDING_KEY, "true");
    mockBalances.mockReturnValue({ data: grant(20), isLoading: false });
    mockBlueprints.mockReturnValue({ data: { agents: [{ name: "agent-1" }] } });
    render(<FreeTrialModalHost />);

    await user.click(screen.getByRole("button", { name: /deploy an agent/i }));

    expect(mockNavigate).toHaveBeenCalledWith("/blueprints?account=acme");
    expect(localStorage.getItem(PENDING_KEY)).toBeNull();
  });

  it("navigates to explore on the CTA when the account has no blueprints", async () => {
    const user = userEvent.setup();
    localStorage.setItem(PENDING_KEY, "true");
    mockBalances.mockReturnValue({ data: grant(20), isLoading: false });
    mockBlueprints.mockReturnValue({ data: { agents: [] } });
    render(<FreeTrialModalHost />);

    await user.click(screen.getByRole("button", { name: /deploy an agent/i }));

    expect(mockNavigate).toHaveBeenCalledWith("/explore");
  });

  it("opens on the ?freeTrial=1 override regardless of the pending flag", () => {
    searchParams = new URLSearchParams("freeTrial=1");
    mockBalances.mockReturnValue({ data: grant(20), isLoading: false });
    render(<FreeTrialModalHost />);
    expect(screen.getByRole("button", { name: /deploy an agent/i })).toBeInTheDocument();
  });

  it("waits for balances to load before deciding, without clearing pending", () => {
    localStorage.setItem(PENDING_KEY, "true");
    mockBalances.mockReturnValue({ data: undefined, isLoading: true });
    const { container } = render(<FreeTrialModalHost />);
    expect(container).toBeEmptyDOMElement();
    expect(localStorage.getItem(PENDING_KEY)).toBe("true");
  });

  // A row came back, just not a qualifying one, so retrying cannot help.
  it("stays closed for a grant that is not money, without retrying", () => {
    localStorage.setItem(PENDING_KEY, "true");
    const refetch = vi.fn();
    mockBalances.mockReturnValue({
      data: {
        available: true,
        data: {
          credits: [
            {
              name: "Token grant",
              balance: 1000,
              access_schedule: {
                credit_type: { name: "Tokens" },
                schedule_items: [{ amount: 1000 }],
              },
            },
          ],
        },
      },
      isLoading: false,
      isError: false,
      refetch,
    });
    const { container } = render(<FreeTrialModalHost />);
    expect(container).toBeEmptyDOMElement();
    expect(refetch).not.toHaveBeenCalled();
    expect(localStorage.getItem(PENDING_KEY)).toBeNull();
  });

  it("retries a successful but empty response before clearing the pending flag", () => {
    vi.useFakeTimers();
    localStorage.setItem(PENDING_KEY, "true");
    const refetch = vi.fn();
    mockBalances.mockReturnValue({
      data: { available: true, data: { credits: [] } },
      isLoading: false,
      isError: false,
      refetch,
    });
    const { container } = render(<FreeTrialModalHost />);

    // A credit granted off the request path may not have landed yet.
    for (let i = 0; i < NO_GRANT_MAX_RETRIES - 1; i++) {
      expect(localStorage.getItem(PENDING_KEY)).toBe("true");
      act(() => {
        vi.advanceTimersByTime(NO_GRANT_RETRY_MS);
      });
    }
    expect(refetch).toHaveBeenCalledTimes(NO_GRANT_MAX_RETRIES - 1);
    expect(localStorage.getItem(PENDING_KEY)).toBe("true");

    act(() => {
      vi.advanceTimersByTime(NO_GRANT_RETRY_MS);
    });
    expect(container).toBeEmptyDOMElement();
    expect(localStorage.getItem(PENDING_KEY)).toBeNull();
  });

  // An empty response resolves (above). An error never answered, so the window
  // closes without spending a flag only account creation sets.
  it("stops retrying a persistent query error without clearing the pending flag", () => {
    vi.useFakeTimers();
    localStorage.setItem(PENDING_KEY, "true");
    const refetch = vi.fn();
    mockBalances.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch });
    const { container } = render(<FreeTrialModalHost />);

    for (let i = 0; i < NO_GRANT_MAX_RETRIES - 1; i++) {
      expect(localStorage.getItem(PENDING_KEY)).toBe("true");
      act(() => {
        vi.advanceTimersByTime(NO_GRANT_RETRY_MS);
      });
    }
    expect(refetch).toHaveBeenCalledTimes(NO_GRANT_MAX_RETRIES - 1);

    act(() => {
      vi.advanceTimersByTime(NO_GRANT_RETRY_MS);
    });
    expect(container).toBeEmptyDOMElement();
    // The flag survives, so a later load retries once the endpoint recovers.
    expect(localStorage.getItem(PENDING_KEY)).toBe("true");

    // And it stops asking: the window is closed, not merely paused.
    const callsAtExhaustion = refetch.mock.calls.length;
    act(() => {
      vi.advanceTimersByTime(NO_GRANT_RETRY_MS * 3);
    });
    expect(refetch).toHaveBeenCalledTimes(callsAtExhaustion);
  });

  // available:false is "could not tell us", and covers a failed customer
  // resolve, which runs on first billing access.
  it("stops retrying an unavailable response without clearing the pending flag", () => {
    vi.useFakeTimers();
    localStorage.setItem(PENDING_KEY, "true");
    const refetch = vi.fn();
    mockBalances.mockReturnValue({
      data: { available: false },
      isLoading: false,
      isError: false,
      refetch,
    });
    const { container } = render(<FreeTrialModalHost />);

    for (let i = 0; i < NO_GRANT_MAX_RETRIES; i++) {
      act(() => {
        vi.advanceTimersByTime(NO_GRANT_RETRY_MS);
      });
    }
    expect(container).toBeEmptyDOMElement();
    expect(localStorage.getItem(PENDING_KEY)).toBe("true");

    const callsAtExhaustion = refetch.mock.calls.length;
    act(() => {
      vi.advanceTimersByTime(NO_GRANT_RETRY_MS * 3);
    });
    expect(refetch).toHaveBeenCalledTimes(callsAtExhaustion);
  });

  it("keeps the pending flag through an error and shows the grant once the query recovers", () => {
    vi.useFakeTimers();
    localStorage.setItem(PENDING_KEY, "true");
    const refetch = vi.fn();
    mockBalances.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch });
    render(<FreeTrialModalHost />);

    // Two retries into the error, well short of NO_GRANT_MAX_RETRIES.
    act(() => {
      vi.advanceTimersByTime(NO_GRANT_RETRY_MS * 2);
    });
    expect(localStorage.getItem(PENDING_KEY)).toBe("true");

    mockBalances.mockReturnValue({ data: grant(20), isLoading: false, isError: false, refetch });
    act(() => {
      vi.advanceTimersByTime(NO_GRANT_RETRY_MS);
    });

    expect(screen.getByRole("button", { name: /deploy an agent/i })).toBeInTheDocument();
    expect(localStorage.getItem(PENDING_KEY)).toBe("true");
  });
});
