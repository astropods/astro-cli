import { useState } from 'react'
import { Lock, Plus, Pencil, Trash2, Upload, MoreHorizontal, Loader2 } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from '@/components/ui/dialog'
import { NewEntryDialog } from '@/components/settings/secrets/NewEntryDialog'
import { OverwriteSecretDialog } from '@/components/settings/secrets/OverwriteSecretDialog'
import { ImportEnvDialog } from '@/components/settings/secrets/ImportEnvDialog'
import { EditVariableDialog } from '@/components/settings/secrets/EditVariableDialog'
import {
  useAccountVariables,
  useCreateAccountVariable,
  useUpdateAccountVariable,
  useDeleteAccountVariable,
} from '@/api/queries'
import { useAuth } from '@/lib/auth'
import type { VaultEntry } from '@/lib/vault'
import type { AccountVariable, CreateAccountVariableInput } from '@/lib/api'

const GRID_COLS = '1.5fr 1.5fr 0.75fr 56px'

function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr)
  const diffSecs = Math.round((date.getTime() - Date.now()) / 1000)
  const diffMins = Math.round(diffSecs / 60)
  const diffHours = Math.round(diffMins / 60)
  const diffDays = Math.round(diffHours / 24)
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
  if (Math.abs(diffSecs) < 60) return 'just now'
  if (Math.abs(diffMins) < 60) return rtf.format(diffMins, 'minute')
  if (Math.abs(diffHours) < 24) return rtf.format(diffHours, 'hour')
  return rtf.format(diffDays, 'day')
}

function toVaultEntry(v: AccountVariable): VaultEntry {
  return {
    name: v.name,
    type: v.secret ? 'secret' : 'variable',
    description: v.description || undefined,
    updatedAt: v.updated_at,
    value: v.value,
  }
}

