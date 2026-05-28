import { useEffect, useState } from 'react'
import { X, WandSparkles, CaseSensitive, CaseLower } from 'lucide-react'
import { MagnifyingGlassIcon, KeyIcon, PlusIcon } from '@heroicons/react/24/outline'
import { Popover as PopoverPrimitive } from 'radix-ui'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import type { AccountVariable } from '@/lib/api'
import { NewEntryDialog } from '@/components/settings/secrets/NewEntryDialog'
import { useCreateAccountVariables } from '@/api/queries/variables'
import { useAuth } from '@/lib/use-auth'

// Coalesce concurrent switchOrg calls across multiple VaultPicker instances rendered
// on the same page (e.g. one per variable field on a deploy form). WorkOS refresh-token
// rotation cannot tolerate parallel calls for the same target org.
const inflightScopeSwitches = new Map<string, Promise<unknown>>()

// Parse a token like {{secrets.FOO}} or {{vars.BAR}} into its parts
export function parseVaultToken(token: string): { type: 'secret' | 'variable'; name: string } | null {
  const match = token.match(/^\{\{(secrets|vars)\.([a-zA-Z_][a-zA-Z0-9_]*)\}\}$/)
  if (!match) return null
  return { type: match[1] === 'secrets' ? 'secret' : 'variable', name: match[2] }
}

// Build the {{secrets.NAME}} / {{vars.NAME}} token form for a variable.
export function buildVaultToken(name: string, secret: boolean): string {
  return secret ? `{{secrets.${name}}}` : `{{vars.${name}}}`
}

interface VaultPickerProps {
  onSelect: (token: string) => void
  entries?: AccountVariable[]
  accountName?: string
  vaultSettingsUrl?: string
  /** When set, failed vault list fetch — do not show empty-vault copy. */
  loadError?: string | null
  bestMatchNames?: string[]
  possibleMatchNames?: string[]
  selectedName?: string
  open?: boolean
  onOpenChange?: (open: boolean) => void
  /** Form-level handler that maps a name→value record onto matching fields.
   *  When provided and multiple variables are created in one shot, it's used
   *  to fan the new tokens out to all matching fields. */
  bulkSetVariables?: (imported: Record<string, string>) => void
}

