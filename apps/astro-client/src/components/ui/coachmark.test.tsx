import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Coachmark } from "./coachmark";

afterEach(cleanup);

describe("Coachmark", () => {
  it("mounts the polite live region empty, then fills it after mount", async () => {
    render(
      <Coachmark
        open
        anchor={<button type="button">Agents</button>}
        announcement="Switch agents here"
      >
        Switch agents here
      </Coachmark>,
    );

    const region = screen.getByRole("status");
    expect(region).toHaveAttribute("aria-live", "polite");
    expect(region).toHaveTextContent("");

    await waitFor(() => expect(region).toHaveTextContent("Switch agents here"));
  });
});
