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
  open: boolean
  isPending?: boolean
  onClose: () => void
  onConfirm: (value: string) => void
}

export function OverwriteSecretDialog({ secretName, open, isPending, onClose, onConfirm }: OverwriteSecretDialogProps) {
  const [value, setValue] = useState('')

  const handleClose = () => {
    setValue('')
    onClose()
  }

  const handleConfirm = () => {
    if (!value.trim()) return
    onConfirm(value)
  }

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[420px]">
        <DialogHeader>
          <DialogTitle>
            Update secret for{' '}
            <span className="font-mono text-sm font-medium">{secretName}</span>
          </DialogTitle>
          <DialogDescription>
            The current value will be permanently replaced. The previous value can't be recovered.
          </DialogDescription>
        </DialogHeader>

        <div className="py-1">
          <Label size="md" htmlFor="overwrite-value">New value</Label>
          <Input
            id="overwrite-value"
            type="password"
            value={value}
            onChange={e => setValue(e.target.value)}
            placeholder="••••••••••••"
            autoComplete="off"
            autoFocus
            className="mt-1.5"
          />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isPending}>Cancel</Button>
          <Button onClick={handleConfirm} disabled={!value.trim() || isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Update secret
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
