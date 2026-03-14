import { useState } from "react";
import { Link } from "react-router";
import { useCustomers, useDeleteCustomer, useUpdateCustomer, useCreateSubscription, useCancelSubscription, usePlans } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Trash2, X } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import type { Customer } from "@/types/openmeter";

export function CustomersPage() {
  const { data, isLoading, error } = useCustomers();
  const deleteMut = useDeleteCustomer();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [lastClicked, setLastClicked] = useState<number | null>(null);

  const handleRowClick = (index: number, e: { shiftKey: boolean; metaKey: boolean; ctrlKey: boolean }) => {
    if (!data) return;
    const id = data[index].id;

    setSelected((prev) => {
      const next = new Set(prev);

      if (e.shiftKey && lastClicked !== null) {
        const start = Math.min(lastClicked, index);
        const end = Math.max(lastClicked, index);
        for (let i = start; i <= end; i++) next.add(data[i].id);
      } else if (e.metaKey || e.ctrlKey) {
        if (next.has(id)) next.delete(id); else next.add(id);
      } else {
        if (next.has(id)) next.delete(id); else next.add(id);
      }

      return next;
    });
    setLastClicked(index);
  };

  const toggleAll = () => {
    if (!data) return;
    setSelected((s) => s.size === data.length ? new Set() : new Set(data.map((c) => c.id)));
  };

  const selectedCustomers = data?.filter((c) => selected.has(c.id)) ?? [];

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Customers</h2>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      <BulkActions
        customers={selectedCustomers}
        onDone={() => setSelected(new Set())}
        disabled={selected.size === 0}
      />

      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-0.5 w-6">
                  <input
                    type="checkbox"
                    checked={data.length > 0 && selected.size === data.length}
                    onChange={toggleAll}
                    className="size-3 accent-amber"
                  />
                </th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">ID</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Email</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Currency</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Subscription</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((c, i) => (
                <tr
                  key={c.id}
                  className={`border-b border-comb-light hover:bg-glass-light select-none cursor-pointer ${selected.has(c.id) ? "bg-pollen/10" : ""}`}
                  onClick={(e) => {
                    if ((e.target as HTMLElement).closest("a, button")) return;
                    handleRowClick(i, e);
                  }}
                >
                  <td className="px-2 py-0.5">
                    <input
                      type="checkbox"
                      checked={selected.has(c.id)}
                      readOnly
                      className="size-3 accent-amber cursor-pointer"
                    />
                  </td>
                  <td className="px-2 py-0.5">
                    <Link to={`/openmeter/customers/${c.id}`} className="text-amber hover:underline">
                      {c.id}
                    </Link>
                  </td>
                  <td className="px-2 py-0.5">{c.name}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{c.primaryEmail || "-"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{c.currency || "-"}</td>
                  <td className="px-2 py-0.5">
                    {c.currentSubscriptionId
                      ? <span className="text-green-600">active</span>
                      : <span className="text-muted-foreground">none</span>}
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground">{c.createdAt ? formatDateTime(c.createdAt) : "-"}</td>
                  <td className="px-2 py-0.5">
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => { if (confirm(`Delete customer "${c.name}"?`)) deleteMut.mutate(c.id); }}
                    >
                      <Trash2 className="size-3 text-red-500" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function BulkActions({ customers, onDone, disabled }: { customers: Customer[]; onDone: () => void; disabled: boolean }) {
  const updateMut = useUpdateCustomer();
  const createSub = useCreateSubscription();
  const cancelSub = useCancelSubscription();
  const { data: plans } = usePlans();
  const activePlans = plans?.filter((p) => p.status === "active") ?? [];

  const [action, setAction] = useState<string>("");
  const [currency, setCurrency] = useState("USD");
  const [planKey, setPlanKey] = useState("");
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(0);

  const run = async () => {
    setRunning(true);
    setProgress(0);

    for (let i = 0; i < customers.length; i++) {
      const c = customers[i];
      try {
        if (action === "currency") {
          await updateMut.mutateAsync({
            id: c.id,
            body: { name: c.name, key: c.key, primaryEmail: c.primaryEmail, currency } as Partial<Customer>,
          });
        } else if (action === "subscribe") {
          await createSub.mutateAsync({ customerId: c.id, plan: { key: planKey } });
        } else if (action === "upgrade") {
          // Cancel current subscription, then resubscribe to latest version
          if (c.currentSubscriptionId) {
            await cancelSub.mutateAsync({ id: c.currentSubscriptionId, body: { effectiveDate: "immediately" } });
          }
          await createSub.mutateAsync({ customerId: c.id, plan: { key: planKey } });
        }
      } catch {
        // continue on error
      }
      setProgress(i + 1);
    }

    setRunning(false);
    onDone();
  };

  const needsPlan = action === "subscribe" || action === "upgrade";

  return (
    <div className={`rounded-lg glass px-3 py-2 flex items-center gap-3 ${disabled ? "opacity-50" : ""}`}>
      <span className="text-[11px] font-semibold shrink-0">
        {disabled ? "Select customers to apply bulk actions" : `${customers.length} selected`}
      </span>

      {!disabled && (
        <Button variant="ghost" size="icon-xs" onClick={onDone} title="Clear selection" className="shrink-0">
          <X className="size-3" />
        </Button>
      )}

      <Select value={action} onValueChange={(v) => setAction(v)} disabled={disabled}>
        <SelectTrigger className="w-44 h-7"><SelectValue placeholder="Choose action..." /></SelectTrigger>
        <SelectContent>
          <SelectItem value="currency">Set Currency</SelectItem>
          <SelectItem value="subscribe">Assign Subscription</SelectItem>
          <SelectItem value="upgrade">Upgrade Subscription</SelectItem>
        </SelectContent>
      </Select>

      {action === "currency" && !disabled && (
        <Input className="w-24 h-7" value={currency} onChange={(e) => setCurrency(e.target.value.toUpperCase())} placeholder="USD" maxLength={3} />
      )}

      {needsPlan && !disabled && (
        <>
          {activePlans.length > 0 ? (
            <Select value={planKey} onValueChange={setPlanKey}>
              <SelectTrigger className="w-48 h-7"><SelectValue placeholder="Select plan..." /></SelectTrigger>
              <SelectContent>
                {activePlans.map((p) => (
                  <SelectItem key={p.key} value={p.key}>{p.name} v{p.version}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input className="w-48 h-7" value={planKey} onChange={(e) => setPlanKey(e.target.value)} placeholder="plan_key" />
          )}
          {action === "upgrade" && (
            <span className="text-[9px] text-muted-foreground">Cancels current &rarr; subscribes to latest</span>
          )}
        </>
      )}

      {action && !disabled && (
        <Button
          size="xs"
          onClick={run}
          disabled={running || (needsPlan && !planKey) || (action === "currency" && !currency)}
        >
          {running ? `${progress}/${customers.length}` : "Apply"}
        </Button>
      )}
    </div>
  );
}
