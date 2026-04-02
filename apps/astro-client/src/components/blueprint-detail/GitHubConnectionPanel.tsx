import { useState, useEffect } from "react";
import { useSearchParams } from "react-router";
import { Github, GitBranch, CheckCircle2, XCircle, Clock, Loader2, Link2Off, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { SidebarSection } from "./SidebarSection";
import {
  useGitHubStatus,
  useGitHubRepos,
  useGitHubConnect,
  useGitHubLink,
  useGitHubDisconnect,
} from "@/api/queries/github";
import type { GitHubBuild } from "@/lib/api";
import { cn } from "@/lib/utils";

interface GitHubConnectionPanelProps {
  account: string;
  name: string;
}

export function GitHubConnectionPanel({ account, name }: GitHubConnectionPanelProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const githubConnected = searchParams.get("github_connected") === "true";

  const { data: status, isLoading: statusLoading } = useGitHubStatus(account, name);
  const connect = useGitHubConnect(account, name);
  const disconnect = useGitHubDisconnect(account, name);

  // After OAuth callback, open the repo selector automatically.
  const [repoDialogOpen, setRepoDialogOpen] = useState(githubConnected);

  // Clean up the query param once dialog is open.
  useEffect(() => {
    if (githubConnected) {
      setSearchParams((p) => { p.delete("github_connected"); return p; }, { replace: true });
    }
  }, [githubConnected]);

  function handleConnect() {
    connect.mutate(undefined, {
      onSuccess: (data) => {
        if (data.connected) {
          // Token already exists — go straight to repo selector.
          setRepoDialogOpen(true);
        } else if (data.redirect_url) {
          window.location.href = data.redirect_url;
        }
      },
    });
  }

  if (statusLoading) {
    return (
      <SidebarSection title="GitHub">
        <div className="flex items-center gap-2 py-1 text-muted-foreground text-sm">
          <Spinner size={14} />
          <span>Loading…</span>
        </div>
      </SidebarSection>
    );
  }

  return (
    <>
      <SidebarSection title="GitHub">
        {status?.connected ? (
          <div className="space-y-3">
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <a
                  href={`https://github.com/${status.repo_full_name}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1.5 text-sm font-medium hover:underline truncate"
                >
                  <Github className="h-3.5 w-3.5 shrink-0" />
                  <span className="truncate">{status.repo_full_name}</span>
                  <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
                </a>
                <div className="flex items-center gap-1 mt-0.5 text-xs text-muted-foreground">
                  <GitBranch className="h-3 w-3" />
                  <span>{status.branch}</span>
                </div>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 shrink-0 text-muted-foreground hover:text-destructive"
                onClick={() => disconnect.mutate()}
                disabled={disconnect.isPending}
                title="Disconnect repo"
              >
                {disconnect.isPending ? <Spinner size={14} /> : <Link2Off className="h-3.5 w-3.5" />}
              </Button>
            </div>

            {status.builds.length > 0 && (
              <div className="space-y-1.5">
                <p className="text-[11px] uppercase tracking-wide text-muted-foreground font-mono">Recent Builds</p>
                <div className="space-y-1">
                  {status.builds.slice(0, 5).map((build) => (
                    <BuildRow key={build.id} build={build} />
                  ))}
                </div>
              </div>
            )}

            {status.builds.length === 0 && (
              <p className="text-xs text-muted-foreground">
                Push to <span className="font-mono">{status.branch}</span> to trigger a build.
              </p>
            )}
          </div>
        ) : (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              Connect a GitHub repo to auto-build on every push to main.
            </p>
            <Button
              size="sm"
              variant="outline"
              className="w-full gap-2"
              onClick={handleConnect}
              disabled={connect.isPending}
            >
              {connect.isPending ? <Spinner size={14} /> : <Github className="h-3.5 w-3.5" />}
              Connect GitHub repo
            </Button>
            {connect.isError && (
              <p className="text-xs text-destructive">
                {connect.error instanceof Error ? connect.error.message : "Failed to connect"}
              </p>
            )}
          </div>
        )}
      </SidebarSection>

      <RepoSelectorDialog
        account={account}
        name={name}
        open={repoDialogOpen}
        onOpenChange={setRepoDialogOpen}
      />
    </>
  );
}

function BuildRow({ build }: { build: GitHubBuild }) {
  const short = build.commit_sha.slice(0, 7);
  return (
    <div className="flex items-center gap-2 text-xs">
      <BuildStatusIcon status={build.status} />
      <span className="font-mono text-muted-foreground">{short}</span>
      <span className={cn(
        "capitalize",
        build.status === "failed" && "text-destructive",
        build.status === "registered" && "text-green-600 dark:text-green-400",
      )}>
        {build.status}
      </span>
    </div>
  );
}

function BuildStatusIcon({ status }: { status: GitHubBuild["status"] }) {
  switch (status) {
    case "registered":
      return <CheckCircle2 className="h-3.5 w-3.5 text-green-600 dark:text-green-400 shrink-0" />;
    case "failed":
      return <XCircle className="h-3.5 w-3.5 text-destructive shrink-0" />;
    case "building":
      return <Loader2 className="h-3.5 w-3.5 text-blue-500 shrink-0 animate-spin" />;
    default:
      return <Clock className="h-3.5 w-3.5 text-muted-foreground shrink-0" />;
  }
}

interface RepoSelectorDialogProps {
  account: string;
  name: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function RepoSelectorDialog({ account, name, open, onOpenChange }: RepoSelectorDialogProps) {
  const { data: reposData, isLoading: reposLoading } = useGitHubRepos(account, name, { enabled: open });
  const link = useGitHubLink(account, name);

  const [selectedRepo, setSelectedRepo] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("main");

  const selectedRepoData = reposData?.repos.find((r) => r.full_name === selectedRepo);

  // Default branch to repo default when repo changes.
  useEffect(() => {
    if (selectedRepoData) setSelectedBranch(selectedRepoData.default_branch);
  }, [selectedRepo]);

  function handleLink() {
    if (!selectedRepo) return;
    link.mutate(
      { repo_full_name: selectedRepo, branch: selectedBranch },
      { onSuccess: () => onOpenChange(false) }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Connect GitHub repository</DialogTitle>
          <DialogDescription>
            Select a repository to link to <span className="font-mono font-medium">{name}</span>.
            Pushes to the selected branch will trigger automatic builds.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {reposLoading ? (
            <div className="flex items-center justify-center py-6 gap-2 text-muted-foreground text-sm">
              <Spinner size={16} />
              <span>Loading repositories…</span>
            </div>
          ) : (
            <>
              <div className="space-y-1.5">
                <label className="text-sm font-medium">Repository</label>
                <Select value={selectedRepo} onValueChange={setSelectedRepo}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select a repository…" />
                  </SelectTrigger>
                  <SelectContent>
                    {reposData?.repos.map((repo) => (
                      <SelectItem key={repo.full_name} value={repo.full_name}>
                        <span className="flex items-center gap-2">
                          {repo.full_name}
                          {repo.private && (
                            <span className="text-[10px] text-muted-foreground bg-muted px-1 rounded">private</span>
                          )}
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {selectedRepo && (
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">Branch</label>
                  <Select value={selectedBranch} onValueChange={setSelectedBranch}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="main">main</SelectItem>
                      <SelectItem value="master">master</SelectItem>
                      {selectedRepoData?.default_branch &&
                        !["main", "master"].includes(selectedRepoData.default_branch) && (
                          <SelectItem value={selectedRepoData.default_branch}>
                            {selectedRepoData.default_branch}
                          </SelectItem>
                        )}
                    </SelectContent>
                  </Select>
                </div>
              )}
            </>
          )}

          {link.isError && (
            <p className="text-sm text-destructive">
              {link.error instanceof Error ? link.error.message : "Failed to link repository"}
            </p>
          )}
        </div>

        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleLink}
            disabled={!selectedRepo || link.isPending}
          >
            {link.isPending && <Spinner size={14} className="mr-2" />}
            Connect repository
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
