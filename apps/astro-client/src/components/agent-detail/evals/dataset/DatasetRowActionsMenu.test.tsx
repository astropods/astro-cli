import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EvaluationSetEvaluator } from "@/lib/api";
import {
  DatasetRowActionsMenu,
  type DatasetRowActionsMenuProps,
} from "./DatasetRowActionsMenu";

afterEach(cleanup);

const EVALUATORS: EvaluationSetEvaluator[] = [
  {
    key: "exposed_pii",
    label: "Exposed PII",
    description: "Flags personal data in the output.",
    type: "llm",
    output: { type: "boolean" },
  },
  {
    key: "claim_grounding",
    label: "Claim grounding",
    type: "llm",
    output: { type: "enum", options: ["grounded", "no_claims"] },
  },
];

function renderMenu(overrides: Partial<DatasetRowActionsMenuProps> = {}) {
  const props: DatasetRowActionsMenuProps = {
    traceId: "t1",
    evaluators: EVALUATORS,
    savedOutputs: [{ key: "exposed_pii", value: false }],
    isRemoving: false,
    isSavingOutputs: false,
    onRemove: vi.fn(),
    onSaveOutputs: vi.fn(),
    ...overrides,
  };
  const view = render(<DatasetRowActionsMenu {...props} />);
  return { ...view, props };
}

function openMenu(user: ReturnType<typeof userEvent.setup>) {
  return user.click(screen.getByRole("button", { name: /trace actions/i }));
}

describe("DatasetRowActionsMenu", () => {
  it("calls onRemove from the remove item", async () => {
    const user = userEvent.setup();
    const { props } = renderMenu();
    await openMenu(user);

    await user.click(
      screen.getByRole("menuitem", { name: /remove from dataset/i }),
    );

    expect(props.onRemove).toHaveBeenCalledWith(expect.any(HTMLElement));
  });

  it("locks editing for an item admitted under an older set", async () => {
    const user = userEvent.setup();
    renderMenu({ outdated: true });
    await openMenu(user);

    const edit = screen.getByRole("menuitem", { name: /edit evaluations/i });
    expect(edit).toHaveAttribute("data-disabled");
    await user.hover(edit.parentElement as HTMLElement);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "This item was added with an old evaluator.",
    );
  });

  it("locks editing until the evaluators arrive", async () => {
    const user = userEvent.setup();
    renderMenu({ editDisabled: true });
    await openMenu(user);

    const edit = screen.getByRole("menuitem", { name: /edit evaluations/i });
    expect(edit).toHaveAttribute("data-disabled");
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /remove from dataset/i }),
    ).not.toHaveAttribute("data-disabled");
  });

  it("blocks the actions while a mutation is in flight", async () => {
    const user = userEvent.setup();
    renderMenu({ isSavingOutputs: true });

    expect(
      screen.getByRole("button", { name: /trace actions/i }),
    ).toBeDisabled();

    await openMenu(user);

    expect(
      screen.getByRole("menuitem", { name: /remove from dataset/i }),
    ).toHaveAttribute("data-disabled");
    expect(
      screen.getByRole("menuitem", { name: /edit evaluations/i }),
    ).toHaveAttribute("data-disabled");
  });
});

describe("DatasetRowActionsMenu editing", () => {
  async function openEditor(user: ReturnType<typeof userEvent.setup>) {
    await openMenu(user);
    await user.click(screen.getByRole("menuitem", { name: /edit evaluations/i }));
  }

  it("seeds the controls from the item's saved values", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openEditor(user);

    expect(screen.getByText("Edit evaluator values")).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Exposed PII" }),
    ).toHaveTextContent("False");
    expect(
      screen.getByRole("combobox", { name: "Claim grounding" }),
    ).toHaveTextContent("Select");
  });

  it("closes without a request when nothing changed", async () => {
    const user = userEvent.setup();
    const { props } = renderMenu();
    await openEditor(user);

    const save = screen.getByRole("button", { name: "Save" });
    expect(save).toBeEnabled();
    await user.click(save);

    expect(props.onSaveOutputs).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("emits every value the item ends up with", async () => {
    const user = userEvent.setup();
    const { props } = renderMenu();
    await openEditor(user);

    await user.click(screen.getByRole("combobox", { name: "Claim grounding" }));
    await user.click(screen.getByRole("option", { name: "Grounded" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(props.onSaveOutputs).toHaveBeenCalledWith(
      "t1",
      [
        { key: "exposed_pii", value: false },
        { key: "claim_grounding", value: "grounded" },
      ],
      expect.any(Function),
    );
  });

  it("records a value the reviewer clears as unset", async () => {
    const user = userEvent.setup();
    const { props } = renderMenu();
    await openEditor(user);

    await user.click(screen.getByRole("button", { name: "Clear Exposed PII" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(props.onSaveOutputs).toHaveBeenCalledWith("t1", [], expect.any(Function));
  });

  it("offers each evaluator's definition beside its label", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openEditor(user);

    await user.hover(screen.getByLabelText("About Exposed PII"));
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Flags personal data in the output.",
    );
  });
});
