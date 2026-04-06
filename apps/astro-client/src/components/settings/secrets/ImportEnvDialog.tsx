import { useState, useMemo } from 'react'
import { WarningPanel } from '@/components/ui/status-panel'
import { InlineBadge } from '@/components/InlineBadge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Loader2 } from 'lucide-react'
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

export function ImportEnvDialog({ open, isPending, existingNames, onClose, onImport }: ImportEnvDialogProps) {
  const [raw, setRaw] = useState('')
  const [importAsSecret, setImportAsSecret] = useState(false)

  const parsed = useMemo(
    () => raw.trim()
      ? parseEnvLines(raw).map(l => ({ ...l, valid: l.valid && VARIABLE_NAME_PATTERN.test(l.name) }))
      : [],
    [raw],
  )
  const validLines = useMemo(() => parsed.filter(l => l.valid), [parsed])
  const conflicts = useMemo(
    () => validLines.filter(l => existingNames.includes(l.name)),
    [validLines, existingNames],
  )

  const handleImport = () => {
    const entries: CreateAccountVariableInput[] = validLines.map(line => ({
      name: line.name,
      value: line.value,
      secret: importAsSecret,
    }))
    onImport(entries)
  }

  const handleClose = () => {
    setRaw('')
    setImportAsSecret(false)
    onClose()
  }

  const noun = importAsSecret ? 'secret' : 'variable'

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[520px]">
        <DialogHeader>
          <DialogTitle>Import from .env</DialogTitle>
          <DialogDescription>
            Paste the contents of a .env file to bulk-import entries.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-1">
          {/* Import as toggle */}
          <div className="space-y-2">
            <Label size="md">Import as</Label>
            <div className="flex gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="import-as"
                  checked={!importAsSecret}
                  onChange={() => setImportAsSecret(false)}
                  className="accent-teal-600"
                />
                <span className="text-sm">Variables <span className="text-muted-foreground">(values visible)</span></span>
              </label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="import-as"
                  checked={importAsSecret}
                  onChange={() => setImportAsSecret(true)}
                  className="accent-teal-600"
                />
                <span className="text-sm">Secrets <span className="text-muted-foreground">(values encrypted)</span></span>
              </label>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label size="md" htmlFor="env-content">Paste .env content</Label>
            <Textarea
              id="env-content"
              value={raw}
              onChange={e => setRaw(e.target.value)}
              placeholder={'BASE_URL=https://api.example.com\nMAX_RETRIES=3\nAPI_KEY=sk-...'}
              className="font-mono text-xs min-h-[140px] resize-none"
              autoFocus
            />
          </div>

          {parsed.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs text-muted-foreground">
                {validLines.length} {noun}{validLines.length !== 1 ? 's' : ''} will be imported
                {parsed.length !== validLines.length && (
                  <span className="text-destructive">
                    {' '}· {parsed.length - validLines.length} line{parsed.length - validLines.length !== 1 ? 's' : ''} skipped (invalid format)
                  </span>
                )}
              </p>

              <div className="rounded-md border border-border overflow-hidden max-h-[180px] overflow-y-auto divide-y divide-border">
                {parsed.map((line, i) => (
                  <div
                    key={i}
                    className={`flex items-center gap-3 px-3 py-2 ${!line.valid ? 'opacity-40' : ''}`}
                  >
                    <span className="font-mono text-xs font-medium text-foreground truncate flex-1">
                      {line.name || <span className="text-muted-foreground italic">invalid</span>}
                    </span>
                    <span className="font-mono text-xs text-muted-foreground truncate max-w-[160px]">
                      {importAsSecret ? '••••••••' : (line.value || '—')}
                    </span>
                    {conflicts.find(c => c.name === line.name) && (
                      <InlineBadge variant="soft" className="text-amber-700 dark:text-amber-300 border-amber-300 dark:border-amber-700 shrink-0">
                        overwrites existing
                      </InlineBadge>
                    )}
                  </div>
                ))}
              </div>

              {conflicts.length > 0 && (
                <WarningPanel variant="inline">
                  {conflicts.length} {noun}{conflicts.length !== 1 ? 's' : ''} will overwrite existing values.
                </WarningPanel>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isPending}>Cancel</Button>
          <Button onClick={handleImport} disabled={validLines.length === 0 || isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Import {validLines.length > 0 ? `${validLines.length} ${noun}${validLines.length !== 1 ? 's' : ''}` : ''}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
