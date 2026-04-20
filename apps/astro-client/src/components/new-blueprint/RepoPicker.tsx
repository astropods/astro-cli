import { useState, useRef, useEffect } from "react";
import { ArrowPathIcon, XMarkIcon } from "@heroicons/react/24/outline";
import { Check, ChevronDown, Search } from "lucide-react";
import { cn } from "@/lib/utils";
import { inputBase, inputFocusWithin } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tag } from "@/components/Tag";
import type { GitHubRepo } from "@/lib/api";

type Connection = { agent_name: string; repo_full_name: string };

type RepoPickerProps = {
  githubLogin: string | undefined;
  selectedRepo: GitHubRepo | null;
  selectedBranch: string;
  isLoadingRepos: boolean;
  repos: GitHubRepo[];
  connections: Connection[] | undefined;
  onSelectRepo: (repo: GitHubRepo | null) => void;
  onSelectBranch: (branch: string) => void;
};

export function RepoPicker({
  githubLogin,
  selectedRepo,
  selectedBranch,
  isLoadingRepos,
  repos,
  connections,
  onSelectRepo,
  onSelectBranch,
}: RepoPickerProps) {
  const [query, setQuery] = useState("");
  const [repoOpen, setRepoOpen] = useState(false);
  const [branchOpen, setBranchOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const filtered = query.trim()
    ? repos.filter(r => r.full_name.toLowerCase().includes(query.toLowerCase()))
    : repos;

  const branches = [
    "main",
    "master",
    ...(selectedRepo?.default_branch && !["main", "master"].includes(selectedRepo.default_branch)
      ? [selectedRepo.default_branch]
      : []),
  ];

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

  function handleSelectRepo(repo: GitHubRepo) {
    onSelectRepo(repo);
    setQuery("");
    setRepoOpen(false);
  }

  function handleClear() {
    onSelectRepo(null);
    setQuery("");
    setTimeout(() => inputRef.current?.focus(), 0);
  }

  return (
    <div ref={containerRef} className="px-4 py-3 space-y-3">

      {/* Repo autocomplete */}
      <div>
        <div
          className={cn(inputBase, inputFocusWithin, "flex h-9 items-center gap-2 cursor-text px-3")}
          onClick={() => inputRef.current?.focus()}
        >
          {selectedRepo && !repoOpen ? (
            <>
              <Check className="size-3.5 text-green-700 shrink-0" />
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
                onChange={e => { setQuery(e.target.value); setRepoOpen(!!e.target.value.trim()); }}
              />
              {query && (
                <button
                  type="button"
                  onClick={() => { setQuery(""); inputRef.current?.focus(); }}
                  className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                  aria-label="Clear search"
                >
                  <XMarkIcon className="size-3.5" />
                </button>
              )}
            </>
          )}
        </div>

        {/* Repo dropdown — inline so the card grows naturally */}
        <div className={cn(
          "grid transition-[grid-template-rows] duration-200 ease-out",
          repoOpen && query.trim() ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}>
          <div className="overflow-hidden">
            <div className="mt-1.5 max-h-52 overflow-y-auto rounded-sm border border-border bg-background">
              {isLoadingRepos ? (
                <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
                  <ArrowPathIcon className="size-4 animate-spin" />
                  Loading repositories...
                </div>
              ) : filtered.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  No repos matching &ldquo;{query}&rdquo;
                </div>
              ) : filtered.map(repo => {
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
                      isSelected ? "bg-primary/5" : "hover:bg-muted/60",
                      usedBy ? "opacity-50 cursor-not-allowed" : "cursor-pointer",
                    )}
                  >
                    <span className="flex-1 font-medium truncate">{repo.full_name.split("/")[1]}</span>
                    {repo.private && <Tag className="text-[10px] px-1.5 py-0.5">Private</Tag>}
                    {usedBy && <span className="text-[10px] text-muted-foreground shrink-0">linked to {usedBy.agent_name}</span>}
                    {isSelected && <Check className="size-3.5 shrink-0 text-primary" />}
                  </button>
                );
              })}
            </div>
          </div>
        </div>
      </div>

      {/* Branch selector — slides in when a repo is selected, expands inline */}
      <div className={cn(
        "grid transition-[grid-template-rows] duration-200 ease-out",
        selectedRepo ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}>
        <div className="overflow-hidden">
          <div className="space-y-1.5 pt-0.5">
            <Label size="md">Branch</Label>

            <div>
              <button
                type="button"
                onClick={() => setBranchOpen(prev => !prev)}
                className={cn(inputBase, inputFocusWithin, "w-full flex h-9 items-center justify-between px-3 cursor-pointer")}
              >
                <span className="text-sm">{selectedBranch}</span>
                <ChevronDown className={cn(
                  "size-3.5 text-muted-foreground transition-transform duration-200",
                  branchOpen && "rotate-180",
                )} />
              </button>

              {/* Branch list — inline so it pushes the card down */}
              <div className={cn(
                "grid transition-[grid-template-rows] duration-200 ease-out",
                branchOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
              )}>
                <div className="overflow-hidden">
                  <div className="mt-1.5 rounded-sm border border-border bg-background">
                    {branches.map(branch => (
                      <button
                        key={branch}
                        type="button"
                        onClick={() => { onSelectBranch(branch); setBranchOpen(false); }}
                        className={cn(
                          "w-full flex items-center justify-between px-3 py-2.5 text-sm text-left transition-colors",
                          selectedBranch === branch ? "bg-primary/5" : "hover:bg-muted/60",
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

    </div>
  );
}
