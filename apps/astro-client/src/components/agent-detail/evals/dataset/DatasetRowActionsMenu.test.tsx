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
    savedCriteriaKeys: [],
    isRemoving: false,
    isSavingCriteria: false,
    onRemove: vi.fn(),
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

describe("DatasetRowActionsMenu", () => {
  it("calls onRemove from the remove item", async () => {
    const { props } = renderMenu();
    await openMenu();
    await userEvent
      .setup()
      .click(screen.getByRole("menuitem", { name: /remove from dataset/i }));
    expect(props.onRemove).toHaveBeenCalledWith(expect.any(HTMLElement));
  });
});

describe("DatasetRowActionsMenu criteria", () => {
  it("seeds pills from saved criteria and only shows Save once changed", async () => {
    renderMenu({ savedCriteriaKeys: ["accuracy"] });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /trace actions/i }));

    expect(screen.getByText("Evaluate item")).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /correct info/i })).toHaveAttribute(
      "data-active",
    );
    expect(screen.queryByRole("menuitem", { name: /^save$/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole("menuitem", { name: /^complete$/i }));
    expect(screen.getByRole("menuitem", { name: /^save$/i })).toBeInTheDocument();
  });

  it("Save emits selected criteria as positive and omits unselected criteria", async () => {
    const { props } = renderMenu({
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
      savedCriteriaKeys: ["accuracy"],
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /trace actions/i }));

    rerender(<DatasetRowActionsMenu {...props} isSavingCriteria />);

    expect(
      screen.getByRole("menuitem", { name: /remove from dataset/i }),
    ).toHaveAttribute("data-disabled");
    expect(screen.getByRole("menuitem", { name: /correct info/i })).toBeDisabled();

    await user.click(screen.getByRole("menuitem", { name: /correct info/i }));

    expect(screen.queryByRole("menuitem", { name: /^save$/i })).not.toBeInTheDocument();
  });

  it("resets the selection when saved criteria change", async () => {
    const base: DatasetRowActionsMenuProps = {
      traceId: "t1",
      savedCriteriaKeys: ["accuracy"],
      isRemoving: false,
      isSavingCriteria: false,
      onRemove: vi.fn(),
      onSaveCriteria: vi.fn(),
    };
    const { rerender } = render(<DatasetRowActionsMenu {...base} />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /trace actions/i }));
    await user.click(screen.getByRole("menuitem", { name: /^complete$/i }));
    expect(screen.getByRole("menuitem", { name: /^save$/i })).toBeInTheDocument();

    rerender(<DatasetRowActionsMenu {...base} savedCriteriaKeys={[]} />);
    expect(screen.getByText("Evaluate item")).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /^save$/i })).not.toBeInTheDocument();
  });
});
