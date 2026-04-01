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
import { Switch } from '@/components/ui/switch'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Loader2, Info } from 'lucide-react'
import type { CreateAccountVariableInput } from '@/lib/api'

interface NewEntryDialogProps {
  open: boolean
  isPending?: boolean
  onClose: () => void
  onCreate: (data: CreateAccountVariableInput) => void
}

const NAME_PATTERN = /^[A-Z][A-Z0-9_]*$/

export function NewEntryDialog({ open, isPending, onClose, onCreate }: NewEntryDialogProps) {
  const [name, setName] = useState('')
  const [value, setValue] = useState('')
  const [description, setDescription] = useState('')
  const [isSecret, setIsSecret] = useState(false)
  const [nameError, setNameError] = useState('')

  const isValid = NAME_PATTERN.test(name) && value.trim().length > 0

  const handleNameChange = (v: string) => {
    const upper = v.toUpperCase().replace(/[^A-Z0-9_]/g, '')
    setName(upper)
    if (upper && !NAME_PATTERN.test(upper)) {
      setNameError('Must start with a letter and contain only A–Z, 0–9, _')
    } else {
      setNameError('')
    }
  }

  const handleCreate = () => {
    if (!isValid) return
    onCreate({
      name,
      value,
      secret: isSecret,
      description: description.trim() || undefined,
    })
  }

  const handleClose = () => {
    setName('')
    setValue('')
    setDescription('')
    setIsSecret(false)
    setNameError('')
    onClose()
  }

  return (
    <Dialog open={open} onOpenChange={open => !open && handleClose()}>
      <DialogContent className="max-w-[480px]">
        <DialogHeader>
          <DialogTitle>New {isSecret ? 'secret' : 'variable'}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-1">
          {/* Name */}
          <div className="space-y-1.5">
            <Label size="md" htmlFor="entry-name">Name</Label>
            <Input
              id="entry-name"
              value={name}
              onChange={e => handleNameChange(e.target.value)}
              placeholder="EXAMPLE_API_KEY"
              variant="code"
              autoFocus
              aria-invalid={!!nameError || undefined}
            />
            {nameError ? (
              <p className="text-destructive text-xs">{nameError}</p>
            ) : (
              <p className="text-xs text-muted-foreground">Uppercase letters, numbers, and underscores only</p>
            )}
          </div>

          {/* Value + Secret toggle */}
          <div className="space-y-1.5">
            <Label size="md" htmlFor="entry-value">Value</Label>
            <div className="flex items-center gap-2">
              <Input
                id="entry-value"
                type={isSecret ? 'password' : 'text'}
                value={value}
                onChange={e => setValue(e.target.value)}
                placeholder={isSecret ? 'Your secret value...' : 'Enter value'}
                autoComplete="off"
              />
              <div className="flex items-center gap-1.5 shrink-0">
                <label htmlFor="secret-toggle" className="text-sm font-medium text-foreground cursor-pointer whitespace-nowrap">Secret</label>
                <TooltipProvider delayDuration={200}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Info className="size-3.5 text-muted-foreground cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent side="top" className="max-w-[200px] text-xs">
                      Encrypted at rest. Value can't be read after saving.
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
                <Switch
                  id="secret-toggle"
                  checked={isSecret}
                  onCheckedChange={setIsSecret}
                />
              </div>
            </div>
            {isSecret && (
              <p className="text-xs text-muted-foreground">This value can't be read after saving</p>
            )}
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <Label size="md" htmlFor="entry-description">
              Description <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="entry-description"
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="What is this used for?"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isPending}>Cancel</Button>
          <Button onClick={handleCreate} disabled={!isValid || isPending}>
            {isPending && <Loader2 className="size-3.5 animate-spin" />}
            Create {isSecret ? 'secret' : 'variable'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
