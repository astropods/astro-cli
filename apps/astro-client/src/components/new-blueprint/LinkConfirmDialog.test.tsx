import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { LinkConfirmDialog } from "./LinkConfirmDialog";

function baseProps() {
  return {
    open: true,
    onOpenChange: vi.fn(),
    avatarPreviewUrl: null,
    slug: "my-agent",
    name: "My Agent",
    selectedOrg: "testuser",
    repoBase: "testuser/my-agent",
    selectedBranch: "main",
    visibility: "public" as const,
    isCreatingBlueprint: false,
    onConfirm: vi.fn(),
  };
}

describe("LinkConfirmDialog", () => {
  it("renders blueprint name and repo in description", () => {
    render(<LinkConfirmDialog {...baseProps()} />);
    expect(screen.getByText("My Agent")).toBeInTheDocument();
    expect(screen.getByText("testuser/my-agent")).toBeInTheDocument();
  });

  it("shows organization in details table", () => {
    render(<LinkConfirmDialog {...baseProps()} />);
    expect(screen.getAllByText("testuser").length).toBeGreaterThanOrEqual(1);
  });

  it("shows branch in details table", () => {
    render(<LinkConfirmDialog {...baseProps()} />);
    expect(screen.getAllByText("main").length).toBeGreaterThanOrEqual(1);
  });

  it("shows Public visibility", () => {
    render(<LinkConfirmDialog {...baseProps()} visibility="public" />);
    expect(screen.getAllByText("Public").length).toBeGreaterThanOrEqual(1);
  });

  it("shows Private visibility", () => {
    render(<LinkConfirmDialog {...baseProps()} visibility="private" />);
    expect(screen.getByText("Private")).toBeInTheDocument();
  });

  it("calls onConfirm when Create blueprint button is clicked", () => {
    const onConfirm = vi.fn();
    render(<LinkConfirmDialog {...baseProps()} onConfirm={onConfirm} />);
    fireEvent.click(screen.getByRole("button", { name: /create blueprint/i }));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("disables Create blueprint button when isCreatingBlueprint is true", () => {
    render(<LinkConfirmDialog {...baseProps()} isCreatingBlueprint />);
    expect(screen.getByRole("button", { name: /create blueprint/i })).toBeDisabled();
  });

  it("renders Back button", () => {
    render(<LinkConfirmDialog {...baseProps()} />);
    expect(screen.getByRole("button", { name: /back/i })).toBeInTheDocument();
  });

  it("uses slug as fallback when name is empty", () => {
    render(<LinkConfirmDialog {...baseProps()} name="" slug="my-agent" />);
    expect(screen.getByText("my-agent")).toBeInTheDocument();
  });

  it("renders avatar image when avatarPreviewUrl is provided", () => {
    render(<LinkConfirmDialog {...baseProps()} avatarPreviewUrl="https://example.com/avatar.png" />);
    const img = screen.getByRole("img", { name: /my-agent/i });
    expect(img).toBeInTheDocument();
    expect(img).toHaveAttribute("src", "https://example.com/avatar.png");
  });

  it("is not visible when open is false", () => {
    render(<LinkConfirmDialog {...baseProps()} open={false} />);
    // Radix Dialog keeps the node in the DOM but marks it hidden
    const btn = screen.queryByRole("button", { name: /create blueprint/i });
    expect(btn == null || !btn.checkVisibility()).toBe(true);
  });
});
