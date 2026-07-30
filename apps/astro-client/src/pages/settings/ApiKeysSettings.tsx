import { useState, type ReactNode } from 'react'
import { useSearchParams, type MetaFunction } from 'react-router'
import { Trash2, MoreHorizontal, Loader2, ShieldOff, X, Eye, EyeOff, Database } from 'lucide-react'
import { PlusIcon } from '@heroicons/react/24/outline'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { InlineBadge } from '@/components/InlineBadge'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useCleanupOAuthParams } from '@/hooks/use-cleanup-oauth-params'
import { getIntegrationIconUrl } from '@/lib/assets'
import { resolveDataSourceKind } from '@/lib/data-source-kinds'
import { cn } from '@/lib/utils'
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
  useUpdateOtelIngestKeyExclusions,
} from '@/api/queries'
import { useAuth } from '@/lib/auth'
import { formatRelativeTime } from '@/lib/deployment-utils'
import { ApiRequestError, type OtelIngestKey, type CreateOtelIngestKeyResponse } from '@/lib/api'
import { ErrorPanel } from '@/components/ui/status-panel'

export const meta: MetaFunction = () => [{ title: "Data Sources - Settings | Astro" }];

// Environment-specific; the real value comes from the server (OTEL_INGEST_ENDPOINT).
// This placeholder shows only when that is unset, making misconfig visible instead
// of emitting a plausible-but-wrong URL.
const UNSET_ENDPOINT_PLACEHOLDER = '<OTEL_INGEST_ENDPOINT not set>'

// Hotlink param: /settings/…/api-keys?new=1 opens the create dialog on load
// (e.g. the "Add a source" action on Insights links here).
const NEW_SOURCE_HOTLINK_PARAMS = ['new'] as const

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

// Mirrors the server's normalizeEmails check so invalid input is caught before submit.
function isValidEmail(email: string): boolean {
  return /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)
}

// Masks the back half of a key for display; copy always uses the full value.
function maskKey(token: string): string {
  const shown = Math.ceil(token.length / 2)
  return token.slice(0, shown) + '•'.repeat(token.length - shown)
}

/**
 * Editable list of emails excluded from full-text collection. Enforcement is
 * server-side and per-key; this is the shared editor for both the create and
 * the edit-after-the-fact dialogs.
 */