export function VaultPicker({ onSelect, entries = [], accountName, vaultSettingsUrl, loadError, bestMatchNames, possibleMatchNames, selectedName, open: controlledOpen, onOpenChange: controlledOnOpenChange, bulkSetVariables }: VaultPickerProps) {
  const [localOpen, setLocalOpen] = useState(false)
  const open = controlledOpen ?? localOpen
  const setOpen = (o: boolean) => { setLocalOpen(o); controlledOnOpenChange?.(o) }
  const [search, setSearch] = useState('')
  const [newVarOpen, setNewVarOpen] = useState(false)
  const createMutation = useCreateAccountVariables(accountName ?? '')

  const { accounts, organizationId, switchOrg } = useAuth()
  const acct = accountName ? accounts.find((a) => a.name === accountName) : undefined
  const targetOrgId =
    acct?.type === 'organization' && acct.organization_id && acct.organization_id !== organizationId
      ? acct.organization_id
      : null
  const scopeReady = targetOrgId === null
  // Mirrors the server's variable:write gate so members of an org (who can read but not write
  // variables) don't see a "+ New" affordance that would 403 on submit. Unknown accounts fall
  // through to true and let the server enforce — but only when the caller actually supplied an
  // account name; without one the create endpoint would 400 ("account name is required") because
  // the URL collapses to `/accounts//variables`.
  const canCreate = !!accountName && (
    !acct ||
    acct.type === 'personal' ||
    acct.role === 'admin' ||
    acct.role === 'owner'
  )

  useEffect(() => {
    if (!targetOrgId) return
    const key = targetOrgId
    let promise = inflightScopeSwitches.get(key)
    if (!promise) {
      promise = switchOrg(targetOrgId)
      inflightScopeSwitches.set(key, promise)
      promise.finally(() => { inflightScopeSwitches.delete(key) })
    }
    promise.catch(() => { /* swallow — gate stays closed until session updates */ })
  }, [targetOrgId, switchOrg])

  const filtered = entries.filter(e =>
    e.name.toLowerCase().includes(search.toLowerCase()) ||
    e.description?.toLowerCase().includes(search.toLowerCase())
  )
  const hasResults = filtered.length > 0

  const handleSelect = (entry: AccountVariable) => {
    onSelect(buildVaultToken(entry.name, entry.secret))
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
            open && 'border-indigo-500 text-indigo-700 dark:text-indigo-400 bg-indigo-50 dark:bg-indigo-950/40'
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
          {loadError ? (
            <div className="px-4 py-5 text-center space-y-2">
              <KeyIcon className="size-5 text-muted-foreground/50 mx-auto" />
              <p className="text-sm font-medium text-foreground">Could not load variables</p>
              <p className="text-xs text-muted-foreground whitespace-pre-wrap">{loadError}</p>
              {vaultSettingsUrl ? (
                <Button size="sm" variant="outline" className="mt-1" asChild>
                  <a href={vaultSettingsUrl}>Variables settings</a>
                </Button>
              ) : null}
            </div>
          ) : entries.length === 0 ? (
            <div className="px-4 py-5 text-center">
              <KeyIcon className="size-5 text-muted-foreground/50 mx-auto mb-2" />
              <p className="text-sm font-medium text-foreground">No variables yet</p>
              <p className="text-xs text-muted-foreground mt-1 mb-3">
                {canCreate
                  ? 'Set and manage reusable credentials and configuration values for your agents'
                  : 'No variables have been added for this account yet.'}
              </p>
              {scopeReady && canCreate && (
                <Button size="sm" onClick={() => { setOpen(false); setNewVarOpen(true) }}>
                  <PlusIcon className="size-3.5" />
                  New variable
                </Button>
              )}
            </div>
          ) : (
            <>
              <div className="px-3 pt-3 pb-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-foreground">Select a reference</p>
                  {scopeReady && canCreate && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-6 px-2 text-xs shrink-0"
                      onClick={() => { setOpen(false); setNewVarOpen(true) }}
                    >
                      <PlusIcon className="size-3" />
                      New
                    </Button>
                  )}
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
                  filtered.map(entry => {
                    const isSelected = entry.name === selectedName
                    const isExactMatch = bestMatchNames?.includes(entry.name)
                    const isPossibleMatch = !isExactMatch && possibleMatchNames?.includes(entry.name)
                    return (
                      <button
                        key={entry.name}
                        type="button"
                        onClick={() => handleSelect(entry)}
                        className={cn(
                          "w-full flex items-center pl-2.5 pr-3 py-2 text-left hover:bg-muted/60 transition-colors border-l-2",
                          isSelected ? "border-indigo-500" : "border-transparent"
                        )}
                      >
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5">
                            <p className="font-mono text-xs font-medium text-foreground truncate flex-1 min-w-0">{entry.name}</p>
                            {isExactMatch && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span className="shrink-0 cursor-default" aria-label="Exact match">
                                    <CaseSensitive className="size-3.5 text-indigo-500 dark:text-indigo-400" />
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent side="top" sideOffset={4}>Exact match</TooltipContent>
                              </Tooltip>
                            )}
                            {isPossibleMatch && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span className="shrink-0 cursor-default" aria-label="Case insensitive match">
                                    <CaseLower className="size-3.5 text-muted-foreground/70" />
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent side="top" sideOffset={4}>Case insensitive match</TooltipContent>
                              </Tooltip>
                            )}
                          </div>
                          {entry.description && (
                            <p className="text-[11px] text-muted-foreground truncate mt-0.5">{entry.description}</p>
                          )}
                        </div>
                      </button>
                    )
                  })
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
      onCreate={(inputs) => {
        createMutation.mutate(inputs, {
          onSuccess: (response) => {
            setNewVarOpen(false)
            // Restrict to inputs the server actually created. If the response
            // omits per-name results, treat all inputs as successful.
            const created = response?.results
              ? inputs.filter((i) => response.results.some((r) => r.name === i.name && r.status === 'created'))
              : inputs
            if (created.length === 0) return
            if (created.length === 1) {
              // Single variable — fill the field that opened the picker.
              onSelect(buildVaultToken(created[0].name, created[0].secret))
            } else if (bulkSetVariables) {
              // Multiple variables — defer to the form's existing name-based
              // mapping logic (same path used by .env import).
              const tokenMap: Record<string, string> = {}
              for (const v of created) tokenMap[v.name] = buildVaultToken(v.name, v.secret)
              bulkSetVariables(tokenMap)
            } else {
              // No form handler available — fall back to filling the current field
              // with the first created variable so creation isn't silently lost.
              onSelect(buildVaultToken(created[0].name, created[0].secret))
            }
          },
        })
      }}
    />
    </>
  )
}

