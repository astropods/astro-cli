import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SubpathPicker } from "./SubpathPicker";

describe("SubpathPicker", () => {
  it("renders label and input", () => {
    render(<SubpathPicker value="" onChange={vi.fn()} />);
    expect(screen.getByText("Subdirectory")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("e.g. services/my-agent")).toBeInTheDocument();
  });

  it("calls onChange when user types", () => {
    const onChange = vi.fn();
    render(<SubpathPicker value="" onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText("e.g. services/my-agent"), { target: { value: "svc/agent" } });
    expect(onChange).toHaveBeenCalledWith("svc/agent");
  });

  it("shows clear button when value is set", () => {
    render(<SubpathPicker value="svc" onChange={vi.fn()} />);
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  it("calls onChange with empty string when clear button is clicked", () => {
    const onChange = vi.fn();
    render(<SubpathPicker value="svc" onChange={onChange} />);
    fireEvent.click(screen.getByRole("button"));
    expect(onChange).toHaveBeenCalledWith("");
  });

  it("hides clear button when value is empty", () => {
    render(<SubpathPicker value="" onChange={vi.fn()} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
