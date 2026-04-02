import { useState } from "react";
import { useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { useCreateBlueprint } from "@/api/queries";

interface CreateBlueprintDialogProps {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateBlueprintDialog({ account, open, onOpenChange }: CreateBlueprintDialogProps) {
  const [name, setName] = useState("");
  const navigate = useNavigate();
  const create = useCreateBlueprint(account);

  const nameError = validateName(name);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (nameError || !name) return;
    create.mutate(
      { name },
      {
        onSuccess: (data) => {
          onOpenChange(false);
          setName("");
          navigate(`/${data.account}/${data.name}`);
        },
      }
    );
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setName("");
      create.reset();
    }
    onOpenChange(next);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Create blueprint</DialogTitle>
          <DialogDescription>
            Give your agent a name. You can connect a GitHub repo and push builds afterwards.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 py-1">
          <div className="space-y-1.5">
            <label htmlFor="bp-name" className="text-sm font-medium">
              Name
            </label>
            <Input
              id="bp-name"
              placeholder="my-agent"
              value={name}
              onChange={(e) => { setName(e.target.value); create.reset(); }}
              autoComplete="off"
              autoFocus
            />
            {name && nameError && (
              <p className="text-xs text-destructive">{nameError}</p>
            )}
            {create.isError && (
              <p className="text-xs text-destructive">
                {create.error instanceof Error ? create.error.message : "Failed to create blueprint"}
              </p>
            )}
          </div>

          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name || !!nameError || create.isPending}>
              {create.isPending && <Spinner size={14} className="mr-2" />}
              Create
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function validateName(name: string): string | null {
  if (!name) return null;
  if (name.length < 4) return "Name must be at least 4 characters";
  if (name.length > 39) return "Name must be at most 39 characters";
  if (!/^[a-z]/.test(name)) return "Name must start with a lowercase letter";
  if (name.endsWith("-")) return "Name must not end with a hyphen";
  if (/--/.test(name)) return "Name must not contain consecutive hyphens";
  if (!/^[a-z0-9-]+$/.test(name)) return "Name must contain only lowercase letters, digits, and hyphens";
  return null;
}
