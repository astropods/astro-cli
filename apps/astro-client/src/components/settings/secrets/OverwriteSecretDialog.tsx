import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Loader2 } from 'lucide-react'

interface OverwriteSecretDialogProps {
  secretName: string
  description?: string
  open: boolean
  isPending?: boolean
  onClose: () => void
  onConfirm: (data: { value?: string; description: string }) => void
}

export function OverwriteSecretDialog({ secretName, description: initialDescription = '', open, isPending, onClose, onConfirm }: OverwriteSecretDialogProps) {
  const [value, setValue] = useState('')
  const [description, setDescription] = useState(initialDescription)

  const trimmedDescription = description.trim()
  const descriptionChanged = trimmedDescription !== initialDescription.trim()
  const canSave = Boolean(value.trim()) || descriptionChanged

  const handleClose = () => {
    setValue('')
    setDescription(initialDescription)
    onClose()
  }

  const handleConfirm = () => {
    if (!canSave) return
    onConfirm({
      ...(value.trim() ? { value } : {}),
      description: trimmedDescription,
    })
  }

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[420px]">
        <DialogHeader>
          <DialogTitle>
            Edit{' '}
            <span className="font-mono text-sm font-medium">{secretName}</span>
          </DialogTitle>
          <DialogDescription>
            Leave the value blank to keep the current secret. Enter a new value to replace it permanently.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-1">
          <div className="space-y-1.5">
            <Label size="md" htmlFor="overwrite-value">Value</Label>
            <Input
              id="overwrite-value"
              type="password"
              value={value}
              onChange={e => setValue(e.target.value)}
              placeholder="••••••••••••"
              autoComplete="off"
              data-1p-ignore
              autoFocus
            />
          </div>
          <div className="space-y-1.5">
            <Label size="md" htmlFor="overwrite-description">
              Description <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="overwrite-description"
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="What is this used for?"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isPending}>Cancel</Button>
          <Button onClick={handleConfirm} disabled={!canSave || isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
