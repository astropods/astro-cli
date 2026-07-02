import { render, screen, cleanup, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ChatAgentSwitchCoachmark } from "./ChatAgentSwitchCoachmark";

afterEach(cleanup);

describe("ChatAgentSwitchCoachmark", () => {
  it("mounts the polite live region empty, then fills it after mount so it announces", async () => {
    render(<ChatAgentSwitchCoachmark onClose={() => {}} />);

    // The region must exist in the DOM before its text is inserted, otherwise a
    // region that mounts already-populated is not announced by screen readers.
    const region = screen.getByRole("status");
    expect(region).toHaveAttribute("aria-live", "polite");
    expect(region).toHaveTextContent("");

    await waitFor(() => expect(region).toHaveTextContent("Switch agents here"));
  });
});