/** Inline badge matching vault auto-fill affordance (WandSparkles + label). */
export function AutoFilledBadge({
  label = 'Auto-filled',
  onClick,
}: {
  label?: string
  onClick?: () => void
}) {
  const className =
    'ml-2 flex items-center gap-1 text-xs text-muted-foreground/60 select-none transition-colors'
  const content = (
    <>
      <WandSparkles className="size-3 shrink-0" />
      {label}
    </>
  )
  if (onClick) {
    return (
      <Button
        type="button"
        variant="ghost"
        size="xs"
        onClick={onClick}
        className={cn(className, 'h-auto min-h-0 p-0 font-normal hover:bg-transparent hover:text-muted-foreground')}
      >
        {content}
      </Button>
    )
  }
  return <span className={cn(className, 'pointer-events-none')}>{content}</span>
}

export const CONFIGURED_INLINE_SECRET_MASK = '•••••••'

/** Masked inline secret saved on a prior deploy (configure/redeploy prefill). */
export function ConfiguredInlineSecretChip({
  label,
  onReplace,
  invalid,
}: {
  label: string
  onReplace: () => void
  invalid?: boolean
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onReplace}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onReplace()
        }
      }}
      className={cn(
        'flex h-11 w-full min-w-0 cursor-pointer items-center rounded-sm border border-input bg-[var(--input-background)] px-3.5 pr-24 text-body shadow-none transition-[color,box-shadow,border-color] outline-none',
        invalid && 'border-destructive',
      )}
      aria-label={`${label}: ${CONFIGURED_INLINE_SECRET_MASK} Auto-filled, click to edit`}
    >
      <span className="font-mono text-sm tracking-wider text-muted-foreground">
        {CONFIGURED_INLINE_SECRET_MASK}
      </span>
      <AutoFilledBadge />
    </div>
  )
}

// Chip shown in the input field when a vault ref is active.
// invalid=true means the referenced variable doesn't exist in the target account.
export function VaultRefChip({ token, onClear, invalid, autoFillLabel, onAutoFillClick }: { token: string; onClear: () => void; invalid?: boolean; autoFillLabel?: string; onAutoFillClick?: () => void }) {
  const parsed = parseVaultToken(token)
  if (!parsed) return null

  const chipClass = cn(
    "group flex items-center rounded px-2 py-0.5 border transition-colors",
    invalid
      ? "border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-950/40 hover:border-red-400"
      : "border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/40 hover:border-slate-300"
  )
  const textClass = cn(
    "font-mono text-xs font-medium",
    invalid ? "text-red-700 dark:text-red-300" : "text-slate-700 dark:text-slate-300"
  )
  const iconClass = cn(
    "size-3 shrink-0",
    invalid ? "text-red-500 dark:text-red-400" : "text-slate-500 dark:text-slate-400"
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
      {autoFillLabel && (
        <AutoFilledBadge label={autoFillLabel} onClick={onAutoFillClick} />
      )}
    </div>
  )
}
