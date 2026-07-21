import { useState, useCallback, useMemo, useEffect, useRef, useReducer } from "react";
import { useAuth } from "../lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useCreateBlueprint, useUploadBlueprintAvatar, useBlueprint, useGitHubAccountConnect, useGitHubAccountStatus, useGitHubLink, useGitHubAccountScan, useGitHubRebuild } from "@/api/queries";
import { bustAgentAvatar } from "@/lib/avatar-bust";
import { repoBase, repoSubPath } from "@/lib/github-utils";
import { Link, useNavigate, useSearchParams, type MetaFunction } from "react-router";
import { ApiRequestError } from "@/lib/api";
import { accountSettingsPath } from "@/lib/settings-paths";
import { useAccountUsage } from "@/api/queries/usage";
import { RequestIncreaseDialog } from "@/components/RequestIncreaseDialog";
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
import { Camera, Check } from "lucide-react";
import {
  ArrowPathIcon,
  BuildingOffice2Icon,
  CheckCircleIcon,
  CommandLineIcon,
} from "@heroicons/react/24/outline";
import { UserAvatar } from "@/components/UserAvatar";
import { Globe, LockKeyhole } from "lucide-react";
import { LiveRevealConfetti } from "@/components/ui/LiveRevealConfetti";
import { GitHubIcon } from "@/components/ui/svgs/githubIcon";
import { RepoPicker, type RepoPickerValue } from "@/components/new-blueprint/RepoPicker";
import { LinkConfirmDialog } from "@/components/new-blueprint/LinkConfirmDialog";

export const meta: MetaFunction = () => [{ title: "New Agent | Astro" }];

// ─── Types ───────────────────────────────────────────────────────────────────

type Step = "setup" | "source" | "publishing" | "review";
type SourcePath = "fresh" | "import" | null;

// ─── Helpers ─────────────────────────────────────────────────────────────────

const WIZARD_STATE_KEY = "astro:new-blueprint-wizard";

type OAuthReturn = {
  login: string | undefined;
  savedWizard: { name: string; selectedOrg: string; visibility: "public" | "private" } | null;
};

// Read OAuth callback state synchronously from the URL and sessionStorage.
// Called as a lazy useState initializer so state is correct on the very first render —
// the carousel starts at the source step with no CSS transition at all.
function readOAuthReturn(): OAuthReturn | null {
  if (typeof window === "undefined") return null;
  const params = new URLSearchParams(window.location.search);
  if (params.get("github_connected") !== "true") return null;
  const login = params.get("github_login") ?? undefined;
  let savedWizard: OAuthReturn["savedWizard"] = null;
  try {
    const saved = sessionStorage.getItem(WIZARD_STATE_KEY);
    if (saved) savedWizard = JSON.parse(saved);
  } catch { /* ignore */ }
  return { login, savedWizard };
}

function slugify(str: string): string {
  return str.toLowerCase().replace(/[^a-z0-9-\s]/g, "").replace(/\s+/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "").slice(0, 64);
}

const STEPS: { id: Step; label: string; description: string }[] = [
  { id: "setup", label: "Identity", description: "Provide a name and image for your agent." },
  { id: "source", label: "Starting point", description: "Start from scratch or bring in existing code." },
  { id: "publishing", label: "Initializing...", description: "Creating your blueprint in the registry." },
  { id: "review", label: "Review", description: "Your blueprint is ready." },
];

// ─── Source step reducer ──────────────────────────────────────────────────────

type SourceState = {
  sourcePath: SourcePath;
  githubConnected: boolean;
  scanResult: "scanning" | "found" | "not-found" | null;
};

type SourceAction =
  | { type: "SET_SOURCE_PATH"; path: SourcePath }
  | { type: "GITHUB_CONNECTED" }
  | { type: "SET_SCAN_RESULT"; result: "scanning" | "found" | "not-found" | null };

const initialSourceState: SourceState = {
  sourcePath: null,
  githubConnected: false,
  scanResult: null,
};

