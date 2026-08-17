import {
  render,
  screen,
  cleanup,
  fireEvent,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DatasetItemRow, type ResolvedReviewer } from "./DatasetItemRow";
import type { EvalDatasetItem } from "@/lib/api";

afterEach(cleanup);

function makeItem(overrides: Partial<EvalDatasetItem> = {}): EvalDatasetItem {
  return {
    id: "item-1",
    input: "What is the capital of France?",
    expected_output: "Paris.",
    metadata: {
      judged_by_user_id: "user_1",
      judged_at: new Date(Date.now() - 60_000).toISOString(),
    },
    source_trace_id: "trace-1",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

const reviewer: ResolvedReviewer = { handle: "alice", name: "Alice" };

interface RenderOpts {
  item?: EvalDatasetItem;
  isOpen?: boolean;
  reviewer?: ResolvedReviewer | null;
}

function renderRow(opts: RenderOpts = {}) {
  const onToggle = vi.fn();
  const onRemove = vi.fn();
  const onSaveCriteria = vi.fn();
  render(
    <DatasetItemRow
      item={opts.item ?? makeItem()}
      isOpen={opts.isOpen ?? false}
      onToggle={onToggle}
      onRemove={onRemove}
      onSaveCriteria={onSaveCriteria}
      isRemoving={false}
      isSavingCriteria={false}
      reviewer={opts.reviewer === undefined ? reviewer : opts.reviewer}
    />,
  );
  return { onToggle, onRemove, onSaveCriteria };
}

describe("DatasetItemRow collapsed", () => {
  it("renders the reviewer without a verdict pill", () => {
    renderRow();
    expect(screen.queryByText("Good")).not.toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("does not render expanded preview when isOpen is false", () => {
    renderRow({ isOpen: false });
    expect(screen.queryByText(/^Input$/)).not.toBeInTheDocument();
    expect(screen.queryByText(/^Expected output$/)).not.toBeInTheDocument();
  });

  it("clicking the row toggles via onToggle with its id", () => {
    const { onToggle } = renderRow({ item: makeItem({ id: "abc" }) });
    const row = screen.getByText("What is the capital of France?").closest("tr");
    expect(row).not.toBeNull();
    fireEvent.click(row!);
    expect(onToggle).toHaveBeenCalledWith("abc");
  });

  it("removing a trace from the actions menu does not toggle the row", async () => {
    const { onToggle, onRemove } = renderRow({
      item: makeItem({ source_trace_id: "trace-undo" }),
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /^trace actions$/i }));
    await user.click(
      await screen.findByRole("menuitem", {
        name: /remove from dataset/i,
      }),
    );

    expect(onRemove).toHaveBeenCalledWith(expect.any(HTMLElement));
    expect(onToggle).not.toHaveBeenCalled();
  });

  it("activating the criterion overflow chip does not toggle the row", async () => {
    const { onToggle } = renderRow({
      item: makeItem({
        metadata: {
          judgment_criteria: [
            { dimension_key: "accuracy", value: 1 },
            { dimension_key: "completeness", value: 1 },
          ],
        },
      }),
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /show 2 criteria/i }));

    expect(onToggle).not.toHaveBeenCalled();
  });

  it("selects the stored positive and negative criterion choices", async () => {
    renderRow({
      item: makeItem({
        metadata: {
          judgment_criteria: [
            { dimension_key: "accuracy", value: 1 },
            { dimension_key: "completeness", value: -1 },
          ],
        },
      }),
    });

    await userEvent.setup().click(
      screen.getByRole("button", { name: /^trace actions$/i }),
    );
    expect(
      screen.getByRole("menuitem", { name: /correct info/i }),
    ).toHaveAttribute("data-active");
    expect(
      screen.getByRole("menuitem", { name: /^incomplete$/i }),
    ).toHaveAttribute("data-active");
  });

  it("renders dash when reviewer is null", () => {
    renderRow({
      reviewer: null,
      item: makeItem({
        metadata: {
          judgment_criteria: [{ dimension_key: "accuracy", value: 1 }],
        },
      }),
    });
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});

describe("DatasetItemRow expanded", () => {
  it("shows expanded preview headers", () => {
    renderRow({ isOpen: true });
    expect(screen.getByText("Input")).toBeInTheDocument();
    expect(screen.getByText("Expected output")).toBeInTheDocument();
  });

  it("aria-expanded reflects open state", () => {
    renderRow({ isOpen: true });
    expect(screen.getByRole("button", { expanded: true })).toBeInTheDocument();
  });

  it("keeps the expanded row unfilled while preserving the left accent", () => {
    renderRow({ isOpen: true });
    const row = screen.getByRole("button", { expanded: true });
    expect(row).not.toHaveClass("bg-primary/10");
    expect(row).toHaveClass("border-b-0");
    expect(row.querySelector("td")).toHaveClass(
      "shadow-[inset_3px_0_0_var(--color-primary)]",
    );
  });
});

describe("DatasetItemRow content rendering", () => {
  it("renders JSON via the pretty syntax-highlighted code block", () => {
    renderRow({
      isOpen: true,
      item: makeItem({
        input: { question: "hi" },
        expected_output: { answer: "hello" },
      }),
    });
    // JsonView wraps content in <pre><code>; the language hint shows in the class list.
    const container = screen.getByText("Input").closest("div")?.parentElement;
    expect(container).not.toBeNull();
    expect(within(container!).getAllByText(/question/i).length).toBeGreaterThan(0);
  });

});
