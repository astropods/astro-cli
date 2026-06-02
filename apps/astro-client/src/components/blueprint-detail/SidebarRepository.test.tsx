import { describe, it, expect } from "vitest";
import { buildRepoDirectoryUrl } from "./SidebarRepository";

describe("buildRepoDirectoryUrl", () => {
  it("returns the original url when no directory is given", () => {
    expect(buildRepoDirectoryUrl("https://github.com/acme/agent")).toBe(
      "https://github.com/acme/agent",
    );
  });

  it("returns the original url when directory is an empty string", () => {
    expect(buildRepoDirectoryUrl("https://github.com/acme/agent", "")).toBe(
      "https://github.com/acme/agent",
    );
  });

  it("appends /tree/HEAD/<dir> for github.com", () => {
    expect(
      buildRepoDirectoryUrl("https://github.com/acme/agent", "packages/foo"),
    ).toBe("https://github.com/acme/agent/tree/HEAD/packages/foo");
  });

  it("appends /tree/HEAD/<dir> for gist.github.com", () => {
    expect(
      buildRepoDirectoryUrl("https://gist.github.com/acme/abc123", "src"),
    ).toBe("https://gist.github.com/acme/abc123/tree/HEAD/src");
  });

  it("uses gitlab's /-/tree/HEAD/<dir> path", () => {
    expect(
      buildRepoDirectoryUrl("https://gitlab.com/acme/agent", "packages/foo"),
    ).toBe("https://gitlab.com/acme/agent/-/tree/HEAD/packages/foo");
  });

  it("uses bitbucket's /src/HEAD/<dir> path", () => {
    expect(
      buildRepoDirectoryUrl("https://bitbucket.org/acme/agent", "packages/foo"),
    ).toBe("https://bitbucket.org/acme/agent/src/HEAD/packages/foo");
  });

  it("strips a trailing .git from the repo url", () => {
    expect(
      buildRepoDirectoryUrl("https://github.com/acme/agent.git", "src"),
    ).toBe("https://github.com/acme/agent/tree/HEAD/src");
  });

  it("strips a trailing slash from the repo url", () => {
    expect(
      buildRepoDirectoryUrl("https://github.com/acme/agent/", "src"),
    ).toBe("https://github.com/acme/agent/tree/HEAD/src");
  });

  it("strips a trailing slash that follows .git on the repo url", () => {
    expect(
      buildRepoDirectoryUrl("https://github.com/acme/agent.git/", "src"),
    ).toBe("https://github.com/acme/agent/tree/HEAD/src");
  });

  it("strips leading and trailing slashes from the directory", () => {
    expect(
      buildRepoDirectoryUrl("https://github.com/acme/agent", "/packages/foo/"),
    ).toBe("https://github.com/acme/agent/tree/HEAD/packages/foo");
  });

  it("normalizes a www. prefix on the hostname", () => {
    expect(
      buildRepoDirectoryUrl("https://www.github.com/acme/agent", "src"),
    ).toBe("https://www.github.com/acme/agent/tree/HEAD/src");
  });

  it("falls back to the repo url for unknown providers", () => {
    expect(
      buildRepoDirectoryUrl("https://git.example.com/acme/agent", "src"),
    ).toBe("https://git.example.com/acme/agent");
  });

  it("falls back to the input when the url is not parseable", () => {
    expect(buildRepoDirectoryUrl("not-a-url", "src")).toBe("not-a-url");
  });
});
