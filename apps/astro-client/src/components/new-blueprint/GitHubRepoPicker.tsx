import { useState, useCallback } from "react";
import { cn } from "@/lib/utils";
import { RepoPicker } from "./RepoPicker";
import { SubpathPicker } from "./SubpathPicker";
import { useGitHubAccountRepos, useGitHubAccountConnections } from "@/api/queries/github";
import type { GitHubRepo } from "@/lib/api";

export type GitHubRepoPickerValue = {
  repoFullName: string | null;
  branch: string;
};

type Props = {
  account: string;
  agentName: string;
  githubLogin?: string;
  enabled?: boolean;
  onChange: (value: GitHubRepoPickerValue) => void;
};

export function GitHubRepoPicker({ account, agentName, githubLogin, enabled = true, onChange }: Props) {
  const [repoQuery, setRepoQuery] = useState("");
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [selectedBranch, setSelectedBranch] = useState("main");
  const [subpath, setSubpath] = useState("");

  const { data: reposData, isLoading: isLoadingRepos } = useGitHubAccountRepos(account, {
    enabled,
    q: repoQuery,
    login: githubLogin,
  });
  const { data: connectionsData } = useGitHubAccountConnections(account, { enabled });

  const takenSubpaths = selectedRepo
    ? (connectionsData?.connections ?? [])
        .filter(c => {
          const slash = c.repo_full_name.indexOf("/", c.repo_full_name.indexOf("/") + 1);
          return slash !== -1
            && c.repo_full_name.slice(0, slash) === selectedRepo.full_name
            && c.agent_name !== agentName;
        })
        .map(c => ({
          subpath: c.repo_full_name.slice(selectedRepo.full_name.length + 1),
          agentName: c.agent_name,
        }))
    : [];

  const toRepoFullName = useCallback((repo: GitHubRepo, sub: string): string => {
    const cleaned = sub.trim().replace(/^\/+|\/+$/g, "");
    return cleaned ? `${repo.full_name}/${cleaned}` : repo.full_name;
  }, []);

  const handleSelectRepo = useCallback((repo: GitHubRepo | null) => {
    setSelectedRepo(repo);
    const branch = repo?.default_branch ?? "main";
    setSelectedBranch(branch);
    setSubpath("");
    onChange({ repoFullName: repo ? repo.full_name : null, branch });
  }, [onChange]);

  const handleSelectBranch = useCallback((branch: string) => {
    setSelectedBranch(branch);
    onChange({ repoFullName: selectedRepo ? toRepoFullName(selectedRepo, subpath) : null, branch });
  }, [onChange, selectedRepo, subpath, toRepoFullName]);

  const handleSubpathChange = useCallback((newSubpath: string) => {
    setSubpath(newSubpath);
    if (selectedRepo) {
      onChange({ repoFullName: toRepoFullName(selectedRepo, newSubpath), branch: selectedBranch });
    }
  }, [onChange, selectedRepo, selectedBranch, toRepoFullName]);

  return (
    <>
      <RepoPicker
        githubLogin={githubLogin}
        selectedRepo={selectedRepo}
        selectedBranch={selectedBranch}
        isLoadingRepos={isLoadingRepos}
        repos={reposData?.repos ?? []}
        connections={connectionsData?.connections}
        onSelectRepo={handleSelectRepo}
        onSelectBranch={handleSelectBranch}
        onSearchChange={setRepoQuery}
      />
      <div className={cn(
        "grid transition-[grid-template-rows] duration-150 ease-out",
        selectedRepo ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}>
        <div className="overflow-hidden">
          <div className="px-4 pb-3">
            <SubpathPicker
              account={account}
              repo={selectedRepo?.full_name ?? ""}
              branch={selectedBranch}
              value={subpath}
              onChange={handleSubpathChange}
              enabled={enabled && !!selectedRepo}
              takenSubpaths={takenSubpaths}
            />
          </div>
        </div>
      </div>
    </>
  );
}
