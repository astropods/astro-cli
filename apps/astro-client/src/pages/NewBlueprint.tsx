import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { useAuth } from "../lib/auth";
import { useExperiments } from "@/lib/experiments";
import { useCreateBlueprint, useUploadBlueprintAvatar, useBlueprint, useGitHubAccountConnect, useGitHubAccountRepos, useGitHubLink, useGitHubAccountScan, useGitHubRebuild, useGitHubStatus } from "@/api/queries";
import type { GitHubRepo } from "@/lib/api";
import { bustAgentAvatar } from "@/lib/avatar-bust";
import { useNavigate, useSearchParams, type MetaFunction } from "react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { Camera } from "lucide-react";
import {
  ArrowPathIcon,
  BuildingOffice2Icon,
  CommandLineIcon,
  CheckCircleIcon,
} from "@heroicons/react/24/outline";
import { UserAvatar } from "@/components/UserAvatar";
import { Globe, LockKeyhole } from "lucide-react";
import { LiveRevealConfetti } from "@/components/deployed-agent/detail/LiveRevealConfetti";

export const meta: MetaFunction = () => [{ title: "New Agent | Astro" }];

// ─── Types ───────────────────────────────────────────────────────────────────

type Step = "setup" | "source" | "publishing" | "review";
type SourcePath = "fresh" | "import" | null;

// ─── Icons ───────────────────────────────────────────────────────────────────

function GitHubIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="currentColor">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
    </svg>
  );
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

const WIZARD_STATE_KEY = "astro:new-blueprint-wizard";

function slugify(str: string): string {
  return str.toLowerCase().replace(/[^a-z0-9-\s]/g, "").replace(/\s+/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "").slice(0, 64);
}

const STEPS: { id: Step; label: string; description: string }[] = [
  { id: "setup", label: "Identity", description: "Provide a name and image for your agent." },
  { id: "source", label: "Starting point", description: "Start from scratch or bring in existing code." },
  { id: "publishing", label: "Initializing...", description: "Creating your blueprint in the registry." },
  { id: "review", label: "Review", description: "Your blueprint is ready." },
];

// ─── Main ────────────────────────────────────────────────────────────────────