export default function SecretsSettings() {
  const { personalAccount } = useAuth()
  const accountName = personalAccount?.name ?? ''

  const { data, isLoading, error } = useAccountVariables(accountName)
  const createMutation = useCreateAccountVariable(accountName)
  const updateMutation = useUpdateAccountVariable(accountName)
  const deleteMutation = useDeleteAccountVariable(accountName)

  const [newDialogOpen, setNewDialogOpen] = useState(false)
  const [overwriteEntry, setOverwriteEntry] = useState<VaultEntry | null>(null)
  const [deleteEntry, setDeleteEntry] = useState<VaultEntry | null>(null)
  const [editVariableEntry, setEditVariableEntry] = useState<VaultEntry | null>(null)
  const [importEnvOpen, setImportEnvOpen] = useState(false)

  const entries: VaultEntry[] = (data?.variables ?? []).map(toVaultEntry)
  const existingNames = entries.map(e => e.name)

  const handleCreate = (input: CreateAccountVariableInput) => {
    createMutation.mutate(input, {
      onSuccess: () => setNewDialogOpen(false),
    })
  }

  const handleOverwrite = (value: string) => {
    if (!overwriteEntry) return
    updateMutation.mutate(
      { name: overwriteEntry.name, data: { value, secret: true } },
      { onSuccess: () => setOverwriteEntry(null) }
    )
  }

  const handleEditVariable = (data: { value: string; description: string }) => {
    if (!editVariableEntry) return
    updateMutation.mutate(
      { name: editVariableEntry.name, data: { value: data.value, description: data.description } },
      { onSuccess: () => setEditVariableEntry(null) }
    )
  }

  const handleImport = (entries: CreateAccountVariableInput[]) => {
    // Sequential creates; invalidation happens after each, UI stays consistent
    let pending = entries.length
    const done = () => {
      pending--
      if (pending === 0) setImportEnvOpen(false)
    }
    for (const entry of entries) {
      createMutation.mutate(entry, { onSuccess: done, onError: done })
    }
  }

  const handleDelete = () => {
    if (!deleteEntry) return
    deleteMutation.mutate(deleteEntry.name, {
      onSuccess: () => setDeleteEntry(null),
    })
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-heading-2 text-foreground">Secrets & Variables</h2>
          <p className="text-[13px] text-muted-foreground mt-1">
            Set and manage reusable credentials and configuration values for your agents
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button variant="outline" size="sm" onClick={() => setImportEnvOpen(true)}>
            <Upload className="size-3.5" />
            Import .env
          </Button>
          <Button size="sm" onClick={() => setNewDialogOpen(true)}>
            <Plus className="size-3.5" />
            New
          </Button>
        </div>
      </div>

      <Separator />

      {isLoading ? (
        <div className="flex items-center gap-2 py-8 text-[13px] text-muted-foreground">
          <Loader2 size={14} className="animate-spin" />
          Loading...
        </div>
      ) : error ? (
        <p className="text-[13px] text-muted-foreground py-4">Failed to load variables.</p>
      ) : entries.length === 0 ? (
        <EmptyState onNew={() => setNewDialogOpen(true)} />
      ) : (
        <div style={{ border: '1px solid var(--border)', borderRadius: 10, overflow: 'hidden' }}>
          {/* Header */}
          <div style={{ display: 'grid', gridTemplateColumns: GRID_COLS, columnGap: 12, padding: '0 16px', borderBottom: '1px solid var(--border)', background: 'var(--muted)' }}>
            {['Name', 'Value', 'Last updated', 'Actions'].map((h, i) => (
              <div key={i} style={{
                fontFamily: 'var(--font-mono)',
                fontSize: 'var(--text-label)',
                letterSpacing: '0.07em',
                color: 'var(--faint-foreground)',
                padding: '10px 0',
                textTransform: 'uppercase',
                textAlign: 'left',
              }}>
                {h}
              </div>
            ))}
          </div>
          {/* Rows */}
          <div style={{ background: 'var(--surface)' }}>
            {entries.map((entry, i) => (
              <EntryRow
                key={entry.name}
                entry={entry}
                isLast={i === entries.length - 1}
                onOverwrite={() => setOverwriteEntry(entry)}
                onEditVariable={() => setEditVariableEntry(entry)}
                onDelete={() => setDeleteEntry(entry)}
              />
            ))}
          </div>
        </div>
      )}

      <NewEntryDialog
        open={newDialogOpen}
        isPending={createMutation.isPending}
        onClose={() => setNewDialogOpen(false)}
        onCreate={handleCreate}
      />

      {overwriteEntry && (
        <OverwriteSecretDialog
          secretName={overwriteEntry.name}
          open
          isPending={updateMutation.isPending}
          onClose={() => setOverwriteEntry(null)}
          onConfirm={handleOverwrite}
        />
      )}

      {editVariableEntry && (
        <EditVariableDialog
          entry={editVariableEntry}
          open
          isPending={updateMutation.isPending}
          onClose={() => setEditVariableEntry(null)}
          onSave={handleEditVariable}
        />
      )}

      <ImportEnvDialog
        open={importEnvOpen}
        isPending={createMutation.isPending}
        existingNames={existingNames}
        onClose={() => setImportEnvOpen(false)}
        onImport={handleImport}
      />

      <Dialog open={!!deleteEntry} onOpenChange={open => !open && setDeleteEntry(null)}>
        <DialogContent className="max-w-[400px]">
          <DialogHeader>
            <DialogTitle>Delete variable</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete{' '}
              <span className="font-mono font-medium text-foreground">{deleteEntry?.name}</span>?
              {deleteEntry?.type === 'secret' && (
                <> The value will be permanently removed and can't be recovered.</>
              )}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" disabled={deleteMutation.isPending}>Cancel</Button>
            </DialogClose>
            <Button
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={handleDelete}
            >
              {deleteMutation.isPending && <Loader2 className="size-3.5 animate-spin" />}
              Yes, delete this variable
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function EntryRow({
  entry,
  isLast,
  onOverwrite,
  onEditVariable,
  onDelete,
}: {
  entry: VaultEntry
  isLast: boolean
  onOverwrite: () => void
  onEditVariable: () => void
  onDelete: () => void
}) {
  const isSecret = entry.type === 'secret'

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: GRID_COLS,
        columnGap: 12,
        padding: '0 16px',
        alignItems: 'center',
        borderBottom: isLast ? 'none' : '1px solid var(--border)',
      }}
      className="hover:bg-muted/40 transition-colors"
    >
      {/* Name */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '12px 0', minWidth: 0, overflow: 'hidden' }}>
        <div style={{ minWidth: 0 }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-mono-sm)', fontWeight: 500, color: 'var(--foreground)' }}>
            {entry.name}
          </span>
          {entry.description && (
            <p className="text-xs text-muted-foreground mt-0.5 truncate">
              {entry.description}
            </p>
          )}
        </div>
      </div>

      {/* Value */}
      <div style={{ display: 'flex', minWidth: 0, overflow: 'hidden', padding: '12px 0' }}>
        {isSecret ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <Lock size={12} style={{ color: 'var(--foreground)', flexShrink: 0 }} />
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--foreground)', letterSpacing: '0.1em' }}>
              ••••••••
            </span>
          </div>
        ) : (
          <TooltipProvider delayDuration={400}>
            <Tooltip>
              <TooltipTrigger asChild>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--foreground)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', cursor: 'default', maxWidth: '100%' }}>
                  {entry.value || '—'}
                </span>
              </TooltipTrigger>
              {entry.value && (
                <TooltipContent side="top" className="font-mono text-xs max-w-[360px] break-all">
                  {entry.value}
                </TooltipContent>
              )}
            </Tooltip>
          </TooltipProvider>
        )}
      </div>

      {/* Last updated */}
      <div style={{ display: 'flex' }}>
        <span style={{ fontSize: 12, color: 'var(--foreground)' }}>
          {formatRelativeTime(entry.updatedAt)}
        </span>
      </div>

      {/* Actions */}
      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-xs">
              <MoreHorizontal className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-[160px]">
            {isSecret ? (
              <DropdownMenuItem onClick={onOverwrite}>
                <Pencil className="size-3.5" />
                Update value
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={onEditVariable}>
                <Pencil className="size-3.5" />
                Edit variable
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={onDelete}
              className="text-destructive focus:text-destructive focus:bg-destructive/10"
            >
              <Trash2 className="size-3.5 text-destructive" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}

function EmptyState({ onNew }: { onNew: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <div className="flex justify-center mb-3 text-muted-foreground">
        <Lock className="size-6" />
      </div>
      <p className="text-sm font-medium text-foreground">No variables yet</p>
      <p className="text-xs text-muted-foreground mt-1 mb-4">
        Add credentials and configuration values for your agents
      </p>
      <Button size="sm" onClick={onNew}>
        <Plus className="size-3.5" />
        New
      </Button>
    </div>
  )
}
