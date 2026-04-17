import { useState, useEffect } from "react";
import { useSearchParams } from "react-router";
import { Github, GitBranch, CheckCircle2, XCircle, Clock, Loader2, Link2Off, ExternalLink, ScrollText, RefreshCw, MoreHorizontal, FlaskConical, ChevronDown, ChevronRight, CircleDot } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
  useGitHubBuildLogs,
  useGitHubRebuild,
} from "@/api/queries/github";
import type { GitHubBuild } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useExperiments } from "@/lib/experiments";

interface GitHubConnectionPanelProps {
  account: string;
  name: string;
  preConnectedRepo?: string;
  preConnectedBranch?: string;
}

export function GitHubConnectionPanel({ account, name, preConnectedRepo, preConnectedBranch }: GitHubConnectionPanelProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const githubConnected = searchParams.get("github_connected") === "true";

  const { data: status, isLoading: statusLoading } = useGitHubStatus(account, name);
  const connect = useGitHubConnect(account, name);
  const disconnect = useGitHubDisconnect(account, name);
  const rebuild = useGitHubRebuild(account, name);

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

  // Use the server-confirmed repo if connected; fall back to the wizard-supplied repo
  // so the panel never flashes back to "Connect" while the status query is in flight or
  // if the server is slightly behind the just-completed githubLink call.
  const effectiveRepo = status?.connected ? status.repo_full_name : preConnectedRepo;

  if (statusLoading && !preConnectedRepo) {
    return (
      <SidebarSection title="GitHub" badge={<FlaskConical className="h-3 w-3" />} badgeTooltip="Experimental feature">
        <div className="flex items-center gap-2 py-1 text-muted-foreground text-sm">
          <Spinner size={14} />
          <span>Loading…</span>
        </div>
      </SidebarSection>
    );
  }

  return (
    <>
      <SidebarSection title="GitHub" badge={<FlaskConical className="h-3 w-3" />} badgeTooltip="Experimental feature">
        {status?.connected || effectiveRepo ? (
          <ConnectedRepoView
            account={account}
            name={name}
            status={status?.connected ? status : { repo_full_name: effectiveRepo, branch: preConnectedBranch, builds: [] }}
            statusLoading={statusLoading}
            rebuild={rebuild}
            disconnect={disconnect}
          />
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

interface ConnectedRepoViewProps {
  account: string;
  name: string;
  status: { repo_full_name?: string; branch?: string; builds: GitHubBuild[] };
  statusLoading: boolean;
  rebuild: { mutate: () => void; isPending: boolean };
  disconnect: { mutate: () => void; isPending: boolean };
}

function ConnectedRepoView({ account, name, status, statusLoading, rebuild, disconnect }: ConnectedRepoViewProps) {
  return (
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
          {status.branch && (
            <div className="flex items-center gap-1 mt-0.5 text-xs text-muted-foreground">
              <GitBranch className="h-3 w-3" />
              <span>{status.branch}</span>
            </div>
          )}
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 shrink-0 text-muted-foreground hover:text-foreground"
            >
              <MoreHorizontal className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onClick={() => rebuild.mutate()}
              disabled={rebuild.isPending}
            >
              {rebuild.isPending ? <Spinner size={14} /> : <RefreshCw className="h-3.5 w-3.5" />}
              Rebuild branch
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => disconnect.mutate()}
              disabled={disconnect.isPending}
              className="text-destructive focus:text-destructive"
            >
              {disconnect.isPending ? <Spinner size={14} /> : <Link2Off className="h-3.5 w-3.5" />}
              Disconnect
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {status.builds.length === 0 && statusLoading && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Spinner size={10} />
          <span>Checking build status…</span>
        </div>
      )}
      {status.builds.length === 0 && !statusLoading && (
        <div className="flex items-start gap-2 text-xs text-muted-foreground">
          <span className="relative flex h-2 w-2 shrink-0 mt-0.5">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75" />
            <span className="relative inline-flex rounded-full h-2 w-2 bg-amber-500" />
          </span>
          <span>
            Waiting for{" "}
            <span className="font-mono text-foreground">astropods.yml</span>
            {status.branch && (
              <> on <span className="font-mono text-foreground">{status.branch}</span></>
            )}
          </span>
        </div>
      )}

      {status.builds.length > 0 && (
        <div className="space-y-1">
          {status.builds.slice(0, 2).map((build) => (
            <BuildRow key={build.id} build={build} account={account} name={name} />
          ))}
        </div>
      )}
    </div>
  );
}

// Build pipeline steps in order.
const BUILD_STEPS = [
  { key: "fetching-spec", label: "Fetch spec" },
  { key: "building",      label: "Build"      },
  { key: "registering",   label: "Register"   },
] as const;

// Strip repetitive Go/BuildKit error prefixes and truncate to the meaningful part.
function cleanBuildError(err: string): string {
  const stripped = err
    .replace(/^JobCancelError:\s*/i, "")
    .replace(/^build \w+:\s*/i, "")
    .replace(/^build job failed:\s*/i, "")
    .replace(/^error:\s*/i, "")
    .replace(/^failed to solve:\s*/i, "")
    .trim();
  return stripped.length > 120 ? stripped.slice(0, 120) + "…" : stripped;
}

function elapsedLabel(from: string, to?: string | null): string {
  const start = new Date(from).getTime();
  const end = to ? new Date(to).getTime() : Date.now();
  const s = Math.floor((end - start) / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rem = s % 60;
  return rem > 0 ? `${m}m ${rem}s` : `${m}m`;
}

function BuildRow({ build, account, name }: { build: GitHubBuild; account: string; name: string }) {
  const shortSha = build.commit_sha?.slice(0, 7) ?? "unknown";
  const [logsOpen, setLogsOpen] = useState(false);
  const isActive = build.status === "pending" || build.status === "building";

  const title = build.commit_message
    ? build.commit_message.split("\n")[0]
    : build.build_id;

  return (
    <>
      <div className="rounded border border-border bg-muted/20 px-2.5 py-2 space-y-1.5 text-xs">
        {/* Top row: status icon + title + logs button */}
        <div className="flex items-start gap-1.5">
          <BuildStatusIcon status={build.status} className="mt-0.5 shrink-0" />
          <span className={cn(
            "flex-1 leading-snug font-medium truncate",
            build.status === "failed" && "text-destructive",
          )}>
            {title}
          </span>
          <button
            onClick={() => setLogsOpen(true)}
            className="text-muted-foreground hover:text-foreground shrink-0 ml-1"
            title="View logs"
          >
            <ScrollText className="h-3 w-3" />
          </button>
        </div>

        {/* Meta row: sha + author + elapsed */}
        <div className="flex items-center gap-1.5 text-muted-foreground font-mono pl-5">
          <span>{shortSha}</span>
          {build.commit_author && (
            <><span>·</span><span className="font-sans truncate max-w-[100px]">{build.commit_author}</span></>
          )}
          <span className="ml-auto">{elapsedLabel(build.enqueued_at, build.completed_at)}</span>
        </div>

        {/* Step pipeline — shown for active builds */}
        {isActive && (
          <StepPipeline currentStep={build.step} />
        )}

        {/* Per-component status — shown when components exist */}
        {build.components && build.components.length > 0 && (
          <div className="flex items-center gap-1 pl-5 flex-wrap">
            {build.components.map((comp) => (
              <span key={comp.component_name} className={cn(
                "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium border",
                comp.status === "succeeded" && "text-green-600 dark:text-green-400 border-green-600/20 bg-green-600/5",
                comp.status === "failed" && "text-destructive border-destructive/20 bg-destructive/5",
                comp.status === "building" && "text-blue-600 dark:text-blue-400 border-blue-600/20 bg-blue-600/5",
                comp.status === "pending" && "text-muted-foreground border-border",
              )}>
                {comp.status === "succeeded" && <CheckCircle2 className="h-2.5 w-2.5" />}
                {comp.status === "failed" && <XCircle className="h-2.5 w-2.5" />}
                {comp.status === "building" && <Loader2 className="h-2.5 w-2.5 animate-spin" />}
                {comp.status === "pending" && <CircleDot className="h-2.5 w-2.5 opacity-40" />}
                {comp.component_name}
              </span>
            ))}
          </div>
        )}

        {/* Error — shown for failed builds */}
        {build.status === "failed" && build.error && (
          <p className="text-destructive pl-5 leading-snug break-words">{cleanBuildError(build.error)}</p>
        )}
      </div>

      <BuildLogsDialog
        account={account}
        name={name}
        buildId={build.build_id}
        commitSha={shortSha}
        isActive={isActive}
        open={logsOpen}
        onOpenChange={setLogsOpen}
      />
    </>
  );
}

// Extract build progress detail from step strings like "building (1/3: agent)".
function parseBuildProgress(step?: string): string | null {
  if (!step) return null;
  const match = step.match(/\((.+)\)$/);
  return match ? match[1] : null;
}

function StepPipeline({ currentStep }: { currentStep?: string }) {
  const currentIdx = BUILD_STEPS.findIndex((s) => currentStep?.startsWith(s.key));
  const buildProgress = parseBuildProgress(currentStep);

  return (
    <div className="flex items-center gap-0 pl-5">
      {BUILD_STEPS.map((step, i) => {
        const isDone = currentIdx > i;
        const isActive = currentIdx === i;
        return (
          <div key={step.key} className="flex items-center">
            <div className={cn(
              "flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium",
              isActive && "text-blue-600 dark:text-blue-400",
              isDone && "text-green-600 dark:text-green-400",
              !isActive && !isDone && "text-muted-foreground",
            )}>
              {isDone && <CheckCircle2 className="h-2.5 w-2.5" />}
              {isActive && <Loader2 className="h-2.5 w-2.5 animate-spin" />}
              {!isDone && !isActive && <CircleDot className="h-2.5 w-2.5 opacity-30" />}
              {step.label}
              {isActive && buildProgress && (
                <span className="text-muted-foreground font-normal">{buildProgress}</span>
              )}
            </div>
            {i < BUILD_STEPS.length - 1 && (
              <span className="text-muted-foreground/40 mx-0.5">›</span>
            )}
          </div>
        );
      })}
    </div>
  );
}

// Parses "=== container ===\n..." sections from raw log output.
function parseLogSections(raw: string): { name: string; content: string }[] {
  const sections: { name: string; content: string }[] = [];
  const parts = raw.split(/^=== .+? ===$\n?/m);
  const headers = [...raw.matchAll(/^=== (.+?) ===/gm)].map((m) => m[1]);

  // First part before any header — attach as unnamed if non-empty
  if (parts[0]?.trim()) {
    sections.push({ name: "output", content: parts[0] });
  }
  headers.forEach((name, i) => {
    sections.push({ name, content: parts[i + 1] ?? "" });
  });

  // Fallback: no headers found
  if (sections.length === 0 && raw.trim()) {
    sections.push({ name: "output", content: raw });
  }
  return sections;
}

function statusColor(s: string) {
  if (s === "succeeded") return "text-green-500";
  if (s === "failed") return "text-red-400";
  if (s === "building") return "text-blue-400";
  return "text-zinc-500";
}

function BuildLogsDialog({
  account, name, buildId, commitSha, isActive, open, onOpenChange,
}: {
  account: string; name: string; buildId: string; commitSha: string;
  isActive: boolean; open: boolean; onOpenChange: (v: boolean) => void;
}) {
  const { data, isLoading, isError } = useGitHubBuildLogs(account, name, buildId, {
    enabled: open,
    refetchInterval: open && isActive ? 3000 : false,
  });

  // Build structured component logs, or fall back to flat logs for old builds.
  const componentLogs = data?.components && data.components.length > 0
    ? data.components
    : data?.logs
      ? [{ name: "agent", status: "unknown", logs: data.logs }]
      : [];

  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  // Auto-expand all sections when data arrives.
  useEffect(() => {
    if (componentLogs.length > 0) {
      const keys = new Set<string>();
      for (const comp of componentLogs) {
        keys.add(comp.name);
        for (const s of parseLogSections(comp.logs || "")) {
          keys.add(`${comp.name}/${s.name}`);
        }
      }
      setExpanded(keys);
    }
  }, [data]);

  function toggle(key: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      next.has(key) ? next.delete(key) : next.add(key);
      return next;
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl gap-0 p-0">
        <DialogHeader className="px-4 pt-4 pb-3 border-b">
          <DialogTitle className="text-sm font-medium">
            Build logs — <span className="font-mono">{buildId}</span>{" "}
            <span className="text-muted-foreground font-normal">·{commitSha}</span>
          </DialogTitle>
          <DialogDescription className="text-xs">
            {componentLogs.length > 1
              ? `${componentLogs.length} components`
              : "Last 500 lines per container"}
          </DialogDescription>
        </DialogHeader>

        <div className="overflow-y-auto max-h-[65vh] bg-zinc-950 text-zinc-100 rounded-b-lg">
          {isLoading && (
            <div className="flex items-center justify-center py-10 gap-2 text-zinc-400 text-sm">
              <Spinner size={16} /><span>Loading logs…</span>
            </div>
          )}
          {isError && (
            <p className="text-sm text-red-400 p-4">Logs unavailable — the pod may have been cleaned up.</p>
          )}
          {data && componentLogs.length === 0 && (
            <p className="text-zinc-500 text-xs p-4 font-mono">(no output)</p>
          )}
          {componentLogs.map((comp) => {
            const sections = parseLogSections(comp.logs || "");
            const compOpen = expanded.has(comp.name);
            return (
              <div key={comp.name} className="border-b border-zinc-800 last:border-0">
                {/* Component header */}
                <button
                  onClick={() => toggle(comp.name)}
                  className="w-full flex items-center gap-2 px-3 py-2 text-xs font-mono hover:bg-zinc-900 text-left"
                >
                  {compOpen
                    ? <ChevronDown className="h-3 w-3 text-zinc-500 shrink-0" />
                    : <ChevronRight className="h-3 w-3 text-zinc-500 shrink-0" />}
                  <span className="text-zinc-200 font-medium">{comp.name}</span>
                  <span className={cn("text-[10px]", statusColor(comp.status))}>{comp.status}</span>
                </button>
                {/* Container sections within this component */}
                {compOpen && sections.map((section) => {
                  const sectionKey = `${comp.name}/${section.name}`;
                  const sectionOpen = expanded.has(sectionKey);
                  return (
                    <div key={sectionKey} className="border-t border-zinc-800/50">
                      <button
                        onClick={() => toggle(sectionKey)}
                        className="w-full flex items-center gap-2 px-5 py-1.5 text-[11px] font-mono text-zinc-400 hover:bg-zinc-900/50 text-left"
                      >
                        {sectionOpen
                          ? <ChevronDown className="h-2.5 w-2.5 text-zinc-600 shrink-0" />
                          : <ChevronRight className="h-2.5 w-2.5 text-zinc-600 shrink-0" />}
                        {section.name}
                      </button>
                      {sectionOpen && section.content.trim() && (
                        <pre className="text-[11px] font-mono whitespace-pre-wrap break-all leading-[1.6] px-6 pb-3 pt-1 text-zinc-300">
                          {section.content}
                        </pre>
                      )}
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function BuildStatusIcon({ status, className }: { status: GitHubBuild["status"]; className?: string }) {
  switch (status) {
    case "registered":
      return <CheckCircle2 className={cn("h-3.5 w-3.5 text-green-600 dark:text-green-400", className)} />;
    case "failed":
      return <XCircle className={cn("h-3.5 w-3.5 text-destructive", className)} />;
    case "building":
      return <Loader2 className={cn("h-3.5 w-3.5 text-blue-500 animate-spin", className)} />;
    default:
      return <Clock className={cn("h-3.5 w-3.5 text-muted-foreground", className)} />;
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
  const { setExperiment } = useExperiments();

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
      {
        onSuccess: () => {
          setExperiment("githubAutoBuild", true);
          onOpenChange(false);
        },
      }
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
