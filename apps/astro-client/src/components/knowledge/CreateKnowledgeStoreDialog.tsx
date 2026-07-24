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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Spinner } from "@/components/ui/spinner";
import { useCreateKnowledgeStore } from "@/api/queries/knowledge";
import { knowledgeDetailPath } from "@/lib/routes";
import { validateStoreName, MANAGED_PROVIDERS, PROVIDER_LABELS } from "./knowledge-utils";
import type { KnowledgeProvider } from "@/lib/api";

interface CreateKnowledgeStoreDialogProps {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateKnowledgeStoreDialog({ account, open, onOpenChange }: CreateKnowledgeStoreDialogProps) {
  const [name, setName] = useState("");
  const [provider, setProvider] = useState<KnowledgeProvider | "">("");
  const [storage, setStorage] = useState("10Gi");
  const [isPublic, setIsPublic] = useState(false);
  const navigate = useNavigate();
  const create = useCreateKnowledgeStore(account);

  const nameError = validateStoreName(name);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (nameError || !name || !provider) return;
    create.mutate(
      { name, provider: provider as KnowledgeProvider, storage: storage || undefined, public: isPublic || undefined },
      {
        onSuccess: (store) => {
          onOpenChange(false);
          resetForm();
          navigate(knowledgeDetailPath(store.name, account));
        },
      }
    );
  }

  function resetForm() {
    setName("");
    setProvider("");
    setStorage("10Gi");
    setIsPublic(false);
    create.reset();
  }

  function handleOpenChange(next: boolean) {
    if (!next) resetForm();
    onOpenChange(next);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Create knowledge store</DialogTitle>
          <DialogDescription>
            Provision a managed database for your agents.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 py-1">
          <div className="space-y-1.5">
            <label htmlFor="ks-name" className="text-sm font-medium">Name</label>
            <Input
              id="ks-name"
              placeholder="my-store"
              value={name}
              onChange={(e) => { setName(e.target.value); create.reset(); }}
              autoComplete="off"
              autoFocus
            />
            {name && nameError && <p className="text-xs text-destructive">{nameError}</p>}
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium">Provider</label>
            <Select value={provider} onValueChange={(v) => setProvider(v as KnowledgeProvider)}>
              <SelectTrigger>
                <SelectValue placeholder="Select a provider" />
              </SelectTrigger>
              <SelectContent>
                {MANAGED_PROVIDERS.map((p) => (
                  <SelectItem key={p} value={p}>{PROVIDER_LABELS[p]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <label htmlFor="ks-storage" className="text-sm font-medium">Storage</label>
            <Input
              id="ks-storage"
              placeholder="10Gi"
              value={storage}
              onChange={(e) => setStorage(e.target.value)}
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">Kubernetes quantity (e.g. 10Gi, 500Mi)</p>
          </div>

          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Public access</p>
              <p className="text-xs text-muted-foreground">Expose with a DNS hostname</p>
            </div>
            <Switch checked={isPublic} onCheckedChange={setIsPublic} />
          </div>

          {create.isError && (
            <p className="text-xs text-destructive">
              {create.error instanceof Error ? create.error.message : "Failed to create store"}
            </p>
          )}

          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name || !provider || !!nameError || create.isPending}>
              {create.isPending && <Spinner size={14} className="mr-2" />}
              Create
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
