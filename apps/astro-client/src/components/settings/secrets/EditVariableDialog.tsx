import { useState } from 'react'
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
import { Loader2 } from 'lucide-react'
import type { VaultEntry } from '@/lib/vault'

interface EditVariableDialogProps {
  entry: VaultEntry
  open: boolean
  isPending?: boolean
  onClose: () => void
  onSave: (data: { value: string; description: string }) => void
}

export function EditVariableDialog({ entry, open, isPending, onClose, onSave }: EditVariableDialogProps) {
  const [value, setValue] = useState(entry.value ?? '')
  const [description, setDescription] = useState(entry.description ?? '')

  const handleSave = () => {
    onSave({ value, description: description.trim() })
  }

  const handleClose = () => {
    setValue(entry.value ?? '')
    setDescription(entry.description ?? '')
    onClose()
  }

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[480px]">
        <DialogHeader>
          <DialogTitle>
            Edit variable —{' '}
            <span className="font-mono text-sm font-medium">{entry.name}</span>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-1">
          <div className="space-y-1.5">
            <Label htmlFor="edit-value">Value</Label>
            <Input
              id="edit-value"
              value={value}
              onChange={e => setValue(e.target.value)}
              autoFocus
              autoComplete="off"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-description">
              Description <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="edit-description"
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="What is this used for?"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isPending}>Cancel</Button>
          <Button onClick={handleSave} disabled={isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Save variable
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
