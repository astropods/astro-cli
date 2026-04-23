import { describe, it, expect } from "vitest";
import { toRepoFullName, repoBase, repoSubPath, repoLabel, repoHref } from "./github-utils";
import type { GitHubRepo } from "./api";

function repo(fullName: string): GitHubRepo {
  return { full_name: fullName, default_branch: "main", private: false };
}

describe("repoBase", () => {
  it("returns owner/repo for a root connection", () => {
    expect(repoBase("owner/repo")).toBe("owner/repo");
  });

  it("strips subpath segments", () => {
    expect(repoBase("owner/repo/svc")).toBe("owner/repo");
    expect(repoBase("owner/repo/services/my-agent")).toBe("owner/repo");
  });
});

describe("repoSubPath", () => {
  it("returns empty string for a root connection", () => {
    expect(repoSubPath("owner/repo")).toBe("");
  });

  it("returns single-segment subpath", () => {
    expect(repoSubPath("owner/repo/svc")).toBe("svc");
  });

  it("returns multi-segment subpath", () => {
    expect(repoSubPath("owner/repo/services/my-agent")).toBe("services/my-agent");
  });
});

describe("repoLabel", () => {
  it("returns only the repo name for a root connection", () => {
    expect(repoLabel("owner/repo")).toBe("repo");
  });

  it("appends subpath to repo name", () => {
    expect(repoLabel("owner/repo/svc")).toBe("repo/svc");
    expect(repoLabel("owner/repo/services/my-agent")).toBe("repo/services/my-agent");
  });
});

describe("repoHref", () => {
  it("returns base GitHub URL for a root connection", () => {
    expect(repoHref("owner/repo")).toBe("https://github.com/owner/repo");
  });

  it("returns base GitHub URL for subpath without branch", () => {
    expect(repoHref("owner/repo/svc")).toBe("https://github.com/owner/repo");
  });

  it("returns tree URL for subpath with branch", () => {
    expect(repoHref("owner/repo/svc", "main")).toBe(
      "https://github.com/owner/repo/tree/main/svc"
    );
    expect(repoHref("owner/repo/services/my-agent", "feat")).toBe(
      "https://github.com/owner/repo/tree/feat/services/my-agent"
    );
  });

  it("returns base URL for root connection even when branch is provided", () => {
    expect(repoHref("owner/repo", "main")).toBe("https://github.com/owner/repo");
  });
});

describe("toRepoFullName", () => {
  it("returns repo full_name when subpath is empty", () => {
    expect(toRepoFullName(repo("owner/repo"), "")).toBe("owner/repo");
  });

  it("appends subpath to full_name", () => {
    expect(toRepoFullName(repo("owner/repo"), "svc")).toBe("owner/repo/svc");
    expect(toRepoFullName(repo("owner/repo"), "services/my-agent")).toBe(
      "owner/repo/services/my-agent"
    );
  });

  it("strips leading and trailing slashes from subpath", () => {
    expect(toRepoFullName(repo("owner/repo"), "/svc/")).toBe("owner/repo/svc");
    expect(toRepoFullName(repo("owner/repo"), "///svc///")).toBe("owner/repo/svc");
  });

  it("treats whitespace-only subpath as empty", () => {
    expect(toRepoFullName(repo("owner/repo"), "   ")).toBe("owner/repo");
  });
});
