import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DatasetFilterChips, type FilterKey } from "./DatasetFilterChips";

afterEach(cleanup);

function renderChips(selected: FilterKey[] = []) {
  const onToggle = vi.fn();
  render(
    <DatasetFilterChips
      selected={new Set(selected)}
      counts={{ good: 12, bad: 3 }}
      onToggle={onToggle}
    />,
  );
  return { onToggle };
}

describe("DatasetFilterChips", () => {
  it("renders Good and Bad with counts from props", () => {
    renderChips();
    expect(screen.getByRole("button", { name: /good/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /bad/i })).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("clicking a chip calls onToggle with its key", () => {
    const { onToggle } = renderChips();
    fireEvent.click(screen.getByRole("button", { name: /good/i }));
    expect(onToggle).toHaveBeenCalledWith("good");
    fireEvent.click(screen.getByRole("button", { name: /bad/i }));
    expect(onToggle).toHaveBeenCalledWith("bad");
  });

  it("active chip carries data-active and inactive does not", () => {
    renderChips(["good"]);
    expect(screen.getByRole("button", { name: /good/i })).toHaveAttribute("data-active");
    expect(screen.getByRole("button", { name: /bad/i })).not.toHaveAttribute("data-active");
  });

  it("renders the clear (X) icon only on active chips", () => {
    const { container } = render(
      <DatasetFilterChips
        selected={new Set<FilterKey>(["good"])}
        counts={{ good: 1, bad: 0 }}
        onToggle={() => {}}
      />,
    );
    // lucide-react renders icons with this class hint
    const goodBtn = screen.getByRole("button", { name: /good/i });
    const badBtn = screen.getByRole("button", { name: /bad/i });
    // The X svg sits inside the active button only.
    expect(goodBtn.querySelector(".lucide-x")).not.toBeNull();
    expect(badBtn.querySelector(".lucide-x")).toBeNull();
    // Silence unused warning
    void container;
  });
});
