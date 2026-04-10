import { useState, useEffect, useMemo, useRef } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Textarea } from '@/components/ui/textarea'
import { Loader2, Info, Eye, EyeOff, Plus, Trash2, UploadCloud } from 'lucide-react'
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
}

const ALLOWED_FILE_PATTERN = /(\.(env|json|txt)(\.?\w*)$)|(^\.env)/i
const MAX_FILE_SIZE = 256 * 1024
const ROW_GRID = 'grid-cols-[minmax(0,1fr)_minmax(0,1fr)_170px]'

function newRow(partial?: Partial<EntryRow>): EntryRow {
  return {
    id: crypto.randomUUID(),
    name: '',
    value: '',
    secret: true,
    ...partial,
  }
}

export function NewEntryDialog({ open, isPending, accountName, onClose, onCreate }: NewEntryDialogProps) {
  const [rows, setRows] = useState<EntryRow[]>([newRow()])
  const [revealedById, setRevealedById] = useState<Record<string, boolean>>({})
  const [fileName, setFileName] = useState<string | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [showPasteRaw, setShowPasteRaw] = useState(false)
  const [rawText, setRawText] = useState('')
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    if (!open) {
      setRows([newRow()])
      setRevealedById({})
      setFileName(null)
      setFileError(null)
      setShowPasteRaw(false)
      setRawText('')
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
        })),
    [activeRows],
  )

  const canSave =
    saveEntries.length > 0 &&
    invalidKeyCount === 0 &&
    duplicateNames.size === 0 &&
    emptyValueCount === 0

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
  }

  const toggleReveal = (id: string) => {
    setRevealedById((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const applyEnvText = (text: string) => {
    const parsed = parseEnvLines(text).map((line) => ({
      ...line,
      valid: line.valid && VARIABLE_NAME_PATTERN.test(line.name),
    }))
    const valid = parsed.filter((line) => line.valid)
    if (valid.length === 0) {
      setFileError('No valid KEY=VALUE pairs found.')
      return
    }
    const byName = new Map<string, EntryRow>()
    for (const line of valid) {
      byName.set(line.name, newRow({ name: line.name, value: line.value, secret: true }))
    }
    setRows(Array.from(byName.values()))
    setRevealedById({})
    setFileError(null)
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
    setFileName(file.name)
    const reader = new FileReader()
    reader.onload = (event) => {
      const text = event.target?.result
      if (typeof text === 'string') applyEnvText(text)
    }
    reader.readAsText(file)
  }

  const handleCreate = () => {
    if (!canSave) return
    onCreate(saveEntries)
  }

  const handleClose = () => {
    setRows([newRow()])
    setRevealedById({})
    setFileName(null)
    setFileError(null)
    setShowPasteRaw(false)
    setRawText('')
    onClose()
  }

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[720px]">
        <DialogHeader>
          <DialogTitle>New variable</DialogTitle>
          {accountName && (
            <p className="text-xs text-muted-foreground">Saving to <span className="font-medium text-foreground">{accountName}</span></p>
          )}
        </DialogHeader>

        <div className="space-y-3 py-1 max-h-[65vh] overflow-y-auto pr-1">
          <div className={`grid ${ROW_GRID} gap-2 items-center`}>
            <Label size="sm">Key</Label>
            <Label size="sm">Value</Label>
            <span />
          </div>

          {rows.map((row, index) => {
            const trimmedName = row.name.trim()
            const invalidKey = trimmedName !== '' && !VARIABLE_NAME_PATTERN.test(trimmedName)
            const duplicateKey = trimmedName !== '' && duplicateNames.has(trimmedName)
            return (
              <div key={row.id} className="space-y-1">
                <div className={`grid ${ROW_GRID} gap-2 items-start`}>
                  <div className="space-y-1 min-w-0">
                    <Input
                      value={row.name}
                      onPaste={(e) => {
                        const text = e.clipboardData.getData('text')
                        if (text.includes('=')) {
                          e.preventDefault()
                          applyEnvText(text)
                        }
                      }}
                      onChange={(e) => updateRow(row.id, { name: e.target.value.toUpperCase().replace(/\s+/g, '_') })}
                      placeholder="CLIENT_KEY..."
                      className="h-10 font-mono text-xs"
                      autoFocus={index === 0}
                      aria-invalid={invalidKey || duplicateKey || undefined}
                    />
                    {invalidKey && <p className="text-[11px] text-destructive">Invalid key format</p>}
                    {duplicateKey && <p className="text-[11px] text-destructive">Duplicate key</p>}
                  </div>
                  <div className="space-y-1 min-w-0">
                    <div className="relative">
                      <Input
                        type={row.secret && !revealedById[row.id] ? 'password' : 'text'}
                        value={row.value}
                        onChange={(e) => updateRow(row.id, { value: e.target.value })}
                        className={row.secret ? 'h-10 font-mono text-xs pr-8' : 'h-10 font-mono text-xs'}
                        autoComplete="off"
                        spellCheck={false}
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
                  </div>
                  <div className="flex items-center gap-2 h-10">
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
              </div>
            )
          })}

          <Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => setRows((prev) => [...prev, newRow()])}>
            <Plus className="size-3.5" />
            Add another
          </Button>

        </div>

        <div className="space-y-2 border-t border-border pt-3">
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
              <UploadCloud className="size-3.5" />
              Import .env
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => setShowPasteRaw((prev) => !prev)}>
              Or paste
            </Button>
          </div>

          {fileName && <p className="text-xs text-muted-foreground">Loaded {fileName}</p>}

          {showPasteRaw && (
            <div className="space-y-2">
              <Textarea
                value={rawText}
                onChange={(e) => setRawText(e.target.value)}
                placeholder="PASTE .env CONTENT HERE"
                className="min-h-[96px] font-mono text-xs"
              />
              <div className="flex justify-end">
                <Button
                  type="button"
                  size="sm"
                  onClick={() => {
                    applyEnvText(rawText)
                  }}
                  disabled={rawText.trim().length === 0}
                >
                  Apply
                </Button>
              </div>
            </div>
          )}

          {fileError && <p className="text-xs text-destructive">{fileError}</p>}
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
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isPending}>Cancel</Button>
          <Button onClick={handleCreate} disabled={!canSave || isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
