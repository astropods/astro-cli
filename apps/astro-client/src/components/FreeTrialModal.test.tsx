import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FreeTrialModal } from "./FreeTrialModal";

const mockIsMobile = vi.fn(() => false);
vi.mock("@/hooks/use-compact-layout", () => ({
  useIsMobile: () => mockIsMobile(),
}));

afterEach(cleanup);
beforeEach(() => {
  mockIsMobile.mockReset();
  mockIsMobile.mockReturnValue(false);
});

describe("FreeTrialModal", () => {
  it("renders nothing when closed", () => {
    const { container } = render(
      <FreeTrialModal open={false} onOpenChange={() => {}} credits={20} onCta={() => {}} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the badge, the starting balance, and the CTA when open", () => {
    render(<FreeTrialModal open onOpenChange={() => {}} credits={20} onCta={() => {}} />);
    expect(screen.getByRole("heading", { name: "Free credits on us" })).toBeInTheDocument();
    // Renders synchronously at $0.00 before the roll-up animation's timers fire.
    expect(screen.getByText("$0.00")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deploy an agent" })).toBeInTheDocument();
  });

  it("closes via the X button", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(<FreeTrialModal open onOpenChange={onOpenChange} credits={20} onCta={() => {}} />);

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("fires onCta when the CTA button is clicked", async () => {
    const user = userEvent.setup();
    const onCta = vi.fn();
    render(
      <FreeTrialModal open onOpenChange={() => {}} credits={20} ctaLabel="Start building" onCta={onCta} />,
    );

    await user.click(screen.getByRole("button", { name: "Start building" }));

    expect(onCta).toHaveBeenCalled();
  });

  it("renders as a centered dialog on desktop", () => {
    render(<FreeTrialModal open onOpenChange={() => {}} credits={20} onCta={() => {}} />);
    expect(document.querySelector('[data-slot="sheet-content"]')).not.toBeInTheDocument();
  });

  it("renders as a bottom sheet below the mobile breakpoint", () => {
    mockIsMobile.mockReturnValue(true);
    render(<FreeTrialModal open onOpenChange={() => {}} credits={20} onCta={() => {}} />);
    expect(document.querySelector('[data-slot="sheet-content"]')).toBeInTheDocument();
  });
});
