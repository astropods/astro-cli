import { useState } from 'react'
import type { MetaFunction } from 'react-router'
import { Trash2, MoreHorizontal, Loader2, TriangleAlert } from 'lucide-react'
import { KeyIcon, PlusIcon } from '@heroicons/react/24/outline'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { CopyButton } from '@/components/ui/copy-button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { SectionHeader } from '@/components/settings/SettingsShared'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from '@/components/ui/dialog'
import {
  useOtelIngestKeys,
  useCreateOtelIngestKey,
  useRevokeOtelIngestKey,
} from '@/api/queries'
import { useAuth } from '@/lib/auth'
import { formatRelativeTime } from '@/lib/deployment-utils'
import { ApiRequestError, type OtelIngestKey, type CreateOtelIngestKeyResponse } from '@/lib/api'
import { ErrorPanel } from '@/components/ui/status-panel'

export const meta: MetaFunction = () => [{ title: "API Keys - Settings | Astro" }];

// Environment-specific; the real value comes from the server (OTEL_INGEST_ENDPOINT).
// This placeholder shows only when that is unset, making misconfig visible instead
// of emitting a plausible-but-wrong URL.
const UNSET_ENDPOINT_PLACEHOLDER = '<OTEL_INGEST_ENDPOINT not set>'

/** Content-collection options that append logs-signal settings to the block. */
type CollectionOptions = { collectPrompts: boolean; storeToolCalls: boolean }

/** Builds the Anthropic managed-settings env block for a freshly created key. */
function managedSettingsBlock(endpoint: string, token: string, opts: CollectionOptions): string {
  const vars: [string, string][] = [
    ['CLAUDE_CODE_ENABLE_TELEMETRY', '1'],
    ['OTEL_METRICS_EXPORTER', 'otlp'],
    ['OTEL_TRACES_EXPORTER', 'otlp'],
    ['CLAUDE_CODE_ENHANCED_TELEMETRY_BETA', '1'],
    ['OTEL_EXPORTER_OTLP_PROTOCOL', 'http/protobuf'],
    ['OTEL_EXPORTER_OTLP_ENDPOINT', endpoint || UNSET_ENDPOINT_PLACEHOLDER],
    ['OTEL_EXPORTER_OTLP_HEADERS', `Authorization=Bearer ${token}`],
    ['OTEL_METRICS_INCLUDE_SESSION_ID', 'false'],
  ]
  // Prompt/response text and tool inputs ride the logs signal, off unless opted in.
  if (opts.collectPrompts || opts.storeToolCalls) vars.push(['OTEL_LOGS_EXPORTER', 'otlp'])
  if (opts.collectPrompts) vars.push(['OTEL_LOG_USER_PROMPTS', '1'])
  if (opts.storeToolCalls) vars.push(['OTEL_LOG_TOOL_DETAILS', '1'])
  return vars.map(([k, v]) => `${k.padEnd(36)}= ${v}`).join('\n')
}

