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
import { Loader2, Eye, EyeOff } from 'lucide-react'

interface OverwriteSecretDialogProps {
  secretName: string
  description?: string
  open: boolean
  isPending?: boolean
  onClose: () => void
  onConfirm: (data: { value: string; description: string }) => void
}

export function OverwriteSecretDialog({ secretName, description: initialDescription = '', open, isPending, onClose, onConfirm }: OverwriteSecretDialogProps) {
  const [value, setValue] = useState('')
  const [description, setDescription] = useState(initialDescription)
  const [revealed, setRevealed] = useState(false)

  const handleClose = () => {
    setValue('')
    setDescription(initialDescription)
    setRevealed(false)
    onClose()
  }

  const handleConfirm = () => {
    if (!value.trim()) return
    onConfirm({ value, description: description.trim() })
  }

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[420px]">
        <DialogHeader>
          <DialogTitle>
            Change value for{' '}
            <span className="font-mono text-sm font-medium">{secretName}</span>
          </DialogTitle>
          <DialogDescription>
            The current value will be permanently replaced. The previous value can't be recovered.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-1">
          <div className="space-y-1.5">
            <Label size="md" htmlFor="overwrite-value">Value</Label>
            <div className="relative">
              <Input
                id="overwrite-value"
                type={revealed ? 'text' : 'password'}
                value={value}
                onChange={e => setValue(e.target.value)}
                placeholder="••••••••••••"
                autoComplete="off"
                data-1p-ignore
                autoFocus
                className="pr-9"
              />
              <button
                type="button"
                onClick={() => setRevealed(r => !r)}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                aria-label={revealed ? 'Hide value' : 'Reveal value'}
              >
                {revealed ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
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
          <Button onClick={handleConfirm} disabled={!value.trim() || isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
