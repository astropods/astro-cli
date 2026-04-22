import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SubpathPicker } from "./SubpathPicker";

afterEach(cleanup);

vi.mock("@/api/queries/github", () => ({
  useGitHubAccountDirs: vi.fn(),
}));

import { useGitHubAccountDirs } from "@/api/queries/github";
const mockUseDirs = vi.mocked(useGitHubAccountDirs);

function makeDirsResult(dirs: string[], isLoading = false) {
  return { data: { dirs }, isLoading } as ReturnType<typeof useGitHubAccountDirs>;
}

function renderPicker(value = "", onChange = vi.fn()) {
  return render(
    <SubpathPicker
      account="myorg"
      repo="owner/repo"
      branch="main"
      value={value}
      onChange={onChange}
    />
  );
}

describe("SubpathPicker", () => {
  beforeEach(() => {
    mockUseDirs.mockReturnValue(makeDirsResult(["svc", "svc/agent", "infra"]));
  });

  it("renders label and input", () => {
    renderPicker();
    expect(screen.getByText("Subdirectory")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("e.g. services/my-agent")).toBeInTheDocument();
  });

  it("shows dirs dropdown when input is focused", () => {
    renderPicker();
    fireEvent.focus(screen.getByPlaceholderText("e.g. services/my-agent"));
    expect(screen.getByText("svc")).toBeInTheDocument();
    expect(screen.getByText("svc/agent")).toBeInTheDocument();
    expect(screen.getByText("infra")).toBeInTheDocument();
  });

  it("filters dirs based on input value", () => {
    renderPicker("svc");
    fireEvent.focus(screen.getByPlaceholderText("e.g. services/my-agent"));
    expect(screen.getByText("svc")).toBeInTheDocument();
    expect(screen.getByText("svc/agent")).toBeInTheDocument();
    expect(screen.queryByText("infra")).not.toBeInTheDocument();
  });

  it("calls onChange when a dir is selected", () => {
    const onChange = vi.fn();
    renderPicker("", onChange);
    fireEvent.focus(screen.getByPlaceholderText("e.g. services/my-agent"));
    fireEvent.click(screen.getByText("svc/agent"));
    expect(onChange).toHaveBeenCalledWith("svc/agent");
  });

  it("calls onChange with empty string when clear button is clicked", () => {
    const onChange = vi.fn();
    renderPicker("svc", onChange);
    fireEvent.click(screen.getByRole("button", { name: "" }));
    expect(onChange).toHaveBeenCalledWith("");
  });

  it("shows loading state while dirs are fetching", () => {
    mockUseDirs.mockReturnValue(makeDirsResult([], true));
    renderPicker();
    fireEvent.focus(screen.getByPlaceholderText("e.g. services/my-agent"));
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("passes enabled=false to hook when disabled", () => {
    render(
      <SubpathPicker
        account="myorg"
        repo=""
        branch="main"
        value=""
        onChange={vi.fn()}
        enabled={false}
      />
    );
    expect(mockUseDirs).toHaveBeenCalledWith("myorg", "", "main", { enabled: false });
  });
});
