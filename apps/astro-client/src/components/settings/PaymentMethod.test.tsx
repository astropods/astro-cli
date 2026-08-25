import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PaymentMethod } from "./PaymentMethod";

const mockPaymentMethod = vi.fn();
const mockRemove = vi.fn();
const mockSetupIntent = vi.fn();
const mockLoadStripe = vi.fn((key: string) => Promise.resolve({ key }));

vi.mock("@stripe/stripe-js", () => ({ loadStripe: (key: string) => mockLoadStripe(key) }));
// The real Elements provider awaits the Stripe promise before rendering
// children meaningfully; standing in for it lets a test read exactly what
// clientSecret it was given without waiting on a real Stripe round trip.
vi.mock("@stripe/react-stripe-js", () => ({
  Elements: ({ options, children }: { options: { clientSecret: string }; children: ReactNode }) => (
    <div data-testid="stripe-elements" data-client-secret={options.clientSecret}>
      {children}
    </div>
  ),
  PaymentElement: () => null,
  useElements: () => null,
  useStripe: () => null,
}));
vi.mock("@/lib/auth", () => ({ useAuth: () => ({ user: { email: "owner@acme.dev" } }) }));
vi.mock("@/api/queries/billing", () => ({
  usePaymentMethod: () => mockPaymentMethod(),
  useConfirmPaymentMethod: () => ({ mutateAsync: vi.fn() }),
  useDeletePaymentMethod: () => ({ mutate: mockRemove, isPending: false }),
  // AddCardDialog is always mounted (its own `open` prop just hides Dialog
  // content), so this fires even in tests that never open it.
  useCreateSetupIntent: () => mockSetupIntent(),
}));

function renderCard() {
  return render(<PaymentMethod account="acme" />);
}

type SetupIntentState = {
  mutate: ReturnType<typeof vi.fn>;
  reset: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  data: { client_secret: string; publishable_key: string } | undefined;
};

function setupIntentState(partial: Partial<SetupIntentState> = {}): SetupIntentState {
  return {
    mutate: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    isError: false,
    isSuccess: false,
    data: undefined,
    ...partial,
  };
}

beforeEach(() => {
  mockPaymentMethod.mockReset();
  mockRemove.mockReset();
  mockSetupIntent.mockReset().mockReturnValue(setupIntentState());
  mockLoadStripe.mockClear();
});

