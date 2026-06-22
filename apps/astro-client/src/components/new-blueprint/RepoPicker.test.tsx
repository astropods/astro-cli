import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { RepoPicker } from "./RepoPicker";
import type { GitHubRepo } from "@/lib/api";

vi.mock("@/api/queries/github", () => ({
  useGitHubAccountRepos: vi.fn(),
  useGitHubAccountConnections: vi.fn(),
  useGitHubAccountBranches: vi.fn(),
}));

import { useGitHubAccountRepos, useGitHubAccountConnections, useGitHubAccountBranches } from "@/api/queries/github";

const REPOS: GitHubRepo[] = [
  { full_name: "testuser/my-agent", default_branch: "main", private: false },
  { full_name: "testuser/private-repo", default_branch: "main", private: true },
  { full_name: "testuser/another-repo", default_branch: "develop", private: false },
];

function baseProps() {
  return {
    account: "testuser",
    githubLogin: "testuser",
    enabled: true,
    onChange: vi.fn(),
  };
}

beforeEach(() => {
  vi.mocked(useGitHubAccountRepos).mockReturnValue({ data: { repos: REPOS }, isLoading: false } as any);
  vi.mocked(useGitHubAccountConnections).mockReturnValue({ data: { connections: [] } } as any);
  vi.mocked(useGitHubAccountBranches).mockReturnValue({ data: { branches: [] }, isLoading: false } as unknown as ReturnType<typeof useGitHubAccountBranches>);
});

describe("RepoPicker", () => {
  it("renders search input with placeholder", () => {
    render(<RepoPicker {...baseProps()} />);
    expect(screen.getByPlaceholderText(/search repositories/i)).toBeInTheDocument();
  });

  it("shows loading state when repos are loading", () => {
    vi.mocked(useGitHubAccountRepos).mockReturnValue({ data: undefined, isLoading: true } as any);
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByText(/loading repositories/i)).toBeInTheDocument();
  });

  it("shows empty state when no repos match", () => {
    vi.mocked(useGitHubAccountRepos).mockReturnValue({ data: { repos: [] }, isLoading: false } as any);
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "xyz" } });
    expect(screen.getByText(/no repos matching/i)).toBeInTheDocument();
  });

  it("opens the dropdown when the control body is clicked, not just the chevron", () => {
    render(<RepoPicker {...baseProps()} />);
    // Collapsed initially: the chevron offers to open the list. (Repo buttons stay
    // mounted but are hidden via the CSS grid collapse, so assert on open state.)
    expect(screen.getByRole("button", { name: /browse repositories/i })).toBeInTheDocument();
    // Click the control body (the search input area), away from the chevron.
    fireEvent.click(screen.getByPlaceholderText(/search repositories/i));
    // Now open: the chevron toggles to "close".
    expect(screen.getByRole("button", { name: /close repository list/i })).toBeInTheDocument();
  });

  it("renders all repos when dropdown is open", () => {
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByRole("button", { name: /my-agent/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /private-repo/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /another-repo/ })).toBeInTheDocument();
  });

  it("calls onChange when a repo is clicked", () => {
    const onChange = vi.fn();
    render(<RepoPicker {...baseProps()} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    expect(onChange).toHaveBeenCalledWith({ repoFullName: "testuser/my-agent", branch: "main" });
  });

  it("shows the selected repo name after selection", () => {
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    expect(screen.getByText("testuser/my-agent")).toBeInTheDocument();
  });

  it("disables repos that are already linked", () => {
    vi.mocked(useGitHubAccountConnections).mockReturnValue({
      data: { connections: [{ agent_name: "other-agent", repo_full_name: "testuser/my-agent" }] },
    } as any);
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByRole("button", { name: /my-agent/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /another-repo/ })).not.toBeDisabled();
  });

  it("shows 'linked to' hint for disabled repos", () => {
    vi.mocked(useGitHubAccountConnections).mockReturnValue({
      data: { connections: [{ agent_name: "other-agent", repo_full_name: "testuser/my-agent" }] },
    } as any);
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByText(/linked to other-agent/i)).toBeInTheDocument();
  });

  it("shows github login prefix in search bar", () => {
    render(<RepoPicker {...baseProps()} />);
    expect(screen.getByText(/testuser/)).toBeInTheDocument();
  });

  it("branch selector is collapsed before repo selection", () => {
    const { container } = render(<RepoPicker {...baseProps()} />);
    expect(container.querySelector(".grid-rows-\\[0fr\\]")).toBeInTheDocument();
  });

  it("branch selector appears after repo selection", () => {
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    expect(screen.getByText("Branch")).toBeInTheDocument();
  });

  it("lists the repo's real branches from the API, default branch first", () => {
    vi.mocked(useGitHubAccountBranches).mockReturnValue({
      data: { branches: ["dev", "main", "release/1.0"] },
      isLoading: false,
    } as unknown as ReturnType<typeof useGitHubAccountBranches>);
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    const branchOptions = screen.getAllByTestId("branch-option").map(b => b.textContent?.trim());
    // Real branches from the API are offered, default branch (main) first, and the
    // old hardcoded "master" is gone.
    expect(branchOptions).toEqual(["main", "dev", "release/1.0"]);
  });

  it("subpath input appears after repo selection", () => {
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    expect(screen.getByText("Path")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("e.g. path/to/my-agent")).toBeInTheDocument();
  });

  it("typing a subpath includes it in onChange repoFullName", () => {
    const onChange = vi.fn();
    render(<RepoPicker {...baseProps()} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    onChange.mockClear();
    fireEvent.change(screen.getByPlaceholderText("e.g. path/to/my-agent"), { target: { value: "svc/worker" } });
    expect(onChange).toHaveBeenCalledWith({ repoFullName: "testuser/my-agent/svc/worker", branch: "main" });
  });

  it("clearing the subpath restores bare repo name in onChange", () => {
    const onChange = vi.fn();
    render(<RepoPicker {...baseProps()} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    fireEvent.change(screen.getByPlaceholderText("e.g. path/to/my-agent"), { target: { value: "svc/worker" } });
    onChange.mockClear();
    fireEvent.change(screen.getByPlaceholderText("e.g. path/to/my-agent"), { target: { value: "" } });
    expect(onChange).toHaveBeenCalledWith({ repoFullName: "testuser/my-agent", branch: "main" });
  });

  it("renders all server-returned repos — no client-side cap", () => {
    const manyRepos: GitHubRepo[] = Array.from({ length: 150 }, (_, i) => ({
      full_name: `testuser/repo-${i}`,
      default_branch: "main",
      private: false,
    }));
    vi.mocked(useGitHubAccountRepos).mockReturnValue({ data: { repos: manyRepos }, isLoading: false } as any);
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "repo" } });
    expect(screen.getAllByRole("button", { name: /repo-/ })).toHaveLength(150);
  });
});