function sourceReducer(state: SourceState, action: SourceAction): SourceState {
  switch (action.type) {
    case "SET_SOURCE_PATH":
      return { ...state, sourcePath: action.path, scanResult: null };
    case "GITHUB_CONNECTED":
      return { ...state, sourcePath: "import", githubConnected: true };
    case "SET_SCAN_RESULT":
      return { ...state, scanResult: action.result };
  }
}

// ─── Validation ────────────────────────────────────────────────────────────────

// Per-step correctness rules: each returns field → message, empty when valid.
// The submit gate and the setup step's proactive hints share these.
type FieldErrors = Record<string, string>;

function validateSetup(account: string, slug: string, nameIsTaken: boolean): FieldErrors {
  if (slug.length === 0) return { name: "Name is required" };
  if (slug.length < 4) return { name: "Name must be at least 4 characters" };
  if (!/^[a-z]/.test(slug)) return { name: "Name must start with a letter" };
  if (nameIsTaken) return { name: `${account}/${slug} already exists` };
  return {};
}

function validateSource(sourcePath: SourcePath, isGitHubConnected: boolean, repoFullName: string | null): FieldErrors {
  if (!sourcePath) return { source: "Choose how you'd like to start" };
  if (sourcePath === "import" && !isGitHubConnected) return { source: "Connect GitHub to continue" };
  if (sourcePath === "import" && !repoFullName) return { source: "Select a repository" };
  return {};
}

// ─── Main ────────────────────────────────────────────────────────────────────

