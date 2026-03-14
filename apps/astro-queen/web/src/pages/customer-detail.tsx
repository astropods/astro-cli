import { useState } from "react";
import { useParams } from "react-router";
import {
  useCustomer, useUpdateCustomer, useCustomerEntitlements, useEntitlementValue, useEntitlementGrants,
  useCreateEntitlement, useDeleteEntitlement, useCreateGrant,
  useSubscription, useCreateSubscription, useCancelSubscription, usePlans,
} from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Trash2, ChevronDown, XCircle, Plus, Pencil, Check, X as XIcon } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import { SchemaFormPanel } from "@/components/schema-form-panel";
import type { Entitlement, Customer } from "@/types/openmeter";

export function CustomerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: customer, isLoading, error } = useCustomer(id ?? "");
  const { data: entitlements } = useCustomerEntitlements(id ?? "");
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
      <CustomerInfo customer={customer} />

      <SubscriptionSection customer={customer} />

      <div>
        <h3 className="mb-2 text-sm font-medium text-muted-foreground">Entitlements</h3>
        <div className="flex gap-4 items-start">
          <div className="min-w-0 flex-1">
            {entitlements?.map((ent) => <EntitlementRow key={ent.id} customerId={id!} entitlement={ent} />)}
            {entitlements?.length === 0 && <p className="text-sm text-muted-foreground">No entitlements</p>}
          </div>
          <SchemaFormPanel
            title="Create Entitlement"
            description="Grant a feature entitlement to this customer."
            schemaRef="EntitlementCreate"
            onSubmit={(body) => createEnt.mutate({ customerId: id!, body })}
            isPending={createEnt.isPending}
          />
        </div>
      </div>
    </div>
  );
}

function SubscriptionSection({ customer }: { customer: Customer }) {
  const subId = customer.currentSubscriptionId;
  const { data: sub, isLoading } = useSubscription(subId ?? "");
  const cancelMut = useCancelSubscription();
  const [showSubscribe, setShowSubscribe] = useState(false);

  return (
    <div>
      <h3 className="mb-2 text-sm font-medium text-muted-foreground">Subscription</h3>
      {isLoading && subId && <Skeleton className="h-16 w-full" />}
      {sub && (
        <div className="rounded-lg glass px-3 py-2 space-y-1.5">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium">{sub.name}</span>
              <span className={`text-[10px] ${
                sub.status === "active" ? "text-green-600" :
                sub.status === "canceled" ? "text-red-500" :
                sub.status === "scheduled" ? "text-blue-500" :
                "text-muted-foreground"
              }`}>
                {sub.status}
              </span>
              {sub.plan && (
                <span className="text-[10px] text-muted-foreground">
                  Plan: {sub.plan.key} v{sub.plan.version}
                </span>
              )}
            </div>
            {sub.status === "active" && (
              <Button
                variant="ghost"
                size="icon-xs"
                title="Cancel subscription"
                onClick={() => {
                  if (confirm(`Cancel subscription "${sub.name}"?`))
                    cancelMut.mutate({ id: sub.id, body: { effectiveDate: "immediately" } });
                }}
              >
                <XCircle className="size-3 text-red-500" />
              </Button>
            )}
          </div>
          <div className="grid grid-cols-2 gap-x-6 gap-y-0.5 text-[10px] md:grid-cols-4">
            <div><span className="text-muted-foreground">Currency:</span> {sub.currency}</div>
            <div><span className="text-muted-foreground">Billing:</span> {sub.billingCadence}</div>
            <div><span className="text-muted-foreground">Active from:</span> {formatDateTime(sub.activeFrom)}</div>
            {sub.activeTo && <div><span className="text-muted-foreground">Active to:</span> {formatDateTime(sub.activeTo)}</div>}
          </div>
        </div>
      )}
      {!subId && !showSubscribe && (
        <div className="flex items-center gap-3">
          <p className="text-sm text-muted-foreground">No active subscription.</p>
          <Button size="xs" variant="outline" onClick={() => setShowSubscribe(true)}>
            Subscribe to Plan
          </Button>
        </div>
      )}
      {showSubscribe && (
        <SubscribeForm customerId={customer.id} onDone={() => setShowSubscribe(false)} />
      )}
    </div>
  );
}

