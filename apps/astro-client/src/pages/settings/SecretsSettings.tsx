import { useState, useMemo } from 'react'
import { Pencil, Trash2, Upload, MoreHorizontal, Loader2, Lock } from 'lucide-react'
import { KeyIcon, PlusIcon } from '@heroicons/react/24/outline'
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
import { formatRelativeTime } from '@/lib/deployment-utils'
import type { VaultEntry } from '@/lib/vault'
import type { AccountVariable, CreateAccountVariableInput } from '@/lib/api'

const GRID_COLS = 'grid-cols-[1.5fr_1.5fr_0.75fr_56px]'

function toVaultEntry(v: AccountVariable): VaultEntry {
  return {
    name: v.name,
    type: v.secret ? 'secret' : 'variable',
    description: v.description || undefined,
    updatedAt: v.updated_at,
    value: v.value,
  }
}

export function VaultSettings({ account: accountName }: { account: string }) {

  const { data, isLoading, error } = useAccountVariables(accountName)
  const createMutation = useCreateAccountVariable(accountName)
  const updateMutation = useUpdateAccountVariable(accountName)
  const deleteMutation = useDeleteAccountVariable(accountName)

  const [newDialogOpen, setNewDialogOpen] = useState(false)
  const [overwriteEntry, setOverwriteEntry] = useState<VaultEntry | null>(null)
  const [deleteEntry, setDeleteEntry] = useState<VaultEntry | null>(null)
  const [editVariableEntry, setEditVariableEntry] = useState<VaultEntry | null>(null)
  const [importEnvOpen, setImportEnvOpen] = useState(false)

  const entries = useMemo(
    () => (data?.variables ?? []).map(toVaultEntry),
    [data?.variables],
  )
  const existingNames = useMemo(() => entries.map(e => e.name), [entries])

  const handleCreate = (input: CreateAccountVariableInput) => {
    createMutation.mutate(input, {
      onSuccess: () => setNewDialogOpen(false),
    })
  }

  const handleOverwrite = (data: { value: string; description: string }) => {
    if (!overwriteEntry) return
    updateMutation.mutate(
      { name: overwriteEntry.name, data: { value: data.value, secret: true, description: data.description } },
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

  const handleImport = async (entries: CreateAccountVariableInput[]) => {
    const results = await Promise.allSettled(
      entries.map(entry => createMutation.mutateAsync(entry)),
    )
    const failed = results.filter(r => r.status === 'rejected').length
    if (failed > 0) {
      console.warn(`Import: ${failed} of ${entries.length} entries failed`)
    }
    setImportEnvOpen(false)
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
          <h2 className="text-heading-2 text-foreground">Variables & Secrets</h2>
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
            <PlusIcon className="size-3.5" />
            New variable
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
        <div className="rounded-[10px] border border-border overflow-hidden">
          {/* Header */}
          <div className={`grid ${GRID_COLS} gap-x-3 px-4 border-b border-border bg-muted`}>
            {['Name', 'Value', 'Last updated', 'Actions'].map((h, i) => (
              <div key={i} className="font-mono text-label tracking-wider text-faint-foreground py-2.5 uppercase text-left">
                {h}
              </div>
            ))}
          </div>
          {/* Rows */}
          <div className="bg-surface">
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
          description={overwriteEntry.description}
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
      className={`grid ${GRID_COLS} gap-x-3 px-4 items-center hover:bg-muted/40 transition-colors ${isLast ? '' : 'border-b border-border'}`}
    >
      <div className="flex items-center gap-2.5 py-3 min-w-0 overflow-hidden">
        <div className="min-w-0">
          <span className="font-mono text-mono-sm font-medium text-foreground">
            {entry.name}
          </span>
          {entry.description && (
            <p className="text-xs text-muted-foreground mt-0.5 truncate">
              {entry.description}
            </p>
          )}
        </div>
      </div>

      <div className="flex min-w-0 overflow-hidden py-3">
        {isSecret ? (
          <div className="flex items-center gap-1.5">
            <Lock size={12} className="text-foreground shrink-0" />
            <span className="font-mono text-xs text-foreground tracking-widest">
              ••••••••
            </span>
          </div>
        ) : (
          <TooltipProvider delayDuration={400}>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="font-mono text-xs text-foreground truncate cursor-default max-w-full">
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

      <div className="flex">
        <span className="text-xs text-foreground">
          {formatRelativeTime(entry.updatedAt)}
        </span>
      </div>

      <div className="flex justify-end">
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
                Edit
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={onEditVariable}>
                <Pencil className="size-3.5" />
                Edit
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
        <KeyIcon className="size-6" />
      </div>
      <p className="text-sm font-medium text-foreground">No variables yet</p>
      <p className="text-xs text-muted-foreground mt-1 mb-4">
        Create a new variable to get started.
      </p>
      <Button size="sm" onClick={onNew}>
        <PlusIcon className="size-3.5" />
        New variable
      </Button>
    </div>
  )
}

export default function SecretsSettings() {
  const { personalAccount } = useAuth()
  return <VaultSettings account={personalAccount?.name ?? ''} />
}