describe("PaymentMethod loading", () => {
  it("holds the card's shape instead of an inline loading string", () => {
    mockPaymentMethod.mockReturnValue({ data: undefined, isLoading: true, isError: false });
    const { container } = renderCard();

    expect(screen.queryByText("Loading…")).not.toBeInTheDocument();
    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0);
    // Card presence decides the button label; unknown yet, so neither shows.
    expect(screen.queryByRole("button", { name: "Add credit card" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update" })).not.toBeInTheDocument();
  });
});

describe("PaymentMethod that's never loaded successfully", () => {
  it("shows a retry state, keeping the section's scroll anchor", () => {
    const refetch = vi.fn();
    mockPaymentMethod.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch });
    const { container } = renderCard();

    expect(screen.getByText("Couldn't load this.")).toBeInTheDocument();
    expect(container.querySelector("#payment-details")).not.toBeNull();
  });

  it("retries the query on click", async () => {
    const refetch = vi.fn();
    mockPaymentMethod.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch });
    renderCard();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

describe("PaymentMethod when a background refetch fails after loading", () => {
  it("keeps showing the loaded card instead of an error banner", () => {
    mockPaymentMethod.mockReturnValue({
      data: { available: true, card: { brand: "visa", last4: "4242", exp_month: 4, exp_year: 2028 } },
      isLoading: false,
      isError: true,
      isLoadingError: false,
    });
    renderCard();

    expect(screen.getByText(/Visa •••• 4242/)).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this.")).not.toBeInTheDocument();
  });
});

describe("PaymentMethod when payments aren't configured", () => {
  it("shows a coming-soon placeholder with the add button disabled", () => {
    mockPaymentMethod.mockReturnValue({ data: { available: false }, isLoading: false, isError: false });
    renderCard();

    expect(screen.getByText("Coming soon")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add credit card" })).toBeDisabled();
  });
});

describe("PaymentMethod with no card on file", () => {
  it("offers to add one and shows the signed-in billing email", () => {
    mockPaymentMethod.mockReturnValue({ data: { available: true, card: undefined }, isLoading: false, isError: false });
    renderCard();

    expect(screen.getByText("No payment method on file")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add credit card" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove" })).not.toBeInTheDocument();
    expect(screen.getByText("owner@acme.dev")).toBeInTheDocument();
  });
});

describe("PaymentMethod with a card on file", () => {
  it("shows the card summary and offers to update or remove it", async () => {
    mockPaymentMethod.mockReturnValue({
      data: {
        available: true,
        card: { brand: "visa", last4: "4242", exp_month: 4, exp_year: 2028 },
      },
      isLoading: false,
      isError: false,
    });
    renderCard();

    expect(screen.getByText(/Visa •••• 4242 · Expires 04\/28/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Update" })).toBeInTheDocument();

    // Remove opens a confirmation dialog rather than removing the card on
    // the first click; RemovePaymentMethodDialog owns the actual mutation.
    await userEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Remove payment method" })).toBeInTheDocument();
    expect(mockRemove).not.toHaveBeenCalled();
  });
});

describe("PaymentMethod add-card dialog", () => {
  it("shows a retry button, not a dead-end message, when starting card setup fails", async () => {
    mockPaymentMethod.mockReturnValue({ data: { available: true, card: undefined }, isLoading: false, isError: false });
    const mutate = vi.fn();
    mockSetupIntent.mockReturnValue(setupIntentState({ mutate, isError: true }));
    renderCard();

    await userEvent.click(screen.getByRole("button", { name: "Add credit card" }));

    expect(await screen.findByText("Couldn't start card setup.")).toBeInTheDocument();
    const callsBeforeRetry = mutate.mock.calls.length;
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(mutate.mock.calls.length).toBeGreaterThan(callsBeforeRetry);
  });

  it("explains when payments aren't configured for the environment", async () => {
    mockPaymentMethod.mockReturnValue({ data: { available: true, card: undefined }, isLoading: false, isError: false });
    mockSetupIntent.mockReturnValue(
      setupIntentState({ isSuccess: true, data: { client_secret: "", publishable_key: "" } }),
    );
    renderCard();

    await userEvent.click(screen.getByRole("button", { name: "Add credit card" }));

    expect(await screen.findByText("Payments are not configured.")).toBeInTheDocument();
  });

  it("keeps the client secret paired with the attempt it came from when a stale response arrives late", async () => {
    mockPaymentMethod.mockReturnValue({ data: { available: true, card: undefined }, isLoading: false, isError: false });
    const mutate = vi.fn();
    mockSetupIntent.mockReturnValue(setupIntentState({ mutate }));
    renderCard();

    await userEvent.click(screen.getByRole("button", { name: "Add credit card" }));
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("button", { name: "Add credit card" }));
    expect(mutate).toHaveBeenCalledTimes(2);

    const [stale, fresh] = mutate.mock.calls.map((call) => call[1].onSuccess);
    // The newer attempt's response lands first; the older one arrives after.
    act(() => fresh({ client_secret: "secret-fresh", publishable_key: "pk_fresh" }));
    act(() => stale({ client_secret: "secret-stale", publishable_key: "pk_stale" }));

    const elements = await screen.findByTestId("stripe-elements");
    expect(elements).toHaveAttribute("data-client-secret", "secret-fresh");
    expect(mockLoadStripe).toHaveBeenLastCalledWith("pk_fresh");
  });

  it("discards a response that lands after the dialog is closed, not just after a newer attempt", async () => {
    mockPaymentMethod.mockReturnValue({ data: { available: true, card: undefined }, isLoading: false, isError: false });
    const mutate = vi.fn();
    mockSetupIntent.mockReturnValue(setupIntentState({ mutate }));
    renderCard();

    await userEvent.click(screen.getByRole("button", { name: "Add credit card" }));
    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    // The first attempt's request is still in flight when the dialog closes.
    const abandoned = mutate.mock.calls[0][1].onSuccess;
    act(() => abandoned({ client_secret: "secret-abandoned", publishable_key: "pk-abandoned" }));
    expect(screen.queryByTestId("stripe-elements")).not.toBeInTheDocument();

    // Reopening starts a fresh attempt; the abandoned response must not have
    // left a secret behind for this render to mount Elements against before
    // the new one resolves.
    await userEvent.click(screen.getByRole("button", { name: "Add credit card" }));
    expect(screen.queryByTestId("stripe-elements")).not.toBeInTheDocument();

    const fresh = mutate.mock.calls[1][1].onSuccess;
    act(() => fresh({ client_secret: "secret-fresh", publishable_key: "pk-fresh" }));

    const elements = await screen.findByTestId("stripe-elements");
    expect(elements).toHaveAttribute("data-client-secret", "secret-fresh");
  });
});
