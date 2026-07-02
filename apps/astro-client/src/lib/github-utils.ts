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

// repoOwner returns the owner/org portion of a full repo name. e.g. "myorg/my-repo" → "myorg".
export function repoOwner(repoFullName: string): string {
  return repoFullName.split("/")[0] ?? "";
}

// repoPickerLabel returns the label for a repo entry in the connected-repos dropdown.
// When the repo's owner is the authenticated personal account (githubLogin), only the repo
// name is shown. For any other owner (e.g. an organization) the owner is prefixed as
// "owner/repo" so org-owned repos are distinguishable from personal ones at a glance.
export function repoPickerLabel(repoFullName: string, githubLogin?: string): string {
  const owner = repoOwner(repoFullName);
  const name = repoFullName.split("/")[1] ?? "";
  const isPersonal = !!githubLogin && owner.toLowerCase() === githubLogin.toLowerCase();
  return isPersonal || !owner ? name : `${owner}/${name}`;
}

// repoHref returns a GitHub URL for the repo, pointing to the subpath directory when present.
// A branch must be provided to navigate to the subpath tree; without it the link falls back
// to the repo root regardless of whether a subpath is configured.
export function repoHref(repoFullName: string, branch?: string): string {
  const base = repoBase(repoFullName);
  const sub = repoSubPath(repoFullName);
  return sub && branch
    ? `https://github.com/${base}/tree/${branch}/${sub}`
    : `https://github.com/${base}`;
}

// commitUrl returns the GitHub URL for a commit, or undefined when repo or sha is missing.
export function commitUrl(repoFullName?: string, sha?: string): string | undefined {
  return repoFullName && sha ? `https://github.com/${repoFullName}/commit/${sha}` : undefined;
}

// commitTitle returns the first non-empty line of a commit message, trimmed, or undefined.
export function commitTitle(message?: string): string | undefined {
  return message?.split("\n")[0].trim() || undefined;
}

// shortSha returns the abbreviated 7-char commit SHA, or undefined when absent.
export function shortSha(sha?: string): string | undefined {
  return sha?.slice(0, 7);
}