function NewBlueprintContent() {
  const { personalAccount, accounts } = useAuth();
  const userAccount = personalAccount?.name ?? "user";
  const sortedAccounts = useMemo(
    () => [...accounts].sort((a, b) => a.type === "personal" ? -1 : b.type === "personal" ? 1 : a.name.localeCompare(b.name)),
    [accounts],
  );

  const [searchParams, setSearchParams] = useSearchParams();

  // Step state
  const [activeStep, setActiveStep] = useState<Step>("setup");
  const [completedSteps, setCompletedSteps] = useState<Set<Step>>(new Set());
  const isAlreadyPublished = completedSteps.has("publishing");

  // Form state
  const [name, setName] = useState("");
  const [selectedOrg, setSelectedOrg] = useState(userAccount);
  const [visibility, setVisibility] = useState<"public" | "private">("private");
  const [avatarFile, setAvatarFile] = useState<Blob | null>(null);
  const [avatarPreviewUrl, setAvatarPreviewUrl] = useState<string | null>(null);
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const slug = useMemo(() => slugify(name), [name]);

  // Source step state
  const [sourcePath, setSourcePath] = useState<SourcePath>(null);
  const [githubConnected, setGithubConnected] = useState(false);
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [selectedBranch, setSelectedBranch] = useState("main");
  const [scanResult, setScanResult] = useState<"scanning" | "found" | "not-found" | "build-failed" | null>(null);

  const createBlueprint = useCreateBlueprint(selectedOrg);
  const uploadAvatar = useUploadBlueprintAvatar();
  const isCreatingBlueprint = createBlueprint.isPending || uploadAvatar.isPending;
  const navigate = useNavigate();

  const accountConnect = useGitHubAccountConnect(selectedOrg);
  const accountRepos = useGitHubAccountRepos(selectedOrg, { enabled: githubConnected });
  const githubLink = useGitHubLink(selectedOrg, slug);
  const accountScan = useGitHubAccountScan(selectedOrg);
  const rebuild = useGitHubRebuild(selectedOrg, slug);

  // Poll GitHub status while waiting for a build triggered by the scan fast-path.
  const { data: buildStatus } = useGitHubStatus(selectedOrg, slug, {
    enabled: activeStep === "publishing" && scanResult === "found",
    refetchInterval: 3000,
  });

  // Once the build completes on the scan fast-path, advance to review.
  useEffect(() => {
    if (activeStep !== "publishing" || scanResult !== "found") return;
    const status = buildStatus?.builds[0]?.status;
    if (status === "registered") {
      setCompletedSteps(prev => { const s = new Set(prev); s.add("publishing"); return s; });
      setActiveStep("review");
    } else if (status === "failed") {
      setScanResult("build-failed");
      setCompletedSteps(prev => { const s = new Set(prev); s.add("publishing"); return s; });
      setActiveStep("review");
    }
  }, [buildStatus, activeStep, scanResult]);

  // Restore wizard state when returning from GitHub OAuth
  useEffect(() => {
    if (searchParams.get("github_connected") !== "true") return;
    const saved = sessionStorage.getItem(WIZARD_STATE_KEY);
    if (saved) {
      try {
        const { name: n, selectedOrg: org, visibility: vis } = JSON.parse(saved) as { name: string; selectedOrg: string; visibility: "public" | "private" };
        setName(n);
        setSelectedOrg(org);
        setVisibility(vis);
        sessionStorage.removeItem(WIZARD_STATE_KEY);
      } catch { /* ignore */ }
    }
    setSourcePath("import");
    setGithubConnected(true);
    setActiveStep("source");
    setCompletedSteps(new Set<Step>(["setup"]));
    setSearchParams({}, { replace: true });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Proactive name availability check — fires while user is typing on the setup step
  const slugIsValid = slug.length >= 4 && /^[a-z]/.test(slug);
  const { data: existingBlueprint } = useBlueprint(selectedOrg, slug, {
    enabled: activeStep === "setup" && slugIsValid && !isAlreadyPublished,
    retry: false,
  });
  const nameIsTaken = !!existingBlueprint;

  // Poll for ast push — as soon as versions appear, route to the blueprint detail page.
  // isFetchedAfterMount prevents stale cache from a previously-deleted same-name blueprint
  // from triggering an immediate redirect before the fresh fetch returns.
  const { data: publishedBlueprint, isFetchedAfterMount } = useBlueprint(selectedOrg, slug, {
    enabled: activeStep === "review" && !!slug,
    refetchInterval: 5_000,
  });
  useEffect(() => {
    if (activeStep === "review" && isFetchedAfterMount && publishedBlueprint && publishedBlueprint.versions.length > 0 && sourcePath !== "import") {
      navigate(`/${selectedOrg}/${slug}`);
    }
  }, [activeStep, publishedBlueprint, isFetchedAfterMount, selectedOrg, slug, navigate, sourcePath]);

  const { setExperiment } = useExperiments();

  const handleGoToBlueprint = useCallback(() => {
    if (sourcePath === "import" && selectedRepo) {
      setExperiment("githubAutoBuild", true);
      sessionStorage.setItem(`astro:github-repo:${selectedOrg}/${slug}`, JSON.stringify({ repo: selectedRepo.full_name, branch: selectedBranch }));
    }
    navigate(`/${selectedOrg}/${slug}`);
  }, [sourcePath, selectedRepo, selectedOrg, slug, selectedBranch, navigate, setExperiment]);

  // Auto-route on the review step when the scan fast-path succeeded (build registered).
  useEffect(() => {
    if (activeStep !== "review" || scanResult !== "found") return;
    setExperiment("githubAutoBuild", true);
    const t = setTimeout(() => navigate(`/${selectedOrg}/${slug}`), 1500);
    return () => clearTimeout(t);
  }, [activeStep, scanResult, selectedOrg, slug, navigate, setExperiment]);

  // Revoke staged preview URL when it changes
  useEffect(() => {
    return () => {
      if (avatarPreviewUrl) URL.revokeObjectURL(avatarPreviewUrl);
    };
  }, [avatarPreviewUrl]);

  const handleStageAvatar = useCallback(async (blob: Blob): Promise<void> => {
    setAvatarFile(blob);
    setAvatarPreviewUrl(URL.createObjectURL(blob));
  }, []);

  const activeStepIndex = STEPS.findIndex((s) => s.id === activeStep);
  const reviewPanelRef = useRef<HTMLDivElement>(null);


  const handleContinueToSource = useCallback(() => {
    setCompletedSteps(prev => { const s = new Set(prev); s.add("setup"); return s; });
    setActiveStep("source");
  }, []);

  const handleGitHubConnect = useCallback(async () => {
    try {
      sessionStorage.setItem(WIZARD_STATE_KEY, JSON.stringify({ name, selectedOrg, visibility }));
      const res = await accountConnect.mutateAsync("/new/custom");
      if (res.connected) {
        // Token already exists via Pipes — skip OAuth, go straight to repo selection
        sessionStorage.removeItem(WIZARD_STATE_KEY);
        setSourcePath("import");
        setGithubConnected(true);
      } else if (res.redirect_url) {
        window.location.href = res.redirect_url;
      }
    } catch {
      sessionStorage.removeItem(WIZARD_STATE_KEY);
    }
  }, [accountConnect, name, selectedOrg, visibility]);

  const handlePublish = useCallback(async () => {
    if (isCreatingBlueprint) return;
    setCompletedSteps(prev => { const s = new Set(prev); s.add("source"); return s; });
    setActiveStep("publishing");

    try {
      // 1. Create blueprint + upload avatar (with 2s minimum for UX).
      await Promise.all([
        (async () => {
          if (!isAlreadyPublished) {
            await createBlueprint.mutateAsync({ name: slug, visibility });
          }
          if (avatarFile) {
            await uploadAvatar.mutateAsync({ account: selectedOrg, name: slug, file: avatarFile }).catch(() => {});
            bustAgentAvatar(selectedOrg, slug, avatarFile);
          }
        })(),
        new Promise(resolve => setTimeout(resolve, 2000)),
      ]);

      if (sourcePath === "import" && selectedRepo) {
        // 2. Scan first — lightweight read, no connection needed.
        setScanResult("scanning");
        let found = false;
        try {
          const scan = await accountScan.mutateAsync({ repo: selectedRepo.full_name, branch: selectedBranch });
          found = scan.found;
        } catch { /* treat scan errors as not-found */ }

        // 3. Link (always — installs webhook for future pushes).
        await githubLink.mutateAsync({
          repo_full_name: selectedRepo.full_name,
          branch: selectedBranch,
        }).catch(() => {});

        if (found) {
          setScanResult("found");
          rebuild.mutate();
          // Stay on publishing — useEffect above will advance to review once build registers.
          return;
        }
        setScanResult("not-found");
      }

      setCompletedSteps(prev => { const s = new Set(prev); s.add("publishing"); return s; });
      setActiveStep("review");
    } catch {
      // error state shown in publishing card
    }
  }, [isCreatingBlueprint, isAlreadyPublished, createBlueprint, uploadAvatar, slug, visibility, selectedOrg, avatarFile, sourcePath, selectedRepo, selectedBranch, githubLink, accountScan, rebuild]);

  const avatarPreview = avatarPreviewUrl ? (
    <img src={avatarPreviewUrl} alt={slug} className="size-full object-cover" />
  ) : (
    <BlueprintIdentity account={selectedOrg} name={slug || selectedOrg} size={68} className="size-full" />
  );

  return (
    <div
      className="min-h-screen min-h-[100dvh] w-full"
      style={{
        background: "radial-gradient(ellipse at 50% 0%, hsla(40, 50%, 90%, 0.8) 0%, hsla(40, 30%, 96%, 0.4) 60%), radial-gradient(ellipse at 80% 100%, hsla(170, 40%, 88%, 0.5) 0%, transparent 60%), hsl(40, 20%, 97%)",
        backgroundRepeat: "no-repeat",
      }}
    >
      <div className="mx-auto max-w-[640px] px-6 pt-16 pb-40">
        <div className="mb-10 text-center">
          <h1 className="text-3xl font-bold tracking-tight">Setup your agent blueprint</h1>
          <p className="mt-2 text-muted-foreground max-w-[540px] mx-auto">A blueprint is your agent's packaged definition. Once it's pushed to the registry, you can deploy it as a running agent.</p>
        </div>

        {/* Progress bar */}
        <div className="flex gap-1.5 mb-10">
          {STEPS.map((step, i) => (
            <div key={step.id} className={cn(
              "h-1 flex-1 rounded-full transition-colors duration-500",
              i <= activeStepIndex ? "bg-primary" : "bg-border/40"
            )} />
          ))}
        </div>

        {/* Carousel — sliding flex row, height driven by content */}
        <div className="overflow-hidden rounded-xl">
          <div
            className="flex"
            style={{
              width: `${STEPS.length * 100}%`,
              transform: `translateX(-${activeStepIndex * (100 / STEPS.length)}%)`,
              transition: 'transform 0.55s cubic-bezier(0.16,1,0.3,1)',
            }}
          >
          {STEPS.map((step, i) => {
            return (
              <div
                key={step.id}
                style={{ width: `${100 / STEPS.length}%` }}
              >
                <div className="rounded-xl border border-border bg-white overflow-hidden flex flex-col min-h-[460px]">

                  {/* ── Source ── */}
                  {step.id === "source" && i <= activeStepIndex && (
                    <div className="flex flex-col flex-1">
                      <div className="flex-1 px-6 pt-6 pb-4 overflow-y-auto">
                        <p className="text-sm font-semibold mb-0.5">Starting point</p>
                        <p className="text-xs text-muted-foreground mb-5">Start from scratch or bring in existing code.</p>
                        <div className="space-y-3">

                          {/* Set up locally */}
                          <button
                            type="button"
                            onClick={() => setSourcePath("fresh")}
                            className={cn(
                              "flex w-full cursor-pointer items-start gap-4 rounded-lg border p-4 text-left transition-all",
                              sourcePath === "fresh" ? "border-primary/50 bg-card ring-1 ring-primary/20" : "border-border bg-card hover:border-primary/30"
                            )}
                          >
                            <div className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border-2 border-primary">
                              {sourcePath === "fresh" && <div className="size-2.5 rounded-full bg-primary" />}
                            </div>
                            <CommandLineIcon className="mt-0.5 size-5 shrink-0 text-foreground" />
                            <div className="flex-1">
                              <h3 className="text-sm font-semibold mb-0.5">Set up locally</h3>
                              <p className="text-xs leading-relaxed text-muted-foreground">
                                Scaffold a new agent with the Astro CLI and build it locally.
                              </p>
                            </div>
                          </button>

                          {/* Set up with GitHub */}
                          <div className={cn(
                            "w-full rounded-lg border transition-all",
                            sourcePath === "import" ? "border-primary/50 bg-card ring-1 ring-primary/20" : "border-border bg-card"
                          )}>
                            <button
                              type="button"
                              onClick={() => setSourcePath("import")}
                              className="w-full cursor-pointer text-left p-4"
                            >
                              <div className="flex items-start gap-4">
                                <div className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border-2 border-primary">
                                  {sourcePath === "import" && <div className="size-2.5 rounded-full bg-primary" />}
                                </div>
                                <GitHubIcon className="mt-0.5 size-5 shrink-0 text-foreground" />
                                <div className="flex-1">
                                  <h3 className="text-sm font-semibold mb-0.5">Set up with GitHub</h3>
                                  <p className="text-xs leading-relaxed text-muted-foreground">
                                    Connect a repo — any git push will automatically build and push your agent.
                                  </p>
                                </div>
                              </div>
                            </button>

                            {sourcePath === "import" && (
                              <div className="border-t border-border px-4 pb-4 pt-3">
                                {!githubConnected ? (
                                  <div className="space-y-2">
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      className="gap-2"
                                      onClick={handleGitHubConnect}
                                      disabled={accountConnect.isPending}
                                    >
                                      {accountConnect.isPending
                                        ? <ArrowPathIcon className="size-4 animate-spin" />
                                        : <GitHubIcon className="size-4" />
                                      }
                                      {accountConnect.isPending ? "Connecting..." : "Connect GitHub"}
                                    </Button>
                                    {accountConnect.isError && (
                                      <p className="text-xs text-destructive">Failed to connect. Please try again.</p>
                                    )}
                                  </div>
                                ) : (
                                  <div className="space-y-3">
                                    <p className="inline-flex items-center gap-1.5 text-xs text-foreground">
                                      <CheckCircleIcon className="size-3.5 text-green-700" />
                                      GitHub connected
                                    </p>
                                    <Select
                                      value={selectedRepo?.full_name ?? ""}
                                      onValueChange={(value) => {
                                        const repo = accountRepos.data?.repos.find(r => r.full_name === value) ?? null;
                                        setSelectedRepo(repo);
                                        setSelectedBranch(repo?.default_branch || "main");
                                      }}
                                    >
                                      <SelectTrigger>
                                        <SelectValue placeholder={accountRepos.isLoading ? "Loading repositories..." : "Select a repository"} />
                                      </SelectTrigger>
                                      <SelectContent>
                                        {accountRepos.data?.repos.map((repo) => (
                                          <SelectItem key={repo.full_name} value={repo.full_name}>
                                            <span className="flex items-center gap-2">
                                              <GitHubIcon className="size-3.5 shrink-0 text-muted-foreground" />
                                              {repo.full_name}
                                              {repo.private && <span className="text-[10px] text-muted-foreground">private</span>}
                                            </span>
                                          </SelectItem>
                                        ))}
                                      </SelectContent>
                                    </Select>
                                    {selectedRepo && (
                                      <Select value={selectedBranch} onValueChange={setSelectedBranch}>
                                        <SelectTrigger>
                                          <SelectValue />
                                        </SelectTrigger>
                                        <SelectContent>
                                          <SelectItem value="main">main</SelectItem>
                                          <SelectItem value="master">master</SelectItem>
                                          {selectedRepo.default_branch && !["main", "master"].includes(selectedRepo.default_branch) && (
                                            <SelectItem value={selectedRepo.default_branch}>
                                              {selectedRepo.default_branch}
                                            </SelectItem>
                                          )}
                                        </SelectContent>
                                      </Select>
                                    )}
                                  </div>
                                )}
                              </div>
                            )}
                          </div>

                        </div>
                      </div>
                      <div className="border-t border-border px-6 py-4 flex items-center justify-between">
                        <Button variant="outline" size="sm" onClick={() => setActiveStep("setup")}>Back</Button>
                        <Button
                          size="sm"
                          onClick={handlePublish}
                          disabled={
                            isCreatingBlueprint ||
                            !sourcePath ||
                            (sourcePath === "import" && (!githubConnected || !selectedRepo))
                          }
                        >
                          {isCreatingBlueprint ? (
                            <span className="inline-flex items-center gap-1.5">
                              <ArrowPathIcon className="size-4 animate-spin" />
                              Creating...
                            </span>
                          ) : "Create blueprint"}
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* ── Publishing ── */}
                  {step.id === "publishing" && i <= activeStepIndex && (
                    <div className="flex flex-1 flex-col items-center justify-center gap-6 px-6 py-12">
                      <div className="relative flex items-center justify-center">
                        <div className="absolute size-24 rounded-full bg-primary/10 animate-ping" style={{ animationDuration: "1.6s" }} />
                        <div className="absolute size-20 rounded-full bg-primary/15 animate-ping" style={{ animationDuration: "1.6s", animationDelay: "0.4s" }} />
                        <div className="relative z-10 size-16 overflow-hidden rounded-2xl border border-border shadow-sm">
                          {avatarPreviewUrl ? (
                            <img src={avatarPreviewUrl} alt={slug} className="size-full object-cover" />
                          ) : slug ? (
                            <BlueprintIdentity account={selectedOrg} name={slug} size={64} className="size-full" />
                          ) : (
                            <div className="flex size-full items-center justify-center bg-muted">
                              <ArrowPathIcon className="size-7 animate-spin text-primary" />
                            </div>
                          )}
                        </div>
                      </div>
                      <div className="text-center">
                        <p className="text-sm font-semibold">
                          {scanResult === "scanning"
                            ? `Scanning ${selectedRepo?.full_name ?? "repo"}…`
                            : scanResult === "found"
                            ? `Building ${slug}…`
                            : `Initializing ${slug || "your agent"}…`}
                        </p>
                        <p className="mt-1.5 text-xs text-muted-foreground max-w-[280px]">
                          {scanResult === "scanning"
                            ? "Looking for astropods.yml — we'll kick off a build if we find one."
                            : scanResult === "found"
                            ? "Found astropods.yml. Building your agent image and pushing to the registry."
                            : "Registering your blueprint in the registry."}
                        </p>
                        <p className="mt-2 font-mono text-xs text-muted-foreground/60">{selectedOrg}/{slug}</p>
                      </div>
                      <div className="flex gap-2">
                        {[0, 1, 2].map((j) => (
                          <div key={j} className="size-2 rounded-full bg-primary/60 animate-bounce" style={{ animationDelay: `${j * 0.15}s` }} />
                        ))}
                      </div>
                    </div>
                  )}

                  {/* ── Create identity ── */}
                  {step.id === "setup" && i <= activeStepIndex && (
                    <div className="flex flex-col flex-1">
                      <div className="flex-1 px-6 pt-6 pb-4 overflow-y-auto">
                        <p className="text-sm font-semibold mb-0.5">{step.label}</p>
                        <p className="text-xs text-muted-foreground mb-5">{step.description}</p>
                        <div className="space-y-5">
                          <div className="flex items-start gap-4">
                            {/* Avatar — deterministic by default, click to upload */}
                            <button
                              type="button"
                              aria-label="Edit agent avatar"
                              className="group relative shrink-0 cursor-pointer rounded-sm overflow-hidden border border-border size-[68px]"
                              onClick={() => setAvatarDialogOpen(true)}
                              disabled={isAlreadyPublished}
                            >
                              {avatarPreview}
                              <div className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
                                <Camera className="size-5 text-white" />
                              </div>
                            </button>
                            <AvatarUploadDialog
                              open={avatarDialogOpen}
                              onOpenChange={setAvatarDialogOpen}
                              onUpload={handleStageAvatar}
                              isPending={false}
                              title="Upload blueprint image"
                              cropShape="rect"
                            />
                            <div className="flex-1">
                              <Label size="md">Name <span className="text-destructive">*</span></Label>
                              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="my-agent" autoFocus disabled={isAlreadyPublished} />
                              {isAlreadyPublished ? (
                                <p className="mt-1.5 pl-4 text-xs text-muted-foreground">Published as <span className="font-mono text-foreground">{selectedOrg}/{slug}</span></p>
                              ) : name.trim().length > 0 && slug.length > 0 && (
                                slug.length < 4
                                  ? <p className="mt-1.5 pl-4 text-xs text-amber-600">Name must be at least 4 characters</p>
                                  : !/^[a-z]/.test(slug)
                                    ? <p className="mt-1.5 pl-4 text-xs text-amber-600">Name must start with a letter</p>
                                    : nameIsTaken
                                      ? <p className="mt-1.5 pl-4 text-xs text-destructive"><span className="font-mono">{selectedOrg}/{slug}</span> already exists</p>
                                      : <p className="mt-1.5 pl-4 text-xs text-muted-foreground">Will be created as <span className="font-mono text-foreground">{selectedOrg}/{slug}</span></p>
                              )}
                            </div>
                          </div>
                          <div>
                            <Label size="md">Create in</Label>
                            <Select value={selectedOrg} onValueChange={setSelectedOrg} disabled={isAlreadyPublished}>
                              <SelectTrigger className="[&>span]:flex [&>span]:items-center"><SelectValue /></SelectTrigger>
                              <SelectContent>
                                {sortedAccounts.map((a) => (
                                  <SelectItem key={a.id} value={a.name}>
                                    <span className="inline-flex items-center gap-2">
                                      {a.type === "personal" ? (
                                        <UserAvatar handle={a.name} name={a.display_name || a.name} className="size-[18px] shrink-0" />
                                      ) : (
                                        <span className="flex size-[18px] items-center justify-center rounded-md bg-accent shrink-0">
                                          <BuildingOffice2Icon className="size-2.5 text-muted-foreground" />
                                        </span>
                                      )}
                                      {a.display_name || a.name}
                                    </span>
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                          <div>
                            <Label size="md">Visibility</Label>
                            <div className="mt-1.5 flex gap-2">
                              <button
                                type="button"
                                onClick={() => setVisibility("private")}
                                className={cn(
                                  "flex flex-1 cursor-pointer items-start gap-3 rounded-lg border p-3 text-left transition-all",
                                  visibility === "private" ? "border-primary/50 bg-card ring-1 ring-primary/20" : "border-border bg-card hover:border-primary/30"
                                )}
                              >
                                <div className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border-2 border-primary">
                                  {visibility === "private" && <div className="size-2 rounded-full bg-primary" />}
                                </div>
                                <div>
                                  <div className="flex items-center gap-1.5 mb-0.5">
                                    <LockKeyhole className="size-3.5 text-foreground" />
                                    <span className="text-sm font-semibold">Private</span>
                                  </div>
                                  <p className="text-xs text-muted-foreground leading-relaxed">Only members with access can see this blueprint.</p>
                                </div>
                              </button>
                              <button
                                type="button"
                                onClick={() => setVisibility("public")}
                                className={cn(
                                  "flex flex-1 cursor-pointer items-start gap-3 rounded-lg border p-3 text-left transition-all",
                                  visibility === "public" ? "border-primary/50 bg-card ring-1 ring-primary/20" : "border-border bg-card hover:border-primary/30"
                                )}
                              >
                                <div className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border-2 border-primary">
                                  {visibility === "public" && <div className="size-2 rounded-full bg-primary" />}
                                </div>
                                <div>
                                  <div className="flex items-center gap-1.5 mb-0.5">
                                    <Globe className="size-3.5 text-foreground" />
                                    <span className="text-sm font-semibold">Public</span>
                                  </div>
                                  <p className="text-xs text-muted-foreground leading-relaxed">Anyone can discover and deploy their own variant of your blueprint.</p>
                                </div>
                              </button>
                            </div>
                          </div>
                        </div>
                      </div>
                      <div className="border-t border-border px-6 py-4 flex items-center justify-end">
                        <Button size="sm" onClick={handleContinueToSource} disabled={slug.length < 4 || !/^[a-z]/.test(slug) || nameIsTaken}>
                          Continue
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* ── Review ── */}
                  {step.id === "review" && i <= activeStepIndex && (
                    <div className="flex flex-col flex-1">
                      <div ref={reviewPanelRef} className="relative flex flex-1 flex-col items-center justify-center bg-muted/30 px-6 py-8 gap-4 overflow-hidden">
                        {scanResult === "found" && <LiveRevealConfetti containerRef={reviewPanelRef} />}
                        <div className="flex items-center gap-2">
                          {scanResult === "build-failed" ? (
                            <div className="flex size-5 items-center justify-center rounded-full bg-amber-100 text-amber-700 shrink-0">
                              <svg viewBox="0 0 12 12" className="size-3" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <path d="M6 2v5M6 9.5v.5" />
                              </svg>
                            </div>
                          ) : (
                            <div className="flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground shrink-0">
                              <svg viewBox="0 0 12 12" className="size-3" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <path d="M2.5 6l2.5 2.5 4.5-5" />
                              </svg>
                            </div>
                          )}
                          <span className="text-base font-semibold text-foreground">
                            {scanResult === "found"
                              ? "Blueprint registered and built!"
                              : scanResult === "build-failed" || scanResult === "not-found"
                              ? "Blueprint registered, repo connected"
                              : "Blueprint registered!"}
                          </span>
                        </div>
                        <div className="relative size-20 overflow-hidden rounded-2xl border border-border">
                          {avatarPreviewUrl
                            ? <img src={avatarPreviewUrl} alt={slug} className="size-full object-cover" />
                            : <BlueprintIdentity account={selectedOrg} name={slug} size={80} className="size-full" />
                          }
                          <div
                            className="absolute left-0 right-0 h-[2px] opacity-80"
                            style={{
                              background: "linear-gradient(90deg, transparent, var(--color-teal-500), transparent)",
                              animation: "scanLine 2.5s ease-in-out infinite",
                              boxShadow: "0 0 12px 2px color-mix(in oklch, var(--color-teal-500) 30%, transparent)",
                            }}
                          />
                          <div className="absolute top-1.5 left-1.5 size-3 border-t-2 border-l-2 border-teal-500/50 rounded-tl-sm" />
                          <div className="absolute top-1.5 right-1.5 size-3 border-t-2 border-r-2 border-teal-500/50 rounded-tr-sm" />
                          <div className="absolute bottom-1.5 left-1.5 size-3 border-b-2 border-l-2 border-teal-500/50 rounded-bl-sm" />
                          <div className="absolute bottom-1.5 right-1.5 size-3 border-b-2 border-r-2 border-teal-500/50 rounded-br-sm" />
                        </div>
                        <div className="text-center">
                          <p className="text-sm font-semibold">{slug || "my-agent"}</p>
                          <p className="mt-0.5 font-mono text-xs text-muted-foreground/60">{selectedOrg}/{slug}</p>
                        </div>
                        <p className="text-xs text-muted-foreground text-center max-w-[300px]">
                          {scanResult === "found"
                            ? "Your agent image is in the registry. Redirecting you now…"
                            : scanResult === "build-failed"
                            ? "Something went wrong during the build. Head to your blueprint page to see the error and fix it."
                            : scanResult === "not-found"
                            ? `We didn't find an astropods.yml in ${selectedRepo?.full_name ?? "your repo"}. Push one to trigger your first build — we'll pick it up automatically.`
                            : `Install the Astro CLI, run ast init ${slug}, then ast push to get your first image into the registry.`}
                        </p>
                      </div>
                      <div className="border-t border-border flex items-center justify-between px-6 py-4">
                        <div className="flex items-center gap-3">
                          <div className="flex size-5 shrink-0 items-center justify-center rounded-full border-2 border-dashed border-stone-300" />
                          <div>
                            <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Up next</p>
                            <p className="text-sm text-foreground">
                              {scanResult === "found"
                                ? "View your blueprint"
                                : scanResult === "build-failed"
                                ? "Fix the build error"
                                : scanResult === "not-found"
                                ? "Add astropods.yml to your repo"
                                : "Set up your agent in code"}
                            </p>
                          </div>
                        </div>
                        <Button size="sm" onClick={handleGoToBlueprint}>
                          {sourcePath === "import" ? "View blueprint →" : "Continue setup →"}
                        </Button>
                      </div>
                      <style>{`
                        @keyframes scanLine {
                          0%, 100% { top: 10%; }
                          50% { top: 85%; }
                        }
                      `}</style>
                    </div>
                  )}

                </div>
              </div>
            );
          })}
          </div>
        </div>
      </div>
    </div>
  );
}

export default function NewBlueprint() {
  return <NewBlueprintContent />;
}
