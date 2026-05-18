import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TimeRangeSelector } from "./TimeRangeSelector";
import { ACTIVITY_RANGES } from "./ranges";

afterEach(cleanup);

describe("TimeRangeSelector", () => {
  it("renders all range labels", () => {
    render(<TimeRangeSelector value="7d" onChange={() => {}} />);
    for (const { label } of ACTIVITY_RANGES) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it("active range has aria-pressed='true'; all others have aria-pressed='false'", () => {
    render(<TimeRangeSelector value="14d" onChange={() => {}} />);
    for (const { key, label } of ACTIVITY_RANGES) {
      const btn = screen.getByRole("button", { name: label });
      expect(btn).toHaveAttribute("aria-pressed", key === "14d" ? "true" : "false");
    }
  });

  it("clicking an inactive range calls onChange with its key", () => {
    const handleChange = vi.fn();
    render(<TimeRangeSelector value="7d" onChange={handleChange} />);

    // Click "30D" (inactive when active is "7d")
    const thirtyDBtn = screen.getByRole("button", { name: "30D" });
    fireEvent.click(thirtyDBtn);

    expect(handleChange).toHaveBeenCalledOnce();
    expect(handleChange).toHaveBeenCalledWith("30d");
  });

  it("clicking the active range still calls onChange with its key", () => {
    const handleChange = vi.fn();
    render(<TimeRangeSelector value="7d" onChange={handleChange} />);

    const sevenDBtn = screen.getByRole("button", { name: "7D" });
    fireEvent.click(sevenDBtn);

    expect(handleChange).toHaveBeenCalledOnce();
    expect(handleChange).toHaveBeenCalledWith("7d");
  });

  it("clicking 'All' range calls onChange with 'all'", () => {
    const handleChange = vi.fn();
    render(<TimeRangeSelector value="7d" onChange={handleChange} />);

    fireEvent.click(screen.getByRole("button", { name: "All" }));
    expect(handleChange).toHaveBeenCalledWith("all");
  });

  it("accepts custom ranges prop and renders those labels", () => {
    const customRanges = [
      { key: "1h", label: "1H" },
      { key: "6h", label: "6H" },
    ];
    render(<TimeRangeSelector value="1h" onChange={() => {}} ranges={customRanges} />);
    expect(screen.getByText("1H")).toBeInTheDocument();
    expect(screen.getByText("6H")).toBeInTheDocument();
    expect(screen.queryByText("7D")).not.toBeInTheDocument();
  });
});