export function IngestKeysPanel({ account }: { account: string }) {
  const { data, isLoading, error } = useOtelIngestKeys(account)
  const createMutation = useCreateOtelIngestKey(account)
  const revokeMutation = useRevokeOtelIngestKey(account)

  const [newOpen, setNewOpen] = useState(false)
  const [name, setName] = useState('')
  const [created, setCreated] = useState<CreateOtelIngestKeyResponse | null>(null)
  const [revokeKey, setRevokeKey] = useState<OtelIngestKey | null>(null)
  const [collectPrompts, setCollectPrompts] = useState(false)
  const [storeToolCalls, setStoreToolCalls] = useState(false)

  const keys = data?.tokens ?? []
  const endpoint = data?.endpoint ?? ''

  const openNew = () => {
    setName('')
    setCollectPrompts(false)
    setStoreToolCalls(false)
    setNewOpen(true)
  }

  const handleCreate = () => {
    const trimmed = name.trim()
    if (!trimmed) return
    createMutation.mutate(trimmed, {
      onSuccess: (res) => {
        setNewOpen(false)
        setCreated(res)
      },
    })
  }

  const handleRevoke = () => {
    if (!revokeKey) return
    revokeMutation.mutate(revokeKey.id, {
      onSuccess: () => setRevokeKey(null),
    })
  }

  return (
    <>
      <SectionHeader
        title="API Keys"
        subtitle="Stream usage telemetry from local AI coding tools (e.g. Claude Code) into Astro observability. Create an ingest key and set it as a forced environment in your Anthropic admin console."
        action={
          <Button size="sm" onClick={openNew}>
            <PlusIcon className="size-3.5" />
            New ingest key
          </Button>
        }
      />

      {isLoading ? (
        <div className="flex items-center gap-2 py-8 text-[13px] text-muted-foreground">
          <Loader2 size={14} className="animate-spin" />
          Loading...
        </div>
      ) : error ? (
        <ErrorPanel title="Could not load ingest keys">
          {error instanceof ApiRequestError
            ? error.message
            : error instanceof Error
              ? error.message
              : 'Something went wrong.'}
        </ErrorPanel>
      ) : keys.length === 0 ? (
        <EmptyState onNew={openNew} />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Key</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Last used</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => (
              <KeyRow key={key.id} entry={key} onRevoke={() => setRevokeKey(key)} />
            ))}
          </TableBody>
        </Table>
      )}

      {/* Create dialog — name only */}
      <Dialog open={newOpen} onOpenChange={(open) => !open && setNewOpen(false)}>
        <DialogContent className="max-w-[440px]">
          <DialogHeader>
            <DialogTitle>New ingest key</DialogTitle>
            <DialogDescription>
              Give the key a name so you can recognize it later. The secret is shown only once after creation.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="otel-key-name">Name</Label>
            <Input
              id="otel-key-name"
              value={name}
              autoFocus
              placeholder="e.g. Engineering laptops"
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            />
            {createMutation.isError && (
              <p className="text-xs text-destructive">
                {createMutation.error instanceof Error ? createMutation.error.message : 'Failed to create key.'}
              </p>
            )}
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" disabled={createMutation.isPending}>Cancel</Button>
            </DialogClose>
            <Button disabled={!name.trim() || createMutation.isPending} onClick={handleCreate}>
              {createMutation.isPending && <Loader2 className="size-3.5 animate-spin" />}
              Create key
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Created dialog — reveal secret once + managed-settings block */}
      <Dialog open={!!created} onOpenChange={(open) => !open && setCreated(null)}>
        <DialogContent className="max-w-[560px]">
          <DialogHeader>
            <DialogTitle>Ingest key created</DialogTitle>
            <DialogDescription>
              Copy this now — the secret won't be shown again. Paste the block below into your Anthropic admin
              console under Claude Code → Managed settings.
            </DialogDescription>
          </DialogHeader>

          {created && (
            <div className="space-y-3 min-w-0">
              <div className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-foreground">
                <TriangleAlert className="size-3.5 shrink-0 text-warning mt-0.5" />
                This is the only time the key is displayed. Store it somewhere safe.
              </div>

              <div className="flex items-center gap-2 min-w-0">
                <code className="min-w-0 flex-1 truncate rounded-md bg-surface border border-border px-2 py-1.5 font-mono text-xs text-foreground">
                  {created.token}
                </code>
                <CopyButton copyText={created.token} title="Copy key" iconClassName="size-3.5" />
              </div>

              <div className="space-y-2.5 min-w-0">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <p className="text-sm font-medium text-foreground">Collect prompts and responses</p>
                    <p className="text-xs text-muted-foreground">
                      Include the text of prompts and Claude's replies.
                    </p>
                  </div>
                  <Switch checked={collectPrompts} onCheckedChange={setCollectPrompts} />
                </div>
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <p className="text-sm font-medium text-foreground">Store tool calls</p>
                    <p className="text-xs text-muted-foreground">
                      Include the inputs to the tools Claude Code runs.
                    </p>
                  </div>
                  <Switch checked={storeToolCalls} onCheckedChange={setStoreToolCalls} />
                </div>
              </div>

              <div className="space-y-1.5 min-w-0">
                <div className="flex items-center justify-between">
                  <Label>Managed settings block</Label>
                  <CopyButton
                    copyText={managedSettingsBlock(created.endpoint ?? endpoint, created.token, {
                      collectPrompts,
                      storeToolCalls,
                    })}
                    title="Copy block"
                    iconClassName="size-3.5"
                  />
                </div>
                <pre className="w-full min-w-0 overflow-x-auto rounded-md bg-surface border border-border p-3 font-mono text-[11px] leading-relaxed text-foreground">
                  {managedSettingsBlock(created.endpoint ?? endpoint, created.token, {
                    collectPrompts,
                    storeToolCalls,
                  })}
                </pre>
              </div>
            </div>
          )}

          <DialogFooter>
            <DialogClose asChild>
              <Button>Done</Button>
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Revoke confirmation */}
      <Dialog open={!!revokeKey} onOpenChange={(open) => !open && setRevokeKey(null)}>
        <DialogContent className="max-w-[400px]">
          <DialogHeader>
            <DialogTitle>Revoke ingest key</DialogTitle>
            <DialogDescription>
              Are you sure you want to revoke{' '}
              <span className="font-mono font-medium text-foreground">{revokeKey?.name}</span>? Machines using this
              key will stop sending telemetry immediately. This can't be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" disabled={revokeMutation.isPending}>Cancel</Button>
            </DialogClose>
            <Button variant="destructive" disabled={revokeMutation.isPending} onClick={handleRevoke}>
              {revokeMutation.isPending && <Loader2 className="size-3.5 animate-spin" />}
              Yes, revoke this key
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function KeyRow({ entry, onRevoke }: { entry: OtelIngestKey; onRevoke: () => void }) {
  return (
    <TableRow>
      <TableCell>
        <span className="font-medium text-foreground">{entry.name}</span>
      </TableCell>
      <TableCell>
        <span className="font-mono text-xs text-muted-foreground">{entry.token_prefix}…</span>
      </TableCell>
      <TableCell>
        <span className="text-body-sm text-foreground">{formatRelativeTime(entry.created_at)}</span>
      </TableCell>
      <TableCell>
        <span className="text-body-sm text-muted-foreground">
          {entry.last_used_at ? formatRelativeTime(entry.last_used_at) : 'Never'}
        </span>
      </TableCell>
      <TableCell className="w-10 text-right">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-xs">
              <MoreHorizontal className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-[160px]">
            <DropdownMenuItem
              onClick={onRevoke}
              className="text-destructive focus:text-destructive focus:bg-destructive/10"
            >
              <Trash2 className="size-3.5 text-destructive" />
              Revoke
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  )
}

function EmptyState({ onNew }: { onNew: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <div className="flex justify-center mb-3 text-muted-foreground">
        <KeyIcon className="size-6" />
      </div>
      <p className="text-sm font-medium text-foreground">No ingest keys yet</p>
      <p className="text-xs text-muted-foreground mt-1 mb-4">
        Create a key to start collecting telemetry from local coding tools.
      </p>
      <Button size="sm" onClick={onNew}>
        <PlusIcon className="size-3.5" />
        New ingest key
      </Button>
    </div>
  )
}

export default function ApiKeysSettings() {
  const { personalAccount } = useAuth()
  return <IngestKeysPanel account={personalAccount?.name ?? ''} />
}
