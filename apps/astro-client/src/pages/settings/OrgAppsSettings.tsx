import { useState } from "react";
import { useParams, type MetaFunction } from "react-router";
import { Plus, Trash2, KeyRound, Check } from "lucide-react";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CopyButton } from "@/components/ui/copy-button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useApps, useCreateApp, useCreateAppSecret, useDeleteApp, useDeleteAppSecret } from "@/api/queries";
import { getApiErrorMessage, type MachineApp, type NewAppSecret } from "@/lib/api";

export const meta: MetaFunction = () => [
  { title: "OAuth apps - Organization Settings | Astro" },
];

export default function OrgAppsSettings() {
  const { orgSlug = "" } = useParams();
  const apps = useApps(orgSlug);
  const createApp = useCreateApp(orgSlug);
  const createSecret = useCreateAppSecret(orgSlug);
  const deleteSecret = useDeleteAppSecret(orgSlug);
  const removeApp = useDeleteApp(orgSlug);

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [revealed, setRevealed] = useState<{ appName: string; secret: NewAppSecret } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<MachineApp | null>(null);

  const list = apps.data?.apps ?? [];
  const error = apps.error ?? createApp.error ?? createSecret.error ?? deleteSecret.error ?? removeApp.error;

  const submit = () => {
    createApp.mutate(
      { name: name.trim() },
      {
        onSuccess: (data) => {
          setCreateOpen(false);
          setName("");
          setRevealed({ appName: data.app.name, secret: data.secret });
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
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            New OAuth app
          </Button>
        }
      />

      {apps.isPending ? (
        <p className="text-body-sm text-muted-foreground">Loading OAuth apps…</p>
      ) : list.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border px-6 py-10 text-center">
          <div className="mb-3 flex justify-center text-muted-foreground">
            <KeyRound className="size-6" />
          </div>
          <p className="text-sm font-medium text-foreground">No OAuth apps yet</p>
          <p className="mt-1 mb-4 text-xs text-muted-foreground">
            An OAuth app holds a client ID and secret that a script, pipeline, or governance tool
            uses to call Astro on its own.
          </p>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            Create your first OAuth app
          </Button>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              {["Name", "Client ID", "Secrets"].map((header) => (
                <TableHead key={header}>{header}</TableHead>
              ))}
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.map((app) => (
              <TableRow key={app.id}>
                <TableCell>
                  <span className="font-medium text-foreground">{app.name}</span>
                  {app.description && (
                    <span className="mt-0.5 block max-w-xs truncate text-body-sm text-muted-foreground">
                      {app.description}
                    </span>
                  )}
                </TableCell>
                <TableCell>
                  <span className="flex items-center gap-1.5">
                    <span className="max-w-[12rem] truncate font-mono text-mono-sm text-muted-foreground">
                      {app.client_id}
                    </span>
                    <CopyButton copyText={app.client_id} title="Copy client ID" iconClassName="size-3.5" />
                  </span>
                </TableCell>
                <TableCell>
                  <div className="flex flex-col gap-1">
                    {app.secrets.map((secret) => (
                      <span key={secret.id} className="flex items-center gap-2 text-body-sm">
                        <span className="font-mono text-mono-sm text-muted-foreground">
                          …{secret.hint}
                        </span>
                        <span className="text-[11px] text-muted-foreground">
                          {secret.last_used_at
                            ? `used ${new Date(secret.last_used_at).toLocaleDateString()}`
                            : "never used"}
                        </span>
                        {app.secrets.length > 1 && (
                          <button
                            type="button"
                            aria-label={`Revoke secret ending ${secret.hint}`}
                            disabled={deleteSecret.isPending}
                            onClick={() => deleteSecret.mutate({ id: app.id, secretId: secret.id })}
                            className="cursor-pointer text-muted-foreground hover:text-destructive disabled:opacity-50"
                          >
                            <Trash2 className="size-3.5" />
                          </button>
                        )}
                      </span>
                    ))}
                    <button
                      type="button"
                      disabled={createSecret.isPending}
                      onClick={() =>
                        createSecret.mutate(app.id, {
                          onSuccess: (secret) => setRevealed({ appName: app.name, secret }),
                        })
                      }
                      className="w-fit cursor-pointer text-[11px] text-foreground-accent hover:underline disabled:opacity-50"
                    >
                      Add a secret
                    </button>
                  </div>
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`Delete ${app.name}`}
                    onClick={() => setDeleteTarget(app)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {error && (
        <p role="alert" className="mt-3 text-body-sm text-destructive">
          {getApiErrorMessage(error, "Could not update OAuth apps.")}
        </p>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>New OAuth app</DialogTitle>
            <DialogDescription>
              The secret is shown once, when the app is created.
            </DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
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
              <div className="rounded-md border border-dashed border-border px-3 py-2.5">
                <p className="text-body-sm text-muted-foreground">
                  Choosing what an app can reach is coming soon. Until then an app is created
                  without scopes and is refused by every scoped endpoint.
                </p>
              </div>
            </div>
            {createApp.isError && (
              <p role="alert" className="text-body-sm text-destructive">
                {getApiErrorMessage(createApp.error, "Could not create the OAuth app.")}
              </p>
            )}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!name.trim() || createApp.isPending}>
                {createApp.isPending ? "Creating…" : "Create OAuth app"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!revealed} onOpenChange={(open) => !open && setRevealed(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Copy the secret now</DialogTitle>
            <DialogDescription>
              This is the only time {revealed?.appName}&rsquo;s secret is shown. Store it in your
              secret manager before closing this dialog.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2">
            <span className="min-w-0 flex-1 truncate font-mono text-mono-sm text-foreground">
              {revealed?.secret.value}
            </span>
            {revealed && (
              <CopyButton copyText={revealed.secret.value} title="Copy secret" iconClassName="size-4" />
            )}
          </div>
          <DialogFooter>
            <Button onClick={() => setRevealed(null)}>
              <Check className="size-4" />
              I saved it
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {deleteTarget && (
        <Dialog open onOpenChange={(open) => !open && setDeleteTarget(null)}>
          <DialogContent className="max-w-sm">
            <DialogHeader>
              <DialogTitle>Delete &ldquo;{deleteTarget.name}&rdquo;?</DialogTitle>
              <DialogDescription>
                Every secret is revoked immediately, and anything using them stops working.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setDeleteTarget(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                disabled={removeApp.isPending}
                onClick={() =>
                  removeApp.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) })
                }
              >
                {removeApp.isPending ? "Deleting…" : "Delete OAuth app"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
