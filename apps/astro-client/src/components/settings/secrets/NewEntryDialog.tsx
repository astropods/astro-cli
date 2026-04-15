import { useState, useEffect, useMemo, useRef } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Textarea } from '@/components/ui/textarea'
import { Loader2, Info, Eye, EyeOff, Plus, Trash2, Import, UploadCloud, ClipboardPaste } from 'lucide-react'
import type { CreateAccountVariableInput } from '@/lib/api'
import { VARIABLE_NAME_PATTERN } from '@/lib/vault'
import { parseEnvLines } from '@/components/deploy/parse-env'

interface NewEntryDialogProps {
  open: boolean
  isPending?: boolean
  accountName?: string
  onClose: () => void
  onCreate: (data: CreateAccountVariableInput[]) => void
}

type EntryRow = {
  id: string
  name: string
  value: string
  secret: boolean
  description: string
  showDescription: boolean
}

const ALLOWED_FILE_PATTERN = /(\.(env|json|txt)(\.?\w*)$)|(^\.env$)/i
const MAX_FILE_SIZE = 256 * 1024
const MAX_ROWS = 30
const ROW_GRID = 'grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'

function newRow(partial?: Partial<EntryRow>): EntryRow {
  return {
    id: crypto.randomUUID(),
    name: '',
    value: '',
    secret: true,
    description: '',
    showDescription: false,
    ...partial,
  }
}

