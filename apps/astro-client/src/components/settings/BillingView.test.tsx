import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { buildSpendResponse } from "@/api/queries/billing.fixtures";
import { BillingView } from "./BillingView";

const mockInvoices = vi.fn();
const mockDownload = vi.fn();
const mockSpend = vi.fn();

const mockRole = vi.fn(() => "owner");
vi.mock("@/lib/auth", () => ({ useAuth: () => ({ role: mockRole() }) }));

vi.mock("@/api/queries", () => ({
  useBillingInvoices: () => mockInvoices(),
  useDownloadInvoicePdf: () => ({ mutate: mockDownload, isPending: false }),
}));
vi.mock("@/api/queries/billing", () => ({
  useBillingSpend: () => mockSpend(),
  useBillingStatus: () => ({ data: { credits_exhausted: false, has_payment_method: true } }),
  useSetBillingSpendThresholds: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
// Stripe Elements need a live publishable key and a network round trip.
vi.mock("@/components/settings/PaymentMethod", () => ({
  PaymentMethod: () => null,
}));

function renderView() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <BillingView account="acme" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockInvoices.mockReturnValue({ data: undefined, isLoading: false });
  mockSpend.mockReturnValue({
    data: buildSpendResponse({
      plan: "credit",
      has_usage_spend: false,
      has_current_spend: false,
      has_credit: true,
      credit_remaining: 20,
    }),
    isLoading: false,
  });
  mockDownload.mockReset();
});

describe("BillingView", () => {
  it("has no tab bar left over from the old layout", () => {
    renderView();
    for (const name of ["Usage", "Credits", "Quotas"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
  });

  it("shows the pay-as-you-go card and the invoices section on one page", () => {
    renderView();
    expect(screen.getByText("Pay-as-you-go")).toBeInTheDocument();
    expect(screen.getByText("Invoices")).toBeInTheDocument();
  });
});

describe("BillingView invoices", () => {
  it("downloads a finalized invoice under an account-scoped filename", async () => {
    mockInvoices.mockReturnValue({
      data: {
        available: true,
        data: [{ id: "inv-1", status: "FINALIZED", issued_at: "2026-08-01T00:00:00Z", total: 1200 }],
      },
      isLoading: false,
    });
    renderView();
    await userEvent.click(screen.getByRole("button", { name: "Download invoice" }));

    expect(mockDownload).toHaveBeenCalledWith(
      { invoiceId: "inv-1", filename: "acme-invoice-2026-08-01.pdf" },
      expect.anything(),
    );
  });
});

describe("BillingView invoices with more than one downloadable row", () => {
  it("disables only the row being downloaded, not every row", async () => {
    mockInvoices.mockReturnValue({
      data: {
        available: true,
        data: [
          { id: "inv-1", status: "FINALIZED", issued_at: "2026-08-01T00:00:00Z", total: 1200 },
          { id: "inv-2", status: "FINALIZED", issued_at: "2026-07-01T00:00:00Z", total: 1500 },
        ],
      },
      isLoading: false,
    });
    renderView();

    const [firstRow, secondRow] = screen.getAllByRole("button", { name: "Download invoice" });
    await userEvent.click(firstRow!);

    expect(firstRow).toBeDisabled();
    expect(secondRow).not.toBeDisabled();
  });

  it("keeps each row's disabled state independent when two downloads overlap", async () => {
    mockInvoices.mockReturnValue({
      data: {
        available: true,
        data: [
          { id: "inv-1", status: "FINALIZED", issued_at: "2026-08-01T00:00:00Z", total: 1200 },
          { id: "inv-2", status: "FINALIZED", issued_at: "2026-07-01T00:00:00Z", total: 1500 },
        ],
      },
      isLoading: false,
    });
    renderView();

    const [firstRow, secondRow] = screen.getAllByRole("button", { name: "Download invoice" });
    await userEvent.click(firstRow!);
    await userEvent.click(secondRow!);
    expect(firstRow).toBeDisabled();
    expect(secondRow).toBeDisabled();

    // The first click's download settles while the second is still in
    // flight. A single shared id would have cleared both rows here instead
    // of just the one that actually finished.
    const firstOnSettled = mockDownload.mock.calls[0]![1].onSettled;
    act(() => firstOnSettled());

    expect(firstRow).not.toBeDisabled();
    expect(secondRow).toBeDisabled();
  });
});

describe("BillingView invoices that have never loaded successfully", () => {
  it("shows a retry state instead of the empty-invoices message", () => {
    mockInvoices.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: vi.fn() });
    renderView();

    expect(screen.getByText("Couldn't load this.")).toBeInTheDocument();
    expect(screen.queryByText("No invoices yet.")).not.toBeInTheDocument();
  });

  it("retries the query on click", async () => {
    const refetch = vi.fn();
    mockInvoices.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch });
    renderView();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

describe("BillingView invoices when a background refetch fails after loading", () => {
  it("keeps showing already-loaded invoices instead of an error banner", () => {
    mockInvoices.mockReturnValue({
      data: {
        available: true,
        data: [{ id: "inv-1", status: "FINALIZED", issued_at: "2026-08-01T00:00:00Z", total: 1200 }],
      },
      isLoading: false,
      isError: true,
      isLoadingError: false,
    });
    renderView();

    expect(screen.getByRole("button", { name: "Download invoice" })).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this.")).not.toBeInTheDocument();
  });
});

describe("BillingView invoices without a PDF", () => {
  function renderDraft(partial: Record<string, unknown> = {}) {
    mockInvoices.mockReturnValue({
      data: {
        available: true,
        data: [
          {
            id: "inv_draft",
            status: "DRAFT",
            total: 1240,
            issued_at: "2026-08-01T00:00:00Z",
            end_timestamp: "2026-09-11T12:00:00Z",
            ...partial,
          },
        ],
      },
      isLoading: false,
    });
    renderView();
    return screen.getByRole("button", { name: "Download invoice" });
  }

  it("greys out the download rather than dropping the control", () => {
    expect(renderDraft()).toBeDisabled();
  });

  it("says when a draft invoice becomes downloadable", async () => {
    const trigger = renderDraft().parentElement!;
    await userEvent.hover(trigger);

    await waitFor(() =>
      expect(trigger).toHaveAccessibleDescription(
        "Available to download once this period closes on Sep 11, 2026.",
      ),
    );
  });

  it("falls back to a flat reason for an invoice that will never have a PDF", async () => {
    const trigger = renderDraft({ status: "VOID", end_timestamp: undefined }).parentElement!;
    await userEvent.hover(trigger);

    await waitFor(() => expect(trigger).toHaveAccessibleDescription("No PDF for this invoice."));
  });

  it("does not fire a download when clicked", async () => {
    const button = renderDraft();
    await userEvent.click(button);

    expect(mockDownload).not.toHaveBeenCalled();
  });
});
