import { useState } from "react";
import { useParams, type MetaFunction } from "react-router";
import { ChevronDown, ChevronRight, KeyRound, Plus, Trash2 } from "lucide-react";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CopyButton } from "@/components/ui/copy-button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useAppScopes, useApps, useCreateApp, useCreateAppSecret, useDeleteApp, useDeleteAppSecret } from "@/api/queries";
import { AppScopePicker } from "@/components/settings/AppScopePicker";
import { getApiErrorMessage, type MachineApp, type NewAppSecret } from "@/lib/api";
import { cn } from "@/lib/utils";

export const meta: MetaFunction = () => [
  { title: "OAuth apps - Organization Settings | Astro" },
];

const COLUMN_COUNT = 4;

export default function OrgAppsSettings() {
  const { orgSlug = "" } = useParams();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>([]);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [newSecrets, setNewSecrets] = useState<Record<string, NewAppSecret>>({});

  const apps = useApps(orgSlug);
  const appScopes = useAppScopes(orgSlug, creating);
  const createApp = useCreateApp(orgSlug);

  const list = apps.data?.apps ?? [];

  const submit = () => {
    createApp.mutate(
      { name: name.trim(), scopes },
      {
        onSuccess: (data) => {
          setCreating(false);
          setName("");
          setScopes([]);
          setExpanded(data.app.id);
          setNewSecrets((prev) => ({ ...prev, [data.app.id]: data.secret }));
        },
      },
    );
  };

  return (
    <>
      <SectionHeader
        title="OAuth apps"
        subtitle="Credentials for systems that call Astro on their own, without a person signed in"
        action={
          !creating && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus className="size-4" />
              New OAuth app
            </Button>
          )
        }
      />

      {creating && (
        <form
          className="mb-4 flex flex-col gap-3 rounded-lg border border-border bg-card p-4"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <div>
            <Label size="md" htmlFor="app-name">Name</Label>
            <Input
              id="app-name"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="lumos-connector"
              autoComplete="off"
              maxLength={100}
            />
          </div>
          <div>
            <Label size="md">Scopes</Label>
            <AppScopePicker
              scopes={appScopes.data?.scopes ?? []}
              value={scopes}
              onChange={setScopes}
              loading={appScopes.isPending}
              error={
                appScopes.isError
                  ? getApiErrorMessage(appScopes.error, "Could not load the available scopes.")
                  : undefined
              }
            />
          </div>
          {createApp.isError && (
            <p role="alert" className="text-body-sm text-destructive">
              {getApiErrorMessage(createApp.error, "Could not create the OAuth app.")}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setCreating(false);
                setName("");
                setScopes([]);
                createApp.reset();
              }}
            >
              Cancel
            </Button>
            <Button type="submit" size="sm" disabled={!name.trim() || createApp.isPending}>
              {createApp.isPending ? "Creating…" : "Create OAuth app"}
            </Button>
          </div>
        </form>
      )}

      {apps.isError && (
        <p role="alert" className="mb-3 text-body-sm text-destructive">
          {getApiErrorMessage(apps.error, "Could not load OAuth apps.")}
        </p>
      )}

      {apps.isPending ? (
        <p className="text-body-sm text-muted-foreground">Loading OAuth apps…</p>
      ) : list.length === 0 ? (
        !creating && (
          <div className="rounded-lg border border-dashed border-border px-6 py-10 text-center">
            <div className="mb-3 flex justify-center text-muted-foreground">
              <KeyRound className="size-6" />
            </div>
            <p className="text-sm font-medium text-foreground">No OAuth apps yet</p>
            <p className="mt-1 mb-4 text-xs text-muted-foreground">
              An OAuth app holds a client ID and secret that a script, pipeline, or governance tool
              uses to call Astro on its own.
            </p>
            <Button onClick={() => setCreating(true)}>
              <Plus className="size-4" />
              Create your first OAuth app
            </Button>
          </div>
        )
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8" />
              {["Name", "Client ID", "Secrets"].map((header) => (
                <TableHead key={header}>{header}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.map((app) => (
              <AppRow
                key={app.id}
                app={app}
                account={orgSlug}
                open={expanded === app.id}
                onToggle={() => setExpanded(expanded === app.id ? null : app.id)}
                newSecret={newSecrets[app.id]}
                onDismissNewSecret={() =>
                  setNewSecrets((prev) => {
                    const next = { ...prev };
                    delete next[app.id];
                    return next;
                  })
                }
                onRevealSecret={(secret) =>
                  setNewSecrets((prev) => ({ ...prev, [app.id]: secret }))
                }
              />
            ))}
          </TableBody>
        </Table>
      )}
    </>
  );
}

interface AppRowProps {
  app: MachineApp;
  account: string;
  open: boolean;
  onToggle: () => void;
  newSecret?: NewAppSecret;
  onDismissNewSecret: () => void;
  onRevealSecret: (secret: NewAppSecret) => void;
}

function AppRow({
  app,
  account,
  open,
  onToggle,
  newSecret,
  onDismissNewSecret,
  onRevealSecret,
}: AppRowProps) {
  const createSecret = useCreateAppSecret(account);
  const deleteSecret = useDeleteAppSecret(account);
  const removeApp = useDeleteApp(account);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  const error = createSecret.error ?? deleteSecret.error ?? removeApp.error;
  const Chevron = open ? ChevronDown : ChevronRight;
  // The list refetches after a mint, so carry the new secret until it lands
  // rather than dropping it for a frame.
  const secretRows =
    newSecret && !app.secrets.some((s) => s.id === newSecret.id)
      ? [newSecret, ...app.secrets]
      : app.secrets;

  return (
    <>
      <TableRow interactive onClick={onToggle} data-selected={open || undefined}>
        <TableCell className="w-8 text-muted-foreground">
          <Chevron className="size-4" aria-hidden />
          <span className="sr-only">{open ? "Collapse" : "Expand"} {app.name}</span>
        </TableCell>
        <TableCell>
          <span className="font-medium text-foreground">{app.name}</span>
          {app.description && (
            <span className="mt-0.5 block max-w-xs truncate text-body-sm text-muted-foreground">
              {app.description}
            </span>
          )}
        </TableCell>
        <TableCell>
          <span className="flex items-center gap-1.5" onClick={(e) => e.stopPropagation()}>
            <span className="max-w-[14rem] truncate font-mono text-mono-sm text-muted-foreground">
              {app.client_id}
            </span>
            <CopyButton copyText={app.client_id} title="Copy client ID" iconClassName="size-3.5" />
          </span>
        </TableCell>
        <TableCell className="text-muted-foreground">
          {app.secrets.length === 1 ? "1 secret" : `${app.secrets.length} secrets`}
        </TableCell>
      </TableRow>

      {open && (
        <TableRow>
          <TableCell
            colSpan={COLUMN_COUNT}
            className="p-0 shadow-[inset_3px_0_0_var(--color-primary)]"
          >
            <TooltipProvider delayDuration={150}>
            <div className="flex flex-col gap-4 px-4 py-4">
              <div className="flex flex-col gap-2.5">
                <p className="text-heading-4 text-foreground">
                  Secrets
                  <span className="ml-2 font-normal text-muted-foreground">
                    {app.secrets.length}
                  </span>
                </p>

                <ul className="flex flex-col gap-2">
                  {secretRows.map((secret) => {
                    const isNew = newSecret?.id === secret.id;
                    return (
                      <li
                        key={secret.id}
                        className={cn(
                          "rounded-md border px-3.5 py-3",
                          isNew ? "border-warning/50 bg-warning/10" : "border-border bg-card",
                        )}
                      >
                        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
                          <div className="flex min-w-0 items-center gap-3">
                            <span
                              className={cn(
                                "flex size-8 shrink-0 items-center justify-center rounded-full",
                                isNew ? "bg-warning/20 text-warning" : "bg-primary/10 text-foreground-accent",
                              )}
                            >
                              <KeyRound className="size-4" />
                            </span>
                            <div className="flex min-w-0 flex-col gap-0.5">
                              {isNew ? (
                                <span className="text-body-sm font-medium text-foreground">
                                  New secret
                                </span>
                              ) : (
                                <span className="font-mono text-xs text-foreground">
                                  ••••••••{secret.hint}
                                </span>
                              )}
                              <span className="flex flex-wrap items-center gap-x-2 text-[11px] text-muted-foreground">
                                {isNew ? (
                                  <span>Copy it now. It is not shown again.</span>
                                ) : (
                                  <>
                                    {secret.created_at && (
                                      <>
                                        <span>
                                          Added {new Date(secret.created_at).toLocaleDateString()}
                                        </span>
                                        <span aria-hidden>·</span>
                                      </>
                                    )}
                                    <span>
                                      {secret.last_used_at
                                        ? `Last used ${new Date(secret.last_used_at).toLocaleDateString()}`
                                        : "Never used"}
                                    </span>
                                  </>
                                )}
                              </span>
                            </div>
                          </div>
                          {isNew ? (
                            <Button type="button" variant="outline" size="sm" onClick={onDismissNewSecret}>
                              I saved it
                            </Button>
                          ) : (
                            <RevokeSecretButton
                              hint={secret.hint}
                              onlySecret={app.secrets.length <= 1}
                              pending={deleteSecret.isPending}
                              onRevoke={() =>
                                deleteSecret.mutate({ id: app.id, secretId: secret.id })
                              }
                            />
                          )}
                        </div>

                        {isNew && newSecret && (
                          <div className="relative mt-3 min-w-0 rounded-md border border-border bg-background">
                            <pre className="min-w-0 overflow-x-auto whitespace-pre-wrap break-all p-3 pr-11 font-mono text-xs leading-relaxed text-foreground">
                              {newSecret.value}
                            </pre>
                            <CopyButton
                              copyText={newSecret.value}
                              title="Copy secret"
                              iconClassName="size-3.5"
                              className="absolute top-1.5 right-1.5 shrink-0"
                            />
                          </div>
                        )}
                      </li>
                    );
                  })}
                  <li>
                    <button
                      type="button"
                      disabled={createSecret.isPending}
                      onClick={() =>
                        createSecret.mutate(app.id, {
                          onSuccess: (secret) => onRevealSecret(secret),
                        })
                      }
                      className="flex w-full cursor-pointer items-center gap-2 rounded-md border border-dashed border-border px-3.5 py-3 text-body-sm text-muted-foreground transition-colors hover:border-border-strong hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted/40">
                        <Plus className="size-4" />
                      </span>
                      {createSecret.isPending ? "Adding a secret…" : "Add a secret"}
                    </button>
                  </li>
                </ul>

              </div>

              {error && (
                <p role="alert" className="text-body-sm text-destructive">
                  {getApiErrorMessage(error, "Could not update this OAuth app.")}
                </p>
              )}

              <div className="flex items-center gap-2 border-t border-border pt-3">
                {confirmingDelete ? (
                  <>
                    <p className="flex-1 text-body-sm text-foreground">
                      Delete {app.name}? Every secret is revoked and anything using them stops
                      working.
                    </p>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setConfirmingDelete(false)}
                    >
                      Cancel
                    </Button>
                    <Button
                      type="button"
                      variant="destructive"
                      size="sm"
                      disabled={removeApp.isPending}
                      onClick={() => removeApp.mutate(app.id)}
                    >
                      {removeApp.isPending ? "Deleting…" : "Delete app"}
                    </Button>
                  </>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setConfirmingDelete(true)}
                  >
                    <Trash2 className="size-3.5" />
                    Delete app
                  </Button>
                )}
              </div>
            </div>
            </TooltipProvider>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

const LONE_SECRET_REASON =
  "Add a second secret before revoking this one, so nothing loses access while you swap it over.";

function RevokeSecretButton({
  hint,
  onlySecret,
  pending,
  onRevoke,
}: {
  hint: string;
  onlySecret: boolean;
  pending: boolean;
  onRevoke: () => void;
}) {
  const button = (
    <Button
      type="button"
      variant="outline"
      size="sm"
      aria-label={`Revoke secret ending ${hint}`}
      aria-disabled={onlySecret || pending}
      title={onlySecret ? LONE_SECRET_REASON : undefined}
      onClick={onlySecret || pending ? undefined : onRevoke}
      className={cn(
        "text-destructive hover:text-destructive",
        (onlySecret || pending) && "cursor-not-allowed opacity-50 hover:text-destructive",
      )}
    >
      Revoke
    </Button>
  );

  if (!onlySecret) return button;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0}>{button}</span>
      </TooltipTrigger>
      <TooltipContent side="left" className="max-w-64">
        {LONE_SECRET_REASON}
      </TooltipContent>
    </Tooltip>
  );
}
