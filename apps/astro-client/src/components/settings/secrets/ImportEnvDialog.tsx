import { useMemo, useRef, useState } from 'react'
import { InlineBadge } from '@/components/InlineBadge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Loader2, UploadCloud, Trash2, Eye, EyeOff, Plus } from 'lucide-react'
import type { CreateAccountVariableInput } from '@/lib/api'
import { parseEnvLines } from '@/components/deploy/parse-env'
import { VARIABLE_NAME_PATTERN } from '@/lib/vault'

interface ImportEnvDialogProps {
  open: boolean
  isPending?: boolean
  existingNames: string[]
  onClose: () => void
  onImport: (entries: CreateAccountVariableInput[]) => void
}

interface RowEntry {
  id: string
  name: string
  value: string
  secret: boolean
}

const ALLOWED_FILE_PATTERN = /(\.(env|json|txt)(\.?\w*)$)|(^\.env)/i
const MAX_FILE_SIZE = 256 * 1024

const createRow = (partial?: Partial<RowEntry>): RowEntry => ({
  id: crypto.randomUUID(),
  name: '',
  value: '',
  secret: true,
  ...partial,
})

function parseEnvToRows(text: string): { rows: RowEntry[]; invalidCount: number; duplicateCount: number } {
  const parsed = parseEnvLines(text).map((line) => ({
    ...line,
    valid: line.valid && VARIABLE_NAME_PATTERN.test(line.name),
  }))
  const valid = parsed.filter((line) => line.valid)
  const byName = new Map<string, RowEntry>()
  for (const line of valid) {
    byName.set(line.name, createRow({ name: line.name, value: line.value, secret: true }))
  }
  const rows = Array.from(byName.values())
  return {
    rows,
    invalidCount: parsed.length - valid.length,
    duplicateCount: valid.length - rows.length,
  }
}

