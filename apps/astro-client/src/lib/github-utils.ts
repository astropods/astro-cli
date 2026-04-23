import type { GitHubRepo } from "@/lib/api";

// toRepoFullName combines a repo and optional subpath into the owner/repo[/subpath]
// encoding used by the backend. Leading/trailing slashes are stripped from the subpath.
export function toRepoFullName(repo: GitHubRepo, subpath: string): string {
  const cleaned = subpath.trim().replace(/^\/+|\/+$/g, "");
  return cleaned ? `${repo.full_name}/${cleaned}` : repo.full_name;
}

// repoBase returns the owner/repo portion of a full repo name.
export function repoBase(repoFullName: string): string {
  return repoFullName.split("/").slice(0, 2).join("/");
}

// repoSubPath returns the subpath portion of a full repo name, or "" for root connections.
export function repoSubPath(repoFullName: string): string {
  return repoFullName.split("/").slice(2).join("/");
}

// repoLabel returns a display label for the repo — repo name only, with subpath appended when present.
// e.g. "myorg/my-repo/svc/agent" → "my-repo/svc/agent", "myorg/my-repo" → "my-repo"
export function repoLabel(repoFullName: string): string {
  const sub = repoSubPath(repoFullName);
  const name = repoFullName.split("/")[1] ?? "";
  return sub ? `${name}/${sub}` : name;
}

// repoHref returns a GitHub URL for the repo, pointing to the subpath directory when present.
export function repoHref(repoFullName: string, branch?: string): string {
  const base = repoBase(repoFullName);
  const sub = repoSubPath(repoFullName);
  return sub && branch
    ? `https://github.com/${base}/tree/${branch}/${sub}`
    : `https://github.com/${base}`;
}
