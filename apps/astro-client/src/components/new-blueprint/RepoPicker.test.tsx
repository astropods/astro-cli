import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { RepoPicker } from "./RepoPicker";
import type { GitHubRepo } from "@/lib/api";

vi.mock("@/api/queries/github", () => ({
  useGitHubAccountRepos: vi.fn(),
  useGitHubAccountConnections: vi.fn(),
}));

import { useGitHubAccountRepos, useGitHubAccountConnections } from "@/api/queries/github";

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

  it("subpath input appears after repo selection", () => {
    render(<RepoPicker {...baseProps()} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    expect(screen.getByText("Path")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("services/my-agent")).toBeInTheDocument();
  });

  it("typing a subpath includes it in onChange repoFullName", () => {
    const onChange = vi.fn();
    render(<RepoPicker {...baseProps()} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    onChange.mockClear();
    fireEvent.change(screen.getByPlaceholderText("services/my-agent"), { target: { value: "svc/worker" } });
    expect(onChange).toHaveBeenCalledWith({ repoFullName: "testuser/my-agent/svc/worker", branch: "main" });
  });

  it("clearing the subpath restores bare repo name in onChange", () => {
    const onChange = vi.fn();
    render(<RepoPicker {...baseProps()} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), { target: { value: "a" } });
    fireEvent.click(screen.getByRole("button", { name: /my-agent/ }));
    fireEvent.change(screen.getByPlaceholderText("services/my-agent"), { target: { value: "svc/worker" } });
    onChange.mockClear();
    fireEvent.change(screen.getByPlaceholderText("services/my-agent"), { target: { value: "" } });
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