export function ImportEnvDialog({ open, isPending, existingNames, onClose, onImport }: ImportEnvDialogProps) {
  const [rows, setRows] = useState<RowEntry[]>([createRow()])
  const [fileName, setFileName] = useState<string | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [summary, setSummary] = useState<{ imported: number; invalid: number; duplicates: number } | null>(null)
  const [revealedById, setRevealedById] = useState<Record<string, boolean>>({})

  const fileInputRef = useRef<HTMLInputElement | null>(null)

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
    () => new Set(Array.from(nameCounts.entries()).filter(([, count]) => count > 1).map(([name]) => name)),
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
  const overwriteCount = useMemo(
    () => activeRows.filter((row) => row.name.trim() && existingNames.includes(row.name.trim())).length,
    [activeRows, existingNames],
  )

  const entriesToImport = useMemo<CreateAccountVariableInput[]>(
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

  const canImport =
    entriesToImport.length > 0 &&
    invalidKeyCount === 0 &&
    duplicateNames.size === 0 &&
    emptyValueCount === 0

  const resetState = () => {
    setRows([createRow()])
    setFileName(null)
    setFileError(null)
    setSummary(null)
    setRevealedById({})
  }

  const handleClose = () => {
    resetState()
    onClose()
  }

  const applyParsedText = (text: string) => {
    const { rows: parsedRows, invalidCount, duplicateCount } = parseEnvToRows(text)
    if (parsedRows.length === 0) {
      setFileError('No valid KEY=VALUE pairs found.')
      return
    }
    setRows(parsedRows)
    setRevealedById({})
    setFileError(null)
    setSummary({
      imported: parsedRows.length,
      invalid: invalidCount,
      duplicates: duplicateCount,
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

    setFileName(file.name)
    const reader = new FileReader()
    reader.onload = (event) => {
      const text = event.target?.result
      if (typeof text === 'string') {
        applyParsedText(text)
      }
    }
    reader.readAsText(file)
  }

  const updateRow = (id: string, patch: Partial<RowEntry>) => {
    setRows((prev) => prev.map((row) => (row.id === id ? { ...row, ...patch } : row)))
  }

  const removeRow = (id: string) => {
    setRows((prev) => {
      const next = prev.filter((row) => row.id !== id)
      return next.length === 0 ? [createRow()] : next
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

  const handleKeyPaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text')
    if (text.includes('=')) {
      e.preventDefault()
      applyParsedText(text)
    }
  }

  const handleImport = () => {
    if (!canImport) return
    onImport(entriesToImport)
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && handleClose()}>
      <DialogContent className="max-w-[640px]">
        <DialogHeader>
          <DialogTitle>Import from .env</DialogTitle>
          <DialogDescription>
            Upload a file or paste .env content in any Key field. You can edit all values before importing.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-1">
          <div
            className="rounded-xl border border-border bg-background p-5 cursor-pointer hover:bg-muted/20 transition-colors"
            onClick={() => fileInputRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault()
              e.stopPropagation()
            }}
            onDrop={(e) => {
              e.preventDefault()
              e.stopPropagation()
              const file = e.dataTransfer.files?.[0]
              if (file) processFile(file)
            }}
          >
            <div className="flex flex-col items-center justify-center gap-2 text-center">
              <UploadCloud className="size-7 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">
                {fileName ? `Loaded ${fileName}` : 'Click or drag and drop a .env file.'}
              </p>
              <p className="text-xs text-muted-foreground">Also supports .json and .txt</p>
            </div>
            {fileError && <p className="mt-2 text-xs text-destructive">{fileError}</p>}
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

          {summary && (
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
              <p className="text-xs text-foreground">{summary.imported} ready to import</p>
              {summary.invalid > 0 && <p className="text-xs text-foreground">{summary.invalid} skipped</p>}
              {summary.duplicates > 0 && <p className="text-xs text-foreground">{summary.duplicates} duplicate resolved</p>}
            </div>
          )}

          <div className="max-h-[360px] overflow-y-auto space-y-2 pr-1">
            {rows.map((row) => {
              const trimmedName = row.name.trim()
              const invalidKey = trimmedName !== '' && !VARIABLE_NAME_PATTERN.test(trimmedName)
              const duplicateKey = trimmedName !== '' && duplicateNames.has(trimmedName)
              const overwrite = trimmedName !== '' && existingNames.includes(trimmedName)
              return (
                <div key={row.id} className="rounded-lg border border-border bg-background p-3 space-y-2">
                  <div className="grid grid-cols-[1fr_1fr_auto] items-start gap-3">
                    <div className="min-w-0 space-y-1">
                      <Label size="sm">Key</Label>
                      <Input
                        value={row.name}
                        onPaste={handleKeyPaste}
                        onChange={(e) =>
                          updateRow(row.id, { name: e.target.value.toUpperCase().replace(/\s+/g, '_') })
                        }
                        className="h-8 font-mono text-xs"
                        autoComplete="off"
                        spellCheck={false}
                        aria-invalid={invalidKey || duplicateKey || undefined}
                      />
                      {overwrite && (
                        <InlineBadge
                          variant="soft"
                          className="mt-0.5"
                          style={{
                            color: 'var(--color-yellow-700)',
                            background: 'color-mix(in oklch, var(--color-yellow-700) 12%, transparent)',
                          }}
                        >
                          Overwrite
                        </InlineBadge>
                      )}
                      {invalidKey && <p className="text-[11px] text-destructive">Invalid key format</p>}
                      {duplicateKey && <p className="text-[11px] text-destructive">Duplicate key</p>}
                    </div>

                    <div className="min-w-0">
                      <Label size="sm">Value</Label>
                      <div className="relative mt-1">
                        <Input
                          value={row.value}
                          type={row.secret && !revealedById[row.id] ? 'password' : 'text'}
                          onChange={(e) => updateRow(row.id, { value: e.target.value })}
                          className={row.secret ? 'h-8 font-mono text-xs pr-8' : 'h-8 font-mono text-xs'}
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

                    <div className="flex items-start gap-2 pt-6">
                      <div className="flex items-center gap-2">
                        <Label size="sm" htmlFor={`secret-toggle-${row.id}`}>
                          Secret
                        </Label>
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
          </div>

          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-fit"
            onClick={() => setRows((prev) => [...prev, createRow()])}
          >
            <Plus className="size-3.5" />
            Add another
          </Button>

          <p className="text-xs text-muted-foreground">
            {invalidKeyCount > 0
              ? `${invalidKeyCount} key${invalidKeyCount === 1 ? '' : 's'} need a valid variable name`
              : duplicateNames.size > 0
              ? `${duplicateNames.size} duplicate key${duplicateNames.size === 1 ? '' : 's'} must be resolved`
              : emptyValueCount > 0
              ? `${emptyValueCount} value${emptyValueCount === 1 ? '' : 's'} cannot be empty`
              : overwriteCount > 0
              ? `${overwriteCount} ${overwriteCount === 1 ? 'entry' : 'entries'} will overwrite existing values`
              : 'No existing values will be overwritten'}
          </p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isPending}>
            Cancel
          </Button>
          <Button onClick={handleImport} disabled={!canImport || isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Import {entriesToImport.length} {entriesToImport.length === 1 ? 'entry' : 'entries'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
