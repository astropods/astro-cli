import { useState, useRef, useEffect, useCallback } from "react";
import { toRepoFullName, repoPickerLabel } from "@/lib/github-utils";
import { ArrowPathIcon, XMarkIcon } from "@heroicons/react/24/outline";
import { Check, ChevronDown, Search, GitBranch, FolderOpen } from "lucide-react";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { cn } from "@/lib/utils";
import { inputBase, inputFocusWithin } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tag } from "@/components/Tag";
import { useGitHubAccountRepos, useGitHubAccountConnections, useGitHubAccountBranches } from "@/api/queries/github";
import type { GitHubRepo } from "@/lib/api";

export type RepoPickerValue = {
  repoFullName: string | null;
  branch: string;
};

type Props = {
  account: string;
  githubLogin?: string;
  enabled?: boolean;
  onChange: (value: RepoPickerValue) => void;
};

export function RepoPicker({ account, githubLogin, enabled = true, onChange }: Props) {
  const [query, setQuery] = useState("");
  const [apiQuery, setApiQuery] = useState("");
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [selectedBranch, setSelectedBranch] = useState("main");
  const [subpath, setSubpath] = useState("");
  const [repoOpen, setRepoOpen] = useState(false);
  const [branchOpen, setBranchOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const { data: reposData, isLoading: isLoadingRepos } = useGitHubAccountRepos(account, {
    enabled,
    q: apiQuery,
    login: githubLogin,
  });
  const { data: connectionsData } = useGitHubAccountConnections(account, { enabled });

  const repos = reposData?.repos ?? [];
  const connections = connectionsData?.connections;

  // Real branches for the selected repo. While loading (or if the fetch fails),
  // fall back to the repo's default branch so the selector always has a value.
  const { data: branchesData, isLoading: isLoadingBranches } = useGitHubAccountBranches(
    account,
    selectedRepo?.full_name ?? "",
    { enabled: enabled && !!selectedRepo },
  );

  const defaultBranch = selectedRepo?.default_branch ?? "main";
  const fetchedBranches = branchesData?.branches ?? [];
  // Surface the default branch first, then the rest (deduped).
  const branches = fetchedBranches.length
    ? [defaultBranch, ...fetchedBranches.filter(b => b !== defaultBranch)]
    : [defaultBranch];

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setRepoOpen(false);
        setBranchOpen(false);
      }
    }
    document.addEventListener("mousedown", onMouseDown);
    return () => document.removeEventListener("mousedown", onMouseDown);
  }, []);

  function handleQueryChange(value: string) {
    setQuery(value);
    setRepoOpen(true);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => setApiQuery(value), 120);
  }

  const handleSelectRepo = useCallback((repo: GitHubRepo) => {
    setSelectedRepo(repo);
    const branch = repo.default_branch ?? "main";
    setSelectedBranch(branch);
    setSubpath("");
    setQuery("");
    setRepoOpen(false);
    onChange({ repoFullName: repo.full_name, branch });
  }, [onChange]);

  const handleClear = useCallback(() => {
    setSelectedRepo(null);
    setSelectedBranch("main");
    setSubpath("");
    setQuery("");
    setApiQuery("");
    if (debounceRef.current) clearTimeout(debounceRef.current);
    onChange({ repoFullName: null, branch: "main" });
    setTimeout(() => inputRef.current?.focus(), 0);
  }, [onChange]);

  const handleCaretClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    const next = !repoOpen;
    setRepoOpen(next);
    if (next) setTimeout(() => inputRef.current?.focus(), 0);
  }, [repoOpen]);

  const handleSelectBranch = useCallback((branch: string) => {
    setSelectedBranch(branch);
    setBranchOpen(false);
    onChange({ repoFullName: selectedRepo ? toRepoFullName(selectedRepo, subpath) : null, branch });
  }, [onChange, selectedRepo, subpath]);

  const handleSubpathChange = useCallback((newSubpath: string) => {
    setSubpath(newSubpath);
    if (selectedRepo) {
      onChange({ repoFullName: toRepoFullName(selectedRepo, newSubpath), branch: selectedBranch });
    }
  }, [onChange, selectedRepo, selectedBranch]);

  return (
    <div ref={containerRef} className="px-4 py-3 space-y-3">

      {/* Repo autocomplete */}
      <div>
        <Label size="md" className="flex items-center gap-1.5 mb-1.5">
          <span className="size-3.5">{getIntegrationIcon("github")}</span>
          Repository
        </Label>
        <div
          className={cn(inputBase, inputFocusWithin, "flex h-9 items-center gap-2 cursor-text px-3")}
          onClick={() => {
            if (selectedRepo) return;
            setRepoOpen(true);
            inputRef.current?.focus();
          }}
        >
          {selectedRepo && !repoOpen ? (
            <>
              <Check className="size-3.5 text-success shrink-0" />
              <span className="flex-1 text-sm font-medium truncate">
                {selectedRepo.full_name}
              </span>
              <button
                type="button"
                onClick={handleClear}
                className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                aria-label="Clear repository"
              >
                <XMarkIcon className="size-3.5" />
              </button>
            </>
          ) : (
            <>
              <Search className="size-3.5 text-muted-foreground shrink-0" />
              {githubLogin && (
                <span className="font-mono text-xs text-muted-foreground shrink-0">{githubLogin} /</span>
              )}
              <input
                ref={inputRef}
                type="text"
                autoFocus={!selectedRepo}
                className="flex-1 min-w-0 bg-transparent border-none outline-none text-sm placeholder:text-muted-foreground"
                placeholder="Search repositories..."
                value={query}
                onChange={e => handleQueryChange(e.target.value)}
              />
              {query ? (
                <button
                  type="button"
                  onClick={() => { setQuery(""); setApiQuery(""); inputRef.current?.focus(); }}
                  className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                  aria-label="Clear search"
                >
                  <XMarkIcon className="size-3.5" />
                </button>
              ) : (
                <button
                  type="button"
                  onClick={handleCaretClick}
                  className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                  aria-label={repoOpen ? "Close repository list" : "Browse repositories"}
                >
                  <ChevronDown className={cn(
                    "size-3.5 transition-transform duration-200",
                    repoOpen && "rotate-180",
                  )} />
                </button>
              )}
            </>
          )}
        </div>

        {/* Repo dropdown */}
        <div className={cn(
          "grid transition-[grid-template-rows] duration-150 ease-out",
          repoOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}>
          <div className="overflow-hidden">
            <div className="mt-1.5 h-48 overflow-y-auto rounded-sm border border-border bg-background">
              {isLoadingRepos ? (
                <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
                  <ArrowPathIcon className="size-4 animate-spin" />
                  Loading repositories...
                </div>
              ) : repos.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  {query ? <>No repos matching &ldquo;{query}&rdquo;</> : "No repositories found"}
                </div>
              ) : (
                repos.map(repo => {
                  const usedBy = connections?.find(c => c.repo_full_name === repo.full_name);
                  const isSelected = selectedRepo?.full_name === repo.full_name;
                  return (
                    <button
                      key={repo.full_name}
                      type="button"
                      disabled={!!usedBy}
                      onClick={() => handleSelectRepo(repo)}
                      className={cn(
                        "w-full flex items-center gap-2.5 px-3 py-2.5 text-sm text-left transition-colors",
                        isSelected ? "bg-primary/15" : "hover:bg-muted/60",
                        usedBy ? "opacity-50 cursor-not-allowed" : "cursor-pointer",
                      )}
                    >
                      <span className="flex-1 font-medium truncate">{repoPickerLabel(repo.full_name, githubLogin)}</span>
                      {repo.private && <Tag className="text-[10px] px-1.5 py-0.5">Private</Tag>}
                      {usedBy && <span className="text-[10px] text-muted-foreground shrink-0">Linked to {usedBy.agent_name}</span>}
                      {isSelected && <Check className="size-3.5 shrink-0 text-primary" />}
                    </button>
                  );
                })
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Branch selector — slides in when a repo is selected */}
      <div className={cn(
        "grid transition-[grid-template-rows] duration-150 ease-out",
        selectedRepo ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}>
        <div className="overflow-hidden">
          <div className="space-y-1.5 pt-0.5">
            <Label size="md" className="flex items-center gap-1.5">
              <GitBranch className="size-3.5" />
              Branch
            </Label>
            <div>
              <button
                type="button"
                onClick={() => setBranchOpen(prev => !prev)}
                className={cn(inputBase, inputFocusWithin, "w-full flex h-9 items-center justify-between px-3 cursor-pointer")}
              >
                <span className="text-sm">{selectedBranch}</span>
                {isLoadingBranches ? (
                  <ArrowPathIcon className="size-3.5 text-muted-foreground animate-spin" />
                ) : (
                  <ChevronDown className={cn(
                    "size-3.5 text-muted-foreground transition-transform duration-200",
                    branchOpen && "rotate-180",
                  )} />
                )}
              </button>
              <div className={cn(
                "grid transition-[grid-template-rows] duration-150 ease-out",
                branchOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
              )}>
                <div className="overflow-hidden">
                  <div className="mt-1.5 rounded-sm border border-border bg-background">
                    {branches.map(branch => (
                      <button
                        key={branch}
                        type="button"
                        data-testid="branch-option"
                        onClick={() => handleSelectBranch(branch)}
                        className={cn(
                          "w-full flex items-center justify-between px-3 py-2.5 text-sm text-left transition-colors",
                          selectedBranch === branch ? "bg-primary/15" : "hover:bg-muted/60",
                        )}
                      >
                        {branch}
                        {selectedBranch === branch && <Check className="size-3.5 text-primary" />}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Subpath — slides in when a repo is selected */}
      <div className={cn(
        "grid transition-[grid-template-rows] duration-150 ease-out",
        selectedRepo ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}>
        <div className="overflow-hidden">
          <div className="space-y-1.5 pt-0.5">
            <Label size="md" className="flex items-center gap-1.5">
              <FolderOpen className="size-3.5" />
              Path
              <span className="font-normal text-muted-foreground">(optional)</span>
            </Label>
            <div className={cn(inputBase, inputFocusWithin, "flex h-9 items-center gap-2 px-3")}>
              <input
                type="text"
                className="flex-1 min-w-0 bg-transparent border-none outline-none text-sm placeholder:text-muted-foreground"
                placeholder="e.g. path/to/my-agent"
                value={subpath}
                onChange={e => handleSubpathChange(e.target.value)}
              />
            </div>
            <p className="text-xs text-muted-foreground">
              Only trigger builds when files inside this path change.
            </p>
          </div>
        </div>
      </div>

    </div>
  );
}
