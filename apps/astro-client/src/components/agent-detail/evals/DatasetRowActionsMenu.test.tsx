import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DatasetRowActionsMenu,
  type DatasetRowActionsMenuProps,
} from "./DatasetRowActionsMenu";

afterEach(cleanup);

function renderMenu(overrides: Partial<DatasetRowActionsMenuProps> = {}) {
  const props: DatasetRowActionsMenuProps = {
    traceId: "t1",
    verdict: "good",
    savedCriteriaKeys: [],
    isChanging: false,
    isRemoving: false,
    isSavingCriteria: false,
    onChangeVerdict: vi.fn(),
    onRemoveVerdict: vi.fn(),
    onSaveCriteria: vi.fn(),
    ...overrides,
  };
  const view = render(<DatasetRowActionsMenu {...props} />);
  return { ...view, props };
}

function openMenu() {
  return userEvent.setup().click(
    screen.getByRole("button", { name: /trace actions/i }),
  );
}

describe("DatasetRowActionsMenu verdict options", () => {
  it("highlights the active verdict", async () => {
    renderMenu({ verdict: "good" });
    await openMenu();
    expect(screen.getByRole("menuitem", { name: "Good" })).toHaveClass(
      "bg-success/15",
    );
    expect(screen.getByRole("menuitem", { name: "Bad" })).not.toHaveClass(
      "bg-destructive/15",
    );
  });

  it("changing to another verdict calls onChangeVerdict and keeps the menu open", async () => {
    const { props } = renderMenu({ verdict: "good" });
    await openMenu();
    await userEvent.setup().click(screen.getByRole("menuitem", { name: "Bad" }));

    expect(props.onChangeVerdict).toHaveBeenCalledWith("t1", "bad");
    // Good/bad keep the menu open so criteria can be adjusted.
    expect(screen.getByText("Change verdict")).toBeInTheDocument();
  });

  it("selecting the already-active verdict is a no-op", async () => {
    const { props } = renderMenu({ verdict: "good" });
    await openMenu();
    await userEvent.setup().click(screen.getByRole("menuitem", { name: "Good" }));
    expect(props.onChangeVerdict).not.toHaveBeenCalled();
  });

  it("calls onRemoveVerdict from the remove item", async () => {
    const { props } = renderMenu({ verdict: "good" });
    await openMenu();
    await userEvent
      .setup()
      .click(screen.getByRole("menuitem", { name: /remove from dataset/i }));
    expect(props.onRemoveVerdict).toHaveBeenCalledWith(
      "t1",
      expect.any(HTMLElement),
    );
  });
});

describe("DatasetRowActionsMenu criteria", () => {
  it("omits the criteria section for a neutral item", async () => {
    renderMenu({ verdict: null });
    await openMenu();
    expect(screen.queryByText(/why is it/i)).not.toBeInTheDocument();
  });

  it("seeds pills from saved criteria and only shows Save once changed", async () => {
    renderMenu({ verdict: "good", savedCriteriaKeys: ["accuracy"] });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /trace actions/i }));

    expect(screen.getByText("Why is it good?")).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /correct info/i })).toHaveAttribute(
      "data-active",
    );
    expect(screen.queryByRole("menuitem", { name: /^save$/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole("menuitem", { name: /^complete$/i }));
    expect(screen.getByRole("menuitem", { name: /^save$/i })).toBeInTheDocument();
  });

  it("Save emits the selected criteria in display order", async () => {
    const { props } = renderMenu({
      verdict: "good",
      savedCriteriaKeys: ["accuracy"],
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /trace actions/i }));
    await user.click(screen.getByRole("menuitem", { name: /^complete$/i }));
    await user.click(screen.getByRole("menuitem", { name: /^save$/i }));

    expect(props.onSaveCriteria).toHaveBeenCalledWith(
      "t1",
      [
        { dimension_key: "accuracy", value: 1 },
        { dimension_key: "completeness", value: 1 },
      ],
      expect.any(Function),
    );
  });

  it("disables row actions and criteria chips while criteria are saving", async () => {
    const { props, rerender } = renderMenu({
      verdict: "good",
      savedCriteriaKeys: ["accuracy"],
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /trace actions/i }));

    rerender(<DatasetRowActionsMenu {...props} isSavingCriteria />);

    expect(screen.getByRole("menuitem", { name: "Bad" })).toHaveAttribute(
      "data-disabled",
    );
    expect(
      screen.getByRole("menuitem", { name: /remove from dataset/i }),
    ).toHaveAttribute("data-disabled");
    expect(screen.getByRole("menuitem", { name: /correct info/i })).toBeDisabled();

    await user.click(screen.getByRole("menuitem", { name: "Bad" }));
    await user.click(screen.getByRole("menuitem", { name: /correct info/i }));

    expect(props.onChangeVerdict).not.toHaveBeenCalled();
    expect(screen.queryByRole("menuitem", { name: /^save$/i })).not.toBeInTheDocument();
  });

  it("resets the selection when the verdict changes", async () => {
    const base: DatasetRowActionsMenuProps = {
      traceId: "t1",
      verdict: "good",
      savedCriteriaKeys: ["accuracy"],
      isChanging: false,
      isRemoving: false,
      isSavingCriteria: false,
      onChangeVerdict: vi.fn(),
      onRemoveVerdict: vi.fn(),
      onSaveCriteria: vi.fn(),
    };
    const { rerender } = render(<DatasetRowActionsMenu {...base} />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /trace actions/i }));
    await user.click(screen.getByRole("menuitem", { name: /^complete$/i }));
    expect(screen.getByRole("menuitem", { name: /^save$/i })).toBeInTheDocument();

    // A verdict change clears saved criteria server-side; the keyed remount
    // resets the pills, so Save disappears and bad-side labels appear.
    rerender(
      <DatasetRowActionsMenu {...base} verdict="bad" savedCriteriaKeys={[]} />,
    );
    expect(screen.getByText("Why is it bad?")).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /^save$/i })).not.toBeInTheDocument();
  });
});