function NewBlueprintContent() {
  const { personalAccount, accounts, organizationId, switchOrg } = useAuth();
  const { activeAccount } = useActiveAccount();
  const userAccount = personalAccount?.name ?? "user";
  const sortedAccounts = useMemo(
    () => [...accounts].sort((a, b) => a.type === "personal" ? -1 : b.type === "personal" ? 1 : a.name.localeCompare(b.name)),
    [accounts],
  );

  const [, setSearchParams] = useSearchParams();

  // Read OAuth callback state once, synchronously, before the first render so the
  // carousel starts at the source step immediately — no CSS transition, no blank flash.
  const [oauthReturn] = useState<OAuthReturn | null>(readOAuthReturn);

  // Step state
  const [activeStep, setActiveStep] = useState<Step>(() => oauthReturn ? "source" : "setup");
  const [completedSteps, setCompletedSteps] = useState<Set<Step>>(() =>
    oauthReturn ? new Set<Step>(["setup"]) : new Set()
  );
  const isAlreadyPublished = completedSteps.has("publishing");
  const [publishError, setPublishError] = useState<{ message: string; usageUrl?: string } | null>(null);
  const [isBlueprintCreated, setIsBlueprintCreated] = useState(false);
  const [isPublishing, setIsPublishing] = useState(false);
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false);

  // Form state
  const [name, setName] = useState(() => oauthReturn?.savedWizard?.name ?? "");
  const [selectedOrg, setSelectedOrg] = useState(() => oauthReturn?.savedWizard?.selectedOrg ?? (activeAccount || userAccount));
  const [visibility, setVisibility] = useState<"public" | "private">(() => oauthReturn?.savedWizard?.visibility ?? "private");
  const [avatarFile, setAvatarFile] = useState<Blob | null>(null);
  const [avatarPreviewUrl, setAvatarPreviewUrl] = useState<string | null>(null);
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const [showLinkConfirm, setShowLinkConfirm] = useState(false);
  const slug = useMemo(() => slugify(name), [name]);

  // Submit-gate errors keyed by field — one store for the whole form.
  const [errors, setErrors] = useState<FieldErrors>({});
  const clearError = useCallback((field: string) => {
    setErrors(prev => {
      if (!(field in prev)) return prev;
      const next = { ...prev };
      delete next[field];
      return next;
    });
  }, []);
  // Replace the visible errors with a step's result; returns whether it passed.
  const gate = useCallback((fieldErrors: FieldErrors) => {
    setErrors(fieldErrors);
    return Object.keys(fieldErrors).length === 0;
  }, []);

  // Source step state
  const [{ sourcePath, githubConnected, scanResult }, dispatch] = useReducer(sourceReducer, initialSourceState);
  const [pickerValue, setPickerValue] = useState<RepoPickerValue>({ repoFullName: null, branch: "main" });

  const createBlueprint = useCreateBlueprint(selectedOrg);
  const uploadAvatar = useUploadBlueprintAvatar();
  const isCreatingBlueprint = createBlueprint.isPending || uploadAvatar.isPending;
  const navigate = useNavigate();

  const [githubLogin, setGithubLogin] = useState<string | undefined>(undefined);

  const accountConnect = useGitHubAccountConnect(selectedOrg);
  const githubLink = useGitHubLink(selectedOrg, slug);
  const accountScan = useGitHubAccountScan(selectedOrg);
  const rebuild = useGitHubRebuild(selectedOrg, slug);

  // Only load usage once we hit the agents quota, to populate the request dialog.
  const isQuotaError = !!publishError?.usageUrl;
  const { data: usageData } = useAccountUsage(selectedOrg, isQuotaError);
  const agentsMeter = usageData?.meters?.agents ?? { usage: 0, quota: undefined };

  // Proactively check whether the selected org is already connected to GitHub.
  // If so, the source step can skip the "Connect GitHub" intermediate click and
  // render the repo list directly. Only fetched once the user reaches the source step.
  const { data: accountStatus, isLoading: isStatusLoading } = useGitHubAccountStatus(selectedOrg, {
    enabled: activeStep === "source",
  });
  // Effective connection state: either the user just connected via OAuth/Pipes
  // (githubConnected) or the account already has a stored connection.
  const isGitHubConnected = githubConnected || !!accountStatus?.connected;
  const effectiveGithubLogin = githubLogin ?? accountStatus?.github_login;

  // Restore wizard state when returning from GitHub OAuth. The account is now
  // connected, so jump straight to the import path with the repo list visible.
  useEffect(() => {
    if (!oauthReturn) return;
    if (oauthReturn.login) setGithubLogin(oauthReturn.login);
    dispatch({ type: "GITHUB_CONNECTED" });
    sessionStorage.removeItem(WIZARD_STATE_KEY);
    setSearchParams({}, { replace: true });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Proactive name availability check — archived blueprints don't block reuse unless the name
  // is reserved (was ever public or deployed), in which case the server keeps it permanently.
  const slugIsValid = slug.length >= 4 && /^[a-z]/.test(slug);
  const { data: existingBlueprint } = useBlueprint(selectedOrg, slug, {
    enabled: activeStep === "setup" && slugIsValid && !isAlreadyPublished,
    retry: false,
  });
  const nameIsTaken = !!existingBlueprint && (!existingBlueprint.archived_at || !!existingBlueprint.name_reserved);

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

  const handleGoToBlueprint = useCallback(() => {
    navigate(`/${selectedOrg}/${slug}`);
  }, [selectedOrg, slug, navigate]);

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
    if (!gate(validateSetup(selectedOrg, slug, nameIsTaken))) return;
    setCompletedSteps(prev => { const s = new Set(prev); s.add("setup"); return s; });
    setActiveStep("source");
  }, [gate, selectedOrg, slug, nameIsTaken]);

  const handleSelectGitHub = useCallback(() => {
    dispatch({ type: "SET_SOURCE_PATH", path: "import" });
    clearError("source");
  }, [clearError]);

  const handleSelectLocal = useCallback(() => {
    dispatch({ type: "SET_SOURCE_PATH", path: "fresh" });
    setPickerValue({ repoFullName: null, branch: "main" });
    clearError("source");
  }, [clearError]);

  const handleBack = useCallback(() => {
    setActiveStep("setup");
  }, []);

  const handleGitHubConnect = useCallback(async () => {
    try {
      sessionStorage.setItem(WIZARD_STATE_KEY, JSON.stringify({ name, selectedOrg, visibility }));
      const res = await accountConnect.mutateAsync({ redirectTo: "/new/custom" });
      if (res.connected) {
        // Token already exists via Pipes — skip OAuth, go straight to repo selection
        sessionStorage.removeItem(WIZARD_STATE_KEY);
        if (res.github_login) setGithubLogin(res.github_login);
        dispatch({ type: "GITHUB_CONNECTED" });
      } else if (res.redirect_url) {
        window.location.href = res.redirect_url;
      }
    } catch {
      sessionStorage.removeItem(WIZARD_STATE_KEY);
    }
  }, [accountConnect, name, selectedOrg, visibility]);

  const handlePublish = useCallback(async () => {
    if (isPublishing) return;
    setIsPublishing(true);
    setPublishError(null);
    setCompletedSteps(prev => { const s = new Set(prev); s.add("source"); return s; });
    setActiveStep("publishing");

    try {
      // Scope the JWT to the selected org before writing, if not already scoped.
      const acct = accounts.find(a => a.name === selectedOrg);
      if (acct?.type === "organization" && acct.organization_id && acct.organization_id !== organizationId) {
        await switchOrg(acct.organization_id);
      }

      // 1. Create blueprint + upload avatar (with 2s minimum for UX).
      await Promise.all([
        (async () => {
          if (!isAlreadyPublished && !isBlueprintCreated) {
            await createBlueprint.mutateAsync({ name: slug, visibility });
            setIsBlueprintCreated(true);
          }
          if (avatarFile) {
            await uploadAvatar.mutateAsync({ account: selectedOrg, name: slug, file: avatarFile }).catch(() => {});
            bustAgentAvatar(selectedOrg, slug, avatarFile);
          }
        })(),
        new Promise(resolve => setTimeout(resolve, 2000)),
      ]);

      if (sourcePath === "import" && pickerValue.repoFullName) {
        // 2. Scan first — lightweight read, no connection needed.
        dispatch({ type: "SET_SCAN_RESULT", result: "scanning" });
        let found = false;
        try {
          const scan = await accountScan.mutateAsync({ repo: pickerValue.repoFullName, branch: pickerValue.branch, agentName: slug });
          found = scan.found;
        } catch { /* treat scan errors as not-found */ }

        // 3. Link (always — installs webhook for future pushes).
        await githubLink.mutateAsync({
          repo_full_name: pickerValue.repoFullName,
          branch: pickerValue.branch,
        });

        if (found) {
          dispatch({ type: "SET_SCAN_RESULT", result: "found" });
          // Await so the build row exists before navigating to the detail page.
          await rebuild.mutateAsync().catch(() => {});
        } else {
          dispatch({ type: "SET_SCAN_RESULT", result: "not-found" });
        }
      }

      setCompletedSteps(prev => { const s = new Set(prev); s.add("publishing"); return s; });
      setActiveStep("review");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Something went wrong. Please try again.";
      // Entitlement quota limits surface as HTTP 402; the entitlement middleware
      // sets error="Limit reached" (vs "Feature not available" for plan gaps).
      // Offer a scoped usage link + quota-increase dialog for the agents limit.
      const isQuotaLimit = err instanceof ApiRequestError && err.status === 402
        && err.code === "Limit reached";
      const usageUrl = isQuotaLimit ? accountSettingsPath(accounts, selectedOrg, "billing") : undefined;
      setPublishError({ message, usageUrl });
    } finally {
      setIsPublishing(false);
    }
  }, [isPublishing, isAlreadyPublished, isBlueprintCreated, createBlueprint, uploadAvatar, slug, visibility, selectedOrg, avatarFile, sourcePath, pickerValue, githubLink, accountScan, rebuild, accounts, organizationId, switchOrg]);

  const handleCreateOrConfirm = useCallback(() => {
    if (!gate(validateSource(sourcePath, isGitHubConnected, pickerValue.repoFullName))) return;
    if (sourcePath === "import") {
      setShowLinkConfirm(true);
    } else {
      handlePublish();
    }
  }, [gate, sourcePath, isGitHubConnected, pickerValue.repoFullName, handlePublish]);

  const handleConfirmAndPublish = useCallback(() => {
    setShowLinkConfirm(false);
    handlePublish();
  }, [handlePublish]);

  const avatarPreview = avatarPreviewUrl ? (
    <img src={avatarPreviewUrl} alt={slug} className="size-full object-cover" />
  ) : (
    <BlueprintIdentity account={selectedOrg} name={slug || selectedOrg} size={68} className="size-full" />
  );

  // Proactive name hint while typing, from the same rules the submit gate uses.
  const setupFieldErrors = validateSetup(selectedOrg, slug, nameIsTaken);

  return (
    <div className="dp-blueprint-bg flex flex-1 flex-col overflow-y-auto">
      <div className="mx-auto w-full max-w-[640px] px-6 pt-16 pb-6 flex flex-1 flex-col">
        <div className="mb-10 text-center">
          <h1 className="text-3xl font-bold tracking-tight text-foreground">Setup your agent blueprint</h1>
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

        {/* Carousel — sliding flex row, height driven by content.
            overflow-clip (not hidden) so the browser can't scroll the off-screen
            slides into view on focus, which would knock the active slide out of
            alignment (blank/offset steps). */}
        <div className="overflow-clip rounded-xl">
          <div
            className="flex"
            style={{
              width: `${STEPS.length * 100}%`,
              transform: `translateX(-${activeStepIndex * (100 / STEPS.length)}%)`,
              transition: 'transform 0.3s ease-out',
            }}
          >
          {STEPS.map((step, i) => {
            return (
              <div
                key={step.id}
                className="flex flex-col"
                style={{ width: `${100 / STEPS.length}%` }}
              >
                <div className="rounded-xl border border-border bg-card overflow-hidden flex flex-col" style={{ minHeight: 460 }}>

                  {/* ── Source ── */}
                  {step.id === "source" && i <= activeStepIndex && (
                    <div className="flex flex-col flex-1">

                      {/* Fixed header */}
                      <div className="px-6 pt-6 pb-4 shrink-0">
                        <p className="text-sm font-semibold mb-0.5">Starting point</p>
                        <p className="text-xs text-muted-foreground">Start from scratch or bring in existing code.</p>
                      </div>
                      <div className="border-b border-border shrink-0" />

                      {/* Content — no overflow-y-auto so inline dropdowns expand the card */}
                      <div className="flex-1">
                            <div className="px-6 py-4 space-y-3">

                              {/* Set up with GitHub */}
                              <div className={cn(
                                "w-full rounded-lg border transition-all",
                                sourcePath === "import" ? "border-primary bg-card ring-1 ring-primary/40" : "border-border bg-card"
                              )}>
                                <button
                                  type="button"
                                  onClick={handleSelectGitHub}
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
                                        Connect a repo. Any git push will automatically build and push your agent.
                                      </p>
                                    </div>
                                  </div>
                                </button>

                                {sourcePath === "import" && isStatusLoading && (
                                  <div className="flex items-center gap-2 border-t border-border px-4 pb-4 pt-3 text-xs text-muted-foreground">
                                    <ArrowPathIcon className="size-4 animate-spin" />
                                    Checking GitHub connection…
                                  </div>
                                )}

                                {sourcePath === "import" && !isGitHubConnected && !isStatusLoading && (
                                  <div className="border-t border-border px-4 pb-4 pt-3 space-y-2">
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
                                )}

                                {sourcePath === "import" && (
                                  <div className={cn(
                                    "grid transition-[grid-template-rows] duration-200 ease-out",
                                    isGitHubConnected ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
                                  )}>
                                    <div className="overflow-hidden">
                                      <div className="border-t border-border">
                                          <>
                                            <p className="inline-flex items-center gap-1.5 px-4 pt-3 text-xs text-foreground">
                                              <CheckCircleIcon className="size-3.5 text-success" />
                                              {effectiveGithubLogin ? `${effectiveGithubLogin} connected` : "GitHub connected"}
                                            </p>
                                            <RepoPicker
                                              account={selectedOrg}

                                              githubLogin={effectiveGithubLogin}
                                              enabled={isGitHubConnected}
                                              onChange={(v) => { setPickerValue(v); clearError("source"); }}
                                            />
                                          </>
                                      </div>
                                    </div>
                                  </div>
                                )}
                              </div>

                              {/* Set up locally */}
                              <button
                                type="button"
                                onClick={handleSelectLocal}
                                className={cn(
                                  "flex w-full cursor-pointer items-start gap-4 rounded-lg border p-4 text-left transition-all",
                                  sourcePath === "fresh" ? "border-primary bg-card ring-1 ring-primary/40" : "border-border bg-card hover:border-primary/30"
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

                            </div>
                      </div>

                      {/* Fixed footer */}
                      <div className="border-t border-border px-6 py-4 flex items-center justify-between shrink-0">
                        <Button variant="outline" size="sm" onClick={handleBack}>
                          Back
                        </Button>
                        <div className="flex items-center gap-3">
                          {errors.source && (
                            <p className="text-xs text-destructive">{errors.source}</p>
                          )}
                          <Button
                            size="sm"
                            onClick={handleCreateOrConfirm}
                            disabled={isCreatingBlueprint || isPublishing}
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

                      <LinkConfirmDialog
                        open={showLinkConfirm}
                        onOpenChange={setShowLinkConfirm}
                        avatarPreviewUrl={avatarPreviewUrl}
                        slug={slug}
                        name={name}
                        selectedOrg={selectedOrg}
                        repoBase={pickerValue.repoFullName ? repoBase(pickerValue.repoFullName) : null}
                        selectedBranch={pickerValue.branch}
                        subpath={(pickerValue.repoFullName && repoSubPath(pickerValue.repoFullName)) || undefined}
                        visibility={visibility}
                        isCreatingBlueprint={isCreatingBlueprint}
                        onConfirm={handleConfirmAndPublish}
                      />

                      {publishError?.usageUrl && (
                        <RequestIncreaseDialog
                          featureKey="agents"
                          label="Agents"
                          meter={agentsMeter}
                          account={selectedOrg}
                          open={quotaDialogOpen}
                          onOpenChange={setQuotaDialogOpen}
                        />
                      )}

                    </div>
                  )}

                  {/* ── Publishing ── */}
                  {step.id === "publishing" && i <= activeStepIndex && (
                    <div className="flex flex-1 flex-col">
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
                                <ArrowPathIcon className="size-7 animate-spin text-foreground-accent" />
                              </div>
                            )}
                          </div>
                        </div>
                        <div className="text-center">
                          <p className="text-sm font-semibold">
                            {scanResult === "scanning"
                              ? `Scanning ${pickerValue.repoFullName ?? "repo"}…`
                              : scanResult === "found"
                              ? `Building ${slug}…`
                              : `Initializing ${slug || "your agent"}…`}
                          </p>
                          <p className="mt-1.5 text-xs text-muted-foreground max-w-[280px]">
                            {scanResult === "scanning"
                              ? "Looking for astropods.yml. We'll kick off a build if we find one."
                              : "Registering your blueprint in the registry."}
                          </p>
                          <p className="mt-2 font-mono text-xs text-muted-foreground/60">{selectedOrg}/{slug}</p>
                        </div>
                        {publishError ? (
                          <div className="flex flex-col items-center gap-1.5 max-w-[280px] text-center">
                            <p className="text-xs text-destructive">{publishError.message}</p>
                            {publishError.usageUrl && (
                              <p className="text-xs text-muted-foreground">
                                <button
                                  type="button"
                                  className="font-medium text-foreground-accent underline underline-offset-2 cursor-pointer"
                                  onClick={() => setQuotaDialogOpen(true)}
                                >
                                  Request a quota increase
                                </button>{" "}
                                or{" "}
                                <Link
                                  to={publishError.usageUrl}
                                  className="font-medium text-foreground-accent underline underline-offset-2"
                                >
                                  review your billing in Settings
                                </Link>.
                              </p>
                            )}
                          </div>
                        ) : (
                          <div className="flex gap-2">
                            {[0, 1, 2].map((j) => (
                              <div key={j} className="size-2 rounded-full bg-primary/60 animate-bounce" style={{ animationDelay: `${j * 0.15}s` }} />
                            ))}
                          </div>
                        )}
                      </div>
                      <div className="border-t border-border px-6 py-4">
                        <span className={isPublishing ? "cursor-not-allowed" : undefined}>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={isPublishing}
                            onClick={() => setActiveStep("source")}
                          >
                            Back
                          </Button>
                        </span>
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
                              <Input value={name} onChange={(e) => { setName(e.target.value); clearError("name"); }} placeholder="my-agent" autoFocus disabled={isAlreadyPublished} />
                              {isAlreadyPublished ? (
                                <p className="mt-1.5 pl-4 text-xs text-muted-foreground">Published as <span className="font-mono text-foreground">{selectedOrg}/{slug}</span></p>
                              ) : errors.name ? (
                                <p className="mt-1.5 pl-4 text-xs text-destructive">{errors.name}</p>
                              ) : slug.length > 0 ? (
                                setupFieldErrors.name
                                  ? <p className="mt-1.5 pl-4 text-xs text-yellow-700 dark:text-yellow-400">{setupFieldErrors.name}</p>
                                  : <p className="mt-1.5 pl-4 text-xs text-muted-foreground">Will be created as <span className="font-mono text-foreground">{selectedOrg}/{slug}</span></p>
                              ) : null}
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
                                  visibility === "private" ? "border-primary bg-card ring-1 ring-primary/40" : "border-border bg-card hover:border-primary/30"
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
                                  visibility === "public" ? "border-primary bg-card ring-1 ring-primary/40" : "border-border bg-card hover:border-primary/30"
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
                        <Button size="sm" onClick={handleContinueToSource}>
                          Continue
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* ── Review ── */}
                  {step.id === "review" && i <= activeStepIndex && (
                    <div className="flex flex-col flex-1">
                      <div ref={reviewPanelRef} className="relative flex flex-1 flex-col items-center justify-center bg-muted/30 px-6 py-8 gap-4 overflow-hidden">
                        <LiveRevealConfetti containerRef={reviewPanelRef} />
                        <div className="flex items-center gap-2">
                          <div className="flex size-5 items-center justify-center rounded-full bg-success text-white dark:text-slate-950 shrink-0">
                            <Check className="size-3" />
                          </div>
                          <span className="text-base font-semibold text-foreground">
                            {scanResult === "found"
                              ? "astropods.yml found, build in progress"
                              : scanResult === "not-found"
                              ? "Blueprint registered, repo connected"
                              : "Blueprint registered!"}
                          </span>
                        </div>
                        <div className="relative size-20 overflow-hidden rounded-md border border-border">
                          {avatarPreviewUrl
                            ? <img src={avatarPreviewUrl} alt={slug} className="size-full object-cover" />
                            : <BlueprintIdentity account={selectedOrg} name={slug} size={80} className="size-full" />
                          }
                          <div className="dp-scan-line absolute left-0 right-0 h-[2px] mix-blend-overlay" />
                          <div className="dp-scan-corner absolute top-1.5 left-1.5 size-3 border-t border-l mix-blend-overlay rounded-tl-sm" />
                          <div className="dp-scan-corner absolute top-1.5 right-1.5 size-3 border-t border-r mix-blend-overlay rounded-tr-sm" />
                          <div className="dp-scan-corner absolute bottom-1.5 left-1.5 size-3 border-b border-l mix-blend-overlay rounded-bl-sm" />
                          <div className="dp-scan-corner absolute bottom-1.5 right-1.5 size-3 border-b border-r mix-blend-overlay rounded-br-sm" />
                        </div>
                        <div className="text-center">
                          <p className="text-sm font-semibold">{slug || "my-agent"}</p>
                          <p className="mt-0.5 font-mono text-xs text-muted-foreground/60">{selectedOrg}/{slug}</p>
                        </div>
                        <p className="text-xs text-muted-foreground text-center max-w-[300px]">
                          {scanResult === "found"
                            ? "Build kicked off. Head to your blueprint to track progress in the GitHub sidebar."
                            : scanResult === "not-found"
                            ? `We didn't find an astropods.yml in ${pickerValue.repoFullName ?? "your repo"}. Push one to trigger your first build. We'll pick it up automatically.`
                            : `Install the Astro CLI, run ast init ${slug}, then ast push to get your first image into the registry.`}
                        </p>
                      </div>
                      <div className="border-t border-border flex items-center justify-between px-6 py-4">
                        <div className="flex items-center gap-3">
                          <div className="flex size-5 shrink-0 items-center justify-center rounded-full border-2 border-dashed border-border" />
                          <div>
                            <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Up next</p>
                            <p className="text-sm text-foreground">
                              {scanResult === "found"
                                ? "Track your build"
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
