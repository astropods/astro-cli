import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { OverflowPopover } from "./OverflowPopover";

afterEach(cleanup);

const itemNoun = { singular: "reason", plural: "reasons" };

function renderPopover(trigger?: "click" | "hover") {
  render(
    <OverflowPopover overflow={2} total={3} itemNoun={itemNoun} trigger={trigger}>
      <ul>
        <li>Alpha</li>
        <li>Beta</li>
      </ul>
    </OverflowPopover>,
  );
  return screen.getByRole("button", { name: /show 3 reasons/i });
}

describe("OverflowPopover", () => {
  it("renders the +N chip with a pluralized aria-label", () => {
    expect(renderPopover()).toHaveTextContent("+2");
  });

  it("click mode: opens the panel on click", async () => {
    const user = userEvent.setup();
    const trigger = renderPopover("click");
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();
    await user.click(trigger);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
  });

  it("hover mode: opens on mouse enter and closes on mouse leave", () => {
    const trigger = renderPopover("hover");
    fireEvent.mouseEnter(trigger);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
    fireEvent.mouseLeave(trigger);
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();
  });

  // Regression: focus-driven opening caused the panel to reappear on tab return
  // (focus restored to the chip) and when focus bounced back on close.
  it("hover mode: focusing the chip does not open the panel", () => {
    const trigger = renderPopover("hover");
    fireEvent.focus(trigger);
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();
  });
});