function EmailExclusionsEditor({
  value,
  onChange,
  disabled,
  hideHeading,
}: {
  value: string[]
  onChange: (next: string[]) => void
  disabled?: boolean
  // Hide the label + description when the surrounding dialog already provides
  // them (the edit-exclusions modal); shown inline in the create flow.
  hideHeading?: boolean
}) {
  const [draft, setDraft] = useState('')
  const [error, setError] = useState<string | null>(null)

  const add = () => {
    const email = draft.trim().toLowerCase()
    if (!email) return
    if (!isValidEmail(email)) {
      setError('Enter a valid email address.')
      return
    }
    if (!value.includes(email)) onChange([...value, email])
    setDraft('')
    setError(null)
  }

  return (
    <div className="space-y-2">
      {!hideHeading && (
        <div>
          <Label size="md" htmlFor="excl-email">
            Excluded users <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <p className="text-xs text-muted-foreground">
            Provide <strong className="font-medium text-foreground">Anthropic</strong> account emails for users to
            exclude from prompt and tool call collection. Usage metadata is collected for all ingested users
            regardless of exclusion.
          </p>
        </div>
      )}
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {value.map((email) => (
            <InlineBadge key={email} variant="soft" className="gap-1 border border-border-strong bg-popover">
              {email}
              <button
                type="button"
                aria-label={`Remove ${email}`}
                className="text-muted-foreground hover:text-foreground disabled:opacity-50"
                disabled={disabled}
                onClick={() => onChange(value.filter((e) => e !== email))}
              >
                <X className="size-3" />
              </button>
            </InlineBadge>
          ))}
        </div>
      )}
      <div className="flex items-center gap-2">
        <Input
          id="excl-email"
          value={draft}
          disabled={disabled}
          placeholder="person@company.com"
          onChange={(e) => {
            setDraft(e.target.value)
            setError(null)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
        />
        <Button type="button" variant="outline" disabled={disabled || !draft.trim()} onClick={add} className="h-11 shrink-0">
          Add
        </Button>
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}

/** One numbered step in the created-key flow: a badge in the gutter + content. */
function StepSection({ n, className, children }: { n: number; className?: string; children: ReactNode }) {
  return (
    <div className={cn('flex gap-3 px-6 py-4', className)}>
      <span className="flex size-6 shrink-0 items-center justify-center rounded-full border border-border text-xs font-medium text-muted-foreground">
        {n}
      </span>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}

export function IngestKeysPanel({ account }: { account: string }) {
  const { data, isLoading, error } = useOtelIngestKeys(account)
  const createMutation = useCreateOtelIngestKey(account)
  const revokeMutation = useRevokeOtelIngestKey(account)
  const exclusionsMutation = useUpdateOtelIngestKeyExclusions(account)
  const { copy: copyKey, copied: keyCopied } = useCopyToClipboard()
  const { copy: copyBlock, copied: blockCopied } = useCopyToClipboard()
  const [searchParams] = useSearchParams()
  useCleanupOAuthParams(NEW_SOURCE_HOTLINK_PARAMS)

  // Seed the create dialog open when hotlinked with ?new=1 (form state starts
  // empty, so this is equivalent to clicking "Add a source").
  const [newOpen, setNewOpen] = useState(() => searchParams.get('new') === '1')
  const [name, setName] = useState('')
  const [created, setCreated] = useState<CreateOtelIngestKeyResponse | null>(null)
  const [keyRevealed, setKeyRevealed] = useState(false)
  const [revealExclusions, setRevealExclusions] = useState<string[]>([])
  const [revokeKey, setRevokeKey] = useState<OtelIngestKey | null>(null)
  const [editKey, setEditKey] = useState<OtelIngestKey | null>(null)
  const [editExclusions, setEditExclusions] = useState<string[]>([])
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

  const openEdit = (key: OtelIngestKey) => {
    setEditKey(key)
    setEditExclusions(key.excluded_emails ?? [])
    exclusionsMutation.reset()
  }

  const handleCreate = () => {
    const trimmed = name.trim()
    if (!trimmed) return
    createMutation.mutate(trimmed, {
      onSuccess: (res) => {
        setNewOpen(false)
        setCreated(res)
        setKeyRevealed(false)
        setRevealExclusions(res.excluded_emails ?? [])
        exclusionsMutation.reset()
      },
    })
  }

  // In the reveal dialog the key already exists, so exclusions save immediately
  // via PATCH. Roll the visible list back on failure so a chip never claims an
  // exclusion the server didn't actually record.
  const handleRevealExclusionsChange = (next: string[]) => {
    const prev = revealExclusions
    setRevealExclusions(next)
    if (created) {
      exclusionsMutation.mutate(
        { keyId: created.id, excludedEmails: next },
        { onError: () => setRevealExclusions(prev) },
      )
    }
  }

  const handleRevoke = () => {
    if (!revokeKey) return
    revokeMutation.mutate(revokeKey.id, {
      onSuccess: () => setRevokeKey(null),
    })
  }

  const handleSaveExclusions = () => {
    if (!editKey) return
    exclusionsMutation.mutate(
      { keyId: editKey.id, excludedEmails: editExclusions },
      { onSuccess: () => setEditKey(null) },
    )
  }

  return (
    <>
      <SectionHeader
        title="Data Sources"
        subtitle="Connect to external data sources to see insights for coding agents like Claude Code and other agents running outside of Astro AI."
        action={
          <Button size="sm" onClick={openNew}>
            <PlusIcon className="size-3.5" />
            Add a source
          </Button>
        }
      />

      {isLoading ? (
        <div className="flex items-center gap-2 py-8 text-[13px] text-muted-foreground">
          <Loader2 size={14} className="animate-spin" />
          Loading...
        </div>
      ) : error ? (
        <ErrorPanel title="Could not load data sources">
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
              <TableHead>Source</TableHead>
              <TableHead>Ingestion key</TableHead>
              <TableHead>Last used</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => (
              <KeyRow key={key.id} entry={key} onRevoke={() => setRevokeKey(key)} onEdit={() => openEdit(key)} />
            ))}
          </TableBody>
        </Table>
      )}

      {/* Create dialog — name only */}
      <Dialog open={newOpen} onOpenChange={(open) => !open && setNewOpen(false)}>
        <DialogContent className="max-w-[440px]">
          <DialogHeader>
            <DialogTitle>New data source</DialogTitle>
            <DialogDescription>
              Give the source a name so you can recognize it later. The secret is shown only once after creation.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label size="md" htmlFor="otel-key-name">Name</Label>
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
                {createMutation.error instanceof Error ? createMutation.error.message : 'Failed to create data source.'}
              </p>
            )}
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" disabled={createMutation.isPending}>Cancel</Button>
            </DialogClose>
            <Button disabled={!name.trim() || createMutation.isPending} onClick={handleCreate}>
              {createMutation.isPending && <Loader2 className="size-3.5 animate-spin" />}
              Create source
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Created dialog — numbered setup flow: save key → configure → copy config */}
      <Dialog open={!!created} onOpenChange={(open) => !open && setCreated(null)}>
        <DialogContent className="gap-0 p-0 sm:max-w-[620px]">
          <DialogHeader className="px-6 py-4">
            <DialogTitle>Data source created</DialogTitle>
          </DialogHeader>

          {created && (
            <div className="min-w-0 divide-y divide-border border-y border-border">
              {/* Step 1 — save the key (shown once) */}
              <StepSection n={1} className="bg-surface">
                <h3 className="text-sm font-medium text-foreground">Save your ingestion key</h3>
                <p className="text-xs text-muted-foreground">This is only shown once</p>
                <div className="mt-2 flex items-center gap-1.5 rounded-md border border-border bg-background py-1.5 pl-3 pr-1.5">
                  <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                    {keyRevealed ? created.token : maskKey(created.token)}
                  </code>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="shrink-0 text-muted-foreground"
                    aria-label={keyRevealed ? 'Hide key' : 'Reveal key'}
                    title={keyRevealed ? 'Hide key' : 'Reveal key'}
                    onClick={() => setKeyRevealed((v) => !v)}
                  >
                    {keyRevealed ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                  </Button>
                  <Button size="sm" className="shrink-0" onClick={() => void copyKey(created.token)}>
                    {keyCopied ? 'Copied' : 'Copy'}
                  </Button>
                </div>
              </StepSection>

              {/* Step 2 — choose what to collect + per-user exclusions */}
              <StepSection n={2}>
                <h3 className="text-sm font-medium text-foreground">Choose what to collect</h3>
                <div className="mt-2 space-y-2.5">
                  <div className="flex items-center gap-2.5">
                    <Switch checked={collectPrompts} onCheckedChange={setCollectPrompts} />
                    <span className="text-sm text-foreground">Collect prompts and responses</span>
                  </div>
                  <div className="flex items-center gap-2.5">
                    <Switch checked={storeToolCalls} onCheckedChange={setStoreToolCalls} />
                    <span className="text-sm text-foreground">Collect tool call inputs</span>
                  </div>
                </div>
                <div className="mt-4 border-t border-border pt-4">
                  <EmailExclusionsEditor
                    value={revealExclusions}
                    onChange={handleRevealExclusionsChange}
                    disabled={exclusionsMutation.isPending}
                  />
                  {exclusionsMutation.isError && (
                    <p className="mt-1.5 text-xs text-destructive">
                      {exclusionsMutation.error instanceof Error ? exclusionsMutation.error.message : 'Failed to save exclusions.'}
                    </p>
                  )}
                </div>
              </StepSection>

              {/* Step 3 — copy the resulting config */}
              <StepSection n={3} className="bg-surface">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h3 className="text-sm font-medium text-foreground">Copy into Anthropic managed settings</h3>
                    <p className="text-xs text-muted-foreground">
                      <a
                        href="https://console.anthropic.com"
                        target="_blank"
                        rel="noreferrer"
                        className="text-foreground-accent hover:underline"
                      >
                        Admin console
                      </a>{' '}
                      → Claude Code → Managed settings
                    </p>
                  </div>
                  <Button
                    size="sm"
                    className="shrink-0"
                    onClick={() =>
                      void copyBlock(
                        managedSettingsBlock(created.endpoint ?? endpoint, created.token, {
                          collectPrompts,
                          storeToolCalls,
                        }),
                      )
                    }
                  >
                    {blockCopied ? 'Copied' : 'Copy'}
                  </Button>
                </div>
                <pre className="mt-3 w-full min-w-0 overflow-x-auto rounded-md border border-border bg-background p-3 font-mono text-[11px] leading-relaxed text-foreground">
                  {managedSettingsBlock(created.endpoint ?? endpoint, created.token, {
                    collectPrompts,
                    storeToolCalls,
                  })}
                </pre>
              </StepSection>
            </div>
          )}

          <DialogFooter className="p-6 pt-4">
            <DialogClose asChild>
              <Button variant="outline">Cancel</Button>
            </DialogClose>
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
            <DialogTitle>Revoke ingestion key</DialogTitle>
            <DialogDescription>
              Are you sure you want to revoke the ingestion key for{' '}
              <span className="font-mono font-medium text-foreground">{revokeKey?.name}</span>? Machines using it
              will stop sending telemetry immediately. This can't be undone.
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

      {/* Edit exclusions — no key rotation, key never revealed */}
      <Dialog open={!!editKey} onOpenChange={(open) => !open && setEditKey(null)}>
        <DialogContent className="max-w-[460px]">
          <DialogHeader>
            <DialogTitle>Edit excluded users</DialogTitle>
            <DialogDescription>
              Provide <strong className="font-medium text-foreground">Anthropic</strong> account emails for users to
              exclude from prompt and tool call collection. Usage metadata is collected for all ingested users
              regardless of exclusion.
            </DialogDescription>
          </DialogHeader>
          <EmailExclusionsEditor
            value={editExclusions}
            onChange={setEditExclusions}
            disabled={exclusionsMutation.isPending}
            hideHeading
          />
          {exclusionsMutation.isError && (
            <p className="text-xs text-destructive">
              {exclusionsMutation.error instanceof Error ? exclusionsMutation.error.message : 'Failed to save exclusions.'}
            </p>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" disabled={exclusionsMutation.isPending}>Cancel</Button>
            </DialogClose>
            <Button disabled={exclusionsMutation.isPending} onClick={handleSaveExclusions}>
              {exclusionsMutation.isPending && <Loader2 className="size-3.5 animate-spin" />}
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function KeyRow({ entry, onRevoke, onEdit }: { entry: OtelIngestKey; onRevoke: () => void; onEdit: () => void }) {
  const exclusionCount = entry.excluded_emails?.length ?? 0
  const kind = resolveDataSourceKind(entry.source_type)
  return (
    <TableRow>
      <TableCell>
        <div className="flex items-center gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-sm border border-border bg-card">
            <img
              src={getIntegrationIconUrl(kind.icon, 'light')}
              alt=""
              className="size-6 object-contain dark:hidden"
            />
            <img
              src={getIntegrationIconUrl(kind.icon, 'dark')}
              alt=""
              className="hidden size-6 object-contain dark:block"
            />
          </span>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{entry.name}</div>
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span>{kind.label}</span>
              {exclusionCount > 0 && (
                <InlineBadge variant="soft" className="bg-card text-muted-foreground">
                  {exclusionCount} excluded
                </InlineBadge>
              )}
            </div>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <span className="font-mono text-xs text-muted-foreground">{entry.token_prefix}…</span>
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
            <DropdownMenuItem onClick={onEdit}>
              <ShieldOff className="size-3.5" />
              Edit exclusions
            </DropdownMenuItem>
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
        <Database className="size-6" />
      </div>
      <p className="text-sm font-medium text-foreground">No data sources yet</p>
      <p className="text-xs text-muted-foreground mt-1 mb-4">
        Add a source to start collecting telemetry from local coding tools.
      </p>
      <Button size="sm" onClick={onNew}>
        <PlusIcon className="size-3.5" />
        Add a source
      </Button>
    </div>
  )
}

export default function ApiKeysSettings() {
  const { personalAccount } = useAuth()
  return <IngestKeysPanel account={personalAccount?.name ?? ''} />
}
