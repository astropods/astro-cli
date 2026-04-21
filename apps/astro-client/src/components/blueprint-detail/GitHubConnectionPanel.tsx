import { useState, useEffect } from "react";
import { useSearchParams } from "react-router";
import { Github, GitBranch, CheckCircle2, XCircle, Clock, Loader2, Link2Off, ExternalLink, ScrollText, RefreshCw, MoreHorizontal, ChevronDown, ChevronRight, CircleDot } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Spinner } from "@/components/ui/spinner";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { SidebarSection } from "./SidebarSection";
import {
  useGitHubStatus,
  useGitHubAccountRepos,
  useGitHubAccountConnect,
  useGitHubLink,
  useGitHubDisconnect,
  useGitHubBuildLogs,
  useGitHubRebuild,
} from "@/api/queries/github";
import { RepoPicker } from "@/components/new-blueprint/RepoPicker";
import type { GitHubBuild, GitHubRepo } from "@/lib/api";
import { cn } from "@/lib/utils";

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
  const connect = useGitHubAccountConnect(account);
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
    connect.mutate(`/${account}/${name}?github_connected=true`, {
      onSuccess: (data) => {
        if (data.connected) {
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
      <SidebarSection title="GitHub">
        <div className="flex items-center gap-2 py-1 text-muted-foreground text-sm">
          <Spinner size={14} />
          <span>Loading…</span>
        </div>
      </SidebarSection>
    );
  }

  const githubLogin = (status?.connected ? status.repo_full_name : effectiveRepo)?.split("/")[0];

  return (
    <>
      <SidebarSection
        title="GitHub"
        trailing={githubLogin && (
          <span className="flex items-center gap-1 font-mono text-[10px] text-foreground">
            <CheckCircle2 className="size-3 shrink-0 text-green-600 dark:text-green-400" />
            {githubLogin}
          </span>
        )}
      >
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

export interface ConnectedRepoViewProps {
  account: string;
  name: string;
  status: { repo_full_name?: string; branch?: string; builds: GitHubBuild[] };
  statusLoading: boolean;
  rebuild: { mutate: () => void; isPending: boolean };
  disconnect: { mutate: () => void; isPending: boolean };
}

export function ConnectedRepoView({ account, name, status, statusLoading, rebuild, disconnect }: ConnectedRepoViewProps) {
  const [logsOpen, setLogsOpen] = useState(false);
  const latestBuild = status.builds[0];

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
            <span className="truncate">{status.repo_full_name?.split("/")[1] ?? status.repo_full_name}</span>
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
            {latestBuild && (
              <>
                <DropdownMenuItem onClick={() => setLogsOpen(true)}>
                  <ScrollText className="h-3.5 w-3.5" />
                  Build Logs
                </DropdownMenuItem>
                <DropdownMenuSeparator />
              </>
            )}
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
      {latestBuild && (
        <BuildLogsDialog
          account={account}
          name={name}
          buildId={latestBuild.build_id}
          commitSha={latestBuild.commit_sha?.slice(0, 7) ?? "unknown"}
          isActive={latestBuild.status === "pending" || latestBuild.status === "building"}
          open={logsOpen}
          onOpenChange={setLogsOpen}
        />
      )}

      {status.builds.length === 0 && statusLoading && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Spinner size={10} />
          <span>Checking build status…</span>
        </div>
      )}
      {status.builds.length === 0 && !statusLoading && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="relative flex h-2 w-2 shrink-0">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75" />
            <span className="relative inline-flex rounded-full h-2 w-2 bg-amber-500" />
          </span>
          <span>
            Awaiting{" "}
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
  { key: "fetching-spec", label: "Fetching spec" },
  { key: "building",      label: "Building" },
  { key: "registering",   label: "Registering" },
] as const;

function BuildRow({ build, account, name }: { build: GitHubBuild; account: string; name: string }) {
  const [logsOpen, setLogsOpen] = useState(false);
  const isActive = build.status === "pending" || build.status === "building";

  const title = build.commit_message
    ? build.commit_message.split("\n")[0]
    : build.build_id;

  return (
    <>
      <div
        className="rounded border border-border bg-muted/20 px-2.5 py-2 space-y-1.5 text-xs cursor-pointer hover:bg-muted/40 transition-colors"
        onClick={() => setLogsOpen(true)}
      >
        {/* Row 1: title */}
        <span className={cn(
          "block leading-snug font-medium truncate",
          build.status === "failed" && "text-destructive",
        )}>
          {title}
        </span>

        {/* Row 2: step pipeline (active) or status icon (inactive) */}
        {isActive ? (
          <StepPipeline currentStep={build.step} />
        ) : (
          <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
            <BuildStatusIcon status={build.status} className="shrink-0" />
            {build.status === "registered" && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="font-mono text-green-600 dark:text-green-400 cursor-default">{build.build_id} successful</span>
                  </TooltipTrigger>
                  {build.completed_at && (
                    <TooltipContent>{new Date(build.completed_at).toLocaleString()}</TooltipContent>
                  )}
                </Tooltip>
              </TooltipProvider>
            )}
            {build.status === "failed" && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="font-mono text-destructive cursor-default">Error: see logs for more</span>
                  </TooltipTrigger>
                  {build.completed_at && (
                    <TooltipContent>{new Date(build.completed_at).toLocaleString()}</TooltipContent>
                  )}
                </Tooltip>
              </TooltipProvider>
            )}
          </div>
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

      </div>
      <BuildLogsDialog
        account={account}
        name={name}
        buildId={build.build_id}
        commitSha={build.commit_sha?.slice(0, 7) ?? "unknown"}
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

  const activeStep = currentIdx >= 0 ? BUILD_STEPS[currentIdx] : null;
  const stepNum = currentIdx >= 0 ? currentIdx + 1 : 1;

  return (
    <div className="flex items-center gap-1.5 text-xs text-blue-600 dark:text-blue-400">
      <Loader2 className="h-3 w-3 animate-spin shrink-0" />
      <span className="font-medium">{activeStep?.label ?? "Building"}</span>
      {buildProgress && (
        <span className="text-muted-foreground font-normal">{buildProgress}</span>
      )}
      <span className="text-muted-foreground font-mono ml-auto">({stepNum}/{BUILD_STEPS.length})</span>
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
            Build logs: <span className="font-mono">{buildId}</span>{" "}
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
            <p className="text-sm text-red-400 p-4">Logs unavailable. The pod may have been cleaned up.</p>
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
  const [repoQuery, setRepoQuery] = useState("");
  const { data: reposData, isLoading: reposLoading } = useGitHubAccountRepos(account, { enabled: open, q: repoQuery });
  const link = useGitHubLink(account, name);
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [selectedBranch, setSelectedBranch] = useState("main");

  // Default branch to repo default when repo changes.
  useEffect(() => {
    if (selectedRepo) setSelectedBranch(selectedRepo.default_branch);
  }, [selectedRepo]);

  function handleLink() {
    if (!selectedRepo) return;
    link.mutate(
      { repo_full_name: selectedRepo.full_name, branch: selectedBranch },
      {
        onSuccess: () => {
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

        <div className="py-2">
          <RepoPicker
            githubLogin={undefined}
            selectedRepo={selectedRepo}
            selectedBranch={selectedBranch}
            isLoadingRepos={reposLoading}
            repos={reposData?.repos ?? []}
            connections={undefined}
            onSelectRepo={setSelectedRepo}
            onSelectBranch={setSelectedBranch}
            onSearchChange={setRepoQuery}
          />

          {link.isError && (
            <p className="text-sm text-destructive px-4">
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