function SubscribeForm({ customerId, onDone }: { customerId: string; onDone: () => void }) {
  const createSub = useCreateSubscription();
  const { data: plans } = usePlans();
  const activePlans = plans?.filter((p) => p.status === "active") ?? [];
  const [planKey, setPlanKey] = useState("");
  const [name, setName] = useState("");

  const handleSubmit = () => {
    const body: Record<string, unknown> = {
      customerId,
      plan: { key: planKey },
    };
    if (name) body.name = name;
    createSub.mutate(body, { onSuccess: () => onDone() });
  };

  return (
    <div className="rounded-lg glass p-3 space-y-2">
      <h4 className="text-[11px] font-semibold">Subscribe to Plan</h4>
      <FieldGroup className="gap-2">
        <div className="grid grid-cols-2 gap-2">
          <Field className="gap-1">
            <FieldLabel className="text-[10px]">Plan *</FieldLabel>
            {activePlans.length > 0 ? (
              <Select value={planKey} onValueChange={setPlanKey}>
                <SelectTrigger><SelectValue placeholder="Select a plan" /></SelectTrigger>
                <SelectContent>
                  {activePlans.map((p) => (
                    <SelectItem key={p.key} value={p.key}>
                      {p.name} ({p.key})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input value={planKey} onChange={(e) => setPlanKey(e.target.value)} placeholder="plan_key" />
            )}
          </Field>
          <Field className="gap-1">
            <FieldLabel className="text-[10px]">Name</FieldLabel>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Optional" />
          </Field>
        </div>
      </FieldGroup>
      {createSub.error && <p className="text-[10px] text-destructive">{createSub.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button variant="outline" size="xs" onClick={onDone}>Cancel</Button>
        <Button size="xs" onClick={handleSubmit} disabled={!planKey || createSub.isPending}>
          Subscribe
        </Button>
      </div>
    </div>
  );
}

function CustomerInfo({ customer }: { customer: Customer }) {
  const updateMut = useUpdateCustomer();
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

  const startEdit = (field: string, value: string) => {
    setEditing(field);
    setDraft(value);
  };

  const save = () => {
    if (!editing) return;
    updateMut.mutate({
      id: customer.id,
      body: {
        name: customer.name,
        key: customer.key,
        primaryEmail: customer.primaryEmail,
        currency: customer.currency,
        [editing]: draft,
      } as Partial<Customer>,
    });
    setEditing(null);
  };

  const cancel = () => setEditing(null);

  const rows: { label: string; field?: string; value: string }[] = [
    { label: "Key", value: customer.key },
    { label: "Email", field: "primaryEmail", value: customer.primaryEmail },
    { label: "Currency", field: "currency", value: customer.currency },
  ];

  return (
    <div className="text-xs space-y-1">
      {rows.map(({ label, field, value }) => (
        <div key={label} className="flex items-center gap-2">
          <span className="text-muted-foreground w-16 shrink-0">{label}</span>
          {editing === field ? (
            <div className="flex items-center gap-1">
              <Input
                className="h-5 w-48 text-xs"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") save(); if (e.key === "Escape") cancel(); }}
                autoFocus
              />
              <Button variant="ghost" size="icon-xs" onClick={save}><Check className="size-3 text-green-600" /></Button>
              <Button variant="ghost" size="icon-xs" onClick={cancel}><XIcon className="size-3 text-muted-foreground" /></Button>
            </div>
          ) : (
            <span className="flex items-center gap-1">
              {value || "-"}
              {field && (
                <Button variant="ghost" size="icon-xs" onClick={() => startEdit(field, value)} title={`Edit ${label}`}>
                  <Pencil className="size-2.5 text-muted-foreground" />
                </Button>
              )}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

function EntitlementRow({ customerId, entitlement }: { customerId: string; entitlement: Entitlement }) {
  const deleteEnt = useDeleteEntitlement();
  const { data: value } = useEntitlementValue(customerId, entitlement.id);
  const { data: grants } = useEntitlementGrants(customerId, entitlement.id);
  const createGrant = useCreateGrant();
  const [showGrant, setShowGrant] = useState(false);

  return (
    <Collapsible className="mb-2 rounded-lg glass">
      <div className="flex items-center justify-between px-3 py-2">
        <CollapsibleTrigger className="flex items-center gap-1 text-sm hover:text-foreground">
          <ChevronDown className="size-3.5 text-muted-foreground" />
          <span className="font-medium text-amber">{entitlement.featureKey}</span>
          <span className="ml-2 text-xs text-muted-foreground">{entitlement.type}</span>
        </CollapsibleTrigger>
        <Button variant="ghost" size="icon-xs" onClick={() => deleteEnt.mutate({ customerId, entitlementId: entitlement.id })}><Trash2 className="size-3 text-red-500" /></Button>
      </div>
      <CollapsibleContent className="border-t border-glass-border-honey px-3 py-2">
        <div className="grid grid-cols-4 gap-3 text-xs">
          <div><p className="text-muted-foreground">Access</p><p className={value?.hasAccess ? "text-green-600" : "text-destructive"}>{value?.hasAccess ? "Yes" : "No"}</p></div>
          <div><p className="text-muted-foreground">Balance</p><p>{value?.balance ?? "-"}</p></div>
          <div><p className="text-muted-foreground">Usage</p><p>{value?.usage ?? "-"}</p></div>
          <div><p className="text-muted-foreground">Overage</p><p>{value?.overage ?? "-"}</p></div>
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
          {showGrant ? (
            <>
              <SchemaFormPanel
                title="Add Grant"
                description="Add a usage grant to this entitlement."
                schemaRef="GrantCreate"
                submitLabel="Add"
                defaults={{ effectiveAt: new Date().toISOString(), priority: 1 }}
                onSubmit={(body) => {
                  createGrant.mutate({ customerId, entitlementId: entitlement.id, body }, {
                    onSuccess: () => setShowGrant(false),
                  });
                }}
                isPending={createGrant.isPending}
                className="w-full static"
              />
              <div className="flex justify-end mt-1">
                <Button variant="ghost" size="xs" onClick={() => setShowGrant(false)}>Cancel</Button>
              </div>
            </>
          ) : (
            <Button variant="outline" size="xs" onClick={() => setShowGrant(true)}>
              <Plus className="size-3 mr-1" /> Add Grant
            </Button>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
