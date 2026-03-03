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
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Plus, Trash2, ChevronDown, X } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import type { Entitlement } from "@/types/openmeter";

export function CustomerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: customer, isLoading, error } = useCustomer(id ?? "");
  const { data: entitlements } = useCustomerEntitlements(id ?? "");
  const [showCreateEnt, setShowCreateEnt] = useState(false);
  const createEnt = useCreateEntitlement();

  if (isLoading) return <Skeleton className="h-64 w-full" />;
  if (error) return <p className="text-red-400">Error: {error.message}</p>;
  if (!customer) return null;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{customer.name}</h2>
        <p className="text-sm text-stone-500">{customer.id}</p>
      </div>

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <InfoCard label="Email" value={customer.email} />
        <InfoCard label="Key" value={customer.key} />
        <InfoCard label="Currency" value={customer.currency} />
        <InfoCard label="Timezone" value={customer.timezone} />
      </div>

      <div>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-sm font-medium text-stone-400">Entitlements</h3>
          <Button size="xs" variant="outline" onClick={() => setShowCreateEnt(true)}>
            <Plus className="size-3" /> Add
          </Button>
        </div>

        {showCreateEnt && (
          <CreateEntitlementForm
            customerId={id!}
            onClose={() => setShowCreateEnt(false)}
            onSubmit={(body) => createEnt.mutate({ customerId: id!, body }, { onSuccess: () => setShowCreateEnt(false) })}
            isPending={createEnt.isPending}
          />
        )}

        {entitlements?.map((ent) => (
          <EntitlementRow key={ent.id} customerId={id!} entitlement={ent} />
        ))}
        {entitlements?.length === 0 && <p className="text-sm text-stone-500">No entitlements</p>}
      </div>
    </div>
  );
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-stone-800 bg-stone-900/30 px-3 py-2">
      <p className="text-xs text-stone-500">{label}</p>
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
    <Collapsible className="mb-2 rounded-md border border-stone-800 bg-stone-900/30">
      <div className="flex items-center justify-between px-3 py-2">
        <CollapsibleTrigger className="flex items-center gap-1 text-sm hover:text-stone-200">
          <ChevronDown className="size-3.5 text-stone-500" />
          <span className="font-medium text-amber">{entitlement.featureKey}</span>
          <span className="ml-2 text-xs text-stone-500">{entitlement.type}</span>
        </CollapsibleTrigger>
        <Button variant="ghost" size="icon-xs" onClick={() => deleteEnt.mutate({ customerId, entitlementId: entitlement.id })}>
          <Trash2 className="size-3 text-red-400" />
        </Button>
      </div>
      <CollapsibleContent className="border-t border-stone-800 px-3 py-2">
        <div className="grid grid-cols-4 gap-3 text-xs">
          <div>
            <p className="text-stone-500">Access</p>
            <p className={value?.hasAccess ? "text-green-400" : "text-red-400"}>{value?.hasAccess ? "Yes" : "No"}</p>
          </div>
          <div>
            <p className="text-stone-500">Balance</p>
            <p>{value?.balance ?? "-"}</p>
          </div>
          <div>
            <p className="text-stone-500">Usage</p>
            <p>{value?.usage ?? "-"}</p>
          </div>
          <div>
            <p className="text-stone-500">Overage</p>
            <p>{value?.overage ?? "-"}</p>
          </div>
        </div>

        {grants && grants.length > 0 && (
          <div className="mt-3">
            <p className="mb-1 text-xs font-medium text-stone-400">Grants</p>
            {grants.map((g) => (
              <div key={g.id} className="mb-1 rounded border border-stone-800 px-2 py-1 text-xs">
                <span className="text-amber">Amount: {g.amount}</span>
                <span className="ml-2 text-stone-500">Priority: {g.priority}</span>
                <span className="ml-2 text-stone-500">Effective: {formatDateTime(g.effectiveAt)}</span>
              </div>
            ))}
          </div>
        )}

        <div className="mt-2">
          {showGrant ? (
            <CreateGrantForm
              onClose={() => setShowGrant(false)}
              onSubmit={(body) => createGrant.mutate({ customerId, entitlementId: entitlement.id, body }, { onSuccess: () => setShowGrant(false) })}
              isPending={createGrant.isPending}
            />
          ) : (
            <Button size="xs" variant="outline" onClick={() => setShowGrant(true)}>
              <Plus className="size-3" /> Add Grant
            </Button>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function CreateEntitlementForm({ customerId: _customerId, onClose, onSubmit, isPending }: { customerId: string; onClose: () => void; onSubmit: (body: unknown) => void; isPending: boolean }) {
  const [featureKey, setFeatureKey] = useState("");
  const [type, setType] = useState("metered");

  return (
    <div className="mb-3 rounded-md border border-stone-800 bg-stone-900/50 p-3">
      <div className="mb-2 flex items-center justify-between">
        <p className="text-xs font-medium">Create Entitlement</p>
        <Button variant="ghost" size="icon-xs" onClick={onClose}><X className="size-3" /></Button>
      </div>
      <div className="flex gap-2">
        <Input placeholder="Feature key" value={featureKey} onChange={(e) => setFeatureKey(e.target.value)} className="flex-1" />
        <Input placeholder="Type" value={type} onChange={(e) => setType(e.target.value)} className="w-32" />
        <Button size="sm" onClick={() => onSubmit({ featureKey, type })} disabled={isPending || !featureKey}>Create</Button>
      </div>
    </div>
  );
}

function CreateGrantForm({ onClose, onSubmit, isPending }: { onClose: () => void; onSubmit: (body: unknown) => void; isPending: boolean }) {
  const [amount, setAmount] = useState("100");
  const [priority, setPriority] = useState("1");

  return (
    <div className="rounded border border-stone-800 bg-stone-950 p-2">
      <div className="mb-1 flex items-center justify-between">
        <p className="text-xs font-medium">Add Grant</p>
        <Button variant="ghost" size="icon-xs" onClick={onClose}><X className="size-3" /></Button>
      </div>
      <div className="flex gap-2">
        <Input placeholder="Amount" value={amount} onChange={(e) => setAmount(e.target.value)} className="w-24" />
        <Input placeholder="Priority" value={priority} onChange={(e) => setPriority(e.target.value)} className="w-20" />
        <Button size="xs" onClick={() => onSubmit({ amount: Number(amount), priority: Number(priority), effectiveAt: new Date().toISOString() })} disabled={isPending}>
          Add
        </Button>
      </div>
    </div>
  );
}
