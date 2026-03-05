import { useState } from "react";
import { useParams } from "react-router";
import {
  useCustomer,
  useCustomerEntitlements,
  useEntitlementValue,
  useEntitlementGrants,
  useCreateEntitlement,
  useDeleteEntitlement,
  useCreateGrant,
} from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Plus, Trash2, ChevronDown } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import type { Entitlement } from "@/types/openmeter";

export function CustomerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: customer, isLoading, error } = useCustomer(id ?? "");
  const { data: entitlements } = useCustomerEntitlements(id ?? "");
  const [showCreateEnt, setShowCreateEnt] = useState(false);
  const createEnt = useCreateEntitlement();

  if (isLoading) return <Skeleton className="h-64 w-full" />;
  if (error) return <p className="text-destructive">Error: {error.message}</p>;
  if (!customer) return null;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{customer.name}</h2>
        <p className="text-sm text-muted-foreground">{customer.id}</p>
      </div>

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <InfoCard label="Email" value={customer.email} />
        <InfoCard label="Key" value={customer.key} />
        <InfoCard label="Currency" value={customer.currency} />
        <InfoCard label="Timezone" value={customer.timezone} />
      </div>

      <div>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-sm font-medium text-muted-foreground">Entitlements</h3>
          <Button size="xs" variant="outline" onClick={() => setShowCreateEnt(true)}>
            <Plus className="size-3" /> Add
          </Button>
        </div>

        <CreateEntitlementDialog
          open={showCreateEnt}
          onOpenChange={setShowCreateEnt}
          onSubmit={(body) => createEnt.mutate({ customerId: id!, body }, { onSuccess: () => setShowCreateEnt(false) })}
          isPending={createEnt.isPending}
        />

        {entitlements?.map((ent) => (
          <EntitlementRow key={ent.id} customerId={id!} entitlement={ent} />
        ))}
        {entitlements?.length === 0 && <p className="text-sm text-muted-foreground">No entitlements</p>}
      </div>
    </div>
  );
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg glass px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-0.5 truncate text-sm">{value || "-"}</p>
    </div>
  );
}

function EntitlementRow({ customerId, entitlement }: { customerId: string; entitlement: Entitlement }) {
  const deleteEnt = useDeleteEntitlement();
  const { data: value } = useEntitlementValue(customerId, entitlement.id);
  const { data: grants } = useEntitlementGrants(customerId, entitlement.id);
  const [showGrant, setShowGrant] = useState(false);
  const createGrant = useCreateGrant();

  return (
    <Collapsible className="mb-2 rounded-lg glass">
      <div className="flex items-center justify-between px-3 py-2">
        <CollapsibleTrigger className="flex items-center gap-1 text-sm hover:text-foreground">
          <ChevronDown className="size-3.5 text-muted-foreground" />
          <span className="font-medium text-amber">{entitlement.featureKey}</span>
          <span className="ml-2 text-xs text-muted-foreground">{entitlement.type}</span>
        </CollapsibleTrigger>
        <Button variant="ghost" size="icon-xs" onClick={() => deleteEnt.mutate({ customerId, entitlementId: entitlement.id })}>
          <Trash2 className="size-3 text-red-500" />
        </Button>
      </div>
      <CollapsibleContent className="border-t border-glass-border-honey px-3 py-2">
        <div className="grid grid-cols-4 gap-3 text-xs">
          <div>
            <p className="text-muted-foreground">Access</p>
            <p className={value?.hasAccess ? "text-green-600" : "text-destructive"}>{value?.hasAccess ? "Yes" : "No"}</p>
          </div>
          <div>
            <p className="text-muted-foreground">Balance</p>
            <p>{value?.balance ?? "-"}</p>
          </div>
          <div>
            <p className="text-muted-foreground">Usage</p>
            <p>{value?.usage ?? "-"}</p>
          </div>
          <div>
            <p className="text-muted-foreground">Overage</p>
            <p>{value?.overage ?? "-"}</p>
          </div>
        </div>

        {grants && grants.length > 0 && (
          <div className="mt-3">
            <p className="mb-1 text-xs font-medium text-muted-foreground">Grants</p>
            {grants.map((g) => (
              <div key={g.id} className="mb-1 rounded-lg border border-glass-border-honey px-2 py-1 text-xs">
                <span className="text-amber">Amount: {g.amount}</span>
                <span className="ml-2 text-muted-foreground">Priority: {g.priority}</span>
                <span className="ml-2 text-muted-foreground">Effective: {formatDateTime(g.effectiveAt)}</span>
              </div>
            ))}
          </div>
        )}

        <div className="mt-2">
          <Button size="xs" variant="outline" onClick={() => setShowGrant(true)}>
            <Plus className="size-3" /> Add Grant
          </Button>
          <CreateGrantDialog
            open={showGrant}
            onOpenChange={setShowGrant}
            onSubmit={(body) => createGrant.mutate({ customerId, entitlementId: entitlement.id, body }, { onSuccess: () => setShowGrant(false) })}
            isPending={createGrant.isPending}
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function CreateEntitlementDialog({ open, onOpenChange, onSubmit, isPending }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: unknown) => void; isPending: boolean }) {
  const [featureKey, setFeatureKey] = useState("");
  const [type, setType] = useState("metered");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Create Entitlement</DialogTitle>
          <DialogDescription>Grant a feature entitlement to this customer.</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>Feature Key <span className="text-red-500">*</span></Label>
            <Input value={featureKey} onChange={(e) => setFeatureKey(e.target.value)} />
          </div>
          <div>
            <Label>Type</Label>
            <Input value={type} onChange={(e) => setType(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button size="sm" onClick={() => onSubmit({ featureKey, type })} disabled={isPending || !featureKey}>
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CreateGrantDialog({ open, onOpenChange, onSubmit, isPending }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: unknown) => void; isPending: boolean }) {
  const [amount, setAmount] = useState("100");
  const [priority, setPriority] = useState("1");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xs">
        <DialogHeader>
          <DialogTitle>Add Grant</DialogTitle>
          <DialogDescription>Add a usage grant to this entitlement.</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>Amount</Label>
            <Input value={amount} onChange={(e) => setAmount(e.target.value)} />
          </div>
          <div>
            <Label>Priority</Label>
            <Input value={priority} onChange={(e) => setPriority(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button size="sm" onClick={() => onSubmit({ amount: Number(amount), priority: Number(priority), effectiveAt: new Date().toISOString() })} disabled={isPending}>
            Add
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
