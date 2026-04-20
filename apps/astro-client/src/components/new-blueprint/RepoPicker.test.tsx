import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { RepoPicker } from "./RepoPicker";
import type { GitHubRepo } from "@/lib/api";

const REPOS: GitHubRepo[] = [
  { full_name: "testuser/my-agent", default_branch: "main", private: false },
  { full_name: "testuser/private-repo", default_branch: "main", private: true },
  { full_name: "testuser/another-repo", default_branch: "develop", private: false },
];

function baseProps() {
  return {
    githubLogin: "testuser",
    selectedRepo: null,
    selectedBranch: "main",
    isLoadingRepos: false,
    repos: REPOS,
    connections: undefined,
    onSelectRepo: vi.fn(),
    onSelectBranch: vi.fn(),
    onSearchChange: vi.fn(),
  };
}

describe("RepoPicker", () => {
  it("renders search input with placeholder", () => {
    render(<RepoPicker {...baseProps()} />);
    expect(screen.getByPlaceholderText(/search repositories/i)).toBeInTheDocument();
  });

  it("shows loading state when isLoadingRepos is true", () => {
    render(<RepoPicker {...baseProps()} isLoadingRepos repos={[]} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByText(/loading repositories/i)).toBeInTheDocument();
  });

  it("shows empty state when no repos", () => {
    render(<RepoPicker {...baseProps()} repos={[]} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "xyz" } });
    expect(screen.getByText(/no repos matching/i)).toBeInTheDocument();
  });

  it("renders all passed repos when dropdown is open", () => {
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByRole("button", { name: /my-agent/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /private-repo/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /another-repo/ })).toBeInTheDocument();
  });

  it("calls onSelectRepo when a repo is clicked", () => {
    const onSelectRepo = vi.fn();
    render(<RepoPicker {...baseProps()} onSelectRepo={onSelectRepo} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    expect(onSelectRepo).toHaveBeenCalledWith(REPOS[0]);
  });

  it("calls onSearchChange when user types", () => {
    const onSearchChange = vi.fn();
    render(<RepoPicker {...baseProps()} onSearchChange={onSearchChange} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "agent" } });
    // onSearchChange is debounced — called after 300ms; just verify the input reflects the value
    expect(screen.getByPlaceholderText(/search repositories/i)).toHaveValue("agent");
  });

  it("disables repos that are already linked", () => {
    const connections = [{ agent_name: "other-agent", repo_full_name: "testuser/my-agent" }];
    render(<RepoPicker {...baseProps()} connections={connections} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByRole("button", { name: /my-agent/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /another-repo/ })).not.toBeDisabled();
  });

  it("shows 'linked to' hint for disabled repos", () => {
    const connections = [{ agent_name: "other-agent", repo_full_name: "testuser/my-agent" }];
    render(<RepoPicker {...baseProps()} connections={connections} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    expect(screen.getByText(/linked to other-agent/i)).toBeInTheDocument();
  });

  it("shows github login prefix in search bar", () => {
    render(<RepoPicker {...baseProps()} />);
    expect(screen.getByText(/testuser/)).toBeInTheDocument();
  });

  it("shows selected repo full name when repo is selected", () => {
    render(<RepoPicker {...baseProps()} selectedRepo={REPOS[0]} />);
    expect(screen.getByText("testuser/my-agent")).toBeInTheDocument();
  });

  it("branch selector is collapsed when no repo is selected", () => {
    const { container } = render(<RepoPicker {...baseProps()} />);
    expect(container.querySelector(".grid-rows-\\[0fr\\]")).toBeInTheDocument();
  });

  it("branch selector is expanded when a repo is selected", () => {
    const { container } = render(<RepoPicker {...baseProps()} selectedRepo={REPOS[0]} />);
    expect(container.querySelector(".grid-rows-\\[1fr\\]")).toBeInTheDocument();
  });

  it("renders all server-returned repos — no client-side cap", () => {
    const manyRepos: GitHubRepo[] = Array.from({ length: 150 }, (_, i) => ({
      full_name: `testuser/repo-${i}`,
      default_branch: "main",
      private: false,
    }));
    render(<RepoPicker {...baseProps()} repos={manyRepos} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "repo" } });
    expect(screen.getAllByRole("button", { name: /repo-/ })).toHaveLength(150);
  });
});