export function NewEntryDialog({ open, isPending, accountName, onClose, onCreate }: NewEntryDialogProps) {
  const [rows, setRows] = useState<EntryRow[]>([newRow()])
  const [revealedById, setRevealedById] = useState<Record<string, boolean>>({})
  const [importCount, setImportCount] = useState<number | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [pasteDialogOpen, setPasteDialogOpen] = useState(false)
  const [touched, setTouched] = useState<Set<string>>(new Set())
  const [submitAttempted, setSubmitAttempted] = useState(false)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const scrollRef = useRef<HTMLDivElement | null>(null)

  const markTouched = (field: string) => {
    setTouched((prev) => {
      if (prev.has(field)) return prev
      const next = new Set(prev)
      next.add(field)
      return next
    })
  }

  const isTouched = (field: string) => submitAttempted || touched.has(field)

  useEffect(() => {
    if (!open) {
      setRows([newRow()])
      setRevealedById({})
      setImportCount(null)
      setFileError(null)
      setPasteDialogOpen(false)
      setTouched(new Set())
      setSubmitAttempted(false)
    }
  }, [open])

  const activeRows = useMemo(
    () => rows.filter((row) => row.name.trim() !== '' || row.value.trim() !== ''),
    [rows],
  )

  const nameCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const row of activeRows) {
      const key = row.name.trim()
      if (!key) continue
      counts.set(key, (counts.get(key) ?? 0) + 1)
    }
    return counts
  }, [activeRows])

  const duplicateNames = useMemo(
    () => new Set(Array.from(nameCounts.entries()).filter(([, c]) => c > 1).map(([name]) => name)),
    [nameCounts],
  )

  const invalidKeyCount = useMemo(
    () => activeRows.filter((row) => !VARIABLE_NAME_PATTERN.test(row.name.trim())).length,
    [activeRows],
  )

  const emptyValueCount = useMemo(
    () => activeRows.filter((row) => row.value.trim() === '').length,
    [activeRows],
  )

  const saveEntries = useMemo<CreateAccountVariableInput[]>(
    () =>
      activeRows
        .filter((row) => row.name.trim() && row.value.trim())
        .map((row) => ({
          name: row.name.trim(),
          value: row.value,
          secret: row.secret,
          description: row.description.trim() || undefined,
        })),
    [activeRows],
  )

  const hasErrors =
    saveEntries.length === 0 ||
    invalidKeyCount > 0 ||
    duplicateNames.size > 0 ||
    emptyValueCount > 0

  const updateRow = (id: string, patch: Partial<EntryRow>) => {
    setRows((prev) => prev.map((row) => (row.id === id ? { ...row, ...patch } : row)))
  }

  const removeRow = (id: string) => {
    setRows((prev) => {
      const next = prev.filter((row) => row.id !== id)
      return next.length === 0 ? [newRow()] : next
    })
    setRevealedById((prev) => {
      const next = { ...prev }
      delete next[id]
      return next
    })
    setTouched((prev) => {
      const next = new Set(prev)
      next.delete(`${id}:key`)
      next.delete(`${id}:value`)
      return next
    })
  }

  const toggleReveal = (id: string) => {
    setRevealedById((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const applyEnvText = (text: string) => {
    const parsed = parseEnvLines(text).filter((line) => line.valid)
    if (parsed.length === 0) {
      setFileError('No valid KEY=VALUE pairs found.')
      return
    }
    const byName = new Map<string, EntryRow>()
    for (const line of parsed) {
      const name = line.name.replace(/\s+/g, '_')
      byName.set(name, newRow({ name, value: line.value, secret: true }))
    }
    const incoming = Array.from(byName.values())
    setRows((prev) => {
      const existing = prev.filter((r) => r.name.trim() !== '' || r.value.trim() !== '')
      const combined = existing.length > 0 ? [...existing, ...incoming] : incoming
      if (combined.length > MAX_ROWS) {
        setFileError(`Only ${MAX_ROWS} variables allowed at a time. ${combined.length - MAX_ROWS} entries were dropped.`)
        return combined.slice(0, MAX_ROWS)
      }
      return combined
    })
    setFileError(null)
    setImportCount(incoming.length)
    // Mark imported rows as touched so validation errors show immediately
    setTouched((prev) => {
      const next = new Set(prev)
      for (const row of incoming) {
        next.add(`${row.id}:key`)
        next.add(`${row.id}:value`)
      }
      return next
    })
    requestAnimationFrame(() => {
      scrollRef.current?.scrollTo(0, scrollRef.current.scrollHeight)
    })
  }

  const processFile = (file: File) => {
    if (!ALLOWED_FILE_PATTERN.test(file.name)) {
      setFileError(`"${file.name}" isn't supported. Use .env, .json, or .txt.`)
      return
    }
    if (file.size > MAX_FILE_SIZE) {
      setFileError('File is too large. Maximum size is 256 KB.')
      return
    }
    const reader = new FileReader()
    reader.onload = (event) => {
      const text = event.target?.result
      if (typeof text === 'string') applyEnvText(text)
    }
    reader.readAsText(file)
  }

  const handleCreate = () => {
    if (hasErrors) {
      setSubmitAttempted(true)
      requestAnimationFrame(() => {
        const firstError = scrollRef.current?.querySelector('[aria-invalid="true"]')
        firstError?.scrollIntoView({ block: 'center' })
      })
      return
    }
    onCreate(saveEntries)
  }

  const handleClose = () => onClose()

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[720px] max-h-[85vh] flex flex-col overflow-hidden">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <DialogTitle>New variable</DialogTitle>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button type="button" variant="outline" size="sm">
                  <Import className="size-3.5" />
                  Import .env
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => fileInputRef.current?.click()}>
                  <UploadCloud className="size-3.5" />
                  Upload file
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => setPasteDialogOpen(true)}>
                  <ClipboardPaste className="size-3.5" />
                  Paste text
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          {accountName && (
            <p className="text-xs text-muted-foreground">Saving to <span className="font-medium text-foreground">{accountName}</span></p>
          )}
        </DialogHeader>

        <div ref={scrollRef} className="space-y-3 py-1 min-h-0 flex-1 overflow-y-auto pr-1 pl-1 -ml-1">
          {rows.map((row, index) => {
            const trimmedName = row.name.trim()
            const isActive = trimmedName !== '' || row.value.trim() !== ''
            const invalidKey = trimmedName !== '' && !VARIABLE_NAME_PATTERN.test(trimmedName)
            const duplicateKey = trimmedName !== '' && duplicateNames.has(trimmedName)
            const emptyKey = isActive && trimmedName === ''
            const emptyValue = isActive && row.value.trim() === ''
            const keyTouched = isTouched(`${row.id}:key`)
            const valueTouched = isTouched(`${row.id}:value`)
            const showKeyError = keyTouched && (invalidKey || duplicateKey || emptyKey)
            const showValueError = valueTouched && emptyValue
            return (
              <div key={row.id} className="space-y-1">
                <div className={`grid ${ROW_GRID} gap-2 items-start`}>
                  <div className="space-y-1 min-w-0">
                    <Label size="md" htmlFor={`key-${row.id}`}>Key</Label>
                    <Input
                      id={`key-${row.id}`}
                      value={row.name}
                      onPaste={(e) => {
                        const text = e.clipboardData.getData('text')
                        const lines = text.split(/\r?\n/).filter((l) => l.trim() && !l.trim().startsWith('#'))
                        if (lines.length > 1 && lines.some((l) => l.includes('='))) {
                          e.preventDefault()
                          applyEnvText(text)
                        }
                      }}
                      onChange={(e) => updateRow(row.id, { name: e.target.value.replace(/\s+/g, '_') })}
                      onBlur={() => markTouched(`${row.id}:key`)}
                      placeholder="CLIENT_KEY..."
                      className="h-10 font-mono text-xs"
                      autoFocus={index === 0}
                      aria-invalid={showKeyError || undefined}
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-6 px-0 text-xs font-medium text-muted-foreground hover:text-foreground"
                      onClick={() =>
                        updateRow(row.id, {
                          showDescription: !row.showDescription,
                        })
                      }
                    >
                      {row.showDescription || row.description ? 'Hide description' : 'Add description'}
                    </Button>
                    {showKeyError && emptyKey && <p className="text-[11px] text-destructive">Key is required</p>}
                    {showKeyError && invalidKey && <p className="text-[11px] text-destructive">Invalid key format</p>}
                    {showKeyError && duplicateKey && <p className="text-[11px] text-destructive">Duplicate key</p>}
                  </div>
                  <div className="space-y-1 min-w-0">
                    <Label size="md" htmlFor={`value-${row.id}`}>Value</Label>
                    <div className="relative">
                      <Input
                        id={`value-${row.id}`}
                        type={row.secret && !revealedById[row.id] ? 'password' : 'text'}
                        value={row.value}
                        onChange={(e) => updateRow(row.id, { value: e.target.value })}
                        onBlur={() => markTouched(`${row.id}:value`)}
                        className={row.secret ? 'h-10 font-mono text-xs pr-8' : 'h-10 font-mono text-xs'}
                        autoComplete="off"
                        spellCheck={false}
                        aria-invalid={showValueError || undefined}
                      />
                      {row.secret && (
                        <button
                          type="button"
                          onClick={() => toggleReveal(row.id)}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                          aria-label={revealedById[row.id] ? 'Hide value' : 'Reveal value'}
                        >
                          {revealedById[row.id] ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                        </button>
                      )}
                    </div>
                    {showValueError && <p className="text-[11px] text-destructive">Value is required</p>}
                  </div>
                  <div className="flex items-center justify-end gap-2 h-10 mt-6">
                    <div className="flex items-center gap-1.5 shrink-0">
                      <label htmlFor={`secret-toggle-${row.id}`} className="text-sm font-medium text-foreground cursor-pointer whitespace-nowrap">Secret</label>
                      <TooltipProvider delayDuration={200}>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Info className="size-3.5 text-muted-foreground cursor-help" />
                          </TooltipTrigger>
                          <TooltipContent side="top" className="max-w-[200px] text-xs text-center">
                            Hides the value permanently after saving. Can't be read or recovered.
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                      <Switch
                        id={`secret-toggle-${row.id}`}
                        checked={row.secret}
                        onCheckedChange={(checked) => updateRow(row.id, { secret: checked })}
                      />
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => removeRow(row.id)}
                      aria-label={`Remove ${row.name || 'row'}`}
                    >
                      <Trash2 className="size-3.5 text-muted-foreground" />
                    </Button>
                  </div>
                </div>
                {(row.showDescription || row.description) && (
                  <div className={`grid ${ROW_GRID} gap-2`}>
                    <div className="col-span-2">
                      <Input
                        value={row.description}
                        onChange={(e) => updateRow(row.id, { description: e.target.value })}
                        placeholder="e.g. Database key from Supabase..."
                        className="h-9 text-sm"
                      />
                    </div>
                  </div>
                )}
              </div>
            )
          })}

          {rows.length < MAX_ROWS && (
            <Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => {
              setRows((prev) => [...prev, newRow()])
              requestAnimationFrame(() => {
                scrollRef.current?.scrollTo(0, scrollRef.current.scrollHeight)
              })
            }}>
              <Plus className="size-3.5" />
              Add another
            </Button>
          )}

        </div>

        {(importCount != null || fileError) && (
          <div className="space-y-1">
            {importCount != null && (
              <p className="text-xs text-green-600">
                Loaded {importCount} {importCount === 1 ? 'variable' : 'variables'}
              </p>
            )}
            {fileError && <p className="text-xs text-destructive">{fileError}</p>}
          </div>
        )}

        <input
          ref={fileInputRef}
          type="file"
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0]
            if (file) processFile(file)
            e.target.value = ''
          }}
        />

        <PasteEnvDialog
          open={pasteDialogOpen}
          onClose={() => setPasteDialogOpen(false)}
          onApply={(text) => {
            applyEnvText(text)
            setPasteDialogOpen(false)
          }}
        />

        <DialogFooter>
          {submitAttempted && hasErrors && (
            <p className="text-xs text-destructive mr-auto self-center">Fix the errors above before saving</p>
          )}
          <Button variant="outline" onClick={handleClose} disabled={isPending}>Cancel</Button>
          <Button onClick={handleCreate} disabled={isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PasteEnvDialog({ open, onClose, onApply }: { open: boolean; onClose: () => void; onApply: (text: string) => void }) {
  const [text, setText] = useState('')

  useEffect(() => {
    if (!open) setText('')
  }, [open])

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Paste .env content</DialogTitle>
          <DialogDescription>
            Paste KEY=VALUE pairs and they'll be added as rows.
          </DialogDescription>
        </DialogHeader>
        <Textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={'DATABASE_URL=postgresql://...\nAPI_KEY=sk-...\nAPP_ENV=production'}
          className="min-h-[160px] font-mono text-xs"
          autoFocus
        />
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button onClick={() => onApply(text)} disabled={text.trim().length === 0}>
            Apply
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
