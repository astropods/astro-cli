import { useState, useCallback, useMemo, useEffect } from "react";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { useAuth } from "../lib/auth";
import { useCreateBlueprint, useUploadBlueprintAvatar, useBlueprint } from "@/api/queries";
import { bustAgentAvatar } from "@/lib/avatar-bust";
import { useNavigate } from "react-router";
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
} from "@heroicons/react/24/outline";
import { UserAvatar } from "@/components/UserAvatar";
import { Globe, LockKeyhole } from "lucide-react";

// ─── Types ───────────────────────────────────────────────────────────────────

type Step = "setup" | "publishing" | "review";

// ─── Helpers ─────────────────────────────────────────────────────────────────

function slugify(str: string): string {
  return str.toLowerCase().replace(/[^a-z0-9-\s]/g, "").replace(/\s+/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "").slice(0, 64);
}

const STEPS: { id: Step; label: string; description: string }[] = [
  { id: "setup", label: "Identity", description: "Provide a name and image for your agent." },
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

  const createBlueprint = useCreateBlueprint(selectedOrg);
  const uploadAvatar = useUploadBlueprintAvatar();
  const isCreatingBlueprint = createBlueprint.isPending || uploadAvatar.isPending;
  const navigate = useNavigate();

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
    if (activeStep === "review" && isFetchedAfterMount && publishedBlueprint && publishedBlueprint.versions.length > 0) {
      navigate(`/${selectedOrg}/${slug}`);
    }
  }, [activeStep, publishedBlueprint, isFetchedAfterMount, selectedOrg, slug, navigate]);

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


  const handlePublish = useCallback(async () => {
    if (isCreatingBlueprint) return;
    setCompletedSteps(prev => { const s = new Set(prev); s.add("setup"); return s; });
    setActiveStep("publishing");

    try {
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
      setCompletedSteps(prev => { const s = new Set(prev); s.add("publishing"); return s; });
      setActiveStep("review");
    } catch {
      // error state shown in publishing card
    }
  }, [isCreatingBlueprint, isAlreadyPublished, createBlueprint, uploadAvatar, slug, visibility, selectedOrg, avatarFile]);

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
                        <p className="text-sm font-semibold">Initializing {slug || "your agent"}…</p>
                        <p className="mt-0.5 font-mono text-xs text-muted-foreground">{selectedOrg}/{slug}</p>
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
                                <p className="mt-1.5 text-xs text-muted-foreground">Published as <span className="font-mono text-foreground">{selectedOrg}/{slug}</span></p>
                              ) : name.trim().length > 0 && slug.length > 0 && (
                                slug.length < 4
                                  ? <p className="mt-1.5 text-xs text-amber-600">Name must be at least 4 characters</p>
                                  : !/^[a-z]/.test(slug)
                                    ? <p className="mt-1.5 text-xs text-amber-600">Name must start with a letter</p>
                                    : nameIsTaken
                                      ? <p className="mt-1.5 text-xs text-destructive"><span className="font-mono">{selectedOrg}/{slug}</span> already exists</p>
                                      : <p className="mt-1.5 text-xs text-muted-foreground">Will be created as <span className="font-mono text-foreground">{selectedOrg}/{slug}</span></p>
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
                        <Button size="sm" onClick={handlePublish} disabled={slug.length < 4 || !/^[a-z]/.test(slug) || nameIsTaken}>
                          Create blueprint
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* ── Review ── */}
                  {step.id === "review" && i <= activeStepIndex && (
                    <div className="flex flex-col flex-1">
                      <div className="flex flex-1 flex-col items-center justify-center bg-muted/30 px-6 py-8 gap-4">
                        <div className="flex items-center gap-2">
                          <div className="flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground shrink-0">
                            <svg viewBox="0 0 12 12" className="size-3" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                              <path d="M2.5 6l2.5 2.5 4.5-5" />
                            </svg>
                          </div>
                          <span className="text-base font-semibold text-foreground">Blueprint initialized</span>
                        </div>
                        <div className="relative size-20 overflow-hidden rounded-2xl border border-border">
                          {avatarPreviewUrl ? (
                            <img src={avatarPreviewUrl} alt={slug} className="size-full object-cover" />
                          ) : (
                            <BlueprintIdentity account={selectedOrg} name={slug} size={80} className="size-full" />
                          )}
                          {/* Scan line */}
                          <div
                            className="absolute left-0 right-0 h-[2px] opacity-80"
                            style={{
                              background: "linear-gradient(90deg, transparent, var(--color-teal-500), transparent)",
                              animation: "scanLine 2.5s ease-in-out infinite",
                              boxShadow: "0 0 12px 2px color-mix(in oklch, var(--color-teal-500) 30%, transparent)",
                            }}
                          />
                          {/* Corner brackets */}
                          <div className="absolute top-1.5 left-1.5 size-3 border-t-2 border-l-2 border-teal-500/50 rounded-tl-sm" />
                          <div className="absolute top-1.5 right-1.5 size-3 border-t-2 border-r-2 border-teal-500/50 rounded-tr-sm" />
                          <div className="absolute bottom-1.5 left-1.5 size-3 border-b-2 border-l-2 border-teal-500/50 rounded-bl-sm" />
                          <div className="absolute bottom-1.5 right-1.5 size-3 border-b-2 border-r-2 border-teal-500/50 rounded-br-sm" />
                        </div>
                        <div className="text-center">
                          <p className="text-sm font-semibold">{slug || "my-agent"}</p>
                          <p className="mt-0.5 font-mono text-xs text-muted-foreground">{selectedOrg}/{slug}</p>
                        </div>
                      </div>

                      <div className="border-t border-border flex items-center justify-between px-6 py-4">
                        <div className="flex items-center gap-3">
                          <div className="flex size-5 shrink-0 items-center justify-center rounded-full border-2 border-dashed border-stone-300" />
                          <div>
                            <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Up next</p>
                            <p className="text-sm text-foreground">Set up your agent in code</p>
                          </div>
                        </div>
                        <Button size="sm" onClick={() => navigate(`/${selectedOrg}/${slug}`)}>
                          Continue setup →
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
  return (
    <ProtectedRoute>
      <NewBlueprintContent />
    </ProtectedRoute>
  );
}
