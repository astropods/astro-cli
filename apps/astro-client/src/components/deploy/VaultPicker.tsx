import { useState } from 'react'
import { X, Plus } from 'lucide-react'
import { MagnifyingGlassIcon, KeyIcon } from '@heroicons/react/24/outline'
import { Popover as PopoverPrimitive } from 'radix-ui'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { AccountVariable } from '@/lib/api'
import { NewEntryDialog } from '@/components/settings/secrets/NewEntryDialog'
import { useCreateAccountVariable } from '@/api/queries/variables'

// Parse a token like {{secrets.FOO}} or {{vars.BAR}} into its parts
export function parseVaultToken(token: string): { type: 'secret' | 'variable'; name: string } | null {
  const match = token.match(/^\{\{(secrets|vars)\.([A-Z][A-Z0-9_]*)\}\}$/)
  if (!match) return null
  return { type: match[1] === 'secrets' ? 'secret' : 'variable', name: match[2] }
}

interface VaultPickerProps {
  onSelect: (token: string) => void
  entries?: AccountVariable[]
  accountName?: string
  vaultSettingsUrl?: string
}

export function VaultPicker({ onSelect, entries = [], accountName }: VaultPickerProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [newVarOpen, setNewVarOpen] = useState(false)
  const createMutation = useCreateAccountVariable(accountName ?? '')

  const filtered = entries.filter(e =>
    e.name.toLowerCase().includes(search.toLowerCase()) ||
    e.description?.toLowerCase().includes(search.toLowerCase())
  )
  const hasResults = filtered.length > 0

  const handleSelect = (entry: AccountVariable) => {
    const token = entry.secret
      ? `{{secrets.${entry.name}}}`
      : `{{vars.${entry.name}}}`
    onSelect(token)
    setOpen(false)
    setSearch('')
  }

  return (
    <>
    <PopoverPrimitive.Root open={open} onOpenChange={(o) => { setOpen(o); if (!o) setSearch('') }}>
      <PopoverPrimitive.Trigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          title="Insert vault reference"
          className={cn(
            'shrink-0 text-foreground border border-border transition-colors',
            open && 'border-teal-600 text-teal-700 dark:text-teal-400 bg-teal-50 dark:bg-teal-950/40'
          )}
        >
          <KeyIcon className="size-3.5" />
        </Button>
      </PopoverPrimitive.Trigger>

      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          sideOffset={6}
          align="end"
          className="z-50 w-[280px] rounded-lg border border-border bg-popover shadow-lg overflow-hidden data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
        >
          {entries.length === 0 ? (
            <div className="px-4 py-5 text-center">
              <KeyIcon className="size-5 text-muted-foreground/50 mx-auto mb-2" />
              <p className="text-sm font-medium text-foreground">No variables yet</p>
              <p className="text-xs text-muted-foreground mt-1 mb-3">
                Set and manage reusable credentials and configuration values for your agents
              </p>
              <Button size="sm" onClick={() => { setOpen(false); setNewVarOpen(true) }}>
                <Plus className="size-3.5" />
                New variable
              </Button>
            </div>
          ) : (
            <>
              <div className="px-3 pt-3 pb-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-foreground">Select a reference</p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-6 px-2 text-xs shrink-0"
                    onClick={() => { setOpen(false); setNewVarOpen(true) }}
                  >
                    <Plus className="size-3" />
                    New
                  </Button>
                </div>
                {accountName && (
                  <p className="text-[11px] text-muted-foreground mt-0.5">
                    From <span className="font-medium">{accountName}</span>
                  </p>
                )}
              </div>

              <div className="px-2 pb-2 border-b border-border">
                <div className="relative">
                  <MagnifyingGlassIcon className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none" />
                  <Input
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                    placeholder="Find..."
                    className="h-8 text-sm pl-7"
                    autoFocus
                  />
                </div>
              </div>

              <div className="max-h-[240px] overflow-y-auto py-1">
                {!hasResults ? (
                  <p className="px-3 py-4 text-sm text-center text-muted-foreground">No matches</p>
                ) : (
                  filtered.map(entry => (
                    <button
                      key={entry.name}
                      type="button"
                      onClick={() => handleSelect(entry)}
                      className="w-full flex items-center px-3 py-2 text-left hover:bg-muted/60 transition-colors"
                    >
                      <div className="flex-1 min-w-0">
                        <p className="font-mono text-xs font-medium text-foreground truncate">{entry.name}</p>
                        {entry.description && (
                          <p className="text-[11px] text-muted-foreground truncate mt-0.5">{entry.description}</p>
                        )}
                      </div>
                    </button>
                  ))
                )}
              </div>
            </>
          )}
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>

    <NewEntryDialog
      open={newVarOpen}
      isPending={createMutation.isPending}
      accountName={accountName}
      onClose={() => setNewVarOpen(false)}
      onCreate={input => createMutation.mutate(input, { onSuccess: () => setNewVarOpen(false) })}
    />
    </>
  )
}

// Chip shown in the input field when a vault ref is active.
// invalid=true means the referenced variable doesn't exist in the target account.
export function VaultRefChip({ token, onClear, invalid }: { token: string; onClear: () => void; invalid?: boolean }) {
  const parsed = parseVaultToken(token)
  if (!parsed) return null

  const chipClass = cn(
    "group flex items-center rounded px-2 py-0.5 border transition-colors",
    invalid
      ? "border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-950/40 hover:border-red-400"
      : "border-teal-200 dark:border-teal-800 bg-teal-50 dark:bg-teal-950/40 hover:border-teal-300"
  )
  const textClass = cn(
    "font-mono text-xs font-medium",
    invalid ? "text-red-700 dark:text-red-300" : "text-teal-700 dark:text-teal-300"
  )
  const iconClass = cn(
    "size-3 shrink-0",
    invalid ? "text-red-500 dark:text-red-400" : "text-teal-500 dark:text-teal-400"
  )

  return (
    <div className="flex items-center h-11 flex-1 min-w-0 px-3 rounded-sm border border-border bg-[var(--input-background)]">
      <button
        type="button"
        onClick={onClear}
        className={chipClass}
        aria-label="Clear vault reference"
      >
        <span className={textClass}>{parsed.name}</span>
        <span className="w-0 ml-0 overflow-hidden opacity-0 group-hover:w-3 group-hover:ml-1.5 group-hover:opacity-100 transition-all duration-150">
          <X className={iconClass} />
        </span>
      </button>
    </div>
  )
}
