import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReviewQueueDatasetActions } from "./ReviewQueueDatasetActions";

afterEach(cleanup);

describe("ReviewQueueDatasetActions", () => {
  it("exposes only neutral membership actions", async () => {
    const onAdd = vi.fn();
    const onRemove = vi.fn();
    const user = userEvent.setup();
    render(
      <ReviewQueueDatasetActions
        isPending={false}
        showError={false}
        onAdd={onAdd}
        onRemove={onRemove}
      />,
    );

    const addButton = screen.getByRole("button", { name: "Add to dataset" });
    expect(addButton.querySelector("svg")).toBeInTheDocument();
    await user.click(addButton);
    const removeButton = screen.getByRole("button", { name: "Remove" });
    await user.hover(removeButton);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Remove from review queue",
    );
    await user.click(removeButton);
    expect(onAdd).toHaveBeenCalledWith(expect.any(HTMLElement));
    expect(onRemove).toHaveBeenCalledWith(expect.any(HTMLElement));
    expect(screen.queryByText(/good|bad|not sure/i)).not.toBeInTheDocument();
  });

  it("disables both actions while a mutation is pending", () => {
    render(
      <ReviewQueueDatasetActions
        isPending
        showError
        onAdd={vi.fn()}
        onRemove={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Add to dataset" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
    expect(screen.getByText(/could not update the review queue/i)).toBeInTheDocument();
  });
});
