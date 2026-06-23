import { render, screen, cleanup, fireEvent, within } from "@testing-library/react";
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
      verdict: 1,
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
  rawMode?: "pretty" | "raw";
  reviewer?: ResolvedReviewer | null;
}

function renderRow(opts: RenderOpts = {}) {
  const onToggle = vi.fn();
  render(
    <DatasetItemRow
      item={opts.item ?? makeItem()}
      isOpen={opts.isOpen ?? false}
      onToggle={onToggle}
      reviewer={opts.reviewer === undefined ? reviewer : opts.reviewer}
      rawMode={opts.rawMode ?? "pretty"}
    />,
  );
  return { onToggle };
}

describe("DatasetItemRow collapsed", () => {
  it("renders verdict pill and reviewer", () => {
    renderRow();
    expect(screen.getByText("Good")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("does not render expanded preview when isOpen is false", () => {
    renderRow({ isOpen: false });
    expect(screen.queryByText(/^Input$/)).not.toBeInTheDocument();
    expect(screen.queryByText(/^Expected output$/)).not.toBeInTheDocument();
  });

  it("clicking the row toggles via onToggle with its id", () => {
    const { onToggle } = renderRow({ item: makeItem({ id: "abc" }) });
    fireEvent.click(screen.getByRole("button", { expanded: false }));
    expect(onToggle).toHaveBeenCalledWith("abc");
  });

  it("renders dash when reviewer is null", () => {
    renderRow({ reviewer: null });
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});

describe("DatasetItemRow expanded", () => {
  it("shows expanded preview headers", () => {
    renderRow({ isOpen: true });
    expect(screen.getByText("Input")).toBeInTheDocument();
    expect(screen.getByText("Expected output")).toBeInTheDocument();
  });

  it("shows the Good example label with verdict = good", () => {
    renderRow({ isOpen: true });
    expect(screen.getByText("Good example")).toBeInTheDocument();
  });

  it("shows the Bad example label with verdict = bad", () => {
    renderRow({
      isOpen: true,
      item: makeItem({ metadata: { verdict: -1 } }),
    });
    expect(screen.getByText("Bad example")).toBeInTheDocument();
  });

  it("aria-expanded reflects open state", () => {
    renderRow({ isOpen: true });
    expect(screen.getByRole("button", { expanded: true })).toBeInTheDocument();
  });
});

describe("DatasetItemRow content rendering", () => {
  it("Pretty mode renders JSON via syntax-highlighted code block", () => {
    renderRow({
      isOpen: true,
      rawMode: "pretty",
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

  it("Raw mode renders JSON content in a plain <pre> block", () => {
    renderRow({
      isOpen: true,
      rawMode: "raw",
      item: makeItem({
        input: { question: "hi" },
        expected_output: { answer: "hello" },
      }),
    });
    const preEls = document.querySelectorAll("pre");
    expect(preEls.length).toBeGreaterThan(0);
    const allText = Array.from(preEls).map((p) => p.textContent ?? "").join(" ");
    expect(allText).toContain('"question"');
    expect(allText).toContain('"answer"');
  });
});
