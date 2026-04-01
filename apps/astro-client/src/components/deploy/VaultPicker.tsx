import { useState } from 'react'
import { KeyRound, X } from 'lucide-react'
import { Popover as PopoverPrimitive } from 'radix-ui'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useAuth } from '@/lib/auth'
import { useAccountVariables } from '@/api/queries'
import type { AccountVariable } from '@/lib/api'

// Parse a token like {{secrets.FOO}} or {{vars.BAR}} into its parts
export function parseVaultToken(token: string): { type: 'secret' | 'variable'; name: string } | null {
  const match = token.match(/^\{\{(secrets|vars)\.([A-Z0-9_]+)\}\}$/)
  if (!match) return null
  return { type: match[1] === 'secrets' ? 'secret' : 'variable', name: match[2] }
}

interface VaultPickerProps {
  onSelect: (token: string) => void
}

export function VaultPicker({ onSelect }: VaultPickerProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')

  const { personalAccount } = useAuth()
  const accountName = personalAccount?.name ?? ''
  const { data } = useAccountVariables(accountName)
  const entries = data?.variables ?? []

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
          <KeyRound className="size-3.5" />
        </Button>
      </PopoverPrimitive.Trigger>

      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          sideOffset={6}
          align="end"
          className="z-50 w-[280px] rounded-lg border border-border bg-popover shadow-lg overflow-hidden data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
        >
          <div className="px-3 pt-3 pb-2">
            <p className="text-xs font-semibold text-foreground">Reference an existing value</p>
          </div>

          <div className="px-2 pb-2 border-b border-border">
            <Input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search variables and secrets..."
              className="h-8 text-sm"
              autoFocus
            />
          </div>

          <div className="max-h-[240px] overflow-y-auto py-1">
            {!hasResults ? (
              <p className="px-3 py-4 text-sm text-center text-muted-foreground">
                {search ? 'No matches' : 'No vault entries'}
              </p>
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
                      <p className="text-[10px] text-muted-foreground truncate mt-0.5">{entry.description}</p>
                    )}
                  </div>
                </button>
              ))
            )}
          </div>
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  )
}

// Chip shown in the input field when a vault ref is active
export function VaultRefChip({ token, onClear }: { token: string; onClear: () => void }) {
  const parsed = parseVaultToken(token)
  const [hovered, setHovered] = useState(false)
  if (!parsed) return null

  return (
    <div className="flex items-center h-11 flex-1 min-w-0 px-3 rounded-sm border border-border bg-[var(--input-background)]">
      <button
        type="button"
        onClick={onClear}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        className="flex items-center gap-1.5 rounded px-2 py-0.5 border border-teal-200 dark:border-teal-800 bg-teal-50 dark:bg-teal-950/40 hover:border-teal-300 transition-colors"
        aria-label="Clear vault reference"
      >
        <span className="font-mono text-xs font-medium text-teal-700 dark:text-teal-300">
          {parsed.name}
        </span>
        {hovered && <X className="size-3 text-teal-500 dark:text-teal-400 shrink-0" />}
      </button>
    </div>
  )
}
